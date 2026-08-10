package taskcommander_test

import (
	"context"
	"testing"
	"time"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/task_management_layer/postman"
	"github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_board"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_commander"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_executor"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_planner"
	"github.com/cpmores/lucinda/internal/testutil"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// TestPlannerHasNoProviderDep verifies the planner no longer resolves a
// ProviderController.
func TestPlannerHasNoProviderDep(t *testing.T) {
	planner := taskplanner.NewTaskPlanner(logger.Discard())
	am, ok := planner.(modulemanager.AvailableModule)
	if !ok {
		t.Fatal("planner is not an AvailableModule")
	}
	if _, ok := am.DependsOn()[APIModule.ProviderController]; ok {
		t.Fatal("planner still depends on ProviderController")
	}
}

// TestCommanderHasNoProviderDeps verifies the commander no longer resolves a
// ProviderController or StreamRouter.
func TestCommanderHasNoProviderDeps(t *testing.T) {
	cmdr := taskcommander.NewTaskCommander(logger.Discard())
	am, ok := cmdr.(modulemanager.AvailableModule)
	if !ok {
		t.Fatal("commander is not an AvailableModule")
	}
	for _, dep := range []APIModule.ModuleType{APIModule.ProviderController, APIModule.StreamRouter} {
		if _, ok := am.DependsOn()[dep]; ok {
			t.Fatalf("commander still depends on %s", dep)
		}
	}
}

// pfNode is one mesh node in the provider-free cross-node test.
type pfNode struct {
	eb     *eventbus.InMemoryEventBus
	tp     *testutil.MockTransport
	router streamrouter.StreamRouter
}

// newPFNode builds a node. withProvider adds an executor + a provider;
// withCommander adds a commander (whose node has no provider).
func newPFNode(t *testing.T, id string, withProvider, withCommander bool) *pfNode {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	log := logger.Discard()
	mm := modulemanager.NewModuleManager()
	eb := eventbus.NewInMemoryEventBus(log.Child(id + "-eb"))
	tp := testutil.NewMockTransport(id)
	eb.RegisterWithManager(mm)
	tp.RegisterWithManager(mm)

	pc := testutil.NewMockProviderController()
	if withProvider {
		pc.Providers = append(pc.Providers, testutil.NewMockProvider(id+"-prov", nil))
	}
	pc.RegisterWithManager(mm)

	pm := taskpostman.NewTaskPostman(log.Child(id + "-pm"))
	streamR := streamrouter.NewStreamRouter(log.Child(id + "-router"))
	board := taskboard.NewTaskBoard(log.Child(id + "-board"))
	tr := tasktracer.NewTaskTracer(log.Child(id + "-tracer"))
	pm.RegisterWithManager(mm)
	streamR.RegisterWithManager(mm)
	board.RegisterWithManager(mm)
	tr.RegisterWithManager(mm)

	var ex taskexecutor.TaskExecutor
	var cmdr taskcommander.TaskCommander
	if withProvider {
		ex = taskexecutor.NewTaskExecutor(log.Child(id + "-executor"))
		ex.RegisterWithManager(mm)
	}
	if withCommander {
		cmdr = taskcommander.NewTaskCommander(log.Child(id + "-commander"))
		cmdr.RegisterWithManager(mm)
	}

	if err := mm.VerifyInit(); err != nil {
		t.Fatalf("%s VerifyInit: %v", id, err)
	}
	if err := mm.EnableDeps(); err != nil {
		t.Fatalf("%s EnableDeps: %v", id, err)
	}
	start := func(name string, fn func(context.Context) error) {
		if err := fn(ctx); err != nil {
			t.Fatalf("%s %s: %v", id, name, err)
		}
	}
	start("postman", pm.Start)
	start("stream", streamR.Start)
	start("board", board.Start)
	if ex != nil {
		start("executor", ex.Start)
	}
	if cmdr != nil {
		start("commander", cmdr.Start)
	}
	return &pfNode{eb: eb, tp: tp, router: streamR}
}

// TestProviderFreeCrossNodeReAct drives a ReAct loop where the commander's
// node has no provider: reasoning, execution, and synthesis are all served by
// a remote node, and the final answer streams back over the mesh.
func TestProviderFreeCrossNodeReAct(t *testing.T) {
	owner := newPFNode(t, "node-A", false, true)  // commander, no provider
	worker := newPFNode(t, "node-B", true, false) // provider + executor
	owner.tp.Peer = worker.tp
	worker.tp.Peer = owner.tp

	done := owner.eb.Subscribe(APIEvent.TaskPlanDone, 4)
	streamCh := owner.router.Subscribe("plan-xnode")

	plan := &APITask.TaskPlan{
		ID: "plan-xnode", Owner: "node-A",
		Architecture: APITask.ArchReAct, Goal: "cross node work",
		Transactions: []APITask.Transaction{{ID: "tx-1", Goal: "work"}},
	}
	_ = owner.eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	select {
	case ev := <-done:
		msg, ok := ev.Data.(APITaskmsg.TaskPlanResultMsg)
		if !ok {
			t.Fatalf("got %T, want TaskPlanResultMsg", ev.Data)
		}
		if msg.Result.Status != APITask.PlanOK {
			t.Fatalf("status = %s, want ok", msg.Result.Status)
		}
		if msg.Result.Text == "" {
			t.Fatal("empty result text")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for cross-node ReAct plan")
	}

	// The owner's node had no provider — the reason/execute/synthesize tasks
	// must have been assigned to the worker over the transport.
	if len(owner.tp.Sent) == 0 {
		t.Fatal("owner never sent any assignment to the worker")
	}
	// The final answer streamed across the mesh to the owner's router.
	select {
	case c := <-streamCh:
		if c.Delta == "" && !c.Done {
			t.Fatalf("unexpected chunk: %+v", c)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no stream chunk reached the owner's router")
	}
}

package taskboard_test

import (
	"context"
	"testing"
	"time"

	APIProvider "github.com/cpmores/lucinda/api/v1/domain/provider"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/task_management_layer/postman"
	"github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_board"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_executor"
	"github.com/cpmores/lucinda/internal/testutil"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

type bn struct {
	eb *eventbus.InMemoryEventBus
	tp *testutil.MockTransport
	pm taskpostman.TaskPostman
}

// newBoardNode wires one mesh node: eventbus + mock transport + a provider
// serving `model` + postman/board/executor, all started.
func newBoardNode(t *testing.T, id, model string) *bn {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	log := logger.Discard()
	mm := modulemanager.NewModuleManager()
	eb := eventbus.NewInMemoryEventBus(log.Child(id + "-eb"))
	tp := testutil.NewMockTransport(id)
	pc := testutil.NewMockProviderController(testutil.NewMockProvider(id+"-prov", []APIProvider.ModelInfo{
		{ID: model, Labels: map[string]string{"modality": "text"}, ContextTokens: 2048},
	}))

	eb.RegisterWithManager(mm)
	tp.RegisterWithManager(mm)
	pc.RegisterWithManager(mm)

	pm := taskpostman.NewTaskPostman(log.Child(id + "-pm"))
	streamR := streamrouter.NewStreamRouter(log.Child(id + "-stream"))
	board := taskboard.NewTaskBoard(log.Child(id + "-board"))
	tr := tasktracer.NewTaskTracer(log.Child(id + "-tracer"))
	ex := taskexecutor.NewTaskExecutor(log.Child(id + "-ex"))
	pm.RegisterWithManager(mm)
	streamR.RegisterWithManager(mm)
	board.RegisterWithManager(mm)
	tr.RegisterWithManager(mm)
	ex.RegisterWithManager(mm)

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
	start("executor", ex.Start)

	return &bn{eb: eb, tp: tp, pm: pm}
}

// TestPublishLeaseAcrossNodes exercises the full protocol: the employer
// advertises a task whose model only the worker serves, the worker bids,
// the employer assigns it remotely, the worker's executor runs it, and the
// result flows back to the employer.
func TestPublishLeaseAcrossNodes(t *testing.T) {
	employer := newBoardNode(t, "node-A", "m1")
	worker := newBoardNode(t, "node-B", "worker-only-model")
	employer.tp.Peer = worker.tp
	worker.tp.Peer = employer.tp

	// The employer's commander is subscribed to TaskTraced.
	doneCh := employer.eb.Subscribe(APIEvent.TaskTraced, 16)

	// Employer publishes a ready task requiring the worker-only model.
	planID := APITask.TaskID("plan-PL")
	task := &APITask.Task{
		Meta:     APITask.TaskMeta{ID: "t-1", Type: APITask.StageExecute, Owner: "node-A"},
		Spec:     APITask.TaskSpec{Prompt: "render something", Model: "worker-only-model", BudgetTokens: 100},
		TaskPlan: &APITask.TaskPlan{ID: planID, Owner: "node-A"},
	}
	_ = employer.eb.Publish(APIEvent.TaskReady, APIEvent.NewEvent(APIEvent.TaskReady, task))

	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-doneCh:
			msg, ok := ev.Data.(APITaskmsg.TaskTracedMsg)
			if !ok {
				t.Fatalf("employer got non-TaskTracedMsg: %T", ev.Data)
			}
			if msg.State != APITask.StateDone {
				continue // skip Running etc.; wait for the terminal Done
			}
			if msg.TaskID != "t-1" || msg.PlanID != "plan-PL" {
				t.Fatalf("unexpected result: %+v", msg)
			}
			if msg.Output == "" || len(msg.Output) < 4 {
				t.Fatalf("output empty or trivial: %q", msg.Output)
			}
			goto gotDone
		case <-deadline:
			t.Fatal("employer timed out waiting for remote result")
		}
	}
gotDone:

	// The worker must have received the assignment over the transport
	// (employer → worker), proving remote execution happened.
	gotAssign := false
	for _, m := range employer.tp.Sent {
		if m.Topic == string(APIEvent.TaskAssign) && string(m.To) == "node-B" {
			gotAssign = true
		}
	}
	if !gotAssign {
		t.Fatal("employer never sent TaskAssign to the worker")
	}
}

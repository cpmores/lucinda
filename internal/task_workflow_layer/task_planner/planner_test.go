package taskplanner_test

import (
	"context"
	"testing"
	"time"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	"github.com/cpmores/lucinda/internal/task_management_layer/postman"
	"github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_board"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_executor"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_planner"
	"github.com/cpmores/lucinda/internal/testutil"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

func setupPlanner(t *testing.T) (*testutil.MockProvider, *eventbus.InMemoryEventBus) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	log := logger.Discard()
	mm := modulemanager.NewModuleManager()
	eb := eventbus.NewInMemoryEventBus(log.Child("eb"))
	mockTp := testutil.NewMockTransport("node-A")
	prov := testutil.NewMockProvider("mock", nil)
	pc := testutil.NewMockProviderController(prov)
	eb.RegisterWithManager(mm)
	mockTp.RegisterWithManager(mm)
	pc.RegisterWithManager(mm)

	pm := taskpostman.NewTaskPostman(log.Child("postman"))
	streamR := streamrouter.NewStreamRouter(log.Child("stream"))
	board := taskboard.NewTaskBoard(log.Child("board"))
	tr := tasktracer.NewTaskTracer(log.Child("tracer"))
	ex := taskexecutor.NewTaskExecutor(log.Child("executor"))
	planner := taskplanner.NewTaskPlanner(log.Child("planner"))

	pm.RegisterWithManager(mm)
	streamR.RegisterWithManager(mm)
	board.RegisterWithManager(mm)
	tr.RegisterWithManager(mm)
	ex.RegisterWithManager(mm)
	planner.RegisterWithManager(mm)

	if err := mm.VerifyInit(); err != nil {
		t.Fatalf("VerifyInit: %v", err)
	}
	if err := mm.EnableDeps(); err != nil {
		t.Fatalf("EnableDeps: %v", err)
	}
	start := func(name string, fn func(context.Context) error) {
		if err := fn(ctx); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	start("postman", pm.Start)
	start("stream", streamR.Start)
	start("board", board.Start)
	start("executor", ex.Start)
	start("planner", planner.Start)
	return prov, eb
}

func TestReActArchitecturePropagation(t *testing.T) {
	_, eb := setupPlanner(t)
	planned := eb.Subscribe(APIEvent.TaskPlanned, 8)

	task := &APITask.Task{
		Meta: APITask.TaskMeta{ID: "plan-1", Owner: "node-A"},
		Spec: APITask.TaskSpec{Prompt: "answer the question"},
		TaskPlan: &APITask.TaskPlan{
			ID: "plan-1", Owner: "node-A", Architecture: APITask.ArchReAct,
		},
	}
	_ = eb.Publish(APIEvent.TaskPreplanned, APIEvent.NewEvent(APIEvent.TaskPreplanned, task))

	select {
	case ev := <-planned:
		plan, ok := ev.Data.(*APITask.TaskPlan)
		if !ok {
			t.Fatalf("got %T, want *TaskPlan", ev.Data)
		}
		if plan.Architecture != APITask.ArchReAct {
			t.Fatalf("architecture = %s, want react", plan.Architecture)
		}
		if plan.Goal != "answer the question" {
			t.Fatalf("goal = %q, want the raw prompt", plan.Goal)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for TaskPlanned")
	}
}

// TestSemanticDecomposition verifies the planner produces semantic
// transactions with dependencies instead of a node DAG.
func TestSemanticDecomposition(t *testing.T) {
	_, eb := setupPlanner(t)
	planned := eb.Subscribe(APIEvent.TaskPlanned, 8)

	task := &APITask.Task{
		Meta: APITask.TaskMeta{ID: "plan-s", Owner: "node-A"},
		Spec: APITask.TaskSpec{Prompt: "do a and then b"},
		TaskPlan: &APITask.TaskPlan{ID: "plan-s", Owner: "node-A"},
	}
	_ = eb.Publish(APIEvent.TaskPreplanned, APIEvent.NewEvent(APIEvent.TaskPreplanned, task))

	select {
	case ev := <-planned:
		plan := ev.Data.(*APITask.TaskPlan)
		if len(plan.Transactions) != 2 {
			t.Fatalf("transactions = %d, want 2", len(plan.Transactions))
		}
		t1, t2 := plan.Transactions[0], plan.Transactions[1]
		if t1.Goal != "step one" || t2.Goal != "step two" {
			t.Fatalf("unexpected goals: %q, %q", t1.Goal, t2.Goal)
		}
		if len(t2.Deps) != 1 || t2.Deps[0] != t1.ID {
			t.Fatalf("t2 deps = %v, want [%s]", t2.Deps, t1.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for TaskPlanned")
	}
}

// TestCycleRejection verifies a cyclic transaction graph falls back to a
// single transaction instead of producing an un-executable plan.
func TestCycleRejection(t *testing.T) {
	prov, eb := setupPlanner(t)
	planned := eb.Subscribe(APIEvent.TaskPlanned, 8)

	prov.PlanOut = `{"transactions":[{"id":"a","goal":"a","deps":["b"]},{"id":"b","goal":"b","deps":["a"]}]}`

	task := &APITask.Task{
		Meta: APITask.TaskMeta{ID: "plan-cyc", Owner: "node-A"},
		Spec: APITask.TaskSpec{Prompt: "cyclic"},
		TaskPlan: &APITask.TaskPlan{ID: "plan-cyc", Owner: "node-A"},
	}
	_ = eb.Publish(APIEvent.TaskPreplanned, APIEvent.NewEvent(APIEvent.TaskPreplanned, task))

	select {
	case ev := <-planned:
		plan := ev.Data.(*APITask.TaskPlan)
		if len(plan.Transactions) != 1 {
			t.Fatalf("transactions = %d, want 1 (fallback after cycle)", len(plan.Transactions))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for TaskPlanned")
	}
}

func TestDefaultArchitectureIsPlanExecute(t *testing.T) {
	_, eb := setupPlanner(t)
	planned := eb.Subscribe(APIEvent.TaskPlanned, 8)

	task := &APITask.Task{
		Meta: APITask.TaskMeta{ID: "plan-2", Owner: "node-A"},
		Spec: APITask.TaskSpec{Prompt: "write a story"},
		TaskPlan: &APITask.TaskPlan{ID: "plan-2", Owner: "node-A"}, // no architecture
	}
	_ = eb.Publish(APIEvent.TaskPreplanned, APIEvent.NewEvent(APIEvent.TaskPreplanned, task))

	select {
	case ev := <-planned:
		plan := ev.Data.(*APITask.TaskPlan)
		if plan.Architecture != APITask.ArchPlanExecute {
			t.Fatalf("architecture = %s, want plan_execute", plan.Architecture)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for TaskPlanned")
	}
}

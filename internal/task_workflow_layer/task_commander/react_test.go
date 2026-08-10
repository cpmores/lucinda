package taskcommander_test

import (
	"context"
	"strings"
	"testing"
	"time"

	APISteam "github.com/cpmores/lucinda/api/v1/domain/stream"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/task_management_layer/postman"
	"github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_board"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_commander"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_executor"
	"github.com/cpmores/lucinda/internal/testutil"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

func setupReAct(t *testing.T) (*testutil.MockProvider, *eventbus.InMemoryEventBus) {
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
	cmdr := taskcommander.NewTaskCommander(log.Child("commander"))

	pm.RegisterWithManager(mm)
	streamR.RegisterWithManager(mm)
	board.RegisterWithManager(mm)
	tr.RegisterWithManager(mm)
	ex.RegisterWithManager(mm)
	cmdr.RegisterWithManager(mm)

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
	start("commander", cmdr.Start)

	return prov, eb
}

// TestReActLoop drives one full reasoning cycle: continue (issue a subtask),
// observe its result, then done (final answer from the LLM).
func TestReActLoop(t *testing.T) {
	prov, eb := setupReAct(t)
	prov.ReActOut = []string{
		`{"action":"continue","task":{"prompt":"subtask","model":"mock-model"}}`,
		`{"action":"done","answer":"one-shot fallback"}`,
	}
	// The final answer must come from the STREAMED synthesis, not the
	// one-shot fallback.
	prov.StreamOut = []string{"streamed ", "answer 42"}

	done := eb.Subscribe(APIEvent.TaskPlanDone, 4)

	plan := &APITask.TaskPlan{
		ID:           "plan-react",
		Owner:        "node-A",
		Architecture: APITask.ArchReAct,
		Goal:         "answer the meaning of life",
	}
	_ = eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	select {
	case ev := <-done:
		msg, ok := ev.Data.(APITaskmsg.TaskPlanResultMsg)
		if !ok {
			t.Fatalf("got %T, want TaskPlanResultMsg", ev.Data)
		}
		if msg.Result.Status != APITask.PlanOK {
			t.Fatalf("status = %s, want ok", msg.Result.Status)
		}
		// The streamed answer proves the whole loop ran AND that the final
		// answer streamed instead of using the one-shot fallback.
		if msg.Result.Text != "streamed answer 42" {
			t.Fatalf("final text = %q, want %q (streamed)", msg.Result.Text, "streamed answer 42")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the ReAct loop to finish")
	}
}

// TestReActMaxStepsCap bounds the loop: MaxSteps=1 means only one action is
// issued before the commander finalizes, even though the LLM keeps saying
// continue.
func TestReActMaxStepsCap(t *testing.T) {
	prov, eb := setupReAct(t)
	prov.ReActOut = []string{
		`{"action":"continue","task":{"prompt":"do one","model":"mock-model"}}`,
		`{"action":"continue","task":{"prompt":"do two","model":"mock-model"}}`,
	}

	done := eb.Subscribe(APIEvent.TaskPlanDone, 4)
	ready := eb.Subscribe(APIEvent.TaskReady, 8)

	plan := &APITask.TaskPlan{
		ID: "plan-cap", Owner: "node-A",
		Architecture: APITask.ArchReAct, Goal: "bounded",
		MaxSteps: 1,
	}
	_ = eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	deadline := time.After(5 * time.Second)
	select {
	case <-deadline:
		t.Fatal("timed out waiting for the capped loop to finalize")
	case <-done:
	}

	issued := 0
	for {
		select {
		case ev := <-ready:
			task, ok := ev.Data.(*APITask.Task)
			if ok && task.Spec.Kind == APITask.TaskKindExecute {
				issued++
			}
		case <-time.After(100 * time.Millisecond):
			goto counted
		}
	}
counted:
	if issued != 1 {
		t.Fatalf("issued %d execute actions, want 1 (max-steps cap)", issued)
	}
}

// TestReActLLMFailureFallback finalizes with collected outputs when the
// reasoning LLM returns garbage.
func TestReActLLMFailureFallback(t *testing.T) {
	prov, eb := setupReAct(t)
	prov.ReActOut = []string{"this is not json"}

	done := eb.Subscribe(APIEvent.TaskPlanDone, 4)
	plan := &APITask.TaskPlan{
		ID: "plan-fb", Owner: "node-A",
		Architecture: APITask.ArchReAct, Goal: "unparseable",
	}
	_ = eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	select {
	case ev := <-done:
		msg := ev.Data.(APITaskmsg.TaskPlanResultMsg)
		if msg.Result.Status != APITask.PlanOK {
			t.Fatalf("status = %s, want ok (graceful fallback)", msg.Result.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the fallback to finalize")
	}
}

// TestReActActionFailure terminates the plan with PlanError when a
// decision-task's executor fails.
func TestReActActionFailure(t *testing.T) {
	prov, eb := setupReAct(t)
	prov.ReActOut = []string{`{"action":"continue","task":{"prompt":"boom step","model":"mock-model"}}`}
	prov.ErrPrompt = "boom step"

	done := eb.Subscribe(APIEvent.TaskPlanDone, 4)
	plan := &APITask.TaskPlan{
		ID: "plan-fail", Owner: "node-A",
		Architecture: APITask.ArchReAct, Goal: "will fail",
	}
	_ = eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	select {
	case ev := <-done:
		msg := ev.Data.(APITaskmsg.TaskPlanResultMsg)
		if msg.Result.Status != APITask.PlanError {
			t.Fatalf("status = %s, want error", msg.Result.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the failure to terminate the plan")
	}
}

// TestReActReasoningTelemetry emits a status frame per iteration.
func TestReActReasoningTelemetry(t *testing.T) {
	prov, eb := setupReAct(t)
	prov.ReActOut = []string{
		`{"action":"continue","task":{"prompt":"subtask","model":"mock-model"}}`,
		`{"action":"done","answer":"final"}`,
	}

	thinking := eb.Subscribe(APIEvent.TelemetryThinking, 16)
	plan := &APITask.TaskPlan{
		ID: "plan-telemetry", Owner: "node-A",
		Architecture: APITask.ArchReAct, Goal: "show steps",
	}
	_ = eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-thinking:
			sd, ok := ev.Data.(APISteam.StatusData)
			if !ok {
				t.Fatalf("got %T, want StatusData", ev.Data)
			}
			if sd.State != "react step 1" {
				continue // skip the initial "thinking" frame
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for reasoning telemetry")
		}
	}
}

func waitPlanDone(t *testing.T, done <-chan APIEvent.Event) string {
	t.Helper()
	select {
	case ev := <-done:
		msg, ok := ev.Data.(APITaskmsg.TaskPlanResultMsg)
		if !ok {
			t.Fatalf("got %T, want TaskPlanResultMsg", ev.Data)
		}
		if msg.Result.Status != APITask.PlanOK {
			t.Fatalf("status = %s, want ok", msg.Result.Status)
		}
		return msg.Result.Text
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for plan done")
		return ""
	}
}

// TestParallelTransactions runs two independent transactions; both results
// must appear in the merged final answer.
func TestParallelTransactions(t *testing.T) {
	_, eb := setupReAct(t)
	done := eb.Subscribe(APIEvent.TaskPlanDone, 4)

	plan := &APITask.TaskPlan{
		ID: "plan-par", Owner: "node-A",
		Architecture: APITask.ArchPlanExecute,
		Transactions: []APITask.Transaction{
			{ID: "tx-a", Goal: "do alpha"},
			{ID: "tx-b", Goal: "do beta"},
		},
	}
	_ = eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	text := waitPlanDone(t, done)
	if !strings.Contains(text, "RESULT:do alpha") || !strings.Contains(text, "RESULT:do beta") {
		t.Fatalf("parallel transactions missing an output: %q", text)
	}
}

// TestDependentTransactions verifies ordering: a transaction waits for its
// dependency, so its result appears after the dependency's in the merge.
func TestDependentTransactions(t *testing.T) {
	_, eb := setupReAct(t)
	done := eb.Subscribe(APIEvent.TaskPlanDone, 4)

	plan := &APITask.TaskPlan{
		ID: "plan-dep", Owner: "node-A",
		Architecture: APITask.ArchPlanExecute,
		Transactions: []APITask.Transaction{
			{ID: "tx-1", Goal: "first"},
			{ID: "tx-2", Goal: "second", Deps: []APITask.TaskID{"tx-1"}},
		},
	}
	_ = eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	text := waitPlanDone(t, done)
	i1, i2 := strings.Index(text, "RESULT:first"), strings.Index(text, "RESULT:second")
	if i1 < 0 || i2 < 0 {
		t.Fatalf("dependent transactions missing an output: %q", text)
	}
	if i1 > i2 {
		t.Fatalf("tx-2 result appears before its dependency: %q", text)
	}
}

// TestDependencyFeedsDownstream verifies a ReAct transaction's reasoning
// prompt includes its completed dependency's output.
func TestDependencyFeedsDownstream(t *testing.T) {
	prov, eb := setupReAct(t)
	prov.ReActOut = []string{
		`{"action":"continue","task":{"prompt":"step1","model":"mock-model"}}`,
		`{"action":"done","answer":"T1-done"}`,
		`{"action":"done","answer":"T2-done"}`,
	}
	prov.StreamOut = []string{"T1-", "result"}

	done := eb.Subscribe(APIEvent.TaskPlanDone, 4)
	plan := &APITask.TaskPlan{
		ID: "plan-feed", Owner: "node-A",
		Architecture: APITask.ArchReAct,
		Transactions: []APITask.Transaction{
			{ID: "tx-1", Goal: "one"},
			{ID: "tx-2", Goal: "two", Deps: []APITask.TaskID{"tx-1"}},
		},
	}
	_ = eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))

	text := waitPlanDone(t, done)
	// tx-2's reasoning prompt (the last orchestrator call) must carry tx-1's
	// streamed output.
	if !strings.Contains(prov.LastReactPrompt, "T1-result") {
		t.Fatalf("downstream prompt missing dependency output: %q", prov.LastReactPrompt)
	}
	if !strings.Contains(text, "T1-result") {
		t.Fatalf("final answer missing dependency output: %q", text)
	}
}

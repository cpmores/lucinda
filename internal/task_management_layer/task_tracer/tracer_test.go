package tasktracer_test

import (
	"testing"
	"time"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

func TestTracerTracksAndEmits(t *testing.T) {
	log := logger.Discard()
	mm := modulemanager.NewModuleManager()
	eb := eventbus.NewInMemoryEventBus(log.Child("eb"))
	eb.RegisterWithManager(mm)

	tr := tasktracer.NewTaskTracer(log.Child("tracer"))
	tr.RegisterWithManager(mm)

	if err := mm.VerifyInit(); err != nil {
		t.Fatalf("VerifyInit: %v", err)
	}
	if err := mm.EnableDeps(); err != nil {
		t.Fatalf("EnableDeps: %v", err)
	}

	traced := eb.Subscribe(APIEvent.TaskTraced, 8)

	// Import a local task → tracked as Ready, one TaskTraced emitted.
	task := &APITask.Task{Meta: APITask.TaskMeta{ID: "t-1", Owner: "node-A"}}
	if err := tr.Import(task); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if got, ok := tr.Get("t-1"); !ok || got.TaskNode.State != APITask.StateReady {
		t.Fatalf("after Import: got=%+v ok=%v, want StateReady", got, ok)
	}
	expectTraced(t, traced, APITask.StateReady)

	// Update → emits the new state.
	if err := tr.Update("t-1", APITask.StateDone); err != nil {
		t.Fatalf("Update: %v", err)
	}
	expectTraced(t, traced, APITask.StateDone)

	// Assigned task lands in its own registry.
	if err := tr.Assigned(&APITask.Task{Meta: APITask.TaskMeta{ID: "a-1", Owner: "node-B"}}); err != nil {
		t.Fatalf("Assigned: %v", err)
	}
	if n := len(tr.ListAssigned()); n != 1 {
		t.Fatalf("ListAssigned = %d, want 1", n)
	}
	expectTraced(t, traced, APITask.StateRunning)

	// Remove drops it everywhere.
	if err := tr.Remove("a-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := tr.Get("a-1"); ok {
		t.Fatal("a-1 still tracked after Remove")
	}
}

func expectTraced(t *testing.T, ch <-chan APIEvent.Event, want APITask.NodeState) {
	t.Helper()
	select {
	case ev := <-ch:
		msg, ok := ev.Data.(APITaskmsg.TaskTracedMsg)
		if !ok {
			t.Fatalf("TaskTraced payload = %T, want TaskTracedMsg", ev.Data)
		}
		if msg.State != want {
			t.Fatalf("traced state = %v, want %v", msg.State, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for TaskTraced with state %v", want)
	}
}

package taskpostman_test

import (
	"context"
	"testing"
	"time"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/testutil"
	"github.com/cpmores/lucinda/internal/task_management_layer/postman"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

type pmNode struct {
	eb *eventbus.InMemoryEventBus
	tp *testutil.MockTransport
	pm taskpostman.TaskPostman
}

func newPMNode(t *testing.T, id string) *pmNode {
	t.Helper()
	log := logger.Discard()
	mm := modulemanager.NewModuleManager()
	eb := eventbus.NewInMemoryEventBus(log.Child(id + "-eb"))
	tp := testutil.NewMockTransport(id)
	eb.RegisterWithManager(mm)
	tp.RegisterWithManager(mm)

	pm := taskpostman.NewTaskPostman(log.Child(id + "-postman"))
	pm.RegisterWithManager(mm)

	if err := mm.VerifyInit(); err != nil {
		t.Fatalf("%s VerifyInit: %v", id, err)
	}
	if err := mm.EnableDeps(); err != nil {
		t.Fatalf("%s EnableDeps: %v", id, err)
	}
	return &pmNode{eb: eb, tp: tp, pm: pm}
}

func startPair(t *testing.T, a, b *pmNode) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := a.pm.Start(ctx); err != nil {
		t.Fatalf("a start: %v", err)
	}
	if err := b.pm.Start(ctx); err != nil {
		t.Fatalf("b start: %v", err)
	}
	a.tp.Peer = b.tp
	b.tp.Peer = a.tp
}

func TestWorkerResultReachesOwner(t *testing.T) {
	owner := newPMNode(t, "node-A")
	worker := newPMNode(t, "node-B")
	startPair(t, owner, worker)

	// The owner's commander is subscribed to TaskTraced locally.
	got := owner.eb.Subscribe(APIEvent.TaskTraced, 8)

	// Worker executor traces a completed task owned by node-A; the postman
	// transparently forwards the TaskTraced to the owner.
	_ = worker.eb.Publish(APIEvent.TaskTraced, APIEvent.NewEvent(APIEvent.TaskTraced, APITaskmsg.TaskTracedMsg{
		TaskID: "t-1", PlanID: "plan-X", State: APITask.StateDone, Output: "done", Owner: "node-A",
	}))

	select {
	case ev := <-got:
		msg, ok := ev.Data.(APITaskmsg.TaskTracedMsg)
		if !ok {
			t.Fatalf("owner got non-TaskTracedMsg: %T", ev.Data)
		}
		if msg.TaskID != "t-1" || msg.Output != "done" || msg.PlanID != "plan-X" {
			t.Fatalf("unexpected result: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("owner timed out waiting for remote result")
	}

	// The worker must have unicast exactly one message to the owner.
	if len(worker.tp.Sent) != 1 || string(worker.tp.Sent[0].To) != "node-A" {
		t.Fatalf("worker sent %d messages, want 1 to node-A: %+v", len(worker.tp.Sent), worker.tp.Sent)
	}
}

func TestSendEventUnicast(t *testing.T) {
	owner := newPMNode(t, "node-A")
	worker := newPMNode(t, "node-B")
	startPair(t, owner, worker)

	got := owner.eb.Subscribe(APIEvent.TaskAssigned, 8)

	assign := APITaskmsg.TaskAssignMsg{
		TaskID: "t-2", Spec: APITask.TaskSpec{Model: "m1"}, Prompt: "do it",
		Owner: "node-A", PlanID: "plan-Y",
	}
	if err := worker.pm.SendEvent(context.Background(), owner.tp.ID(), APIEvent.TaskAssigned, assign); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}

	select {
	case ev := <-got:
		msg, ok := ev.Data.(APITaskmsg.TaskAssignMsg)
		if !ok {
			t.Fatalf("owner got non-TaskAssignMsg: %T", ev.Data)
		}
		if msg.TaskID != "t-2" || msg.PlanID != "plan-Y" {
			t.Fatalf("unexpected assignment: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("owner timed out waiting for assignment")
	}
}

func TestLocalResultNotForwarded(t *testing.T) {
	owner := newPMNode(t, "node-A")
	worker := newPMNode(t, "node-B")
	startPair(t, owner, worker)

	// A completion owned locally (node-A) must stay local, not cross the wire.
	_ = owner.eb.Publish(APIEvent.TaskTraced, APIEvent.NewEvent(APIEvent.TaskTraced, APITaskmsg.TaskTracedMsg{
		TaskID: "t-3", PlanID: "plan-Z", State: APITask.StateDone, Output: "local", Owner: "node-A",
	}))

	time.Sleep(50 * time.Millisecond) // allow any (incorrect) forwarding to happen
	if n := len(owner.tp.Sent); n != 0 {
		t.Fatalf("local result was forwarded %d times over transport", n)
	}
}

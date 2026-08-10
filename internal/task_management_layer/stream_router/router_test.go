package streamrouter

import (
	"context"
	"testing"
	"time"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/testutil"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
)

func newRouterForTest(t *testing.T, mock *testutil.MockTransport) *router {
	t.Helper()
	return &router{
		tp:   mock,
		log:  logger.Discard(),
		subs: make(map[string][]chan APITaskmsg.StreamChunkMsg),
	}
}

func TestDemuxByPlanID(t *testing.T) {
	mock := testutil.NewMockTransport("node-A")
	r := newRouterForTest(t, mock)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	chA := r.Subscribe("plan-A")
	chB := r.Subscribe("plan-B")

	// All chunks owned locally (node-A) → delivered directly.
	r.Send(context.Background(), APITaskmsg.StreamChunkMsg{
		PlanID: APITask.TaskID("plan-A"), Delta: "a1", Owner: "node-A",
	})
	r.Send(context.Background(), APITaskmsg.StreamChunkMsg{
		PlanID: APITask.TaskID("plan-B"), Delta: "b1", Owner: "node-A",
	})
	r.Send(context.Background(), APITaskmsg.StreamChunkMsg{
		PlanID: APITask.TaskID("plan-A"), Delta: "a2", Done: true, Owner: "node-A",
	})

	expectChunks(t, chA, "plan-A", []string{"a1", "a2"})
	expectChunks(t, chB, "plan-B", []string{"b1"})

	// No cross-talk: chA must be drained/exhausted after a2 (Done).
	select {
	case c, ok := <-chA:
		t.Fatalf("chA leaked cross-plan chunk: %+v ok=%v", c, ok)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSendRoutesToRemoteOwner(t *testing.T) {
	mock := testutil.NewMockTransport("node-A")
	r := newRouterForTest(t, mock)
	r.Start(context.Background())

	chunk := APITaskmsg.StreamChunkMsg{
		PlanID: APITask.TaskID("plan-A"), Delta: "hello", Owner: "node-B",
	}
	if err := r.Send(context.Background(), chunk); err != nil {
		t.Fatalf("send: %v", err)
	}

	if len(mock.Sent) != 1 {
		t.Fatalf("expected 1 transport message, got %d", len(mock.Sent))
	}
	msg := mock.Sent[0]
	if string(msg.To) != "node-B" {
		t.Errorf("message addressed to %q, want node-B", msg.To)
	}
}

func expectChunks(t *testing.T, ch <-chan APITaskmsg.StreamChunkMsg, wantPlan string, wantDeltas []string) {
	t.Helper()
	timeout := time.After(500 * time.Millisecond)
	for i, want := range wantDeltas {
		select {
		case c := <-ch:
			if string(c.PlanID) != wantPlan {
				t.Fatalf("chunk %d on %s: plan %s, want %s", i, wantPlan, c.PlanID, wantPlan)
			}
			if c.Delta != want {
				t.Fatalf("chunk %d on %s: delta %q, want %q", i, wantPlan, c.Delta, want)
			}
		case <-timeout:
			t.Fatalf("chunk %d on %s: timed out waiting for %q", i, wantPlan, want)
		}
	}
}

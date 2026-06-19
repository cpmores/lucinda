package taskstatemanager

import (
	"context"
	"testing"
	"time"

	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	APITask "github.com/cpmores/lucinda/api/v1/task"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
)

func makePlan() *APITask.TaskPlan {
	return &APITask.TaskPlan{
		ID:    "plan-1",
		Owner: "user-1",
		Roots: []APITask.TaskID{"parse", "profile"},
		Nodes: map[APITask.TaskID]*APITask.TaskNode{
			"parse": {
				ID: "parse", Spec: APITask.TaskSpec{
				Model: "gemma3", BudgetTokens: 100, Priority: 0},
			},
			"profile": {
				ID: "profile", Spec: APITask.TaskSpec{
				Model: "gemma3", BudgetTokens: 100, Priority: 0},
			},
			"extract": {
				ID: "extract", Spec: APITask.TaskSpec{
				Model: "gemma3", BudgetTokens: 200, Priority: 0},
			},
			"analyze": {
				ID: "analyze", Spec: APITask.TaskSpec{
				Model: "gemma3", BudgetTokens: 300, Priority: 0},
			},
			"reduce": {
				ID: "reduce", Spec: APITask.TaskSpec{
				Model: "gemma3", BudgetTokens: 150, Priority: 0},
			},
		},
		Successors: map[APITask.TaskID][]APITask.TaskID{
			"parse":   {"extract"},
			"profile": {"extract"},
			"extract": {"analyze"},
			"analyze": {"reduce"},
		},
		PredecessorNums: map[APITask.TaskID]int{
			"extract": 2,
			"analyze": 1,
			"reduce":  1,
		},
		Deadline:  time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now(),
	}
}

func TestIngestPublishesRoots(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)

	plan := makePlan()

	pubCh := eb.Subscribe(APIEvent.TaskReady, 10)
	reopenCh := eb.Subscribe(APIEvent.TaskRepublished, 10)

	if err := mgr.Ingest(plan); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Two root nodes should be published.
	published := drainEvents(pubCh, 2, 500*time.Millisecond)
	if len(published) != 2 {
		t.Fatalf("expected 2 TaskPublished events, got %d", len(published))
	}

	// Verify root nodes are Ready.
	for _, e := range published {
		node, ok := e.Data.(*APITask.TaskNode)
		if !ok {
			t.Fatalf("expected TaskNode data, got %T", e.Data)
		}
		if node.State != APITask.StateReady {
			t.Fatalf("expected Ready, got %s for node %s", node.State, node.ID)
		}
	}

	// No reopened events should fire on ingest.
	if len(drainEvents(reopenCh, 1, 100*time.Millisecond)) > 0 {
		t.Fatal("unexpected TaskRepublished event on ingest")
	}
}

func TestIngestDuplicate(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)

	if err := mgr.Ingest(makePlan()); err != nil {
		t.Fatalf("first Ingest: %v", err)
	}
	if err := mgr.Ingest(makePlan()); err == nil {
		t.Fatal("expected error on duplicate ingest")
	}
}

func TestClaimAndStart(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	// Drain initial publish events.
	ch := eb.Subscribe(APIEvent.TaskReady, 10)
	drainEvents(ch, 2, 500*time.Millisecond)

	// Claim a ready node.
	ctx := context.Background()
	if err := mgr.Claim(ctx, "parse", "peer-a", 60); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	status, _ := mgr.Status("plan-1")
	if status["parse"] != APITask.StateClaimed {
		t.Fatalf("expected Claimed, got %s", status["parse"])
	}

	// Start it.
	if err := mgr.Start("parse"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status, _ = mgr.Status("plan-1")
	if status["parse"] != APITask.StateRunning {
		t.Fatalf("expected Running, got %s", status["parse"])
	}
}

func TestClaimNotReady(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	// Drain initial publishes.
	ch := eb.Subscribe(APIEvent.TaskReady, 10)
	drainEvents(ch, 2, 500*time.Millisecond)

	// Claim extract before its deps are done — it's Pending, not Ready.
	ctx := context.Background()
	if err := mgr.Claim(ctx, "extract", "peer-a", 60); err == nil {
		t.Fatal("expected error claiming pending node, got nil")
	}
}

func TestDoubleClaim(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	ch := eb.Subscribe(APIEvent.TaskReady, 10)
	drainEvents(ch, 2, 500*time.Millisecond)

	ctx := context.Background()
	mgr.Claim(ctx, "parse", "peer-a", 60)
	if err := mgr.Claim(ctx, "parse", "peer-b", 60); err == nil {
		t.Fatal("expected error on double claim")
	}
}

func TestStartNotClaimed(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	if err := mgr.Start("parse"); err == nil {
		t.Fatal("expected error starting unclaimed node")
	}
}

func TestCompleteUnblocksSuccessors(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	pubCh := eb.Subscribe(APIEvent.TaskReady, 10)
	drainEvents(pubCh, 2, 500*time.Millisecond) // roots

	ctx := context.Background()

	// Complete parse (first dependency of extract).
	mgr.Claim(ctx, "parse", "peer-a", 60)
	mgr.Start("parse")
	if err := mgr.Complete("parse"); err != nil {
		t.Fatalf("Complete parse: %v", err)
	}

	// Extract should still be Pending (needs parse AND profile).
	status, _ := mgr.Status("plan-1")
	if status["extract"] != APITask.StatePending {
		t.Fatalf("expected Pending after one dep done, got %s", status["extract"])
	}

	// Complete profile (second dependency of extract).
	mgr.Claim(ctx, "profile", "peer-a", 60)
	mgr.Start("profile")
	mgr.Complete("profile")

	// Now extract should be published as Ready.
	published := drainEvents(pubCh, 1, 500*time.Millisecond)
	if len(published) != 1 {
		t.Fatalf("expected 1 TaskPublished for extract, got %d", len(published))
	}
	node := published[0].Data.(*APITask.TaskNode)
	if node.ID != "extract" || node.State != APITask.StateReady {
		t.Fatalf("expected extract Ready, got %s/%s", node.ID, node.State)
	}
}

func TestCompleteNotRunning(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	if err := mgr.Complete("parse"); err == nil {
		t.Fatal("expected error completing non-running node")
	}
}

func TestFailedPublishesReopened(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	pubCh := eb.Subscribe(APIEvent.TaskReady, 10)
	reopenCh := eb.Subscribe(APIEvent.TaskRepublished, 10)
	drainEvents(pubCh, 2, 500*time.Millisecond)

	ctx := context.Background()
	mgr.Claim(ctx, "parse", "peer-a", 60)
	mgr.Start("parse")

	if err := mgr.Failed("parse"); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	events := drainEvents(reopenCh, 1, 500*time.Millisecond)
	if len(events) != 1 {
		t.Fatalf("expected 1 TaskRepublished, got %d", len(events))
	}
	node := events[0].Data.(*APITask.TaskNode)
	if node.ID != "parse" || node.State != APITask.StateFailed {
		t.Fatalf("expected parse Failed, got %s/%s", node.ID, node.State)
	}

	// Node should be unclaimed.
	plan, _ := mgr.Plan("plan-1")
	if plan.Nodes["parse"].ClaimedBy != "" {
		t.Fatal("ClaimedBy should be empty after failure")
	}
}

func TestExpired(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	pubCh := eb.Subscribe(APIEvent.TaskReady, 10)
	drainEvents(pubCh, 2, 500*time.Millisecond)

	// Claim normally, then force expiration.
	ctx := context.Background()
	mgr.Claim(ctx, "parse", "peer-a", 60)
	plan, _ := mgr.Plan("plan-1")
	plan.Nodes["parse"].ExpiresAt = time.Now().Unix() - 1 // already expired

	expired := mgr.Expired()
	if len(expired) != 1 || expired[0].ID != "parse" {
		t.Fatalf("expected parse expired, got %d nodes", len(expired))
	}
}

func TestPlanQuery(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	plan, err := mgr.Plan("plan-1")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(plan.Nodes))
	}
}

func TestPlanNotFound(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)

	if _, err := mgr.Plan("nope"); err == nil {
		t.Fatal("expected error for unknown plan")
	}
}

func TestIsComplete(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	pubCh := eb.Subscribe(APIEvent.TaskReady, 10)
	drainEvents(pubCh, 2, 500*time.Millisecond) // roots

	// Initial state.
	done, _ := mgr.IsComplete("plan-1")
	if done {
		t.Fatal("should not be complete initially")
	}

	ctx := context.Background()
	// Walk the full DAG: parse+profile → extract → analyze → reduce.
	for _, id := range []APITask.TaskID{"parse", "profile"} {
		mgr.Claim(ctx, id, "peer-a", 60)
		mgr.Start(id)
		mgr.Complete(id)
	}

	// Drain extract publish.
	drainEvents(pubCh, 1, 500*time.Millisecond)
	mgr.Claim(ctx, "extract", "peer-a", 60)
	mgr.Start("extract")
	mgr.Complete("extract")

	// Drain analyze publish.
	drainEvents(pubCh, 1, 500*time.Millisecond)
	mgr.Claim(ctx, "analyze", "peer-a", 60)
	mgr.Start("analyze")
	mgr.Complete("analyze")

	// Drain reduce publish.
	drainEvents(pubCh, 1, 500*time.Millisecond)
	mgr.Claim(ctx, "reduce", "peer-a", 60)
	mgr.Start("reduce")
	mgr.Complete("reduce")

	done, _ = mgr.IsComplete("plan-1")
	if !done {
		t.Fatal("should be complete after all nodes done")
	}
}

func TestClaimLeaseExpiryReopens(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	pubCh := eb.Subscribe(APIEvent.TaskReady, 10)
	reopenCh := eb.Subscribe(APIEvent.TaskRepublished, 10)
	drainEvents(pubCh, 2, 500*time.Millisecond)

	// Claim with 1s TTL, never start.
	ctx := context.Background()
	if err := mgr.Claim(ctx, "parse", "peer-a", 1); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// Wait for lease to expire and watchdog to fire.
	events := drainEvents(reopenCh, 1, 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 TaskRepublished after lease expiry, got %d", len(events))
	}
	node := events[0].Data.(*APITask.TaskNode)
	if node.ID != "parse" || node.State != APITask.StateReady {
		t.Fatalf("expected parse Ready after expiry, got %s/%s", node.ID, node.State)
	}
}

func TestIdempotentStartAndComplete(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	mgr := NewTaskStateManager(eb)
	mgr.Ingest(makePlan())

	pubCh := eb.Subscribe(APIEvent.TaskReady, 10)
	drainEvents(pubCh, 2, 500*time.Millisecond)

	ctx := context.Background()
	mgr.Claim(ctx, "parse", "peer-a", 60)
	mgr.Start("parse")

	// Second Start should be a no-op.
	if err := mgr.Start("parse"); err != nil {
		t.Fatalf("second Start should be idempotent: %v", err)
	}

	mgr.Complete("parse")

	// Second Complete should be a no-op.
	if err := mgr.Complete("parse"); err != nil {
		t.Fatalf("second Complete should be idempotent: %v", err)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

func drainEvents(ch <-chan APIEvent.Event, count int, timeout time.Duration) []APIEvent.Event {
	var events []APIEvent.Event
	deadline := time.After(timeout)
	for len(events) < count {
		select {
		case e := <-ch:
			events = append(events, e)
		case <-deadline:
			return events
		}
	}
	return events
}

package taskboard

import (
	"context"
	"fmt"
	"sync"
	"testing"

	APICapability "github.com/cpmores/lucinda/api/v1/domain/capability"
	APINode "github.com/cpmores/lucinda/api/v1/domain/node"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	taskpostman "github.com/cpmores/lucinda/internal/task_management_layer/task_postman"
	tasktracer "github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
)

type mockTransport struct {
	mu     sync.Mutex
	nodeID APINode.NodeID
	pub    []APINode.NodeMessage
	sent   []APINode.NodeMessage
	ins    map[APINode.Protocol]chan APINode.NodeMessage
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		nodeID: "mock-node",
		ins:    make(map[APINode.Protocol]chan APINode.NodeMessage),
	}
}

func (m *mockTransport) ID() APINode.NodeID              { return m.nodeID }
func (m *mockTransport) Start(ctx context.Context) error { return nil }
func (m *mockTransport) Stop() error                     { return nil }
func (m *mockTransport) Open(ctx context.Context, p APINode.Protocol) error {
	m.mu.Lock()
	m.ins[p] = make(chan APINode.NodeMessage, 10)
	m.mu.Unlock()
	return nil
}
func (m *mockTransport) Close(ctx context.Context, p APINode.Protocol) error {
	m.mu.Lock()
	delete(m.ins, p)
	m.mu.Unlock()
	return nil
}
func (m *mockTransport) Incoming(p APINode.Protocol) (<-chan APINode.NodeMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.ins[p]
	if !ok {
		return nil, fmt.Errorf("protocol not open")
	}
	return ch, nil
}
func (m *mockTransport) Send(ctx context.Context, to APINode.NodeID, msg APINode.NodeMessage) error {
	m.mu.Lock()
	m.sent = append(m.sent, msg)
	m.mu.Unlock()
	return nil
}
func (m *mockTransport) Publish(ctx context.Context, msg APINode.NodeMessage) error {
	m.mu.Lock()
	m.pub = append(m.pub, msg)
	m.mu.Unlock()
	return nil
}

func (m *mockTransport) Published() []APINode.NodeMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]APINode.NodeMessage{}, m.pub...)
}
func (m *mockTransport) Sent() []APINode.NodeMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]APINode.NodeMessage{}, m.sent...)
}

func testBoard(t *testing.T) (*board, *mockTransport, *eventbus.InMemoryEventBus, tasktracer.TaskTracer) {
	t.Helper()
	eb := eventbus.NewInMemoryEventBus()
	tp := newMockTransport()
	pm := taskpostman.NewTaskPostman(eb)
	tt := tasktracer.NewTaskTracer()

	b := &board{
		tp:      tp,
		pm:      pm,
		tt:      tt,
		myAds:   make(map[APITask.TaskID]*APITask.TaskAd),
		peerAds: make(map[APITask.TaskID]*APITask.TaskAd),
		bids:    make(map[APITask.TaskID][]*APICapability.CapabilityCV),
	}
	return b, tp, eb, tt
}

func TestPutup(t *testing.T) {
	b, tp, _, tt := testBoard(t)

	tt.Import(&APITask.Task{
		Meta: APITask.TaskMeta{ID: "task-1"},
		Spec: APITask.TaskSpec{Prompt: "Parse the data", Model: "gemma3", BudgetTokens: 100},
	})

	if err := b.Putup("task-1"); err != nil {
		t.Fatalf("Putup: %v", err)
	}

	// Putup broadcasts + self-delivers through full chain (Drawup→Interview→cleanup).
	if len(tp.Published()) < 1 {
		t.Fatal("expected at least 1 publish")
	}
	// myAds is cleaned by Interview after award — expected empty.
}

func TestPutupDuplicate(t *testing.T) {
	b, tp, _, tt := testBoard(t)

	tt.Import(&APITask.Task{
		Meta: APITask.TaskMeta{ID: "task-1"},
		Spec: APITask.TaskSpec{Prompt: "hi", Model: "gemma3"},
	})
	b.Putup("task-1")
	pub1 := len(tp.Published())
	b.Putup("task-1")

	// After first Putup processes the ad (Drawup→Interview→cleanup),
	// myAds is empty, so second Putup re-advertises. One extra publish.
	if len(tp.Published()) != pub1+1 {
		t.Fatalf("expected %d publishes, got %d", pub1+1, len(tp.Published()))
	}
}

func TestRipup(t *testing.T) {
	b, _, _, _ := testBoard(t)

	b.peerAds["ad-1"] = &APITask.TaskAd{ID: "ad-1"}
	b.bids["ad-1"] = []*APICapability.CapabilityCV{{}}

	b.Ripup("ad-1")

	if _, ok := b.peerAds["ad-1"]; ok {
		t.Fatal("ad should be removed")
	}
	if _, ok := b.bids["ad-1"]; ok {
		t.Fatal("bids should be removed")
	}
}

func TestHandoutStoresBid(t *testing.T) {
	b, _, _, _ := testBoard(t)

	cv := &APICapability.CapabilityCV{PeerID: "peer-a"}
	if err := b.Handout("task-1", cv); err != nil {
		t.Fatalf("Handout: %v", err)
	}

	b.mu.RLock()
	bids := b.bids["task-1"]
	b.mu.RUnlock()

	if len(bids) != 1 || bids[0].PeerID != "peer-a" {
		t.Fatal("bid not stored")
	}
}

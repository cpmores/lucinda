// Package testutil holds small fakes shared by tests across the internal
// layers.
package testutil

import (
	"context"
	"sync"

	APINode "github.com/cpmores/lucinda/api/v1/domain/node"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

// MockTransport is an in-memory Transport fake. Messages sent to its own ID
// are delivered to its opened protocol channel (loopback); messages sent to
// another ID are delivered to the configured Peer (simulating two real nodes).
// It records every Send for assertion.
type MockTransport struct {
	mu       sync.Mutex
	IDValue  APINode.NodeID
	Peer     *MockTransport
	incoming map[APINode.Protocol]chan APINode.NodeMessage
	Sent     []APINode.NodeMessage
}

var _ transport.Transport = (*MockTransport)(nil)

func NewMockTransport(id string) *MockTransport {
	return &MockTransport{
		IDValue:  APINode.NodeID(id),
		incoming: make(map[APINode.Protocol]chan APINode.NodeMessage),
	}
}

func (m *MockTransport) ID() APINode.NodeID { return m.IDValue }

func (m *MockTransport) Start(ctx context.Context) error { return nil }

func (m *MockTransport) Open(ctx context.Context, proto APINode.Protocol) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.incoming[proto]; !ok {
		m.incoming[proto] = make(chan APINode.NodeMessage, 256)
	}
	return nil
}

func (m *MockTransport) Close(ctx context.Context, proto APINode.Protocol) error {
	return nil
}

func (m *MockTransport) Stop() error { return nil }

func (m *MockTransport) Send(ctx context.Context, to APINode.NodeID, msg APINode.NodeMessage) error {
	m.mu.Lock()
	m.Sent = append(m.Sent, msg)
	m.mu.Unlock()
	if to == m.IDValue {
		return m.deliver(msg)
	}
	if m.Peer != nil {
		return m.Peer.deliver(msg)
	}
	return nil
}

// Publish delivers to the local node (loopback) and every connected peer,
// simulating a mesh broadcast.
func (m *MockTransport) Publish(ctx context.Context, msg APINode.NodeMessage) error {
	_ = m.deliver(msg)
	if m.Peer != nil {
		return m.Peer.deliver(msg)
	}
	return nil
}

func (m *MockTransport) Incoming(proto APINode.Protocol) (<-chan APINode.NodeMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.incoming[proto], nil
}

// deliver routes a message into the receiving node's opened protocol channel.
func (m *MockTransport) deliver(msg APINode.NodeMessage) error {
	m.mu.Lock()
	ch := m.incoming[msg.Protocol]
	m.mu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case ch <- msg:
	default:
	}
	return nil
}

// ── AvailableModule ────────────────────────────────────────────────────

func (m *MockTransport) GetModuleType() APIModule.ModuleType { return APIModule.Transport }
func (m *MockTransport) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(m.GetModuleType(), "default")
}
func (m *MockTransport) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(m.GetModuleID(), m.GetModuleType(), APIModule.Running)
}
func (m *MockTransport) RegisterWithManager(mm modulemanager.ModuleManager) error {
	return mm.Register(m)
}
func (m *MockTransport) DependsOn() map[APIModule.ModuleType]string { return nil }
func (m *MockTransport) DependsEnable() error                       { return nil }

var _ modulemanager.AvailableModule = (*MockTransport)(nil)

// Package streamrouter carries the final-answer token stream on a dedicated
// Transport protocol, separate from the EventBus. A producer (e.g. a remote
// commander/executor) routes each chunk toward the plan owner; the owner
// node reconstructs a local per-plan chunk channel that the TaskMonitor
// drains. Subscribers are keyed by plan ID, so concurrent plans never mix.
package streamrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	APINode "github.com/cpmores/lucinda/api/v1/domain/node"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

// StreamProtocol is the data-plane protocol for final-answer token chunks.
// Raw tokens never ride the EventBus.
const StreamProtocol = "/lucinda/stream/1.0.0"

// StreamRouter is the interface for the stream router module.
type StreamRouter interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error

	// Send routes a chunk toward the plan owner. If the owner is the local
	// node it is delivered directly; otherwise unicast via Transport.
	Send(ctx context.Context, chunk APITaskmsg.StreamChunkMsg) error
	// Subscribe returns a buffered channel receiving the reconstructed
	// chunks for a plan (owner side). Caller must drain it.
	Subscribe(planID string) <-chan APITaskmsg.StreamChunkMsg
	// Unsubscribe stops delivery to a plan's subscriber channel.
	Unsubscribe(planID string, ch <-chan APITaskmsg.StreamChunkMsg)
}

type router struct {
	mm  modulemanager.ModuleManager
	tp  transport.Transport
	log *logger.Logger

	mu   sync.Mutex
	subs map[string][]chan APITaskmsg.StreamChunkMsg
}

// NewStreamRouter creates a router. Deps (Transport) resolved via
// DependsEnable.
func NewStreamRouter(log *logger.Logger) StreamRouter {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &router{
		log:  log,
		subs: make(map[string][]chan APITaskmsg.StreamChunkMsg),
	}
}

func (r *router) Start(ctx context.Context) error {
	proto := APINode.Protocol(StreamProtocol)
	if err := r.tp.Open(ctx, proto); err != nil {
		return fmt.Errorf("open stream protocol: %w", err)
	}

	ch, err := r.tp.Incoming(proto)
	if err != nil {
		return fmt.Errorf("stream protocol incoming: %w", err)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				chunk, ok := asStreamChunk(msg.Body)
				if !ok {
					r.log.Warn("dropping malformed stream chunk", "from", msg.From)
					continue
				}
				r.deliver(chunk)
			}
		}
	}()

	r.log.Info("started", "protocol", proto, "local", r.tp.ID())
	return nil
}

func (r *router) Stop() error {
	r.log.Info("stopped")
	return nil
}

// Send routes a chunk to the plan owner, or delivers locally when the owner
// is this node.
func (r *router) Send(ctx context.Context, chunk APITaskmsg.StreamChunkMsg) error {
	if chunk.Owner == "" || chunk.Owner == string(r.tp.ID()) {
		r.deliver(chunk)
		return nil
	}
	msg := APINode.NewNodeMessage(
		APINode.Protocol(StreamProtocol), "stream",
		r.tp.ID(), APINode.NodeID(chunk.Owner), chunk,
	)
	return r.tp.Send(ctx, APINode.NodeID(chunk.Owner), msg)
}

// Subscribe registers a subscriber channel for a plan.
func (r *router) Subscribe(planID string) <-chan APITaskmsg.StreamChunkMsg {
	ch := make(chan APITaskmsg.StreamChunkMsg, 256)
	r.mu.Lock()
	r.subs[planID] = append(r.subs[planID], ch)
	r.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel for a plan.
func (r *router) Unsubscribe(planID string, ch <-chan APITaskmsg.StreamChunkMsg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	chs, ok := r.subs[planID]
	if !ok {
		return
	}
	for i, c := range chs {
		if c == ch {
			r.subs[planID] = append(chs[:i], chs[i+1:]...)
			break
		}
	}
	if len(r.subs[planID]) == 0 {
		delete(r.subs, planID)
	}
}

// deliver fans a chunk out to every subscriber of its plan (demux by planID).
func (r *router) deliver(chunk APITaskmsg.StreamChunkMsg) {
	r.mu.Lock()
	chs := r.subs[string(chunk.PlanID)]
	r.mu.Unlock()
	for _, ch := range chs {
		select {
		case ch <- chunk:
		default:
			r.log.Warn("stream subscriber slow, dropping chunk", "plan", chunk.PlanID)
		}
	}
}

// asStreamChunk re-types a chunk that crossed the wire as map[string]any.
func asStreamChunk(body any) (APITaskmsg.StreamChunkMsg, bool) {
	if chunk, ok := body.(APITaskmsg.StreamChunkMsg); ok {
		return chunk, true
	}
	b, err := json.Marshal(body)
	if err != nil {
		return APITaskmsg.StreamChunkMsg{}, false
	}
	var chunk APITaskmsg.StreamChunkMsg
	if err := json.Unmarshal(b, &chunk); err != nil {
		return APITaskmsg.StreamChunkMsg{}, false
	}
	return chunk, true
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (r *router) GetModuleType() APIModule.ModuleType { return APIModule.StreamRouter }

func (r *router) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(r.GetModuleType(), "default")
}

func (r *router) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(r.GetModuleID(), r.GetModuleType(), APIModule.Running)
}

func (r *router) RegisterWithManager(m modulemanager.ModuleManager) error {
	r.mm = m
	return m.Register(r)
}

func (r *router) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{APIModule.Transport: "default"}
}

func (r *router) DependsEnable() error {
	id := APIModule.NewModuleID(APIModule.Transport, "default")
	mod, err := r.mm.Get(id)
	if err != nil {
		return fmt.Errorf("resolve dependency %s: %w", id, err)
	}
	tp, ok := mod.(transport.Transport)
	if !ok {
		return fmt.Errorf("dependency %s is not a Transport", id)
	}
	r.tp = tp
	return nil
}

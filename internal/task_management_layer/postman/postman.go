// Package taskpostman bridges task-coordination messages between the
// EventBus and the Transport — the control plane of the mesh. It is the
// general counterpart of the telemetry bridge (user-facing progress) and
// the stream router (final-answer tokens): it moves the messages components
// use to assign tasks and return results.
//
// Direction 1 (EventBus → Transport): the postman watches coordination
// topics and forwards events that belong to a remote node. TaskTraced
// always routes back to the task's Owner node. Explicit unicast
// (a board assigning a remote executor) and broadcast (task advertisements)
// are exposed as SendEvent / BroadcastEvent.
//
// Direction 2 (Transport → EventBus): every message received on the task
// protocol is re-published to the local EventBus under its topic, so a local
// component (e.g. the commander) consumes remote results exactly like local
// ones.
package taskpostman

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	APINode "github.com/cpmores/lucinda/api/v1/domain/node"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	eventbus "github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	logger "github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	transport "github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

// TaskPostmanProtocol is the control-plane channel for task coordination
// messages.
const TaskPostmanProtocol = "/lucinda/taskpostman/1.0.0"

// outboundTopics are the coordination topics the postman auto-forwards to a
// remote Owner node. TaskTraced carries the task's Owner (plan owner), so a
// worker that executed a remote task routes the completion back without the
// commander ever knowing the worker's address. It is the single completion
// signal — TaskDone / TaskFailed were merged into it.
var outboundTopics = []APIEvent.EventType{
	APIEvent.TaskTraced,
}

// TaskPostman is the interface for the postman module.
type TaskPostman interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error

	// Publish delivers an event to the local EventBus (compatibility with
	// the raw EventBus for components that route through the postman).
	Publish(topic APIEvent.EventType, event APIEvent.Event) error
	// Watch subscribes to a local EventBus topic (compatibility wrapper).
	Watch(ctx context.Context, topic APIEvent.EventType, handler func(any)) error

	// SendEvent unicasts an event's data to a specific node on the task
	// protocol. Used by the board to assign a task to a remote executor.
	SendEvent(ctx context.Context, to APINode.NodeID, topic APIEvent.EventType, data any) error
	// BroadcastEvent publishes an event's data to every peer (e.g. task
	// advertisements for capability bidding).
	BroadcastEvent(ctx context.Context, topic APIEvent.EventType, data any) error
}

type postman struct {
	mm  modulemanager.ModuleManager
	eb  eventbus.EventBus
	tp  transport.Transport
	log *logger.Logger

	mu       sync.Mutex
	watchers []context.CancelFunc
	wg       sync.WaitGroup
	cancel   context.CancelFunc
}

func NewTaskPostman(log *logger.Logger) TaskPostman {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &postman{
		log: log,
	}
}

// ── Lifecycle ──────────────────────────────────────────────────────────

func (p *postman) Start(ctx context.Context) error {
	ctx, p.cancel = context.WithCancel(ctx)
	proto := APINode.Protocol(TaskPostmanProtocol)

	if err := p.tp.Open(ctx, proto); err != nil {
		return fmt.Errorf("open task protocol: %w", err)
	}
	go p.serveIncoming(ctx, proto)
	for _, topic := range outboundTopics {
		p.watchOutbound(ctx, topic)
	}

	p.log.Info("started", "protocol", proto, "local", p.tp.ID())
	return nil
}

func (p *postman) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Lock()
	for _, cancel := range p.watchers {
		cancel()
	}
	p.mu.Unlock()
	p.wg.Wait()
	p.log.Info("stopped")
	return nil
}

// ── Transport → EventBus ───────────────────────────────────────────────

// serveIncoming re-publishes every task-protocol message onto the local
// EventBus under its topic, so remote results are consumed like local ones.
func (p *postman) serveIncoming(ctx context.Context, proto APINode.Protocol) {
	ch, err := p.tp.Incoming(proto)
	if err != nil {
		p.log.Error("postman incoming", "err", err)
		return
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				topic := APIEvent.EventType(msg.Topic)
				// Re-type the payload: the transport decodes Body into a map,
				// but local consumers type-assert (e.g. the commander asserts
				// TaskResultMsg in onResult).
				payload := decode(topic, msg.Body)
				_ = p.eb.Publish(topic, APIEvent.NewEvent(topic, payload))
				p.log.Debug("delivered from network", "topic", topic, "from", msg.From)
			}
		}
	}()
}

// ── EventBus → Transport ───────────────────────────────────────────────

// watchOutbound forwards locally-produced events whose Owner is a remote
// node. Events owned locally stay on the bus untouched (local fast path).
func (p *postman) watchOutbound(ctx context.Context, topic APIEvent.EventType) {
	ch := p.eb.Subscribe(topic, 64)
	ctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.watchers = append(p.watchers, cancel)
	p.mu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer p.eb.UnSubscribe(topic, ch)
		proto := APINode.Protocol(TaskPostmanProtocol)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				owner, ok := resultOwner(ev.Data)
				if !ok || owner == "" || owner == string(p.tp.ID()) {
					continue
				}
				msg := APINode.NewNodeMessage(proto, string(topic), p.tp.ID(), APINode.NodeID(owner), ev.Data)
				if err := p.tp.Send(ctx, APINode.NodeID(owner), msg); err != nil {
					p.log.Warn("task send failed", "owner", owner, "topic", topic, "err", err)
				}
			}
		}
	}()
}

// SendEvent unicasts an event's data to a specific node.
func (p *postman) SendEvent(ctx context.Context, to APINode.NodeID, topic APIEvent.EventType, data any) error {
	msg := APINode.NewNodeMessage(APINode.Protocol(TaskPostmanProtocol), string(topic), p.tp.ID(), to, data)
	return p.tp.Send(ctx, to, msg)
}

// BroadcastEvent publishes an event's data to every peer.
func (p *postman) BroadcastEvent(ctx context.Context, topic APIEvent.EventType, data any) error {
	msg := APINode.NewNodeMessage(APINode.Protocol(TaskPostmanProtocol), string(topic), p.tp.ID(), "", data)
	return p.tp.Publish(ctx, msg)
}

// ── EventBus compatibility ─────────────────────────────────────────────

func (p *postman) Publish(topic APIEvent.EventType, event APIEvent.Event) error {
	return p.eb.Publish(topic, event)
}

func (p *postman) Watch(ctx context.Context, topic APIEvent.EventType, handler func(any)) error {
	ch := p.eb.Subscribe(topic, 64)
	ctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.watchers = append(p.watchers, cancel)
	p.mu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer p.eb.UnSubscribe(topic, ch)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				handler(ev.Data)
			}
		}
	}()
	return nil
}

// resultOwner extracts the plan-owner routing key from a coordination
// payload. Only meaningful on the producing side, where payloads are typed.
func resultOwner(data any) (string, bool) {
	switch d := data.(type) {
	case APITaskmsg.TaskTracedMsg:
		return d.Owner, d.Owner != ""
	}
	return "", false
}

// decode re-types a payload that crossed the wire as map[string]any.
// Already-typed payloads (local path) pass through unchanged.
func decode(topic APIEvent.EventType, body any) any {
	switch topic {
	case APIEvent.TaskTraced:
		return as[APITaskmsg.TaskTracedMsg](body)
	}
	return body
}

func as[T any](body any) T {
	var zero T
	if v, ok := body.(T); ok {
		return v
	}
	b, err := json.Marshal(body)
	if err != nil {
		return zero
	}
	if json.Unmarshal(b, &zero) == nil {
		return zero
	}
	return zero
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (p *postman) GetModuleType() APIModule.ModuleType { return APIModule.TaskPostman }

func (p *postman) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(p.GetModuleType(), "default")
}

func (p *postman) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(p.GetModuleID(), p.GetModuleType(), APIModule.Running)
}

func (p *postman) RegisterWithManager(m modulemanager.ModuleManager) error {
	p.mm = m
	return m.Register(p)
}

func (p *postman) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{
		APIModule.EventBus:  "default",
		APIModule.Transport: "default",
	}
}

func (p *postman) DependsEnable() error {
	for depType, name := range p.DependsOn() {
		id := APIModule.NewModuleID(depType, name)
		mod, err := p.mm.Get(id)
		if err != nil {
			return fmt.Errorf("resolve dependency %s: %w", id, err)
		}
		switch depType {
		case APIModule.EventBus:
			eb, ok := mod.(eventbus.EventBus)
			if !ok {
				return fmt.Errorf("dependency %s is not an EventBus", id)
			}
			p.eb = eb
		case APIModule.Transport:
			tp, ok := mod.(transport.Transport)
			if !ok {
				return fmt.Errorf("dependency %s is not a Transport", id)
			}
			p.tp = tp
		}
	}
	return nil
}

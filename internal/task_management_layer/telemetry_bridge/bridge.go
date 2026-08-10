// Package telemetrybridge routes user-facing telemetry from any node back
// to the plan owner node. On the producing node it watches telemetry events
// and unicasts those not owned locally over the telemetry protocol; on the
// owner node it re-publishes the decoded payload to the local EventBus so
// the TaskMonitor sees remote and local progress identically.
package telemetrybridge

import (
	"context"
	"encoding/json"
	"fmt"

	APINode "github.com/cpmores/lucinda/api/v1/domain/node"
	APISteam "github.com/cpmores/lucinda/api/v1/domain/stream"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

// TelemetryProtocol is the dedicated cross-node channel for user-facing
// progress. It is separate from coordination traffic so a busy stream of
// status events never crowds task advertisements.
const TelemetryProtocol = "/lucinda/telemetry/1.0.0"

// telemetryTopics are the EventBus topics bridged to the owner node.
var telemetryTopics = []APIEvent.EventType{
	APIEvent.TelemetryPlanning,
	APIEvent.TelemetryPlanned,
	APIEvent.TelemetryThinking,
	APIEvent.TelemetryWaiting,
	APIEvent.TelemetryFinalizing,
	APIEvent.TelemetryRunning,
	APIEvent.TelemetryExecDone,
	APIEvent.TelemetryStepResult,
}

// TelemetryBridge is the interface for the telemetry bridge module.
type TelemetryBridge interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error
}

type bridge struct {
	mm  modulemanager.ModuleManager
	eb  eventbus.EventBus
	tp  transport.Transport
	log *logger.Logger

	cancel context.CancelFunc
}

// NewTelemetryBridge creates a bridge. Deps (EventBus, Transport) are
// resolved via DependsEnable.
func NewTelemetryBridge(log *logger.Logger) TelemetryBridge {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &bridge{log: log}
}

func (b *bridge) Start(ctx context.Context) error {
	ctx, b.cancel = context.WithCancel(ctx)
	proto := APINode.Protocol(TelemetryProtocol)

	if err := b.tp.Open(ctx, proto); err != nil {
		return fmt.Errorf("open telemetry protocol: %w", err)
	}
	go b.serveIncoming(ctx, proto)

	for _, topic := range telemetryTopics {
		b.watchOutbound(ctx, topic)
	}

	b.log.Info("started", "protocol", proto, "local", b.tp.ID())
	return nil
}

func (b *bridge) Stop() error {
	if b.cancel != nil {
		b.cancel()
	}
	b.log.Info("stopped")
	return nil
}

// serveIncoming re-publishes remote telemetry onto the local EventBus so the
// monitor consumes it exactly like local events.
func (b *bridge) serveIncoming(ctx context.Context, proto APINode.Protocol) {
	ch, err := b.tp.Incoming(proto)
	if err != nil {
		b.log.Error("telemetry bridge incoming", "err", err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			topic := APIEvent.EventType(msg.Topic)
			if !isTelemetryTopic(topic) {
				continue
			}
			payload := decodeTelemetry(topic, msg.Body)
			_ = b.eb.Publish(topic, APIEvent.NewEvent(topic, payload))
		}
	}
}

// watchOutbound unicasts locally-produced telemetry whose owner is a remote
// node. Local events (owner == this node) stay on the bus untouched.
func (b *bridge) watchOutbound(ctx context.Context, topic APIEvent.EventType) {
	ch := b.eb.Subscribe(topic, 64)
	go func() {
		defer b.eb.UnSubscribe(topic, ch)
		proto := APINode.Protocol(TelemetryProtocol)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				owner, ok := payloadOwner(ev.Data)
				if !ok || owner == "" || owner == string(b.tp.ID()) {
					continue // local fast path
				}
				msg := APINode.NewNodeMessage(proto, string(topic), b.tp.ID(), APINode.NodeID(owner), ev.Data)
				if err := b.tp.Send(ctx, APINode.NodeID(owner), msg); err != nil {
					b.log.Warn("telemetry send failed", "owner", owner, "err", err)
				}
			}
		}
	}()
}

// payloadOwner extracts the Owner routing key from a telemetry payload.
// Only meaningful on the producing side, where payloads are typed.
func payloadOwner(data any) (string, bool) {
	switch d := data.(type) {
	case APISteam.StatusData:
		return d.Owner, d.Owner != ""
	case APISteam.StepResultData:
		return d.Owner, d.Owner != ""
	}
	return "", false
}

func isTelemetryTopic(t APIEvent.EventType) bool {
	for _, topic := range telemetryTopics {
		if topic == t {
			return true
		}
	}
	return false
}

// decodeTelemetry re-types a payload that crossed the wire as map[string]any.
// Already-typed payloads (local path) pass through unchanged.
func decodeTelemetry(topic APIEvent.EventType, body any) any {
	if topic == APIEvent.TelemetryStepResult {
		return as[APISteam.StepResultData](body)
	}
	return as[APISteam.StatusData](body)
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

func (b *bridge) GetModuleType() APIModule.ModuleType { return APIModule.TelemetryBridge }

func (b *bridge) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(b.GetModuleType(), "default")
}

func (b *bridge) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(b.GetModuleID(), b.GetModuleType(), APIModule.Running)
}

func (b *bridge) RegisterWithManager(m modulemanager.ModuleManager) error {
	b.mm = m
	return m.Register(b)
}

func (b *bridge) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{
		APIModule.EventBus:  "default",
		APIModule.Transport: "default",
	}
}

func (b *bridge) DependsEnable() error {
	for depType, name := range b.DependsOn() {
		id := APIModule.NewModuleID(depType, name)
		mod, err := b.mm.Get(id)
		if err != nil {
			return fmt.Errorf("resolve dependency %s: %w", id, err)
		}
		switch depType {
		case APIModule.EventBus:
			eb, ok := mod.(eventbus.EventBus)
			if !ok {
				return fmt.Errorf("dependency %s is not an EventBus", id)
			}
			b.eb = eb
		case APIModule.Transport:
			tp, ok := mod.(transport.Transport)
			if !ok {
				return fmt.Errorf("dependency %s is not a Transport", id)
			}
			b.tp = tp
		}
	}
	return nil
}

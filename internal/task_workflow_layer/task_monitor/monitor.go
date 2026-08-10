// Package taskmonitor is the read-only aggregator on the plan owner node.
// It subscribes to user-facing telemetry (local events plus remote events
// re-published by the telemetry bridge), converts each into an SSE frame,
// and fans frames out per plan ID. It also forwards final-answer token
// chunks from the stream router. It never consumes control flow — it only
// observes, so optional component loading and topology changes do not affect
// it. The terminal done frame is emitted by the wrapper, not here, so
// exactly one done frame is written per plan.
package taskmonitor

import (
	"context"
	"fmt"
	"sync"

	APISteam "github.com/cpmores/lucinda/api/v1/domain/stream"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// TaskMonitor is the interface for the monitor module.
type TaskMonitor interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error

	// Register marks a plan as known before the workflow emits telemetry for
	// it, so Open never races the planner. Idempotent.
	Register(planID string)
	// Open returns a channel receiving this plan's SSE frames (status,
	// step_result, stream). Returns an error for an unknown plan.
	Open(planID string) (<-chan APISteam.SSEFrame, error)
}

// telemetryTopics are the event types whose payloads become SSE frames.
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

type monitor struct {
	mm  modulemanager.ModuleManager
	eb  eventbus.EventBus
	sr  streamrouter.StreamRouter
	log *logger.Logger

	mu       sync.Mutex
	active   map[string]bool
	subs     map[string][]chan APISteam.SSEFrame
	streamed map[string]bool // router subscribed once per plan
	cancel   context.CancelFunc
}

// NewTaskMonitor creates a monitor. Deps (EventBus, StreamRouter) are
// resolved via DependsEnable; the module manager is captured at
// RegisterWithManager.
func NewTaskMonitor(log *logger.Logger) TaskMonitor {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &monitor{
		log:      log,
		active:   make(map[string]bool),
		subs:     make(map[string][]chan APISteam.SSEFrame),
		streamed: make(map[string]bool),
	}
}

func (m *monitor) Start(ctx context.Context) error {
	ctx, m.cancel = context.WithCancel(ctx)

	for _, topic := range telemetryTopics {
		ch := m.eb.Subscribe(topic, 128)
		go func(topic APIEvent.EventType, ch chan APIEvent.Event) {
			defer m.eb.UnSubscribe(topic, ch)
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-ch:
					if !ok {
						return
					}
					m.onTelemetry(topic, ev.Data)
				}
			}
		}(topic, ch)
	}

	m.log.Info("started")
	return nil
}

func (m *monitor) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	m.log.Info("stopped")
	return nil
}

// Register marks a plan as known.
func (m *monitor) Register(planID string) {
	m.mu.Lock()
	m.active[planID] = true
	m.mu.Unlock()
}

// Open validates the plan and returns its frame stream.
func (m *monitor) Open(planID string) (<-chan APISteam.SSEFrame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.active[planID] {
		return nil, fmt.Errorf("unknown plan %s", planID)
	}
	ch := make(chan APISteam.SSEFrame, 256)
	m.subs[planID] = append(m.subs[planID], ch)
	if !m.streamed[planID] {
		m.streamed[planID] = true
		// Subscribe synchronously so the router has this plan's subscriber
		// registered before Open returns — otherwise an early stream chunk
		// can be delivered before the forwarding goroutine subscribes.
		chunks := m.sr.Subscribe(planID)
		go m.forwardStream(planID, chunks)
	}
	return ch, nil
}

// onTelemetry converts a telemetry event into an SSE frame and fans it out.
func (m *monitor) onTelemetry(topic APIEvent.EventType, data any) {
	planID, frame, ok := frameFor(topic, data)
	if !ok {
		return
	}
	m.mu.Lock()
	m.active[planID] = true
	m.mu.Unlock()
	m.fanout(planID, frame)
}

// forwardStream forwards reconstructed token chunks as stream frames. The
// chunk subscription is created synchronously by Open before this goroutine
// starts.
func (m *monitor) forwardStream(planID string, chunks <-chan APITaskmsg.StreamChunkMsg) {
	for c := range chunks {
		m.fanout(planID, APISteam.SSEFrame{
			Event:  APISteam.SSETypeStream,
			PlanID: planID,
			Data:   APISteam.StreamData{Delta: c.Delta, Done: c.Done},
		})
		if c.Done {
			break
		}
	}
}

func (m *monitor) fanout(planID string, frame APISteam.SSEFrame) {
	m.mu.Lock()
	chs := m.subs[planID]
	m.mu.Unlock()
	for _, ch := range chs {
		select {
		case ch <- frame:
		default:
			m.log.Warn("monitor subscriber slow, dropping frame", "plan", planID)
		}
	}
}

// frameFor maps a telemetry event to its plan ID and SSE frame. Status and
// step-result frames keep the subset of fields the user needs; the wrapper
// emits the terminal done frame separately.
func frameFor(topic APIEvent.EventType, data any) (string, APISteam.SSEFrame, bool) {
	switch topic {
	case APIEvent.TelemetryStepResult:
		sd, ok := data.(APISteam.StepResultData)
		if !ok {
			return "", APISteam.SSEFrame{}, false
		}
		return sd.PlanID, APISteam.SSEFrame{
			Event:  APISteam.SSETypeStepResult,
			PlanID: sd.PlanID,
			Data:   APISteam.StepResultData{TaskID: sd.TaskID, Output: sd.Output},
		}, true
	case APIEvent.TelemetryPlanning, APIEvent.TelemetryPlanned,
		APIEvent.TelemetryThinking, APIEvent.TelemetryWaiting,
		APIEvent.TelemetryFinalizing, APIEvent.TelemetryRunning,
		APIEvent.TelemetryExecDone:
		sd, ok := data.(APISteam.StatusData)
		if !ok {
			return "", APISteam.SSEFrame{}, false
		}
		return sd.PlanID, APISteam.SSEFrame{
			Event:  APISteam.SSETypeStatus,
			PlanID: sd.PlanID,
			Data: APISteam.StatusData{
				Component: sd.Component,
				State:     sd.State,
				TaskID:    sd.TaskID,
				Model:     sd.Model,
			},
		}, true
	}
	return "", APISteam.SSEFrame{}, false
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (m *monitor) GetModuleType() APIModule.ModuleType { return APIModule.TaskMonitor }
func (m *monitor) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(m.GetModuleType(), "default")
}
func (m *monitor) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(m.GetModuleID(), m.GetModuleType(), APIModule.Running)
}
func (m *monitor) RegisterWithManager(mm modulemanager.ModuleManager) error {
	m.mm = mm
	return mm.Register(m)
}
func (m *monitor) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{
		APIModule.EventBus:    "default",
		APIModule.StreamRouter: "default",
	}
}
func (m *monitor) DependsEnable() error {
	for depType, name := range m.DependsOn() {
		id := APIModule.NewModuleID(depType, name)
		mod, err := m.mm.Get(id)
		if err != nil {
			return fmt.Errorf("resolve dependency %s: %w", id, err)
		}
		switch depType {
		case APIModule.EventBus:
			eb, ok := mod.(eventbus.EventBus)
			if !ok {
				return fmt.Errorf("dependency %s is not an EventBus", id)
			}
			m.eb = eb
		case APIModule.StreamRouter:
			sr, ok := mod.(streamrouter.StreamRouter)
			if !ok {
				return fmt.Errorf("dependency %s is not a StreamRouter", id)
			}
			m.sr = sr
		}
	}
	return nil
}

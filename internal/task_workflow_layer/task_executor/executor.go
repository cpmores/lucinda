// Package taskexecutor executes assigned tasks — via an LLM, a toolbox, or
// an MCP server. Phase 1 executes text LLM actions: it consumes a
// TaskAssignMsg, runs provider.Generate, and publishes a structured
// TaskResultMsg back so the commander can decide the next step. Raw token
// streams are carried separately (final-answer path), never on the EventBus.
package taskexecutor

import (
	"context"
	"fmt"
	"strings"
	"time"

	APIChat "github.com/cpmores/lucinda/api/v1/domain/chat"
	APIProvider "github.com/cpmores/lucinda/api/v1/domain/provider"
	APISteam "github.com/cpmores/lucinda/api/v1/domain/stream"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	streamrouter "github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	tasktracer "github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/eventx"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	providerctrl "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
)

const (
	BusyProviderRetries    = 3
	BusyProviderRetryDelay = 500 // milliseconds
)

// TaskExecutor is the interface for the executor module.
type TaskExecutor interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error
}

type executor struct {
	mm  modulemanager.ModuleManager
	eb  eventbus.EventBus
	pc  providerctrl.ProviderController
	tt  tasktracer.TaskTracer
	log *logger.Logger
	sr  streamrouter.StreamRouter

	cancel context.CancelFunc
}

// NewTaskExecutor creates an executor. Deps are resolved via DependsEnable;
// the module manager is captured at RegisterWithManager.
func NewTaskExecutor(log *logger.Logger) TaskExecutor {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &executor{log: log}
}

func (e *executor) Start(ctx context.Context) error {
	ctx, e.cancel = context.WithCancel(ctx)

	eventx.Watch(ctx, e.eb, e.log, APIEvent.TaskAssigned, func(data any) {
		msg, ok := data.(APITaskmsg.TaskAssignMsg)
		if !ok {
			return
		}
		// Run in a goroutine so concurrent assignments execute in parallel
		// instead of blocking the watch loop.
		go e.execute(ctx, &msg)
	})

	e.log.Info("started")
	return nil
}

func (e *executor) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	e.log.Info("stopped")
	return nil
}

// execute runs one assigned task and publishes the structured result back.
func (e *executor) execute(ctx context.Context, msg *APITaskmsg.TaskAssignMsg) error {
	eventx.Emit(e.eb, e.log, APIEvent.TelemetryRunning, APISteam.StatusData{
		Component: "executor", State: "running", Owner: msg.Owner,
		PlanID: string(msg.PlanID), TaskID: string(msg.TaskID), Model: msg.Spec.Model,
	})
	_ = e.tt.Assigned(&APITask.Task{
		Meta:     APITask.TaskMeta{ID: msg.TaskID, Owner: msg.Owner},
		Spec:     msg.Spec,
		TaskPlan: &APITask.TaskPlan{ID: msg.PlanID},
	})

	prov, err := pickProvider(e.pc, msg.Spec)
	busyProviderRetry := 1
	for err != nil && busyProviderRetry < BusyProviderRetries {
		time.Sleep(BusyProviderRetryDelay * time.Millisecond)
		prov, err = pickProvider(e.pc, msg.Spec)
		busyProviderRetry = busyProviderRetry + 1
	}
	if err != nil {
		e.fail(msg, err)
		return err
	}

	model := msg.Spec.Model
	if model == "" {
		models := prov.GetModels()
		if len(models) > 0 {
			model = models[0].ID
		}
	}

	output, err := e.run(ctx, msg, prov, model)
	if err != nil {
		e.fail(msg, err)
		return err
	}

	// The tracer is the single progress authority: record the output and emit
	// a TaskTraced Done. The commander and board both advance from it.
	_ = e.tt.SetOutput(msg.TaskID, output)
	_ = e.tt.Update(msg.TaskID, APITask.StateDone)

	eventx.Emit(e.eb, e.log, APIEvent.TelemetryExecDone, APISteam.StatusData{
		Component: "executor", State: "done", Owner: msg.Owner,
		PlanID: string(msg.PlanID), TaskID: string(msg.TaskID),
	})
	e.log.Info("task done", "task", msg.TaskID, "plan", msg.PlanID)
	return nil
}

// run dispatches on the task kind: reason/execute use one-shot Generate;
// synthesize streams the answer and routes chunks to the plan owner.
func (e *executor) run(ctx context.Context, msg *APITaskmsg.TaskAssignMsg, prov APIProvider.Provider, model string) (string, error) {
	req := &APIChat.ChatRequest{
		Model: model,
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: msg.Prompt}},
		}},
		Options: APIChat.ModelOptions{MaxTokens: msg.Spec.BudgetTokens, Temperature: 0.7},
	}

	switch msg.Spec.Kind {
	case APITask.TaskKindSynthesize:
		chunks, err := prov.Stream(ctx, req)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for chunk := range chunks {
			if chunk == nil {
				break
			}
			b.WriteString(chunk.Delta)
			_ = e.sr.Send(ctx, APITaskmsg.StreamChunkMsg{
				PlanID: msg.PlanID,
				TaskID: msg.TaskID,
				Delta:  chunk.Delta,
				Done:   chunk.Done,
				Owner:  msg.Owner,
			})
			if chunk.Done {
				break
			}
		}
		return b.String(), nil
	default: // reason, execute
		resp, err := prov.Generate(ctx, req)
		if err != nil {
			return "", err
		}
		return textFromResponse(resp), nil
	}
}

// fail releases the task back to the board on failure: it emits
// TaskTraced{Released}, which the board answers by reassigning to another
// candidate (or, when exhausted, failing the plan). The commander does not
// treat Released as fatal.
func (e *executor) fail(msg *APITaskmsg.TaskAssignMsg, err error) {
	e.log.Error("task failed, releasing back", "task", msg.TaskID, "err", err)
	_ = e.tt.Update(msg.TaskID, APITask.StateReleased)
}

// pickProvider prefers the spec's explicit model; otherwise it selects a free
// provider whose models carry the employer label "TaskExecutor". Fails if
// none qualify.
func pickProvider(pc providerctrl.ProviderController, spec APITask.TaskSpec) (APIProvider.Provider, error) {
	if spec.Model != "" {
		// Only Free providers can serve — a Busy/Error provider is skipped so
		// the two selection paths stay consistent.
		for _, p := range pc.List() {
			if p.Status() != APIProvider.Free {
				continue
			}
			for _, m := range p.GetModels() {
				if m.ID == spec.Model {
					return p, nil
				}
			}
		}
		return nil, fmt.Errorf("no free provider serving model %s", spec.Model)
	}

	matches, err := pc.GetProvByFilter(APIProvider.ModelFilter{
		Required: []APIProvider.Term{{
			MatchExpression: []APIProvider.MatchExpression{{
				Key: "employer", Operator: "In", Values: []string{"TaskExecutor"},
			}},
		}},
	})
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no free provider for TaskExecutor")
	}
	return matches[0].Provider, nil
}

func textFromResponse(resp *APIChat.ChatResponse) string {
	var b strings.Builder
	for _, part := range resp.Message.Content {
		if part.Type == APIChat.ContentText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (e *executor) GetModuleType() APIModule.ModuleType { return APIModule.TaskExecutor }

func (e *executor) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(e.GetModuleType(), "default")
}

func (e *executor) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(e.GetModuleID(), e.GetModuleType(), APIModule.Running)
}

func (e *executor) RegisterWithManager(m modulemanager.ModuleManager) error {
	e.mm = m
	return m.Register(e)
}

func (e *executor) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{
		APIModule.EventBus:           "default",
		APIModule.ProviderController: "default",
		APIModule.TaskTracer:         "default",
		APIModule.StreamRouter:       "default",
	}
}

func (e *executor) DependsEnable() error {
	for depType, name := range e.DependsOn() {
		id := APIModule.NewModuleID(depType, name)
		mod, err := e.mm.Get(id)
		if err != nil {
			return fmt.Errorf("resolve dependency %s: %w", id, err)
		}
		switch depType {
		case APIModule.EventBus:
			eb, ok := mod.(eventbus.EventBus)
			if !ok {
				return fmt.Errorf("dependency %s is not an EventBus", id)
			}
			e.eb = eb
		case APIModule.ProviderController:
			pc, ok := mod.(providerctrl.ProviderController)
			if !ok {
				return fmt.Errorf("dependency %s is not a ProviderController", id)
			}
			e.pc = pc
		case APIModule.TaskTracer:
			tt, ok := mod.(tasktracer.TaskTracer)
			if !ok {
				return fmt.Errorf("dependency %s is not a TaskTracer", id)
			}
			e.tt = tt
		case APIModule.StreamRouter:
			sr, ok := mod.(streamrouter.StreamRouter)
			if !ok {
				return fmt.Errorf("dependency %s is not a StreamRouter", id)
			}
			e.sr = sr
		}
	}
	return nil
}

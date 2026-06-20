// Package taskreducer provides task output reduction
// service for every completed task,
package taskreducer

import (
	"context"
	"fmt"
	"log"
	"strings"

	APIChat "github.com/cpmores/lucinda/api/v1/chat"
	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	APIModule "github.com/cpmores/lucinda/api/v1/module"
	APITask "github.com/cpmores/lucinda/api/v1/task"
	taskpostman "github.com/cpmores/lucinda/internel/task_management_layer/task_postman"
	taskstatemanager "github.com/cpmores/lucinda/internel/task_management_layer/task_state_manager"
	tasktracer "github.com/cpmores/lucinda/internel/task_management_layer/task_tracer"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	providerctrl "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
)

type TaskReducer interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error

	// ── Section ──────────────────────────────────────────────────────────

	reduce(ctx context.Context, taskID APITask.TaskID) error // reduce a completed plan
}

type reducer struct {
	mm     modulemanager.ModuleManager
	pm     taskpostman.Postman
	tt     tasktracer.TaskTracer
	sm     taskstatemanager.TaskStateManager
	pc     providerctrl.ProviderController
	cancel context.CancelFunc
}

func NewTaskReducer(mm modulemanager.ModuleManager) TaskReducer {
	postmans := mm.GetByType(APIModule.TASKPOSTMAN)
	if len(postmans) == 0 {
		log.Fatalln("task_reducer: no task_postman")
	}
	pm := postmans[0].(taskpostman.Postman)

	tracers := mm.GetByType(APIModule.TASKTRACER)
	if len(tracers) == 0 {
		log.Fatalln("task_reducer: no task_tracer")
	}
	tt := tracers[0].(tasktracer.TaskTracer)

	sms := mm.GetByType(APIModule.TASKSTATEMANAGER)
	if len(sms) == 0 {
		log.Fatalln("task_reducer: no task_state_manager")
	}
	sm := sms[0].(taskstatemanager.TaskStateManager)

	pcs := mm.GetByType(APIModule.PROVIDERCONTROLLER)
	if len(pcs) == 0 {
		log.Fatalln("task_reducer: no provider_controller")
	}
	pc := pcs[0].(providerctrl.ProviderController)

	return &reducer{mm: mm, pm: pm, tt: tt, sm: sm, pc: pc}
}

func (r *reducer) Start(ctx context.Context) error {
	ctx, r.cancel = context.WithCancel(ctx)

	// Reduce-stage nodes: combine predecessor outputs locally.
	r.pm.Watch(ctx, APIEvent.TaskReady, func(data any) error {
		node, ok := data.(*APITask.TaskNode)
		if !ok || node.Spec.Stage != APITask.StageReduce {
			return nil
		}
		return r.reduce(ctx, node.ID)
	})

	log.Println("task_reducer: started")
	return nil
}

func (r *reducer) Stop() error {
	if r.cancel != nil {
		r.cancel()
	}
	log.Println("task_reducer: stopped")
	return nil
}

func (r *reducer) reduce(ctx context.Context, taskID APITask.TaskID) error {
	planID := parentPlanID(taskID)
	plan, err := r.sm.Plan(planID)
	if err != nil {
		return fmt.Errorf("reduce: %w", err)
	}

	// Collect outputs from all predecessor nodes.
	var outputs []string
	for _, n := range plan.Nodes {
		if n.Spec.Stage == APITask.StageReduce {
			continue
		}
		t, err := r.tt.GetLocal(n.ID)
		if err != nil {
			t, err = r.tt.GetAssigned(n.ID)
		}
		if err != nil || t.Spec.Output == "" {
			continue
		}
		outputs = append(outputs, fmt.Sprintf("[%s]: %s", n.ID, t.Spec.Output))
	}

	if len(outputs) == 0 {
		return fmt.Errorf("reduce: no outputs to combine for %s", planID)
	}

	prov, err := r.pc.GetPlanProv()
	if err != nil {
		return fmt.Errorf("reduce: %w", err)
	}

	models := prov.GetModels()
	if len(models) == 0 {
		return fmt.Errorf("reduce: no model available")
	}

	// Combine.
	prompt := fmt.Sprintf("Combine these sub-task results into a single coherent response:\n%s",
		strings.Join(outputs, "\n"))

	resp, err := prov.Generate(ctx, &APIChat.ChatRequest{
		Model: models[0],
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: "text", Text: prompt}},
		}},
	})
	if err != nil {
		return fmt.Errorf("reduce generate: %w", err)
	}

		output := textFromResponse(resp)
		r.tt.SetOutput(taskID, output)

	// Reduce runs locally — Claim → Start → Complete to trigger plan.complete.
	if err := r.sm.Claim(ctx, taskID, "local", 30); err != nil {
		log.Printf("task_reducer: claim %s: %v", taskID, err)
	}
	if err := r.sm.Start(taskID); err != nil {
		log.Printf("task_reducer: start %s: %v", taskID, err)
	}
	if err := r.sm.Complete(taskID, output); err != nil {
		return fmt.Errorf("reduce complete: %w", err)
	}

	return nil
}

func parentPlanID(id APITask.TaskID) APITask.TaskID {
	s := string(id)
	if strings.Count(s, "-") >= 2 {
		last := strings.LastIndex(s, "-")
		return APITask.TaskID(s[:last])
	}
	return id
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (r *reducer) GetModuleType() APIModule.ModuleType { return APIModule.TASKREDUCER }

func (r *reducer) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(r.GetModuleType(), "default")
}

func (r *reducer) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(r.GetModuleID(), r.GetModuleType(), APIModule.RUNNING)
}
func (r *reducer) RegisterWithManager(m modulemanager.ModuleManager) error { return m.Register(r) }

func textFromResponse(resp *APIChat.ChatResponse) string {
	var b strings.Builder
	for _, part := range resp.Message.Content {
		if part.Type == APIChat.ContentText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

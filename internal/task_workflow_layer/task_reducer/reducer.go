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
	taskpostman "github.com/cpmores/lucinda/internal/task_management_layer/task_postman"
	taskstatemanager "github.com/cpmores/lucinda/internal/task_management_layer/task_state_manager"
	tasktracer "github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
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
	postmans := mm.GetByType(APIModule.TaskPostman)
	if len(postmans) == 0 {
		log.Fatalln("task_reducer: no task_postman")
	}
	pm := postmans[0].(taskpostman.Postman)

	tracers := mm.GetByType(APIModule.TaskTracer)
	if len(tracers) == 0 {
		log.Fatalln("task_reducer: no task_tracer")
	}
	tt := tracers[0].(tasktracer.TaskTracer)

	sms := mm.GetByType(APIModule.TaskStateManager)
	if len(sms) == 0 {
		log.Fatalln("task_reducer: no task_state_manager")
	}
	sm := sms[0].(taskstatemanager.TaskStateManager)

	pcs := mm.GetByType(APIModule.ProviderController)
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

	// Combine sub-task outputs. If the combined text is too long for the
	// model's context window, truncate each output to stay within limits.
	maxPromptChars := prov.MaxContextTokens() * 3 // rough: ~3 chars per token
	if maxPromptChars < 256 {
		maxPromptChars = 256
	}
	combined := strings.Join(outputs, "\n")
	prompt := fmt.Sprintf("You are writing a final answer for a user. Below are research notes from earlier steps. Synthesize them into one polished, flowing response. Do NOT mention sub-tasks, research steps, or your process. Do NOT say \"based on the information\" or \"let me combine.\" Just write the answer directly as if you knew it yourself:\n\n%s", combined)
	if len(prompt) > maxPromptChars {
		// Truncate each output proportionally to fit.
		perOutput := (maxPromptChars - 100) / len(outputs)
		if perOutput < 50 {
			perOutput = 50
		}
		var truncated []string
		for _, o := range outputs {
			if len(o) > perOutput {
				truncated = append(truncated, o[:perOutput]+"...")
			} else {
				truncated = append(truncated, o)
			}
		}
		prompt = fmt.Sprintf("You are writing a final answer for a user. Below are research notes from earlier steps. Synthesize them into one polished, flowing response. Do NOT mention sub-tasks, research steps, or your process. Do NOT say \"based on the information\" or \"let me combine.\" Just write the answer directly as if you knew it yourself:\n\n%s",
			strings.Join(truncated, "\n"))
	}

	var output string
	resp, err := prov.Generate(ctx, &APIChat.ChatRequest{
		Model: models[0],
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: "text", Text: prompt}},
		}},
	})
	if err != nil {
		// LLM combine failed (context too long, model down, etc.).
		// Fall back to simple concatenation so the plan can complete.
		log.Printf("task_reducer: generate failed, using raw concat: %v", err)
		output = combined
	} else {
		output = textFromResponse(resp)
	}

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

func (r *reducer) GetModuleType() APIModule.ModuleType { return APIModule.TaskReducer }

func (r *reducer) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(r.GetModuleType(), "default")
}

func (r *reducer) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(r.GetModuleID(), r.GetModuleType(), APIModule.Running)
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

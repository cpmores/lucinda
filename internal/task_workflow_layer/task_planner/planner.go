// Package taskplanner provides planning service for every raw task,
// after planning, send to PlanCreated Channel
package taskplanner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	APIChat "github.com/cpmores/lucinda/api/v1/chat"
	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	APIModule "github.com/cpmores/lucinda/api/v1/module"
	APITask "github.com/cpmores/lucinda/api/v1/task"
	taskpostman "github.com/cpmores/lucinda/internal/task_management_layer/task_postman"
	taskstatemanager "github.com/cpmores/lucinda/internal/task_management_layer/task_state_manager"
	tasktracer "github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	provider "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
)

type TaskPlanner interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error

	// ── Planning ──────────────────────────────────────────────────────────
	plan(ctx context.Context, task *APITask.Task) error
}

type planner struct {
	mm     modulemanager.ModuleManager
	pm     taskpostman.Postman
	sm     taskstatemanager.TaskStateManager
	pc     provider.ProviderController
	tt     tasktracer.TaskTracer
	cancel context.CancelFunc
}

func NewTaskPlanner(mod modulemanager.ModuleManager) TaskPlanner {
	postmans := mod.GetByType(APIModule.TASKPOSTMAN)
	if len(postmans) == 0 {
		log.Fatalln("task_planner: no task_postman module found")
	}
	postman := postmans[0].(taskpostman.Postman)

	stateManagers := mod.GetByType(APIModule.TASKSTATEMANAGER)
	if len(stateManagers) == 0 {
		log.Fatalln("task_planner: no task_state_manager module found")
	}
	stateManager := stateManagers[0].(taskstatemanager.TaskStateManager)

	providerControllers := mod.GetByType(APIModule.PROVIDERCONTROLLER)
	if len(providerControllers) == 0 {
		log.Fatalln("task_planner: no provider_controller module found")
	}
	providerController := providerControllers[0].(provider.ProviderController)

	tracers := mod.GetByType(APIModule.TASKTRACER)
	if len(tracers) == 0 {
		log.Fatalln("task_planner: no task_tracer")
	}
	tt := tracers[0].(tasktracer.TaskTracer)

	return &planner{
		mm: mod,
		pm: postman,
		sm: stateManager,
		pc: providerController,
		tt: tt,
	}
}

// ── Lifecycle ──────────────────────────────────────────────────────────

func (p *planner) Start(ctx context.Context) error {
	ctx, p.cancel = context.WithCancel(ctx)
	// Start watching for a new task to plan
	p.pm.Watch(ctx, APIEvent.TaskPreplanned, func(data any) error {
		task, ok := data.(*APITask.Task)
		if !ok {
			return nil
		}
		return p.plan(ctx, task)
	})

	return nil
}

func (p *planner) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}

	log.Println("task_planner: stopped")
	return nil
}

// ── Planning ──────────────────────────────────────────────────────────
func (p *planner) plan(ctx context.Context, task *APITask.Task) error {
	var plan *APITask.TaskPlan

	decomPrompt := buildDecomposition(task.Spec.Prompt)
	planStr, err := p.generate(ctx, decomPrompt)
	if err != nil {
		// LLM unavailable — fall back to a single-node plan using the
		// original prompt directly. The executor will still need a
		// provider, but the pipeline won't stall.
		log.Printf("task_planner: generate failed, using fallback: %v", err)
		plan = fallbackPlan(task.Meta.ID, task.Meta.Owner, task.Spec.Prompt)
	} else {
		plan = p.parse(task.Meta.ID, task.Meta.Owner, planStr)
	}
	// Preserve the Notify channel from the pre-plan wrapper.
	if task.TaskPlan != nil {
		plan.Notify = task.TaskPlan.Notify
	}
	// Register in tracer BEFORE Ingest — Ingest publishes TaskReady,
	// and TaskBoard.Putup reads from tracer.
	for _, node := range plan.Nodes {
		p.tt.Import(&APITask.Task{
			Meta: APITask.TaskMeta{ID: node.ID, Owner: plan.Owner},
			Spec: node.Spec,
		})
	}

	if err := p.sm.Ingest(plan); err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	log.Printf("task_planner: plan %s -> %d nodes", plan.ID, len(plan.Nodes))
	return nil
}

func buildDecomposition(request string) string {
	// TEST: 8 nodes
	return fmt.Sprintf(`Divide into 8 steps, Decompose this request into a DAG of sub-tasks. Return ONLY valid JSON.

Request: %s

Each sub-task has these fields:
- "id": short name for this step (e.g. "search", "summarize"). Will be prefixed with plan ID.
- "prompt": what this step does
- "tools": list of tools needed (e.g. "web_search", "image_gen"), omit if none
- "labels": ["cpu"] for text work, ["gpu"] for image work
- "deps": list of sub-task IDs that must finish before this one (empty array = no dependencies)

Format: {"nodes": [{"id":"step1","prompt":"...","tools":[],"labels":["cpu"],"deps":[]}]}`, request)
}

func (p *planner) generate(ctx context.Context, decomPrompt string) (string, error) {
	prov, err := p.pc.GetPlanProv()
	if err != nil {
		return "", err
	}

	models := prov.GetModels()
	if len(models) == 0 {
		return "", fmt.Errorf("no model available for planning")
	}
	model := models[0]

	// generate plan string — leave most of the context window for input.
	maxOut := prov.MaxContextTokens() / 4
	if maxOut < 256 {
		maxOut = 256
	}
	resp, err := prov.Generate(ctx, &APIChat.ChatRequest{
		Model: model,
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: decomPrompt}},
		}},
		Options: APIChat.ModelOptions{
			MaxTokens:   maxOut,
			Temperature: 0.7,
		},
	})
	if err != nil {
		return "", fmt.Errorf("planning generation failed: %w", err)
	}

	planStr := textFromResponse(resp)
	return planStr, nil
}

type rawPlan struct {
	Nodes []struct {
		ID     string   `json:"id"`
		Prompt string   `json:"prompt"`
		Tools  []string `json:"tools"`
		Labels []string `json:"labels"`
		Deps   []string `json:"deps"`
	} `json:"nodes"`
}

func (p *planner) parse(planID APITask.TaskID, owner, raw string) *APITask.TaskPlan {
	jsonStr := extractJSON(raw)

	var parsed rawPlan
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		log.Printf("task_planner: parse failed: %v", err)
		return fallbackPlan(planID, owner, raw)
	}

	nodes := make(map[APITask.TaskID]*APITask.TaskNode)
	successors := make(map[APITask.TaskID][]APITask.TaskID)
	predecessorNums := make(map[APITask.TaskID]int)
	var roots []APITask.TaskID

	for _, n := range parsed.Nodes {
		id := APITask.TaskID(string(planID) + "-" + n.ID)
		nodes[id] = &APITask.TaskNode{
			ID: id,
			Spec: APITask.TaskSpec{
				Prompt:       n.Prompt,
				Tools:        n.Tools,
				Labels:       n.Labels,
				BudgetTokens: 500,
			},
		}
		if len(n.Deps) == 0 {
			roots = append(roots, id)
			predecessorNums[id] = 0
		}
	}

	for _, n := range parsed.Nodes {
		id := APITask.TaskID(string(planID) + "-" + n.ID)
		for _, dep := range n.Deps {
			depID := APITask.TaskID(string(planID) + "-" + dep)
			successors[depID] = append(successors[depID], id)
			predecessorNums[id]++
		}
	}

	// Append reduce node — not LLM-generated, added by the system.
	var leaves []APITask.TaskID
	for id := range nodes {
		if len(successors[id]) == 0 {
			leaves = append(leaves, id)
		}
	}
	reduceID := APITask.TaskID(string(planID) + "-reduce")
	nodes[reduceID] = &APITask.TaskNode{
		ID: reduceID,
		Spec: APITask.TaskSpec{
			Prompt:       "Combine all results into a single final response",
			BudgetTokens: 1000,
			Labels:       []string{"cpu"},
			Stage:        APITask.StageReduce,
		},
	}
	for _, leaf := range leaves {
		successors[leaf] = append(successors[leaf], reduceID)
	}
	predecessorNums[reduceID] = len(leaves)

	return &APITask.TaskPlan{
		ID:              planID,
		Owner:           owner,
		Roots:           roots,
		Nodes:           nodes,
		Successors:      successors,
		PredecessorNums: predecessorNums,
		Deadline:        time.Now().Add(5 * time.Minute),
		CreatedAt:       time.Now(),
	}
}

func fallbackPlan(id APITask.TaskID, owner, prompt string) *APITask.TaskPlan {
	nodeID := APITask.TaskID(string(id) + "-single")
	return &APITask.TaskPlan{
		ID:    id,
		Owner: owner,
		Roots: []APITask.TaskID{nodeID},
		Nodes: map[APITask.TaskID]*APITask.TaskNode{
			nodeID: {
				ID:   nodeID,
				Spec: APITask.TaskSpec{Prompt: prompt, BudgetTokens: 500},
			},
		},
		Successors:      map[APITask.TaskID][]APITask.TaskID{},
		PredecessorNums: map[APITask.TaskID]int{nodeID: 0},
		Deadline:        time.Now().Add(5 * time.Minute),
		CreatedAt:       time.Now(),
	}
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (p *planner) GetModuleType() APIModule.ModuleType {
	return APIModule.TASKPLANNER
}

func (p *planner) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(p.GetModuleType(), "default")
}

func (p *planner) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(p.GetModuleID(), p.GetModuleType(), APIModule.Running)
}

func (p *planner) RegisterWithManager(m modulemanager.ModuleManager) error {
	return m.Register(p)
}

func extractJSON(raw string) string {
	start := strings.Index(raw, "{")
	if start < 0 {
		return raw
	}
	// Count braces to find the first complete JSON object.
	depth := 0
	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
		}
	}
	return raw
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

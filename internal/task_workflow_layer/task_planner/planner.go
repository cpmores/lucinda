// Package taskplanner decomposes a raw task into a TaskPlan of semantic
// transactions and hands it to the TaskCommander. It owns the map from plan
// ID to the terminal Notify channel, so when the commander reports a plan
// done, the planner delivers the PlanResult back to the wrapper. The planner
// never touches a provider: decomposition is issued as a board task and the
// plan is built from the observed result.
package taskplanner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	APISteam "github.com/cpmores/lucinda/api/v1/domain/stream"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/eventx"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// TaskPlanner is the interface for the planner module.
type TaskPlanner interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error
}

type planner struct {
	mm  modulemanager.ModuleManager
	eb  eventbus.EventBus
	log *logger.Logger

	mu       sync.Mutex
	pending  map[APITask.TaskID]*APITask.Task // decomposition taskID → raw task
	notifies map[APITask.TaskID]chan<- APITask.PlanResult
	timeouts map[APITask.TaskID]*time.Timer
	cancel   context.CancelFunc
}

// NewTaskPlanner creates a planner. Deps (EventBus) are resolved from the
// module registry via DependsEnable; the module manager is captured at
// RegisterWithManager.
func NewTaskPlanner(log *logger.Logger) TaskPlanner {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &planner{
		log:      log,
		pending:  make(map[APITask.TaskID]*APITask.Task),
		notifies: make(map[APITask.TaskID]chan<- APITask.PlanResult),
		timeouts: make(map[APITask.TaskID]*time.Timer),
	}
}

// ── Lifecycle ──────────────────────────────────────────────────────────

func (p *planner) Start(ctx context.Context) error {
	ctx, p.cancel = context.WithCancel(ctx)

	eventx.Watch(ctx, p.eb, p.log, APIEvent.TaskPreplanned, p.onPreplanned)
	eventx.Watch(ctx, p.eb, p.log, APIEvent.TaskTraced, p.onDecomposition)
	eventx.Watch(ctx, p.eb, p.log, APIEvent.TaskPlanDone, p.onPlanDone)

	p.log.Info("started")
	return nil
}

func (p *planner) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	p.log.Info("stopped")
	return nil
}

// ── Planning ──────────────────────────────────────────────────────────

// onPreplanned issues the semantic decomposition as a reason task through
// the board, storing the raw task for when the result returns.
func (p *planner) onPreplanned(data any) {
	task, ok := data.(*APITask.Task)
	if !ok {
		return
	}
	eventx.Emit(p.eb, p.log, APIEvent.TelemetryPlanning, APISteam.StatusData{
		Component: "planner", State: "planning", Owner: task.Meta.Owner,
		PlanID: string(task.Meta.ID), TaskID: string(task.Meta.ID),
	})

	id := task.Meta.ID + "-decompose"
	decomTask := &APITask.Task{
		Meta:     APITask.TaskMeta{ID: id, Type: APITask.StagePlan, Owner: task.Meta.Owner},
		Spec:     APITask.TaskSpec{Prompt: buildSemanticDecomposition(task.Spec.Prompt), Kind: APITask.TaskKindReason, BudgetTokens: 1000, Labels: []string{"cpu"}},
		TaskPlan: task.TaskPlan,
	}

	p.mu.Lock()
	p.pending[id] = task
	p.mu.Unlock()
	_ = p.eb.Publish(APIEvent.TaskReady, APIEvent.NewEvent(APIEvent.TaskReady, decomTask))
	p.log.Info("decomposition issued", "plan", task.Meta.ID, "task", id)
}

// onDecomposition builds the plan once the decomposition task completes.
func (p *planner) onDecomposition(data any) {
	msg, ok := data.(APITaskmsg.TaskTracedMsg)
	if !ok {
		return
	}
	// The tracer also emits Running; only act on terminal states.
	if msg.State != APITask.StateDone && msg.State != APITask.StateFailed {
		return
	}
	p.mu.Lock()
	original := p.pending[msg.TaskID]
	if original == nil {
		p.mu.Unlock()
		return
	}
	delete(p.pending, msg.TaskID)
	p.mu.Unlock()

	var plan *APITask.TaskPlan
	if msg.State == APITask.StateDone {
		plan = p.parseSemantic(original.Meta.ID, original.Meta.Owner, msg.Output)
	} else {
		p.log.Warn("decomposition failed, using fallback", "task", msg.TaskID)
		plan = fallbackPlan(original.Meta.ID, original.Meta.Owner, original.Spec.Prompt)
	}

	if original.TaskPlan != nil {
		plan.Notify = original.TaskPlan.Notify
		if original.TaskPlan.Architecture != "" {
			plan.Architecture = original.TaskPlan.Architecture
		}
	}
	if plan.Architecture == "" {
		plan.Architecture = APITask.ArchPlanExecute
	}
	plan.Goal = original.Spec.Prompt

	p.mu.Lock()
	p.notifies[plan.ID] = plan.Notify
	until := time.Until(plan.Deadline)
	if until <= 0 {
		until = time.Minute
	}
	p.timeouts[plan.ID] = time.AfterFunc(until, func() { p.onTimeout(plan.ID) })
	p.mu.Unlock()

	eventx.Emit(p.eb, p.log, APIEvent.TelemetryPlanned, APISteam.StatusData{
		Component: "planner", State: "planned", Owner: plan.Owner,
		PlanID: string(plan.ID), TaskID: string(plan.ID),
	})
	_ = p.eb.Publish(APIEvent.TaskPlanned, APIEvent.NewEvent(APIEvent.TaskPlanned, plan))
	p.log.Info("plan produced", "plan", plan.ID, "transactions", len(plan.Transactions))
}

// onPlanDone forwards the terminal plan result to the wrapper's Notify
// channel (exactly once), then drops the plan from the map.
func (p *planner) onPlanDone(data any) {
	msg, ok := data.(APITaskmsg.TaskPlanResultMsg)
	if !ok {
		return
	}
	p.mu.Lock()
	notify, ok := p.notifies[msg.PlanID]
	if ok {
		delete(p.notifies, msg.PlanID)
	}
	if timer, ok := p.timeouts[msg.PlanID]; ok {
		timer.Stop()
		delete(p.timeouts, msg.PlanID)
	}
	p.mu.Unlock()
	if !ok {
		p.log.Warn("plan done for unknown plan", "plan", msg.PlanID)
		return
	}
	p.deliver(msg.PlanID, notify, msg.Result)
}

// onTimeout fires when a plan exceeds its deadline without completing. It
// delivers a timeout PlanResult exactly once (the delete in onPlanDone makes
// completion and timeout mutually exclusive).
func (p *planner) onTimeout(planID APITask.TaskID) {
	p.mu.Lock()
	notify, ok := p.notifies[planID]
	if ok {
		delete(p.notifies, planID)
	}
	delete(p.timeouts, planID)
	p.mu.Unlock()
	if !ok {
		return
	}
	p.deliver(planID, notify, APITask.PlanResult{Status: APITask.PlanTimeout, Text: "plan deadline exceeded"})
}

func (p *planner) deliver(planID APITask.TaskID, notify chan<- APITask.PlanResult, result APITask.PlanResult) {
	select {
	case notify <- result:
		p.log.Info("plan result delivered", "plan", planID, "status", result.Status)
	default:
		p.log.Warn("notify channel full, dropping plan result", "plan", planID)
	}
}

// ── LLM decomposition ──────────────────────────────────────────────────

func buildSemanticDecomposition(request string) string {
	return fmt.Sprintf(`Decompose this request into semantic transactions. Each transaction is ONE deliverable the user can see ("write the doc", "generate the video", "adjust the AC"). Return ONLY valid JSON.

Request: %s

Each transaction has:
- "id": short name (e.g. "write_doc"). Will be prefixed with the plan ID.
- "goal": what this transaction delivers
- "tools": list of tools needed, omit if none
- "labels": ["cpu"] for text work, ["gpu"] for image work
- "deps": list of transaction IDs that must finish first (empty = independent)

Format: {"transactions":[{"id":"t1","goal":"...","tools":[],"labels":["cpu"],"deps":[]}]}`, request)
}

// (provider selection for planning moved behind the executor: decomposition
// is issued as a reason task and routed by the board.)

type rawTransactions struct {
	Transactions []struct {
		ID     string   `json:"id"`
		Goal   string   `json:"goal"`
		Tools  []string `json:"tools"`
		Labels []string `json:"labels"`
		Deps   []string `json:"deps"`
	} `json:"transactions"`
}

func (p *planner) parseSemantic(planID APITask.TaskID, owner, raw string) *APITask.TaskPlan {
	jsonStr := extractJSON(raw)

	var parsed rawTransactions
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil || len(parsed.Transactions) == 0 {
		p.log.Warn("semantic parse failed, using fallback", "err", err)
		return fallbackPlan(planID, owner, raw)
	}

	idOf := func(name string) APITask.TaskID { return APITask.TaskID(string(planID) + "-" + name) }
	known := map[string]bool{}
	for _, tx := range parsed.Transactions {
		known[tx.ID] = true
	}

	var txs []APITask.Transaction
	for _, tx := range parsed.Transactions {
		var deps []APITask.TaskID
		for _, d := range tx.Deps {
			if !known[d] {
				continue // drop dangling deps
			}
			deps = append(deps, idOf(d))
		}
		txs = append(txs, APITask.Transaction{
			ID: idOf(tx.ID), Goal: tx.Goal,
			Tools: tx.Tools, Labels: tx.Labels, Deps: deps,
		})
	}

	if cycle, ok := findCycle(txs); ok {
		p.log.Warn("transaction dependency cycle, using fallback", "cycle", cycle)
		return fallbackPlan(planID, owner, raw)
	}

	return &APITask.TaskPlan{
		ID: planID, Owner: owner, Transactions: txs,
		Deadline: time.Now().Add(5 * time.Minute), CreatedAt: time.Now(),
	}
}

func fallbackPlan(id APITask.TaskID, owner, prompt string) *APITask.TaskPlan {
	return &APITask.TaskPlan{
		ID: id, Owner: owner,
		Transactions: []APITask.Transaction{{ID: id + "-single", Goal: prompt}},
		Deadline:     time.Now().Add(5 * time.Minute),
		CreatedAt:    time.Now(),
	}
}

// findCycle reports whether the transaction dependency graph contains a
// cycle (Kahn's topological sort; a leftover unprocessed node is cyclic).
func findCycle(txs []APITask.Transaction) (string, bool) {
	adj := map[APITask.TaskID][]APITask.TaskID{}
	indeg := map[APITask.TaskID]int{}
	for _, tx := range txs {
		indeg[tx.ID] = 0
	}
	for _, tx := range txs {
		for _, d := range tx.Deps {
			adj[d] = append(adj[d], tx.ID)
			indeg[tx.ID]++
		}
	}
	queue := []APITask.TaskID{}
	for id, n := range indeg {
		if n == 0 {
			queue = append(queue, id)
		}
	}
	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		for _, succ := range adj[id] {
			indeg[succ]--
			if indeg[succ] == 0 {
				queue = append(queue, succ)
			}
		}
	}
	if processed == len(txs) {
		return "", false
	}
	return "transaction dependency cycle", true
}

func extractJSON(raw string) string {
	start := strings.Index(raw, "{")
	if start < 0 {
		return raw
	}
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

// ── AvailableModule Interface ──────────────────────────────────────────

func (p *planner) GetModuleType() APIModule.ModuleType { return APIModule.TaskPlanner }
func (p *planner) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(p.GetModuleType(), "default")
}
func (p *planner) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(p.GetModuleID(), p.GetModuleType(), APIModule.Running)
}
func (p *planner) RegisterWithManager(m modulemanager.ModuleManager) error {
	p.mm = m
	return m.Register(p)
}
func (p *planner) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{APIModule.EventBus: "default"}
}
func (p *planner) DependsEnable() error {
	id := APIModule.NewModuleID(APIModule.EventBus, "default")
	mod, err := p.mm.Get(id)
	if err != nil {
		return fmt.Errorf("resolve dependency %s: %w", id, err)
	}
	eb, ok := mod.(eventbus.EventBus)
	if !ok {
		return fmt.Errorf("dependency %s is not an EventBus", id)
	}
	p.eb = eb
	return nil
}

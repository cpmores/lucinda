// Package taskcommander orchestrates a plan's semantic transactions. Each
// transaction gets its own Commander execution: under Plan-and-Execute it
// dispatches the transaction goal once through the board; under ReAct it runs
// a reasoning loop (reason → act → observe) against the transaction goal.
// Independent transactions run in parallel; dependent ones (Deps) run in
// order, with the dependency outputs fed into the downstream goal context.
package taskcommander

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	APISteam "github.com/cpmores/lucinda/api/v1/domain/stream"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/eventx"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// TaskCommander is the interface for the commander module.
type TaskCommander interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error
}

// planRun is the execution state of one plan across all its transactions.
type planRun struct {
	plan   *APITask.TaskPlan
	txRuns map[APITask.TaskID]*txRun
	done   bool
}

// txRun is one transaction's Commander execution: its own goal, ReAct
// trajectory, step budget, and result.
type txRun struct {
	run         *planRun
	tx          *APITask.Transaction
	trajectory  []reactStep
	steps       int
	maxSteps    int
	output      string
	finalAnswer string
	pending     APITask.TaskKind // what task we are waiting on
	inFlight    bool
	done        bool
	tasks       []APITask.TaskID // tasks issued for this transaction
}

// reactStep is one action/result pair in a transaction's ReAct trajectory.
type reactStep struct {
	action string
	prompt string
	result string
}

// reactDecision is the structured output of the reasoning LLM.
type reactDecision struct {
	Action string            `json:"action"` // "continue" | "done"
	Task   *APITask.TaskSpec `json:"task,omitempty"`
	Answer string            `json:"answer,omitempty"`
}

const defaultMaxSteps = 10

type commander struct {
	mm  modulemanager.ModuleManager
	eb  eventbus.EventBus
	tt  tasktracer.TaskTracer
	log *logger.Logger

	ctx    context.Context
	mu     sync.Mutex
	plans  map[APITask.TaskID]*planRun // planID → planRun
	taskTx map[APITask.TaskID]*txRun   // taskID → its transaction run
	cancel context.CancelFunc
}

// NewTaskCommander creates a commander. Deps are resolved via DependsEnable;
// the module manager is captured at RegisterWithManager.
func NewTaskCommander(log *logger.Logger) TaskCommander {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &commander{
		log:    log,
		plans:  make(map[APITask.TaskID]*planRun),
		taskTx: make(map[APITask.TaskID]*txRun),
	}
}

func (c *commander) Start(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)
	c.ctx = ctx

	eventx.Watch(ctx, c.eb, c.log, APIEvent.TaskPlanned, func(data any) {
		if plan, ok := data.(*APITask.TaskPlan); ok {
			c.onPlanned(plan)
		}
	})
	eventx.Watch(ctx, c.eb, c.log, APIEvent.TaskTraced, func(data any) {
		if msg, ok := data.(APITaskmsg.TaskTracedMsg); ok {
			c.onTraced(&msg)
		}
	})

	c.log.Info("started")
	return nil
}

func (c *commander) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.log.Info("stopped")
	return nil
}

// ── Plan orchestration ────────────────────────────────────────────────

func (c *commander) onPlanned(plan *APITask.TaskPlan) {
	eventx.Emit(c.eb, c.log, APIEvent.TelemetryThinking, APISteam.StatusData{
		Component: "commander", State: "thinking", Owner: plan.Owner,
		PlanID: string(plan.ID), TaskID: string(plan.ID),
	})

	// Backward-compat: a plan without explicit transactions is one
	// transaction carrying the whole goal.
	txs := plan.Transactions
	if len(txs) == 0 {
		txs = []APITask.Transaction{{ID: plan.ID + "-single", Goal: plan.Goal}}
	}

	pr := &planRun{plan: plan, txRuns: make(map[APITask.TaskID]*txRun, len(txs))}
	for i := range txs {
		tx := &txs[i]
		mx := defaultMaxSteps
		if plan.MaxSteps > 0 {
			mx = plan.MaxSteps
		}
		pr.txRuns[tx.ID] = &txRun{run: pr, tx: tx, maxSteps: mx}
	}

	c.mu.Lock()
	c.plans[plan.ID] = pr
	c.mu.Unlock()

	eventx.Emit(c.eb, c.log, APIEvent.TelemetryWaiting, APISteam.StatusData{
		Component: "commander", State: "waiting", Owner: plan.Owner,
		PlanID: string(plan.ID), TaskID: string(plan.ID),
	})
	c.startReady(pr)
	c.log.Info("plan ingested", "plan", plan.ID, "transactions", len(txs))
}

// startReady launches the runnable transactions — those whose dependencies
// are all done and that have not started. Independent ones run concurrently.
func (c *commander) startReady(pr *planRun) {
	for _, txr := range pr.txRuns {
		if txr.done || txr.inFlight {
			continue
		}
		if !depsDone(pr, txr.tx) {
			continue
		}
		go c.runTransaction(pr, txr)
	}
}

// depsDone reports whether every dependency of tx has completed.
func depsDone(pr *planRun, tx *APITask.Transaction) bool {
	for _, dep := range tx.Deps {
		if dr := pr.txRuns[dep]; dr == nil || !dr.done {
			return false
		}
	}
	return true
}

// runTransaction executes one transaction under the plan's architecture.
func (c *commander) runTransaction(pr *planRun, txr *txRun) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pr.plan.Architecture == APITask.ArchReAct {
		c.issueReason(pr, txr)
		return
	}
	// Plan-and-Execute: dispatch the transaction goal once as an execute task.
	c.issueTask(pr, txr, APITask.TaskKindExecute, txr.tx.Goal, APITask.TaskSpec{
		Prompt:       txr.tx.Goal,
		Tools:        txr.tx.Tools,
		Labels:       txr.tx.Labels,
		Model:        txr.tx.Model,
		BudgetTokens: 500,
	})
}

// onTraced advances the owning transaction from a tracer update. The handler
// dispatches by the kind of task we were waiting on: reason → parse the
// decision; execute → record the result and reason again; synthesize → the
// answer is streamed, finish the transaction. A Failed action terminates the
// plan.
func (c *commander) onTraced(msg *APITaskmsg.TaskTracedMsg) {
	c.mu.Lock()
	defer c.mu.Unlock()

	txr := c.taskTx[msg.TaskID]
	if txr == nil || txr.done {
		return
	}
	pr := txr.run

	switch msg.State {
	case APITask.StateDone:
		txr.inFlight = false
		switch txr.pending {
		case APITask.TaskKindReason:
			c.onReasonResult(pr, txr, msg.Output)
		case APITask.TaskKindSynthesize:
			// The answer was streamed by the executor; output carries the text.
			c.finishTx(pr, txr, msg.Output)
		default: // execute
			if pr.plan.Architecture == APITask.ArchReAct {
				c.onExecuteResult(pr, txr, msg.Output)
			} else {
				// Plan-and-Execute: the transaction is done after its one action.
				eventx.Emit(c.eb, c.log, APIEvent.TelemetryStepResult, APISteam.StepResultData{
					Owner: msg.Owner, PlanID: string(pr.plan.ID),
					TaskID: string(msg.TaskID), Output: msg.Output,
				})
				c.finishTx(pr, txr, msg.Output)
			}
		}
	case APITask.StateReleased:
		// The executor failed and the board is reassigning the task — this is
		// not terminal. The board emits the final Failed only when candidates
		// are exhausted, which is what fails the plan.
	case APITask.StateFailed:
		c.emitFinalizing(pr)
		c.terminate(pr, APITask.PlanResult{Status: APITask.PlanError, Text: "transaction action failed"})
	}
}

// onReasonResult parses a ReAct decision and acts: continue issues an
// execute task; done issues a synthesize task.
func (c *commander) onReasonResult(pr *planRun, txr *txRun, raw string) {
	decision, err := parseDecision(raw)
	if err != nil || decision == nil {
		c.log.Warn("react decision unparseable, synthesizing", "plan", pr.plan.ID, "tx", txr.tx.ID, "err", err)
		c.finishOrSynthesize(pr, txr)
		return
	}
	switch decision.Action {
	case "done":
		c.emitFinalizing(pr)
		txr.finalAnswer = decision.Answer
		c.issueSynthesize(pr, txr)
	case "continue":
		if decision.Task == nil || txr.steps >= txr.maxSteps {
			c.log.Info("react loop hit max steps, synthesizing", "plan", pr.plan.ID, "tx", txr.tx.ID, "steps", txr.steps)
			c.finishOrSynthesize(pr, txr)
			return
		}
		txr.steps++
		txr.trajectory = append(txr.trajectory, reactStep{action: "continue", prompt: decision.Task.Prompt})
		eventx.Emit(c.eb, c.log, APIEvent.TelemetryThinking, APISteam.StatusData{
			Component: "commander", State: fmt.Sprintf("react step %d", txr.steps),
			Owner: pr.plan.Owner, PlanID: string(pr.plan.ID), TaskID: string(pr.plan.ID),
		})
		// The executor selects the provider by capability; a model the LLM
		// hallucinated would disqualify every board bid and stall the plan.
		task := *decision.Task
		task.Model = ""
		c.issueTask(pr, txr, APITask.TaskKindExecute, task.Prompt, task)
	default:
		c.log.Warn("unknown react action", "plan", pr.plan.ID, "action", decision.Action)
		c.finishOrSynthesize(pr, txr)
	}
}

// finishOrSynthesize completes a transaction: if it performed work, issue a
// synthesize task so the answer is generated from the trajectory; otherwise
// finish with whatever output was collected. done is NOT set here — the
// synthesize result still needs to reach onTraced.
func (c *commander) finishOrSynthesize(pr *planRun, txr *txRun) {
	if len(txr.trajectory) > 0 {
		c.emitFinalizing(pr)
		c.issueSynthesize(pr, txr)
		return
	}
	c.finishTx(pr, txr, txr.output)
}

// onExecuteResult records a ReAct action's result in the trajectory and asks
// for the next decision.
func (c *commander) onExecuteResult(pr *planRun, txr *txRun, output string) {
	eventx.Emit(c.eb, c.log, APIEvent.TelemetryStepResult, APISteam.StepResultData{
		Owner: pr.plan.Owner, PlanID: string(pr.plan.ID),
		TaskID: string(txr.tx.ID), Output: output,
	})
	if n := len(txr.trajectory); n > 0 {
		txr.trajectory[n-1].result = output
	}
	c.issueReason(pr, txr)
}

// issueReason issues a reason-marked task carrying the decision prompt.
func (c *commander) issueReason(pr *planRun, txr *txRun) {
	prompt := buildReActPrompt(txr)
	c.issueTask(pr, txr, APITask.TaskKindReason, prompt, APITask.TaskSpec{Prompt: prompt, BudgetTokens: 300})
}

// issueSynthesize issues a synthesize-marked task; its executor streams the
// answer through the router.
func (c *commander) issueSynthesize(pr *planRun, txr *txRun) {
	prompt := buildAnswerPrompt(txr)
	c.issueTask(pr, txr, APITask.TaskKindSynthesize, prompt, APITask.TaskSpec{Prompt: prompt, BudgetTokens: 500})
}

// issueTask publishes one task through the TaskReady → board → executor path,
// scoped to its transaction and kind. Caller must hold c.mu.
func (c *commander) issueTask(pr *planRun, txr *txRun, kind APITask.TaskKind, prompt string, spec APITask.TaskSpec) {
	id := APITask.TaskID(fmt.Sprintf("%s-%s-%d", pr.plan.ID, txr.tx.ID, len(txr.tasks)))
	txr.pending = kind
	txr.inFlight = true
	txr.tasks = append(txr.tasks, id)
	c.taskTx[id] = txr
	spec.Prompt = prompt
	spec.Kind = kind
	task := &APITask.Task{
		Meta:     APITask.TaskMeta{ID: id, Type: APITask.StageExecute, Owner: pr.plan.Owner},
		Spec:     spec,
		TaskPlan: pr.plan,
	}
	_ = c.tt.Import(task)
	_ = c.eb.Publish(APIEvent.TaskReady, APIEvent.NewEvent(APIEvent.TaskReady, task))
	c.log.Info("task issued", "plan", pr.plan.ID, "tx", txr.tx.ID, "task", id, "kind", kind)
}

// finishTx completes a transaction with its output and cascades to
// newly-ready transactions and plan completion. Caller must hold c.mu.
func (c *commander) finishTx(pr *planRun, txr *txRun, output string) {
	txr.output = output
	txr.done = true
	txr.inFlight = false
	c.startReady(pr)
	if c.allTxDone(pr) {
		c.finalizePlan(pr)
	}
}

func (c *commander) allTxDone(pr *planRun) bool {
	for _, txr := range pr.txRuns {
		if !txr.done {
			return false
		}
	}
	return true
}

// ── ReAct loop (per transaction) ──────────────────────────────────────

// parseDecision parses the structured decision JSON from a reason task.
func parseDecision(raw string) (*reactDecision, error) {
	var decision reactDecision
	if err := json.Unmarshal([]byte(extractJSON(raw)), &decision); err != nil {
		return nil, fmt.Errorf("parse react decision: %w", err)
	}
	return &decision, nil
}

// buildReActPrompt assembles the reasoning prompt from the transaction goal
// and its trajectory. Caller must hold c.mu.
// maxTrajectorySteps bounds how much of the history is fed back to the
// reasoner and synthesizer, keeping prompts inside the model context window.
const maxTrajectorySteps = 5

func buildReActPrompt(txr *txRun) string {
	var b strings.Builder
	b.WriteString("You are a task orchestrator deciding the next step for a request.\n")
	b.WriteString("Goal: " + txr.tx.Goal + "\n\n")
	if dep := depOutputs(txr); dep != "" {
		b.WriteString("Inputs from completed dependencies:\n" + dep + "\n\n")
	}
	b.WriteString("Actions performed so far (most recent first):\n")
	if len(txr.trajectory) == 0 {
		b.WriteString("(none)\n")
	}
	steps := txr.trajectory
	if len(steps) > maxTrajectorySteps {
		steps = steps[len(steps)-maxTrajectorySteps:]
	}
	for _, s := range steps {
		fmt.Fprintf(&b, "- action: %s\n  prompt: %s\n  result: %s\n", s.action, s.prompt, s.result)
	}
	b.WriteString("\nDecide the next step. Reply ONLY with JSON:\n")
	b.WriteString(`{"action":"continue","task":{"prompt":"what to do","model":"","labels":[],"tools":[]}}` + "\n")
	b.WriteString(`or {"action":"done","answer":"the final answer to the user"}`)
	return b.String()
}

// depOutputs collects the outputs of this transaction's completed
// dependencies for injection into the goal context.
func depOutputs(txr *txRun) string {
	var b strings.Builder
	for _, dep := range txr.tx.Deps {
		if dr := txr.run.txRuns[dep]; dr != nil && dr.done {
			fmt.Fprintf(&b, "- %s: %s\n", dep, dr.output)
		}
	}
	return b.String()
}

// buildAnswerPrompt instructs the synthesis LLM to produce the final user
// answer for this transaction.
func buildAnswerPrompt(txr *txRun) string {
	var b strings.Builder
	b.WriteString("Write the final answer to the user for this request.\n")
	b.WriteString("Goal: " + txr.tx.Goal + "\n\n")
	b.WriteString("Work performed (most recent first):\n")
	steps := txr.trajectory
	if len(steps) > maxTrajectorySteps {
		steps = steps[len(steps)-maxTrajectorySteps:]
	}
	for _, s := range steps {
		fmt.Fprintf(&b, "- action: %s\n  result: %s\n", s.prompt, s.result)
	}
	b.WriteString("\nAnswer directly, no preamble.")
	return b.String()
}

// ── Plan completion ───────────────────────────────────────────────────

func (c *commander) emitFinalizing(pr *planRun) {
	eventx.Emit(c.eb, c.log, APIEvent.TelemetryFinalizing, APISteam.StatusData{
		Component: "commander", State: "finalizing", Owner: pr.plan.Owner,
		PlanID: string(pr.plan.ID), TaskID: string(pr.plan.ID),
	})
}

// finalizePlan merges the per-transaction results and terminates. Caller
// must hold c.mu.
func (c *commander) finalizePlan(pr *planRun) {
	if pr.done {
		return
	}
	pr.done = true
	text := mergeTxOutputs(pr)
	c.emitFinalizing(pr)
	c.terminate(pr, APITask.PlanResult{Status: APITask.PlanOK, Text: text})
}

// terminate publishes TaskPlanDone exactly once and cleans up. Caller must
// hold c.mu.
func (c *commander) terminate(pr *planRun, result APITask.PlanResult) {
	msg := APITaskmsg.TaskPlanResultMsg{PlanID: pr.plan.ID, Result: result}
	_ = c.eb.Publish(APIEvent.TaskPlanDone, APIEvent.NewEvent(APIEvent.TaskPlanDone, msg))
	delete(c.plans, pr.plan.ID)
	for _, txr := range pr.txRuns {
		for _, id := range txr.tasks {
			delete(c.taskTx, id)
		}
	}
	c.log.Info("plan done", "plan", pr.plan.ID, "status", result.Status)
}

// mergeTxOutputs concatenates transaction results in a topological
// (dependency-respecting) order.
func mergeTxOutputs(pr *planRun) string {
	var ordered []APITask.TaskID
	added := map[APITask.TaskID]bool{}
	for len(ordered) < len(pr.txRuns) {
		for id, txr := range pr.txRuns {
			if added[id] {
				continue
			}
			ready := true
			for _, dep := range txr.tx.Deps {
				if !added[dep] {
					ready = false
					break
				}
			}
			if ready {
				ordered = append(ordered, id)
				added[id] = true
			}
		}
	}
	var b strings.Builder
	for _, id := range ordered {
		if out := pr.txRuns[id].output; strings.TrimSpace(out) != "" {
			b.WriteString(out)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (c *commander) GetModuleType() APIModule.ModuleType { return APIModule.TaskCommander }
func (c *commander) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(c.GetModuleType(), "default")
}
func (c *commander) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(c.GetModuleID(), c.GetModuleType(), APIModule.Running)
}
func (c *commander) RegisterWithManager(m modulemanager.ModuleManager) error {
	c.mm = m
	return m.Register(c)
}
func (c *commander) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{
		APIModule.EventBus:   "default",
		APIModule.TaskTracer: "default",
	}
}
func (c *commander) DependsEnable() error {
	for depType, name := range c.DependsOn() {
		id := APIModule.NewModuleID(depType, name)
		mod, err := c.mm.Get(id)
		if err != nil {
			return fmt.Errorf("resolve dependency %s: %w", id, err)
		}
		switch depType {
		case APIModule.EventBus:
			eb, ok := mod.(eventbus.EventBus)
			if !ok {
				return fmt.Errorf("dependency %s is not an EventBus", id)
			}
			c.eb = eb
		case APIModule.TaskTracer:
			tt, ok := mod.(tasktracer.TaskTracer)
			if !ok {
				return fmt.Errorf("dependency %s is not a TaskTracer", id)
			}
			c.tt = tt
		}
	}
	return nil
}

// extractJSON isolates the first complete JSON object from the LLM's reply.
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

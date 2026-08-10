// Package apitask defines types for task decomposition, scheduling, and lifecycle tracking.
package apitask

import "time"

// ── PlanResult ────────────────────────────────────────────────────────

// PlanStatus is the termination status of a plan.
type PlanStatus string

const (
	PlanOK        PlanStatus = "ok"
	PlanError     PlanStatus = "error"
	PlanTimeout   PlanStatus = "timeout"
	PlanCancelled PlanStatus = "cancelled"
)

// PlanResult is sent to the Notify channel when a plan terminates
// (successfully or otherwise). Every termination path must send exactly
// one PlanResult so the SSE handler never blocks forever.
type PlanResult struct {
	Status PlanStatus `json:"status"`
	Text   string     `json:"text"`
}

// ── TaskPlan ──────────────────────────────────────────────────────────

// AgentArch is the execution architecture a plan runs under. Phase 1 ships
// Plan-Execute; ReAct is reserved for the reAct loop.
type AgentArch string

const (
	ArchPlanExecute AgentArch = "plan_execute"
	ArchReAct       AgentArch = "react"
)

// Transaction is one semantic unit of work the planner splits a request
// into (e.g. "write the doc", "generate the video"). A transaction is the
// unit a Commander executes: its Goal drives a ReAct loop, or is dispatched
// as a single action under Plan-and-Execute. Deps lists prerequisite
// transactions whose results are fed into the goal context.
type Transaction struct {
	ID   TaskID   `json:"id"`
	Goal string   `json:"goal"`
	Deps []TaskID `json:"deps,omitempty"`

	// Capability hints for board matching.
	Tools  []string `json:"tools,omitempty"`
	Labels []string `json:"labels,omitempty"`
	Model  string   `json:"model,omitempty"`
}

// TaskPlan is a decomposed macro-task ready for the TaskBoard.
// The TaskPlanner builds it; the TaskStateManager tracks it.
type TaskPlan struct {
	// ── identifies ──────────────────────────────────────────────────────────
	ID    TaskID `json:"id"`
	Owner string `json:"owner"`

	// ── Execution model ────────────────────────────────────────────────
	// Architecture governs whether this plan is executed as a static DAG
	// (plan_execute) or as a reAct loop where the commander decides the next
	// step from each task result (react).
	Architecture AgentArch `json:"architecture,omitempty"`
	// Goal is the original user request. Plan-Execute decomposes it into the
	// DAG; ReAct feeds it to the reasoning LLM each iteration.
	Goal string `json:"goal,omitempty"`
	// MaxSteps bounds the ReAct loop (0 = default cap). Unused by
	// Plan-Execute.
	MaxSteps int `json:"max_steps,omitempty"`

	// ── Transactions ───────────────────────────────────────────────────
	// Transactions are the semantic units of work the planner decomposes the
	// request into. Each is handled by its own Commander. Independent
	// transactions run in parallel; dependent ones (Deps) run in order.
	Transactions []Transaction `json:"transactions,omitempty"`

	// ── Nodes ──────────────────────────────────────────────────────────
	Roots           []TaskID             `json:"roots"`            // entry points with no dependencies
	Nodes           map[TaskID]*TaskNode `json:"nodes"`            // all nodes, indexed by ID for easy lookup
	Successors      map[TaskID][]TaskID  `json:"successors"`       // edges defining dependencies between nodes
	PredecessorNums map[TaskID]int       `json:"predecessor_nums"` // number of unsatisfied dependencies for each node

	// ── Timeline ──────────────────────────────────────────────────────────
	Deadline  time.Time `json:"deadline"`
	CreatedAt time.Time `json:"created_at"`

	// ── Callback ─────────────────────────────────────────────────────────
	Notify chan<- PlanResult `json:"-"` // terminal notification — sent exactly once per plan
}

// ToTasks materializes every node in the plan into a Task, propagating the
// plan's Owner into each task's Meta. A remote executor that sees only one
// task still knows where to route results and telemetry.
func (p *TaskPlan) ToTasks() []Task {
	tasks := make([]Task, 0, len(p.Nodes))
	for id, node := range p.Nodes {
		tasks = append(tasks, Task{
			Meta:     TaskMeta{ID: id, Type: node.Spec.Stage, Owner: p.Owner},
			Spec:     node.Spec,
			TaskPlan: p,
			TaskNode: node,
		})
	}
	return tasks
}

// TaskNode is a single sub-task in the DAG.
type TaskNode struct {
	ID TaskID `json:"id"`

	// What to execute — set by TaskPlanner
	Spec TaskSpec `json:"spec"`

	// Scheduling state — populated by TaskStateManager
	State     NodeState `json:"state"`
	ClaimedBy string    `json:"claimed_by,omitempty"`
	ExpiresAt int64     `json:"expires_at,omitempty"` // lease TTL, unix timestamp
}

// NodeState is the lifecycle stage of a sub-task node.
type NodeState string

const (
	StatePending  NodeState = "pending"  // blocked by upstream dependencies
	StateReady    NodeState = "ready"    // all deps satisfied, available for claiming
	StateClaimed  NodeState = "claimed"  // leased by a peer
	StateRunning  NodeState = "running"  // peer confirmed, executing
	StateDone     NodeState = "done"     // completed successfully
	StateFailed   NodeState = "failed"   // execution failed, retryable
	StateDisposed NodeState = "disposed" // cancelled by owner, will never run
)

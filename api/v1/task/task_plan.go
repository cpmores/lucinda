// Package apitask defines types for task decomposition, scheduling, and lifecycle tracking.
package apitask

import "time"

// ── TaskPlan ──────────────────────────────────────────────────────────

// TaskPlan is a decomposed macro-task ready for the TaskBoard.
// The TaskPlanner builds it; the TaskStateManager tracks it.
type TaskPlan struct {
	// ── identifies ──────────────────────────────────────────────────────────
	ID    TaskID `json:"id"`
	Owner string `json:"owner"`

	// ── Nodes ──────────────────────────────────────────────────────────
	Roots           []TaskID             `json:"roots"`            // entry points with no dependencies
	Nodes           map[TaskID]*TaskNode `json:"nodes"`            // all nodes, indexed by ID for easy lookup
	Successors      map[TaskID][]TaskID  `json:"successors"`       // edges defining dependencies between nodes
	PredecessorNums map[TaskID]int       `json:"predecessor_nums"` // number of unsatisfied dependencies for each node

	// ── Timeline ──────────────────────────────────────────────────────────
	Deadline  time.Time `json:"deadline"`
	CreatedAt time.Time `json:"created_at"`
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

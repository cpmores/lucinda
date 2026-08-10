// Package apitaskmsg defines the wire message types exchanged between
// nodes when tasks are advertised, assigned, and returned.
//
// Every message carries the task's Owner NodeID: the plan owner node that
// created the plan and hosts the user-facing SSE stream. The Owner is the
// single routing key for both result delivery and telemetry, so a remote
// executor knows where to send its output and progress without holding the
// full plan.
package apitaskmsg

import (
	APICapability "github.com/cpmores/lucinda/api/v1/domain/capability"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
)

// TaskBroadcastMsg advertises a task to the mesh so nodes can bid on it
// with a Capability CV (Publish-Lease protocol). Broadcast, not unicast.
type TaskBroadcastMsg struct {
	TaskID APITask.TaskID   `json:"task_id"`
	Spec   APITask.TaskSpec `json:"spec"`
	Owner  string           `json:"owner"`
}

// TaskAssignMsg leases a task to a specific executor. The executor uses
// Owner to decide local-vs-remote handling and to route results and
// telemetry back to the plan owner node.
type TaskAssignMsg struct {
	TaskID APITask.TaskID   `json:"task_id"`
	Spec   APITask.TaskSpec `json:"spec"`
	Prompt string           `json:"prompt"`
	Owner  string           `json:"owner"`
	PlanID APITask.TaskID   `json:"plan_id"`
}

// TaskResultMsg carries an executor's output back to the issuing component
// (normally the TaskCommander on the plan owner node).
type TaskResultMsg struct {
	TaskID APITask.TaskID `json:"task_id"`
	Output string         `json:"output"`
	Owner  string         `json:"owner"`
	PlanID APITask.TaskID `json:"plan_id"`
}

// TaskCVMsg is a peer's bid on a task advertisement: a CapabilityCV sent
// back to the ad's owner for scoring (Publish-Lease step 2).
type TaskCVMsg struct {
	TaskID APITask.TaskID              `json:"task_id"`
	CV     APICapability.CapabilityCV  `json:"cv"`
}

// TaskTracedMsg is a lightweight task lifecycle update emitted by the
// TaskTracer. It carries the state and (on completion) the output so the
// commander judges progress without exchanging the full task, and can be
// unicast back to the plan owner like a TaskResultMsg.
type TaskTracedMsg struct {
	TaskID APITask.TaskID     `json:"task_id"`
	PlanID APITask.TaskID     `json:"plan_id,omitempty"`
	State  APITask.NodeState  `json:"state"`
	Output string             `json:"output,omitempty"`
	Owner  string             `json:"owner"`
}

// StreamChunkMsg carries a single token delta of the final answer across
// the mesh on the dedicated stream protocol. It is a data-plane message and
// SHALL NOT be published on the EventBus.
type StreamChunkMsg struct {
	PlanID APITask.TaskID `json:"plan_id"`
	TaskID APITask.TaskID `json:"task_id"`
	Delta  string         `json:"delta"`
	Done   bool           `json:"done"`
	Owner  string         `json:"owner"`
}

// TaskPlanResultMsg carries a terminal plan result from the component that
// detects completion back to the planner, which maps it to the originating
// request and hands it to the wrapper. Shared by the TaskPlanDone and
// TaskPlanCompleted events.
type TaskPlanResultMsg struct {
	PlanID APITask.TaskID     `json:"plan_id"`
	Result APITask.PlanResult `json:"result"`
}

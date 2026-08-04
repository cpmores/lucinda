// Package apitaskmsg defines wire message types for TaskBoard peer-to-peer communication.
package apitaskmsg

import (
	APICapability "github.com/cpmores/lucinda/api/v1/domain/capability"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
)

// TaskBroadcastMsg is published by the TaskBoard when a node becomes Ready.
// All peers receive it and evaluate whether to bid.
type TaskBroadcastMsg struct {
	Ad APITask.TaskAd `json:"ad"`
}

func TaskAdToTaskBroadcastMsg(ad *APITask.TaskAd) TaskBroadcastMsg {
	return TaskBroadcastMsg{
		Ad: *ad,
	}
}

// TaskRequestMsg is sent by a peer to submit a capability bid on an ad.
type TaskRequestMsg struct {
	CV APICapability.CapabilityCV `json:"cv"`
}

func TaskCVToTaskRequestMsg(cv *APICapability.CapabilityCV) TaskRequestMsg {
	return TaskRequestMsg{
		CV: *cv,
	}
}

// TaskAssignMsg is sent by the TaskBoard to award a task to the winning peer.
// Includes the prompt — the heavy payload — sent only to the winner, not broadcast.
type TaskAssignMsg struct {
	NodeID       APITask.TaskID   `json:"node_id"`
	OriginNodeID string           `json:"origin_node_id"` // node that owns the plan — send results here
	TTL          int64            `json:"ttl"`
	Prompt       string           `json:"prompt"`
	Spec         APITask.TaskSpec `json:"spec"`
}

func TaskToTaskAssignMsg(task *APITask.Task) TaskAssignMsg {
	return TaskAssignMsg{
		NodeID: task.Meta.ID,
		TTL:    task.Spec.Deadline,
		Prompt: task.Spec.Prompt,
		Spec:   task.Spec,
	}
}

// TaskResultMsg is sent by the executor back to the origin with the output.
type TaskResultMsg struct {
	NodeID APITask.TaskID `json:"node_id"`
	Output string         `json:"output"`
}

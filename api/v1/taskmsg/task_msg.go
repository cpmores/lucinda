// Package apitaskmsg defines wire message types for TaskBoard peer-to-peer communication.
package apitaskmsg

import (
	apicapability "github.com/cpmores/lucinda/api/v1/capability"
	apitask "github.com/cpmores/lucinda/api/v1/task"
)

// TaskBroadcastMsg is published by the TaskBoard when a node becomes Ready.
// All peers receive it and evaluate whether to bid.
type TaskBroadcastMsg struct {
	Ad apitask.TaskAd `json:"ad"`
}

func TaskAdToTaskBroadcastMsg(ad *apitask.TaskAd) TaskBroadcastMsg {
	return TaskBroadcastMsg{
		Ad: *ad,
	}
}

// TaskRequestMsg is sent by a peer to submit a capability bid on an ad.
type TaskRequestMsg struct {
	CV apicapability.CapabilityCV `json:"cv"`
}

func TaskCVToTaskRequestMsg(cv *apicapability.CapabilityCV) TaskRequestMsg {
	return TaskRequestMsg{
		CV: *cv,
	}
}

// TaskAssignMsg is sent by the TaskBoard to award a task to the winning peer.
// Includes the prompt — the heavy payload — sent only to the winner, not broadcast.
type TaskAssignMsg struct {
	NodeID apitask.TaskID   `json:"node_id"`
	TTL    int64            `json:"ttl"`
	Prompt string           `json:"prompt"`
	Spec   apitask.TaskSpec `json:"spec"`
}

func TaskToTaskAssignMsg(task *apitask.Task) TaskAssignMsg {
	return TaskAssignMsg{
		NodeID: task.Meta.ID,
		TTL:    task.Spec.Deadline,
		Prompt: task.Spec.Prompt,
		Spec:   task.Spec,
	}
}

// TaskResultMsg is sent by the executor back to the origin with the output.
type TaskResultMsg struct {
	NodeID apitask.TaskID `json:"node_id"`
	Output string         `json:"output"`
}

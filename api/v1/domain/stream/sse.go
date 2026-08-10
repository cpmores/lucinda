// Package apistream defines the Server-Sent Events contract for the task
// workflow egress: the ordered frames a client receives on /stream.
//
// Frame order mirrors causality: status frames (component state changes),
// step_result frames (completed sub-task outputs), stream frames (final
// answer token deltas), then exactly one done frame per plan.
package apistream

// SSEFrameType is the discriminator for a stream frame.
type SSEFrameType string

const (
	SSETypeStatus     SSEFrameType = "status"
	SSETypeStepResult SSEFrameType = "step_result"
	SSETypeStream     SSEFrameType = "stream"
	SSETypeDone       SSEFrameType = "done"
)

// SSEFrame is the envelope written to the wire. PlanID scopes every frame
// to its plan so concurrent sessions never interleave.
type SSEFrame struct {
	Event  SSEFrameType `json:"event"`
	PlanID string       `json:"plan_id"`
	Data   any          `json:"data"`
}

// StatusData reports a component state change, e.g. which agent is doing
// what right now. Owner + PlanID are the routing and demux keys: the
// telemetry bridge unicasts to Owner, and the monitor demuxes by PlanID.
type StatusData struct {
	Component string `json:"component"`
	State     string `json:"state"`
	Owner     string `json:"owner,omitempty"`
	PlanID    string `json:"plan_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	Model     string `json:"model,omitempty"`
}

// StepResultData reports the output of a completed sub-task.
type StepResultData struct {
	Owner  string `json:"owner,omitempty"`
	PlanID string `json:"plan_id"`
	TaskID string `json:"task_id"`
	Output string `json:"output"`
}

// StreamData is a single token delta of the final answer.
type StreamData struct {
	Delta string `json:"delta"`
	Done  bool   `json:"done"`
}

// DoneData terminates the stream. Status mirrors PlanStatus
// (ok / error / timeout / cancelled).
type DoneData struct {
	Status string `json:"status"`
	Text   string `json:"text"`
}

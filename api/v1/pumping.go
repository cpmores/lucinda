// lucinda/api/pumping.go
// pumping task under received from transport incomings
package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// task message sent by owner
type MessageType string
type NodePayload struct {
	MessageType MessageType
}

type PayloadStruct interface {
	GetType() MessageType
}

// payload structure ids
const (
	TASK_BROADCAST_MSG = "TaskBroadcastMsg"
)

type TaskBroadcastMsg struct {
	// 1. the only identify
	TaskID   TaskID `json:"task_id"`
	ParentID TaskID `json:"parent_id,omitempty"`
	Owner    UserID `json:"owner"`

	// 2. core labels, for filters
	RequiredLabels []CapabilityLabel `json:"required_labels"`
	RequiredTools  []ToolID          `json:"required_tools,omitempty"`

	// 3. resources estimate
	EstimatedVram int64 `json:"estimated_vram"`
	BudgetTokens  int   `json:"budget_tokens"` // token allowed
	Priority      int   `json:"priority"`      // high priority means dealing first

	// 4. time limit
	BuildTime time.Time `json:"build_time"`
	Deadline  time.Time `json:"deadline"` // requested time limit
}

func (msg *TaskBroadcastMsg) GetType() MessageType {
	return "TaskBroadcastMsg"
}

func JsonToTaskBroadcastMsg(j []byte) (PayloadStruct, error) {
	var taskBroadMsg TaskBroadcastMsg
	if err := json.Unmarshal(j, &taskBroadMsg); err != nil {
		return nil, fmt.Errorf("failed to transform json payload to TaskBroadcastMsg")
	}

	return &taskBroadMsg, nil
}

func TaskBroadcastMsgToJson(msg TaskBroadcastMsg) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to transform TaskBroadcastMsg to json payload")
	}

	return data, nil
}

// ======================== public json transform method ========================
// from json to payload structure
type JsonToPayloadStruct func(j []byte) (PayloadStruct, error)

// json transform factory
var PumpingFactory map[MessageType]JsonToPayloadStruct

func RegisterPayloadStruct(t MessageType, f JsonToPayloadStruct) {
	PumpingFactory[t] = f
}

func init() {
	RegisterPayloadStruct(TASK_BROADCAST_MSG, JsonToTaskBroadcastMsg)
}

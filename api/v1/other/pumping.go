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
	TASK_BROADCAST_MSG MessageType = "TaskBroadcastMsg"
	TASK_REQUEST_MSG   MessageType = "TaskRequestMsg"
	TASK_ASSIGN_MSG    MessageType = "TaskAssignMsg"
	TASK_ACCEPT_MSG    MessageType = "TaskAcceptMsg"
	TASK_RESULT_MSG    MessageType = "TaskResultMsg"
)

// ======================= TaskBroadcastMsg ===================================
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
	return TASK_BROADCAST_MSG
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

// ======================= TaskBroadcastMsg ===================================

// ======================= TaskRequestMsg =====================================
type TaskRequestMsg struct {
	// 1. the only identity
	TaskID   TaskID `json:"task_id"`
	ParentID TaskID `json:"parent_id,omitempty"`
	Owner    UserID `json:"owner"`

	// 2. NodeProviderStatus, need filtered
	NodeProviderStatus NodeProviderStatus `json:"node_provider_status"`

	// 3. requested time
	RequestedTime time.Time `json:"requested_time"`
}

func (msg *TaskRequestMsg) GetType() MessageType {
	return TASK_REQUEST_MSG
}

func JsonToTaskRequestMsg(j []byte) (PayloadStruct, error) {
	var taskReqMsg TaskRequestMsg
	if err := json.Unmarshal(j, &taskReqMsg); err != nil {
		return nil, fmt.Errorf("failed to transform json payload to TaskRequestMsg")
	}

	return &taskReqMsg, nil
}

func TaskRequestMsgToJson(msg TaskRequestMsg) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to transform TaskRequestMsg to json payload")
	}

	return data, nil
}

// ======================= TaskRequestMsg =====================================

// ======================= TaskAssignMsg ======================================
type TaskAssignMsg struct {
	TaskID   TaskID `json:"task_id"`
	ParentID TaskID `json:"parent_id,omitempty"`
	Owner    UserID `json:"owner"`

	AssignedTime time.Time `json:"assigned_time"`
	TaskImage    TaskImage `json:"task_image"`
}

func (msg *TaskAssignMsg) GetType() MessageType {
	return TASK_ASSIGN_MSG
}

func JsonToTaskAssignMsg(j []byte) (PayloadStruct, error) {
	var taskAssignMsg TaskAssignMsg
	if err := json.Unmarshal(j, &taskAssignMsg); err != nil {
		return nil, fmt.Errorf("failed to transform json payload to TaskAssignMsg")
	}

	return &taskAssignMsg, nil
}

func TaskAssignMsgToJson(msg TaskAssignMsg) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to transform TaskAssignMsg to json payload")
	}

	return data, nil
}

// ======================= TaskAssignMsg ======================================

// ======================= TaskAcceptMsg ======================================
type TaskAcceptMsg struct {
	TaskID   TaskID `json:"task_id"`
	ParentID TaskID `json:"parent_id,omitempty"`
	Owner    UserID `json:"owner"`

	AcceptedTime time.Time `json:"accepted_time"`
}

func (msg *TaskAcceptMsg) GetType() MessageType {
	return TASK_ACCEPT_MSG
}

func JsonToTaskAcceptMsg(j []byte) (PayloadStruct, error) {
	var taskAcceptMsg TaskAcceptMsg
	if err := json.Unmarshal(j, &taskAcceptMsg); err != nil {
		return nil, fmt.Errorf("failed to transform json payload to TaskAcceptMsg")
	}

	return &taskAcceptMsg, nil
}

func TaskAcceptMsgToJson(msg TaskAcceptMsg) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to transform TaskAcceptMsg to json payload")
	}

	return data, nil
}

// ======================= TaskAcceptMsg ======================================

// ======================= TaskResultMsg ======================================
type TaskResultMsg struct {
	TaskID   TaskID `json:"task_id"`
	ParentID TaskID `json:"parent_id,omitempty"`
	Owner    UserID `json:"owner"`

	FinishedTime time.Time `json:"finished_time"`
	Result       any       `json:"result"` // can be url, data or context
}

func (msg *TaskResultMsg) GetType() MessageType {
	return TASK_RESULT_MSG
}

func JsonToTaskResultMsg(j []byte) (PayloadStruct, error) {
	var taskResultMsg TaskResultMsg
	if err := json.Unmarshal(j, &taskResultMsg); err != nil {
		return nil, fmt.Errorf("failed to transform json payload to TaskResultMsg")
	}

	return &taskResultMsg, nil
}

func TaskResultMsgToJson(msg TaskResultMsg) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to transform TaskResultMsg to json payload")
	}

	return data, nil
}

// ======================= TaskResultMsg ======================================

// ======================== public json transform method ========================
// from json to payload structure
type JsonToPayloadStruct func(j []byte) (PayloadStruct, error)

// json transform factory
var PumpingFactory map[MessageType]JsonToPayloadStruct

func RegisterPayloadStruct(t MessageType, f JsonToPayloadStruct) {
	PumpingFactory[t] = f
}

func init() {
	PumpingFactory = make(map[MessageType]JsonToPayloadStruct)
	RegisterPayloadStruct(TASK_BROADCAST_MSG, JsonToTaskBroadcastMsg)
	RegisterPayloadStruct(TASK_REQUEST_MSG, JsonToTaskRequestMsg)
	RegisterPayloadStruct(TASK_ASSIGN_MSG, JsonToTaskAssignMsg)
	RegisterPayloadStruct(TASK_ACCEPT_MSG, JsonToTaskAcceptMsg)
	RegisterPayloadStruct(TASK_RESULT_MSG, JsonToTaskResultMsg)
}

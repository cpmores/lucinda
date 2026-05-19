package api

import (
	"time"
)

type TaskID string
type UserID string
type ToolID string

type TaskMeta struct {
	TaskID      TaskID            `json:"task_id"`
	IsSubTask   bool              `json:"is_sub_task"`
	Labels      []CapabilityLabel `json:"labels"`
	ParentID    TaskID            `json:"parent_id,omitempty"`
	DependOn    []TaskID          `json:"depend_on,omitempty"`
	DividedInto []TaskID          `json:"divided_into,omitempty"`
}

type TaskTime struct {
	// 1. requested time limit
	BuildTime time.Time `json:"build_time"`
	Deadline  time.Time `json:"deadline"`
}

type TaskRuntime struct {
	// 1, model state
	State         int    `json:"state"`
	RunOnNode     NodeID `json:"run_on_node"`
	RunOnProvider string `json:"run_on_provider"`
	RunOnModel    string `json:"run_on_model"`
	// 2. work time limit
	StartTime     time.Time `json:"start_time"`
	ForcastedTime time.Time `json:"forcasted_time"`
	LeftTime      time.Time `json:"left_time"`
}

type TaskContext struct {
	Goal        string         `json:"goal"` // task goal, aim to get
	Prompt      string         `json:"prompt"`
	Workspace   map[string]any `json:"workspace"`   //  can be context, url or data
	Constraints []string       `json:"constraints"` // cannot use web, word number, etc
}

type TaskResources struct {
	Tools []ToolID `json:"tools"`

	Priority     int    `json:"priority"`
	BudgetTokens int    `json:"budget_tokens"`
	Owner        UserID `json:"owner"`
}

type TaskQuality struct{}

type TaskImage struct {
	TaskID        TaskID        `json:"task_id"`
	IsSubTask     bool          `json:"is_sub_task"`
	ParentID      TaskID        `json:"parent_id,omitempty"`
	TaskRuntime   TaskRuntime   `json:"task_runtime"`
	TaskContext   TaskContext   `json:"task_context"`
	TaskResources TaskResources `json:"task_resources"`
}

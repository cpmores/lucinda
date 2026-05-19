package api

import "time"

type TaskID string
type UserID string
type ToolID string

type Task struct {
	TaskMeta
	TaskTime
	TaskRuntime
	TaskContext
	TaskResources
	TaskQuality
}

type TaskMeta struct {
	TaskID      TaskID
	IsSubTask   bool
	Labels      []CapabilityLabel
	DependOn    []TaskID
	DividedInto []TaskID
}

type TaskTime struct {
	// 1. requested time limit
	BuildTime time.Time
	Deadline  time.Time
	// 2, work time limit
	StartTime     time.Time
	ForcastedTime time.Time
	LeftTime      time.Time
}

type TaskRuntime struct {
	State         int
	RunOnNode     NodeID
	RunOnProvider string
	RunOnModel    string
}

type TaskContext struct {
	Goal        string // task goal, aim to get
	Prompt      string
	Workspace   map[string]any //  can be context, url or data
	Constraints []string       // cannot use web, word number, etc
}

type TaskResources struct {
	Tools        []ToolID
	Priority     int
	BudgetTokens int
	Owner        UserID
}

type TaskQuality struct{}

package apitask

// TaskID uniquely identifies a task or sub-task.
type TaskID string

type Task struct {
	Meta     TaskMeta
	Spec     TaskSpec
	TaskPlan *TaskPlan
	TaskNode *TaskNode
}

// TaskMeta carries identifiers and lifecycle type for a task.
type TaskMeta struct {
	ID    TaskID `json:"id"`
	Type  Stage  `json:"type"`
	Owner string `json:"owner"`
}

// Stage defines which phase of the Plan-Execute-Reduce pipeline a task belongs to.
type Stage string

const (
	StagePlan    Stage = "plan"
	StageExecute Stage = "execute"
	StageReduce  Stage = "reduce"
)

// internel/task/task.go
// task build on local
package task

import (
	"github.com/cpmores/lucinda/api/v1"
)

type Task struct {
	api.TaskMeta
	api.TaskTime
	api.TaskRuntime
	api.TaskContext
	api.TaskResources
	api.TaskQuality
}

func (t *Task) EstimateVram() int64 {
	// TODO
	return 0
}

func (t *Task) GeneratePublishMsg() (api.TaskBroadcastMsg, error) {
	taskBroadcastMsg := api.TaskBroadcastMsg{
		TaskID:   t.TaskID,
		ParentID: t.ParentID,
		Owner:    t.Owner,

		RequiredLabels: t.Labels,
		RequiredTools:  t.Tools,

		EstimatedVram: t.EstimateVram(),
		BudgetTokens:  t.BudgetTokens,
		Priority:      t.Priority,

		BuildTime: t.BuildTime,
		Deadline:  t.Deadline,
	}

	return taskBroadcastMsg, nil
}

func (t *Task) GenerateTaskImage() api.TaskImage {
	return api.TaskImage{
		TaskID:        t.TaskID,
		IsSubTask:     t.IsSubTask,
		ParentID:      t.ParentID,
		TaskRuntime:   t.TaskRuntime,
		TaskContext:   t.TaskContext,
		TaskResources: t.TaskResources,
	}
}

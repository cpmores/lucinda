package component

import (
	"context"

	"github.com/cpmores/lucinda/api/v1"
	policy "github.com/cpmores/lucinda/internel/policy/taskController/controller"
	"github.com/cpmores/lucinda/internel/task"
)

type Divider struct {
	policy policy.TaskDividerPolicy
}

func (d *Divider) Divide(ctx context.Context, taskID api.TaskID, policy policy.TaskDividerPolicy) (task.TaskPlan, error) {
	return task.TaskPlan{}, nil
}

func NewTaskDivider(policy policy.TaskDividerPolicy) *Divider {
	return &Divider{
		policy: policy,
	}
}

package component

import (
	"context"

	"github.com/cpmores/lucinda/api/v1"
	policy "github.com/cpmores/lucinda/internel/policy/taskController/controller"
	"github.com/cpmores/lucinda/internel/task"
)

type Wrapper struct {
	policy policy.TaskWrapperPolicy
}

func NewTaskWrapper(policy policy.TaskWrapperPolicy) *Wrapper {
	return &Wrapper{
		policy: policy,
	}
}

func (w *Wrapper) Wrap(ctx context.Context, taskPreSubmit task.TaskPreSubmit, policy policy.TaskWrapperPolicy) (api.TaskID, error) {
	return w.policy.Wrap(ctx, taskPreSubmit)
}

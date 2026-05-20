package taskreducer

import (
	"context"

	"github.com/cpmores/lucinda/api/v1"
	policy "github.com/cpmores/lucinda/internel/policy/taskReducer/reducer"
)

type TaskReducer interface {
	Reduce(ctx context.Context, taskID api.TaskID, policy policy.TaskReducerPolicy) (api.ChatResponse, error)
}

type Reducer struct {
	policy policy.TaskReducerPolicy
}

func NewTaskReducer(policy policy.TaskReducerPolicy) *Reducer {
	return &Reducer{
		policy: policy,
	}
}

func (r *Reducer) Reduce(ctx context.Context, taskID api.TaskID, policy policy.TaskReducerPolicy) (api.ChatResponse, error) {
	return r.policy.Reduce(ctx, taskID)
}

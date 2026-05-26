package component

import (
	"context"

	policy "github.com/cpmores/lucinda/internel/policy/taskController/controller"
	"github.com/cpmores/lucinda/internel/task"
)

type Board struct {
	policy policy.TaskBoardPolicy
}

func NewTaskBoard(policy policy.TaskBoardPolicy) *Board {
	return &Board{
		policy: policy,
	}
}

func (b *Board) Publish(ctx context.Context, plan task.TaskPlan, policy policy.TaskBoardPolicy) error {
	return nil
}

func (b *Board) Interview(ctx context.Context, policy policy.TaskBoardPolicy) error {
	return nil
}

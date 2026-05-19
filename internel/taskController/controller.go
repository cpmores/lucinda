package taskcontroller

import (
	"context"

	"github.com/cpmores/lucinda/api/v1"
	"github.com/cpmores/lucinda/internel/task"
)

type TaskController interface {
	TaskWrapper
	TaskDivider
	TaskBoard
	TaskReducer
}

type TaskWrapper interface {
	Wrap(ctx context.Context, chat api.ChatRequest) (api.TaskID, error)
}

type TaskDivider interface {
	Divide(ctx context.Context, taskID api.TaskID) (task.TaskPlan, error)
}

type TaskBoard interface {
	Publish(ctx context.Context, plan task.TaskPlan) error
	Interview(ctx context.Context) error
}

type TaskReducer interface {
	Reduce(ctx context.Context, taskID api.TaskID) (api.ChatResponse, error)
}

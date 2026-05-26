package policy

import (
	"context"

	"github.com/cpmores/lucinda/api/v1"
	"github.com/cpmores/lucinda/internel/task"
	"github.com/spf13/viper"
)

type TaskControllerPolicy struct {
	TaskWrapperPolicy TaskWrapperPolicy
	TaskDividerPolicy TaskDividerPolicy
	TaskBoardPolicy   TaskBoardPolicy
}

var defaultTaskControllerPolicy = TaskControllerPolicy{
	TaskWrapperPolicy: &DefaultTaskWrapperPolicy{},
	TaskDividerPolicy: &DefaultTaskDividerPolicy{},
	TaskBoardPolicy:   &DefaultTaskBoardPolicy{},
}

func NewTaskControllerPolicy(config *viper.Viper) TaskControllerPolicy {
	// TODO: parse policy from config
	_ = config.GetStringMap("task_controller.policy")
	return defaultTaskControllerPolicy
}

func GetDefaultTaskControllerPolicy() TaskControllerPolicy {
	return defaultTaskControllerPolicy
}

type TaskWrapperPolicy interface {
	Wrap(ctx context.Context, task task.TaskPreSubmit) (api.TaskID, error)
}

type TaskDividerPolicy interface {
	Divide(ctx context.Context, taskID api.TaskID) (task.TaskPlan, error)
}

type TaskBoardPolicy interface {
	Publish(ctx context.Context, plan task.TaskPlan) error
	Interview(ctx context.Context) error
}

type DefaultTaskWrapperPolicy struct {
}

func (p *DefaultTaskWrapperPolicy) Wrap(ctx context.Context, task task.TaskPreSubmit) (api.TaskID, error) {
	return "", nil
}

type DefaultTaskDividerPolicy struct {
}

func (p *DefaultTaskDividerPolicy) Divide(ctx context.Context, taskID api.TaskID) (task.TaskPlan, error) {
	return task.TaskPlan{}, nil
}

type DefaultTaskBoardPolicy struct {
}

func (p *DefaultTaskBoardPolicy) Publish(ctx context.Context, plan task.TaskPlan) error {
	return nil
}

func (p *DefaultTaskBoardPolicy) Interview(ctx context.Context) error {
	return nil
}

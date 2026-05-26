package taskcontroller

import (
	"context"
	"fmt"
	"log"

	"github.com/cpmores/lucinda/api/v1"
	eventbus "github.com/cpmores/lucinda/internel/eventBus"
	policy "github.com/cpmores/lucinda/internel/policy/taskController/controller"
	"github.com/cpmores/lucinda/internel/task"
	"github.com/cpmores/lucinda/internel/taskController/component"
	"github.com/spf13/viper"
)

// ======================== Task Components Interfacs ==============================
type TaskController interface {
	// pipeline is a task processing flow
	StartPipeline(ctx context.Context) error
	TaskWorker
}

type TaskWrapper interface {
	Wrap(ctx context.Context, taskPreSubmit task.TaskPreSubmit, policy policy.TaskWrapperPolicy) (api.TaskID, error)
}

type TaskDivider interface {
	Divide(ctx context.Context, taskID api.TaskID, policy policy.TaskDividerPolicy) (task.TaskPlan, error)
}

type TaskBoard interface {
	Publish(ctx context.Context, plan task.TaskPlan, policy policy.TaskBoardPolicy) error
	Interview(ctx context.Context, policy policy.TaskBoardPolicy) error
}

type TaskWorker interface {
	startWrapper(ctx context.Context) error
	startDivider(ctx context.Context) error
	startPublisher(ctx context.Context) error
	startInterviewer(ctx context.Context) error
}

// ======================== Task Components Interfacs ==============================

// ======================== Task Controller Implementation =========================
type Controller struct {
	Policy   policy.TaskControllerPolicy
	EventBus eventbus.EventBus

	Wrapper TaskWrapper
	Divider TaskDivider
	Board   TaskBoard
}

func (c *Controller) StartPipeline(ctx context.Context) error {
	if err := c.startWrapper(ctx); err != nil {
		return fmt.Errorf("start wrapper: %w", err)
	}

	if err := c.startDivider(ctx); err != nil {
		return fmt.Errorf("start divider: %w", err)
	}

	if err := c.startPublisher(ctx); err != nil {
		return fmt.Errorf("start publisher: %w", err)
	}

	if err := c.startInterviewer(ctx); err != nil {
		return fmt.Errorf("start interviewer: %w", err)
	}

	return nil
}

func (c *Controller) startWrapper(ctx context.Context) error {
	submitEventChan, err := c.EventBus.Subscribe(api.TASK_SUBMITTED)
	if err != nil {
		return fmt.Errorf("subscribe event: %w", err)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-submitEventChan:
				if taskPreSubmit, ok := event.Data.(task.TaskPreSubmit); ok {
					taskID, err := c.Wrapper.Wrap(ctx, taskPreSubmit, c.Policy.TaskWrapperPolicy)
					if err != nil {
						log.Printf("wrap task error: %v\n", err)
						continue
					}
					log.Printf("task wrapped with ID: %s\n", taskID)
				}
			}
		}
	}()
	return nil
}

func (c *Controller) startDivider(ctx context.Context) error {
	return nil
}

func (c *Controller) startPublisher(ctx context.Context) error {
	return nil
}

func (c *Controller) startInterviewer(ctx context.Context) error {
	return nil
}

func NewTaskController(policy policy.TaskControllerPolicy, eventbus eventbus.EventBus) *Controller {
	controller := &Controller{
		Policy:   policy,
		EventBus: eventbus,
	}

	controller.Wrapper = component.NewTaskWrapper(policy.TaskWrapperPolicy)
	controller.Divider = component.NewTaskDivider(policy.TaskDividerPolicy)
	controller.Board = component.NewTaskBoard(policy.TaskBoardPolicy)

	return controller
}

func StartTaskController(ctx context.Context, eventBus eventbus.EventBus, config *viper.Viper) error {
	policy := policy.NewTaskControllerPolicy(config)
	controller := NewTaskController(policy, eventBus)
	return controller.StartPipeline(ctx)
}

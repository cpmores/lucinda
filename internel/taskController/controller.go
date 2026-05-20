package taskcontroller

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/cpmores/lucinda/api/v1"
	eventbus "github.com/cpmores/lucinda/internel/eventBus"
	policy "github.com/cpmores/lucinda/internel/policy/taskController/controller"
	"github.com/cpmores/lucinda/internel/task"
	"github.com/cpmores/lucinda/internel/taskController/component"
)

// ======================== Task Components Interfacs ==============================
type TaskController interface {
	// pipeline is a task processing flow
	Submit(ctx context.Context, chat api.ChatRequest) error
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

func (c *Controller) Submit(ctx context.Context, chat api.ChatRequest) (api.TaskID, error) {
	// TODO: GENERATE UNIQUE TASK ID
	taskID := api.TaskID("test")
	event := task.GenerateTaskPreSumbitEvent(taskID, chat)
	return taskID, c.EventBus.Publish(api.TASK_SUBMITTED, event)
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

var (
	globalTaskController *Controller
	globalOnce           sync.Once
)

func GetGlobalTaskController() (*Controller, error) {
	var err error
	globalOnce.Do(func() {
		globalTaskController = NewTaskController(policy.GetDefaultTaskControllerPolicy(), eventbus.GetGlobalEventBus())
		if err = globalTaskController.StartPipeline(context.Background()); err != nil {
			err = fmt.Errorf("start task pipeline: %w", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return globalTaskController, nil
}

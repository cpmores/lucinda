package taskcontroller

import (
	"context"

	"github.com/cpmores/lucinda/api/v1"
	eventbus "github.com/cpmores/lucinda/internel/eventBus"
	policy "github.com/cpmores/lucinda/internel/policy/taskController/controller"
	"github.com/cpmores/lucinda/internel/task"
	"github.com/cpmores/lucinda/internel/taskController/component"
)

// ======================== Task Components Interfacs ==============================
type TaskController interface {
	// pipeline is a task processing flow
	PushRequest(ctx context.Context, chat api.ChatRequest) error
	StartPipeline(ctx context.Context) error
	TaskWorker
	TaskWrapper
	TaskDivider
	TaskBoard
}

type TaskWrapper interface {
	Wrap(ctx context.Context, chat api.ChatRequest, policy policy.TaskWrapperPolicy) (api.TaskID, error)
}

type TaskDivider interface {
	Divide(ctx context.Context, taskID api.TaskID, policy policy.TaskDividerPolicy) (task.TaskPlan, error)
}

type TaskBoard interface {
	Publish(ctx context.Context, plan task.TaskPlan, policy policy.TaskBoardPolicy) error
	Interview(ctx context.Context, policy policy.TaskBoardPolicy) error
}

type TaskWorker interface {
	StartWrapper(ctx context.Context) error
	StartDivider(ctx context.Context) error
	StartPublisher(ctx context.Context) error
	StartInterviewer(ctx context.Context) error
}

// ======================== Task Components Interfacs ==============================

// ======================== Task Controller Implementation =========================
type Controller struct {
	Policy   policy.TaskControllerPolicy
	EventBus *eventbus.EventBus

	worker  TaskWorker
	wrapper TaskWrapper
	divider TaskDivider
	board   TaskBoard
}

func NewTaskController(policy policy.TaskControllerPolicy, eventbus *eventbus.EventBus) *Controller {
	controller := &Controller{
		Policy:   policy,
		EventBus: eventbus,
	}

	controller.worker = component.NewTaskWorker()
	controller.wrapper = component.NewTaskWrapper(policy.TaskWrapperPolicy)
	controller.divider = component.NewTaskDivider(policy.TaskDividerPolicy)
	controller.board = component.NewTaskBoard(policy.TaskBoardPolicy)
	return controller
}

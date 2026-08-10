// Package taskwrapper is the external utility that adapts a user request
// into a raw task for the workflow layer. It lives outside
// task_workflow_layer: it does not know about planning or execution, only
// how to pack a prompt into a Task with its terminal Notify channel and
// publish it for planning.
package taskwrapper

import (
	"fmt"
	"sync"
	"time"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_monitor"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
)

// TaskWrapper adapts ChatRequests into raw tasks.
type TaskWrapper struct {
	eb      eventbus.EventBus
	monitor taskmonitor.TaskMonitor
	localID string

	mu       sync.Mutex
	notifies map[APITask.TaskID]<-chan APITask.PlanResult
}

// New creates a TaskWrapper. localID is the owner node's own ID.
func New(eb eventbus.EventBus, monitor taskmonitor.TaskMonitor, localID string) *TaskWrapper {
	return &TaskWrapper{
		eb:       eb,
		monitor:  monitor,
		localID:  localID,
		notifies: make(map[APITask.TaskID]<-chan APITask.PlanResult),
	}
}

// Wrap packs a prompt into a raw task (no resource constraints — the
// planner decides those) and publishes TaskPreplanned. It returns the plan
// ID and the terminal result channel the SSE handler waits on. arch selects
// the execution architecture; NormalizeAgent validates it.
func (w *TaskWrapper) Wrap(prompt string, arch APITask.AgentArch) (APITask.TaskID, <-chan APITask.PlanResult, error) {
	planID := APITask.TaskID(fmt.Sprintf("plan-%d", time.Now().UnixNano()))
	notify := make(chan APITask.PlanResult, 1)

	task := &APITask.Task{
		Meta: APITask.TaskMeta{
			ID:    planID,
			Type:  APITask.StagePlan,
			Owner: w.localID,
		},
		Spec: APITask.TaskSpec{Prompt: prompt},
		TaskPlan: &APITask.TaskPlan{
			ID:           planID,
			Owner:        w.localID,
			Notify:       notify,
			Architecture: NormalizeAgent(arch),
		},
	}

	// Register before publishing so the monitor never races the planner's
	// first telemetry event when a client opens /stream right after /chat.
	w.monitor.Register(string(planID))
	w.mu.Lock()
	w.notifies[planID] = notify
	w.mu.Unlock()

	_ = w.eb.Publish(APIEvent.TaskPreplanned, APIEvent.NewEvent(APIEvent.TaskPreplanned, task))
	return planID, notify, nil
}

// Notify returns the terminal result channel for a plan, if it exists.
func (w *TaskWrapper) Notify(planID APITask.TaskID) (<-chan APITask.PlanResult, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ch, ok := w.notifies[planID]
	return ch, ok
}

// NormalizeAgent validates a request's agent string, defaulting to
// plan_execute for any unknown or empty value.
func NormalizeAgent(agent APITask.AgentArch) APITask.AgentArch {
	if agent == APITask.ArchReAct {
		return APITask.ArchReAct
	}
	return APITask.ArchPlanExecute
}

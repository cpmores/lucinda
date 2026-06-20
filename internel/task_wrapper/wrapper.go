// Package taskwrapper wraps incoming ChatRequests into Tasks and publishes
// them for planning. It's called directly by the HTTP handler, not via EventBus.
package taskwrapper

import (
	"fmt"
	"time"

	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	APITask "github.com/cpmores/lucinda/api/v1/task"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
)

type TaskWrapper struct {
	eb eventbus.EventBus
}

func New(eb eventbus.EventBus) *TaskWrapper {
	return &TaskWrapper{eb: eb}
}

// Wrap creates a Task from a user prompt and publishes TaskPreplaned.
// Returns the tracking ID so the caller can open an SSE stream.
func (w *TaskWrapper) Wrap(prompt string, owner string, notify chan<- string) (APITask.TaskID, error) {
	planID := APITask.TaskID(fmt.Sprintf("plan-%d", time.Now().UnixNano()))

	task := &APITask.Task{
		Meta: APITask.TaskMeta{
			ID:    planID,
			Type:  APITask.StagePlan,
			Owner: owner,
		},
		Spec: APITask.TaskSpec{
			Prompt: prompt,
		},
	}

	// Attach the notify channel so results flow back to the SSE handler.
	// This gets copied into the TaskPlan by the Planner.
	task.TaskPlan = &APITask.TaskPlan{
		ID:     planID,
		Owner:  owner,
		Notify: notify,
	}

	w.eb.Publish(APIEvent.TaskPreplaned, APIEvent.NewEvent(APIEvent.TaskPreplaned, task))
	return planID, nil
}

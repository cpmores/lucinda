// Package tasktracer tracks local and assigned tasks for observability.
package tasktracer

import (
	"fmt"
	"sync"

	APIModule "github.com/cpmores/lucinda/api/v1/module"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	APITask "github.com/cpmores/lucinda/api/v1/task"
)

type TaskTracer interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	// Mutations — called by TaskPostman.
	Import(task *APITask.Task) error
	Assigned(task *APITask.Task) error
	Remove(id APITask.TaskID) error
	SetOutput(id APITask.TaskID, output string) error

	// Queries.
	GetLocal(id APITask.TaskID) (*APITask.Task, error)
	GetAssigned(id APITask.TaskID) (*APITask.Task, error)
	ListLocal() []*APITask.Task
	ListAssigned() []*APITask.Task
}

type tracer struct {
	mu       sync.RWMutex
	local    map[APITask.TaskID]*APITask.Task
	assigned map[APITask.TaskID]*APITask.Task
}

func NewTaskTracer() TaskTracer {
	return &tracer{
		local:    make(map[APITask.TaskID]*APITask.Task),
		assigned: make(map[APITask.TaskID]*APITask.Task),
	}
}

func (t *tracer) Import(task *APITask.Task) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.local[task.Meta.ID]; ok {
		return fmt.Errorf("task %s already in local storage", task.Meta.ID)
	}
	t.local[task.Meta.ID] = task
	return nil
}

func (t *tracer) Assigned(task *APITask.Task) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.assigned[task.Meta.ID]; ok {
		return fmt.Errorf("task %s already in assigned storage", task.Meta.ID)
	}
	t.assigned[task.Meta.ID] = task
	return nil
}

func (t *tracer) SetOutput(id APITask.TaskID, output string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if task, ok := t.local[id]; ok {
		task.Spec.Output = output
		return nil
	}
	if task, ok := t.assigned[id]; ok {
		task.Spec.Output = output
		return nil
	}
	return fmt.Errorf("task %s not found", id)
}

func (t *tracer) Remove(id APITask.TaskID) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.local[id]; ok {
		delete(t.local, id)
		return nil
	}
	if _, ok := t.assigned[id]; ok {
		delete(t.assigned, id)
		return nil
	}
	return fmt.Errorf("task %s not found", id)
}

func (t *tracer) GetLocal(id APITask.TaskID) (*APITask.Task, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	task, ok := t.local[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found in local storage", id)
	}
	return task, nil
}

func (t *tracer) GetAssigned(id APITask.TaskID) (*APITask.Task, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	task, ok := t.assigned[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found in assigned storage", id)
	}
	return task, nil
}

func (t *tracer) ListLocal() []*APITask.Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*APITask.Task, 0, len(t.local))
	for _, task := range t.local {
		result = append(result, task)
	}
	return result
}

func (t *tracer) ListAssigned() []*APITask.Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*APITask.Task, 0, len(t.assigned))
	for _, task := range t.assigned {
		result = append(result, task)
	}
	return result
}

func (t *tracer) GetModuleType() APIModule.ModuleType { return APIModule.TASKTRACER }
func (t *tracer) GetModuleID() APIModule.ModuleID { return APIModule.NewModuleID(t.GetModuleType(), "default") }
func (t *tracer) CheckHealth() APIModule.ModuleHealth { return APIModule.NewModuleHealth(t.GetModuleID(), t.GetModuleType(), APIModule.RUNNING) }
func (t *tracer) RegisterWithManager(m modulemanager.ModuleManager) error { return m.Register(t) }

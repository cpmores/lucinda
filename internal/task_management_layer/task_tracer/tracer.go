// Package tasktracer tracks task life-cycles for observability. It keeps two
// registries — tasks I own locally and tasks assigned to me — and emits a
// TaskTraced event on every state change, so the commander (or any observer)
// judges progress from live updates instead of polling.
package tasktracer

import (
	"fmt"
	"sync"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	eventbus "github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	logger "github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// TaskTracer is the interface for the tracer module.
type TaskTracer interface {
	RegisterWithManager(m modulemanager.ModuleManager) error

	// Import registers a task I created locally (ready for the board).
	Import(task *APITask.Task) error
	// Assigned registers a task assigned to me for execution.
	Assigned(task *APITask.Task) error
	// Update changes a task's lifecycle state and emits TaskTraced.
	Update(id APITask.TaskID, state APITask.NodeState) error
	// SetOutput records a task's output without emitting a state change.
	SetOutput(id APITask.TaskID, output string) error
	// Remove drops a task from both registries.
	Remove(id APITask.TaskID) error

	// Queries.
	Get(id APITask.TaskID) (*APITask.Task, bool)
	ListLocal() []*APITask.Task
	ListAssigned() []*APITask.Task
}

type tracer struct {
	mm  modulemanager.ModuleManager
	eb  eventbus.EventBus
	log *logger.Logger

	mu       sync.RWMutex
	local    map[APITask.TaskID]*APITask.Task
	assigned map[APITask.TaskID]*APITask.Task
}

// NewTaskTracer creates a tracer. Deps (EventBus) are resolved via
// DependsEnable; the module manager is captured at RegisterWithManager.
func NewTaskTracer(log *logger.Logger) TaskTracer {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &tracer{
		log:      log,
		local:    make(map[APITask.TaskID]*APITask.Task),
		assigned: make(map[APITask.TaskID]*APITask.Task),
	}
}

// ── Mutations ──────────────────────────────────────────────────────────

func (t *tracer) Import(task *APITask.Task) error {
	t.mu.Lock()
	if _, ok := t.local[task.Meta.ID]; ok {
		t.mu.Unlock()
		return fmt.Errorf("task %s already in local storage", task.Meta.ID)
	}
	if task.TaskNode == nil {
		task.TaskNode = &APITask.TaskNode{ID: task.Meta.ID}
	}
	task.TaskNode.State = APITask.StateReady
	t.local[task.Meta.ID] = task
	t.mu.Unlock()
	t.emit(task)
	return nil
}

func (t *tracer) Assigned(task *APITask.Task) error {
	t.mu.Lock()
	if _, ok := t.assigned[task.Meta.ID]; ok {
		t.mu.Unlock()
		return fmt.Errorf("task %s already in assigned storage", task.Meta.ID)
	}
	if task.TaskNode == nil {
		task.TaskNode = &APITask.TaskNode{ID: task.Meta.ID}
	}
	task.TaskNode.State = APITask.StateRunning
	t.assigned[task.Meta.ID] = task
	t.mu.Unlock()
	t.emit(task)
	return nil
}

func (t *tracer) Update(id APITask.TaskID, state APITask.NodeState) error {
	t.mu.Lock()
	task := t.lookup(id)
	if task == nil {
		t.mu.Unlock()
		return fmt.Errorf("task %s not found", id)
	}
	if task.TaskNode == nil {
		task.TaskNode = &APITask.TaskNode{ID: id}
	}
	task.TaskNode.State = state
	t.mu.Unlock()
	t.emit(task)
	return nil
}

func (t *tracer) SetOutput(id APITask.TaskID, output string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	task := t.lookup(id)
	if task == nil {
		return fmt.Errorf("task %s not found", id)
	}
	task.Spec.Output = output
	return nil
}

func (t *tracer) Remove(id APITask.TaskID) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.local, id)
	delete(t.assigned, id)
	return nil
}

// ── Queries ────────────────────────────────────────────────────────────

func (t *tracer) Get(id APITask.TaskID) (*APITask.Task, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if task := t.lookup(id); task != nil {
		return task, true
	}
	return nil, false
}

func (t *tracer) ListLocal() []*APITask.Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return collect(t.local)
}

func (t *tracer) ListAssigned() []*APITask.Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return collect(t.assigned)
}

// lookup returns a task from either registry. Caller must hold the lock.
func (t *tracer) lookup(id APITask.TaskID) *APITask.Task {
	if task, ok := t.local[id]; ok {
		return task
	}
	return t.assigned[id]
}

// emit publishes a lightweight lifecycle update so observers see the current
// state. The Owner field is the routing key a remote assigner uses to
// receive the update.
func (t *tracer) emit(task *APITask.Task) {
	var planID APITask.TaskID
	if task.TaskPlan != nil {
		planID = task.TaskPlan.ID
	}
	msg := APITaskmsg.TaskTracedMsg{
		TaskID: task.Meta.ID,
		PlanID: planID,
		State:  task.TaskNode.State,
		Output: task.Spec.Output,
		Owner:  task.Meta.Owner,
	}
	_ = t.eb.Publish(APIEvent.TaskTraced, APIEvent.NewEvent(APIEvent.TaskTraced, msg))
}

func collect(m map[APITask.TaskID]*APITask.Task) []*APITask.Task {
	out := make([]*APITask.Task, 0, len(m))
	for _, task := range m {
		out = append(out, task)
	}
	return out
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (t *tracer) GetModuleType() APIModule.ModuleType { return APIModule.TaskTracer }
func (t *tracer) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(t.GetModuleType(), "default")
}
func (t *tracer) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(t.GetModuleID(), t.GetModuleType(), APIModule.Running)
}
func (t *tracer) RegisterWithManager(m modulemanager.ModuleManager) error {
	t.mm = m
	return m.Register(t)
}
func (t *tracer) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{APIModule.EventBus: "default"}
}
func (t *tracer) DependsEnable() error {
	id := APIModule.NewModuleID(APIModule.EventBus, "default")
	mod, err := t.mm.Get(id)
	if err != nil {
		return fmt.Errorf("resolve dependency %s: %w", id, err)
	}
	eb, ok := mod.(eventbus.EventBus)
	if !ok {
		return fmt.Errorf("dependency %s is not an EventBus", id)
	}
	t.eb = eb
	return nil
}

// Package taskstatemanager provides the TaskStateManager struct
// and its associated methods for managing the state of tasks in a task management system.
// It allows for tracking the status of tasks, updating their states,
// and retrieving information about their current status.
package taskstatemanager

import (
	"context"
	"fmt"
	"sync"
	"time"

	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	APIModule "github.com/cpmores/lucinda/api/v1/module"
	APITask "github.com/cpmores/lucinda/api/v1/task"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

var StateCheckSec int64 = 1

type TaskStateManager interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	// ── Ingest ──────────────────────────────────────────────────────────
	// Ingest takes a TaskPlan and initializes its state in the TaskStateManager.
	Ingest(plan *APITask.TaskPlan) error

	// ── Lifecycle ──────────────────────────────────────────────────────────
	// Claim allows a peer to claim a ready task node for execution, setting a lease duration.
	Claim(ctx context.Context, taskID APITask.TaskID, peerID string, leaseDuration int64) error

	// Start marks a claimed task node as running, confirming that the peer has started execution.
	Start(taskID APITask.TaskID) error

	// Complete marks a running task node as done, indicating successful completion.
	// output is the execution result. When the last node in a plan completes
	// and plan.Notify is set, output is sent to that channel.
	Complete(taskID APITask.TaskID, output string) error

	// Failed marks a running node as failed, allowing for retries or error handling.
	Failed(taskID APITask.TaskID) error

	// Abandon allows a peer to give up a claim before starting, reverting the node
	// to Ready so another peer can claim it.
	Abandon(taskID APITask.TaskID) error

	// Dispose marks every non-done node in the plan as disposed, cancelling
	// the entire plan. Running/claimed nodes are abandoned; pending nodes skipped.
	Dispose(planID APITask.TaskID) error

	// Expired returns nodes whose claim lease has lapsed.
	// NOTE: caller publishes them back to the TaskBoard.
	Expired() []APITask.TaskNode

	// ── Query ──────────────────────────────────────────────────────────
	Plan(APITask.TaskID) (APITask.TaskPlan, error)
	Status(APITask.TaskID) (map[APITask.TaskID]APITask.NodeState, error)
	IsComplete(APITask.TaskID) (bool, error)
}

// ── Implementation ──────────────────────────────────────────────────────────

type manager struct {
	sync.RWMutex
	eb      eventbus.EventBus
	plans   map[APITask.TaskID]*APITask.TaskPlan
	nodes   map[APITask.TaskID]*APITask.TaskNode
	claimed map[APITask.TaskID]string // taskID → peerID
}

func NewTaskStateManager(eventBus eventbus.EventBus) TaskStateManager {
	return &manager{
		eb:      eventBus,
		plans:   make(map[APITask.TaskID]*APITask.TaskPlan),
		nodes:   make(map[APITask.TaskID]*APITask.TaskNode),
		claimed: make(map[APITask.TaskID]string),
	}
}

// ── Ingest ────────────────────────────────────────────────────────────

func (m *manager) Ingest(plan *APITask.TaskPlan) error {
	m.Lock()

	if m.plans[plan.ID] != nil {
		m.Unlock()
		return fmt.Errorf("task plan with ID %s already exists", plan.ID)
	}

	m.plans[plan.ID] = plan
	for _, node := range plan.Nodes {
		m.nodes[node.ID] = node
		node.State = APITask.StatePending
	}

	// Collect roots and set Ready under lock.
	var roots []*APITask.TaskNode
	for _, rootID := range plan.Roots {
		node := plan.Nodes[rootID]
		if node == nil {
			continue
		}
		node.State = APITask.StateReady
		roots = append(roots, node)
	}
	m.Unlock()

	// Start the deadline goroutine before publishing roots so the
	// timer is already running when nodes begin executing.
	m.startDeadlineWatch(plan)

	// Publish outside the lock — subscribers may call back into
	// the StateManager (e.g. Claim), which needs the lock.
	for _, node := range roots {
		m.publish(APIEvent.TaskReady, node)
	}

	return nil
}

// ── Claim ─────────────────────────────────────────────────────────────

func (m *manager) Claim(ctx context.Context, taskID APITask.TaskID, peerID string, leaseDuration int64) error {
	m.Lock()

	node := m.nodes[taskID]
	if node == nil {
		m.Unlock()
		return fmt.Errorf("task node %s does not exist", taskID)
	}
	if node.State == APITask.StateClaimed {
		m.Unlock()
		return fmt.Errorf("task node %s is already claimed by %s", taskID, m.claimed[taskID])
	}
	if node.State != APITask.StateReady {
		m.Unlock()
		return fmt.Errorf("task node %s is %s, cannot claim", taskID, node.State)
	}

	node.State = APITask.StateClaimed
	node.ClaimedBy = peerID
	node.ExpiresAt = time.Now().Unix() + leaseDuration
	m.claimed[taskID] = peerID
	m.Unlock()

	// Lease watchdog: if the peer never starts, reopen the node.
	go func() {
		timer := time.NewTimer(time.Duration(leaseDuration) * time.Second)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			m.Lock()
			n := m.nodes[taskID]
			if n != nil && n.State == APITask.StateClaimed {
				n.State = APITask.StateReady
				n.ClaimedBy = ""
				n.ExpiresAt = 0
				delete(m.claimed, taskID)
				m.publish(APIEvent.TaskRepublished, n)
			}
			m.Unlock()
		}
	}()

	return nil
}

// ── Start ─────────────────────────────────────────────────────────────

func (m *manager) Start(taskID APITask.TaskID) error {
	m.Lock()
	defer m.Unlock()

	node := m.nodes[taskID]
	if node == nil {
		return fmt.Errorf("task node %s does not exist", taskID)
	}
	if node.State == APITask.StateRunning {
		return nil
	}
	if node.State != APITask.StateClaimed {
		return fmt.Errorf("task node %s is %s, cannot start", taskID, node.State)
	}

	node.State = APITask.StateRunning
	return nil
}

// ── Complete ──────────────────────────────────────────────────────────

func (m *manager) Complete(taskID APITask.TaskID, output string) error {
	m.Lock()
	defer m.Unlock()

	node := m.nodes[taskID]
	if node == nil {
		return fmt.Errorf("task node %s does not exist", taskID)
	}
	if node.State == APITask.StateDone {
		return nil
	}
	if node.State != APITask.StateRunning {
		return fmt.Errorf("task node %s is %s, cannot complete", taskID, node.State)
	}

	node.State = APITask.StateDone
	delete(m.claimed, taskID)

	// Find the plan this node belongs to and cascade to successors.
	for _, plan := range m.plans {
		if _, ok := plan.Nodes[taskID]; !ok {
			continue
		}
		for _, succID := range plan.Successors[taskID] {
			succ := plan.Nodes[succID]
			if succ == nil {
				continue
			}
			plan.PredecessorNums[succID]--
			if plan.PredecessorNums[succID] <= 0 {
				succ.State = APITask.StateReady
				m.publish(APIEvent.TaskReady, succ)
			}
		}

		if allDone(plan) && plan.Notify != nil {
			plan.Notify <- APITask.PlanResult{Status: APITask.PlanOK, Text: output}
		}
		break
	}

	return nil
}

// ── Failed ────────────────────────────────────────────────────────────

func (m *manager) Failed(taskID APITask.TaskID) error {
	m.Lock()
	defer m.Unlock()

	node := m.nodes[taskID]
	if node == nil {
		return fmt.Errorf("task node %s does not exist", taskID)
	}

	node.State = APITask.StateFailed
	node.ClaimedBy = ""
	node.ExpiresAt = 0
	delete(m.claimed, taskID)
	m.publish(APIEvent.TaskRepublished, node)
	return nil
}

// ── Abandon ───────────────────────────────────────────────────────────

func (m *manager) Abandon(taskID APITask.TaskID) error {
	m.Lock()
	defer m.Unlock()

	node := m.nodes[taskID]
	if node == nil {
		return fmt.Errorf("task node %s does not exist", taskID)
	}
	if node.State != APITask.StateClaimed {
		return fmt.Errorf("task node %s is %s, cannot abandon", taskID, node.State)
	}

	node.State = APITask.StateReady
	node.ClaimedBy = ""
	node.ExpiresAt = 0
	delete(m.claimed, taskID)
	m.publish(APIEvent.TaskRepublished, node)
	return nil
}

// ── Dispose ───────────────────────────────────────────────────────────

func (m *manager) Dispose(planID APITask.TaskID) error {
	m.Lock()
	defer m.Unlock()

	plan, ok := m.plans[planID]
	if !ok {
		return fmt.Errorf("plan %s not found", planID)
	}

	for _, node := range plan.Nodes {
		switch node.State {
		case APITask.StateDone, APITask.StateDisposed, APITask.StateFailed:
			continue
		default:
			node.State = APITask.StateDisposed
			node.ClaimedBy = ""
			node.ExpiresAt = 0
			delete(m.claimed, node.ID)
		}
	}

	return nil
}

// ── Expired ───────────────────────────────────────────────────────────

func (m *manager) Expired() []APITask.TaskNode {
	m.RLock()
	defer m.RUnlock()

	now := time.Now().Unix()
	var expired []APITask.TaskNode
	for _, node := range m.nodes {
		if node.State == APITask.StateClaimed && node.ExpiresAt > 0 && node.ExpiresAt < now {
			expired = append(expired, *node)
		}
	}
	return expired
}

// ── Query ─────────────────────────────────────────────────────────────

func (m *manager) Plan(taskID APITask.TaskID) (APITask.TaskPlan, error) {
	m.RLock()
	defer m.RUnlock()

	p, ok := m.plans[taskID]
	if !ok {
		return APITask.TaskPlan{}, fmt.Errorf("plan %s not found", taskID)
	}
	return *p, nil
}

func (m *manager) Status(taskID APITask.TaskID) (map[APITask.TaskID]APITask.NodeState, error) {
	m.RLock()
	defer m.RUnlock()

	p, ok := m.plans[taskID]
	if !ok {
		return nil, fmt.Errorf("plan %s not found", taskID)
	}
	states := make(map[APITask.TaskID]APITask.NodeState, len(p.Nodes))
	for id, node := range p.Nodes {
		states[id] = node.State
	}
	return states, nil
}

func (m *manager) IsComplete(taskID APITask.TaskID) (bool, error) {
	m.RLock()
	defer m.RUnlock()

	p, ok := m.plans[taskID]
	if !ok {
		return false, fmt.Errorf("plan %s not found", taskID)
	}
	return allDone(p), nil
}

// ── Internal helpers ──────────────────────────────────────────────────

func (m *manager) publish(t APIEvent.EventType, data interface{}) {
	m.eb.Publish(t, APIEvent.NewEvent(t, data))
}

func allDone(plan *APITask.TaskPlan) bool {
	for _, node := range plan.Nodes {
		if node.State != APITask.StateDone {
			return false
		}
	}
	return true
}

func (m *manager) GetModuleType() APIModule.ModuleType { return APIModule.TASKSTATEMANAGER }
func (m *manager) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(m.GetModuleType(), "default")
}

func (m *manager) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(m.GetModuleID(), m.GetModuleType(), APIModule.RUNNING)
}
func (m *manager) RegisterWithManager(mm modulemanager.ModuleManager) error { return mm.Register(m) }

// startDeadlineWatch spawns a goroutine that fires when the plan's
// deadline passes. If the plan hasn't completed by then, it sends a
// timeout notification and disposes all remaining nodes.
func (m *manager) startDeadlineWatch(plan *APITask.TaskPlan) {
	if plan.Deadline.IsZero() || plan.Notify == nil {
		return
	}
	go func() {
		delay := time.Until(plan.Deadline)
		if delay <= 0 {
			delay = 1 // already past — fire immediately
		}
		timer := time.NewTimer(delay)
		defer timer.Stop()

		<-timer.C

		m.Lock()
		done := allDone(plan)
		if !done {
			// Mark remaining nodes as disposed.
			for _, node := range plan.Nodes {
				switch node.State {
				case APITask.StateDone, APITask.StateDisposed, APITask.StateFailed:
					continue
				default:
					node.State = APITask.StateDisposed
					node.ClaimedBy = ""
					node.ExpiresAt = 0
					delete(m.claimed, node.ID)
				}
			}
			m.Unlock()
			plan.Notify <- APITask.PlanResult{
				Status: APITask.PlanTimeout,
				Text:   fmt.Sprintf("plan %s deadline exceeded", plan.ID),
			}
			return
		}
		m.Unlock()
	}()
}

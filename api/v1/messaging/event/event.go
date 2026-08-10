// Package apievent provides the event struct and event type definitions
// transferred between the event bus and other components.
package apievent

import (
	"sync/atomic"
)

// EventID represents the unique identifier for an event in every node
type EventID int64

// EventType represents the type of an event
// which can be used to identify the event and determine how to handle it
type EventType string

type EventOnComplete func(data any) error

const (
	// Task lifecycle
	TaskPreplanned  EventType = "task.preplanned" // single-node plan, preplanned, subscribed by planner
	TaskReady       EventType = "task.ready"
	TaskRunning     EventType = "task.running"
	// Deprecated: task completion is now signalled solely by TaskTraced
	// (Done / Failed states). These two remain for spec compatibility.
	TaskDone        EventType = "task.done"
	TaskFailed      EventType = "task.failed"
	TaskRepublished EventType = "task.republished"

	// Task delivery
	TaskAdReceived EventType = "task.ad.received"
	TaskCVReceived EventType = "task.cv.received"
	// TaskTraced carries a task lifecycle state change from the TaskTracer,
	// so observers (e.g. the commander judging progress) get live updates.
	TaskTraced EventType = "task.traced"
	// TaskAssign is the wire topic for a board assigning a task to a remote
	// executor (unicast to the winner). TaskAssigned is the local trigger the
	// executor subscribes to — a remote assignment is re-published locally
	// under TaskAssigned by the winner's board.
	TaskAssign    EventType = "task.assign"
	TaskAssigned  EventType = "task.assigned"

	// Hardware
	HardwareChanged EventType = "hardware.changed"

	// Workflow — planner ↔ commander ↔ planner loop
	TaskPlanned       EventType = "task.planned"        // planner → commander, carries *TaskPlan
	TaskPlanDone      EventType = "task.plan.done"      // commander detects completion → planner, carries TaskPlanResultMsg
	TaskPlanCompleted EventType = "task.plan.completed" // planner → wrapper, carries TaskPlanResultMsg

	// User telemetry — unicast to the plan owner node for progress display.
	// Distinct from the coordination events above: these exist only to show
	// the user which agent is doing what, and are consumed by TaskMonitor.
	TelemetryPlanning   EventType = "telemetry.planner.planning"
	TelemetryPlanned    EventType = "telemetry.planner.planned"
	TelemetryThinking   EventType = "telemetry.commander.thinking"
	TelemetryWaiting    EventType = "telemetry.commander.waiting"
	TelemetryFinalizing EventType = "telemetry.commander.finalizing"
	TelemetryRunning    EventType = "telemetry.executor.running"
	TelemetryExecDone   EventType = "telemetry.executor.done"
	TelemetryStepResult EventType = "telemetry.step_result"
)

// Event represents an event in the system
type Event struct {
	ID   EventID   `json:"id"`   // Unique identifier for the event
	Type EventType `json:"type"` // Type of the event
	Data any       `json:"data"` // Payload of the event, can be any type
}

var eventID atomic.Int64

func NewEventID() EventID {
	return EventID(eventID.Add(1))
}

func NewEvent(eventType EventType, data any) Event {
	return Event{
		ID:   NewEventID(),
		Type: eventType,
		Data: data,
	}
}

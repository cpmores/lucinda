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
	TaskDone        EventType = "task.done"
	TaskFailed      EventType = "task.failed"
	TaskRepublished EventType = "task.republished"

	// Task delivery
	TaskAdReceived EventType = "task.ad.received"
	TaskCVReceived EventType = "task.cv.received"
	TaskAssigned   EventType = "task.assigned"

	// Hardware
	HardwareChanged EventType = "hardware.changed"
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

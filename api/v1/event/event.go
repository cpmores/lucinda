// Package apievent api event provide event struct and event type definition
// transferred between eventbus and other components
package apievent

// EventID represents the unique identifier for an event in every node
type EventID int64

// EventType represents the type of an event
// which can be usef to identify the event and determine how to handle it
type EventType string

type Topic EventType

const (
	// User Events
	UserRequestReceived EventType = "UserRequestReceived"
	UserResponseSent    EventType = "UserResponseSent"
	// Task Events (Finished)
	TaskCreated  EventType = "TaskCreated"
	TaskPlanned  EventType = "TaskPlanned"
	TaskExecuted EventType = "TaskExecuted"
	TaskReduced  EventType = "TaskReduced"

	// Hardware changed
	HardwareChanged EventType = "hardware_changed"
)

// Event represents an event in the system
type Event struct {
	ID   EventID   `json:"id"`   // Unique identifier for the event
	Type EventType `json:"type"` // Type of the event
	Data any       `json:"data"` // Payload of the event, can be any type
}

func NewEvent(id EventID, eventType EventType, data any) Event {
	return Event{
		ID:   id,
		Type: eventType,
		Data: data,
	}
}

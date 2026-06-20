// Package apievent api event provide event struct and event type definition
// transferred between eventbus and other components
package apievent

// EventID represents the unique identifier for an event in every node
type EventID int64

// EventType represents the type of an event
// which can be usef to identify the event and determine how to handle it
type EventType string

type Topic EventType

type EventOnComplete func(data any) error

const (
	// User
	UserRequestReceived EventType = "user.requested"
	UserResponseSent    EventType = "user.responded"

	// Task lifecycle
	TaskPreplaned   EventType = "task.preplaned" // single-node plan, preplaned, subuscribed by planner
	TaskReady       EventType = "task.ready"
	TaskRunning     EventType = "task.running"
	TaskDone        EventType = "task.done"
	TaskFailed      EventType = "task.failed"
	TaskRepublished EventType = "task.republished"

	// Task Deliver
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

var eventID EventID = 0

func NewEventID() EventID {
	eventID++
	return eventID
}

func NewEvent(eventType EventType, data any) Event {
	return Event{
		ID:   NewEventID(),
		Type: eventType,
		Data: data,
	}
}

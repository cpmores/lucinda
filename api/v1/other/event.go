package api

type EventTopic string

const (
	// TASK_EVENT
	TASK_SUBMITTED EventTopic = "task.submitted"

	TASK_RESULT EventTopic = "task.result"
)

type Event struct {
	Topic EventTopic
	Data  any
}

package api

type EventTopic string

const (
// TASK_EVENT
)

type Event struct {
	Topic EventTopic
	Data  any
}

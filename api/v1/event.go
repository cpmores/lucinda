pakage api

const (
	TASK_EVENT_QUEUE_LENGTH = 100
	NODE_EVENT_QUEUE_LENGTH = 100
	PROVIDER_EVENT_QUEUE_LENGTH = 100
)

type TaskEvent struct {
	TaskID TaskID
	Status string
}

type NodeEvent struct {
	NodeID NodeID
	Status string
}

type ProviderEvent struct {
	ProviderID string
	Status string
}

var TaskEventQueue = make(chan TaskEvent, TASK_EVENT_QUEUE_LENGTH)
var NodeEventQueue = make(chan NodeEvent, NODE_EVENT_QUEUE_LENGTH)
var ProviderEventQueue = make(chan ProviderEvent, PROVIDER_EVENT_QUEUE_LENGTH)

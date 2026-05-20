package eventbus

import (
	"sync"

	"github.com/cpmores/lucinda/api/v1"
)

const EVENT_QUEUE_SIZE = 100

type EventBus interface {
	Subscribe(topic api.EventTopic) (chan api.Event, error)
	Unsubscribe(topic api.EventTopic, ch chan api.Event) error
	Publish(topic api.EventTopic, event api.Event) error
}

type DefaultEventBus struct {
	sync.RWMutex
	subscribers map[api.EventTopic][]chan api.Event
}

func NewDefaultEventBus() *DefaultEventBus {
	return &DefaultEventBus{
		subscribers: make(map[api.EventTopic][]chan api.Event),
	}
}

var (
	globalEventBus *DefaultEventBus
	globalOnce     sync.Once
)

func GetGlobalEventBus() *DefaultEventBus {
	globalOnce.Do(func() {
		globalEventBus = NewDefaultEventBus()
	})
	return globalEventBus
}

func (bus *DefaultEventBus) Subscribe(topic api.EventTopic) (chan api.Event, error) {
	bus.Lock()
	defer bus.Unlock()

	eventChan := make(chan api.Event, EVENT_QUEUE_SIZE)
	bus.subscribers[topic] = append(bus.subscribers[topic], eventChan)
	return eventChan, nil
}

func (bus *DefaultEventBus) Publish(topic api.EventTopic, event api.Event) error {
	bus.RLock()
	defer bus.RUnlock()

	if subscribers, ok := bus.subscribers[topic]; ok {
		for _, subscriber := range subscribers {
			select {
			case subscriber <- event:
			default:
			}
		}
	}

	return nil
}

func (bus *DefaultEventBus) Unsubscribe(topic api.EventTopic, ch chan api.Event) error {
	bus.Lock()
	defer bus.Unlock()

	if subscribers, ok := bus.subscribers[topic]; ok {
		for i, subscriber := range subscribers {
			if subscriber == ch {
				bus.subscribers[topic] = append(subscribers[:i], subscribers[i+1:]...)
				close(ch)
				return nil
			}
		}
	}

	return nil
}

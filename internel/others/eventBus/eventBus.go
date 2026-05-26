package eventbus

import (
	"context"
	"log"
	"sync"

	"github.com/cpmores/lucinda/api/v1"
)

const EVENT_QUEUE_SIZE = 100

type EventBus interface {
	Subscribe(topic api.EventTopic) (chan api.Event, error)
	Unsubscribe(topic api.EventTopic, ch chan api.Event) error
	Shutdown() error
	Publish(topic api.EventTopic, event api.Event) error
}

type DefaultEventBus struct {
	sync.RWMutex
	subscribers map[api.EventTopic][]chan api.Event
}

func NewDefaultEventBus(ctx context.Context) *DefaultEventBus {
	log.Printf("Initializing event bus")
	bus := &DefaultEventBus{
		subscribers: make(map[api.EventTopic][]chan api.Event),
	}

	go func() {
		<-ctx.Done()
		log.Printf("Event bus is shutting down: %s", ctx.Err().Error())
		if err := bus.Shutdown(); err != nil {
			log.Printf("Error shutting down event bus: %s", err.Error())
		} else {
			log.Printf("Event bus shutdown completed")
		}
	}()

	return bus
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
			// send event to subscriber (blocking send)
			subscriber <- event
		}
	}

	return nil
}

func (bus *DefaultEventBus) Shutdown() error {
	bus.Lock()
	defer bus.Unlock()

	for topic, subscribers := range bus.subscribers {
		for _, subscriber := range subscribers {
			close(subscriber)
		}
		delete(bus.subscribers, topic)
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

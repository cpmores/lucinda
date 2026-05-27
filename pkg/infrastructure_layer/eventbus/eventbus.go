// package eventbus providers an implementation of an event bus for lucinda
// This implementation is in-memory and should be used for testing purposes only.
package eventbus

import (
	APIevent "github.com/cpmores/lucinda/api/v1/event"
	APIeventbus "github.com/cpmores/lucinda/api/v1/event/eventbus"
	"log"
	"sync"
)

type EventBus interface {
	Subscribe(topic APIeventbus.Topic, length int64) chan APIevent.Event
	UnSubscribe(topic APIeventbus.Topic, ch chan APIevent.Event)
	Publish(topic APIeventbus.Topic, event APIevent.Event) error
}

// HACK: InMemoryEventBus is an implementation of the EventBus interface that stores subscribers in memory.
type InMemoryEventBus struct {
	sync.RWMutex
	subscribers map[APIeventbus.Topic][]chan APIevent.Event
}

// NewInMemoryEventBus creates a new instance of InMemoryEventBus
// returns a pointer to the newly created InMemoryEventBus
func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		subscribers: make(map[APIeventbus.Topic][]chan APIevent.Event),
	}
}

// Subscribe adds a new subscriber to the specified topic
// topic: the topic to subscribe to
// returns a channel that will receive events published to the specified topic
func (eb *InMemoryEventBus) Subscribe(topic APIeventbus.Topic, length int64) chan APIevent.Event {
	eb.Lock()
	defer eb.Unlock()
	returnCh := make(chan APIevent.Event, length)
	eb.subscribers[topic] = append(eb.subscribers[topic], returnCh)

	return returnCh
}

// Unsubscribe removes a subscriber from the specified topic
// topic: the topic to unsubscribe from
// ch: the channel to remove from the list of subscribers for the specified topic
// returns nothing
func (eb *InMemoryEventBus) UnSubscribe(topic APIeventbus.Topic, ch chan APIevent.Event) {
	eb.Lock()
	defer eb.Unlock()
	if subscribes, ok := eb.subscribers[topic]; ok {
		for i, subscriber := range subscribes {
			if subscriber == ch {
				eb.subscribers[topic] = append(subscribes[:i], subscribes[i+1:]...)
				close(ch)
			}
		}
	}
}

// Publish sends an event to all subscribers of the specified topic
// topic: the topic to publish to
// event: the event to publish
// returns an error if the topic channels are full, otherwise returns nil
func (eb *InMemoryEventBus) Publish(topic APIeventbus.Topic, event APIevent.Event) error {
	eb.RLock()
	defer eb.RUnlock()
	if subscriber, ok := eb.subscribers[topic]; !ok {
		return nil
	} else {
		for _, ch := range subscriber {
			// TODO: Deal with the case where the channel is full by skipping the subscriber
			select {
			case ch <- event:
			default:
				log.Printf("Event bus: channel for topic %s is full, skipping subscriber", topic)
			}
		}
	}

	return nil
}

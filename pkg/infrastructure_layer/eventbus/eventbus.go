// package eventbus providers an implementation of an event bus for lucinda
// This implementation is in-memory and should be used for testing purposes only.
package eventbus

import (
	APIevent "github.com/cpmores/lucinda/api/v1/event"
	APIeventbus "github.com/cpmores/lucinda/api/v1/event/eventbus"
	"sync"
)

type EventBus interface {
	Subscribe(topic APIeventbus.Topic) chan APIevent.Event
	UnSubscribe(topic APIeventbus.Topic, ch chan APIevent.Event)
	Publish(topic APIeventbus.Topic, event APIevent.Event)
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
func (eb *InMemoryEventBus) Subscribe(topic APIeventbus.Topic) chan APIevent.Event {
	eb.Lock()
	defer eb.Unlock()
	returnCh := make(chan APIevent.Event)
	eb.subscribers[topic] = append(eb.subscribers[topic], returnCh)

	return returnCh
}

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
	} else {
		return
	}
}

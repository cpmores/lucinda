// Package eventbus providers an implementation of an event bus for lucinda
// This implementation is in-memory and should be used for testing purposes only.
package eventbus

import (
	"sync"

	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

type EventBus interface {
	Subscribe(topic APIEvent.EventType, length int64) chan APIEvent.Event
	UnSubscribe(topic APIEvent.EventType, ch chan APIEvent.Event)
	Publish(topic APIEvent.EventType, event APIEvent.Event) error
	Release() error
}

// InMemoryEventBus is an implementation of the EventBus interface that stores subscribers in memory.
type InMemoryEventBus struct {
	sync.RWMutex
	log         *logger.Logger
	subscribers map[APIEvent.EventType][]chan APIEvent.Event
}

// NewInMemoryEventBus creates a new instance of InMemoryEventBus
// returns a pointer to the newly created InMemoryEventBus
func NewInMemoryEventBus(log *logger.Logger) *InMemoryEventBus {
	return &InMemoryEventBus{
		log:         log,
		subscribers: make(map[APIEvent.EventType][]chan APIEvent.Event),
	}
}

// Subscribe adds a new subscriber to the specified topic
// topic: the topic to subscribe to
// returns a channel that will receive events published to the specified topic
func (eb *InMemoryEventBus) Subscribe(topic APIEvent.EventType, length int64) chan APIEvent.Event {
	eb.Lock()
	defer eb.Unlock()
	returnCh := make(chan APIEvent.Event, length)
	eb.subscribers[topic] = append(eb.subscribers[topic], returnCh)

	return returnCh
}

// UnSubscribe removes a subscriber from the specified topic
// topic: the topic to unsubscribe from
// ch: the channel to remove from the list of subscribers for the specified topic
// returns nothing
func (eb *InMemoryEventBus) UnSubscribe(topic APIEvent.EventType, ch chan APIEvent.Event) {
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
func (eb *InMemoryEventBus) Publish(topic APIEvent.EventType, event APIEvent.Event) error {
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
				eb.log.Warn("channel full, skipping subscriber", "topic", topic)
			}
		}
	}

	return nil
}

// Release all the channels covering all the topics
func (eb *InMemoryEventBus) Release() error {
	eb.Lock()
	defer eb.Unlock()
	for _, channels := range eb.subscribers {
		for _, channel := range channels {
			close(channel)
		}
	}

	return nil
}

// ── AvailableModule Interface ──────────────────────────────────────────────────────────

func (eb *InMemoryEventBus) GetModuleType() APIModule.ModuleType {
	return APIModule.EventBus
}

func (eb *InMemoryEventBus) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(eb.GetModuleType(), "default")
}

func (eb *InMemoryEventBus) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(eb.GetModuleID(), eb.GetModuleType(), APIModule.Running)
}

func (eb *InMemoryEventBus) RegisterWithManager(manager modulemanager.ModuleManager) error {
	return manager.Register(eb)
}

func (eb *InMemoryEventBus) DependsOn() map[APIModule.ModuleType]string {
	return nil
}

func (eb *InMemoryEventBus) DependsEnable() error {
	return nil
}

// Package taskpostman bridges EventBus ↔ handlers and Transport ↔ EventBus.
package taskpostman

import (
	"context"
	"fmt"
	"log"
	"sync"

	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	APIModule "github.com/cpmores/lucinda/api/v1/module"
	APINode "github.com/cpmores/lucinda/api/v1/node"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

var TopicLen int64 = 100

type Postman interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Publish(topic APIEvent.EventType, event APIEvent.Event) error
	Watch(ctx context.Context, topic APIEvent.EventType, handler APIEvent.EventOnComplete) error
	Deliver(ctx context.Context, tp transport.Transport, protocol APINode.Protocol) error
	Stop()
}

type postman struct {
	eb      eventbus.EventBus
	cancels []context.CancelFunc
	mu      sync.Mutex
	wg      sync.WaitGroup
}

func NewTaskPostman(eb eventbus.EventBus) Postman {
	return &postman{eb: eb}
}

// Watch subscribes to an EventBus topic and calls handler for each event.
func (p *postman) Watch(ctx context.Context, topic APIEvent.EventType, handler APIEvent.EventOnComplete) error {
	ctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.cancels = append(p.cancels, cancel)
	p.mu.Unlock()
	ch := p.eb.Subscribe(topic, TopicLen)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				if topic == "task.ready" {
					log.Printf("postman: Watch task.ready received event")
				}
				if err := handler(event.Data); err != nil {
					log.Printf("postman: handler error for %s: %v", topic, err)
				}
			}
		}
	}()

	return nil
}

// Deliver reads incoming messages from a Transport protocol and publishes
// their Body to an EventBus topic. This bridges network → EventBus.
func (p *postman) Deliver(ctx context.Context, tp transport.Transport, protocol APINode.Protocol) error {
	ch, err := tp.Incoming(protocol)
	if err != nil {
		return fmt.Errorf("deliver: %w", err)
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				p.eb.Publish(APIEvent.EventType(msg.Topic),
					APIEvent.NewEvent(APIEvent.EventType(msg.Topic), msg.Body))
			}
		}
	}()

	return nil
}

// Stop cancels all watchers and waits for handlers to drain.
func (p *postman) Publish(topic APIEvent.EventType, event APIEvent.Event) error {
	err := p.eb.Publish(topic, event)
	return err
}

func (p *postman) Stop() {
	p.mu.Lock()
	for _, cancel := range p.cancels {
		cancel()
	}
	p.mu.Unlock()
	p.wg.Wait()
	log.Println("postman: stopped")
}

func (p *postman) GetModuleType() APIModule.ModuleType { return APIModule.TASKPOSTMAN }
func (p *postman) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(p.GetModuleType(), "default")
}

func (p *postman) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(p.GetModuleID(), p.GetModuleType(), APIModule.Running)
}
func (p *postman) RegisterWithManager(m modulemanager.ModuleManager) error { return m.Register(p) }

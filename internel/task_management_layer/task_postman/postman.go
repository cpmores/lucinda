// Package taskpostman bridges EventBus ↔ handlers and Transport ↔ EventBus.
package taskpostman

import (
	"context"
	"fmt"
	"log"
	"sync"

	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	apinode "github.com/cpmores/lucinda/api/v1/node"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

var TopicLen int64 = 100

type Postman interface {
	Watch(ctx context.Context, topic APIEvent.EventType, handler APIEvent.EventOnComplete) error
	Deliver(ctx context.Context, tp transport.Transport, protocol apinode.Protocol) error
	Stop()
}

type postman struct {
	eb     eventbus.EventBus
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewTaskPostman(eb eventbus.EventBus) Postman {
	return &postman{eb: eb}
}

// Watch subscribes to an EventBus topic and calls handler for each event.
func (p *postman) Watch(ctx context.Context, topic APIEvent.EventType, handler APIEvent.EventOnComplete) error {
	ctx, p.cancel = context.WithCancel(ctx)
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
func (p *postman) Deliver(ctx context.Context, tp transport.Transport, protocol apinode.Protocol) error {
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
func (p *postman) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	log.Println("postman: stopped")
}

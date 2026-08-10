// Package eventx provides small EventBus conveniences shared by the
// workflow components: subscribing with context cancellation, and emitting
// user-facing telemetry events.
package eventx

import (
	"context"

	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
)

// Watch subscribes to a topic and invokes handler for each event's Data
// until ctx is cancelled or the channel closes.
func Watch(ctx context.Context, eb eventbus.EventBus, log *logger.Logger, topic APIEvent.EventType, handler func(any)) {
	ch := eb.Subscribe(topic, 64)
	go func() {
		defer eb.UnSubscribe(topic, ch)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				handler(ev.Data)
			}
		}
	}()
}

// Emit publishes a telemetry event for user-facing progress display. The
// payload is one of the apistream frame data types so the monitor can
// forward it verbatim.
func Emit(eb eventbus.EventBus, log *logger.Logger, ev APIEvent.EventType, data any) {
	if log != nil {
		log.Debug("telemetry", "event", string(ev))
	}
	_ = eb.Publish(ev, APIEvent.NewEvent(ev, data))
}

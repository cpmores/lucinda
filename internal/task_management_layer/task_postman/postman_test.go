package taskpostman

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
)

func TestWatchReceivesEvents(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	pm := NewTaskPostman(eb)

	var count atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer pm.Stop()

	pm.Watch(ctx, APIEvent.TaskReady, func(data any) error {
		count.Add(1)
		return nil
	})

	// Publish some events.
	for i := 0; i < 5; i++ {
		eb.Publish(APIEvent.TaskReady, APIEvent.NewEvent(APIEvent.TaskReady, "hello"))
	}

	// Wait for delivery.
	time.Sleep(100 * time.Millisecond)

	if count.Load() != 5 {
		t.Fatalf("expected 5 events, got %d", count.Load())
	}
}

func TestWatchGetsEventData(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	pm := NewTaskPostman(eb)

	var received any
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer pm.Stop()

	pm.Watch(ctx, APIEvent.TaskReady, func(data any) error {
		received = data
		return nil
	})

	eb.Publish(APIEvent.TaskReady, APIEvent.NewEvent(APIEvent.TaskReady, "payload"))
	time.Sleep(100 * time.Millisecond)

	if received != "payload" {
		t.Fatalf("expected 'payload', got %v", received)
	}
}

func TestStopExitsWatch(t *testing.T) {
	eb := eventbus.NewInMemoryEventBus()
	pm := NewTaskPostman(eb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pm.Watch(ctx, APIEvent.TaskReady, func(data any) error {
		return nil
	})

	pm.Stop()

	// Publishing after stop should not panic — channel is closed but
	// EventBus Publish skips closed channels.
	eb.Publish(APIEvent.TaskReady, APIEvent.NewEvent(APIEvent.TaskReady, "after-stop"))
}

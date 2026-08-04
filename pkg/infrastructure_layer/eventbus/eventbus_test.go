package eventbus

import (
	"testing"
	"time"

	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
)

func TestNewInMemoryEventBus(t *testing.T) {
	bus := NewInMemoryEventBus()
	if bus == nil {
		t.Fatal("NewInMemoryEventBus returned nil")
	}
	if bus.subscribers == nil {
		t.Fatal("subscribers map not initialized")
	}
}

func TestSubscribe(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("test.topic")

	ch := bus.Subscribe(topic, 10)
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
	if cap(ch) != 10 {
		t.Fatalf("expected channel capacity 10, got %d", cap(ch))
	}

	bus.Lock()
	subs := bus.subscribers[topic]
	bus.Unlock()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", len(subs))
	}
}

func TestSubscribeMultipleTopics(t *testing.T) {
	bus := NewInMemoryEventBus()
	topicA := APIEvent.EventType("topic.a")
	topicB := APIEvent.EventType("topic.b")

	chA := bus.Subscribe(topicA, 5)
	chB := bus.Subscribe(topicB, 5)

	bus.Lock()
	if len(bus.subscribers) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(bus.subscribers))
	}
	bus.Unlock()

	if chA == chB {
		t.Fatal("expected different channels for different topics")
	}
}

func TestPublishToSingleSubscriber(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("test.topic")
	ch := bus.Subscribe(topic, 5)

	event := APIEvent.Event{
		ID:   1,
		Type: "test",
		Data: "hello",
	}

	if err := bus.Publish(topic, event); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case received := <-ch:
		if received.ID != event.ID {
			t.Fatalf("expected event ID %d, got %d", event.ID, received.ID)
		}
		if received.Type != event.Type {
			t.Fatalf("expected event type %s, got %s", event.Type, received.Type)
		}
		if received.Data != event.Data {
			t.Fatalf("expected event data %v, got %v", event.Data, received.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestPublishToMultipleSubscribers(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("test.topic")

	ch1 := bus.Subscribe(topic, 5)
	ch2 := bus.Subscribe(topic, 5)
	ch3 := bus.Subscribe(topic, 5)

	event := APIEvent.Event{ID: 42, Type: "broadcast", Data: "all"}
	if err := bus.Publish(topic, event); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	for i, ch := range []chan APIEvent.Event{ch1, ch2, ch3} {
		select {
		case received := <-ch:
			if received.ID != 42 {
				t.Fatalf("subscriber %d: expected event ID 42, got %d", i+1, received.ID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d: timed out waiting for event", i+1)
		}
	}
}

func TestPublishToTopicWithNoSubscribers(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("nobody.here")

	// Should not panic or error — just a no-op.
	if err := bus.Publish(topic, APIEvent.Event{ID: 1}); err != nil {
		t.Fatalf("Publish to empty topic should not error: %v", err)
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("test.topic")
	ch := bus.Subscribe(topic, 5)

	bus.UnSubscribe(topic, ch)

	// Verify the channel is closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after unsubscribe")
		}
	default:
		// Channel may have been drained already — that's fine.
	}

	// Verify subscriber list is empty.
	bus.RLock()
	subs := bus.subscribers[topic]
	bus.RUnlock()
	if len(subs) != 0 {
		t.Fatalf("expected 0 subscribers after unsubscribe, got %d", len(subs))
	}
}

func TestUnsubscribeOnlyRemovesTargetChannel(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("test.topic")

	ch1 := bus.Subscribe(topic, 5)
	ch2 := bus.Subscribe(topic, 5)

	bus.UnSubscribe(topic, ch1)

	// ch1 should be closed. Verify ch2 is still intact by publishing.
	event := APIEvent.Event{ID: 99}
	if err := bus.Publish(topic, event); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case received := <-ch2:
		if received.ID != 99 {
			t.Fatalf("expected event ID 99, got %d", received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch2 should have received the event after ch1 was unsubscribed")
	}

	bus.Lock()
	subs := bus.subscribers[topic]
	bus.Unlock()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscriber remaining, got %d", len(subs))
	}
}

func TestUnsubscribeNonexistentChannel(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("test.topic")
	ch := make(chan APIEvent.Event, 1)

	// Should not panic.
	bus.UnSubscribe(topic, ch)
}

func TestChannelFullDropsMessage(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("test.topic")

	// Create a channel with capacity 1 and fill it.
	ch := bus.Subscribe(topic, 1)
	ch <- APIEvent.Event{ID: 1} // fill the buffer

	// Publish another event — should be dropped (non-blocking).
	if err := bus.Publish(topic, APIEvent.Event{ID: 2, Data: "dropped"}); err != nil {
		t.Fatalf("Publish should not error even when channel is full: %v", err)
	}

	// Drain the first event.
	select {
	case received := <-ch:
		if received.ID != 1 {
			t.Fatalf("expected first event ID 1, got %d", received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for first event")
	}

	// Verify the channel is now empty (the second event was dropped).
	select {
	case <-ch:
		t.Fatal("unexpected event — channel should be empty after drop")
	case <-time.After(50 * time.Millisecond):
		// Expected: nothing in the channel.
	}
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("concurrent")

	const numSubscribers = 10
	const numPublishes = 50

	channels := make([]chan APIEvent.Event, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		channels[i] = bus.Subscribe(topic, numPublishes)
	}

	// Publish from multiple goroutines.
	done := make(chan struct{})
	for i := 0; i < numPublishes; i++ {
		go func(id int) {
			bus.Publish(topic, APIEvent.Event{ID: APIEvent.EventID(id)})
		}(i)
	}

	// Collect results from all subscribers.
	go func() {
		for _, ch := range channels {
			for j := 0; j < numPublishes; j++ {
				<-ch
			}
		}
		close(done)
	}()

	select {
	case <-done:
		// All events received.
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent publish/subscribe")
	}
}

func TestMultiplePublishEventsInOrder(t *testing.T) {
	bus := NewInMemoryEventBus()
	topic := APIEvent.EventType("ordered")

	ch := bus.Subscribe(topic, 10)

	events := []APIEvent.Event{
		{ID: 1, Data: "first"},
		{ID: 2, Data: "second"},
		{ID: 3, Data: "third"},
	}

	for _, e := range events {
		if err := bus.Publish(topic, e); err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	for i, expected := range events {
		select {
		case received := <-ch:
			if received.ID != expected.ID {
				t.Fatalf("event %d: expected ID %d, got %d", i, expected.ID, received.ID)
			}
			if received.Data != expected.Data {
				t.Fatalf("event %d: expected data %v, got %v", i, expected.Data, received.Data)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("event %d: timed out", i)
		}
	}
}

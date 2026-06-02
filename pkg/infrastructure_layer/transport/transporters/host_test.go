package transport_libp2p

import (
	"context"
	"sync"
	"testing"
	"time"

	APINode "github.com/cpmores/lucinda/api/v1/node"
)

const testProtocol = APINode.Protocol("/lucinda/test/1.0.0")

// newTestTransport creates a transport listening on a random port.
func newTestTransport(t *testing.T) *Libp2pTransport {
	t.Helper()
	tr, err := NewLibp2pTransport(Libp2pTransportOptions{
		Addrs:      []string{"/ip4/127.0.0.1/tcp/0"},
		OutsLength: 20,
		InsLength:  100,
	})
	if err != nil {
		t.Fatalf("NewLibp2pTransport: %v", err)
	}
	return tr
}

// startTestTransport starts the transport and opens the test protocol.
func startTestTransport(t *testing.T, tr *Libp2pTransport, ctx context.Context) {
	t.Helper()
	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := tr.Open(ctx, testProtocol); err != nil {
		t.Fatalf("Open: %v", err)
	}
}

func TestNewLibp2pTransport(t *testing.T) {
	tr, err := NewLibp2pTransport(Libp2pTransportOptions{
		Addrs: []string{"/ip4/127.0.0.1/tcp/0"},
	})
	if err != nil {
		t.Fatalf("NewLibp2pTransport failed: %v", err)
	}
	if tr.IsStarted {
		t.Fatal("transport should not be started after New")
	}
	if tr.NodeID != "" {
		t.Fatal("NodeID should be empty before Start")
	}
	if tr.outs == nil || tr.ins == nil {
		t.Fatal("outs/ins maps should be initialized")
	}
}

func TestStartStop(t *testing.T) {
	tr := newTestTransport(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !tr.IsStarted {
		t.Fatal("IsStarted should be true after Start")
	}
	if tr.NodeID == "" {
		t.Fatal("NodeID should be set after Start")
	}

	if err := tr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if tr.IsStarted {
		t.Fatal("IsStarted should be false after Stop")
	}
}

func TestDoubleStartError(t *testing.T) {
	tr := newTestTransport(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := tr.Start(ctx); err == nil {
		t.Fatal("expected error on double Start, got nil")
	}
	tr.Stop()
}

func TestStopBeforeStartError(t *testing.T) {
	tr := newTestTransport(t)
	if err := tr.Stop(); err == nil {
		t.Fatal("expected error stopping before start, got nil")
	}
}

func TestOpenAndCloseProtocol(t *testing.T) {
	tr := newTestTransport(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startTestTransport(t, tr, ctx)

	// Double open should error.
	if err := tr.Open(ctx, testProtocol); err == nil {
		t.Fatal("expected error on double Open, got nil")
	}

	// Close the protocol.
	if err := tr.Close(ctx, testProtocol); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Open again after close should work.
	if err := tr.Open(ctx, testProtocol); err != nil {
		t.Fatalf("re-Open after Close: %v", err)
	}

	tr.Stop()
}

func TestCloseNonexistentProtocol(t *testing.T) {
	tr := newTestTransport(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := tr.Close(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error closing nonexistent protocol, got nil")
	}

	tr.Stop()
}

func TestSendBetweenTwoTransports(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create two transports.
	alice := newTestTransport(t)
	bob := newTestTransport(t)

	startTestTransport(t, alice, ctx)
	startTestTransport(t, bob, ctx)
	defer alice.Stop()
	defer bob.Stop()

	// Get bob's address and have alice dial him.
	bobAddrs := bob.Host.Addrs()
	if len(bobAddrs) == 0 {
		t.Fatal("bob has no addresses")
	}
	bobAddr := bobAddrs[0].String() + "/p2p/" + string(bob.NodeID)

	if err := alice.Dial(ctx, bobAddr); err != nil {
		t.Fatalf("alice dial bob: %v", err)
	}

	// Give the dial a moment to establish.
	time.Sleep(200 * time.Millisecond)

	// Alice sends a message to Bob.
	sentMsg := APINode.NodeMessage{
		Protocol:  testProtocol,
		Timestamp: time.Now().Unix(),
		From:      alice.NodeID,
		To:        bob.NodeID,
		Body:      "hello from alice",
	}

	if err := alice.Send(ctx, bob.NodeID, sentMsg); err != nil {
		t.Fatalf("alice Send: %v", err)
	}

	// Bob should receive the message on his incoming channel.
	bob.Lock()
	bobCh, ok := bob.ins[testProtocol]
	bob.Unlock()
	if !ok {
		t.Fatal("bob has no incoming channel for test protocol")
	}

	select {
	case received := <-bobCh:
		if received.From != alice.NodeID {
			t.Fatalf("expected From=%s, got %s", alice.NodeID, received.From)
		}
		if received.To != bob.NodeID {
			t.Fatalf("expected To=%s, got %s", bob.NodeID, received.To)
		}
		if received.Body != "hello from alice" {
			t.Fatalf("expected body 'hello from alice', got %v", received.Body)
		}
		if received.Protocol != testProtocol {
			t.Fatalf("expected protocol %s, got %s", testProtocol, received.Protocol)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bob: timed out waiting for message from alice")
	}
}

func TestSendMultipleMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	alice := newTestTransport(t)
	bob := newTestTransport(t)

	startTestTransport(t, alice, ctx)
	startTestTransport(t, bob, ctx)
	defer alice.Stop()
	defer bob.Stop()

	bobAddr := bob.Host.Addrs()[0].String() + "/p2p/" + string(bob.NodeID)
	if err := alice.Dial(ctx, bobAddr); err != nil {
		t.Fatalf("alice dial bob: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	const numMessages = 10
	for i := 0; i < numMessages; i++ {
		msg := APINode.NodeMessage{
			Protocol: testProtocol,
			From:     alice.NodeID,
			To:       bob.NodeID,
			Body:     i,
		}
		if err := alice.Send(ctx, bob.NodeID, msg); err != nil {
			t.Fatalf("alice Send %d: %v", i, err)
		}
	}

	bob.Lock()
	bobCh := bob.ins[testProtocol]
	bob.Unlock()

	for i := 0; i < numMessages; i++ {
		select {
		case received := <-bobCh:
			val, ok := received.Body.(float64) // JSON numbers decode as float64
			if !ok {
				t.Fatalf("message %d: expected float64 body, got %T", i, received.Body)
			}
			if int(val) != i {
				t.Fatalf("message %d: expected body %d, got %v", i, i, int(val))
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("bob: timed out waiting for message %d", i)
		}
	}
}

func TestSendToUnknownPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alice := newTestTransport(t)
	startTestTransport(t, alice, ctx)
	defer alice.Stop()

	// Sending to a peer we've never connected to should not block forever.
	msg := APINode.NodeMessage{
		Protocol: testProtocol,
		From:     alice.NodeID,
		To:       "12D3KooWUnknownPeer",
		Body:     "nobody home",
	}

	// The send itself queues to the channel and returns immediately.
	// The sendWorker will fail to open a stream, but shouldn't panic.
	if err := alice.Send(ctx, msg.To, msg); err != nil {
		t.Fatalf("Send should queue even for unknown peer: %v", err)
	}

	// Give the sendWorker time to attempt and fail.
	time.Sleep(500 * time.Millisecond)
	// If we get here without panic, the test passes.
}

func TestPublishToPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	alice := newTestTransport(t)
	bob := newTestTransport(t)
	carol := newTestTransport(t)

	startTestTransport(t, alice, ctx)
	startTestTransport(t, bob, ctx)
	startTestTransport(t, carol, ctx)
	defer alice.Stop()
	defer bob.Stop()
	defer carol.Stop()

	// Connect alice to both bob and carol.
	bobAddr := bob.Host.Addrs()[0].String() + "/p2p/" + string(bob.NodeID)
	carolAddr := carol.Host.Addrs()[0].String() + "/p2p/" + string(carol.NodeID)

	if err := alice.Dial(ctx, bobAddr); err != nil {
		t.Fatalf("alice dial bob: %v", err)
	}
	if err := alice.Dial(ctx, carolAddr); err != nil {
		t.Fatalf("alice dial carol: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Alice publishes a message.
	msg := APINode.NodeMessage{
		Protocol: testProtocol,
		From:     alice.NodeID,
		Body:     "broadcast",
	}
	if err := alice.Publish(ctx, msg); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Both bob and carol should receive it (alice skips self).
	bob.Lock()
	bobCh := bob.ins[testProtocol]
	bob.Unlock()
	carol.Lock()
	carolCh := carol.ins[testProtocol]
	carol.Unlock()

	for _, tc := range []struct {
		name string
		ch   chan APINode.NodeMessage
	}{
		{"bob", bobCh},
		{"carol", carolCh},
	} {
		select {
		case received := <-tc.ch:
			if received.Body != "broadcast" {
				t.Fatalf("%s: expected body 'broadcast', got %v", tc.name, received.Body)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s: timed out waiting for published message", tc.name)
		}
	}
}

func TestContextCancellationStopsTransport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tr := newTestTransport(t)
	startTestTransport(t, tr, ctx)

	// Cancel the context — the auto-shutdown goroutine should call Stop().
	cancel()

	// Give it a moment to shut down.
	time.Sleep(200 * time.Millisecond)

	tr.RLock()
	started := tr.IsStarted
	tr.RUnlock()

	if started {
		t.Fatal("transport should be stopped after context cancellation")
	}
}

func TestPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	alice := newTestTransport(t)
	bob := newTestTransport(t)

	startTestTransport(t, alice, ctx)
	startTestTransport(t, bob, ctx)
	defer alice.Stop()
	defer bob.Stop()

	bobAddr := bob.Host.Addrs()[0].String() + "/p2p/" + string(bob.NodeID)
	if err := alice.Dial(ctx, bobAddr); err != nil {
		t.Fatalf("alice dial bob: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// After dialing, bob should appear in alice's peer list.
	peers := alice.Peers()
	found := false
	for _, p := range peers {
		if p == bob.NodeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("bob (%s) not found in alice's peers: %v", bob.NodeID, peers)
	}
}

func TestDisconnectCleansOutboundChannels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	alice := newTestTransport(t)
	bob := newTestTransport(t)

	startTestTransport(t, alice, ctx)
	startTestTransport(t, bob, ctx)
	defer alice.Stop()

	bobAddr := bob.Host.Addrs()[0].String() + "/p2p/" + string(bob.NodeID)
	if err := alice.Dial(ctx, bobAddr); err != nil {
		t.Fatalf("alice dial bob: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Send a message to create an outbound channel for bob.
	msg := APINode.NodeMessage{
		Protocol: testProtocol,
		From:     alice.NodeID,
		To:       bob.NodeID,
		Body:     "ping",
	}
	if err := alice.Send(ctx, bob.NodeID, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify the outbound channel exists.
	alice.RLock()
	_, hasOut := alice.outs[bob.NodeID]
	alice.RUnlock()
	if !hasOut {
		t.Fatal("expected outbound channel for bob to exist after Send")
	}

	// Stop bob — this should trigger DisconnectedF on alice, cleaning channels.
	bob.Stop()
	time.Sleep(300 * time.Millisecond)

	// Alice's outbound channel for bob should be cleaned up.
	alice.RLock()
	_, hasOut = alice.outs[bob.NodeID]
	alice.RUnlock()
	if hasOut {
		t.Fatal("expected outbound channel for bob to be cleaned after disconnect")
	}
}

func TestConcurrentSends(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alice := newTestTransport(t)
	bob := newTestTransport(t)

	startTestTransport(t, alice, ctx)
	startTestTransport(t, bob, ctx)
	defer alice.Stop()
	defer bob.Stop()

	bobAddr := bob.Host.Addrs()[0].String() + "/p2p/" + string(bob.NodeID)
	if err := alice.Dial(ctx, bobAddr); err != nil {
		t.Fatalf("alice dial bob: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	const numGoroutines = 5
	const messagesPerGoroutine = 4

	// Send concurrently.
	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := APINode.NodeMessage{
					Protocol: testProtocol,
					From:     alice.NodeID,
					To:       bob.NodeID,
					Body:     []int{id, j},
				}
				if err := alice.Send(ctx, bob.NodeID, msg); err != nil {
					t.Errorf("Send goroutine %d msg %d: %v", id, j, err)
				}
			}
		}(i)
	}
	wg.Wait()

	// Count received messages on bob.
	bob.Lock()
	bobCh := bob.ins[testProtocol]
	bob.Unlock()

	totalExpected := numGoroutines * messagesPerGoroutine
	received := 0
	timeout := time.After(15 * time.Second)
	for received < totalExpected {
		select {
		case <-bobCh:
			received++
		case <-timeout:
			t.Fatalf("received %d/%d messages before timeout", received, totalExpected)
		}
	}
}

func TestSendBufferFullReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a transport with a tiny outbound buffer.
	tr, err := NewLibp2pTransport(Libp2pTransportOptions{
		Addrs:      []string{"/ip4/127.0.0.1/tcp/0"},
		OutsLength: 1,
		InsLength:  10,
	})
	if err != nil {
		t.Fatalf("NewLibp2pTransport: %v", err)
	}
	startTestTransport(t, tr, ctx)
	defer tr.Stop()

	// Fill the outbound buffer for a fake peer.
	peerID := APINode.NodeID("12D3KooWFakePeer")
	msg := APINode.NodeMessage{
		Protocol: testProtocol,
		From:     tr.NodeID,
		To:       peerID,
		Body:     "fill",
	}

	// Queue one message (fills the capacity-1 channel).
	if err := tr.Send(ctx, peerID, msg); err != nil {
		t.Fatalf("first Send: %v", err)
	}

	// The second send should return an error because the channel is full.
	if err := tr.Send(ctx, peerID, msg); err == nil {
		t.Fatal("expected error when sending to full buffer, got nil")
	}
}

func TestDial(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alice := newTestTransport(t)
	bob := newTestTransport(t)

	startTestTransport(t, alice, ctx)
	startTestTransport(t, bob, ctx)
	defer alice.Stop()
	defer bob.Stop()

	// Dial bob from alice using full multiaddr.
	bobAddr := bob.Host.Addrs()[0].String() + "/p2p/" + string(bob.NodeID)
	if err := alice.Dial(ctx, bobAddr); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Verify bob appears in alice's peer list.
	peers := alice.Peers()
	found := false
	for _, p := range peers {
		if p == bob.NodeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("bob (%s) not found in alice's peers: %v", bob.NodeID, peers)
	}
}

func TestSelfConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := newTestTransport(t)
	startTestTransport(t, tr, ctx)
	defer tr.Stop()

	// Open already created a self-connection via selfOutConnect.
	// Verify we can send a message to ourself via the self outbound channel.
	msg := APINode.NodeMessage{
		Protocol: testProtocol,
		From:     tr.NodeID,
		To:       tr.NodeID,
		Body:     "self-test",
	}

	if err := tr.Send(ctx, tr.NodeID, msg); err != nil {
		t.Fatalf("Send to self: %v", err)
	}

	// Should arrive on the incoming channel.
	tr.RLock()
	ch := tr.ins[testProtocol]
	tr.RUnlock()

	select {
	case received := <-ch:
		if received.Body != "self-test" {
			t.Fatalf("expected body 'self-test', got %v", received.Body)
		}
		if received.From != tr.NodeID {
			t.Fatalf("expected From=%s, got %s", tr.NodeID, received.From)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for self-sent message")
	}
}

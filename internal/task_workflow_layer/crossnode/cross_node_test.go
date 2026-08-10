// Package crossnode_test verifies the distributed contract: a remote
// node's telemetry and final-answer stream reach the plan owner node over
// the transport, demuxed by plan ID.
package crossnode_test

import (
	"context"
	"testing"
	"time"

	APISteam "github.com/cpmores/lucinda/api/v1/domain/stream"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	"github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	"github.com/cpmores/lucinda/internal/task_management_layer/telemetry_bridge"
	"github.com/cpmores/lucinda/internal/testutil"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/eventx"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_monitor"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// node bundles the per-node wiring for the bridge/router/monitor modules.
type node struct {
	eb      *eventbus.InMemoryEventBus
	tp      *testutil.MockTransport
	bridge  telemetrybridge.TelemetryBridge
	router  streamrouter.StreamRouter
	monitor taskmonitor.TaskMonitor
}

// newNode wires one mesh node and returns it with its deps enabled.
func newNode(t *testing.T, id string, withMonitor bool) *node {
	t.Helper()
	log := logger.Discard()
	mm := modulemanager.NewModuleManager()
	eb := eventbus.NewInMemoryEventBus(log.Child(id + "-eb"))
	tp := testutil.NewMockTransport(id)

	eb.RegisterWithManager(mm)
	tp.RegisterWithManager(mm)

	router := streamrouter.NewStreamRouter(log.Child(id+"-router"))
	bridge := telemetrybridge.NewTelemetryBridge(log.Child(id+"-bridge"))
	router.RegisterWithManager(mm)
	bridge.RegisterWithManager(mm)

	var monitor taskmonitor.TaskMonitor
	if withMonitor {
		monitor = taskmonitor.NewTaskMonitor(log.Child(id+"-monitor"))
		monitor.RegisterWithManager(mm)
	}

	if err := mm.VerifyInit(); err != nil {
		t.Fatalf("%s VerifyInit: %v", id, err)
	}
	if err := mm.EnableDeps(); err != nil {
		t.Fatalf("%s EnableDeps: %v", id, err)
	}
	return &node{eb: eb, tp: tp, bridge: bridge, router: router, monitor: monitor}
}

func (n *node) start(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := n.router.Start(ctx); err != nil {
		t.Fatalf("router start: %v", err)
	}
	if err := n.bridge.Start(ctx); err != nil {
		t.Fatalf("bridge start: %v", err)
	}
	if n.monitor != nil {
		if err := n.monitor.Start(ctx); err != nil {
			t.Fatalf("monitor start: %v", err)
		}
	}
}

func TestRemoteTelemetryReachesOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	owner := newNode(t, "node-A", true)
	worker := newNode(t, "node-B", false)
	owner.tp.Peer = worker.tp
	worker.tp.Peer = owner.tp

	owner.start(t, ctx)
	worker.start(t, ctx)

	const planID = "plan-X"
	owner.monitor.Register(planID)
	frames, err := owner.monitor.Open(planID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Worker produces telemetry owned by node-A.
	eventx.Emit(worker.eb, logger.Discard(), APIEvent.TelemetryRunning, APISteam.StatusData{
		Component: "executor", State: "running", Owner: "node-A",
		PlanID: planID, TaskID: "t-1",
	})

	select {
	case f := <-frames:
		if f.Event != APISteam.SSETypeStatus {
			t.Fatalf("frame event = %s, want status", f.Event)
		}
		sd, ok := f.Data.(APISteam.StatusData)
		if !ok {
			t.Fatalf("frame data not StatusData: %T", f.Data)
		}
		if sd.State != "running" || sd.Component != "executor" {
			t.Fatalf("unexpected status: %+v", sd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote telemetry on owner")
	}

	// The worker must NOT have delivered to itself (unicast to owner only).
	if len(worker.tp.Sent) != 1 {
		t.Fatalf("worker sent %d transport messages, want 1", len(worker.tp.Sent))
	}
}

func TestRemoteStreamReachesOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	owner := newNode(t, "node-A", true)
	worker := newNode(t, "node-B", false)
	owner.tp.Peer = worker.tp
	worker.tp.Peer = owner.tp

	owner.start(t, ctx)
	worker.start(t, ctx)

	const planID = "plan-X"
	owner.monitor.Register(planID)
	frames, err := owner.monitor.Open(planID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Worker streams a chunk owned by node-A via the stream protocol.
	err = worker.router.Send(ctx, APITaskmsg.StreamChunkMsg{
		PlanID: APITask.TaskID(planID), Delta: "hello", Owner: "node-A",
	})
	if err != nil {
		t.Fatalf("send chunk: %v", err)
	}

	select {
	case f := <-frames:
		if f.Event != APISteam.SSETypeStream {
			t.Fatalf("frame event = %s, want stream", f.Event)
		}
		sd, ok := f.Data.(APISteam.StreamData)
		if !ok {
			t.Fatalf("frame data not StreamData: %T", f.Data)
		}
		if sd.Delta != "hello" {
			t.Fatalf("delta = %q, want hello", sd.Delta)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote stream chunk on owner")
	}
}

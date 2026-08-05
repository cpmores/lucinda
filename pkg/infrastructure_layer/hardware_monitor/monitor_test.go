package monitor

import (
	"context"
	"sync"
	"testing"
	"time"

	APIHardware "github.com/cpmores/lucinda/api/v1/domain/hardware"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

func newTestMonitor(t *testing.T, repeatSec int64) (*monitor, eventbus.EventBus) {
	t.Helper()
	mm := modulemanager.NewModuleManager()

	eb := eventbus.NewInMemoryEventBus(logger.Discard())
	if err := eb.RegisterWithManager(mm); err != nil {
		t.Fatalf("eventbus register: %v", err)
	}

	m := NewHardwareMonitor(repeatSec, logger.Discard())
	if err := m.RegisterWithManager(mm); err != nil {
		t.Fatalf("monitor register: %v", err)
	}

	if err := m.DependsEnable(); err != nil {
		t.Fatalf("DependsEnable: %v", err)
	}
	return m, eb
}

func TestNewHardwareMonitor(t *testing.T) {
	m, _ := newTestMonitor(t, 10)
	if m.IsStarted {
		t.Fatal("monitor should not be started after New")
	}
	if m.RepeatSec != 10 {
		t.Fatalf("expected RepeatSec=10, got %d", m.RepeatSec)
	}
	snap := m.Snapshot()
	if snap.Timestamp != 0 {
		t.Fatal("expected zero timestamp before Start")
	}
}

func TestStartStop(t *testing.T) {
	m, _ := newTestMonitor(t, 60)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !m.IsStarted {
		t.Fatal("IsStarted should be true after Start")
	}

	snap := m.Snapshot()
	if snap.Timestamp == 0 {
		t.Fatal("expected non-zero timestamp after Start")
	}
	if snap.CPUSnapshot.Cores <= 0 {
		t.Fatalf("expected cores > 0, got %d", snap.CPUSnapshot.Cores)
	}
	if snap.MemorySnapshot.TotalBytes <= 0 {
		t.Fatal("expected total memory > 0")
	}

	cancel()
	time.Sleep(100 * time.Millisecond)
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.IsStarted {
		t.Fatal("IsStarted should be false after Stop")
	}
}

func TestDoubleStartError(t *testing.T) {
	m, _ := newTestMonitor(t, 60)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := m.Start(ctx); err == nil {
		t.Fatal("expected error on double Start, got nil")
	}
	cancel()
	time.Sleep(100 * time.Millisecond)
	m.Stop()
}

func TestStopBeforeStartError(t *testing.T) {
	m, _ := newTestMonitor(t, 10)
	if err := m.Stop(); err == nil {
		t.Fatal("expected error stopping before start, got nil")
	}
}

func TestSnapshotUpdatesOverTime(t *testing.T) {
	m, _ := newTestMonitor(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
		m.Stop()
	}()

	first := m.Snapshot()
	if first.Timestamp == 0 {
		t.Fatal("first snapshot has no timestamp")
	}

	time.Sleep(1500 * time.Millisecond)

	second := m.Snapshot()
	if second.Timestamp <= first.Timestamp {
		t.Fatalf("expected timestamp to advance: first=%d second=%d", first.Timestamp, second.Timestamp)
	}
}

func TestSnapshotBeforeStartReturnsZero(t *testing.T) {
	m, _ := newTestMonitor(t, 10)
	snap := m.Snapshot()
	if snap.Timestamp != 0 {
		t.Fatal("expected zero timestamp before Start")
	}
	if snap.CPUSnapshot.Cores != 0 {
		t.Fatal("expected zero cores before Start")
	}
	if snap.MemorySnapshot.TotalBytes != 0 {
		t.Fatal("expected zero memory before Start")
	}
}

func TestConcurrentSnapshot(t *testing.T) {
	m, _ := newTestMonitor(t, 60)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
		m.Stop()
	}()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.Snapshot()
			}
		}()
	}
	wg.Wait()
}

func TestStartCancelledContext(t *testing.T) {
	m, _ := newTestMonitor(t, 60)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start should not error on cancelled context: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	snap := m.Snapshot()
	if snap.Timestamp == 0 {
		t.Fatal("expected initial snapshot after Start with cancelled context")
	}

	m.Stop()
}

func TestMemoryValuesAreReasonable(t *testing.T) {
	m, _ := newTestMonitor(t, 60)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
		m.Stop()
	}()

	snap := m.Snapshot()
	if snap.MemorySnapshot.TotalBytes <= 0 {
		t.Fatal("total memory should be > 0")
	}
	if snap.MemorySnapshot.FreeBytes <= 0 {
		t.Fatal("free memory should be > 0")
	}
	if snap.MemorySnapshot.UsedBytes <= 0 {
		t.Fatal("used memory should be > 0")
	}
	if snap.MemorySnapshot.UsedBytes >= snap.MemorySnapshot.TotalBytes {
		t.Fatalf("used (%d) should be < total (%d)",
			snap.MemorySnapshot.UsedBytes, snap.MemorySnapshot.TotalBytes)
	}
}

func TestPublishesHardwareChangedEvent(t *testing.T) {
	m, eb := newTestMonitor(t, 60)

	ch := eb.Subscribe(APIEvent.HardwareChanged, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
		m.Stop()
	}()

	select {
	case event := <-ch:
		if event.Type != APIEvent.HardwareChanged {
			t.Fatalf("expected HardwareChanged event, got %s", event.Type)
		}
		if _, ok := event.Data.(APIHardware.HardwareSnapshot); !ok {
			t.Fatalf("expected HardwareSnapshot data, got %T", event.Data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for HardwareChanged event")
	}
}

func TestMonitorRegisterDependsOnEventBus(t *testing.T) {
	mm := modulemanager.NewModuleManager()

	eb := eventbus.NewInMemoryEventBus(logger.Discard())
	if err := eb.RegisterWithManager(mm); err != nil {
		t.Fatalf("eventbus register: %v", err)
	}

	m := NewHardwareMonitor(10, logger.Discard())
	if err := m.RegisterWithManager(mm); err != nil {
		t.Fatalf("monitor register: %v", err)
	}

	if err := mm.VerifyInit(); err != nil {
		t.Fatalf("VerifyInit should pass when deps registered: %v", err)
	}

	if err := mm.EnableDeps(); err != nil {
		t.Fatalf("EnableDeps: %v", err)
	}
	if m.eventBus == nil {
		t.Fatal("expected eventBus resolved after EnableDeps")
	}
}

func TestMonitorDependsEnableResolves(t *testing.T) {
	mm := modulemanager.NewModuleManager()

	eb := eventbus.NewInMemoryEventBus(logger.Discard())
	if err := eb.RegisterWithManager(mm); err != nil {
		t.Fatalf("eventbus register: %v", err)
	}

	m := NewHardwareMonitor(10, logger.Discard())
	if err := m.RegisterWithManager(mm); err != nil {
		t.Fatalf("monitor register: %v", err)
	}

	// DependsOn declares which instance name each dependency resolves to.
	if got := m.DependsOn()[APIModule.EventBus]; got != "default" {
		t.Fatalf("expected eventbus dep name \"default\", got %q", got)
	}

	if err := m.DependsEnable(); err != nil {
		t.Fatalf("DependsEnable: %v", err)
	}
	if m.eventBus == nil {
		t.Fatal("expected eventBus resolved")
	}
}

func TestMonitorDependsEnableFailsWhenMissing(t *testing.T) {
	mm := modulemanager.NewModuleManager()

	m := NewHardwareMonitor(10, logger.Discard())
	if err := m.RegisterWithManager(mm); err != nil {
		t.Fatalf("monitor register: %v", err)
	}

	if err := m.DependsEnable(); err == nil {
		t.Fatal("expected DependsEnable to fail when eventbus is not registered")
	}
}

func TestMonitorVerifyInitFailsWithoutEventBus(t *testing.T) {
	mm := modulemanager.NewModuleManager()

	m := NewHardwareMonitor(10, logger.Discard())
	if err := m.RegisterWithManager(mm); err != nil {
		t.Fatalf("monitor register: %v", err)
	}

	if err := mm.VerifyInit(); err == nil {
		t.Fatal("expected VerifyInit to fail when eventbus is not registered")
	}
}

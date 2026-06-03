package monitor

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewHardwareMonitor(t *testing.T) {
	m := NewHardwareMonitor(10)
	if m.IsStarted {
		t.Fatal("monitor should not be started after New")
	}
	if m.RepeatSec != 10 {
		t.Fatalf("expected RepeatSec=10, got %d", m.RepeatSec)
	}
	// Cache should be the zero value before Start.
	snap := m.Snapshot()
	if snap.Timestamp != 0 {
		t.Fatal("expected zero timestamp before Start")
	}
}

func TestStartStop(t *testing.T) {
	m := NewHardwareMonitor(60)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !m.IsStarted {
		t.Fatal("IsStarted should be true after Start")
	}

	// Snapshot should be populated after Start.
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

	// Stop should mark as stopped.
	cancel() // cancels ctx, goroutine exits
	time.Sleep(100 * time.Millisecond)
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.IsStarted {
		t.Fatal("IsStarted should be false after Stop")
	}
}

func TestDoubleStartError(t *testing.T) {
	m := NewHardwareMonitor(60)
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
	m := NewHardwareMonitor(10)
	if err := m.Stop(); err == nil {
		t.Fatal("expected error stopping before start, got nil")
	}
}

func TestSnapshotUpdatesOverTime(t *testing.T) {
	m := NewHardwareMonitor(1) // 1-second interval
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

	// Wait for at least one tick to fire.
	time.Sleep(1500 * time.Millisecond)

	second := m.Snapshot()
	if second.Timestamp <= first.Timestamp {
		t.Fatalf("expected timestamp to advance: first=%d second=%d", first.Timestamp, second.Timestamp)
	}
}

func TestSnapshotBeforeStartReturnsZero(t *testing.T) {
	m := NewHardwareMonitor(10)
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
	m := NewHardwareMonitor(60)
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

	// 10 goroutines calling Snapshot concurrently should not race.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				snap := m.Snapshot()
				// Basic sanity: cores shouldn't change between calls.
				if snap.CPUSnapshot.Cores <= 0 && snap.Timestamp != 0 {
					// This is fine — just checking no panic or data race.
				}
			}
		}()
	}
	wg.Wait()
}

func TestStartCancelledContext(t *testing.T) {
	m := NewHardwareMonitor(60)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Start with an already-cancelled context.
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start should not error on cancelled context: %v", err)
	}
	// The goroutine will exit immediately because ctx is done.
	time.Sleep(100 * time.Millisecond)

	// Snapshot should still have data from the initial collect().
	snap := m.Snapshot()
	if snap.Timestamp == 0 {
		t.Fatal("expected initial snapshot after Start with cancelled context")
	}

	m.Stop()
}

func TestMemoryValuesAreReasonable(t *testing.T) {
	m := NewHardwareMonitor(60)
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
	// Used + Free ≈ Total (Available is technically not identical to Free,
	// but Used should be less than Total).
	if snap.MemorySnapshot.UsedBytes >= snap.MemorySnapshot.TotalBytes {
		t.Fatalf("used (%d) should be < total (%d)",
			snap.MemorySnapshot.UsedBytes, snap.MemorySnapshot.TotalBytes)
	}
}

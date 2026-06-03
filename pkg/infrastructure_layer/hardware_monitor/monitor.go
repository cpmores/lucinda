// Package monitor
package monitor

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	APIHardware "github.com/cpmores/lucinda/api/v1/hardware"
)

// HardwareMonitor defines the interface for monitoring hardware resources.
type HardwareMonitor interface {
	Start(ctx context.Context) error
	Stop() error
	Snapshot() APIHardware.HardwareSnapshot
}

type monitor struct {
	sync.RWMutex
	IsStarted     bool
	RepeatSec     int64
	Cache         APIHardware.HardwareSnapshot
	LastTimestamp int64
}

// NewHardwareMonitor creates a new instance of the hardware monitor.
// returns a empty monitor with no data, Start() must be called to begin monitoring and populating the cache.
func NewHardwareMonitor(repeatSec int64) monitor {
	return monitor{
		IsStarted: false,
		RepeatSec: repeatSec,
	}
}

// Start begins the hardware monitoring process.
// It takes an immediate snapshot to prime the cache, then polls at the
// configured RepeatSec interval. The goroutine exits when ctx is cancelled.
func (m *monitor) Start(ctx context.Context) error {
	m.Lock()
	if m.IsStarted {
		m.Unlock()
		return fmt.Errorf("monitor is already started")
	}
	m.IsStarted = true
	m.Unlock()

	// Prime the CPU cache with a blocking read so subsequent non-blocking
	// calls to cpu.PercentWithContext(..., 0, ...) return useful data.
	_, _ = cpu.PercentWithContext(ctx, 1*time.Second, false)

	// Take the initial snapshot immediately.
	m.collect()

	ticker := time.NewTicker(time.Duration(m.RepeatSec) * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("hardware monitor: context done, stopping polling")
				return
			case <-ticker.C:
				m.collect()
			}
		}
	}()

	log.Printf("hardware monitor: started (interval=%ds, cores=%d)", m.RepeatSec, m.Snapshot().CPUSnapshot.Cores)
	return nil
}

// Stop marks the monitor as stopped. The polling goroutine exits via context
// cancellation, so the caller should cancel the context passed to Start first.
func (m *monitor) Stop() error {
	m.Lock()
	defer m.Unlock()
	if !m.IsStarted {
		return fmt.Errorf("monitor is not started")
	}
	m.IsStarted = false
	log.Println("hardware monitor: stopped")
	return nil
}

// Snapshot returns the latest cached hardware snapshot.
// It is safe to call concurrently from multiple goroutines.
func (m *monitor) Snapshot() APIHardware.HardwareSnapshot {
	m.RLock()
	defer m.RUnlock()
	return m.Cache
}

// ============================================================
//                          utils
// ============================================================

// collect gathers fresh CPU, memory, and GPU telemetry, updates the cache,
// and publishes a HardwareChanged event if the delta is significant.
// Caller does not hold the lock — collect acquires it internally.
func (m *monitor) collect() {
	now := time.Now().Unix()
	ctx := context.Background()

	// ── CPU ──────────────────────────────────────────────────────────
	cores := runtime.NumCPU()
	var usagePct float64
	pcts, err := cpu.PercentWithContext(ctx, 0, false) // 0 = return cached delta
	if err != nil {
		log.Printf("hardware monitor: cpu.PercentWithContext failed: %s", err)
	} else if len(pcts) > 0 {
		usagePct = pcts[0]
	}

	// ── Memory ───────────────────────────────────────────────────────
	var memSnap APIHardware.MemorySnapshot
	vmem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		log.Printf("hardware monitor: mem.VirtualMemoryWithContext failed: %s", err)
	} else {
		memSnap = APIHardware.MemorySnapshot{
			TotalBytes: int64(vmem.Total),
			FreeBytes:  int64(vmem.Available),
			UsedBytes:  int64(vmem.Used),
		}
	}

	// ── GPU ──────────────────────────────────────────────────────────
	// HACK: collect GPU telemetry from Ollama /api/ps and/or NVML.
	// For now this stays nil until the monitor is given an Ollama endpoint.
	// After that, we get a complete GPUSnapshot from ProviderController
	// Not includes it, but combined on higher level

	// ── Build & Cache ────────────────────────────────────────────────
	snap := APIHardware.HardwareSnapshot{
		Timestamp: now,
		CPUSnapshot: APIHardware.CPUSnapshot{
			Cores:    cores,
			UsagePct: usagePct,
		},
		MemorySnapshot: memSnap,
		GPUSnapshot:    nil,
	}

	m.Lock()
	prev := m.Cache
	m.Cache = snap
	m.Unlock()

	if APIHardware.SignificantChange(prev, snap) {
		log.Printf("hardware monitor: significant change detected — cpu=%.1f%% mem=%d/%d",
			snap.CPUSnapshot.UsagePct, snap.MemorySnapshot.FreeBytes, snap.MemorySnapshot.TotalBytes)
	}
}

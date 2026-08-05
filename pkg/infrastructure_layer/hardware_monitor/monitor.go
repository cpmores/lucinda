// Package monitor
package monitor

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	APIHardware "github.com/cpmores/lucinda/api/v1/domain/hardware"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// HardwareMonitor defines the interface for monitoring hardware resources.
type HardwareMonitor interface {
	Start(ctx context.Context) error
	Stop() error
	Snapshot() APIHardware.HardwareSnapshot
}

type monitor struct {
	sync.RWMutex
	eventBus      eventbus.EventBus // resolved via DependsEnable
	mm            modulemanager.ModuleManager
	log           *logger.Logger
	IsStarted     bool
	RepeatSec     int64
	Cache         APIHardware.HardwareSnapshot
	LastTimestamp int64
}

// NewHardwareMonitor creates a new instance of the hardware monitor.
func NewHardwareMonitor(repeatSec int64, log *logger.Logger) *monitor {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created", "interval_sec", repeatSec)
	return &monitor{
		log:       log,
		IsStarted: false,
		RepeatSec: repeatSec,
	}
}

// Start begins the hardware monitoring process.
func (m *monitor) Start(ctx context.Context) error {
	m.Lock()
	if m.IsStarted {
		m.Unlock()
		return fmt.Errorf("monitor is already started")
	}
	m.IsStarted = true
	m.Unlock()

	_, _ = cpu.PercentWithContext(ctx, 1*time.Second, false)
	m.collect()

	ticker := time.NewTicker(time.Duration(m.RepeatSec) * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				m.log.Info("context done, stopping polling")
				return
			case <-ticker.C:
				m.collect()
			}
		}
	}()

	m.log.Info("started", "interval_sec", m.RepeatSec, "cores", m.Snapshot().CPUSnapshot.Cores)
	return nil
}

// Stop marks the monitor as stopped.
func (m *monitor) Stop() error {
	m.Lock()
	defer m.Unlock()
	if !m.IsStarted {
		return fmt.Errorf("monitor is not started")
	}
	m.IsStarted = false
	m.log.Info("stopped")
	return nil
}

// Snapshot returns the latest cached hardware snapshot.
func (m *monitor) Snapshot() APIHardware.HardwareSnapshot {
	m.RLock()
	defer m.RUnlock()
	return m.Cache
}

// collect gathers fresh CPU and memory telemetry, updates the cache,
// and publishes a HardwareChanged event if the delta is significant.
func (m *monitor) collect() {
	now := time.Now().Unix()
	ctx := context.Background()

	// CPU
	cores := runtime.NumCPU()
	var usagePct float64
	pcts, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		m.log.Error("cpu.PercentWithContext failed", "err", err)
	} else if len(pcts) > 0 {
		usagePct = pcts[0]
	}

	// Memory
	var memSnap APIHardware.MemorySnapshot
	vmem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		m.log.Error("mem.VirtualMemoryWithContext failed", "err", err)
	} else {
		memSnap = APIHardware.MemorySnapshot{
			TotalBytes: int64(vmem.Total),
			FreeBytes:  int64(vmem.Available),
			UsedBytes:  int64(vmem.Used),
		}
	}

	// GPU: collected by ProviderController, merged at higher level

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
		m.log.Info("significant change detected",
			"cpu_pct", snap.CPUSnapshot.UsagePct,
			"mem_free", snap.MemorySnapshot.FreeBytes,
			"mem_total", snap.MemorySnapshot.TotalBytes)
		m.eventBus.Publish(APIEvent.HardwareChanged,
			APIEvent.NewEvent(APIEvent.HardwareChanged, snap))
	}
}

// ── AvailableModule Interface ──────────────────────────────────────────────────────────

func (m *monitor) GetModuleType() APIModule.ModuleType {
	return APIModule.HardwareMonitor
}

func (m *monitor) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(m.GetModuleType(), "default")
}

func (m *monitor) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(m.GetModuleID(), m.GetModuleType(), APIModule.Running)
}

func (m *monitor) RegisterWithManager(manager modulemanager.ModuleManager) error {
	m.mm = manager
	return manager.Register(m)
}

func (m *monitor) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{APIModule.EventBus: "default"}
}

// DependsEnable resolves every dependency declared in DependsOn() from the
// registry and stores it. Called by ModuleManager.EnableDeps after VerifyInit.
func (m *monitor) DependsEnable() error {
	for depType, name := range m.DependsOn() {
		id := APIModule.NewModuleID(depType, name)
		mod, err := m.mm.Get(id)
		if err != nil {
			return fmt.Errorf("resolve dependency %s: %w", id, err)
		}
		eb, ok := mod.(eventbus.EventBus)
		if !ok {
			return fmt.Errorf("dependency %s is not an EventBus", id)
		}
		m.eventBus = eb
	}
	return nil
}

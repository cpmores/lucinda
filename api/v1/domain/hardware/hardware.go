// Package apihardware provides the API for hardware information and snapshots.
// e.g. CPU, RAM, vRAM
// Info under this package is collected by HardwareMonitor and can be consumed by upper layers
package apihardware

type HardwareSnapshot struct {
	Timestamp      int64          `json:"timestamp"`
	CPUSnapshot    CPUSnapshot    `json:"cpu"`
	MemorySnapshot MemorySnapshot `json:"memory"`
	GPUSnapshot    []GPUSnapshot  `json:"gpu"`
}

type CPUSnapshot struct {
	Cores    int     `json:"cores"`
	UsagePct float64 `json:"usage_pct"`
}

type MemorySnapshot struct {
	TotalBytes int64 `json:"total_bytes"`
	FreeBytes  int64 `json:"free_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
}

type GPUSnapshot struct {
	Device    string `json:"device"`
	TotalVRAM int64  `json:"total_vram"`
	FreeVRAM  int64  `json:"free_vram"`
	UsedVRAM  int64  `json:"used_vram"`
}

// SignificantChange returns true when two snapshots differ enough to
// warrant broadcasting a hardware-changed event. Thresholds are tuned to
// avoid spamming the EventBus on minor fluctuations.
func SignificantChange(prev, curr HardwareSnapshot) bool {
	// First snapshot is always significant.
	if prev.Timestamp == 0 {
		return true
	}

	// CPU usage shifted by more than 10 percentage points.
	if absDiff(prev.CPUSnapshot.UsagePct, curr.CPUSnapshot.UsagePct) > 10.0 {
		return true
	}

	// Free memory changed by more than 5%.
	if prev.MemorySnapshot.FreeBytes > 0 {
		pctChange := absDiff(float64(prev.MemorySnapshot.FreeBytes), float64(curr.MemorySnapshot.FreeBytes)) /
			float64(prev.MemorySnapshot.FreeBytes)
		if pctChange > 0.05 {
			return true
		}
	}

	return false
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

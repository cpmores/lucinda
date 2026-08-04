// Package apicapability defines the CapabilityCV type used by peers to bid on tasks.
package apicapability

import (
	APIHardware "github.com/cpmores/lucinda/api/v1/hardware"
	APITask "github.com/cpmores/lucinda/api/v1/task"
)

// CapabilityCV is a peer's self-reported capability profile, sent with every
// TaskRequestMsg. The TaskBoard scores it against TaskSpec requirements.
type CapabilityCV struct {
	TaskID   APITask.TaskID               `json:"task_id"`
	PeerID   string                       `json:"peer_id"`
	Hardware APIHardware.HardwareSnapshot `json:"hardware"`
	Models   []string                     `json:"models"` // e.g. ["gemma3", "sd-xl"]
	Tools    []string                     `json:"tools"`  // e.g. ["web_search", "image_gen"]
	Labels   []string                     `json:"labels"` // e.g. ["gpu", "vision"]
}

// Match returns a score when this peer qualifies, or -1 if disqualified.
func (cv *CapabilityCV) Match(spec *APITask.TaskSpec) int {
	// VRAM check.
	if spec.MinVRAM > 0 {
		var freeVRAM int64
		for _, g := range cv.Hardware.GPUSnapshot {
			freeVRAM += g.FreeVRAM
		}
		if freeVRAM < spec.MinVRAM {
			return -1
		}
	}

	// Model check — skip if not specified (any model is fine).
	if spec.Model != "" && len(cv.Models) > 0 && !contains(cv.Models, spec.Model) {
		return -1
	}

	// Tool check — skip if CV has no tools (toolbox not implemented yet).
	// Once tools are available, all required tools must be present.
	if len(cv.Tools) > 0 {
		for _, t := range spec.Tools {
			if !contains(cv.Tools, t) {
				return -1
			}
		}
	}

	// Label check — skip if CV has no labels (not configured yet).
	if len(cv.Labels) > 0 && len(spec.Labels) > 0 && !anyOverlap(cv.Labels, spec.Labels) {
		return -1
	}

	// Score: free memory in GB, plus priority discount.
	score := int(cv.Hardware.MemorySnapshot.FreeBytes / (1024 * 1024 * 1024))
	score -= spec.Priority
	return score
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}

	return false
}

func anyOverlap(have, need []string) bool {
	for _, n := range need {
		if contains(have, n) {
			return true
		}
	}
	return false
}

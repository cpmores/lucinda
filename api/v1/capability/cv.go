// Package apicapability defines the CapabilityCV type used by peers to bid on tasks.
package apicapability

import (
	apihardware "github.com/cpmores/lucinda/api/v1/hardware"
	apitask "github.com/cpmores/lucinda/api/v1/task"
)

// CapabilityCV is a peer's self-reported capability profile, sent with every
// TaskRequestMsg. The TaskBoard scores it against TaskSpec requirements.
type CapabilityCV struct {
	TaskID   apitask.TaskID               `json:"task_id"`
	PeerID   string                       `json:"peer_id"`
	Hardware apihardware.HardwareSnapshot `json:"hardware"`
	Models   []string                     `json:"models"` // e.g. ["gemma3", "sd-xl"]
	Tools    []string                     `json:"tools"`  // e.g. ["web_search", "image_gen"]
	Labels   []string                     `json:"labels"` // e.g. ["gpu", "vision"]
}

// Match returns a score when this peer qualifies, or -1 if disqualified.
func (cv *CapabilityCV) Match(spec *apitask.TaskSpec) int {
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

	// Model check — must have the requested model available.
	if spec.Model != "" && !contains(cv.Models, spec.Model) {
		return -1
	}

	// Tool check — all required tools must be present.
	for _, t := range spec.Tools {
		if !contains(cv.Tools, t) {
			return -1
		}
	}

	// Label check — at least one matching label or no labels required.
	if len(spec.Labels) > 0 && !anyOverlap(cv.Labels, spec.Labels) {
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

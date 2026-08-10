// Package apicapability defines the CapabilityCV type peers send to bid on
// task advertisements, and the scoring used by the TaskBoard to pick the
// best executor.
package apicapability

import (
	"slices"

	APIHardware "github.com/cpmores/lucinda/api/v1/domain/hardware"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
)

// CapabilityCV is a peer's self-reported capability profile, sent with every
// TaskCVMsg. The TaskBoard scores it against a TaskSpec to pick an executor.
type CapabilityCV struct {
	TaskID   APITask.TaskID               `json:"task_id"`
	PeerID   string                       `json:"peer_id"`
	Hardware APIHardware.HardwareSnapshot `json:"hardware"`
	Models   []string                     `json:"models"` // e.g. ["qwen-2.5-gptq", "sd-xl"]
	Tools    []string                     `json:"tools"`  // e.g. ["web_search", "image_gen"]
	Labels   []string                     `json:"labels"` // e.g. ["gpu", "vision"]
}

type BestScore struct {
	Best  *CapabilityCV
	Score int
}

func Contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}

func AnyOverlap(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

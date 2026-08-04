package taskreducer

import (
	"testing"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
)

func TestParentPlanID(t *testing.T) {
	tests := []struct {
		nodeID   APITask.TaskID
		expected APITask.TaskID
	}{
		{"plan-7a3f-search", "plan-7a3f"},
		{"plan-7a3f-reduce", "plan-7a3f"},
		{"plana7a3f", "plana7a3f"}, // no dash
		{"plan-7a3f", "plan-7a3f"}, // parent itself
	}
	for _, tc := range tests {
		got := parentPlanID(tc.nodeID)
		if got != tc.expected {
			t.Fatalf("parentPlanID(%s) = %s, want %s", tc.nodeID, got, tc.expected)
		}
	}
}

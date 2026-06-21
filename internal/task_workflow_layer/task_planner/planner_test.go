package taskplanner

import (
	"testing"

	APITask "github.com/cpmores/lucinda/api/v1/task"
)

func TestExtractJSON(t *testing.T) {
	raw := `Some text {"nodes": [{"id":"a","prompt":"hi","tools":[],"labels":["cpu"],"deps":[]}]} more text`
	result := extractJSON(raw)
	if result[0] != '{' || result[len(result)-1] != '}' {
		t.Fatalf("expected JSON object, got: %s", result)
	}
}

func TestExtractJSONNoBrackets(t *testing.T) {
	if result := extractJSON("plain text"); result != "plain text" {
		t.Fatalf("expected raw text back, got: %s", result)
	}
}

func TestBuildDecomposition(t *testing.T) {
	prompt := buildDecomposition("find the richest man")
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
}

func TestParseValidJSON(t *testing.T) {
	p := &planner{}
	raw := `{"nodes": [
		{"id":"search","prompt":"search web","tools":["web_search"],"labels":["cpu"],"deps":[]},
		{"id":"summarize","prompt":"summarize","tools":[],"labels":["cpu"],"deps":["search"]}
	]}`
	plan := p.parse("plan-1", "owner-1", raw)

	if plan.ID != "plan-1" {
		t.Fatalf("expected plan-1, got %s", plan.ID)
	}
	if len(plan.Nodes) != 3 { // 2 from JSON + 1 reduce
		t.Fatalf("expected 3 nodes (2 + reduce), got %d", len(plan.Nodes))
	}
	if len(plan.Roots) != 1 || plan.Roots[0] != "plan-1-search" {
		t.Fatalf("expected root plan-1-search, got %v", plan.Roots)
	}

	// Check reduce node exists.
	reduceID := APITask.TaskID("plan-1-reduce")
	if _, ok := plan.Nodes[reduceID]; !ok {
		t.Fatal("reduce node not appended")
	}
	if plan.Nodes[reduceID].Spec.Stage != APITask.StageReduce {
		t.Fatal("reduce node missing StageReduce")
	}
	// Reduce should depend on summarize (the only leaf besides itself).
	if plan.PredecessorNums[reduceID] != 1 {
		t.Fatalf("reduce should have 1 predecessor, got %d", plan.PredecessorNums[reduceID])
	}
}

func TestFallbackPlan(t *testing.T) {
	p := &planner{}
	plan := p.parse("plan-1", "owner", "not json at all")

	if len(plan.Nodes) != 1 {
		t.Fatalf("fallback should have 1 node, got %d", len(plan.Nodes))
	}
}

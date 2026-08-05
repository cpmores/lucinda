package apiprovider

import "testing"

func testModel() ModelInfo {
	return ModelInfo{
		ID: "qwen-2.5-gptq",
		Labels: map[string]string{
			"modality": "text",
			"employer": "TaskPlanner,TaskCommander,TaskExecutor",
		},
		ParamsB:       7,
		ContextTokens: 2048,
		MinVRAM:       16 << 30,
	}
}

// A comma-separated label (employer) behaves as a set: In matches a member.
func TestMatchExpressionInOnCommaList(t *testing.T) {
	m := testModel()

	e := MatchExpression{Key: "employer", Operator: "In", Values: []string{"TaskPlanner"}}
	if !e.Matches(m) {
		t.Fatal("expected employer In [TaskPlanner] to match")
	}

	e = MatchExpression{Key: "employer", Operator: "In", Values: []string{"TaskReducer"}}
	if e.Matches(m) {
		t.Fatal("expected employer In [TaskReducer] to NOT match")
	}
}

func TestMatchExpressionOperators(t *testing.T) {
	m := testModel()

	cases := []struct {
		expr MatchExpression
		want bool
	}{
		{MatchExpression{Key: "modality", Operator: "In", Values: []string{"text"}}, true},
		{MatchExpression{Key: "modality", Operator: "NotIn", Values: []string{"image"}}, true},
		{MatchExpression{Key: "modality", Operator: "Exists"}, true},
		{MatchExpression{Key: "missing", Operator: "Exists"}, false},
		{MatchExpression{Key: "missing", Operator: "DoesNotExist"}, true},
		{MatchExpression{Key: "params_b", Operator: "Gt", Values: []string{"5"}}, true},
		{MatchExpression{Key: "params_b", Operator: "Lt", Values: []string{"10"}}, true},
		{MatchExpression{Key: "params_b", Operator: "Gt", Values: []string{"70"}}, false},
		{MatchExpression{Key: "min_vram", Operator: "Gt", Values: []string{"10737418240"}}, true}, // >10GiB
	}
	for i, c := range cases {
		if got := c.expr.Matches(m); got != c.want {
			t.Fatalf("case %d: %+v -> got %v, want %v", i, c.expr, got, c.want)
		}
	}
}

// ModelFilter matches if ANY required term matches (OR); a term matches only
// if ALL its expressions match (AND).
func TestModelFilterRequired(t *testing.T) {
	m := testModel()

	// OR: one term matches, one doesn't -> overall match.
	f := ModelFilter{
		Required: []Term{
			{MatchExpression: []MatchExpression{{Key: "modality", Operator: "In", Values: []string{"image"}}}},
			{MatchExpression: []MatchExpression{{Key: "employer", Operator: "In", Values: []string{"TaskPlanner"}}}},
		},
	}
	if !f.Matches(m) {
		t.Fatal("expected OR of terms to match when one term matches")
	}

	// AND within a term: both expressions must match.
	f2 := ModelFilter{
		Required: []Term{{
			MatchExpression: []MatchExpression{
				{Key: "modality", Operator: "In", Values: []string{"text"}},
				{Key: "employer", Operator: "In", Values: []string{"TaskPlanner"}},
			},
		}},
	}
	if !f2.Matches(m) {
		t.Fatal("expected AND within term to match")
	}

	// AND broken: second expression fails.
	f3 := ModelFilter{
		Required: []Term{{
			MatchExpression: []MatchExpression{
				{Key: "modality", Operator: "In", Values: []string{"text"}},
				{Key: "employer", Operator: "In", Values: []string{"TaskReducer"}},
			},
		}},
	}
	if f3.Matches(m) {
		t.Fatal("expected term with one failing expression to NOT match")
	}

	// Empty Required -> no hard constraint.
	if !(ModelFilter{}).Matches(m) {
		t.Fatal("expected empty Required to match everything")
	}
}

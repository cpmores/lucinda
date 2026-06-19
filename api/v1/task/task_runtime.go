package apitask

// TaskSpec describes what to execute and the resources needed.
// Used by the TaskBoard for Capability CV matching and by the TaskExecutor to run the node.
type TaskSpec struct {
	// ── Execution ──────────────────────────────────────────────────────────
	Model  string `json:"model"`

	// ── Resource requirements — matched against Capability CV ─────────────
	MinVRAM      int64    `json:"min_vram"`
	BudgetTokens int      `json:"budget_tokens"`
	Tools        []string `json:"tools,omitempty"`
	Labels       []string `json:"labels,omitempty"`

	// ── Scheduling hint ────────────────────────────────────────────────────
	Priority int `json:"priority"` // 0 = highest

	// ── Timeline ──────────────────────────────────────────────────────────
	Deadline  int64 `json:"deadline"` // Unix timestamp in seconds
	CreatedAt int64 `json:"created_at"`
}

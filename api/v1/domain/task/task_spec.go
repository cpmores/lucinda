package apitask

// TaskKind selects how the executor runs a task. Default is execute.
type TaskKind string

const (
	TaskKindExecute    TaskKind = "execute"    // produce a work result via Generate
	TaskKindReason     TaskKind = "reason"     // produce structured LLM output (decision / decomposition)
	TaskKindSynthesize TaskKind = "synthesize" // produce the final answer via Stream
	TaskKindVerify     TaskKind = "verify"     // judge whether a produced output satisfies its goal (Generate)
)

// TaskSpec describes what to execute and the resources needed.
// Used by the TaskBoard for Capability CV matching and by the TaskExecutor to run the node.
type TaskSpec struct {
	// Kind selects the executor's provider behavior.
	Kind TaskKind `json:"kind,omitempty"`

	// ── Execution ──────────────────────────────────────────────────────────
	Prompt string `json:"prompt,omitempty"`
	Model  string `json:"model"`
	Output string `json:"output,omitempty"`

	// ── Resource requirements — matched against Capability CV ─────────────
	MinVRAM      int64    `json:"min_vram"`
	BudgetTokens int      `json:"budget_tokens"`
	Tools        []string `json:"tools,omitempty"`
	Labels       []string `json:"labels,omitempty"`

	// ── Scheduling hint ────────────────────────────────────────────────────
	Priority int   `json:"priority"` // 0 = highest
	Stage    Stage `json:"stage,omitempty"`

	// ── Timeline ──────────────────────────────────────────────────────────
	Deadline  int64 `json:"deadline"` // Unix timestamp in seconds
	CreatedAt int64 `json:"created_at"`
}

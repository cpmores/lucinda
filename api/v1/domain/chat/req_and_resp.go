package apichat

// ── Request and Response──────────────────────────────────────────────────────────

// Request is raw request from users
type Request struct {
	Prompt    string `json:"prompt"`
	Owner     string `json:"owner,omitempty"`
	ContextID string `json:"context_id,omitempty"`
}

// Response is raw response for users
type Response struct{}

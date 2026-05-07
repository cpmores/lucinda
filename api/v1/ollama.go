// github.com/cpmores/lucinda/api/v1/ollama.go
package api

import (
	"context"
	"time"
)

type OllamaRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  *OllamaOptions `json:"options,omitempty"`
}

type OllamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

type OllamaResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Message   Message   `json:"message"`
	Done      bool      `json:"done"`
	OllamaMetadata
}

// Ollama metadata
type OllamaMetadata struct {
	DoneReason         string `json:"done_reason"`
	TotalDuration      uint64 `json:"total_duration"`
	LoadDuration       uint64 `json:"load_duration"`
	PromptEvalCount    uint64 `json:"prompt_eval_count"`
	PromptEvalDuration uint64 `json:"prompt_eval_duration"`
	EvalCount          uint64 `json:"eval_count"`
	EvalDuration       uint64 `json:"eval_duration"`
}

func ConvertOllamaToChat(ctx context.Context, ollamaResp OllamaResponse) (*ChatResponse, error) {
	return &ChatResponse{
		Model:     ollamaResp.Model,
		CreatedAt: ollamaResp.CreatedAt,
		Message:   ollamaResp.Message,
		Done:      ollamaResp.Done,
		Provider:  "Ollama",
		Metadata:  ollamaResp.OllamaMetadata,
	}, nil
}

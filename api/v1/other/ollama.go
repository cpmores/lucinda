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

// Ollama ps response
type OllamaPsResponse struct {
	Models []OllamaLoadedModel `json:"models"`
}

// OllamaLoadedModel
type OllamaLoadedModel struct {
	Name          string       `json:"name"`
	Model         string       `json:"model"`
	Size          int64        `json:"size"`
	Digest        string       `json:"digest"`
	Details       ModelDetails `json:"details"`
	ExpiresAt     time.Time    `json:"expires_at"`
	SizeVRAM      int64        `json:"size_vram"`
	ContextLength int64        `json:"context_length"`
}

// ModelDetails
type ModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

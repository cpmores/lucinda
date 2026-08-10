// Package apichat
package apichat

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
	// Agent selects the execution architecture: "plan_execute" (default) or
	// "react". Propagated to TaskPlan.Architecture by the wrapper/planner.
	Agent   string       `json:"agent,omitempty"`
	Options ModelOptions `json:"options"`
}

type ChatMessage struct {
	Role    string        `json:"role"`
	Content []ContentPart `json:"content"`
}

type ContentType string

const (
	ContentText     ContentType = "text"
	ContentImageURL ContentType = "image_url"
)

type ContentPart struct {
	Type     ContentType `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL string      `json:"image_url,omitempty"`
}

func NewTextContent(text string) ContentPart {
	return ContentPart{Type: ContentText, Text: text}
}

func NewImageContent(url string) ContentPart {
	return ContentPart{Type: ContentImageURL, ImageURL: url}
}

type ModelOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

// StreamChunk is a single token delta from a streaming provider.
type StreamChunk struct {
	Delta    string `json:"delta"`
	Done     bool   `json:"done"`
	Metadata any    `json:"metadata,omitempty"`
}

// ChatResponse is the complete response from a non-streaming provider call.
type ChatResponse struct {
	ID       string      `json:"id"`
	Model    string      `json:"model"`
	Message  ChatMessage `json:"message"`
	Done     bool        `json:"done"`
	Usage    Usage       `json:"usage"`
	Metadata any         `json:"metadata,omitempty"`
}

// Usage tracks token consumption for a response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Package ollama implements the OllamaProvider,
// which is a provider that interacts with the Ollama API to provide AI services.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	APIHardware "github.com/cpmores/lucinda/api/v1/hardware"
	APIProvider "github.com/cpmores/lucinda/api/v1/provider"

	APIChat "github.com/cpmores/lucinda/api/v1/chat"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/provider/drivers"
)

const DriverName = "ollama"

// ── Wire types ────────────────────────────────────────────────────────

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  *ollamaOptions  `json:"options,omitempty"`
	Images   []string        `json:"images,omitempty"`
}

type ollamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Model           string        `json:"model"`
	CreatedAt       time.Time     `json:"created_at"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	EvalCount       int           `json:"eval_count"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	TotalDuration   int64         `json:"total_duration"`
}

// ── Struct ────────────────────────────────────────────────────────────

type OllamaProvider struct {
	id        string
	client    *http.Client
	baseURL   string
	config    *APIProvider.ProviderConfig
	models    []string
	createdAt int64
	eventID   int64
}

func NewOllamaProvider(config APIProvider.ProviderConfig) (*OllamaProvider, error) {
	if config.Driver != DriverName {
		return nil, fmt.Errorf("invalid driver: expected %s, got %s", DriverName, config.Driver)
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", config.Host, config.Port)
	}
	baseURL = strings.TrimRight(baseURL, "/")

	timeout := 30 * time.Second
	if config.Timeout > 0 {
		timeout = time.Duration(config.Timeout) * time.Second
	}

	return &OllamaProvider{
		id:        config.ID,
		config:    &config,
		client:    &http.Client{Timeout: timeout},
		baseURL:   baseURL,
		models:    config.Models,
		createdAt: time.Now().Unix(),
	}, nil
}

// ── Info ──────────────────────────────────────────────────────────────

func (p *OllamaProvider) GetID() string                     { return p.id }
func (p *OllamaProvider) GetType() APIProvider.ProviderType { return APIProvider.LOCAL }
func (p *OllamaProvider) GetModels() []string               { return p.models }
func (p *OllamaProvider) MaxContextTokens() int {
	if p.config.MaxContextTokens > 0 {
		return p.config.MaxContextTokens
	}
	return 2048
}
func (p *OllamaProvider) GetCreatedAt() int64               { return p.createdAt }

func (p *OllamaProvider) GetInfo() APIProvider.ProviderInfo {
	return APIProvider.ProviderInfo{
		ID:        p.id,
		Type:      APIProvider.LOCAL,
		Models:    p.models,
		CreatedAt: p.createdAt,
	}
}

// ── Health ────────────────────────────────────────────────────────────

func (p *OllamaProvider) Health() APIProvider.ProviderHealth {
	resp, err := p.client.Get(p.baseURL + "/")
	if err != nil {
		return p.errorHealth(err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return p.errorHealth(fmt.Sprintf("status %d", resp.StatusCode))
	}

	return APIProvider.ProviderHealth{
		ID:        p.nextEventID(),
		Status:    APIProvider.FREE,
		Timestamp: time.Now().Unix(),
	}
}

// ── GPU ───────────────────────────────────────────────────────────────

func (p *OllamaProvider) GPU() (APIHardware.GPUSnapshot, error) {
	resp, err := p.client.Get(p.baseURL + "/api/ps")
	if err != nil {
		return APIHardware.GPUSnapshot{}, fmt.Errorf("ps request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return APIHardware.GPUSnapshot{}, fmt.Errorf("ps returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return APIHardware.GPUSnapshot{}, err
	}

	var ps psResponse
	if err := json.Unmarshal(body, &ps); err != nil {
		return APIHardware.GPUSnapshot{}, fmt.Errorf("ps unmarshal: %w", err)
	}

	var usedVRAM int64
	models := make([]string, 0, len(ps.Models))
	for _, m := range ps.Models {
		models = append(models, m.Name)
		usedVRAM += m.SizeVRAM
	}

	return APIHardware.GPUSnapshot{
		Device:    "ollama",
		TotalVRAM: p.config.TotalVRAM,
		FreeVRAM:  p.config.TotalVRAM - usedVRAM,
		UsedVRAM:  usedVRAM,
	}, nil
}

type psResponse struct {
	Models []psModel `json:"models"`
}

type psModel struct {
	Name     string `json:"name"`
	SizeVRAM int64  `json:"size_vram"`
}

// ── Warm ──────────────────────────────────────────────────────────────

func (p *OllamaProvider) Warm(model string) error {
	req := &APIChat.ChatRequest{
		Model: model,
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: "warm"}},
		}},
		Options: APIChat.ModelOptions{MaxTokens: 1},
	}
	_, err := p.Generate(context.Background(), req)
	return err
}

// ── Generate ──────────────────────────────────────────────────────────

func (p *OllamaProvider) Generate(ctx context.Context, req *APIChat.ChatRequest) (*APIChat.ChatResponse, error) {
	ollamaReq := p.buildOllamaRequest(req, false)
	ollamaResp, err := p.doChat(ctx, ollamaReq)
	if err != nil {
		return nil, err
	}
	return p.toChatResponse(ollamaResp), nil
}

// ── Stream ────────────────────────────────────────────────────────────

func (p *OllamaProvider) Stream(ctx context.Context, req *APIChat.ChatRequest) (<-chan *APIChat.StreamChunk, error) {
	ollamaReq := p.buildOllamaRequest(req, true)
	resp, err := p.doChatStream(ctx, ollamaReq)
	if err != nil {
		return nil, err
	}

	ch := make(chan *APIChat.StreamChunk, 64)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var o ollamaResponse
			if err := json.Unmarshal([]byte(line), &o); err != nil {
				continue
			}
			select {
			case ch <- &APIChat.StreamChunk{Delta: o.Message.Content, Done: o.Done}:
			case <-ctx.Done():
				return
			}
			if o.Done {
				return
			}
		}
	}()
	return ch, nil
}

// ── Internal helpers ──────────────────────────────────────────────────

func (p *OllamaProvider) buildOllamaRequest(req *APIChat.ChatRequest, stream bool) ollamaRequest {
	messages := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = ollamaMessage{
			Role:    m.Role,
			Content: contentToText(m.Content),
			Images:  contentToImages(m.Content),
		}
	}

	or := ollamaRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   stream,
	}
	if req.Options.Temperature != 0 || req.Options.TopP != 0 || req.Options.MaxTokens != 0 {
		or.Options = &ollamaOptions{
			Temperature: req.Options.Temperature,
			TopP:        req.Options.TopP,
			NumPredict:  req.Options.MaxTokens,
		}
	}
	return or
}

func (p *OllamaProvider) doChat(ctx context.Context, req ollamaRequest) (*ollamaResponse, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(b))
	}

	var o ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&o); err != nil {
		return nil, fmt.Errorf("ollama decode: %w", err)
	}
	return &o, nil
}

func (p *OllamaProvider) doChatStream(ctx context.Context, req ollamaRequest) (*http.Response, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama stream returned %d: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}

func (p *OllamaProvider) toChatResponse(o *ollamaResponse) *APIChat.ChatResponse {
	return &APIChat.ChatResponse{
		ID:    p.nextEventID(),
		Model: o.Model,
		Message: APIChat.ChatMessage{
			Role:    o.Message.Role,
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: o.Message.Content}},
		},
		Done: o.Done,
		Usage: APIChat.Usage{
			PromptTokens:     o.PromptEvalCount,
			CompletionTokens: o.EvalCount,
			TotalTokens:      o.PromptEvalCount + o.EvalCount,
		},
	}
}

func (p *OllamaProvider) errorHealth(errMsg string) APIProvider.ProviderHealth {
	return APIProvider.ProviderHealth{
		ID:        p.nextEventID(),
		Status:    APIProvider.ERROR,
		Timestamp: time.Now().Unix(),
		Error:     errMsg,
	}
}

func (p *OllamaProvider) nextEventID() string {
	p.eventID++
	return fmt.Sprintf("%s-%d", p.id, p.eventID)
}

func contentToText(parts []APIChat.ContentPart) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Type == APIChat.ContentText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func init() {
	drivers.Register(DriverName, func(config APIProvider.ProviderConfig) (APIProvider.Provider, error) {
		return NewOllamaProvider(config)
	})
}

func contentToImages(parts []APIChat.ContentPart) []string {
	var images []string
	for _, part := range parts {
		if part.Type == APIChat.ContentImageURL {
			images = append(images, part.ImageURL)
		}
	}
	return images
}

// Package vllm implements the VLLMProvider backed by the vLLM OpenAI-compatible API.
package vllm

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

	APIChat "github.com/cpmores/lucinda/api/v1/chat"
	APIHardware "github.com/cpmores/lucinda/api/v1/hardware"
	APIProvider "github.com/cpmores/lucinda/api/v1/provider"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/provider/drivers"
)

const DriverName = "vllm"

// ── OpenAI-compatible wire types ──────────────────────────────────────

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string    `json:"role"`
	Content []content `json:"content"`
}

type chatResponseMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string for text, []content for multimodal
}

type content struct {
	Type     string    `json:"type"` // "text" or "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type chatCompletionResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Index   int                  `json:"index"`
	Message chatResponseMessage  `json:"message"`
	Delta   delta                `json:"delta"`
}

type delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type streamChunk struct {
	Choices []choice `json:"choices"`
}

type modelsResponse struct {
	Data []modelEntry `json:"data"`
}

type modelEntry struct {
	ID string `json:"id"`
}

// ── Struct ────────────────────────────────────────────────────────────

type VLLMProvider struct {
	config    *APIProvider.ProviderConfig
	id        string
	client    *http.Client
	baseURL   string
	apiKey    string
	models    []string
	createdAt int64
	eventID   int64
}

func NewVLLMProvider(config APIProvider.ProviderConfig) (*VLLMProvider, error) {
	if config.Driver != DriverName {
		return nil, fmt.Errorf("invalid driver: expected %s, got %s", DriverName, config.Driver)
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", config.Host, config.Port)
	}
	baseURL = strings.TrimRight(baseURL, "/")

	timeout := 120 * time.Second
	if config.Timeout > 0 {
		timeout = time.Duration(config.Timeout) * time.Second
	}

	return &VLLMProvider{
		config:    &config,
		id:        config.ID,
		client:    &http.Client{Timeout: timeout},
		baseURL:   baseURL,
		apiKey:    config.APIKey,
		models:    config.Models,
		createdAt: time.Now().Unix(),
	}, nil
}

// ── Info ──────────────────────────────────────────────────────────────

func (p *VLLMProvider) GetID() string                     { return p.id }
func (p *VLLMProvider) GetType() APIProvider.ProviderType { return APIProvider.CLOUD }
func (p *VLLMProvider) GetModels() []string               { return p.models }
func (p *VLLMProvider) GetCreatedAt() int64               { return p.createdAt }

func (p *VLLMProvider) GetInfo() APIProvider.ProviderInfo {
	return APIProvider.ProviderInfo{
		ID:        p.id,
		Type:      APIProvider.CLOUD,
		Models:    p.models,
		CreatedAt: p.createdAt,
	}
}

// ── Health ────────────────────────────────────────────────────────────

func (p *VLLMProvider) Health() APIProvider.ProviderHealth {
	resp, err := p.client.Get(p.baseURL + "/health")
	if err != nil {
		return p.errorHealth(err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return APIProvider.ProviderHealth{
			ID:        p.nextEventID(),
			Status:    APIProvider.FREE,
			Timestamp: time.Now().Unix(),
		}
	}

	return p.errorHealth(fmt.Sprintf("status %d", resp.StatusCode))
}

// ── GPU ───────────────────────────────────────────────────────────────

func (p *VLLMProvider) GPU() (APIHardware.GPUSnapshot, error) {
	resp, err := p.client.Get(p.baseURL + "/metrics")
	if err != nil {
		return APIHardware.GPUSnapshot{}, fmt.Errorf("metrics request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return APIHardware.GPUSnapshot{}, fmt.Errorf("metrics returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return APIHardware.GPUSnapshot{}, err
	}

	snap := parseMetrics(body)
	snap.TotalVRAM = p.config.TotalVRAM
	snap.FreeVRAM = snap.TotalVRAM - snap.UsedVRAM
	return snap, nil
}

// parseMetrics extracts GPU cache usage from vLLM Prometheus metrics.
func parseMetrics(body []byte) APIHardware.GPUSnapshot {
	var used int64
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "vllm:gpu_cache_usage_perc") {
			used = parsePromFloat(line)
			break
		}
	}
	return APIHardware.GPUSnapshot{
		Device:   "vllm",
		UsedVRAM: used, // percentage, approximate
	}
}

func parsePromFloat(line string) int64 {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0
	}
	var val float64
	fmt.Sscanf(parts[1], "%f", &val)
	return int64(val * 100)
}

// ── Warm ──────────────────────────────────────────────────────────────

func (p *VLLMProvider) Warm(model string) error {
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

func (p *VLLMProvider) Generate(ctx context.Context, req *APIChat.ChatRequest) (*APIChat.ChatResponse, error) {
	cr := p.buildRequest(req, false)
	response, err := p.doChat(ctx, &cr)
	if err != nil {
		return nil, err
	}
	return p.toChatResponse(response), nil
}

// ── Stream ────────────────────────────────────────────────────────────

func (p *VLLMProvider) Stream(ctx context.Context, req *APIChat.ChatRequest) (<-chan *APIChat.StreamChunk, error) {
	cr := p.buildRequest(req, true)
	resp, err := p.doChatStream(ctx, &cr)
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
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- &APIChat.StreamChunk{Done: true}
				return
			}

			var sc streamChunk
			if err := json.Unmarshal([]byte(data), &sc); err != nil || len(sc.Choices) == 0 {
				continue
			}
			select {
			case ch <- &APIChat.StreamChunk{Delta: sc.Choices[0].Delta.Content, Done: false}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// ── Internal helpers ──────────────────────────────────────────────────

func (p *VLLMProvider) buildRequest(req *APIChat.ChatRequest, stream bool) chatCompletionRequest {
	messages := make([]chatMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = chatMessage{
			Role:    m.Role,
			Content: toOpenAIContent(m.Content),
		}
	}
	return chatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Stream:      stream,
		Temperature: req.Options.Temperature,
		TopP:        req.Options.TopP,
		MaxTokens:   req.Options.MaxTokens,
	}
}

func (p *VLLMProvider) doChat(ctx context.Context, req *chatCompletionRequest) (*chatCompletionResponse, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vllm generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vllm returned %d: %s", resp.StatusCode, string(b))
	}

	var cr chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("vllm decode: %w", err)
	}
	return &cr, nil
}

func (p *VLLMProvider) doChatStream(ctx context.Context, req *chatCompletionRequest) (*http.Response, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vllm stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("vllm stream returned %d: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}

func (p *VLLMProvider) toChatResponse(o *chatCompletionResponse) *APIChat.ChatResponse {
	if len(o.Choices) == 0 {
		return &APIChat.ChatResponse{ID: p.nextEventID(), Model: o.Model, Done: true}
	}
	c := o.Choices[0]
	return &APIChat.ChatResponse{
		ID:    o.ID,
		Model: o.Model,
		Message: APIChat.ChatMessage{
			Role:    c.Message.Role,
			Content: parseResponseContent(c.Message.Content),
		},
		Done: true,
		Usage: APIChat.Usage{
			PromptTokens:     o.Usage.PromptTokens,
			CompletionTokens: o.Usage.CompletionTokens,
			TotalTokens:      o.Usage.TotalTokens,
		},
	}
}

// parseResponseContent handles content as either a plain string or []content array.
func parseResponseContent(raw json.RawMessage) []APIChat.ContentPart {
	if len(raw) == 0 {
		return nil
	}
	// Try string first
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []APIChat.ContentPart{{Type: APIChat.ContentText, Text: text}}
	}
	// Fall back to []content array
	var parts []content
	if err := json.Unmarshal(raw, &parts); err == nil {
		result := make([]APIChat.ContentPart, 0, len(parts))
		for _, p := range parts {
			switch p.Type {
			case "text":
				result = append(result, APIChat.ContentPart{Type: APIChat.ContentText, Text: p.Text})
			case "image_url":
				if p.ImageURL != nil {
					result = append(result, APIChat.ContentPart{Type: APIChat.ContentImageURL, ImageURL: p.ImageURL.URL})
				}
			}
		}
		return result
	}
	return nil
}

func (p *VLLMProvider) errorHealth(errMsg string) APIProvider.ProviderHealth {
	return APIProvider.ProviderHealth{
		ID:        p.nextEventID(),
		Status:    APIProvider.ERROR,
		Timestamp: time.Now().Unix(),
		Error:     errMsg,
	}
}

func (p *VLLMProvider) nextEventID() string {
	p.eventID++
	return fmt.Sprintf("%s-%d", p.id, p.eventID)
}

func toOpenAIContent(parts []APIChat.ContentPart) []content {
	c := make([]content, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case APIChat.ContentText:
			c = append(c, content{Type: "text", Text: part.Text})
		case APIChat.ContentImageURL:
			c = append(c, content{Type: "image_url", ImageURL: &imageURL{URL: part.ImageURL}})
		}
	}
	return c
}

func init() {
	drivers.Register(DriverName, func(config APIProvider.ProviderConfig) (APIProvider.Provider, error) {
		return NewVLLMProvider(config)
	})
}

func textFromContent(parts []content) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

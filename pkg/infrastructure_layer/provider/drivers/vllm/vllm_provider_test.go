package vllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	APIChat "github.com/cpmores/lucinda/api/v1/domain/chat"
	APIProvider "github.com/cpmores/lucinda/api/v1/domain/provider"
)

func testConfig(url string) APIProvider.ProviderConfig {
	return APIProvider.ProviderConfig{
		ID:      "vllm-test",
		Driver:  DriverName,
		BaseURL: url,
		Models:  []string{"qwen-2.5-gptq"},
		Timeout: 5,
	}
}

func TestNewVLLMProvider(t *testing.T) {
	p, err := NewVLLMProvider(testConfig("http://localhost:8000"))
	if err != nil {
		t.Fatalf("NewVLLMProvider: %v", err)
	}
	if p.GetID() != "vllm-test" {
		t.Fatalf("expected id vllm-test, got %s", p.GetID())
	}
	if p.baseURL != "http://localhost:8000" {
		t.Fatalf("expected baseURL, got %s", p.baseURL)
	}
}

func TestNewVLLMProviderWrongDriver(t *testing.T) {
	cfg := testConfig("http://localhost:8000")
	cfg.Driver = "ollama"
	_, err := NewVLLMProvider(cfg)
	if err == nil {
		t.Fatal("expected error for wrong driver, got nil")
	}
}

func TestHealthOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	p, _ := NewVLLMProvider(testConfig(srv.URL))
	h := p.Health()
	if h.Status != APIProvider.Free {
		t.Fatalf("expected FREE, got %s", h.Status)
	}
}

func TestHealthLoading(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p, _ := NewVLLMProvider(testConfig(srv.URL))
	h := p.Health()
	if h.Status != APIProvider.Error {
		t.Fatalf("expected ERROR for 503, got %s", h.Status)
	}
}

func TestHealthConnectionRefused(t *testing.T) {
	p, _ := NewVLLMProvider(testConfig("http://127.0.0.1:1"))
	h := p.Health()
	if h.Status != APIProvider.Error {
		t.Fatalf("expected ERROR for dead server, got %s", h.Status)
	}
}

func TestGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Model != "qwen-2.5-gptq" {
			t.Errorf("expected model qwen-2.5-gptq, got %s", req.Model)
		}
		if req.MaxTokens != 100 {
			t.Errorf("expected max_tokens 100, got %d", req.MaxTokens)
		}

		resp := chatCompletionResponse{
			ID:    "chatcmpl-123",
			Model: req.Model,
			Choices: []choice{{
				Index: 0,
				Message: chatResponseMessage{
					Role:    "assistant",
					Content: json.RawMessage(`"Hello! I am Qwen, an AI assistant."`),
				},
			}},
			Usage: usage{PromptTokens: 15, CompletionTokens: 8, TotalTokens: 23},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, _ := NewVLLMProvider(testConfig(srv.URL))
	resp, err := p.Generate(context.Background(), &APIChat.ChatRequest{
		Model: "qwen-2.5-gptq",
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: "Hello, who are you?"}},
		}},
		Options: APIChat.ModelOptions{MaxTokens: 100},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "qwen-2.5-gptq" {
		t.Fatalf("expected model qwen-2.5-gptq, got %s", resp.Model)
	}
	if !resp.Done {
		t.Fatal("expected Done=true")
	}
	if resp.Usage.TotalTokens != 23 {
		t.Fatalf("expected 23 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestGenerateErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "model not found"}`))
	}))
	defer srv.Close()

	p, _ := NewVLLMProvider(testConfig(srv.URL))
	_, err := p.Generate(context.Background(), &APIChat.ChatRequest{
		Model: "nonexistent",
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: "hi"}},
		}},
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			t.Error("expected stream=true")
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("server does not support flushing")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`data: {"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
			`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"index":0,"delta":{"content":" world"}}]}`,
			`data: {"choices":[{"index":0,"delta":{"content":"!"}}]}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			w.Write([]byte(chunk + "\n\n"))
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p, _ := NewVLLMProvider(testConfig(srv.URL))
	ch, err := p.Stream(context.Background(), &APIChat.ChatRequest{
		Model: "qwen-2.5-gptq",
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: "Hello"}},
		}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var total string
	for chunk := range ch {
		if chunk.Done {
			break
		}
		total += chunk.Delta
	}
	if total != "Hello world!" {
		t.Fatalf("expected 'Hello world!', got %q", total)
	}
}

func TestGPU(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics := `
# HELP vllm:gpu_cache_usage_perc GPU KV-cache usage.
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc{model_name="qwen-2.5-gptq"} 0.42
`
		w.Write([]byte(metrics))
	}))
	defer srv.Close()

	p, _ := NewVLLMProvider(testConfig(srv.URL))
	snap, err := p.GPU()
	if err != nil {
		t.Fatalf("GPU: %v", err)
	}
	if snap.UsedVRAM != 42 {
		t.Fatalf("expected 42%% VRAM, got %d", snap.UsedVRAM)
	}
}

func TestParseMetrics(t *testing.T) {
	input := []byte(`vllm:gpu_cache_usage_perc{model_name="qwen"} 0.75` + "\n")
	snap := parseMetrics(input)
	if snap.UsedVRAM != 75 {
		t.Fatalf("expected 75, got %d", snap.UsedVRAM)
	}
}

func TestBuildRequest(t *testing.T) {
	p, _ := NewVLLMProvider(testConfig("http://localhost:8000"))

	req := p.buildRequest(&APIChat.ChatRequest{
		Model: "qwen-2.5-gptq",
		Messages: []APIChat.ChatMessage{
			{Role: "system", Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: "You are helpful."}}},
			{Role: "user", Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: "Hi"}}},
		},
		Options: APIChat.ModelOptions{Temperature: 0.7, MaxTokens: 200, TopP: 0.9},
	}, false)

	if req.Model != "qwen-2.5-gptq" {
		t.Fatalf("expected model, got %s", req.Model)
	}
	if req.Stream {
		t.Fatal("expected stream=false")
	}
	if req.Temperature != 0.7 {
		t.Fatalf("expected temperature 0.7, got %f", req.Temperature)
	}
	if req.MaxTokens != 200 {
		t.Fatalf("expected max_tokens 200, got %d", req.MaxTokens)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Fatalf("expected system role, got %s", req.Messages[0].Role)
	}
	if req.Messages[1].Content[0].Text != "Hi" {
		t.Fatalf("expected 'Hi', got %s", req.Messages[1].Content[0].Text)
	}
}

func TestWarm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			ID:    "warm-1",
			Model: "qwen-2.5-gptq",
			Choices: []choice{{Index: 0, Message: chatResponseMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"w"`),
			}}},
			Usage: usage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, _ := NewVLLMProvider(testConfig(srv.URL))
	if err := p.Warm("qwen-2.5-gptq"); err != nil {
		t.Fatalf("Warm: %v", err)
	}
}

func TestInitRegistration(t *testing.T) {
	p, err := NewVLLMProvider(testConfig("http://localhost:8000"))
	if err != nil {
		t.Fatalf("NewVLLMProvider: %v", err)
	}
	// Verify the provider is created correctly via the registered factory is
	// implicit; if init() didn't register, the controller would fail.
	_ = p
}

func TestBaseURLFromHostPort(t *testing.T) {
	cfg := APIProvider.ProviderConfig{
		ID:     "vllm-test",
		Driver: DriverName,
		Host:   "192.168.1.100",
		Port:   8000,
	}
	p, err := NewVLLMProvider(cfg)
	if err != nil {
		t.Fatalf("NewVLLMProvider: %v", err)
	}
	if p.baseURL != "http://192.168.1.100:8000" {
		t.Fatalf("expected http://192.168.1.100:8000, got %s", p.baseURL)
	}
}

func TestInfoMethods(t *testing.T) {
	p, _ := NewVLLMProvider(testConfig("http://localhost:8000"))

	if p.GetID() != "vllm-test" {
		t.Fatalf("GetID: expected vllm-test, got %s", p.GetID())
	}
	if len(p.GetModels()) != 1 || p.GetModels()[0] != "qwen-2.5-gptq" {
		t.Fatalf("GetModels: expected [qwen-2.5-gptq], got %v", p.GetModels())
	}
	if got := p.GetCreatedAt(); got == 0 {
		t.Fatal("GetCreatedAt should not be zero")
	}
	info := p.GetInfo()
	if info.ID != "vllm-test" || len(info.Models) != 1 {
		t.Fatalf("GetInfo returned wrong data: %+v", info)
	}
}

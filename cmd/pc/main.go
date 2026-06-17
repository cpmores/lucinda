package main

import (
	"context"
	"fmt"
	"log"
	"time"

	APIChat "github.com/cpmores/lucinda/api/v1/chat"
	APIProvider "github.com/cpmores/lucinda/api/v1/provider"
	eventbus "github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	hardwaremonitor "github.com/cpmores/lucinda/pkg/infrastructure_layer/hardware_monitor"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	provider "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
	transport "github.com/cpmores/lucinda/pkg/infrastructure_layer/transport/transporters"
)

func main() {
	// ── Infrastructure ──────────────────────────────────────────────────────────
	eb := eventbus.NewInMemoryEventBus()
	mm := modulemanager.NewModuleManager()

	tp, err := transport.NewLibp2pTransport(transport.Libp2pTransportOptions{
		Addrs:      []string{"/ip4/127.0.0.1/tcp/0"},
		OutsLength: 20,
		InsLength:  100,
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}

	hm := hardwaremonitor.NewHardwareMonitor(eb, 5)
	pc := provider.NewProviderController()

	// HACK:
	// ── Register vLLM Qwen provider ────────────────────────────────────────────
	if err := pc.Register(APIProvider.ProviderConfig{
		ID:     "vllm-qwen",
		Driver: "vllm",
		Host:   "localhost",
		Port:   8000,
		Models: []string{"qwen-2.5-gptq"},
	}); err != nil {
		log.Fatalf("Failed to register vllm provider: %v", err)
	}

	// ── ModuleManager ───────────────────────────────────────────────────────────
	tp.RegisterWithManager(mm)
	hm.RegisterWithManager(mm)
	pc.RegisterWithManager(mm)

	// ── Test: Generate from Qwen ────────────────────────────────────────────────
	prov, err := pc.Get("vllm-qwen")
	if err != nil {
		log.Fatalf("Failed to get provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := prov.Generate(ctx, &APIChat.ChatRequest{
		Model: "qwen-2.5-gptq",
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: "Hello, who are you?"}},
		}},
		Options: APIChat.ModelOptions{MaxTokens: 100, Temperature: 0.7},
	})
	if err != nil {
		log.Fatalf("Generate failed: %v", err)
	}

	fmt.Printf("Model:   %s\n", resp.Model)
	fmt.Printf("Usage:   %d tokens (prompt: %d, completion: %d)\n",
		resp.Usage.TotalTokens, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	for _, part := range resp.Message.Content {
		if part.Type == APIChat.ContentText {
			fmt.Printf("Qwen:    %s\n", part.Text)
		}
	}

	// ── Test: Stream from Qwen ──────────────────────────────────────────────────
	fmt.Print("\nStream:  ")
	streamCh, err := prov.Stream(ctx, &APIChat.ChatRequest{
		Model: "qwen-2.5-gptq",
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: "Generate a small novel for 3000 words."}},
		}},
		Options: APIChat.ModelOptions{MaxTokens: 1000},
	})
	if err != nil {
		log.Fatalf("Stream failed: %v", err)
	}
	for chunk := range streamCh {
		if chunk.Done {
			fmt.Println()
			break
		}
		fmt.Print(chunk.Delta)
	}

	// ── Health check ────────────────────────────────────────────────────────────
	health := prov.Health()
	fmt.Printf("Health:  %s\n", health.Status)
}

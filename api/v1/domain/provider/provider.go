// Package apiprovider
package apiprovider

import (
	"context"

	APIChat "github.com/cpmores/lucinda/api/v1/domain/chat"
	APIHardware "github.com/cpmores/lucinda/api/v1/domain/hardware"
)

// ── Provider Interface ─────────────────────────────────────────────────

// Provider is the interface every inference backend driver must implement.
type Provider interface {
	GetID() string
	GetType() ProviderType
	GetModels() []string
	GetInfo() ProviderInfo

	// MaxContextTokens returns the maximum context size (input + output tokens)
	// the model can handle. Used by the planner and reducer to stay within limits.
	MaxContextTokens() int

	GPU() (APIHardware.GPUSnapshot, error)
	Health() ProviderHealth

	Generate(ctx context.Context, req *APIChat.ChatRequest) (*APIChat.ChatResponse, error)
	Stream(ctx context.Context, req *APIChat.ChatRequest) (<-chan *APIChat.StreamChunk, error)
	Warm(model string) error
}

// ── Provider ──────────────────────────────────────────────────────────

// ProviderType represents the type of a provider,
// such as "LOCAL", "CLOUD"
type ProviderType string

const (
	Local  ProviderType = "local"
	Cloud  ProviderType = "cloud"
	Hybrid ProviderType = "hybrid"
)

// ProviderInfo represents the information about a provider,
// including its ID and the models it supports.
type ProviderInfo struct {
	ID   string       `json:"id"`
	Type ProviderType `json:"type"`
	// HACK: Update Model Info Structure
	Models    []string `json:"models"`
	CreatedAt int64    `json:"created_at"`
}

type ProviderStatus string

const (
	Initializing ProviderStatus = "initializing"
	Free         ProviderStatus = "free"
	Busy         ProviderStatus = "busy"
	Pending      ProviderStatus = "pending"
	Error        ProviderStatus = "error"
)

// ProviderHealth represents the health status of a provider,
// including its ID, status, timestamp, and any error message if applicable.
// return from Provider.Health
type ProviderHealth struct {
	ID        string         `json:"id"`
	Status    ProviderStatus `json:"status"`
	Timestamp int64          `json:"timestamp"`
	Error     string         `json:"error,omitempty"`
}

// ── Provider Controller ──────────────────────────────────────────────────────────

// ProviderConfig represents the configuration for registering a provider,
type ProviderConfig struct {
	ID               string            `mapstructure:"id"`
	Type             ProviderType      `mapstructure:"type"`
	Driver           string            `mapstructure:"driver"` // "ollama", "openai", "anthropic"
	Host             string            `mapstructure:"host"`
	Port             int               `mapstructure:"port"`
	BaseURL          string            `mapstructure:"base_url"` // optional, overrides host:port
	APIKey           string            `mapstructure:"api_key"`  // empty for ollama
	Headers          map[string]string `mapstructure:"headers"`  // extra headers
	Models           []string          `mapstructure:"models"`
	TotalVRAM        int64             `mapstructure:"total_vram"`         // physical GPU VRAM in bytes
	MaxContextTokens int               `mapstructure:"max_context_tokens"` // model context window (default 2048)
	Timeout          int               `mapstructure:"timeout"`            // seconds between polls/retries
}

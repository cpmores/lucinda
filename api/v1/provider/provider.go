// Package apiprovider
package apiprovider

import (
	"context"

	apichat "github.com/cpmores/lucinda/api/v1/chat"
	apihardware "github.com/cpmores/lucinda/api/v1/hardware"
)

// ── Provider Interface ─────────────────────────────────────────────────

// Provider is the interface every inference backend driver must implement.
type Provider interface {
	GetID()        string
	GetType()      ProviderType
	GetModels()    []string
	GetInfo()      ProviderInfo

	GPU()    (apihardware.GPUSnapshot, error)
	Health() ProviderHealth

	Generate(ctx context.Context, req *apichat.ChatRequest) (*apichat.ChatResponse, error)
	Stream(ctx context.Context, req *apichat.ChatRequest) (<-chan *apichat.StreamChunk, error)
	Warm(model string) error
}

// ── Provider ──────────────────────────────────────────────────────────

// ProviderType represents the type of a provider,
// such as "LOCAL", "CLOUD"
type ProviderType string

const (
	LOCAL  ProviderType = "local"
	CLOUD  ProviderType = "cloud"
	HYBRID ProviderType = "hybrid"
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
	INITIALIZING ProviderStatus = "initializing"
	FREE         ProviderStatus = "free"
	BUSY         ProviderStatus = "busy"
	PENDING      ProviderStatus = "pending"
	ERROR        ProviderStatus = "error"
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
	ID      string            `mapstructure:"id"`
	Type    ProviderType      `mapstructure:"type"`
	Driver  string            `mapstructure:"driver"` // "ollama", "openai", "anthropic"
	Host    string            `mapstructure:"host"`
	Port    int               `mapstructure:"port"`
	BaseURL string            `mapstructure:"base_url"` // optional, overrides host:port
	APIKey  string            `mapstructure:"api_key"`  // empty for ollama
	Headers map[string]string `mapstructure:"headers"`  // extra headers
	Models  []string          `mapstructure:"models"`
	TotalVRAM int64            `mapstructure:"total_vram"` // physical GPU VRAM in bytes
	Timeout   int               `mapstructure:"timeout"`    // seconds between polls/retries
}

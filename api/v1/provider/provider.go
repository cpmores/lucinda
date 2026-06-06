// Package apiprovider
package apiprovider

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
type ProviderConfig struct{}

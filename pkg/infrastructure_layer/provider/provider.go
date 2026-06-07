// Package provider provides the implementation of the provider layer,
// which is responsible for providing external services and resources to the application.
// This layer abstracts away the details of how these services are accessed and used,
// allowing the application to interact with them in a consistent and unified way.
// The provider layer can include implementations for various types of services, such as databases, APIs, messaging systems, and more.
package provider

import (
	"context"

	APIChat "github.com/cpmores/lucinda/api/v1/chat"
	APIHardware "github.com/cpmores/lucinda/api/v1/hardware"
	APIProvider "github.com/cpmores/lucinda/api/v1/provider"
)

// Provider is the interface that defines the methods for interacting with a provider.
// Mainly used by ProviderController
type Provider interface {
	// ── Info ──────────────────────────────────────────────────────────
	GetID() string
	GetType() APIProvider.ProviderType
	GetModels() []string
	GetCreatedAt() int64
	GetInfo() APIProvider.ProviderInfo

	// ── GPU ──────────────────────────────────────────────────────────
	GPU() (APIHardware.GPUSnapshot, error)

	// ── Health ──────────────────────────────────────────────────────────
	Health() APIProvider.ProviderHealth
	Warm(model string) error

	// ── Chat ──────────────────────────────────────────────────────────
	Generate(ctx context.Context, req *APIChat.ChatRequest) (*APIChat.ChatResponse, error)
	Stream(ctx context.Context, req *APIChat.ChatRequest) (<-chan *APIChat.StreamChunk, error)
}

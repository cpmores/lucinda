package provider

import (
	"context"

	api "github.com/cpmores/lucinda/api/v1"
)

type ProviderConfig struct {
	ID     string   `mapstructure:"id"`
	Type   string   `mapstructure:"type"`
	Host   string   `mapstructure:"host"`
	Port   int      `mapstructure:"port"`
	Models []string `mapstructure:"models"`
}

var Providers map[string]AIProvider = make(map[string]AIProvider)

type ProviderType string

type AIProvider interface {
	GetId() string
	Generate(ctx context.Context, req *api.ChatRequest) (*api.ChatResponse, error)
	Stream(ctx context.Context, req *api.ChatRequest) (<-chan *api.ChatResponse, error)
	GetStatus() (*api.ProviderStatus, error)
	CheckHealth() error
}

var AIProviderFactories map[string]AIProviderFactory = make(map[string]AIProviderFactory)

type AIProviderFactory interface {
	Create(config ProviderConfig) (AIProvider, error)
	CreateDefault() (AIProvider, error)
}

func RegisterAIProviderFactory(platform string, factory AIProviderFactory) {
	AIProviderFactories[platform] = factory
}

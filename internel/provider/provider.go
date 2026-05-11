package provider

import (
	"context"
	"fmt"

	api "github.com/cpmores/lucinda/api/v1"
	"github.com/spf13/viper"
)

var Providers map[string]AIProvider = make(map[string]AIProvider)

type ProviderType string

type AIProvider interface {
	GetId() (string, error)
	Generate(ctx context.Context, req *api.ChatRequest) (*api.ChatResponse, error)
	Stream(ctx context.Context, req *api.ChatRequest) (<-chan *api.ChatResponse, error)
	GetStatus() *api.ProviderStatus
	CheckHealth() error
}

var AIProviderFactories map[string]AIProviderFactory = make(map[string]AIProviderFactory)

type AIProviderFactory interface {
	Create(config *viper.Viper) (AIProvider, error)
	CreateDefault() (AIProvider, error)
}

func RegisterAIProviderFactory(platform string, factory AIProviderFactory) {
	AIProviderFactories[platform] = factory
}

func CreateProvider(platform string) (string, error) {
	factory, ok := AIProviderFactories[platform]
	if !ok {
		return "", fmt.Errorf("%s driver not loaded", platform)
	}

	provider, err := factory.CreateDefault()
	if err != nil {
		return "", err
	}

	// check provider health
	err = provider.CheckHealth()
	if err != nil {
		return "", err
	}

	id, err := provider.GetId()
	if err != nil {
		return "", err
	}
	Providers[id] = provider
	return id, nil
}

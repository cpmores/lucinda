package provider

import (
	"context"
	"fmt"
	"log"

	api "github.com/cpmores/lucinda/api/v1"
	eventbus "github.com/cpmores/lucinda/internel/eventBus"
	"github.com/spf13/viper"
)

// provider controller
type DefaultProviderController struct {
	Providers map[string]AIProvider // store loaded providers, id -> *AIProvider
	eventBus  eventbus.EventBus
}

func NewDefaultProviderController(config *viper.Viper, eventBus eventbus.EventBus) (*DefaultProviderController, error) {
	controller := &DefaultProviderController{
		Providers: make(map[string]AIProvider),
		eventBus:  eventBus,
	}
	if err := controller.LoadProviders(config); err != nil {
		return nil, err
	}
	return controller, nil
}

type Controller interface {
	LoadProviders(config *viper.Viper) error
	GetProvider(id string) (AIProvider, error)
	UpdateProvider(id string, provider AIProvider)
	GetStatus() ([]string, map[string]api.ProviderStatus, error)
	CreateProvider(config ProviderConfig) (string, error)
}

func (controller *DefaultProviderController) LoadProviders(config *viper.Viper) error {
	var providers []ProviderConfig
	if err := config.UnmarshalKey("provider_controller.providers", &providers); err != nil {
		return fmt.Errorf("failed to unmarshal providers: %w", err)
	}

	for _, p := range providers {
		id, err := controller.CreateProvider(p)
		if err != nil {
			log.Printf("failed to create provider %s-%s: %s", p.ID, p.Type, err.Error())
			continue
		}
		log.Printf("provider %s-%s created with id %s", p.ID, p.Type, id)
	}

	return nil
}

func (controller *DefaultProviderController) GetProvider(id string) (AIProvider, error) {
	provider, ok := controller.Providers[id]
	if !ok {
		return nil, fmt.Errorf("ollama provider not found")
	}

	return provider, nil
}

func (controller *DefaultProviderController) UpdateProvider(id string, provider AIProvider) {
	controller.Providers[id] = provider
}

func (controller *DefaultProviderController) GetStatus() ([]string, map[string]api.ProviderStatus, error) {
	statuses := make(map[string]api.ProviderStatus)
	ids := make([]string, 0)
	for id, provider := range controller.Providers {
		if status, err := provider.GetStatus(); err == nil {
			statuses[id] = *status
			ids = append(ids, id)
		} else {
			log.Printf("failed to get provider %s status", id)
			// return nil, nil, err
		}
	}

	return ids, statuses, nil
}

func (controller *DefaultProviderController) CreateProvider(config ProviderConfig) (string, error) {
	factory, ok := AIProviderFactories[config.Type]
	if !ok {
		return "", fmt.Errorf("%s driver not loaded", config.Type)
	}

	provider, err := factory.Create(config)
	if err != nil {
		return "", err
	}

	// check provider health
	err = provider.CheckHealth()
	if err != nil {
		return "", err
	}

	id := provider.GetId()
	controller.UpdateProvider(id, provider)
	return id, nil
}

func NewProviderController(ctx context.Context, eventBus eventbus.EventBus, config *viper.Viper) (Controller, error) {
	providerController, err := NewDefaultProviderController(config, eventBus)
	if err != nil {
		return nil, err
	}
	log.Printf("Provider controller started")
	return providerController, nil
}

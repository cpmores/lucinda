package provider

import (
	"fmt"
	"log"

	api "github.com/cpmores/lucinda/api/v1"
)

var ProviderController = &DefaultProviderController{
	Providers: make(map[string]AIProvider),
}

// provider controller
type DefaultProviderController struct {
	Providers map[string]AIProvider // store loaded providers, id -> *AIProvider
}

type Controller interface {
	GetProvider(id string) (AIProvider, error)
	UpdateProvider(id string, provider AIProvider)
	GetStatus() ([]string, map[string]api.ProviderStatus, error)
}

func (controller DefaultProviderController) GetProvider(id string) (AIProvider, error) {
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

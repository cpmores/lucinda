package provider

import (
	"fmt"

	api "github.com/cpmores/lucinda/api/v1"
)

var ProviderController = &defaultProviderController{
	Providers: make(map[string]AIProvider),
}

// provider controller
type defaultProviderController struct {
	Providers map[string]AIProvider // store loaded providers, id -> *AIProvider
}

type Controller interface {
	GetProvider(id string) (AIProvider, error)
	UpdateProvider(id string, provider AIProvider)
	GetStatus() (*api.NodeProviderStatus, error)
}

func (controller defaultProviderController) GetProvider(id string) (AIProvider, error) {
	provider, ok := controller.Providers[id]
	if !ok {
		return nil, fmt.Errorf("ollama provider not found")
	}

	return provider, nil
}

func (controller *defaultProviderController) UpdateProvider(id string, provider AIProvider) {
	controller.Providers[id] = provider
}

func (controller *defaultProviderController) GetStatus() (*api.NodeProviderStatus, error) {
	// TDOO:monitor
	return nil, nil
}

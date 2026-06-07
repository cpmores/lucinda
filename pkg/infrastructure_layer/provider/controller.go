// Package provider
// Provicer Controller
package provider

import (
	"fmt"

	APIHardware "github.com/cpmores/lucinda/api/v1/hardware"
	APIProvider "github.com/cpmores/lucinda/api/v1/provider"
	"github.com/spf13/viper"
)

type ProviderController interface {
	LoadProviders(config *viper.Viper) error
	Register(config APIProvider.ProviderConfig) error
	Get(id string) (Provider, error)
	List() []Provider
	Health(id string) (APIProvider.ProviderHealth, error)
	HealthAll() []APIProvider.ProviderHealth
	GPU() APIHardware.GPUSnapshot
}

type controller struct {
	providers map[string]Provider
}

// HACK: change controller to ProviderController

// NewProviderController creates a new instance of the provider controller.
func NewProviderController() *controller {
	return &controller{
		providers: make(map[string]Provider),
	}
}

// ── Controller Implementation ──────────────────────────────────────────────────────────

// LoadProviders loads providers from the configuration file and registers them.
// return error if any provider fails to register,
// with the index of the failed provider in the configuration file for easier debugging.
func (c *controller) LoadProviders(config *viper.Viper) error {
	var configs []APIProvider.ProviderConfig

	if err := config.UnmarshalKey("providers", &configs); err != nil {
		return fmt.Errorf("failed to unmarshal provider configs: %w", err)
	}

	for i, config := range configs {
		err := c.Register(config)
		if err != nil {
			return fmt.Errorf("failed to register provider INDEX %d: %w", i, err)
		}
	}

	return nil
}

func (c *controller) Register(config APIProvider.ProviderConfig) error {
	return nil
}

// Package provider
// Provicer Controller
package provider

import (
	"fmt"

	APIHardware "github.com/cpmores/lucinda/api/v1/domain/hardware"
	APIProvider "github.com/cpmores/lucinda/api/v1/domain/provider"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/provider/drivers"
	"github.com/spf13/viper"
)

// ── ProviderController ──────────────────────────────────────────────────────────

type ProviderController interface {
	LoadProviders(config *viper.Viper) error
	Register(config APIProvider.ProviderConfig) error
	Get(id string) (APIProvider.Provider, error)
	List() []APIProvider.Provider
	GetPlanProv() (APIProvider.Provider, error) // first available provider for planning
	MaxContext() int                            // context window of the first available provider
	Health(id string) (APIProvider.ProviderHealth, error)
	HealthAll() []APIProvider.ProviderHealth
	GPU() APIHardware.GPUSnapshot
}

type controller struct {
	providers map[string]APIProvider.Provider
}

// HACK: change controller to ProviderController

// NewProviderController creates a new instance of the provider controller.
func NewProviderController() *controller {
	return &controller{
		providers: make(map[string]APIProvider.Provider),
	}
}

// ── Controller Implementation ──────────────────────────────────────────────────────────
// ── Section ──────────────────────────────────────────────────────────

// LoadProviders loads providers from the configuration file and registers them.
// return error if any provider fails to register,
// with the index of the failed provider in the configuration file for easier debugging.
func (c *controller) LoadProviders(config *viper.Viper) error {
	var configs []APIProvider.ProviderConfig

	if err := config.UnmarshalKey("provider_controller.providers", &configs); err != nil {
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

// Register registers a provider with the given configuration.
func (c *controller) Register(config APIProvider.ProviderConfig) error {
	if _, ok := c.providers[config.ID]; ok {
		return fmt.Errorf("provider already registered: %s", config.ID)
	}

	provider, err := drivers.Create(config)
	if err != nil {
		return fmt.Errorf("failed to create provider %s: %w", config.ID, err)
	}

	c.providers[config.ID] = provider
	return nil
}

// Get retrieves a provider by its ID.
// Returns an error if the provider is not found.
func (c *controller) Get(id string) (APIProvider.Provider, error) {
	if c.providers[id] == nil {
		return nil, fmt.Errorf("provider not found: %s", id)
	}

	return c.providers[id], nil
}

// List returns a list of all registered providers.
func (c *controller) List() []APIProvider.Provider {
	var list []APIProvider.Provider
	for _, provider := range c.providers {
		list = append(list, provider)
	}

	return list
}

// GetPlanProv returns the first available provider for planning tasks.
func (c *controller) GetPlanProv() (APIProvider.Provider, error) {
	list := c.List()
	if len(list) == 0 {
		return nil, fmt.Errorf("no provider available for planning")
	}
	return list[0], nil
}

// MaxContext returns the context window size of the first available provider,
// or 2048 as a safe default if no provider is configured.
func (c *controller) MaxContext() int {
	list := c.List()
	if len(list) == 0 {
		return 2048
	}
	return list[0].MaxContextTokens()
}

// Health returns the health status of a provider by its ID.
func (c *controller) Health(id string) (APIProvider.ProviderHealth, error) {
	provider, err := c.Get(id)
	if err != nil {
		return APIProvider.ProviderHealth{}, err
	}

	return provider.Health(), nil
}

// HealthAll returns the health status of all registered providers.
func (c *controller) HealthAll() []APIProvider.ProviderHealth {
	var healthAll []APIProvider.ProviderHealth
	for _, provider := range c.providers {
		healthAll = append(healthAll, provider.Health())
	}

	return healthAll
}

// GPU returns the GPU snapshot from the first local provider that responds.
// All local providers share the same physical GPU, so only one result is needed.
func (c *controller) GPU() APIHardware.GPUSnapshot {
	for _, p := range c.providers {
		snap, err := p.GPU()
		if err != nil || snap.UsedVRAM == 0 {
			continue
		}
		return snap
	}
	return APIHardware.GPUSnapshot{}
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (c *controller) GetModuleType() APIModule.ModuleType {
	return APIModule.ProviderController
}

func (c *controller) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(c.GetModuleType(), "default")
}

func (c *controller) CheckHealth() APIModule.ModuleHealth {
	status := APIModule.Running
	if len(c.providers) == 0 {
		status = APIModule.Pending
	}
	return APIModule.NewModuleHealth(c.GetModuleID(), c.GetModuleType(), status)
}

func (c *controller) RegisterWithManager(manager modulemanager.ModuleManager) error {
	return manager.Register(c)
}

func (c *controller) DependsOn() map[APIModule.ModuleType]string {
	return nil
}

func (c *controller) DependsEnable() error {
	return nil
}

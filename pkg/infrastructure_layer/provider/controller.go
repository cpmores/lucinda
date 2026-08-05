// Package provider
// Provicer Controller
package provider

import (
	"fmt"

	APIHardware "github.com/cpmores/lucinda/api/v1/domain/hardware"
	APIProvider "github.com/cpmores/lucinda/api/v1/domain/provider"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
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
	GetProvByFilter(modelFilter APIProvider.ModelFilter) ([]APIProvider.ModelMatch, error)
	Health(id string) (APIProvider.ProviderHealth, error)
	HealthAll() []APIProvider.ProviderHealth
	GPU() APIHardware.GPUSnapshot
}

type controller struct {
	providers map[string]APIProvider.Provider
	log       *logger.Logger
}

// HACK: change controller to ProviderController

// NewProviderController creates a new instance of the provider controller.
func NewProviderController(log *logger.Logger) *controller {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &controller{
		providers: make(map[string]APIProvider.Provider),
		log:       log,
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

	c.log.Info("providers loaded", "count", len(c.providers))
	c.warmupPing()
	return nil
}

func (c *controller) warmupPing() {
	for _, p := range c.List() {
		if p.Health().Status != APIProvider.Free {
			c.log.Warn("provider not free", "provider", p.GetID(), "status", p.Health().Status)
		} else {
			c.log.Info("provider free", "provider", p.GetID(), "status", p.Health().Status)
			for _, model := range p.GetModels() {
				c.log.Info("free model", "provider", p.GetID(), "model", model.ID)
			}
		}
	}
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
	c.log.Info("provider registered", "id", config.ID, "driver", config.Driver)
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

// GetProvByFilter returns, per provider, the models that satisfy the filter.
// Only providers currently Free (Status()) are considered. Matches return
// empty (nil error) when nothing qualifies — callers decide how to react.
func (c *controller) GetProvByFilter(modelFilter APIProvider.ModelFilter) ([]APIProvider.ModelMatch, error) {
	var matches []APIProvider.ModelMatch
	for _, p := range c.List() {
		if p.Status() != APIProvider.Free {
			continue
		}
		var modelInfos []APIProvider.ModelInfo
		termMap := map[int][]int{}
		for _, m := range p.GetModels() {
			var matchedTerms []int
			for ti, term := range modelFilter.Required {
				if term.Matches(m) {
					matchedTerms = append(matchedTerms, ti)
				}
			}
			if len(matchedTerms) > 0 {
				modelInfos = append(modelInfos, m)
				termMap[len(modelInfos)-1] = matchedTerms
			}
		}
		if len(modelInfos) > 0 {
			matches = append(matches, APIProvider.ModelMatch{
				Provider:   p,
				ModelInfos: modelInfos,
				TermMap:    termMap,
			})
		}
	}
	return matches, nil
}

// List returns a list of all registered providers.
func (c *controller) List() []APIProvider.Provider {
	var list []APIProvider.Provider
	for _, provider := range c.providers {
		list = append(list, provider)
	}

	return list
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
		if err != nil {
			c.log.Warn("gpu query failed", "provider", p.GetID(), "err", err)
			continue
		}
		if snap.UsedVRAM == 0 {
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

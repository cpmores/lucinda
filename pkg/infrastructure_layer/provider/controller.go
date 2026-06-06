// Package provider
// Provicer Controller
package provider

import (
	APIHardware "github.com/cpmores/lucinda/api/v1/hardware"
	APIProvider "github.com/cpmores/lucinda/api/v1/provider"
)

type ProviderController interface {
	Register(config APIProvider.ProviderConfig) error
	Get(id string) (Provider, error)
	List() []Provider
	Health(id string) (APIProvider.ProviderHealth, error)
	HealthAll() []APIProvider.ProviderHealth
	GPU() APIHardware.GPUSnapshot
}

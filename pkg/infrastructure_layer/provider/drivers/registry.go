// Package drivers provides the factory registry for provider drivers.
// Each driver package registers itself via init().
package drivers

import (
	"fmt"

	APIProvider "github.com/cpmores/lucinda/api/v1/provider"
)

var factories = map[string]func(APIProvider.ProviderConfig) (APIProvider.Provider, error){}

func Register(driver string, fn func(APIProvider.ProviderConfig) (APIProvider.Provider, error)) {
	factories[driver] = fn
}

func Create(config APIProvider.ProviderConfig) (APIProvider.Provider, error) {
	fn, ok := factories[config.Driver]
	if !ok {
		return nil, fmt.Errorf("unknown driver: %s", config.Driver)
	}
	return fn(config)
}

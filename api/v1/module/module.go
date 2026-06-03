// Package apimodule
// specifies the data structures and types used for representing modules in the API.
// Mostly used in ModuleManager
package apimodule

type (
	// ModuleType represents the type of a module, such as "eventbus" or "transport"
	ModuleType string
	// ModuleID represnets the identifier for a module, such as "transport-libp2p" or "eventbus-nats"
	ModuleID string
)

// ModuleTypes are defined under API
// ModuleIds defined under Modules
const (
	EVENTBUS        ModuleType = "eventbus"
	TRANSPORT       ModuleType = "transport"
	HARDWAREMONITOR ModuleType = "hardware_monitor"
	ModuleManager   ModuleType = "module_manager"
	// TODO: other module types such as storage, monitor, etc.
)

// ModuleHealth structure
// TODO: Finish ModuleHealth stucture
type ModuleHealth struct{}

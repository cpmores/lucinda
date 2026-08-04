// Package apimodule
// specifies the data structures and types used for representing modules in the API.
// Mostly used in ModuleManager
package apimodule

type (
	// ModuleType represents the type of a module, such as "eventbus" or "transport"
	ModuleType string
	// ModuleID represents the identifier for a module, such as "transport-libp2p" or "eventbus-nats"
	ModuleID string
	// ModuleStatus represents the current status of a module, such as "running", "stopped", or "error"
	ModuleStatus string
)

// ModuleTypes are defined under API
const (
	EventBus           ModuleType = "eventbus"
	Transport          ModuleType = "transport"
	HardwareMonitor    ModuleType = "hardware_monitor"
	ModuleManager      ModuleType = "module_manager"
	ProviderController ModuleType = "provider_controller"
	Logger             ModuleType = "logger"
)

// Module Status
const (
	Initializing ModuleStatus = "initializing"
	Running      ModuleStatus = "running"
	Pending      ModuleStatus = "pending"
	Stopped      ModuleStatus = "stopped"
	Error        ModuleStatus = "error"
)

// Module is the interface every component must implement to register with the ModuleManager.
type Module interface {
	ID() ModuleID
	Type() ModuleType
	Health() ModuleHealth
}

// ModuleHealth reports the current health of a module.
type ModuleHealth struct {
	ID        ModuleID     `json:"module_id"`
	Type      ModuleType   `json:"module_type"`
	Status    ModuleStatus `json:"status"`
	Timestamp int64        `json:"timestamp"`
	Error     string       `json:"error,omitempty"`
}

func NewModuleID(moduleType ModuleType, name string) ModuleID {
	return ModuleID(string(moduleType) + "-" + name)
}

func NewModuleHealth(id ModuleID, typ ModuleType, status ModuleStatus) ModuleHealth {
	return ModuleHealth{ID: id, Type: typ, Status: status}
}

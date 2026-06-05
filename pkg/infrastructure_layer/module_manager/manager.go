// Package manager provides the implementation of the module manager,
// which is responsible for managing the lifecycle of modules in the application.
// It allows for registering, initializing, and retrieving modules as needed.
package manager

import (
	"fmt"
	"sync"

	APIModule "github.com/cpmores/lucinda/api/v1/module"
)

// AvailableModule must be defined for every modules
// And registered with ModuleManager
type AvailableModule interface {
	GetModuleType() APIModule.ModuleType
	GetModuleID() APIModule.ModuleID
	CheckHealth() APIModule.ModuleHealth
	RegisterWithManager(m ModuleManager) error
}

// ModuleManager defines the interface for managing modules,
// including registration, lookup, and capability management.
type ModuleManager interface {
	// ── Registration ──────────────────────────────────────────
	Register(module AvailableModule) error
	Unregister(id APIModule.ModuleID) error

	// ── Lookup ────────────────────────────────────────────────
	Get(id APIModule.ModuleID) (AvailableModule, error)
	GetByType(t APIModule.ModuleType) []AvailableModule
	List() []AvailableModule
	Exists(id APIModule.ModuleID) bool

	// ── Capability ────────────────────────────────────────────
	Grant(granter APIModule.ModuleType, grantee APIModule.ModuleType) error
	Require(caller AvailableModule, targetType APIModule.ModuleType, targetID APIModule.ModuleID) (AvailableModule, error)

	// ── Health ────────────────────────────────────────────────
	Health(id APIModule.ModuleID) (APIModule.ModuleHealth, error)
	HealthAll() map[APIModule.ModuleID]APIModule.ModuleHealth
}

// Manager implements ModuleManager with a RWMutex-guarded registry.
type Manager struct {
	sync.RWMutex
	modules      map[APIModule.ModuleID]AvailableModule
	capabilities map[APIModule.ModuleType]map[APIModule.ModuleType]struct{}
}

func NewModuleManager() ModuleManager {
	return &Manager{
		modules:      make(map[APIModule.ModuleID]AvailableModule),
		capabilities: make(map[APIModule.ModuleType]map[APIModule.ModuleType]struct{}),
	}
}

// Register adds a new module to the manager. Duplicate IDs return an error.
func (m *Manager) Register(module AvailableModule) error {
	moduleID := module.GetModuleID()
	m.Lock()
	defer m.Unlock()

	if _, ok := m.modules[moduleID]; ok {
		return fmt.Errorf("module with ID %s is already registered", moduleID)
	}
	m.modules[moduleID] = module
	return nil
}

// Unregister removes a module by ID.
func (m *Manager) Unregister(id APIModule.ModuleID) error {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.modules[id]; !ok {
		return fmt.Errorf("module with ID %s not found for unregistration", id)
	}
	delete(m.modules, id)
	return nil
}

// Get retrieves a module by ID.
func (m *Manager) Get(id APIModule.ModuleID) (AvailableModule, error) {
	m.RLock()
	defer m.RUnlock()

	if mod, ok := m.modules[id]; !ok {
		return nil, fmt.Errorf("module with ID %s not found", id)
	} else {
		return mod, nil
	}
}

// GetByType returns all modules of a given type.
func (m *Manager) GetByType(t APIModule.ModuleType) []AvailableModule {
	m.RLock()
	defer m.RUnlock()

	result := make([]AvailableModule, 0)
	for _, module := range m.modules {
		if module.GetModuleType() == t {
			result = append(result, module)
		}
	}
	return result
}

// List returns all registered modules.
func (m *Manager) List() []AvailableModule {
	m.RLock()
	defer m.RUnlock()

	result := make([]AvailableModule, 0, len(m.modules))
	for _, module := range m.modules {
		result = append(result, module)
	}
	return result
}

// Exists checks whether a module ID is registered.
func (m *Manager) Exists(id APIModule.ModuleID) bool {
	m.RLock()
	defer m.RUnlock()
	_, ok := m.modules[id]
	return ok
}

// Grant declares that granter may access grantee. Idempotent.
func (m *Manager) Grant(granter APIModule.ModuleType, grantee APIModule.ModuleType) error {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.capabilities[granter]; !ok {
		m.capabilities[granter] = make(map[APIModule.ModuleType]struct{})
	}
	m.capabilities[granter][grantee] = struct{}{}
	return nil
}

// isGranted checks whether granter is allowed to access grantee.
// Caller must hold at least m.RLock().
func (m *Manager) isGranted(granter APIModule.ModuleType, grantee APIModule.ModuleType) bool {
	if _, ok := m.capabilities[granter]; !ok {
		return false
	}
	_, ok := m.capabilities[granter][grantee]
	return ok
}

// Require looks up a module and verifies the caller has permission.
func (m *Manager) Require(caller AvailableModule, targetType APIModule.ModuleType, targetID APIModule.ModuleID) (AvailableModule, error) {
	m.RLock()
	defer m.RUnlock()

	if !m.isGranted(caller.GetModuleType(), targetType) {
		return nil, fmt.Errorf("access denied: %s cannot access %s", caller.GetModuleType(), targetType)
	}

	if mod, ok := m.modules[targetID]; !ok {
		return nil, fmt.Errorf("module with ID %s not found", targetID)
	} else {
		return mod, nil
	}
}

// Health returns the health of a module by ID.
func (m *Manager) Health(id APIModule.ModuleID) (APIModule.ModuleHealth, error) {
	mod, err := m.Get(id)
	if err != nil {
		return APIModule.ModuleHealth{}, err
	}
	return mod.CheckHealth(), nil
}

// HealthAll returns the health of all registered modules.
func (m *Manager) HealthAll() map[APIModule.ModuleID]APIModule.ModuleHealth {
	m.RLock()
	defer m.RUnlock()

	result := make(map[APIModule.ModuleID]APIModule.ModuleHealth, len(m.modules))
	for id, mod := range m.modules {
		result[id] = mod.CheckHealth()
	}
	return result
}

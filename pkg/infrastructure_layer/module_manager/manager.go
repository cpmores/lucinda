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
}

// ModuldeManager defines the interface for managing modules in the application.
// It provides methods for registering, initializing, and retrieving modules.
// The implementation of this interface will handle the lifecycle of modules,
// ensuring they are properly initialized and available when needed.
type ModuldeManager interface {
	Register(module AvailableModule) error
	Require(reqModule AvailableModule, resType APIModule.ModuleType, resID APIModule.ModuleID) (AvailableModule, error)
	Check(moduleType APIModule.ModuleType) bool
	CheckHealth(moduleID APIModule.ModuleID) (APIModule.ModuleHealth, error)
}

// Manager controlls the registerment and requirement of an AvailableModule
type Manager struct {
	sync.RWMutex
	// modules is a map for available module
	modules map[APIModule.ModuleID]AvailableModule
	// moduleTypeList
	moduleTypeList map[APIModule.ModuleType]bool
	// righList
	rightList map[APIModule.ModuleType]map[APIModule.ModuleType]bool
}

// Register a new module into ModuleManager
// returns error when ID is under register
func (m *Manager) Register(module AvailableModule) error {
	// 1. check if module is already registered
	// 2. register into modules
	m.Lock()
	defer m.Unlock()

	// already
	moduleID := module.GetModuleID()
	if _, ok := m.modules[moduleID]; ok {
		return fmt.Errorf("module with ID %s is already registered", moduleID)
	}

	m.modules[moduleID] = module
	return nil
}

func (m *Manager) Require(reqModule AvailableModule, resType APIModule.ModuleType, resID APIModule.ModuleID) (AvailableModule, error) {
	// 1. check request rightList, if registered type if registered id
	// 2. check if right, if right return res
	m.Lock()
	defer m.Unlock()
	reqType := reqModule.GetModuleType()
	if _, ok := m.rightList[reqType]; !ok {
		return nil, fmt.Errorf("request not registered type: %s", reqType)
	}

	if right, ok := m.rightList[reqType][resType]; !ok {
		return nil, fmt.Errorf("response not registered type: %s", resType)
	} else if !right {
		return nil, fmt.Errorf("req module %s has no authority for %s", reqModule.GetModuleID(), resID)
	}

	return m.modules[resID], nil
}

func (m *Manager) Check(moduleType APIModule.ModuleType) bool {
	m.RLock()
	defer m.Unlock()
	if exist, ok := m.moduleTypeList[moduleType]; !ok {
		return false
	} else if !exist {
		return false
	}

	return true
}

// CheckHealth gets module id
// returns module health and error
// needs to implementation modules' checkhealth first
func (m *Manager) CheckHealth(moduleID APIModule.ModuleID) (APIModule.ModuleHealth, error) {
	// 1. check module first, return error
	// 2. get module, use checkHealth
	m.RLock()
	defer m.Unlock()

	if module, ok := m.modules[moduleID]; !ok {
		return APIModule.ModuleHealth{}, fmt.Errorf("module checkhealth: module %s not registered", moduleID)
	} else {
		return module.CheckHealth(), nil
	}
}

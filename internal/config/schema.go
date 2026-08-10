// Package config defines the typed, Kubernetes-style node configuration
// manifest and the loader that reads, validates, and defaults it. main.go
// consumes the typed NodeConfig instead of scattering key lookups.
package config

import (
	APIProvider "github.com/cpmores/lucinda/api/v1/domain/provider"
)

// Supported manifest identity.
const (
	KindNodeConfig = "NodeConfig"
	APIVersion     = "lucinda.dev/v1"
)

// NodeConfig is the root manifest: apiVersion / kind / metadata / spec,
// modeled after a Kubernetes object.
type NodeConfig struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       NodeSpec `yaml:"spec"`
}

// Metadata identifies the node this manifest configures.
type Metadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

// NodeSpec carries every boot setting.
type NodeSpec struct {
	HTTP            HTTPConfig            `yaml:"http"`
	Transport       TransportConfig       `yaml:"transport"`
	HardwareMonitor HardwareMonitorConfig `yaml:"hardwareMonitor"`
	Providers       []APIProvider.ProviderConfig `yaml:"providers"`
}

// HTTPConfig configures the ingress/egress server.
type HTTPConfig struct {
	Port int `yaml:"port"`
}

// TransportConfig selects the mesh transport.
type TransportConfig struct {
	Type   string        `yaml:"type"`
	Libp2p Libp2pConfig  `yaml:"libp2p"`
}

// Libp2pConfig holds libp2p transport settings.
type Libp2pConfig struct {
	Addrs      []string `yaml:"addrs"`
	OutsLength int64    `yaml:"outsLength"`
	InsLength  int64    `yaml:"insLength"`
}

// HardwareMonitorConfig configures the hardware poll interval.
type HardwareMonitorConfig struct {
	IntervalSec int64 `yaml:"intervalSec"`
}

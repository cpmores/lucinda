package config

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// defaultSearchPaths are tried when no explicit config path is given.
var defaultSearchPaths = []string{
	"configs/server/config.yaml",
	"config.yaml",
}

// Load reads, validates, and defaults a config manifest. An empty path falls
// back to the default search paths.
func Load(path string) (*NodeConfig, error) {
	p := resolvePath(path)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", p, err)
	}
	var cfg NodeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", p, err)
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

// Path resolves the config path from an explicit flag, the LUCINDA_CONFIG
// env var, or the default search paths.
func Path(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("LUCINDA_CONFIG"); env != "" {
		return env
	}
	return resolvePath("")
}

func resolvePath(path string) string {
	if path != "" {
		return path
	}
	for _, c := range defaultSearchPaths {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return defaultSearchPaths[0]
}

// validate checks the manifest identity and required fields.
func validate(cfg *NodeConfig) error {
	if cfg.Kind != KindNodeConfig {
		return fmt.Errorf("config kind %q, want %q", cfg.Kind, KindNodeConfig)
	}
	if cfg.APIVersion != APIVersion {
		return fmt.Errorf("config apiVersion %q unsupported, want %q", cfg.APIVersion, APIVersion)
	}
	return nil
}

// applyDefaults fills optional fields so a minimal manifest boots.
func applyDefaults(cfg *NodeConfig) {
	if cfg.Spec.HTTP.Port == 0 {
		cfg.Spec.HTTP.Port = 9090
	}
	if cfg.Spec.Transport.Type == "" {
		cfg.Spec.Transport.Type = "libp2p"
	}
	if len(cfg.Spec.Transport.Libp2p.Addrs) == 0 {
		cfg.Spec.Transport.Libp2p.Addrs = []string{"/ip4/0.0.0.0/tcp/0"}
	}
	if cfg.Spec.Transport.Libp2p.OutsLength <= 0 {
		cfg.Spec.Transport.Libp2p.OutsLength = 20
	}
	if cfg.Spec.Transport.Libp2p.InsLength <= 0 {
		cfg.Spec.Transport.Libp2p.InsLength = 100
	}
	if cfg.Spec.HardwareMonitor.IntervalSec <= 0 {
		cfg.Spec.HardwareMonitor.IntervalSec = 5
	}
}

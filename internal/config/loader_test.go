package config

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const validManifest = `apiVersion: lucinda.dev/v1
kind: NodeConfig
metadata:
  name: node-a
spec:
  http:
    port: 9091
  transport:
    type: libp2p
    libp2p:
      addrs: ["/ip4/0.0.0.0/tcp/0"]
      outsLength: 20
      insLength: 100
  hardwareMonitor:
    intervalSec: 3
  providers:
    - id: vllm
      driver: vllm
      host: localhost
      port: 8000
`

func TestLoadValid(t *testing.T) {
	cfg, err := Load(writeTemp(t, validManifest))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Kind != KindNodeConfig || cfg.APIVersion != APIVersion {
		t.Fatalf("identity: %s/%s", cfg.Kind, cfg.APIVersion)
	}
	if cfg.Metadata.Name != "node-a" {
		t.Fatalf("metadata name = %q", cfg.Metadata.Name)
	}
	if cfg.Spec.HTTP.Port != 9091 {
		t.Fatalf("port = %d, want 9091", cfg.Spec.HTTP.Port)
	}
	if len(cfg.Spec.Providers) != 1 || cfg.Spec.Providers[0].ID != "vllm" {
		t.Fatalf("providers = %+v", cfg.Spec.Providers)
	}
}

func TestLoadInvalidKind(t *testing.T) {
	_, err := Load(writeTemp(t, `apiVersion: lucinda.dev/v1
kind: Deployment
spec: {}`))
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
}

func TestLoadInvalidAPIVersion(t *testing.T) {
	_, err := Load(writeTemp(t, `apiVersion: v1
kind: NodeConfig
spec: {}`))
	if err == nil {
		t.Fatal("expected error for unsupported apiVersion")
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := Load(writeTemp(t, `apiVersion: lucinda.dev/v1
kind: NodeConfig
spec: {}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Spec.HTTP.Port != 9090 {
		t.Fatalf("default port = %d, want 9090", cfg.Spec.HTTP.Port)
	}
	if cfg.Spec.Transport.Libp2p.OutsLength != 20 || cfg.Spec.Transport.Libp2p.InsLength != 100 {
		t.Fatalf("default buffers: %d/%d", cfg.Spec.Transport.Libp2p.OutsLength, cfg.Spec.Transport.Libp2p.InsLength)
	}
	if cfg.Spec.HardwareMonitor.IntervalSec != 5 {
		t.Fatalf("default interval = %d", cfg.Spec.HardwareMonitor.IntervalSec)
	}
}

func TestRoundTripStable(t *testing.T) {
	cfg, err := Load(writeTemp(t, validManifest))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg2, err := Load(writeTemp(t, string(out)))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg2.Spec.HTTP.Port != cfg.Spec.HTTP.Port || len(cfg2.Spec.Providers) != len(cfg.Spec.Providers) {
		t.Fatalf("round-trip changed config: %+v vs %+v", cfg, cfg2)
	}
}

func TestPathOverride(t *testing.T) {
	if p := Path("custom.yaml"); p != "custom.yaml" {
		t.Fatalf("flag path = %q", p)
	}
	t.Setenv("LUCINDA_CONFIG", "env.yaml")
	if p := Path(""); p != "env.yaml" {
		t.Fatalf("env path = %q", p)
	}
}

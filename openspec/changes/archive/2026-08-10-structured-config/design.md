## Context

`main.go` currently bootstraps from a flat viper config with scattered `v.Get(...)` calls, and the provider controller unmarshals its section from a viper handle. There is no typed schema, no validation/defaulting pass, and no way to point a node at an alternate manifest — so real multi-node runs are impractical.

## Goals / Non-Goals

**Goals:**
- A typed, K8s-style manifest (`apiVersion`/`kind`/`metadata`/`spec`) as the configuration contract.
- A config package that loads, validates, and defaults a manifest in one step.
- `main.go` reads typed fields; no string-key lookups.
- A config-path override so each mesh node runs its own manifest.

**Non-Goals:**
- Dynamic config reload / hot-update (K8s watch semantics). Config is read once at boot.
- Centralized config distribution (K8s ConfigMap/etcd). Config is file-based per node.
- Schema validation beyond kind/apiVersion/required fields — full JSON-schema-style validation is future work.

## Decisions

### D1. Manifest structure mirrors Kubernetes

```yaml
apiVersion: lucinda.dev/v1
kind: NodeConfig
metadata:
  name: node-a
spec:
  http:
    port: 9090
  transport:
    type: libp2p
    libp2p:
      addrs: ["/ip4/0.0.0.0/tcp/0"]
      outsLength: 20
      insLength: 100
  hardwareMonitor:
    intervalSec: 5
  providers:
    - id: vllm
      driver: vllm
      host: localhost
      port: 8000
      models:
        - id: qwen-2.5-gptq
          labels:
            employer: "TaskPlanner,TaskCommander,TaskExecutor"
```

Typed structs:

```go
type NodeConfig struct {
    APIVersion string   `yaml:"apiVersion"`
    Kind       string   `yaml:"kind"`
    Metadata   Metadata `yaml:"metadata"`
    Spec       NodeSpec `yaml:"spec"`
}
type NodeSpec struct {
    HTTP            HTTPConfig
    Transport       TransportConfig
    HardwareMonitor HardwareMonitorConfig
    Providers       []APIProvider.ProviderConfig
}
```

- **Alternatives considered**: keep flat viper keys. Rejected — untyped, unvalidated, per-key lookups in `main.go`.
- **Trade-off**: **BREAKING** config format; contained to `main.go` + the config file(s).

### D2. A `internal/config` package owns loading

```go
// Load reads, unmarshals, validates, and defaults a manifest.
func Load(path string) (*NodeConfig, error)
// Path returns the config path from flag/env with a default.
func Path(flagValue string) string
```

Validation: `kind == "NodeConfig"`, supported `apiVersion`, non-zero HTTP port. Defaults: port 9090, transport buffers, hardware interval.

- **Trade-off**: one more package; keeps `main.go` thin and the logic testable.

### D3. Path override for multi-node

`-config <path>` flag (or `LUCINDA_CONFIG` env) selects the manifest. `scripts/start_lucinda.sh` gains a `CONFIG=` env to pass it through. This is the enabler for real two-node smoke tests.

- **Alternatives considered**: hardcoding multiple configs. Rejected — a path override is the general mechanism.

### D4. ProviderController takes typed configs

`LoadProviders` changes from `(config *viper.Viper)` to `(configs []APIProvider.ProviderConfig)`, so the provider section flows typed from the manifest.

- **Trade-off**: `LoadProviders` loses its viper coupling; `main.go` passes `cfg.Spec.Providers`.

### D5. Bootstrap reads the typed object

`main.go` builds every component from `cfg.Spec.*` fields. The only global flag is `-config`.

## Risks / Trade-offs

- [Config format is BREAKING] → Mitigation: contained to `main.go` + config file; smoke scripts and README updated in the same change.
- [YAML mapping (snake_case keys ↔ struct tags) drift] → Mitigation: `yaml` tags on every field; a round-trip test (Load → Marshal → Load) catches drift.
- [viper removal touches provider controller] → Mitigation: `ProviderConfig` type already exists in the API; only the loading entry point changes.

## Migration Plan

1. Add `internal/config`: schema structs + `Load` + `Path` + defaults + validation.
2. Change `ProviderController.LoadProviders` to take typed configs.
3. Rewrite `main.go`: `cfg := config.Load(config.Path(flag))`; read typed fields.
4. Reformat `configs/server/config.yaml` to the manifest; add `config-nodeB.yaml` for multi-node.
5. Update `start_lucinda.sh` (CONFIG= passthrough) and the README config section.
6. Tests: config package unit tests (valid/invalid/defaults/override) + round-trip.

## Open Questions

- Should `metadata.name` become the node's identity anywhere beyond config (e.g. transport NodeID prefix), or stay informational? Currently informational.
- Should provider `apiKey`/`baseURL` stay in the manifest or move to secrets/env? Future concern — kept in the manifest for now.

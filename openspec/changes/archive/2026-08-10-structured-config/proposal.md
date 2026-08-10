## Why

Configuration is currently **flat and scattered in `main.go`**: viper `Get` calls pull `http.port`, `transport.libp2p.addrs`, `hardware_monitor.interval_sec`, etc. one by one, and the provider config is unmarshaled ad hoc. There is no typed schema, no first-class validation/defaulting, and no way to run multiple nodes with different config files (needed for real multi-node smoke tests). This mirrors the problem Kubernetes solved with declarative manifests.

## What Changes

- **Typed, K8s-style config manifest**: `apiVersion` / `kind` / `metadata` / `spec`, unmarshaled into typed Go structs instead of viper lookups. **BREAKING**: the flat `config.yaml` format is replaced; `main.go` stops calling viper directly.
- **Config package**: a `internal/config` package that loads, validates, and applies defaults to a manifest — a first-class step, not inline logic.
- **Path override**: the config file path is selectable via a CLI flag / env var, so each node in a mesh can run its own manifest (e.g. different ports, different providers).
- **`main.go` consumes the typed `NodeConfig`**: reads `cfg.Spec.HTTP.Port`, `cfg.Spec.Transport`, etc., and hands the provider specs to the controller.

## Capabilities

### New Capabilities

- `typed-config-schema`: the declarative manifest schema (`apiVersion`/`kind`/`metadata`/`spec`) with typed Go structs — a stable, documented configuration contract.
- `config-loading`: the loader — YAML → typed `NodeConfig` with validation (kind, apiVersion, required fields) and sane defaults; plus path override for multi-node runs.

### Modified Capabilities

<!-- No existing specs are changing; both capabilities are new. -->

## Impact

- `cmd/pc/main.go` — `loadConfig` rewritten to use the Config package; all viper `Get` calls removed.
- `internal/config/` — new package (schema structs + loader + defaults + validation).
- `configs/server/config.yaml` — reformatted to the manifest structure.
- `configs/server/` — additional per-node manifests for multi-node runs.
- `pkg/infrastructure_layer/provider` — `ProviderConfig` reuse; `LoadProviders` may take typed configs instead of a viper handle.
- `configs/server/config.yaml` consumers — README, smoke scripts (config path awareness).

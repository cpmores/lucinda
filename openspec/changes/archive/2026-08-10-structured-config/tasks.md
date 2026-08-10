## 1. Config Schema (typed-config-schema)

- [x] 1.1 Add `internal/config`: `NodeConfig{APIVersion, Kind, Metadata, Spec}` + `NodeSpec{HTTP, Transport, HardwareMonitor, Providers}` typed structs with `yaml` tags
- [x] 1.2 Map `spec.transport`, `spec.http`, `spec.hardwareMonitor`, `spec.providers` to the existing API types (e.g. `APIProvider.ProviderConfig`)

## 2. Config Loading (config-loading)

- [x] 2.1 `config.Load(path)`: read YAML → unmarshal → validate (kind, apiVersion, required fields)
- [x] 2.2 Defaults: HTTP port, transport buffer sizes, hardware interval; minimal manifest boots
- [x] 2.3 `config.Path(flagValue)`: resolve from `-config` flag / `LUCINDA_CONFIG` env with a default
- [x] 2.4 `ProviderController.LoadProviders` takes typed `[]APIProvider.ProviderConfig` instead of a viper handle

## 3. Bootstrap Integration

- [x] 3.1 Rewrite `main.go`: load via `config.Load`, read every setting from `cfg.Spec.*`, remove viper `Get` calls
- [x] 3.2 Reformat `configs/server/config.yaml` to the manifest structure
- [x] 3.3 Add `configs/server/config-nodeB.yaml` (different port + a model node A lacks) for multi-node runs
- [x] 3.4 `start_lucinda.sh` passes a `CONFIG=` override through to the binary

## 4. Verification

- [x] 4.1 Config unit tests: valid manifest loads, invalid fails, defaults apply, override honored, round-trip (Load → Marshal → Load) stable
- [x] 4.2 Server boots with the reformatted config; `/healthz` responds
- [x] 4.3 Smoke tests still pass (simple + complex) with the new config
- [x] 4.4 Full `-race` suite green

---
change_name: "structured-config"
schema: spec
version: 0.0.1
---

# Config Loading

## Purpose

Defines how the node loads its configuration: a config package reads a YAML manifest into the typed `NodeConfig` with validation, applies sensible defaults for optional fields, honors an overridable config path, and `main.go` consumes the typed object for all boot settings.

## Requirements

### Requirement: Config loads from a YAML file with validation

A config package SHALL load a YAML manifest, unmarshal it into the typed `NodeConfig`, and validate it (kind, apiVersion, and required fields such as the HTTP port) before returning.

#### Scenario: valid manifest loads

- **WHEN** a well-formed `NodeConfig` manifest is loaded
- **THEN** the loader returns the typed config with all sections populated

#### Scenario: invalid manifest fails with an error

- **WHEN** a manifest has the wrong kind or is missing required fields
- **THEN** loading fails with a descriptive error

### Requirement: Config applies sensible defaults

The loader SHALL apply defaults for optional fields so a minimal manifest boots: a default HTTP port, transport buffer sizes, and hardware-monitor interval.

#### Scenario: minimal manifest boots with defaults

- **WHEN** a manifest omits optional settings
- **THEN** the loader fills them with documented defaults

### Requirement: The config path is overridable

The node SHALL accept a config path from a CLI flag or environment variable, so different nodes in a mesh can run different manifests.

#### Scenario: alternate config path is honored

- **WHEN** a node is started with an explicit config path
- **THEN** it loads that manifest instead of the default

### Requirement: main.go consumes the typed config

`cmd/pc/main.go` SHALL read all boot settings from the typed `NodeConfig` object rather than calling the config library directly per key.

#### Scenario: bootstrap uses typed fields

- **WHEN** `main.go` boots the server
- **THEN** it reads `cfg.Spec.HTTP.Port`, `cfg.Spec.Transport`, etc. with no string-key lookups

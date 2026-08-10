## ADDED Requirements

### Requirement: Configuration is a declarative typed manifest

The node configuration SHALL be expressed as a manifest with `apiVersion`, `kind`, `metadata`, and `spec`, and SHALL unmarshal into typed Go structs. The `kind` SHALL be `NodeConfig`.

#### Scenario: manifest loads into typed fields

- **WHEN** a manifest declares `kind: NodeConfig` with HTTP, transport, hardware-monitor, and provider sections
- **THEN** it unmarshals into a typed `NodeConfig` whose fields (e.g. `spec.http.port`) are read without string-key lookups

### Requirement: The schema is versioned

The manifest SHALL carry an `apiVersion`; the loader SHALL accept manifests matching the supported version and reject unknown ones.

#### Scenario: unknown apiVersion is rejected

- **WHEN** a manifest carries an unsupported `apiVersion`
- **THEN** loading fails with a clear error

### Requirement: The manifest is self-describing

The manifest SHALL identify the node via `metadata.name` so a mesh can distinguish nodes, and the spec SHALL cover all boot settings: HTTP, transport, hardware monitor, and providers.

#### Scenario: node name is present

- **WHEN** a manifest declares `metadata.name`
- **THEN** the loader exposes it as the node's config identity

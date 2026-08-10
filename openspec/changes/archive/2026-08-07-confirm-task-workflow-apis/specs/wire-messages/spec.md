## ADDED Requirements

### Requirement: taskmsg defines broadcast, assign, and result wire messages

The `api/v1/messaging/taskmsg` package SHALL define wire message types for the three cross-node exchanges: a broadcast (task advertisement / availability), an assignment (a task leased to an executor), and a result (the executor's output routed back).

#### Scenario: Broadcast advertises a task

- **WHEN** a node has a task ready for the mesh
- **THEN** it sends a broadcast message describing the task's spec and `Owner`

#### Scenario: Assignment leases a task to an executor

- **WHEN** a node decides to execute a task on behalf of another node
- **THEN** the assignment message carries the task spec, task ID, and `Owner`

#### Scenario: Result returns output to the origin

- **WHEN** an executor finishes a task assigned by another node
- **THEN** the result message carries the task ID, the output, and is addressed to the `Owner` node

### Requirement: Assignment messages carry the owner NodeID

Every assignment message SHALL include the `Owner` NodeID so the executing node can decide between local and remote handling and know where to send results and telemetry.

#### Scenario: Executor routes by owner identity

- **WHEN** an executor receives an assignment
- **THEN** if the `Owner` is its own node ID it treats the task as local, otherwise it sends results back to the `Owner` node

### Requirement: taskmsg messages are transport-serializable

Every taskmsg wire message SHALL be encodable to and decodable from a `NodeMessage` body so it can traverse the Transport layer between nodes.

#### Scenario: Message survives a cross-node round trip

- **WHEN** a node sends a taskmsg message to another node over Transport
- **THEN** the receiving node decodes the identical message content from the `NodeMessage` body

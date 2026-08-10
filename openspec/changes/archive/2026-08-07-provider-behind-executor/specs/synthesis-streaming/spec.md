## ADDED Requirements

### Requirement: The final answer streams from the executing node

The final answer's token chunks SHALL be generated on the node that executes the synthesis task — possibly remote — and streamed over the dedicated transport protocol to the plan owner, independent of where the commander runs. The owner node's monitor SHALL reconstruct a local chunk channel feeding the SSE.

#### Scenario: commander and executor are on different nodes

- **WHEN** a plan's commander runs on one node and its synthesis task executes on another
- **THEN** the answer's chunks travel from the executing node to the owner node over the stream protocol and reach the SSE stream

### Requirement: Exactly one done frame closes the stream

A streamed answer SHALL end with exactly one `done` SSE frame carrying the terminal plan status, regardless of the number of synthesis chunks or the distance between nodes.

#### Scenario: streamed answer closes cleanly

- **WHEN** the final answer finishes streaming
- **THEN** exactly one `done` SSE frame is emitted and the stream closes

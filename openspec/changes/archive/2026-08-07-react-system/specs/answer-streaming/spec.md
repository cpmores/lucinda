## ADDED Requirements

### Requirement: The final ReAct answer streams over a dedicated transport protocol

When the reasoning loop reaches `done`, the commander SHALL stream the answer via `provider.Stream`, route the chunks over the dedicated stream protocol, and have the owner node's monitor deliver them as SSE `stream` frames. Raw tokens SHALL NOT be published on the EventBus.

#### Scenario: done decision streams the answer

- **WHEN** the reasoning loop returns `done`
- **THEN** the answer's token chunks reach the client as SSE `stream` frames via the stream router

### Requirement: Intermediate reasoning is not streamed

Reasoning steps SHALL use one-shot `Generate` and produce structured results only; no token stream SHALL be emitted for intermediate thoughts.

#### Scenario: no stream frames for reasoning

- **WHEN** the commander is reasoning or an action is executing
- **THEN** no `stream` SSE frames are emitted

### Requirement: The stream terminates with a single done frame

A streamed answer SHALL end with exactly one `done` SSE frame carrying the terminal plan status.

#### Scenario: streamed answer closes cleanly

- **WHEN** the final answer finishes streaming
- **THEN** exactly one `done` SSE frame is emitted and the stream closes

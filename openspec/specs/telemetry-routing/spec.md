---
change_name: "confirm-task-workflow-apis"
schema: spec
version: 0.0.1
---

# Telemetry Routing

## Purpose

Defines user-facing progress reporting: telemetry status events unicast to the plan owner node, and the final-answer token stream carried over a dedicated Transport protocol (never the EventBus), with `{owner, planID, taskID}` demux on the owner node.

## Requirements

### Requirement: Components emit telemetry status events

Each workflow component SHALL emit telemetry status events when its state changes: the planner on `planning`/`planned`, the commander on `thinking`/`waiting`/`finalizing`, and the executor on `running`/`done`. These events are for user-facing progress display, distinct from internal coordination events.

#### Scenario: Planner announces planning state

- **WHEN** TaskPlanner begins decomposing a raw task
- **THEN** it emits a `planning` telemetry event

#### Scenario: Executor announces a running task

- **WHEN** a TaskExecutor starts executing a task
- **THEN** it emits a `running` telemetry event that identifies the task and model

### Requirement: Telemetry is unicast to the plan owner node

Telemetry events SHALL be routed to the plan's `Owner` node (single destination), not broadcast to the mesh. Local events (produced on the owner node) SHALL reach the monitor directly; remote events SHALL traverse the Transport layer as a unicast addressed to the `Owner`.

#### Scenario: Remote executor telemetry reaches the owner

- **WHEN** a node other than the owner produces a telemetry event
- **THEN** the event is delivered to the owner node's monitor and no other node receives it

#### Scenario: Local telemetry is not sent over the network

- **WHEN** the owner node's own component produces a telemetry event
- **THEN** the event reaches the local monitor without traversing Transport

### Requirement: The final answer token stream uses a dedicated transport protocol

The final-answer token stream SHALL travel over a dedicated Transport protocol (e.g. `/lucinda/stream/1.0.0`) as a sequence of chunk messages. Raw token chunks SHALL NOT be published on the EventBus. The owner node SHALL reconstruct a local `<-chan StreamChunk` from the received chunks and feed it to the SSE handler.

#### Scenario: Remote final answer streams to the user

- **WHEN** the final answer is produced by a remote executor
- **THEN** its token chunks cross the mesh on the stream protocol and the owner node reconstructs a local chunk channel for the SSE stream

#### Scenario: Tokens do not saturate the control plane

- **WHEN** the final answer is being streamed
- **THEN** no token chunk is published as an EventBus event

### Requirement: Telemetry payloads carry owner, plan ID, and task ID for demux

Every telemetry payload and every stream chunk SHALL carry the plan owner NodeID, the plan ID, and the task ID (for step-level events). The owner node SHALL demux incoming telemetry by plan ID so concurrent sessions remain isolated.

#### Scenario: Concurrent plans demux correctly

- **WHEN** the owner node receives telemetry for two different plan IDs
- **THEN** each plan's telemetry is routed to its own SSE stream without cross-talk

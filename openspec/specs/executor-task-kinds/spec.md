---
change_name: "provider-behind-executor"
schema: spec
version: 0.0.1
---

# Executor Task Kinds

## Purpose

Defines how TaskExecutor distinguishes task kinds via a spec marker — `reason`, `execute`, and `synthesize` — and chooses provider behavior accordingly: Generate for reason/execute, Stream for synthesize. A synthesize task streams its chunks toward the plan owner via the stream router.

## Requirements

### Requirement: Executor distinguishes task kinds

TaskExecutor SHALL recognize three task kinds via a spec marker: `reason` (produce a structured decision), `execute` (produce a work result), and `synthesize` (produce the final answer). The executor SHALL choose provider behavior accordingly — Generate for reason/execute, Stream for synthesize.

#### Scenario: reason task produces a decision

- **WHEN** the executor runs a reason-marked task
- **THEN** it calls provider.Generate with the reasoning prompt and returns the decision text as the task output

#### Scenario: execute task produces a result

- **WHEN** the executor runs an execute-marked task
- **THEN** it calls provider.Generate and returns the work output

#### Scenario: synthesize task streams

- **WHEN** the executor runs a synthesize-marked task
- **THEN** it calls provider.Stream and routes each chunk toward the plan owner via the stream router

### Requirement: A synthesis task streams through the router

For a synthesize task, the executor SHALL send `StreamChunkMsg` chunks over the dedicated stream protocol to the plan owner node, where the monitor reconstructs a local channel for the SSE stream.

#### Scenario: remote synthesis streams to the owner

- **WHEN** a synthesis task runs on a node other than the owner
- **THEN** its chunks cross the mesh on the stream protocol and reach the owner's SSE stream

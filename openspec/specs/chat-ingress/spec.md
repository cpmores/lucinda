---
change_name: "confirm-task-workflow-apis"
schema: spec
version: 0.0.1
---

# Chat Ingress

## Purpose

Defines the HTTP surface of the task workflow: `POST /chat` turns a `ChatRequest` into a plan on the owner node and returns its plan ID; `GET /stream?plan=<id>` emits the ordered SSE frame stream (`status`, `step_result`, `stream` chunk, `done`) back to the user.

## Requirements

### Requirement: User can submit a chat request

The system SHALL accept a `ChatRequest` via `POST /chat` and start the task workflow for it. The request SHALL contain messages and MAY contain a model and options. The endpoint SHALL return the plan ID that identifies the resulting workflow run.

#### Scenario: Valid request starts a workflow

- **WHEN** a user sends `POST /chat` with a valid `ChatRequest`
- **THEN** the system returns a plan ID and the task workflow begins

#### Scenario: Request with no messages is rejected

- **WHEN** a user sends `POST /chat` with an empty `messages` array
- **THEN** the system returns an error response and does not start a workflow

### Requirement: User receives the workflow result as an ordered SSE stream

The system SHALL expose `GET /stream?plan=<id>` returning a Server-Sent Events stream. Frames SHALL arrive in causal order and SHALL be one of four types: `status` (component state change), `step_result` (a completed sub-task's output), `stream` (a token chunk of the final answer), or `done` (terminal plan result). Exactly one `done` frame SHALL be emitted per plan.

#### Scenario: Final answer is streamed as token chunks

- **WHEN** a plan reaches its final-answer stage
- **THEN** the stream emits `stream` frames containing token deltas followed by a single `done` frame

#### Scenario: Intermediate progress is visible before the final answer

- **WHEN** the workflow is executing a plan
- **THEN** the stream emits `status` frames as components start and finish, and `step_result` frames as sub-tasks complete, before any `stream` frame

#### Scenario: Plan timeout terminates the stream

- **WHEN** a plan exceeds its deadline before completing
- **THEN** the stream emits a `done` frame with a timeout status and then closes

### Requirement: SSE streams are isolated per plan

The system SHALL scope every SSE stream to a single plan ID and SHALL NOT interleave frames from different plans or sessions. A plan ID that does not exist SHALL NOT produce frames.

#### Scenario: Concurrent plans do not mix frames

- **WHEN** two plans run concurrently and a client opens a stream for each
- **THEN** each stream carries only frames belonging to its own plan

#### Scenario: Unknown plan ID yields no stream

- **WHEN** a client opens a stream for a plan ID that does not exist
- **THEN** the system returns an error and emits no frames

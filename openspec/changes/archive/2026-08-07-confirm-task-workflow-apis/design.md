## Context

The Task Workflow layer is being rebuilt around three optional components — TaskPlanner, TaskCommander, TaskExecutor — that communicate over the EventBus and live on possibly different nodes in a libp2p mesh. `internal/` is currently empty; the prior StateManager/TaskReducer flow was removed. Components are loadable independently ("components equal configurations and capabilities"). The API surface across the workflow — from the user's `ChatRequest` through these components and back to the SSE output — is not yet pinned down, which blocks distributed implementation.

Existing API pieces: `ChatRequest`/`StreamChunk`/`ChatResponse` (`api/v1/domain/chat`), `Task`/`TaskPlan`/`TaskSpec`/`TaskNode`/`TaskAd`/`PlanResult` (`api/v1/domain/task`, incl. `Owner`), `Event`/`EventType` (`api/v1/messaging/event`), `NodeID`/`NodeMessage`/`Protocol` (`api/v1/domain/node`). The `taskmsg` package was deleted and needs rebuilding.

## Goals / Non-Goals

**Goals:**
- Pin down the end-to-end API contract: HTTP ingress → workflow payloads → cross-node wire messages → SSE egress.
- Establish the "single user-facing stream" principle: only the final answer is streamed; all intermediate steps are one-shot structured results.
- Make `Owner` NodeID the single routing key for both results and telemetry.
- Keep every component optionally loadable; the contract must not hard-wire a chain.

**Non-Goals:**
- ReAct implementation details (Phase 2). This change confirms the API contract and its telemetry/streaming backbone only.
- Provider capability expansion (image/video drivers, toolbox/MCP) — Executor "can do" whatever a node's capabilities declare; the API contract stays agnostic.
- Session/context persistence (`feat/context-manager`) — demux by planID is covered; durable sessions are out of scope.

## Decisions

### D1. One user-facing stream; everything else is structured

Only the final answer's token stream reaches the user. Every intermediate hop (planner decomposition, commander reasoning, executor sub-work) uses one-shot `Generate` → structured result → next component. This keeps the control plane (EventBus) unsaturated and gives the user Claude-Code-like progress without raw thought leakage.

- **Alternatives considered**: streaming every component's output. Rejected — floods the control plane and leaks internal reasoning.
- **Trade-off**: the user sees intermediate results only as `step_result` summaries, not raw streams.

### D2. `Owner` NodeID is the single routing key

Every task carries `Owner` (from `TaskPlan.Owner` → `Task.Meta.Owner` → `TaskAd.Owner`). It serves both result routing (executor decides local-vs-remote, sends results back) and telemetry routing (all user-facing progress unicasts to Owner). No second `OriginNodeID` field is introduced — the two responsibilities are one field.

- **Alternatives considered**: separate origin vs telemetry-owner fields. Rejected — they always coincide with the plan owner node and would create sync bugs.
- **Trade-off**: Owner conflates "who created the plan" and "who hosts the SSE". Assumed to always be the same node (the HTTP ingress node).

### D3. Two event classes, two routing modes

- **Coordination events** (`task.ready` / `task.assigned` / `task.done`): broadcast to the mesh, consumed by the TaskBoard for capability bidding.
- **User telemetry events** (planner/commander/executor state, step results): unicast to Owner, consumed by a read-only monitor.

The EventBus is pub/sub, so the monitor subscribes alongside consumers without stealing control flow.

- **Alternatives considered**: one routing mode for everything. Rejected — board bidding needs mesh-wide visibility, user progress does not.
- **Trade-off**: two logical paths to reason about; mitigated by keeping telemetry as a small, well-defined event set.

### D4. Token stream over a dedicated Transport protocol, never the EventBus

`<-chan StreamChunk` is a local memory channel and cannot cross the network. Remote final-answer chunks are packed as `StreamChunk` messages over a dedicated protocol (e.g. `/lucinda/stream/1.0.0`); the owner node reconstructs a local `<-chan StreamChunk` for the SSE handler. Tokens never ride the EventBus.

- **Alternatives considered**: streaming tokens over the EventBus. Rejected — saturates the control plane (existing CLAUDE.md constraint).
- **Trade-off**: chunk-per-message transport overhead on the data plane; acceptable for a mesh.

### D5. TaskMonitor: a read-only aggregator on the owner node

The owner node hosts a monitor that subscribes to local EventBus telemetry, receives remote telemetry via the Postman bridge, and reconstructs the stream protocol. It merges everything into one ordered SSE frame stream and demuxes by planID. It never consumes control flow, so optional component loading and topology changes do not affect it.

- **Alternatives considered**: each component pushing directly to SSE. Rejected — components must not know about HTTP/SSE (TaskWrapper is external utils).
- **Trade-off**: one aggregation point on the owner node; a single point of failure for that session, acceptable per-plan.

### D6. taskmsg rebuilt around the three cross-node exchanges

`api/v1/messaging/taskmsg` is reintroduced with broadcast / assign / result message types, each encodable as a `NodeMessage` body, each carrying the task ID, spec, and `Owner`.

- **Alternatives considered**: reusing domain types directly as wire bodies. Rejected — wire messages need protocol/from/to metadata and stable serialization.
- **Trade-off**: a parallel type set to the domain types; kept minimal (three messages).

## Risks / Trade-offs

- [Two routing modes (broadcast vs unicast) could drift] → Mitigation: telemetry is a small closed event set; `Owner` is the single key for all unicast.
- [Owner conflates creator and SSE host] → Mitigation: documented assumption; if a future topology splits them, introduce an explicit `sessionHost` field rather than overloading.
- [Chunk-per-message streaming overhead on the data plane] → Mitigation: acceptable for a mesh; revisit with batching if profiling warrants.
- [planID demux on the owner node is a session-local single point of failure] → Mitigation: per-plan scope, no cross-plan coupling; crash of one stream does not affect others.

## Migration Plan

1. Introduce API types first (taskmsg, telemetry event types, SSE frame envelopes) — additive, no behavior change.
2. Implement components against the contract (Planner → Commander → Executor), keeping each optionally loadable.
3. Wire telemetry + stream protocol behind the existing Postman/Transport, defaulting to local fast path.
4. Land the HTTP ingress/egress (`/chat`, `/stream`) last, once the workflow returns structured results.
5. Rollback: the API types are additive; any component can be unloaded without breaking the contract.

## Open Questions

- Does the final-answer synthesis run on the Commander's LLM (reduce-style) or as a designated "final task" on an Executor? This decides whether the stream originates on Commander or Executor, but the API contract (a stream protocol + SSE frame type) is identical either way.
- Should `Owner` be upgraded from `string` to the typed `APINode.NodeID`? Type safety vs churn in type assertions.
- Are `step_result` frames desired for every sub-task, or only terminal ones? Affects SSE volume, not the contract.

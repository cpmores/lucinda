## Why

The Task Workflow layer (`internal/task_workflow_layer/`) is being rebuilt from scratch around three optional components — TaskPlanner, TaskCommander, TaskExecutor — but their API contract, from the user's `ChatRequest` to the final SSE output, has never been pinned down. Without a confirmed contract, components that live on different nodes (telemetry routing, token streaming, `Owner` NodeID) can't be implemented against a stable interface.

## What Changes

- Define the full task workflow API surface end-to-end: HTTP ingress (`ChatRequest` → planID), workflow payloads (rawTask / `TaskPlan` / `Task` / `TaskResult`), and SSE egress frames (status / step_result / stream / done).
- Pin down each component's contract: what it consumes, what it produces, and where that result is routed (Planner → Commander → Executor → Planner → Wrapper).
- Rebuild the cross-node wire messages (`api/v1/messaging/taskmsg/`) — deleted in the new architecture — with the `Owner` NodeID as the unified result/telemetry routing key.
- Add telemetry + streaming contracts: user-facing progress events unicast to the plan owner node, and the final-answer token stream carried over a dedicated Transport protocol (never the EventBus).
- Add the `EventType` constants needed to drive these contracts (task planned/done/completed, telemetry status).
- **BREAKING**: `api/v1/messaging/taskmsg` is reintroduced; any code still importing the old `transport.Transport` interface shape will need updating.

## Capabilities

### New Capabilities

- `chat-ingress`: HTTP surface — `POST /chat` turns a `ChatRequest` into a plan on the owner node; `GET /stream?plan=<id>` emits the ordered SSE frame stream (status, step_result, stream chunk, done) back to the user.
- `workflow-contracts`: internal payloads and per-component API — rawTask, `TaskPlan`, `Task`, `TaskSpec`, `TaskAd`, `TaskResult`; what each component consumes/produces and where the result is routed.
- `wire-messages`: cross-node message types in `taskmsg` (broadcast / assign / result) carrying `Owner` NodeID for result routing.
- `telemetry-routing`: user-facing progress — telemetry events unicast to the plan owner node and the final-answer token stream over a dedicated Transport protocol, with `{owner, planID, taskID}` demux on the owner node.

### Modified Capabilities

<!-- No existing specs in openspec/specs/; all capabilities above are new. -->

## Impact

- `api/v1/domain/chat` — `ChatRequest` / `StreamChunk` / `ChatResponse` already exist; SSE frame envelope types added.
- `api/v1/domain/task` — `Task`, `TaskPlan`, `TaskSpec`, `TaskNode`, `TaskAd`, `PlanResult` already exist (incl. `Owner`); `TaskResult` may be added.
- `api/v1/messaging/event` — new `EventType` constants for task planned/done/completed and telemetry status.
- `api/v1/messaging/taskmsg` — **rebuilt** from scratch.
- `api/v1/registry/module` — `ModuleType` entries for the three workflow components.
- `internal/task_workflow_layer/` — TaskPlanner / TaskCommander / TaskExecutor.
- `internal/task_management_layer/` — TaskBoard / TaskTracer / Postman (telemetry bridging).
- `cmd/pc/main.go` — bootstrap of the workflow components.
- `configs/server/config.yaml` — model labels (`employer`) used for capability-matched provider selection.

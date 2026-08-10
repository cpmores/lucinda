# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build/Run/Test

```bash
# Run the server (config-driven bootstrap via viper)
go run ./cmd/pc/

# All tests
go test ./...
go test ./internal/... ./pkg/...

# Single package
go test -v ./internal/task_management_layer/task_tracer/
go test -v ./internal/task_management_layer/task_board/
go test -v ./internal/task_workflow_layer/task_commander/

# Start / stop the server
bash scripts/start_lucinda.sh          # start (background, pid in /tmp/lucinda.pid)
bash scripts/start_lucinda.sh stop     # stop

# Smoke tests (server must be running, or use --start to launch it)
bash scripts/test_smoke_simple.sh      # plan_execute: status/step_result/done
bash scripts/test_smoke.sh             # react (default): ReAct loop → status/step_result/stream/done
AGENT=plan_execute bash scripts/test_smoke.sh  # same script, simpler plan-and-execute agent
```

Note: `scripts/test_e2e.sh` was removed; the two smoke scripts above are the canonical end-to-end checks.

## Architecture

Lucinda is a **compute-aware distributed agent orchestrator for the edge**. It decomposes user requests into semantic transactions, schedules them across a decentralized mesh based on real-time hardware telemetry, and streams a synthesized answer back. Built in Go 1.26, using libp2p for peer discovery and transport.

### Four-Layer Design (strict top-down dependency)

| Layer | Purpose | Key Components |
|---|---|---|
| **4 — Server & TaskWrapper** | HTTP ingress, ChatRequest → raw task, SSE stream | `internal/user_server/`, `internal/task_wrapper/` |
| **3 — Task Workflow** | LLM decomposition, transaction orchestration, execution, SSE aggregation | `internal/task_workflow_layer/`: `task_planner/`, `task_commander/`, `task_executor/`, `task_monitor/`, `eventx/` |
| **2 — Task Management** | Single state authority, advertise→bid→assign, EventBus↔Transport bridges, observability | `internal/task_management_layer/`: `task_tracer/`, `task_board/`, `postman/`, `telemetry_bridge/`, `stream_router/` |
| **1 — Infrastructure** | EventBus, libp2p Transport, HardwareMonitor, ProviderController, ModuleManager | `pkg/infrastructure_layer/` |

Layers communicate through the **EventBus** for control and user telemetry, and through the **Transport** for cross-node messages. Raw LLM token streams travel on a private Transport protocol (`/lucinda/stream/...`) — never on the EventBus.

### Bootstrap Order (main.go)

```
Config → Logger → ModuleManager → EventBus → Transport → HardwareMonitor → ProviderController (LoadProviders)
→ TaskPlanner → TaskCommander → TaskExecutor → TelemetryBridge → StreamRouter → TaskPostman → TaskBoard → TaskTracer → TaskMonitor
→ VerifyInit → EnableDeps                       # two-phase dependency wiring
→ Start: Transport, HardwareMonitor, StreamRouter, TelemetryBridge, TaskPostman, TaskBoard, TaskMonitor, TaskPlanner, TaskCommander, TaskExecutor
→ HTTP Server (needs the transport's NodeID, so it starts last)
```

All modules implement `AvailableModule`, register with `ModuleManager`, then resolve their declared dependencies via `DependsOn()`/`DependsEnable()`.

### Semantic Transaction Pipeline

Requests are decomposed into **transactions** (semantic units of work), not raw DAG nodes. Each transaction runs under the plan's `Architecture` — `plan_execute` (dispatch the goal once) or `react` (reason → act → observe loop).

1. **TaskWrapper** packs the prompt into a raw `Task` with a `Notify chan<- PlanResult`, registers the plan with TaskMonitor, and publishes `TaskPreplanned`.
2. **TaskPlanner** issues a `reason` task through the board to have an LLM decompose the request into `Transaction`s (id / goal / tools / labels / deps). On the traced result it drops dangling deps, runs a Kahn cycle check, and falls back to a single-transaction plan on any failure. It stores the Notify channel and a per-plan deadline timer, then publishes `TaskPlanned` with the `TaskPlan`.
3. **TaskCommander** ingests the plan, creates one `txRun` per transaction, and starts those whose `Deps` are done (independent ones run in parallel).
   - `plan_execute`: dispatch the transaction goal once as an `execute` task.
   - `react`: issue a `reason` decision task; on `done` issue a `synthesize` task. ReAct history fed to the reasoner is bounded by `maxTrajectorySteps` (5); dependency outputs are injected into the goal context.
4. **TaskTracer is the single state authority.** Components mutate tasks only through `Import` / `Assigned` / `Update` / `SetOutput`; every change emits a `TaskTraced` event that observers (commander, board) advance from. `TaskDone`/`TaskFailed` are deprecated — `TaskTraced` is the only completion signal.
5. **TaskBoard runs assignment** (advertise → bid → assign-best): on `TaskReady` it broadcasts a `TaskAd`, self-bids if locally capable, collects `CapabilityCV` bids for a short window (150ms; 300ms for `reason`), then assigns the highest `Match()` score — locally, or to the winning peer via the postman. Re-advertises up to 3× if no bid qualifies, then fails the plan.
6. **TaskExecutor** runs assigned tasks by kind: `reason`/`execute` use one-shot `Generate`; `synthesize` streams the answer through the StreamRouter. On success it calls `SetOutput` + `Update(Done)`; on failure `Update(Failed)`.
7. **TaskCommander** advances each transaction from `TaskTraced`, cascades to newly-ready transactions, and on all-done merges outputs in topological order and publishes `TaskPlanDone` (exactly once). Any `Failed` action terminates the plan as `PlanError`.
8. **TaskPlanner** maps `TaskPlanDone` (or its own timeout timer) back to the Notify channel and delivers a `PlanResult`; the SSE handler writes the single `done` frame.

### Node State FSM

`TaskTracer` is the authority over the lifecycle. The states it actually tracks:

```
Import   → Ready
Assigned → Running        (executor records this before executing)
Update   → Done | Failed
```

The full `NodeState` set (`pending`/`ready`/`claimed`/`running`/`done`/`failed`/`disposed`) is defined in `api/v1/domain/task`; `claimed`/`disposed` are not wired into the workflow yet. There is **no lease TTL or heartbeat scan** — assignment is a one-shot advertise→bid→assign decision:

- **Bid window**: 150ms (execute), 300ms (reason). The self-bid is always present, so a lone node assigns itself after one window.
- **No qualified bid**: re-advertise up to 3 times, then emit `TaskTraced(Failed)` → the commander terminates the plan as `PlanError` (never hangs).
- **Deadline**: the planner owns a per-plan timer; on expiry it delivers `PlanResult{PlanTimeout}`. Completion and timeout are mutually exclusive (delete-then-send).

### Transport Stream Isolation

Three protocols on the libp2p transport, separating planes:

| Protocol | Plane | Purpose |
|---|---|---|
| `/lucinda/taskpostman/1.0.0` | Control | Task ads, CV bids, assignments, `TaskTraced` routing |
| `/lucinda/telemetry/1.0.0` | User progress | Status / step-result events unicast to the plan owner |
| `/lucinda/stream/1.0.0` | Data | Final-answer token deltas (never on the EventBus) |

## Key Patterns

### Module Manager Registry

Every component implements `AvailableModule` and registers with `ModuleManager`:

```go
type AvailableModule interface {
    GetModuleType() APIModule.ModuleType
    GetModuleID() APIModule.ModuleID
    CheckHealth() APIModule.ModuleHealth
    RegisterWithManager(m ModuleManager) error
    DependsOn() map[APIModule.ModuleType]string
    DependsEnable() error
}
```

Dependencies are wired in two phases after registration: `VerifyInit()` (every declared `DependsOn()` type is registered) then `EnableDeps()` (each module resolves its deps from the registry inside `DependsEnable()`, usually with `mm.Get(id)` + a type assertion — assumes exactly one instance per type). `Grant`/`Require` provides optional capability gating.

### EventBus

In-memory pub/sub: `Subscribe(topic, bufferLength)`, `Publish(topic, event)`. Non-blocking publish (drops events on full channels with a log). Topics are `EventType` string constants in `api/v1/messaging/event/event.go`. Workflow components use the `eventx` helpers: `eventx.Watch` (subscribe until ctx cancelled) and `eventx.Emit` (publish a telemetry event). `TaskPostman` / `TelemetryBridge` / `StreamRouter` bridge EventBus topics to/from the Transport.

### Provider Plugin System

Provider drivers (`vllm`, `ollama`) register themselves via `init()` in `pkg/infrastructure_layer/provider/drivers/`. `cmd/pc/plugins.go` imports them with blank identifiers. The `ProviderController` loads configs from YAML and calls `drivers.Create(config)`. Models carry an `employer` label (e.g. `TaskPlanner,TaskCommander,TaskExecutor`) used by `GetProvByFilter`; the executor picks the explicit `spec.Model`, or the first free provider tagged `TaskExecutor`.

### Config

`configs/server/config.yaml` — viper-based. Config paths: `./configs/server/`, `.`, and binary-relative `configs/server/`. Supports multiple providers (host/port/models/employer labels), libp2p transport settings, hardware monitor interval, and HTTP port.

### Cross-Node Task Execution

Every wire message (`TaskAssignMsg`, `TaskTracedMsg`, ...) carries the task's `Owner` NodeID — the plan owner node that hosts the SSE stream and the commander. `Owner` is the single routing key for results and telemetry.

- **Local**: the board assigns the winner by publishing `TaskAssigned` locally → the executor runs → the tracer emits `TaskTraced(Done)`.
- **Remote**: the board unicasts `TaskAssign` via the postman → the peer's board re-publishes `TaskAssigned` locally → the peer's executor runs → its tracer emits `TaskTraced` with `Owner` set → the postman on that node forwards it back to the owner → re-published on the owner's EventBus, where the commander advances identically to a local result.
- **Final-answer tokens**: the remote executor sends `StreamChunkMsg` on the stream protocol; the owner's StreamRouter demuxes by plan ID; TaskMonitor forwards them as SSE `stream` frames.
- **Progress**: telemetry events from remote nodes are unicast over the telemetry protocol and re-published on the owner's EventBus, so TaskMonitor shows remote and local agents identically.

## API Types (`api/v1/`)

Shared types with no internal dependencies, organized by domain:
- **`domain/`** — core domain entities
  - `task/` — `Task`, `TaskPlan`, `Transaction`, `TaskSpec`, `TaskNode`, `PlanResult`, `AgentArch`, `TaskKind`, `NodeState`, `Stage`, `TaskAd`
  - `chat/` — `ChatRequest`, `ChatResponse`, `ChatMessage`, `ContentPart`
  - `capability/` — `CapabilityCV` with `Match()` scoring
  - `hardware/` — `HardwareSnapshot`, `GPUSnapshot`, `MemorySnapshot`
  - `node/` — `NodeID`, `NodeMessage`, `Protocol`
  - `provider/` — `Provider` interface, `ProviderConfig`
  - `stream/` — `SSEFrame`, `StatusData`, `StepResultData`, `StreamData`, `DoneData`
- **`messaging/`** — event bus and wire messages
  - `event/` — `Event` struct, `EventType` constants (task lifecycle, delivery, workflow, telemetry)
  - `taskmsg/` — Wire message types: `TaskBroadcastMsg`, `TaskCVMsg`, `TaskAssignMsg`, `TaskResultMsg`, `TaskTracedMsg`, `StreamChunkMsg`, `TaskPlanResultMsg`
- **`registry/`** — module registry
  - `module/` — `ModuleType`, `ModuleID`, `ModuleHealth`, `ModuleStatus`

## Current State

Active branch: `feat/task-commander`. Pending items (tracked in `docs/log.md`):
- TaskTracer `local`/`assigned` maps grow unbounded — needs a `RemoveByPlan`/GC pass (`fix/tracer-gc`)
- Notify on all termination paths (executor permanent failure, planner generate failure) — `fix/notify-error-path`
- Roadmap: Phase A ContextManager (session state + KV-cache affinity), Phase C MetaRouter / multi-strategy planning

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build/Run/Test

```bash
# Run the server (config-driven bootstrap via viper)
go run ./cmd/pc/

# All tests (12 test packages, no test in api/ or cmd/)
go test ./internal/... ./pkg/...

# Single package
go test -v ./pkg/infrastructure_layer/eventbus/
go test -v -timeout 90s ./pkg/infrastructure_layer/transport/transporters/
go test -v ./internal/task_workflow_layer/task_board/

# End-to-end (starts server, sends /chat, polls /stream, validates non-empty output)
bash ./scripts/test_e2e.sh
```

## Architecture

Lucinda is a **compute-aware distributed agent orchestrator** for the edge. It decomposes user requests into DAGs of sub-tasks, schedules them across a decentralized mesh based on real-time hardware telemetry, and synthesizes results. Built in Go 1.26, using libp2p for peer discovery and transport.

### Four-Layer Design (strict top-down dependency)

| Layer | Purpose | Key Components |
|---|---|---|
| **4 — Server & TaskWrapper** | HTTP ingress, ChatRequest → Task, SSE stream | `internal/user_server/`, `internal/task_wrapper/` |
| **3 — Task Workflow** | Plan (LLM DAG decomposition) → Execute (parallel sub-tasks) → Reduce (LLM synthesis) | `internal/task_workflow_layer/` |
| **2 — Task Management** | FSM per sub-task, Publish-Lease protocol, EventBus↔Transport bridge, observability | `internal/task_management_layer/` |
| **1 — Infrastructure** | EventBus, libp2p Transport, HardwareMonitor, ProviderController, ModuleManager | `pkg/infrastructure_layer/` |

Layers communicate through the **EventBus** for state transitions and the **Transport** for cross-node data payloads. Raw LLM token streams use private Transport channels — not the EventBus — to avoid saturating the control plane.

### Bootstrap Order (main.go)

```
Config → EventBus → Transport → HardwareMonitor → ProviderController (LoadProviders)
→ TaskPostman → TaskTracer → TaskStateManager → TaskBoard → TaskExecutor
→ TaskPlanner → TaskReducer → HTTP Server
```

All modules register with `ModuleManager`, then services with lifecycle (`Start`/`Stop`) are started. Non-lifecycle modules (EventBus, ModuleManager, TaskStateManager, TaskTracer) are used directly.

### Plan-Execute-Reduce Pipeline

1. **TaskWrapper** publishes `TaskPreplaned` with a `Notify chan<- PlanResult` for SSE
2. **TaskPlanner** asks an LLM to decompose the prompt into a DAG of sub-tasks (each with tools, labels, dependencies). Falls back to a single-node plan on LLM failure. Appends a `-reduce` node automatically.
3. **TaskStateManager** ingests the plan, sets roots to Ready, publishes `TaskReady`
4. **TaskBoard** broadcasts Ready nodes via Transport, collects CapabilityCV bids, interviews (scores bids), and assigns the winner. Self-assigns if no qualified bids.
5. **TaskExecutor** subscribes to `TaskAssigned`, executes locally or routes to remote peer. Runs in parallel goroutines per assignment to prevent lease expiry.
6. **TaskReducer** watches for reduce-stage nodes becoming Ready, collects predecessor outputs, synthesizes via LLM. Falls back to raw concatenation on failure.
7. On plan completion, `PlanResult{Status: PlanOK}` is sent to the Notify channel → SSE client

### Node State FSM

```
Pending → Ready → Claimed → Running → Done
                ↘ Claimed → (lease expiry) → Ready (TaskRepublished)
                ↘ Running → Failed → Ready (TaskRepublished)
                                     Any → Disposed (cancelled)
```

- **Lease TTL**: 30-second claim window. If the peer doesn't call `Start` in time, the node reverts to Ready and is re-offered.
- **Heartbeat**: Every 5s, the TaskBoard scans for expired claims and rebroadcasts them.
- **Deadline watch**: On `Ingest()`, a goroutine fires at plan deadline and sends `PlanTimeout` if not done.

### Transport Stream Isolation

Control-plane (EventBus) and data-plane (raw token streams) are separate:
- **State transitions** (task created, lease expired) → global EventBus
- **LLM token streams** → private Transport channels, bypasses EventBus

## Key Patterns

### Module Manager Registry

Every component implements `AvailableModule` and registers with `ModuleManager`:

```go
type AvailableModule interface {
    GetModuleType() APIModule.ModuleType
    GetModuleID() APIModule.ModuleID
    CheckHealth() APIModule.ModuleHealth
    RegisterWithManager(m ModuleManager) error
}
```

Components resolve dependencies by calling `mm.GetByType(moduleType)[0].(ConcreteInterface)`. The type assertion panics on mismatch — the pattern assumes exactly one instance per type. Module types are defined in `api/v1/module/module.go`.

### EventBus

In-memory pub/sub: `Subscribe(topic, bufferLength)`, `Publish(topic, event)`. Non-blocking publish (drops events on full channels with a log). Topics are `EventType` string constants in `api/v1/event/event.go`. The `TaskPostman` bridges EventBus events to Transport for cross-node delivery.

### Provider Plugin System

Provider drivers (`vllm`, `ollama`) register themselves via `init()` in `pkg/infrastructure_layer/provider/drivers/registry.go`. `cmd/pc/plugins.go` imports them with blank identifiers. The `ProviderController` loads configs from YAML and calls `drivers.Create(config)`.

### Config

`configs/server/config.yaml` — viper-based. Config paths: `./configs/server/`, `.`, and binary-relative `configs/server/`. Supports multiple providers, transport settings, hardware monitor interval, and HTTP port.

### Cross-Node Task Execution

`TaskAssignMsg` carries `OriginNodeID`. The executor checks if `OriginNodeID == local transport ID`:
- **Local**: publish `TaskDone` to EventBus → board handler calls `SetOutput` + `Complete`
- **Remote**: send `TaskResultMsg` back to origin via Transport → origin's Postman bridges to EventBus

## API Types (`api/v1/`)

Shared types with no internal dependencies. Key packages:
- `task/` — `TaskPlan`, `TaskNode`, `TaskSpec`, `NodeState`, `PlanResult`, `TaskAd`
- `event/` — `Event` struct, `EventType` constants
- `taskmsg/` — Wire message types: `TaskBroadcastMsg`, `TaskRequestMsg`, `TaskAssignMsg`, `TaskResultMsg`
- `capability/` — `CapabilityCV` with `Match()` scoring
- `chat/` — `ChatRequest`, `ChatResponse`, `ChatMessage`, `ContentPart`
- `provider/` — `Provider` interface, `ProviderConfig`
- `module/` — `ModuleType`, `ModuleID`, `ModuleHealth`
- `node/` — `NodeID`, `NodeMessage`, `Protocol`
- `hardware/` — `HardwareSnapshot`, `GPUSnapshot`

## Current State

Active branch: `fix/sse-goroutine-leak`. Pending items from `docs/log.md`:
- Notify send in `Complete()` should use goroutine (currently blocks under lock at `manager.go:226`)
- Provider model index in ProviderController for O(1) lookup
- Delete `api/v1/other/` and `cmd/node/` (dead code)

Future branches planned: `fix/tracer-gc` (unbounded map growth), `fix/notify-error-path` (Notify on all termination paths), `feat/context-manager` (session management).

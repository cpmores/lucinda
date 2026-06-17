# Lucinda Implementation Pipeline

The core principle: **build from the bottom up** — each layer depends on the one below it.

---

## Phase 1: Infrastructure Layer

*Foundation — no upward dependencies.*

### 1.1 EventBus

- **Status:** Done (`pkg/infrastructure_layer/eventbus`)
- **Depends on:** nothing
- **Rationale:** The entire system is event-driven — everything else publishes or subscribes. This must come first.

### 1.2 Transport (libp2p)

- **Status:** Done (`pkg/infrastructure_layer/transport/transporters/host.go`)
- **Depends on:** EventBus (1.1)
- **Done:** Full `Transport` interface implementation: `Start`/`Stop` lifecycle, `Open`/`Close` per-protocol stream handlers, `Send` (per-peer per-protocol buffered channels with lazy stream creation), `Publish` (broadcast to all peers), `Dial` (explicit peer connection), `Peers`, self-connection for local message delivery, mDNS LAN discovery, network notifee for automatic outbound channel cleanup on peer disconnect, 16 tests passing with race detector.

### 1.3 HardwareMonitor

- **Status:** Done (`pkg/infrastructure_layer/hardware_monitor/monitor.go`)
- **Depends on:** EventBus (1.1)
- **Done:** CPU usage (gopsutil), memory (total/free/used), ticker-based polling, delta detection with configurable thresholds, thread-safe Snapshot(), EventBus integration (publishes `HardwareChanged` events on significant delta), implements `AvailableModule` interface for ModuleManager registration, 9 tests passing with race detector.
- **Note:** GPU snapshots are the responsibility of ProviderController (1.5), not the HardwareMonitor. They will be merged into the Capability CV at a higher level.

### 1.4 ModuleManager

- **Status:** Done (`pkg/infrastructure_layer/module_manager/manager.go`)
- **Depends on:** nothing (api types only)
- **Done:** `ModuleManager` interface: `Register`/`Unregister`, `Get`/`GetByType`/`List`/`Exists`, `Grant`/`Require` (access-control enforcement for the dependency DAG), `Health`/`HealthAll`. `AvailableModule` interface with `RegisterWithManager`. `Module` interface + `ModuleHealth` + status constants in `api/v1/module/`. All methods RWMutex-guarded. 15 tests passing with race detector.

### 1.5 ProviderController + Drivers

- **Status:** Done (`pkg/infrastructure_layer/provider/`)
- **Depends on:** EventBus (1.1), HardwareMonitor (1.3)
- **Done:** `Provider` interface in `api/v1/provider/` (GetID, GetType, GetModels, GetInfo, GPU, Health, Generate, Stream, Warm). `ProviderController` with LoadProviders (viper UnmarshalKey), Register (drivers.Create factory), Get/List, Health/HealthAll, GPU (picks first local provider). `ChatRequest`/`ChatResponse`/`ChatMessage`/`ContentPart`/`StreamChunk` types in `api/v1/chat/`. `ProviderConfig` (with TotalVRAM, Timeout) / `ProviderHealth` / `ProviderInfo` types in `api/v1/provider/`.
- **Ollama driver:** HTTP client → `POST /api/chat` (Generate), NDJSON stream (Stream), `GET /` (Health), `GET /api/ps` → GPUSnapshot with TotalVRAM from config. ContentPart → Ollama text+images wire format.
- **vLLM driver:** HTTP client → `POST /v1/chat/completions` (Generate), SSE stream (Stream), `GET /health` (Health), `GET /metrics` Prometheus parser → GPUSnapshot. Handles content as string or array from API. 15 tests passing.
- **Factory registry:** `drivers/registry.go` — Register/Create pattern. Each driver self-registers via `init()`. `cmd/pc/plugins.go` blank-imports all drivers.
- **ModuleManager:** Implements `AvailableModule` (GetModuleType/ID, CheckHealth, RegisterWithManager). `PROVIDERCONTROLLER` constant added to module types.
- **cmd/pc/main.go:** Working demo — registers vLLM Qwen provider, runs Generate + Stream + Health check.

### 1.6 Toolbox & ContextManager

- **Status:** Not started
- **Depends on:** EventBus (1.1)
- **Work:** Tool management for agents (register, discover, invoke tools). Session context persistence for conversation continuity across devices. Both expose extension APIs for cloud service providers.

---

## Phase 2: Task Management Layer

*Depends on Phase 1.*

### 2.1 TaskStateManager

- **Status:** Not started
- **Depends on:** EventBus (1.1), Transport (1.2)
- **Work:** The finite state machine that tracks every sub-task's lifecycle (Pending → Running → Completed/Failed). Maintains the DAG structure and reacts to event notifications to advance the cursor. Fully decoupled from physical execution lifecycles via Go channels.

### 2.2 TaskBoard + Publish-Lease Protocol

- **Status:** Not started (message types defined in `api/v1/other/pumping.go`)
- **Depends on:** Transport (1.2), EventBus (1.1), TaskStateManager (2.1)
- **Work:** The architectural centerpiece. Implement the five-message protocol already defined:
  - `TaskBroadcastMsg` — inject sub-task DAG nodes onto the board in Pending state
  - `TaskRequestMsg` — peers submit Capability CVs to claim tasks
  - `TaskAssignMsg` — board issues a TTL-bound lease to the winning peer
  - `TaskAcceptMsg` — peer confirms the lease and begins execution
  - `TaskResultMsg` — peer returns the completed result
  - Heartbeat mechanism: workers emit low-overhead heartbeats; if a node goes silent, the lease expires and the sub-task reverts to Pending.

### 2.3 TaskScheduler + Capability CV + ComponentRegistry

- **Status:** Not started
- **Depends on:** TaskBoard (2.2), HardwareMonitor (1.3), ProviderController (1.4)
- **Work:** The "Task Interview" system. Each node continuously builds a Capability CV from live hardware metrics and plugged modules. When sub-tasks appear on the TaskBoard, the scheduler runs a multi-variant matching function that balances task resource demands against candidate CVs. The ComponentRegistry tracks which plugins (tools, wrappers, planners, executors, reducers) are available on each node.

### 2.4 TaskPostman

- **Status:** Not started
- **Depends on:** Transport (1.2)
- **Work:** Routes data packets between endpoints with task-aware addressing. Lightweight — mostly a facade over the Transport layer. Handles direct node-to-node streaming for high-frequency token data (bypassing the global EventBus).

---

## Phase 3: Task Workflow Layer

*Depends on Phase 2.*

### 3.1 TaskPlanner

- **Status:** Not started
- **Depends on:** TaskBoard (2.2), Toolbox & ContextManager (1.5)
- **Work:** Decomposes a macro-task into a Directed Acyclic Graph (DAG) of sub-tasks with dependency edges. Labels each sub-task with required capabilities, tools, and resource estimates. Publishes the DAG onto the TaskBoard.

### 3.2 TaskExecutor

- **Status:** Not started
- **Depends on:** ProviderController (1.4), TaskStateManager (2.1)
- **Work:** Fires parallel agent executions or external tool invocations for each sub-task node. Routes to the appropriate provider (local Ollama or cloud API) based on the scheduler's assignment. Reports completion events to the EventBus.

### 3.3 TaskReducer

- **Status:** Not started
- **Depends on:** TaskStateManager (2.1), EventBus (1.1)
- **Work:** Waits for all DAG leaf nodes to complete, then cleans and synthesizes intermediate tokens into a unified response payload. Streams the final aggregated artifact back to the user.

---

## Phase 4: Server & TaskWrapper Layer

*Depends on Phase 3.*

### 4.1 TaskWrapper

- **Status:** Not started
- **Depends on:** TaskBoard (2.2)
- **Work:** Encapsulates a user ChatRequest into a structured TaskImage with lifecycle tracking. The bridge between the HTTP server and the TaskBoard. Returns a tracking ID immediately for async operation.

### 4.2 HTTP Server

- **Status:** Not started (legacy version in `internel/others/server/`)
- **Depends on:** TaskWrapper (4.1), EventBus (1.1)
- **Work:** User-facing ingress. Port the factory-based server registry from the legacy code. Accepts concurrent ChatRequests, submits them via TaskWrapper, and returns tracking IDs. Later: gRPC, WebSocket, and admin endpoints.

---

## Phase 5: Cross-Cutting & Polish

*Depends on all previous phases.*

### 5.1 main.go Rewrite

- **Depends on:** everything above
- **Work:** Wire the new `pkg/` architecture together. Bootstrap order:

  ```
  EventBus → Transport → HardwareMonitor → ProviderController
      → TaskStateManager → TaskBoard → TaskScheduler
      → TaskPlanner → TaskExecutor → TaskReducer
      → TaskWrapper → HTTP Server
  ```

  Remove all legacy `internel/` import paths.

### 5.2 Stream Isolation

- **Depends on:** ProviderController (1.4), EventBus (1.1), Transport (1.2)
- **Work:** Ensure high-frequency LLM token streams go through private provider channels, not the global EventBus. Only macro state transitions (task started, task completed, lease expired) are broadcast globally, safeguarding network stability.

### 5.3 Service Module Plugin System

- **Depends on:** ComponentRegistry (2.3)
- **Work:** Full dynamic plugin registry so nodes can advertise capabilities and adapt roles at runtime. Extension API for cloud service providers to inject custom wrappers, planners, executors, and reducers.

### 5.4 Tests & Benchmarks

- **Depends on:** everything above
- **Work:** Quantitative evaluation suite as outlined in the poster:
  - Task scheduling latencies
  - Control-overhead of the Capability CV interview process
  - Network throughput stability during asynchronous stream ingestion
  - Self-healing recovery time during simulated node dropouts

---

## Dependency Graph

```
Phase 1 (Infrastructure)
  ModuleManager (registry — all components register here)
       │
  EventBus ──► Transport ──► HardwareMonitor ──► ProviderController
                │                                        │
Phase 2 (Task Management)                                │
  TaskStateManager ◄── TaskBoard ◄── TaskScheduler ◄─────┘
        │                                    │
Phase 3 (Workflow)                           │
  TaskPlanner ──► TaskExecutor ──► TaskReducer            │
                                                       
Phase 4 (Server)                                        
  TaskWrapper ──► HTTP Server                            

Phase 5 (Cross-Cutting)
  main.go ──► Stream Isolation ──► Plugin System ──► Tests
```

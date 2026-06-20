# Lucinda Implementation Pipeline

The core principle: **build from the bottom up** — each layer depends on the one below it.

---

## Phase 1: Infrastructure Layer

*Foundation — no upward dependencies.*

### 1.1 EventBus

- **Status:** Done (`pkg/infrastructure_layer/eventbus/`)
- **Depends on:** nothing
- **Done:** `EventBus` interface: `Subscribe`, `Unsubscribe`, `Publish`. In-memory implementation with RWMutex-guarded subscriber map. Non-blocking publish (drops on full channel with log). Used by every other component for macro state transitions.

### 1.2 Transport (libp2p)

- **Status:** Done (`pkg/infrastructure_layer/transport/transporters/host.go`)
- **Depends on:** nothing (api types only)
- **Done:** `Transport` interface: `ID`, `Start`/`Stop` lifecycle, `Open`/`Close` per-protocol, `Send` (per-peer buffered channels with lazy stream creation), `Publish` (broadcast to all peers), `Incoming` (receive channel per protocol). libp2p implementation with mDNS LAN discovery, self-connection for local message delivery, network notifee for peer disconnect cleanup. 16 tests passing.

### 1.3 HardwareMonitor

- **Status:** Done (`pkg/infrastructure_layer/hardware_monitor/monitor.go`)
- **Depends on:** EventBus (1.1)
- **Done:** CPU usage (gopsutil), memory (total/free/used). Ticker-based polling at configurable interval. Delta detection with configurable thresholds. Thread-safe `Snapshot()`. EventBus integration: publishes `HardwareChanged` on significant delta. Implements `AvailableModule`. 9 tests passing.
- **Note:** GPU snapshots are provided by ProviderController (1.5) and merged into CapabilityCV by the TaskBoard.

### 1.4 ModuleManager

- **Status:** Done (`pkg/infrastructure_layer/module_manager/manager.go`)
- **Depends on:** nothing (api types only)
- **Done:** `ModuleManager` interface: `Register`/`Unregister`, `Get`/`GetByType`/`List`/`Exists`, `Grant`/`Require` (capability-based access control), `Health`/`HealthAll`. `AvailableModule` interface: `GetModuleType`, `GetModuleID`, `CheckHealth`, `RegisterWithManager`. All methods RWMutex-guarded. Module type constants (`TRANSPORT`, `TASKPOSTMAN`, `TASKEXECUTOR`, etc.) in `api/v1/module/`. 15 tests passing.

### 1.5 ProviderController + Drivers

- **Status:** Done (`pkg/infrastructure_layer/provider/`)
- **Depends on:** ModuleManager (1.4)
- **Done:** `Provider` interface: `GetID`, `GetType`, `GetModels`, `GetInfo`, `GPU`, `Health`, `Generate`, `Stream`, `Warm`. `ProviderController`: `LoadProviders` (viper UnmarshalKey from `provider_controller.providers`), `Register` (drivers.Create factory), `Get`/`List`/`GetPlanProv`, `GPU` (aggregates from local providers). `ChatRequest`/`ChatResponse`/`ChatMessage`/`ContentPart`/`StreamChunk` types in `api/v1/chat/`.
- **Ollama driver:** HTTP → `POST /api/chat` (Generate), NDJSON stream (Stream), `GET /` (Health), `GET /api/ps` (GPU).
- **vLLM driver:** HTTP → `POST /v1/chat/completions` (Generate, OpenAI-compatible), SSE stream (Stream), `GET /health` (Health), `GET /metrics` Prometheus parser (GPU). Handles content as string or content array from API. 15 tests passing.
- **Factory registry:** `drivers/registry.go` — Register/Create pattern. Each driver self-registers via `init()`. `cmd/pc/plugins.go` blank-imports all drivers.
- **ModuleManager:** Implements `AvailableModule`. `PROVIDERCONTROLLER` constant in module types.

### 1.6 Toolbox & ContextManager

- **Status:** Not started
- **Depends on:** EventBus (1.1)
- **Work:** Tool management (register, discover, invoke). Session context persistence across devices. Extension API for cloud providers.

---

## Phase 2: Task Management Layer

*Depends on Phase 1.*

### 2.1 TaskStateManager

- **Status:** Done (`internel/task_management_layer/task_state_manager/manager.go`)
- **Depends on:** EventBus (1.1)
- **Done:** Full DAG node FSM: `Ingest` (stores plan, publishes root nodes as Ready), `Claim` (Ready→Claimed, 30s lease with watchdog goroutine), `Start` (Claimed→Running, idempotent), `Complete` (Running→Done, cascades to successors via PredecessorNums, fires allDone notification with output), `Failed` (→Failed, publishes TaskRepublished), `Abandon` (Claimed→Ready), `Dispose` (marks all non-done as Disposed), `Expired` (returns timed-out claimed nodes). Queries: `Plan` (value copy), `Status` (per-node state map), `IsComplete`. 15 tests passing.

### 2.2 TaskBoard + Publish-Lease Protocol

- **Status:** Done (`internel/task_workflow_layer/task_board/board.go`)
- **Depends on:** Transport (1.2), EventBus (1.1), TaskStateManager (2.1), HardwareMonitor (1.3), ProviderController (1.5), TaskTracer (2.4)
- **Done:** Full publish-lease protocol. `Putup`: looks up task in TaskTracer, builds TaskAd, broadcasts via Transport Publish + self-delivers via Drawup. `Drawup`: stores remote ad, submits bid via `submitBid`→`buildCV` (queries HardwareMonitor.Snapshot + ProviderController.GPU). `Handout`: collects CVs, triggers Interview on first bid. `Interview`: scores via CapabilityCV.Match, picks winner → Claim + publish TaskAssignMsg. Self-assignment when no qualified bids (Claim only, executor calls Start). `Ripup`: removes ads/bids. Heartbeat goroutine (every 5s): calls `StateManager.Expired()` and rebroadcasts expired nodes. Watches: `TaskReady`→Putup, `TaskRepublished`→Putup, `TaskAdReceived`→Drawup, `TaskCVReceived`→Handout, `TaskDone`→SetOutput+Complete. ModuleManager integration: Postman, Transport, Tracer, ProviderController, HardwareMonitor, StateManager. 4 tests.

### 2.3 TaskPostman

- **Status:** Done (`internel/task_management_layer/task_postman/postman.go`)
- **Depends on:** EventBus (1.1), Transport (1.2)
- **Done:** `Watch`: subscribes to EventBus topic, runs handler in goroutine with context cancellation. `Deliver`: Transport.Incoming → EventBus.Publish (bridges network messages to local events). `Stop`: cancels all watchers, waits for drain. Implements `AvailableModule`. 3 tests.

### 2.4 TaskTracer

- **Status:** Done (`internel/task_management_layer/task_tracer/tracer.go`)
- **Depends on:** nothing (api types only)
- **Done:** Two-category in-memory task store: `Import` (local planned tasks), `Assigned` (tasks claimed from peers). `SetOutput` (updates task with execution result), `Remove` (deletes from local or assigned). Queries: `GetLocal`, `GetAssigned`, `ListLocal`, `ListAssigned`. RWMutex-guarded. 10 tests.

### 2.5 CapabilityCV

- **Status:** Done (`api/v1/capability/cv.go`)
- **Depends on:** hardware types
- **Done:** `CapabilityCV` struct (PeerID, HardwareSnapshot, Models, Tools, Labels). `Match(spec)` method: VRAM check, model check, tool check, label overlap. Scoring: free memory GB + priority discount. Used by TaskBoard.Interview().

---

## Phase 3: Task Workflow Layer

*Depends on Phase 2.*

### 3.1 TaskPlanner

- **Status:** Done (`internel/task_workflow_layer/task_planner/planner.go`)
- **Depends on:** TaskStateManager (2.1), TaskPostman (2.3), ProviderController (1.5), TaskTracer (2.4)
- **Done:** Watches `TaskPreplaned` events. `plan()`: sends decomposition prompt to LLM via `pc.GetPlanProv().Generate()`, parses JSON response into DAG nodes, appends a reduce node to synthesize outputs, preserves the `Notify` channel from the wrapper for result delivery. `parse()`: extracts JSON from LLM response, unmarshals into nodes, builds successor/predecessor maps, appends reduce node with `Stage: StageReduce`. Falls back to a single-node plan if JSON parsing fails. Imports all nodes into TaskTracer before calling `sm.Ingest()` (which publishes roots as Ready). 4 tests.

### 3.2 TaskExecutor

- **Status:** Done (`internel/task_workflow_layer/task_executor/executor.go`)
- **Depends on:** TaskStateManager (2.1), TaskPostman (2.3), ProviderController (1.5), TaskTracer (2.4), Transport (1.2)
- **Done:** Watches `TaskAssigned` events. `execute()`: records task in tracer via `Assigned`, matches requested model to a provider (or uses any available), calls `sm.Start()` to transition Claimed→Running, calls `prov.Generate()` with the prompt and budget tokens, publishes `TaskDone` with the result output. Does NOT remove from tracer (the board handler does cleanup after Complete). Note: for distributed execution, results currently flow through local EventBus; cross-node result routing is pending. 2 tests.

### 3.3 TaskReducer

- **Status:** Done (`internel/task_workflow_layer/task_reducer/reducer.go`)
- **Depends on:** TaskStateManager (2.1), TaskPostman (2.3), ProviderController (1.5), TaskTracer (2.4)
- **Done:** Watches `TaskReady` for nodes with `Stage: StageReduce`. `reduce()`: looks up the plan via `sm.Plan()`, collects predecessor outputs from `tt.GetLocal()`/`tt.GetAssigned()`, generates a combined response via LLM, stores output in tracer via `tt.SetOutput()`, then runs the node's lifecycle: Claim → Start → Complete. The output string is passed directly to `sm.Complete(id, output)` which relays it to the plan's `Notify` channel when allDone fires. 1 test.

---

## Phase 4: Server & TaskWrapper Layer

*Depends on Phase 3.*

### 4.1 TaskWrapper

- **Status:** Done (`internel/task_wrapper/wrapper.go`)
- **Depends on:** EventBus (1.1)
- **Done:** `Wrap(prompt, owner, notify)`: creates a `Task` with unique plan ID (`plan-{nanotime}`), attaches the notify channel for SSE streaming, publishes `TaskPreplaned` event. Returns the tracking ID for the caller to poll the stream endpoint. Pure bridge — no business logic.

### 4.2 HTTP Server (UserServer)

- **Status:** Done (`internel/user_server/server.go`)
- **Depends on:** TaskWrapper (4.1)
- **Done:** Three endpoints. `POST /chat`: reads JSON body (`prompt`, optional `owner`), creates a stream channel, calls `wrapper.Wrap()`, returns tracking ID as JSON. `GET /stream?plan=<id>`: SSE endpoint, blocks on the plan's notify channel, streams `{"type":"result","text":"..."}` then `{"type":"done"}`. `GET /healthz`: returns `ok`. `Start(addr)`: creates `http.Server` with graceful `Shutdown(ctx)` support. Uses `sync.Mutex` for stream map.

---

## Phase 5: Cross-Cutting & Polish

*Depends on all previous phases.*

### 5.1 main.go (cmd/pc)

- **Status:** Done (`cmd/pc/main.go`)
- **Done:** Config-driven bootstrap via viper. Bootstrap order:
  ```
  loadConfig (viper + YAML)
    → EventBus → Transport (libp2p with config addrs)
    → HardwareMonitor (configurable interval)
    → ProviderController (LoadProviders from config)
    → TaskPostman → TaskTracer → TaskStateManager
    → register all infrastructure modules with ModuleManager
    → TaskBoard → TaskExecutor → TaskPlanner → TaskReducer
    → register workflow modules
    → start all services in order
    → HTTP server on configured port
    → graceful shutdown on SIGINT/SIGTERM
      (Server.Shutdown → te.Stop → reducer.Stop → planner.Stop
       → tb.Stop → hm.Stop → tp.Stop → pm.Stop)
  ```
  All values (transport addrs, provider config, http port, monitor interval) come from `configs/server/config.yaml`. `cmd/pc/plugins.go` blank-imports ollama and vllm drivers.

### 5.2 Stream Isolation

- **Status:** Partial — EventBus/Transport separation exists; LLM token streams currently go through local channels, not bypassed cross-node.
- **Depends on:** ProviderController (1.5), EventBus (1.1), Transport (1.2)
- **Work:** Ensure cross-node LLM token streams go through private Transport channels, not the global EventBus. Only macro state transitions are broadcast globally.

### 5.3 Service Module Plugin System

- **Status:** Not started
- **Depends on:** ModuleManager (1.4)
- **Work:** Dynamic plugin registry for nodes to advertise capabilities and adapt roles at runtime. Extension API for cloud providers.

### 5.4 Tests & Benchmarks

- **Status:** Partial — 11 test packages with good coverage. No benchmarks yet.
- **Work:** Quantitative evaluation: scheduling latencies, CapabilityCV interview overhead, network throughput under stream load, self-healing recovery time under node dropout.

---

## Dependency Graph

```
Phase 1 (Infrastructure) ✅
  ModuleManager (registry — all components register here)
       │
  EventBus ──► Transport ──► HardwareMonitor ──► ProviderController
                │
Phase 2 (Task Management) ✅
  TaskStateManager ──► TaskBoard ──► TaskPostman ──► TaskTracer
        │                    │                         │
Phase 3 (Workflow) ✅        │                         │
  TaskPlanner ───────────────┤                         │
        │                    │                         │
  TaskExecutor ◄─────────────┘                         │
        │                                              │
  TaskReducer ◄────────────────────────────────────────┘

Phase 4 (Server) ✅
  TaskWrapper ──► HTTP Server (user_server)

Phase 5 (Cross-Cutting) 🟡
  main.go ✅ ──► Stream Isolation 🟡 ──► Plugin System 🔴 ──► Benchmarks 🔴
```

### Data flow (end-to-end)

```
POST /chat → TaskWrapper.Wrap → EventBus: TaskPreplaned
  → TaskPlanner: decompose via LLM → DAG → sm.Ingest → EventBus: TaskReady (roots)
  → TaskBoard.Putup: broadcast Ad via Transport, self-Drawup
  → CapabilityCV.Match → Interview → Claim winner → EventBus: TaskAssigned
  → TaskExecutor: sm.Start → prov.Generate → EventBus: TaskDone
  → TaskBoard (TaskDone handler): tt.SetOutput + sm.Complete
    → cascade: PredecessorNums-- → successors become Ready
  → TaskReducer (reduce node Ready): collect from tracer → LLM combine
    → sm.Complete(reduceID, output) → allDone → Notify channel
  → HTTP /stream: receives output → SSE response
```

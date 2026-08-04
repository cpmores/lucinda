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
- **Done:** `Transport` interface: `ID`, `Start`/`Stop` lifecycle, `Open`/`Close` per-protocol, `Send` (per-peer buffered channels with lazy stream creation), `Publish` (broadcast to all peers, skips self), `Incoming` (receive channel per protocol). libp2p implementation with mDNS LAN discovery, self-connection for local message delivery, self-send worker, network notifee for peer disconnect cleanup. 16 tests passing.

### 1.3 HardwareMonitor

- **Status:** Done (`pkg/infrastructure_layer/hardware_monitor/monitor.go`)
- **Depends on:** EventBus (1.1)
- **Done:** CPU usage (gopsutil), memory (total/free/used). Ticker-based polling at configurable interval. Delta detection with configurable thresholds. Thread-safe `Snapshot()`. EventBus integration: publishes `HardwareChanged` on significant delta. Implements `AvailableModule`. 9 tests passing.
- **Note:** GPU snapshots are provided by ProviderController (1.5) and merged into CapabilityCV by the TaskBoard.

### 1.4 ModuleManager

- **Status:** Done (`pkg/infrastructure_layer/module_manager/manager.go`)
- **Depends on:** nothing (api types only)
- **Done:** `ModuleManager` interface: `Register`/`Unregister`, `Get`/`GetByType`/`List`/`Exists`, `Grant`/`Require` (capability-based access control), `Health`/`HealthAll`. `AvailableModule` interface: `GetModuleType`, `GetModuleID`, `CheckHealth`, `RegisterWithManager`. RWMutex-guarded. Module type constants (`Transport`, `TaskPostman`, `TaskExecutor`, etc.) in `api/v1/registry/module/`. 15 tests passing.

### 1.5 ProviderController + Drivers

- **Status:** Done (`pkg/infrastructure_layer/provider/`)
- **Depends on:** ModuleManager (1.4)
- **Done:** `Provider` interface: `GetID`, `GetType`, `GetModels`, `GetInfo`, `MaxContextTokens`, `GPU`, `Health`, `Generate`, `Stream`, `Warm`. `ProviderController`: `LoadProviders` (viper UnmarshalKey from `provider_controller.providers`), `Register` (drivers.Create factory), `Get`/`List`/`GetPlanProv`/`MaxContext`, `GPU` (aggregates from local providers).
- **vLLM driver:** HTTP → `POST /v1/chat/completions` (Generate, OpenAI-compatible), SSE stream (Stream), `GET /health` (Health), `GET /metrics` Prometheus parser (GPU). Handles content as string or array from API. Default context: 2048 tokens, configurable via `max_context_tokens`. 15 tests passing.
- **Ollama driver:** HTTP → `POST /api/chat` (Generate), NDJSON stream (Stream), `GET /` (Health), `GET /api/ps` (GPU). Default context: 2048.
- **Factory registry:** `drivers/registry.go` — Register/Create pattern. Each driver self-registers via `init()`. `cmd/pc/plugins.go` blank-imports all drivers.
- **ModuleManager:** Implements `AvailableModule`.

### 1.6 Toolbox & ContextManager

- **Status:** Not started
- **Depends on:** EventBus (1.1)
- **Work:** Tool management (register, discover, invoke). Session context persistence across devices. Extension API for cloud providers. Once available, populate `Tools` field in CapabilityCV for proper tool-based Match scoring.

---

## Phase 2: Task Management Layer

*Depends on Phase 1.*

### 2.1 TaskStateManager

- **Status:** Done (`internal/task_management_layer/task_state_manager/manager.go`)
- **Depends on:** EventBus (1.1)
- **Done:** Full DAG node FSM: `Ingest` (stores plan, publishes root nodes as Ready), `Claim` (Ready→Claimed, 30s lease with watchdog goroutine), `Start` (Claimed→Running, idempotent), `Complete` (Running→Done, takes output string, cascades to successors via PredecessorNums, fires allDone notification with output directly to Notify channel), `Failed` (→Failed, publishes TaskRepublished), `Abandon` (Claimed→Ready), `Dispose` (marks all non-done as Disposed), `Expired` (returns timed-out claimed nodes). Queries: `Plan` (value copy), `Status` (per-node state map), `IsComplete`. Output flows from caller → Complete → Notify without being stored on the plan node. 15 tests passing.

### 2.2 TaskBoard + Publish-Lease Protocol

- **Status:** Done (`internal/task_workflow_layer/task_board/board.go`)
- **Depends on:** Transport (1.2), EventBus (1.1), TaskStateManager (2.1), HardwareMonitor (1.3), ProviderController (1.5), TaskTracer (2.4)
- **Done:** Full publish-lease protocol with cross-node routing.
  - `Putup`: looks up task in TaskTracer, builds TaskAd, broadcasts via Transport Publish (skips self) + self-delivers via Drawup.
  - `Drawup`: stores remote ad, calls `submitBid`→`buildCV`.
  - `buildCV`: queries HardwareMonitor.Snapshot + ProviderController.GPU (2s timeout, non-blocking via goroutine+channel to prevent event-loop stall). Tool check in CapabilityCV.Match is gated on `len(cv.Tools) > 0` so decomposition tool requirements don't block bids before the toolbox is implemented.
  - `Handout`: collects CVs, triggers Interview on first bid.
  - `Interview`: scores via CapabilityCV.Match, picks winner → Claim + publish/send TaskAssignMsg. Self-assignment when no qualified bids (Claim only, executor calls Start). Local winner → EventBus, remote winner → Transport.Send with OriginNodeID set.
  - `buildAssignMsg`: helper that sets OriginNodeID for cross-node result routing.
  - `Ripup`: removes ads/bids.
  - Heartbeat goroutine (every 5s): calls `StateManager.Expired()` and rebroadcasts expired nodes.
  - Watches: `TaskReady`→Putup, `TaskRepublished`→Putup, `TaskAdReceived`→Drawup, `TaskCVReceived`→Handout, `TaskDone`→SetOutput+Complete.
  - ModuleManager integration: Postman, Transport, Tracer, ProviderController, HardwareMonitor, StateManager. 4 tests.

### 2.3 TaskPostman

- **Status:** Done (`internal/task_management_layer/task_postman/postman.go`)
- **Depends on:** EventBus (1.1), Transport (1.2)
- **Done:** `Watch`: subscribes to EventBus topic, runs handler in goroutine with context cancellation. `Deliver`: Transport.Incoming → EventBus.Publish by msg.Topic (bridges network messages to local events, enabling cross-node result routing). `Stop`: cancels all watchers, waits for drain via WaitGroup. Implements `AvailableModule`. 3 tests.

### 2.4 TaskTracer

- **Status:** Done (`internal/task_management_layer/task_tracer/tracer.go`)
- **Depends on:** nothing (api types only)
- **Done:** Two-category in-memory task store: `Import` (local planned tasks), `Assigned` (tasks claimed from peers). `SetOutput` (updates task with execution result), `Remove` (deletes from local or assigned). Queries: `GetLocal`, `GetAssigned`, `ListLocal`, `ListAssigned`. RWMutex-guarded. 10 tests.

### 2.5 CapabilityCV

- **Status:** Done (`api/v1/domain/capability/cv.go`)
- **Depends on:** hardware types
- **Done:** `CapabilityCV` struct (TaskID, PeerID, HardwareSnapshot, Models, Tools, Labels). `Match(spec)` method: VRAM check, model check, tool check (gated on `len(cv.Tools) > 0` — toolbox not implemented yet), label check (gated on `len(cv.Labels) > 0`). Scoring: free memory GB + priority discount. Used by TaskBoard.Interview().

---

## Phase 3: Task Workflow Layer

*Depends on Phase 2.*

### 3.1 TaskPlanner

- **Status:** Done (`internal/task_workflow_layer/task_planner/planner.go`)
- **Depends on:** TaskStateManager (2.1), TaskPostman (2.3), ProviderController (1.5), TaskTracer (2.4)
- **Done:** Watches `TaskPreplaned` events.
  - `plan()`: sends decomposition prompt to LLM via `pc.GetPlanProv().Generate()`, parses JSON response into DAG nodes, appends a reduce node with `Stage: StageReduce`, preserves the `Notify` channel from the wrapper.
  - `generate()`: uses `prov.MaxContextTokens() / 4` for output budget (min 256) instead of hardcoded 1000.
  - `parse()`: extracts JSON from LLM response via `extractJSON` (brace counting), unmarshals into nodes, builds successor/predecessor maps, appends reduce node.
  - Falls back to a single-node plan if LLM generation fails **or** JSON parsing fails, so the pipeline never stalls.
  - Imports all nodes into TaskTracer before calling `sm.Ingest()` (which publishes roots as Ready).
  - 4 tests.

### 3.2 TaskExecutor

- **Status:** Done (`internal/task_workflow_layer/task_executor/executor.go`)
- **Depends on:** TaskStateManager (2.1), TaskPostman (2.3), ProviderController (1.5), TaskTracer (2.4), Transport (1.2)
- **Done:** Watches `TaskAssigned` events. `execute()` dispatched via `go` (parallel goroutines) so multiple sub-tasks execute concurrently — prevents lease expiry on queued nodes.
  - **Local task** (OriginNodeID empty or == self): calls `sm.Start()` (Claimed→Running, releases lease), `prov.Generate()`, publishes `TaskDone` to local EventBus.
  - **Remote task** (OriginNodeID != self): skips `sm.Start()` (plan is on origin), sends `TaskResultMsg` back to origin via `Transport.Send()` with `TaskBoardProtocol`. Origin's Postman.Deliver bridges to EventBus → board handler → Complete.
  - Model matching: finds provider with requested model, falls back to first available.
  - `textFromResponse()`: uses `strings.Builder`.
  - 2 tests (compile + integration skip).

### 3.3 TaskReducer

- **Status:** Done (`internal/task_workflow_layer/task_reducer/reducer.go`)
- **Depends on:** TaskStateManager (2.1), TaskPostman (2.3), ProviderController (1.5), TaskTracer (2.4)
- **Done:** Watches `TaskReady` for nodes with `Stage: StageReduce`.
  - `reduce()`: looks up plan via `sm.Plan()`, collects predecessor outputs from `tt.GetLocal()`/`tt.GetAssigned()`, generates synthesized response via LLM.
  - **Context truncation**: truncation limit derived from `prov.MaxContextTokens() * 3` chars (not hardcoded). Proportionally truncates each output to fit.
  - **Reduce prompt**: framed as "writing a final answer from research notes" with explicit instructions not to mention sub-tasks, process, or meta-commentary.
  - **Fallback**: if LLM combine fails, falls back to raw concatenation so the plan always completes.
  - Lifecycle: Claim → Start → Complete(output). The output string is passed directly to `sm.Complete(id, output)` which relays it to the plan's `Notify` channel when allDone fires.
  - 1 test.

---

## Phase 4: Server & TaskWrapper Layer

*Depends on Phase 3.*

### 4.1 TaskWrapper

- **Status:** Done (`internal/task_wrapper/wrapper.go`)
- **Depends on:** EventBus (1.1)
- **Done:** `Wrap(prompt, owner, notify)`: creates a `Task` with unique plan ID (`plan-{nanotime}`), attaches the notify channel for SSE streaming, publishes `TaskPreplaned` event. Returns the tracking ID for the caller to poll the stream endpoint.

### 4.2 HTTP Server

- **Status:** Done (`internal/user_server/server.go`)
- **Depends on:** TaskWrapper (4.1)
- **Done:** Three endpoints, graceful shutdown via `http.Server.Shutdown()`.
  - `POST /chat`: reads JSON body (`prompt`, optional `owner`), creates stream channel, calls `wrapper.Wrap()`, returns tracking ID.
  - `GET /stream?plan=<id>`: SSE endpoint, blocks on the plan's notify channel, streams `{"type":"result","text":"..."}` then `{"type":"done"}`.
  - `GET /healthz`: returns `ok`.
  - `Start(addr)`: creates `http.Server`, returns `http.ErrServerClosed` as nil for clean shutdown.

---

## Phase 5: Cross-Cutting & Polish

*Depends on all previous phases.*

### 5.1 main.go (cmd/pc)

- **Status:** Done (`cmd/pc/main.go`)
- **Done:** Config-driven bootstrap via viper + YAML. Bootstrap order:
  ```
  loadConfig (viper + YAML, multi-path search)
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
      (Server.Shutdown(10s) → te.Stop → reducer.Stop → planner.Stop
       → tb.Stop → hm.Stop → tp.Stop → pm.Stop)
  ```
  All values come from `configs/server/config.yaml`. `cmd/pc/plugins.go` blank-imports vllm and ollama drivers.

### 5.2 Stream Isolation

- **Status:** Partial — EventBus/Transport separation exists. Cross-node result routing uses Transport for TaskResultMsg, not EventBus. LLM token streams still flow through local channels.
- **Work:** Route LLM token streams through private Transport channels cross-node, bypassing the EventBus entirely.

### 5.3 Service Module Plugin System

- **Status:** Not started
- **Depends on:** ModuleManager (1.4)
- **Work:** Dynamic plugin registry for nodes to advertise capabilities and adapt roles at runtime. Extension API for cloud providers.

### 5.4 Plan Deadline Enforcement

- **Status:** Not started
- **Depends on:** TaskStateManager (2.1)
- **Work:** The planner sets `Deadline: time.Now().Add(5 * time.Minute)` but nothing reads it. Add deadline checking to the board's heartbeat — if past deadline, call `sm.Dispose(planID)` and send error to `plan.Notify`.

### 5.5 Tracer GC

- **Status:** Not started
- **Depends on:** TaskTracer (2.4)
- **Work:** Tasks accumulate in the tracer's `local` and `assigned` maps forever. Add a `Dispose(planID)` method that removes all nodes for a plan, called from the StateManager's dispose path.

### 5.6 Tests & Benchmarks

- **Status:** Partial — 12 test packages with infrastructure coverage. Workflow + server layers need more tests.
- **Priority:** Add tests for `TaskWrapper`, `UserServer`, `TaskReducer.reduce()`, and `TaskExecutor.execute()`. Add benchmarks for CapabilityCV.Match, TaskBoard.Interview, and end-to-end pipeline latency.

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
  TaskWrapper ──► HTTP Server

Phase 5 (Cross-Cutting) 🟡
  main.go ✅ ──► Stream Isolation 🟡 ──► Deadline Enforce 🔴 ──► Tracer GC 🔴 ──► Tests 🔴
```

### End-to-end data flow

```
POST /chat → TaskWrapper.Wrap → EventBus: TaskPreplaned
  → TaskPlanner.plan: decompose via LLM → DAG + reduce → sm.Ingest → EventBus: TaskReady (roots)
  → TaskBoard.Putup: broadcast Ad via Transport, self-Drawup → submitBid → buildCV
  → CapabilityCV.Match (model, VRAM, tools, labels) → Interview → Claim winner
  → Local: EventBus:TaskAssigned | Remote: Transport.Send(TaskAssigned, originID)
  → TaskExecutor.execute (parallel goroutines):
    Local: sm.Start → prov.Generate → EventBus:TaskDone
    Remote: prov.Generate → Transport.Send(TaskDone, origin)
  → TaskBoard (TaskDone handler): tt.SetOutput + sm.Complete
    → cascade: PredecessorNums-- → successors become Ready
  → TaskReducer (reduce node Ready): collect from tracer → LLM synthesis
    → sm.Complete(reduceID, output) → allDone → Notify channel
  → HTTP /stream: receives output → SSE response
```

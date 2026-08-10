# Lucinda

A **compute-aware distributed agent orchestrator** for the edge. Lucinda decomposes a user request into **semantic transactions**, hands each to its own Commander (a ReAct or Plan-and-Execute agent), schedules the resulting tasks across a decentralized mesh based on real-time hardware telemetry and capability bidding, and streams the synthesized answer back. Providers are reached **only through executors**, so any node with a capable model can serve any step — planner and commander never touch a model directly.

---

## Table of Contents

- [What Problem It Solves](#what-problem-it-solves)
- [Architecture](#architecture)
  - [Four-Layer Blueprint](#four-layer-blueprint)
  - [Semantic Transactions + Multi-Commander](#semantic-transactions--multi-commander)
  - [Publish-Lease TaskBoard](#publish-lease-taskboard)
  - [Provider Behind the Executor](#provider-behind-the-executor)
  - [Telemetry & Stream Isolation](#telemetry--stream-isolation)
- [Project Status](#project-status)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Configuration](#configuration)
  - [Running](#running)
  - [Smoke Tests](#smoke-tests)
- [Project Structure](#project-structure)
- [Development](#development)
  - [Testing](#testing)
- [Comparison to Existing Solutions](#comparison-to-existing-solutions)
- [License](#license)

---

## What Problem It Solves

Today's AI agent landscape has a structural gap:

| Layer | What exists | What's missing |
|---|---|---|
| **Application frameworks** (LangChain, AutoGen) | Macro routing logic, prompt chaining | Zero awareness of underlying CPU, vRAM, or GPU consumption |
| **Inference engines** (Ollama, vLLM) | Single-node model serving | No distributed task coordination, no lifecycle tracking |
| **Cluster orchestrators** (Ray, KubeEdge) | Generic container/job scheduling | No understanding of LLM streaming patterns or agent pipelines |

The result: edge deployments suffer from **resource starvation**, **task blocking**, and **blind scheduling** — agents spawn work without knowing whether the hardware can handle it.

Lucinda sits between these layers. It gives agent frameworks a **hardware-aware runtime** that:

- Runs a **semantic planner** on any node (even a phone) to split a request into transactions — not fine-grained steps
- Hands each transaction to its **own Commander** (ReAct or Plan-and-Execute), running in parallel or dependency-ordered
- Routes every task through a decentralized **Publish-Lease TaskBoard** that matches capability (model, VRAM, tools) to the best-fit node
- Executes tasks on **any node that serves the required model** — the planner/commander never hold a provider
- Streams the **final answer only** to the user over a dedicated data plane; all intermediate work stays structured
- Tracks every task's lifecycle through a single `TaskTraced` signal, with telemetry unicast back to the plan owner

---

## Architecture

### Four-Layer Blueprint

```
┌──────────────────────────────────────────────────────────────┐
│  Layer 4 — Server & TaskWrapper                              │
│  HTTP ingress (/chat), SSE egress (/stream), TaskMonitor     │
├──────────────────────────────────────────────────────────────┤
│  Layer 3 — Task Workflow                                     │
│  TaskPlanner  (semantic decomposition, provider-free)        │
│  TaskCommander (one per transaction: ReAct / Plan-Execute)   │
│  TaskExecutor  (reason / execute / synthesize — the ONLY     │
│                component that touches a provider)            │
│  TaskMonitor   (aggregates telemetry + stream → SSE)         │
├──────────────────────────────────────────────────────────────┤
│  Layer 2 — Task Management                                   │
│  TaskBoard      (Publish-Lease: advertise → bid → assign)    │
│  TaskTracer     (task lifecycle registry + TaskTraced)       │
│  TaskPostman    (coordination EventBus ↔ Transport bridge)   │
│  TelemetryBridge(progress telemetry unicast to owner)        │
│  StreamRouter   (final-answer token stream data plane)       │
├──────────────────────────────────────────────────────────────┤
│  Layer 1 — Infrastructure                                    │
│  EventBus, Transport (libp2p + mDNS), HardwareMonitor,       │
│  ProviderController (vLLM, Ollama), ModuleManager, Logger    │
└──────────────────────────────────────────────────────────────┘
```

Each layer depends only on the ones below it. Layers communicate through the EventBus for control, the Transport for cross-node messages, and a private stream protocol for token data.

### Semantic Transactions + Multi-Commander

The planner decomposes a request into **semantic transactions** — user-visible deliverables ("write the doc", "generate the video", "adjust the AC") — each with a goal and dependencies:

```
Plan = Transaction DAG
  ├─ t1 {Goal: 写文档, Deps: []}
  ├─ t2 {Goal: 生成视频, Deps: []}
  └─ t3 {Goal: 调空调, Deps: []}
```

Each transaction gets **its own Commander**:

- **Independent transactions run in parallel** (one Commander goroutine per transaction)
- **Dependent transactions run in order** — a transaction starts only after its `Deps` complete, and the dependency outputs are fed into its goal context
- The plan's `Architecture` decides how each Commander behaves:
  - **`react`**: the Commander runs a reasoning loop (reason → act → observe) against the transaction goal until it decides `done`
  - **`plan_execute`**: the Commander dispatches the transaction goal once through the board

When all transactions complete, their results are merged into the final answer.

### Publish-Lease TaskBoard

The architectural centerpiece. Instead of a centralized scheduler, every task — whether a plan action or a commander's reasoning step — is advertised and claimed through a **Publish-Lease** protocol:

```
Employer                      TaskBoard (mesh)                  Worker
   │                                │                              │
   │── TaskBroadcastMsg (ad) ──────►│                              │
   │                                │◄── TaskCVMsg (CapabilityCV)──│
   │                                │    (bid: models, VRAM, tools)│
   │                                │── TaskAssignMsg ────────────►│
   │                                │    (unicast to the winner)   │
   │◄───────────────────────────────│── TaskTraced (result) ───────│
```

- **Capability bidding**: a node with no matching model does not bid; the best-qualified bid (VRAM, model, tools, labels) wins. The matching **strategy lives in the TaskBoard** (`matchCV`) so it can evolve without touching the API contract.
- **Per-kind bid windows**: `reason` tasks use a short window (quick internal LLM calls), `execute`/`synthesize` use the normal one.
- **Self-bid fast path**: a lone node assigns itself after one window.
- **Two-layer failure handling**: a failed task emits `TaskTraced{Released}` (retryable) — the board reassigns it to the next best candidate (up to 3) instead of failing the plan; only when candidates are exhausted does the board emit the terminal `TaskTraced{Failed}`, which the commander turns into `PlanError`. The executor also retries a busy provider with backoff before releasing.
- **Retry**: if no bid qualifies, the ad is re-issued up to 3 times; giving up fails the plan instead of hanging.
- **Cross-node routing**: results flow back through the `TaskTraced` signal, which the postman unicasts to the plan owner.

### Provider Behind the Executor

Only the **TaskExecutor** ever touches a provider. The planner and commander are provider-free:

- **Planner** issues semantic **decomposition** as a `reason` task through the board
- **Commander** issues its reasoning decisions as `reason` tasks, its work as `execute` tasks, and the final answer as a `synthesize` task
- **Executor** runs the three task kinds: `reason`/`execute` → one-shot `Generate`; `synthesize` → `Stream`, routing each token chunk to the plan owner via the stream router

This makes every step **cross-node by construction**: whichever node serves the model handles it. A commander's node needs no provider at all.

### Telemetry & Stream Isolation

Lucinda separates control, progress, and data planes:

| Traffic | Channel | Purpose |
|---|---|---|
| Coordination (`TaskTraced`, ads, assignments) | EventBus / task protocol | Control plane, low volume |
| User-facing progress (planning/thinking/running…) | Telemetry protocol → **unicast to plan owner** | Progress display, low volume |
| Final-answer token chunks | **Private stream protocol** (`/lucinda/stream/1.0.0`) | Data plane, high frequency |

Only the **final answer** is streamed; intermediate reasoning and work stay structured. Raw tokens never ride the EventBus.

---

## Project Status

| Component | Status | Location |
|---|---|---|
| EventBus (in-memory) | ✅ | `pkg/infrastructure_layer/eventbus/` |
| Transport (libp2p + mDNS) | ✅ | `pkg/infrastructure_layer/transport/transporters/` |
| HardwareMonitor | ✅ | `pkg/infrastructure_layer/hardware_monitor/` |
| ModuleManager + DI | ✅ `DependsOn`/`DependsEnable` | `pkg/infrastructure_layer/module_manager/` |
| ProviderController + drivers (vLLM, Ollama) | ✅ `Generate`/`Stream`, `ModelFilter`/`GetProvByFilter` | `pkg/infrastructure_layer/provider/` |
| TaskBoard (Publish-Lease, `matchCV` strategy) | ✅ | `internal/task_management_layer/task_board/` |
| TaskTracer (`TaskTraced` lifecycle) | ✅ | `internal/task_management_layer/task_tracer/` |
| TaskPostman (EventBus ↔ Transport) | ✅ | `internal/task_management_layer/postman/` |
| TelemetryBridge (progress → owner) | ✅ | `internal/task_management_layer/telemetry_bridge/` |
| StreamRouter (token stream data plane) | ✅ | `internal/task_management_layer/stream_router/` |
| TaskPlanner (semantic decomposition, provider-free) | ✅ | `internal/task_workflow_layer/task_planner/` |
| TaskCommander (multi-transaction, ReAct/Plan-Execute, provider-free) | ✅ | `internal/task_workflow_layer/task_commander/` |
| TaskExecutor (reason/execute/synthesize task kinds) | ✅ | `internal/task_workflow_layer/task_executor/` |
| TaskMonitor (SSE aggregation) | ✅ | `internal/task_workflow_layer/task_monitor/` |
| HTTP server (`/chat`, `/stream`, `/healthz`) | ✅ | `internal/user_server/` |
| Scripts (start/stop, smoke tests) | ✅ | `scripts/` |
| Toolbox / MCP / image-video drivers | 🔴 Not started | — |
| ContextManager (session state + KV-cache affinity) | 🔴 Not started | — |

---

## Getting Started

### Prerequisites

- **Go** 1.26+
- **vLLM** or **Ollama** for local inference (default: vLLM at `localhost:8000`)

### Configuration

`configs/server/config.yaml`:

```yaml
provider_controller:
  providers:
    - id: "vllm"
      driver: "vllm"
      host: "localhost"
      port: 8000
      max_context_tokens: 2048
      models:
        - id: "qwen-2.5-gptq"
          labels:
            modality: "text"
            employer: "TaskPlanner,TaskCommander,TaskExecutor"
          params_b: 7
          context_tokens: 2048
          min_vram: 17179869184   # 16 GiB

transport:
  type: "libp2p"
  libp2p:
    addrs:
      - "/ip4/0.0.0.0/tcp/0"
    outs_length: 20
    ins_length: 100

hardware_monitor:
  interval_sec: 5

http:
  port: 9090
```

Config search paths: `./configs/server/`, `.`, and binary-relative `configs/server/`.

### Running

```bash
bash scripts/start_lucinda.sh          # start (background, pid in /tmp/lucinda.pid)
bash scripts/start_lucinda.sh stop     # stop
```

### Smoke Tests

```bash
# Simple: plan_execute → status/step_result/done
bash scripts/test_smoke_simple.sh --start

# Complex: ReAct loop → status/step_result/stream/done
bash scripts/test_smoke.sh --start

# Failure flow: Released → board give-up → done(error)
bash scripts/test_release.sh
```

Manual test:

```bash
# Send a request (agent: plan_execute | react)
curl -s -X POST http://localhost:9090/chat \
  -H "Content-Type: application/json" \
  -d '{"agent":"react","messages":[{"role":"user","content":[{"type":"text","text":"写一首关于大海的诗"}]}]}'
# → {"plan_id":"plan-..."}

# Stream the result
curl -s -N "http://localhost:9090/stream?plan=<plan_id>"
# → status / step_result / stream / done SSE frames
```

---

## Project Structure

```
lucinda/
├── api/v1/                       # Shared API types (stable data contracts)
│   ├── capability/               #   CapabilityCV (data; Match strategy lives in TaskBoard)
│   ├── chat/                     #   ChatRequest, ChatResponse, StreamChunk
│   ├── hardware/                 #   HardwareSnapshot types
│   ├── node/                     #   NodeID, Protocol, NodeMessage
│   ├── provider/                 #   Provider interface, ModelInfo, ModelFilter
│   ├── registry/module/          #   ModuleType, ModuleID, ModuleHealth
│   ├── stream/                   #   SSE frame envelope types
│   ├── task/                     #   Task, TaskPlan (Transactions), TaskSpec (Kind), Transaction
│   ├── messaging/event/          #   Event struct + EventType constants
│   └── messaging/taskmsg/        #   Wire messages (broadcast, CV, assign, traced, stream)
│
├── pkg/infrastructure_layer/     # Layer 1 — Infrastructure
│   ├── eventbus/                 #   In-memory EventBus
│   ├── transport/                #   Transport interface
│   │   └── transporters/         #     libp2p implementation (host, mDNS, self-worker)
│   ├── hardware_monitor/         #   CPU/memory polling + EventBus
│   ├── module_manager/           #   Module registry + DI (DependsOn/DependsEnable)
│   ├── logger/                   #   slog wrapper (text/json/colored)
│   └── provider/                 #   ProviderController + drivers (vllm, ollama)
│
├── internal/
│   ├── task_management_layer/    # Layer 2 — Task Management
│   │   ├── task_board/           #   Publish-Lease + matchCV strategy
│   │   ├── task_tracer/          #   Task lifecycle registry + TaskTraced
│   │   ├── postman/              #   Coordination EventBus ↔ Transport bridge
│   │   ├── telemetry_bridge/     #   Progress telemetry unicast to owner
│   │   └── stream_router/        #   Final-answer token stream data plane
│   ├── task_workflow_layer/      # Layer 3 — Task Workflow
│   │   ├── task_planner/         #   Semantic decomposition (provider-free)
│   │   ├── task_commander/       #   Multi-transaction ReAct/Plan-Execute (provider-free)
│   │   ├── task_executor/        #   reason/execute/synthesize task kinds
│   │   ├── task_monitor/         #   SSE aggregation (telemetry + stream)
│   │   └── eventx/               #   Watch/Emit helpers
│   ├── task_wrapper/             #   ChatRequest → raw Task (external util)
│   ├── user_server/              #   HTTP ingress (chat, SSE stream, health)
│   ├── testutil/                 #   Mock transport / provider / controller
│   └── task_workflow_layer/crossnode/  # Two-node integration tests
│
├── cmd/pc/                       # Active entrypoint (config-driven)
│   ├── main.go                   #   Bootstrap + graceful shutdown
│   └── plugins.go                #   Provider driver imports
├── configs/server/               # YAML configuration
├── scripts/                      # start_lucinda.sh, test_smoke.sh, test_smoke_simple.sh, test_release.sh
├── docs/                         # Documentation, proposals, diagrams
└── openspec/                     # Change proposals + capability specs
```

---

## Development

### Testing

```bash
# All tests (race detector)
go test -race ./internal/... ./api/...

# Individual packages
go test -v ./pkg/infrastructure_layer/eventbus/
go test -v -timeout 90s ./pkg/infrastructure_layer/transport/transporters/
go test -v ./internal/task_management_layer/task_board/
go test -v ./internal/task_workflow_layer/task_commander/
go test -v ./internal/task_workflow_layer/task_planner/

# Smoke (needs a running server or --start)
bash scripts/test_smoke_simple.sh --start
bash scripts/test_smoke.sh --start

# Failure flow (starts its own server with a dead provider)
bash scripts/test_release.sh
```

---

## Comparison to Existing Solutions

| Dimension | LangChain / AutoGen | Ollama / vLLM | Ray / KubeEdge | **Lucinda** |
|---|---|---|---|---|
| Hardware awareness | None (cloud-centric) | Static single-node | Strong (cluster telemetry) | **Dynamic real-time telemetry** |
| Distributed scheduling | Standalone routing | No multi-node coordination | Master-worker topology | **Decentralized Publish-Lease DAG** |
| Stream-control isolation | Logic and data coupled | Raw token capture | Generic packet handling | **Private channels + EventBus split** |
| Agent-native pipeline | Macro flow graphs | Inference endpoint only | Generic container compute | **Semantic transactions + per-transaction Commander** |
| Provider coupling | Cloud-locked | Single node | N/A | **Provider-free planner/commander; executor routes by capability** |
| Fault tolerance | None | None | Restart-based | **Re-advertise + graceful plan failure** |

---

## License

TBD

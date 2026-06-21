# Lucinda

A **compute-aware distributed agent orchestrator** for the edge. Lucinda bridges the gap between high-level agent frameworks and the physical hardware they run on — decomposing user requests into DAGs of sub-tasks, scheduling them across a decentralized mesh based on real-time hardware telemetry, and synthesizing the results into a single response.

---

## Table of Contents

- [What Problem It Solves](#what-problem-it-solves)
- [Architecture](#architecture)
  - [Four-Layer Blueprint](#four-layer-blueprint)
  - [Publish-Lease TaskBoard](#publish-lease-taskboard)
  - [Plan-Execute-Reduce Pipeline](#plan-execute-reduce-pipeline)
  - [Stream Isolation](#stream-isolation)
- [Project Status](#project-status)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Configuration](#configuration)
  - [Running](#running)
  - [End-to-End Test](#end-to-end-test)
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

- Continuously collects live telemetry (vRAM, CPU, active models) from every node
- Decomposes macro-tasks into DAGs of sub-tasks with explicit resource requirements via LLM
- Runs decentralized "Task Interviews" to match sub-tasks to the best-fit node
- Executes sub-tasks in parallel across providers (vLLM, Ollama, cloud APIs)
- Reduces results into a single polished response
- Recovers transparently from node failure via TTL-bound lease expiry

---

## Architecture

![Lucinda Architecture](docs/photos/Lucinda-poster.png)

### Four-Layer Blueprint

```
┌──────────────────────────────────────────────┐
│  Layer 4 — Server & TaskWrapper              │
│  HTTP ingress, ChatRequest → Task, SSE stream│
├──────────────────────────────────────────────┤
│  Layer 3 — Task Workflow                     │
│  Plan (LLM DAG decomposition)                │
│  Execute (parallel sub-task invocation)      │
│  Reduce (LLM synthesis → unified response)   │
├──────────────────────────────────────────────┤
│  Layer 2 — Task Management                   │
│  TaskStateManager (FSM per sub-task)         │
│  TaskBoard + Publish-Lease protocol          │
│  TaskPostman (EventBus ↔ Transport bridge)   │
│  TaskTracer (observability store)            │
├──────────────────────────────────────────────┤
│  Layer 1 — Infrastructure                    │
│  EventBus (in-memory, macro signaling)       │
│  Transport (libp2p + mDNS discovery)         │
│  HardwareMonitor (live CPU/memory)           │
│  ProviderController (vLLM, Ollama, cloud)    │
│  ModuleManager (registry + capability grant) │
└──────────────────────────────────────────────┘
```

Each layer depends only on the one below it. Layers communicate through the EventBus for state transitions and the Transport for cross-node data payloads.

### Publish-Lease TaskBoard

The architectural centerpiece. Instead of a centralized scheduler, sub-tasks are published onto a shared **TaskBoard** and claimed by peers through a message protocol:

```
Publisher                    TaskBoard                   Worker
   │                             │                          │
   │── TaskBroadcastMsg ────────►│                          │
   │   (publish DAG node Ad)     │                          │
   │                             │◄── TaskRequestMsg ───────│
   │                             │    (submit Capability CV) │
   │                             │── TaskAssignMsg ────────►│
   │                             │    (TTL-bound lease)      │
   │                             │◄── TaskResultMsg ────────│
   │                             │    (completed output)     │
```

- **Lease TTL**: 30-second claim lease. If a peer doesn't call `Start` within the window, the lease expires and the sub-task reverts to Ready.
- **Self-healing**: The TaskBoard heartbeat (every 5s) re-offers expired nodes. Surviving peers re-interview and re-claim them transparently.
- **No central lock manager**: Decentralized by design — any node can host the TaskBoard.
- **Self-assignment**: If no qualified bids arrive, the publishing node executes the task locally.
- **Cross-node routing**: TaskAssignMsg carries an `OriginNodeID` so remote executors know where to send results back. The Transport bridges results to the origin's EventBus via Postman.Deliver.

### Plan-Execute-Reduce Pipeline

Every macro-task flows through three stages:

1. **Plan** — `TaskPlanner` asks an LLM to decompose the request into a DAG of sub-tasks, each with tool requirements, resource labels, and dependency edges. A reduce node is always appended to synthesize the final response. Falls back to a single-node plan if the LLM is unavailable or JSON parsing fails.
2. **Execute** — `TaskExecutor` subscribes to `TaskAssigned` events and executes sub-tasks via the assigned provider. Nodes run in **parallel goroutines** so the claim lease never expires on queued nodes. Each node's state is tracked by the `TaskStateManager` FSM (Pending→Ready→Claimed→Running→Done).
3. **Reduce** — `TaskReducer` watches for reduce-stage nodes to become Ready, collects all predecessor outputs from the `TaskTracer`, generates a synthesized response via LLM, and signals plan completion. The final output flows through the plan's `Notify` channel to the SSE client. Context truncation and fallback to raw concatenation prevent hangs when the combined input exceeds the model's context window.

### Stream Isolation

Lucinda separates control-plane and data-plane traffic:

| Traffic type | Channel | Purpose |
|---|---|---|
| State transitions (task created, lease expired) | Global **EventBus** | Macro coordination, low volume |
| Raw LLM token streams | Private **Transport** channels | High-frequency, node-to-node, bypasses EventBus |

This prevents token-volume traffic from saturating the control plane — a critical design choice for network stability under load.

---

## Project Status

| Component | Status | Location |
|---|---|---|
| **EventBus** (in-memory) | ✅ Done | `pkg/infrastructure_layer/eventbus/` |
| **Transport** (libp2p + mDNS) | ✅ Done | `pkg/infrastructure_layer/transport/transporters/` |
| **HardwareMonitor** | ✅ CPU + memory | `pkg/infrastructure_layer/hardware_monitor/` |
| **ModuleManager** | ✅ Registry + capability grant | `pkg/infrastructure_layer/module_manager/` |
| **ProviderController + Drivers** (vLLM, Ollama) | ✅ LoadProviders, Generate, Stream | `pkg/infrastructure_layer/provider/` |
| **MaxContextTokens** | ✅ Per-provider configurable context window | config + drivers |
| **Toolbox & ContextManager** | 🔴 Not started | — |
| **TaskStateManager** | ✅ DAG FSM + cascade + lease watchdog | `internal/task_management_layer/task_state_manager/` |
| **TaskBoard + Publish-Lease** | ✅ Broadcast/bid/assign/heartbeat + cross-node routing | `internal/task_workflow_layer/task_board/` |
| **TaskPostman** | ✅ Watch + Deliver bridge | `internal/task_management_layer/task_postman/` |
| **TaskTracer** | ✅ Local + assigned observability | `internal/task_management_layer/task_tracer/` |
| **CapabilityCV** | ✅ Match scoring (VRAM, model, tools, labels) | `api/v1/capability/` |
| **TaskPlanner** | ✅ LLM decomposition + fallback | `internal/task_workflow_layer/task_planner/` |
| **TaskExecutor** | ✅ Parallel execution + cross-node routing | `internal/task_workflow_layer/task_executor/` |
| **TaskReducer** | ✅ LLM synthesis + context truncation + fallback | `internal/task_workflow_layer/task_reducer/` |
| **TaskWrapper** | ✅ ChatRequest → Task + tracking ID | `internal/task_wrapper/` |
| **HTTP Server** | ✅ POST /chat, SSE /stream, /healthz, graceful shutdown | `internal/user_server/` |
| **main.go (cmd/pc)** | ✅ Config-driven bootstrap via viper + YAML | `cmd/pc/main.go` |
| **Configuration** | ✅ Providers, transport, hardware, HTTP | `configs/server/config.yaml` |
| **E2E Test** | ✅ Tests full pipeline, validates non-empty output | `scripts/test_e2e.sh` |
| **Tests** | ✅ 12 test packages | `*_test.go` |

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
    - id: "vllm-qwen"
      driver: "vllm"
      host: "localhost"
      port: 8000
      max_context_tokens: 2048
      models:
        - "qwen-2.5-gptq"

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
go run ./cmd/pc/
# → lucinda: all services started on :9090
```

Bootstrap order: Config → EventBus → Transport → HardwareMonitor → ProviderController (LoadProviders) → TaskPostman → TaskTracer → TaskStateManager → TaskBoard → TaskExecutor → TaskPlanner → TaskReducer → HTTP Server.

### End-to-End Test

```bash
bash ./scripts/test_e2e.sh
```

Sends a POST to `/chat`, polls the SSE `/stream` endpoint, validates the response is non-empty.

Manual test:

```bash
# Send a request
curl -s -X POST http://localhost:9090/chat \
  -H "Content-Type: application/json" \
  -d '{"prompt":"what is 2+2"}'
# → {"tracking_id":"plan-..."}

# Stream the result
curl -s -N "http://localhost:9090/stream?plan=<tracking_id>"
# → data: {"type":"result","text":"2+2 equals 4..."}
# → data: {"type":"done"}
```

Health check:

```bash
curl http://localhost:9090/healthz
# → ok
```

---

## Project Structure

```
lucinda/
├── api/v1/                       # Shared API types
│   ├── capability/               #   CapabilityCV + Match scoring
│   ├── chat/                     #   ChatRequest, ChatResponse, StreamChunk
│   ├── event/                    #   Event struct + EventType constants
│   ├── hardware/                 #   HardwareSnapshot types
│   ├── module/                   #   ModuleType, ModuleID, ModuleHealth
│   ├── node/                     #   NodeID, Protocol, NodeMessage
│   ├── provider/                 #   Provider interface + ProviderConfig
│   ├── task/                     #   Task, TaskPlan, TaskNode, TaskSpec
│   └── taskmsg/                  #   Wire message types (broadcast, bid, assign, result)
│
├── pkg/infrastructure_layer/     # Phase 1 — Foundation
│   ├── eventbus/                 #   In-memory EventBus
│   ├── transport/                #   Transport interface
│   │   └── transporters/         #     libp2p implementation (host, mDNS, self-worker)
│   ├── hardware_monitor/         #   CPU/memory polling + EventBus
│   ├── module_manager/           #   Module registry + capability grant
│   └── provider/                 #   ProviderController
│       └── drivers/              #     vllm, ollama (factory + init registration)
│
├── internal/                     # Business logic
│   ├── task_management_layer/
│   │   ├── task_postman/         #   EventBus ↔ Transport bridge
│   │   ├── task_state_manager/   #   Node FSM (Pending→Ready→Claimed→Running→Done)
│   │   └── task_tracer/          #   Task observability store
│   ├── task_workflow_layer/
│   │   ├── task_board/           #   Publish-Lease protocol + cross-node routing
│   │   ├── task_planner/         #   LLM decomposition + DAG construction
│   │   ├── task_executor/        #   Parallel sub-task execution
│   │   └── task_reducer/         #   LLM synthesis + context truncation
│   ├── task_wrapper/             #   ChatRequest → Task conversion
│   └── user_server/              #   HTTP ingress (chat, SSE stream, health)
│
├── cmd/
│   ├── pc/                       #   Active entrypoint (config-driven)
│   │   ├── main.go               #     Bootstrap + graceful shutdown
│   │   └── plugins.go            #     Provider driver imports
│   └── node/                     #   Legacy (commented out)
│
├── configs/server/               # YAML configuration
├── scripts/                      # test_e2e.sh
├── docs/                         # Documentation, diagrams, pipeline
└── go.mod
```

---

## Development

### Testing

```bash
# All tests
go test ./internal/... ./pkg/...

# Individual packages
go test -v ./pkg/infrastructure_layer/eventbus/
go test -v -timeout 90s ./pkg/infrastructure_layer/transport/transporters/
go test -v -timeout 30s ./pkg/infrastructure_layer/hardware_monitor/
go test -v ./pkg/infrastructure_layer/module_manager/
go test -v ./pkg/infrastructure_layer/provider/drivers/vllm/
go test -v ./internal/task_management_layer/task_state_manager/
go test -v ./internal/task_management_layer/task_tracer/
go test -v ./internal/task_management_layer/task_postman/
go test -v ./internal/task_workflow_layer/task_board/
go test -v ./internal/task_workflow_layer/task_planner/
go test -v ./internal/task_workflow_layer/task_executor/
go test -v ./internal/task_workflow_layer/task_reducer/

# End-to-end test
bash ./scripts/test_e2e.sh
```

---

## Comparison to Existing Solutions

| Dimension | LangChain / AutoGen | Ollama / vLLM | Ray / KubeEdge | **Lucinda** |
|---|---|---|---|---|
| Hardware awareness | None (cloud-centric) | Static single-node | Strong (cluster telemetry) | **Dynamic real-time telemetry** |
| Distributed scheduling | Standalone routing | No multi-node coordination | Master-worker topology | **Decentralized Publish-Lease DAG** |
| Stream-control isolation | Logic and data coupled | Raw token capture | Generic packet handling | **Private channels + EventBus split** |
| Agent-native pipeline | Macro flow graphs | Inference endpoint only | Generic container compute | **Native Plan-Execute-Reduce workflow** |
| Fault tolerance | None | None | Restart-based | **30s TTL-lease with automatic reclamation** |

---

## License

TBD

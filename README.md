# Lucinda

A **compute-aware distributed agent orchestrator** for the edge. Lucinda bridges the gap between high-level agent frameworks and the physical hardware they run on — scheduling sub-tasks across a decentralized mesh based on real-time telemetry, not static labels.

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
  - [Running a Node](#running-a-node)
- [Project Structure](#project-structure)
- [Development](#development)
  - [Testing](#testing)
  - [Implementation Roadmap](#implementation-roadmap)
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
- Decomposes macro-tasks into DAGs of sub-tasks with explicit resource requirements
- Runs decentralized "Task Interviews" to match sub-tasks to the best-fit node
- Routes high-frequency LLM token streams through private peer-to-peer channels, isolating them from control-plane traffic
- Recovers transparently from node failure via TTL-bound lease expiry

---

## Architecture

![Lucinda Architecture](docs/photos/Lucinda-poster.png)

### Four-Layer Blueprint

```
┌──────────────────────────────────────────────┐
│  Layer 4 — Server & TaskWrapper              │
│  HTTP/gRPC ingress, ChatRequest → TaskImage  │
├──────────────────────────────────────────────┤
│  Layer 3 — Task Workflow                     │
│  Plan (DAG decomposition)                    │
│  Execute (parallel agent invocations)        │
│  Reduce (token synthesis → unified response) │
├──────────────────────────────────────────────┤
│  Layer 2 — Task Management                   │
│  TaskStateManager (FSM per sub-task)         │
│  TaskBoard + Publish-Lease protocol          │
│  TaskScheduler + Capability CV matching      │
│  TaskPostman (node-to-node data routing)     │
├──────────────────────────────────────────────┤
│  Layer 1 — Infrastructure                    │
│  EventBus (lock-free, macro signaling)       │
│  Transport (libp2p, mDNS discovery)          │
│  HardwareMonitor (live vRAM/CPU/GPU)         │
│  ProviderController (Ollama, cloud APIs)     │
│  Toolbox + ContextManager                    │
└──────────────────────────────────────────────┘
```

Each layer depends only on the one below it. Layers communicate exclusively through the EventBus for state transitions and the Transport for data payloads.

### Publish-Lease TaskBoard

The architectural centerpiece. Instead of a centralized scheduler, sub-tasks are published onto a shared **TaskBoard** and claimed by peers through a five-message protocol:

```
Publisher                    TaskBoard                   Worker
   │                             │                          │
   │── TaskBroadcastMsg ────────►│                          │
   │   (inject DAG nodes,        │                          │
   │    state = Pending)         │                          │
   │                             │◄── TaskRequestMsg ───────│
   │                             │    (submit Capability CV) │
   │                             │── TaskAssignMsg ────────►│
   │                             │    (TTL-bound lease)      │
   │                             │◄── TaskAcceptMsg ────────│
   │                             │    (confirm + heartbeat)  │
   │                             │◄── TaskResultMsg ────────│
   │                             │    (completed output)     │
```

- **Lease TTL**: Workers emit low-overhead heartbeats. If a node goes silent (crash, starvation), the lease expires and the sub-task reverts to Pending.
- **Self-healing**: The TaskBoard automatically re-offers orphaned sub-tasks. Surviving peers re-interview and re-claim them transparently.
- **No central lock manager**: Decentralized by design — any node can host the TaskBoard.

### Plan-Execute-Reduce Pipeline

Every macro-task (e.g., a user ChatRequest) flows through three stages:

1. **Plan** — `TaskPlanner` decomposes the request into a DAG of sub-tasks, each labeled with resource estimates, required tools, and dependency edges.
2. **Execute** — `TaskExecutor` fires parallel invocations across the assigned providers (local Ollama, remote cloud API). Each sub-task is tracked by its own FSM in `TaskStateManager`.
3. **Reduce** — `TaskReducer` waits for all DAG leaf nodes to complete, cleans intermediate artifacts, synthesizes tokens, and streams the final response back to the user.

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
| **HardwareMonitor** | 🔴 Not started | — |
| **ProviderController + Ollama Driver** | 🟡 Legacy (internel/) | `internel/others/provider/` |
| **Toolbox & ContextManager** | 🔴 Not started | — |
| **TaskStateManager** | 🔴 Not started | — |
| **TaskBoard + Publish-Lease** | 🔴 Message types only | `api/v1/other/pumping.go` |
| **TaskScheduler + Capability CV** | 🔴 Not started | — |
| **TaskPostman** | 🔴 Not started | — |
| **TaskPlanner / Executor / Reducer** | 🟡 Stubbed (internel/) | `internel/others/taskController/` |
| **HTTP Server** | 🟡 Legacy (internel/) | `internel/others/server/` |
| **TaskWrapper** | 🟡 Partial (internel/) | `internel/others/taskController/component/` |
| **Tests** | ✅ EventBus + Transport | `*_test.go` |

**Current focus**: completing Phase 1 (Infrastructure Layer) by porting legacy `internel/` code into `pkg/`, then building Phase 2 (TaskBoard + Publish-Lease protocol).

See [docs/pipeline.md](docs/pipeline.md) for the full phased implementation plan and dependency graph.

---

## Getting Started

### Prerequisites

- **Go** 1.26+
- **Ollama** (optional — for local inference; otherwise the node can route to cloud APIs)

### Configuration

Create `configs/server/config.yaml`:

```yaml
provider_controller:
  providers:
    - id:   "ollama"
      type: "ollama"
      host: "localhost"
      port: 11434
      models:
        - "gemma3"

task_controller:
  policy:
    task_wrapper: "default"
    task_divider: "default"
    task_board:   "default"

http:
  port: 8080

transport:
  type: "libp2p"
  libp2p:
    addrs:
      - "/ip4/0.0.0.0/tcp/0"
```

### Running a Node

```bash
go run cmd/node/main.go
```

This starts a Lucinda node with:
1. An in-memory EventBus
2. A ProviderController that connects to the configured Ollama instance
3. A libp2p Transport listening on a random port with mDNS discovery
4. A TaskController (pipeline stubs)
5. An HTTP server on `:8080`

Send a test request:

```bash
curl -X POST http://localhost:8080/chat -d '{"prompt": "Hello, Lucinda"}'
```

Health check:

```bash
curl http://localhost:8080/healthz
# → Good Health
```

---

## Project Structure

```
lucinda/
├── api/v1/                  # Shared API types (versioned)
│   ├── event/               #   Event struct + EventType constants
│   │   └── eventbus/        #   Topic type
│   ├── node/                #   NodeID, Protocol, NodeMessage
│   ├── hardware/            #   Hardware status types
│   ├── task/                #   Task lifecycle types
│   ├── tool/                #   Tool definitions
│   └── user/                #   User request/response types
│
├── pkg/infrastructure_layer/  # NEW: Phase 1 foundation (in progress)
│   ├── eventbus/            #   In-memory EventBus (done)
│   └── transport/           #   Transport interface
│       └── transporters/    #     libp2p implementation (done)
│
├── internel/                # Legacy code (being ported to pkg/)
│   └── others/
│       ├── eventBus/        #   Legacy EventBus
│       ├── transport/       #   Legacy libp2p transport + Postman
│       ├── provider/        #   ProviderController + Ollama driver
│       ├── server/          #   HTTP server (factory-based)
│       ├── taskController/  #   Wrapper, Divider, Board stubs
│       ├── taskReducer/     #   Reducer stubs
│       ├── task/            #   Task model + pre-submit
│       └── monitor/         #   Hardware monitor stubs
│
├── cmd/
│   └── node/main.go         # Node entrypoint
│
├── configs/server/          # YAML configuration
├── docs/                    # Documentation, diagrams, paper
└── test/                    # Integration test fixtures
```

---

## Development

### Testing

```bash
# Unit tests — EventBus
go test -v ./pkg/infrastructure_layer/eventbus/

# Unit tests — Transport (libp2p)
go test -v -timeout 90s ./pkg/infrastructure_layer/transport/transporters/

# All tests
go test -v -timeout 90s ./pkg/...
```

### Implementation Roadmap

Follows a strict bottom-up build order. See [docs/pipeline.md](docs/pipeline.md) for details.

```
Phase 1 — Infrastructure (in progress)
  EventBus ✅ → Transport ✅ → HardwareMonitor → ProviderController → Toolbox

Phase 2 — Task Management
  TaskStateManager → TaskBoard + Publish-Lease → TaskScheduler → TaskPostman

Phase 3 — Task Workflow
  TaskPlanner → TaskExecutor → TaskReducer

Phase 4 — Server
  TaskWrapper → HTTP/gRPC Server

Phase 5 — Polish
  Stream isolation → Plugin system → Benchmarks
```

---

## Comparison to Existing Solutions

| Dimension | LangChain / AutoGen | Ollama / vLLM | Ray / KubeEdge | **Lucinda** |
|---|---|---|---|---|
| Hardware awareness | None (cloud-centric) | Static single-node | Strong (cluster telemetry) | **Dynamic real-time telemetry** |
| Distributed scheduling | Standalone routing | No multi-node coordination | Master-worker topology | **Decentralized Publish-Lease DAG** |
| Stream-control isolation | Logic and data coupled | Raw token capture | Generic packet handling | **Private channels + EventBus split** |
| Agent-native pipeline | Macro flow graphs | Inference endpoint only | Generic container compute | **Native Plan-Execute-Reduce workflow** |
| Fault tolerance | None | None | Restart-based | **TTL-lease with automatic reclamation** |

---

## License

TBD

# Task Management Layer

This layer owns **task coordination, cross-node delivery, and observability**. It is the glue between the workflow components (Layer 3) and the mesh transport (Layer 1): tasks are advertised, bid on, assigned, tracked, and their progress and results are routed back to the plan owner.

```
                    Layer 3 (Planner / Commander / Executor / Monitor)
                                     │  EventBus / Transport
┌────────────────────────────────────▼────────────────────────────────────┐
│  TaskBoard        Publish-Lease: advertise → bid (CapabilityCV) → assign │
│  TaskTracer       task lifecycle registry + the single TaskTraced signal│
│  TaskPostman      coordination messages: EventBus ↔ Transport           │
│  TelemetryBridge  user-facing progress: unicast to the plan owner       │
│  StreamRouter     final-answer token chunks: dedicated data-plane proto │
└─────────────────────────────────────────────────────────────────────────┘
```

## Components

### TaskBoard — Publish-Lease

Distributes every task (plan actions, reasoning steps, synthesis) by capability bidding. It plays both roles:

- **Employer** (plan owner): on `TaskReady`, self-bids, broadcasts a `TaskAd`, collects `CapabilityCV` bids, and after a short window assigns the best — locally, or to the winning peer via the postman. Re-advertises up to 3× if no bid qualifies; giving up fails the plan.
- **Employee** (worker): on a peer's `TaskAd`, evaluates local capability and bids; on a remote `TaskAssign`, re-publishes `TaskAssigned` locally so the executor runs.

The **matching strategy lives here** (`matchCV`: VRAM, model, tools, labels, memory-based score) — the API `CapabilityCV` type stays a pure data contract, so the policy can evolve without touching the API. Bid windows are per task kind: `reason` tasks use a short window, `execute`/`synthesize` the normal one.

### TaskTracer — lifecycle registry

Tracks tasks in two registries — tasks the node owns locally and tasks assigned to it — and emits a lightweight **`TaskTraced`** message on every state change (`Ready`/`Running`/`Done`/`Failed` + output). This is the **single progress signal** the commander and board advance from, whether the task ran locally or on a remote node.

### TaskPostman — coordination bridge

Delivers task-coordination messages between the EventBus and the Transport:

- **Transport → EventBus**: re-publishes every incoming task message (re-typed from the wire) so local components consume remote results like local ones.
- **EventBus → Transport**: auto-forwards `TaskTraced` to its plan owner (unicast); exposes `SendEvent`/`BroadcastEvent` for the board's assignments and advertisements.

### TelemetryBridge — user progress

Routes **user-facing progress** (planner.planning, commander.thinking, executor.running, step_result) back to the plan owner node, unicast over its own protocol. Local events stay local; remote events are re-published on the owner's EventBus so the monitor sees them identically.

### StreamRouter — token data plane

Carries the **final-answer token stream** on a dedicated protocol (`/lucinda/stream/1.0.0`), separate from the EventBus. A producer (e.g. a remote executor's `synthesize` task) sends `StreamChunkMsg` chunks toward the plan owner; the owner's router reconstructs a local per-plan channel that the monitor feeds to the SSE. Subscribers are keyed by plan ID, so concurrent plans never mix.

## The three channels

| Channel | Protocol | Purpose |
|---|---|---|
| Coordination | `/lucinda/taskpostman/1.0.0` | ads, CVs, assignments, `TaskTraced` routing |
| User progress | `/lucinda/telemetry/1.0.0` | unicast status/step_result to the plan owner |
| Final answer | `/lucinda/stream/1.0.0` | token chunks (never on the EventBus) |

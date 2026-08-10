## 1. API Surface (foundation types)

- [x] 1.1 Reintroduce `api/v1/messaging/taskmsg` with `TaskBroadcastMsg`, `TaskAssignMsg`, `TaskResultMsg`, each carrying task ID, spec, and `Owner`, encodable as a `NodeMessage` body
- [x] 1.2 Add telemetry `EventType` constants to `api/v1/messaging/event` (planner: planning/planned; commander: thinking/waiting/finalizing; executor: running/done; step_result)
- [x] 1.3 Add workflow `EventType` constants for the planner↔commander loop (task planned, plan done, plan completed)
- [x] 1.4 Add SSE frame envelope types (`status`, `step_result`, `stream`, `done`) to the chat/stream domain
- [x] 1.5 Add `TaskPlan.Architecture` field plus `AgentArch` constants (`plan_execute`, `react`)
- [x] 1.6 Add `TaskResult` type (task ID + output) to `api/v1/domain/task` if not already present

## 2. Payload & Owner Propagation

- [x] 2.1 Verify `TaskToTaskAd` copies `Owner` and add a unit test asserting it
- [x] 2.2 Ensure every sub-task of a plan carries `Meta.Owner` equal to the plan's `Owner`; add a test covering a multi-node plan

## 3. Component Contracts (optionally loadable)

- [x] 3.1 Implement TaskPlanner: consumes a raw task, produces `TaskPlan` (ID, Owner, DAG nodes, successors, Architecture); emits planning/planned telemetry
- [x] 3.2 Implement TaskCommander: accepts a `Task`, issues sub-tasks to an executor or produces a `taskPlanResult`; emits thinking/waiting/finalizing telemetry
- [x] 3.3 Implement TaskExecutor: accepts a `Task`, executes (Generate/Stream), produces a `TaskResult` routed back to the issuer; emits running/done telemetry
- [x] 3.4 Register all three as `AvailableModule` with no hard-wired chain (each loadable/startable independently)

## 4. Telemetry & Streaming Backbone

- [x] 4.1 Telemetry bridge: unicast telemetry events to the plan `Owner` via Transport; local fast path when owner is the local node
- [x] 4.2 Stream protocol: dedicated Transport protocol (e.g. `/lucinda/stream/1.0.0`) carrying `StreamChunk` messages; owner node reconstructs a local `<-chan StreamChunk`
- [x] 4.3 planID demux: owner node routes incoming telemetry/stream by planID to the correct session; test with two concurrent plans

## 5. TaskMonitor & SSE Egress

- [x] 5.1 TaskMonitor aggregator on the owner node: subscribes to telemetry events + rebuilds the remote stream, merges into one ordered frame stream
- [x] 5.2 SSE handler: emits `status`, `step_result`, `stream`, `done` frames; exactly one `done` per plan; plan timeout emits `done(timeout)`
- [x] 5.3 Unknown plan ID returns an error and emits no frames

## 6. HTTP Ingress

- [x] 6.1 `POST /chat`: validates messages, creates the raw task (no resource constraints), starts the workflow, returns the plan ID
- [x] 6.2 `GET /stream?plan=<id>`: opens the SSE stream scoped to the plan and demuxed by planID

## 7. Verification

- [x] 7.1 Unit tests mapping each spec scenario (chat-ingress, workflow-contracts, wire-messages, telemetry-routing)
- [x] 7.2 Cross-node test: remote executor's telemetry and final stream reach the owner (mock transport)
- [x] 7.3 End-to-end: `/chat` then `/stream` yields status → step_result → stream → done frame sequence

## ADDED Requirements

### Requirement: TaskWrapper produces a raw task with no resource constraints

The system SHALL have a component that turns a user `ChatRequest` into a raw task containing only the request prompt and messages. The raw task SHALL NOT carry resource requirements (`MinVRAM`, model, budget); those are decided downstream by the planner.

#### Scenario: ChatRequest becomes a raw task

- **WHEN** TaskWrapper receives a `ChatRequest`
- **THEN** it produces a raw task whose prompt matches the request and whose resource constraints are unset

### Requirement: TaskPlanner consumes a raw task and produces a task plan

TaskPlanner SHALL take a raw task and produce a `TaskPlan` containing: a plan ID, the `Owner` NodeID, a set of DAG nodes, successor edges, a root set, and an agent-architecture type. The plan SHALL mark which architecture (Plan-Execute or ReAct) governs its execution.

#### Scenario: Raw task is decomposed into a DAG

- **WHEN** TaskPlanner receives a raw task
- **THEN** it produces a `TaskPlan` with at least one root node and explicit successor edges

#### Scenario: Planner returns the plan to the owner node

- **WHEN** TaskPlanner completes a plan
- **THEN** the plan's `Owner` equals the node ID of the node that submitted the request

### Requirement: TaskCommander consumes a task and decides the next step

TaskCommander SHALL accept a `Task` from the planner. During an active plan it SHALL issue one or more sub-tasks to a TaskExecutor; when it determines the work is complete it SHALL produce a `taskPlanResult` that terminates the plan. These two behaviors form the reAct / Plan-Execute loop.

#### Scenario: Commander issues a sub-task to an executor

- **WHEN** TaskCommander has a task that needs sub-work
- **THEN** it sends a `Task` with an execution spec to a TaskExecutor

#### Scenario: Commander finalizes a plan

- **WHEN** TaskCommander determines all sub-work is done
- **THEN** it produces a `taskPlanResult` carrying the final result and status

### Requirement: TaskExecutor consumes a task and produces a task result

TaskExecutor SHALL accept a `Task`, execute it (via an LLM, a toolbox, or an MCP server), and produce a `TaskResult` that SHALL be routed back to the component that issued the task (normally TaskCommander).

#### Scenario: Executor returns a result to the commander

- **WHEN** a TaskExecutor finishes executing a task
- **THEN** it produces a `TaskResult` containing the task ID and output, delivered to the issuing component

#### Scenario: Executor reports progress while running

- **WHEN** a TaskExecutor starts executing a task
- **THEN** it emits a telemetry status event identifying the task and the model in use

### Requirement: Plan results flow back through the planner to the wrapper

The final `taskPlanResult` SHALL flow from TaskCommander back to TaskPlanner, which SHALL map it to the originating request and deliver the resulting `PlanResult` to TaskWrapper for the SSE response.

#### Scenario: Completed plan reaches the wrapper

- **WHEN** a plan completes
- **THEN** TaskWrapper receives a `PlanResult` and emits the terminal `done` SSE frame

### Requirement: Every task carries the owner NodeID

Every task in the workflow SHALL carry the `Owner` NodeID of the plan, propagated from `TaskPlan.Owner` through `Task.Meta.Owner` and into `TaskAd.Owner`. All sub-tasks of a plan SHALL share the same `Owner`. The `Owner` SHALL be the routing key for results and telemetry.

#### Scenario: Owner propagates from plan to advertisement

- **WHEN** a task from a plan is advertised for execution
- **THEN** the advertisement's `Owner` equals the plan's `Owner`

#### Scenario: Remote executor knows where to route results

- **WHEN** a node executes a task it did not create
- **THEN** it sends results and telemetry back to the task's `Owner` node

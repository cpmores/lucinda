## 1. Semantic Plan Model (semantic-planning)

- [x] 1.1 Add `Transaction{ID, Goal, Deps}` and `TaskPlan.Transactions`; deprecate the node-DAG fields in plan construction
- [x] 1.2 Planner: semantic decomposition prompt + JSON parser (transactions with deps), single-transaction fallback
- [x] 1.3 Planner: topological cycle check — reject a cyclic transaction graph with a plan error
- [x] 1.4 Plan carries `Architecture` (react | plan_execute) and `Goal` at plan time

## 2. Per-Transaction Commander (commander-orchestration)

- [x] 2.1 Refactor `TaskCommander` to execute a single `Transaction`: goal-scoped trajectory/steps/max-steps
- [x] 2.2 Plan-and-Execute transaction: dispatch the transaction's action once through the board
- [x] 2.3 ReAct transaction: run the existing reasoning loop against the transaction goal
- [x] 2.4 Commander emits per-transaction telemetry and streams its `done` answer via the router

## 3. Multi-Commander Orchestration (commander-orchestration + transaction-dependencies)

- [x] 3.1 New orchestrator in the workflow layer: spawn one Commander instance per transaction
- [x] 3.2 Dependency gating: start a transaction's Commander only when all `Deps` have results
- [x] 3.3 Result feeding: prepend dependency outputs to the downstream transaction's goal context
- [x] 3.4 Independent transactions run concurrently (parallel Commanders)
- [x] 3.5 Collect per-transaction results and merge into one final streamed answer
- [x] 3.6 Clean up each Commander's state on transaction completion

## 4. Verification

- [x] 4.1 Planner test: a request decomposes into semantic transactions with correct deps + architecture
- [x] 4.2 Planner test: a cyclic transaction graph is rejected
- [x] 4.3 Orchestration test: two independent transactions run in parallel and both results appear
- [x] 4.4 Orchestration test: a dependent transaction waits for its dependency and receives its output
- [x] 4.5 Single-transaction regression: current `/chat → /stream` e2e still yields `status → step_result → done` (plan_execute) and `status → step_result → stream → done` (react)
- [x] 4.6 Full `-race` suite green

## Why

The current planner decomposes a request into low-level DAG nodes and a single commander executes the whole plan. But the intended architecture is semantic: split a request into **semantic transactions** (e.g. "write a doc", "generate a video", "adjust the AC"), each handled by its **own Commander** (a ReAct agent). Parallel transactions → parallel Commanders; dependent transactions → dependency-aware orchestration. The planner runs lightweight on the phone and only picks the semantic units; the plan selects whether the request executes as ReAct (per-commander reasoning loop) or Plan-and-Execute (deterministic transaction execution).

## What Changes

- **TaskPlan model**: replace the low-level `Nodes`/`Successors` DAG with a set of semantic `Transactions`, each carrying a `Goal` and `Deps`. **BREAKING**: consumers of `TaskPlan.Nodes` must migrate to `Transactions`.
- **Semantic decomposition**: the planner's LLM prompt asks for semantic transactions (with dependencies), not fine-grained steps; keeps the goal and the chosen architecture (`react` | `plan_execute`).
- **Multi-Commander orchestration**: the execution layer spawns one Commander per transaction — concurrently for independent transactions, dependency-ordered for dependent ones — and collects each transaction's result into the final streamed answer.
- **Architecture selected at plan time**: the plan's `Architecture` decides how each transaction's Commander behaves — `react` runs the reasoning loop on the transaction goal; `plan_execute` dispatches the transaction as a single action through the board.

## Capabilities

### New Capabilities

- `semantic-planning`: the planner decomposes a request into semantic transactions (Goal + Deps) and carries the chosen architecture, replacing low-level node decomposition.
- `commander-orchestration`: the execution layer creates one Commander per transaction, runs independent ones in parallel and dependent ones in order, and collects per-transaction results into the final answer.
- `transaction-dependencies`: a transaction's Commander starts only after its dependencies complete; the dependencies' outputs are fed into the downstream transaction's context.

### Modified Capabilities

<!-- No existing specs are changing; all capabilities above are new. -->

## Impact

- `api/v1/domain/task` — `TaskPlan.Transactions` model; `TaskNode` low-level usage deprecated for plan construction.
- `internal/task_workflow_layer/task_planner` — semantic decomposition prompt + parser; transaction DAG.
- `internal/task_workflow_layer/task_commander` — per-transaction Commander instance; reuse of the existing ReAct loop and Plan-Execute dispatch at the transaction level.
- `internal/task_workflow_layer/` — new orchestrator for spawning/collecting Commanders.
- `internal/user_server`, monitor, board, tracer — reused; the tracer's `TaskTraced` already carries per-task state.
- Test scaffolding (`testutil.MockProvider`) — semantic-decomposition and multi-commander flows.

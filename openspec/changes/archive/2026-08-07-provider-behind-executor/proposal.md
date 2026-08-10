## Why

The planner and a transaction's Commander currently reach for a **local** provider directly (`plannerProvider(p.pc)` / `commanderProvider(c.pc)` → `Generate`). In a mesh the node running them may not host a suitable model, so decomposition and reasoning degrade to fallback instead of being served by a capable node. Execution already crosses nodes because it goes through the board (Publish-Lease → executor). The fix: make provider access live **only inside the TaskExecutor**, and route the planner's decomposition, the commander's reasoning, and answer synthesis through the board too — so any node with the model can serve them.

## What Changes

- **Planner becomes provider-free**: drop the `ProviderController` dependency; issue semantic decomposition as a board task, observe the result, and build the plan asynchronously. **BREAKING**: `TaskPlanner.DependsOn` loses `ProviderController`; `plan()` becomes event-driven.
- **Commander becomes provider-free**: drop the `ProviderController` and `StreamRouter` dependencies; it issues reasoning, execution, and synthesis as tasks through the board, and reads results from the tracer. **BREAKING**: `TaskCommander.DependsOn` loses two dependencies and the direct `reactDecide`/`streamSynthesis` provider calls are removed.
- **Reasoning and decomposition are board tasks**: the planner's decomposition and the commander's decisions become reason-marked `TaskReady`s; a capable node's executor runs `Generate` and returns the structured JSON via `TaskTraced`.
- **Executor task kinds**: the executor distinguishes `reason` / `execute` / `synthesize` tasks (via a spec marker) and picks provider behavior accordingly; synthesis uses `provider.Stream` and routes chunks through the stream router to the owner.
- **ReAct loop alternates**: `reason-task → execution-task → reason-task …` until `done`, then a synthesis task streams the answer.

## Capabilities

### New Capabilities

- `planner-provider-free`: the planner never touches a provider; semantic decomposition is issued as a board task and the plan is built from the observed result.
- `commander-provider-free`: the commander never touches a provider; all LLM work (reasoning, answer synthesis) is issued as tasks through the board and observed via the tracer.
- `executor-task-kinds`: the executor recognizes reason/execute/synthesize task kinds and runs the provider accordingly, enabling cross-node decomposition/reasoning and streaming synthesis.
- `synthesis-streaming`: the final answer's token chunks are generated on the executing node and streamed over the dedicated protocol to the owner node's SSE, independent of where the commander runs.

### Modified Capabilities

<!-- No existing specs are changing; all capabilities above are new. -->

## Impact

- `internal/task_workflow_layer/task_planner` — drop the provider dep; decomposition via a board task; async plan construction.
- `internal/task_workflow_layer/task_commander` — drop provider/router deps; reasoning and synthesis become board tasks; the ReAct loop restructures to alternate task kinds.
- `internal/task_workflow_layer/task_executor` — task-kind dispatch; a synthesis task streams via the router.
- `api/v1/domain/task` — a `Kind` marker on `TaskSpec` (or a dedicated reason/synthesize flag).
- `internal/task_management_layer/task_board` — reused as-is (already routes any task to a capable node).
- Tests: mock provider stays local to the executor; planner/commander tests issue tasks through the board.

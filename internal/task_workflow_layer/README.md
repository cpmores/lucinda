# Task Workflow Layer

This layer is the "brain" of Lucinda. It turns a user request into semantic transactions, orchestrates one Commander per transaction, and streams the final answer back. **The planner and commander never touch a provider** — every LLM step (decomposition, reasoning, work, synthesis) is issued as a task through the TaskBoard, and only the executor runs a model.

```
/chat → TaskWrapper → raw Task
  → TaskPlanner   (semantic decomposition as a reason task → Transaction DAG)
  → TaskCommander (one per transaction: ReAct loop or Plan-Execute dispatch)
      ├─ reason task   → board → executor Generate      (decide next step)
      ├─ execute task  → board → executor Generate      (do the work)
      └─ synthesize    → board → executor Stream        (final answer → SSE)
  → TaskMonitor    (aggregates telemetry + stream → SSE)
  → done SSE frame → user
```

## Components

### TaskPlanner — provider-free semantic decomposition

Watches `TaskPreplanned`, and instead of calling a model directly, issues a **reason-marked decomposition task** through the board. When it completes, the planner parses the returned transactions JSON into a `TaskPlan` and publishes `TaskPlanned`. Falls back to a single-transaction plan if decomposition fails. Carries the plan's `Architecture` (`react` | `plan_execute`) and `Goal`.

### TaskCommander — multi-transaction orchestration

Ingests a `TaskPlan` and runs **one Commander per transaction**:

- **Independent transactions run in parallel**; **dependent ones run in order**, with dependency outputs fed into the downstream goal context.
- Under **`react`**, a transaction runs a reasoning loop: issue a `reason` task → parse the decision (`continue`/`done`) → issue an `execute` task → observe → reason again. On `done` (or when the step budget is exhausted) it issues a `synthesize` task so the answer is generated from the trajectory.
- Under **`plan_execute`**, a transaction dispatches its goal once as an `execute` task.

Progress is judged from the **`TaskTraced`** signal: a completed task advances its transaction; a failed one terminates the plan with `PlanError`. The commander never resolves a provider — it only issues tasks and parses structured JSON.

### TaskExecutor — the only provider-touching component

Runs assigned tasks by **task kind**:

| Kind | Provider call | Purpose |
|---|---|---|
| `reason` | `Generate` (one-shot) | structured output: decisions, decomposition |
| `execute` | `Generate` (one-shot) | work results |
| `synthesize` | `Stream` | the final answer, token-chunked to the plan owner |

Because provider access lives only here, any node with a capable model can serve any step — the planner and commander's node needs no model at all.

### TaskMonitor — SSE aggregation

On the plan owner node, subscribes to user-facing telemetry (local + remote re-published by the telemetry bridge) and to the stream router's per-plan chunks, merging them into one ordered SSE frame stream: `status`, `step_result`, `stream`, then a single `done`. It only observes — never consumes control flow.

## Result & Telemetry Routing

Components may run on different nodes than the user. All user-facing output is routed back to the **plan owner node** — the node that received the request and hosts the SSE stream.

- **Owner NodeID**: every task carries it (`TaskPlan.Owner → Task.Meta.Owner → TaskAd.Owner`). It is the routing key for both results and telemetry.
- **Telemetry** (status/step_result) is **unicast to the owner** by the telemetry bridge; the monitor sees local and remote progress identically.
- **Token stream** travels on a dedicated transport protocol; the owner's stream router reconstructs a local chunk channel for the SSE.
- Every telemetry payload carries `{owner, planID, taskID}` so the owner demuxes concurrent plans.

## The single progress signal

`TaskTraced` is the only completion signal: `Done` advances a transaction, `Failed` terminates the plan. Results flow back to the owner via the postman, so the commander consumes remote results exactly like local ones.

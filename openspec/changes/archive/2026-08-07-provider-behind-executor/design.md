## Context

The planner calls the node-local provider directly for semantic decomposition (`plannerProvider(p.pc)` → `Generate`), and a transaction's Commander calls it for ReAct decisions (`commanderProvider(c.pc)` → `Generate`) and answer synthesis (`streamSynthesis` → `Stream`). All are local-only: if a node lacks a capable model, decomposition/reasoning degrade to fallback. Execution already crosses nodes via the board → executor. The architectural fix is to make the TaskExecutor the only component that touches a provider, and route decomposition, reasoning, and synthesis through the board too.

## Goals / Non-Goals

**Goals:**
- Remove `ProviderController` from the planner; remove `ProviderController` (and `StreamRouter`) from the commander; both become pure orchestration.
- Decomposition and reasoning and answer synthesis become board tasks executed by whichever node serves the model.
- The executor dispatches task kinds (reason / execute / synthesize); synthesis streams via the router.
- The ReAct loop alternates reason-task ↔ execution-task, observed via the tracer.

**Non-Goals:**
- Changing the Publish-Lease board mechanics — it already routes any task to a capable node.
- Multi-model routing per reason step — the board's capability match picks the model; fine-grained selection is future work.
- Removing the local fast path — a local executor winning the bid keeps decomposition/reasoning fast when the model is local.

## Decisions

### D1. TaskSpec gains a Kind marker

`TaskSpec.Kind` takes one of `reason | execute | synthesize` (default `execute`). The commander sets it on the tasks it issues; the executor dispatches on it. This is explicit and avoids overloading model/label conventions.

- **Alternatives considered**: distinguishing by model label (`employer=TaskCommander` vs `TaskExecutor`). Rejected — labels describe capability, not task intent; an executor node may serve both and needs to know how to run the task.
- **Trade-off**: a new spec field; defaults keep existing behavior (execute).

### D2. Commander issues reasoning as a board task

Instead of `commanderProvider(c.pc)` → `Generate`, the commander publishes a reason-marked `TaskReady` whose `Spec.Prompt` is the decision prompt and whose `Spec.Kind` is `reason`. The board broadcasts it; any node with a capable model (the reason task's label, e.g. `employer=TaskCommander`) bids and its executor runs `Generate`, returning the decision text via `TaskTraced`. The commander parses the output.

- **Alternatives considered**: keeping a direct local call with a remote fallback. Rejected — that reintroduces provider knowledge in the commander and duplicates the board's routing.
- **Trade-off**: each reasoning step costs a bid window. The local node's self-bid usually wins, so local reasoning stays fast; remote only when the model is elsewhere.

### D3. The ReAct loop alternates task kinds

A transaction's loop becomes: issue reason-task → (observe) → parse decision → issue execute-task → (observe) → issue reason-task … → `done` → issue synthesize-task. The `onTraced` handler dispatches by the completed task's kind: reason → parse + act; execute → record result + reason again; synthesize → streamed, then finish the transaction.

- **Alternatives considered**: keeping reasoning synchronous and only making execution cross-node. Rejected — the user's principle is provider access lives solely in the executor.
- **Trade-off**: the loop is now fully event-driven (two task round-trips per iteration instead of one); acceptable given the board's self-bid fast path.

### D4. The executor gains a streaming synthesize path

For a `synthesize` task the executor calls `provider.Stream`, routes each chunk as a `StreamChunkMsg` through the stream router (owner = plan owner), and reports completion. The executor gains a `StreamRouter` dependency. This removes the router from the commander and lets synthesis run on any node.

- **Alternatives considered**: the commander streaming a one-shot synthesis output. Rejected — a remote synthesis would need the commander's node to hold the stream, defeating cross-node.
- **Trade-off**: the executor now depends on three infra components (EventBus, ProviderController, StreamRouter) plus the tracer; it becomes the single provider-touching component.

### D5. The planner becomes provider-free and asynchronous

The planner drops `ProviderController`. On `TaskPreplanned` it stores the raw task as a pending plan and issues a reason-marked decomposition task through the board; when that task completes it parses the transactions JSON into the `TaskPlan` and publishes `TaskPlanned`. `plan()` becomes event-driven (issue → observe → build) instead of a synchronous `Generate` + parse.

- **Alternatives considered**: keeping a direct local call with a remote fallback. Rejected — reintroduces provider knowledge and duplicates the board's routing.
- **Trade-off**: one extra round-trip before `TaskPlanned`; the local self-bid usually wins, so the decomposition stays fast when the model is local.

### D6. The commander keeps only orchestration deps

After the change `TaskCommander` depends on `EventBus` and `TaskTracer` only. It knows the plan/transactions, the board path, and how to parse decision JSON — nothing about models or providers.

- **Trade-off**: neither the planner nor the commander can guarantee a provider exists locally; correctness now relies on the board finding a capable node, which the reason task's labels express.

### D7. Reason tasks get a short bid window

The board's employer waits `bidWindow` (150ms) before assigning, to collect remote bids. A reason task is a fast internal LLM call and the local self-bid usually wins, so per-kind windows apply: `reason` tasks use a short window (e.g. 50ms), `execute`/`synthesize` keep the normal one. This keeps the ReAct loop snappy (every reasoning step would otherwise pay a full window) without starving remote bids.

- **Alternatives considered**: skipping the window entirely when there are no peers. Rejected for now — it needs peer awareness on the board; a short fixed window is simpler.
- **Trade-off**: remote reason bids may occasionally be missed if they arrive after the short window; the local self-bid is the common case.

## Risks / Trade-offs

- [Decomposition/reasoning round-trip latency grows with each bid window] → Mitigation: local self-bid usually wins (no network hop); remote only when the model is absent locally.
- [The executor must not mis-handle a reason task as a user action] → Mitigation: the `Kind` marker is explicit and defaults to `execute`; reason/synthesize are set only by the planner/commander.
- [Streaming synthesis on a remote node depends on the stream router existing there] → Mitigation: the router is a per-node L2 module already; the executor resolves it via the registry like other deps.
- [Parser drift between the reason task's output and the planner/commander schemas] → Mitigation: shared schemas and the existing `extractJSON` parsing, kept with each consumer.

## Migration Plan

1. Add `TaskSpec.Kind` (default `execute`).
2. Executor: dispatch on `Kind`; add `StreamRouter` dep; implement the synthesize streaming path.
3. Planner: drop `ProviderController`; issue decomposition as a reason task; build the plan from the observed result.
4. Commander: drop `ProviderController`/`StreamRouter` deps; reason-task and synthesize-task issuance; rework `onTraced` to dispatch by kind; keep the decision schema.
5. Update the ReAct loop and tests (planner/commander tests now go through the board; mock provider stays in the executor).
6. Rollback: `Kind` defaults to `execute`, so existing execute-only flows are unchanged.

## Open Questions

- Should a reason task carry the decision schema hint (so the executor can validate output), or is the raw text enough? Currently raw text; the planner/commander parse.
- Does the reason task need a max-token ceiling distinct from execute tasks? Implementation detail — a `BudgetTokens` on the spec suffices.

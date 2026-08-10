## Context

The planner currently decomposes a request into a low-level DAG of `TaskNode`s and a single commander executes the whole plan (Plan-Execute) or runs one ReAct loop (goal = whole request). The intended architecture is semantic: the planner produces **semantic transactions** — high-level units such as "write the doc", "generate the video", "adjust the AC" — and each transaction is handled by its **own Commander**. The plan selects at plan time whether Commanders run ReAct loops or Plan-and-Execute dispatch.

## Goals / Non-Goals

**Goals:**
- Re-model `TaskPlan` around semantic transactions (Goal + Deps) instead of fine-grained nodes.
- Decompose semantically at plan time; keep the architecture choice (`react` | `plan_execute`).
- Spawn one Commander per transaction; run independent ones in parallel, dependent ones in order.
- Feed dependency outputs into downstream transaction context; reject cycles at plan time.

**Non-Goals:**
- Distributed placement of individual Commanders across nodes (each Commander already routes its actions through the board, so remote execution works); placing whole Commanders on remote nodes is future work.
- Fine-grained intra-transaction parallelism — a transaction's Commander stays sequential (one action in flight).

## Decisions

### D1. TaskPlan is a set of semantic transactions

`TaskPlan` gains `Transactions []Transaction`; the low-level `Nodes`/`Successors`/`PredecessorNums` DAG is removed from plan construction.

```go
type Transaction struct {
    ID   TaskID
    Goal string    // semantic unit of work
    Deps []TaskID  // prerequisite transactions
    // optional capability hints (Tools, Labels, Model) for the board.
}
```

A transaction is the unit a Commander executes. The old node DAG was an implementation artifact of the single-commander model; transactions are the user-facing semantic units.

- **Alternatives considered**: keep the node DAG and treat "transactions" as node groups. Rejected — the planner should not know about execution nodes; transactions are the contract.
- **Trade-off**: **BREAKING** for consumers of `TaskPlan.Nodes` (commander Plan-Execute path, planner parse, tests) — they migrate to transactions.

### D2. Semantic decomposition prompt

The planner's LLM prompt asks for semantic transactions and their dependencies, not fine-grained steps:

```
Decompose this request into independent or ordered transactions. Each
transaction is one deliverable the user can see ("write the doc",
"generate the video", "adjust the AC"). Reply ONLY JSON:
{"transactions":[{"id":"t1","goal":"...","deps":[]}]}
```

Fallback stays: a single transaction carrying the whole request.

- **Alternatives considered**: keep the fine-grained decomposition. Rejected — it produces node soup the commander must re-group.
- **Trade-off**: the planner is lighter (matches the phone-client vision) and pushes decisions into the Commanders.

### D3. One Commander per transaction

A new orchestrator in the workflow layer spawns a **Commander instance per transaction**. Each Commander is the existing `TaskCommander` logic scoped to a transaction: it gets the transaction's goal, a fresh trajectory, and its own max-steps. The orchestrator owns the transaction DAG (dependencies) and collects results.

- **Alternatives considered**: one commander internally switching between transaction loops. Rejected — the user's model is distinct Commanders (parallel, independently observable).
- **Trade-off**: per-transaction Commander lifecycle (spawn/cleanup) is new; the existing ReAct loop and Plan-Execute dispatch are reused per Commander.

### D4. Architecture selected at plan time governs per-Commander behavior

`plan.Architecture` decides how each transaction's Commander behaves:
- `react`: the Commander runs the reasoning loop (reason → act → observe) against the transaction's goal until it decides `done`.
- `plan_execute`: the Commander dispatches the transaction as a single action through the board and returns the result.

- **Alternatives considered**: per-transaction architecture. Rejected — one architecture for the request keeps the plan simple; per-transaction override is future work.
- **Trade-off**: mixed-mode requests aren't expressible yet.

### D5. Dependency orchestration with result feeding

The orchestrator starts a transaction's Commander only when all its `Deps` have results, and injects those results into the downstream goal context (prepended to the transaction's goal). Cycles are rejected at plan time (topological check in the planner).

- **Alternatives considered**: transactions pass results via the tracer only. Rejected — the downstream Commander needs the dep outputs in its prompt, not just a completion signal.
- **Trade-off**: the orchestrator holds a results map keyed by transaction ID; memory is bounded by the transaction count.

### D6. Final answer collection

When all transactions complete, the orchestrator combines their results into one final answer and streams it via the existing answer-streaming path (the last Commander's `done` answer, or a concatenation when multiple).

- **Trade-off**: multi-transaction answers are concatenated/LLM-merged; a single-transaction plan (the current case) behaves exactly as today.

## Risks / Trade-offs

- [Migration of the node DAG breaks the current commander/tests] → Mitigation: keep `plan_execute` behavior per transaction (single dispatch) so the e2e stays; update tests to the transaction model.
- [Spawned Commanders are lightweight state but unbounded in number] → Mitigation: bounded by transaction count; each is cleaned up on completion.
- [Dependency result feeding grows prompts] → Mitigation: pass dep outputs (not full trajectories) as context; truncate long outputs.
- [Mixed react/plan_execute transactions unsupported] → Mitigation: documented as a single per-request architecture.

## Migration Plan

1. Add `TaskPlan.Transactions`; planner emits semantic transactions + architecture; keep `Goal`.
2. Migrate the commander: Plan-Execute path dispatches transactions; ReAct path scopes the loop per transaction.
3. Add the orchestrator: spawn/collect Commanders, dependency gating, result feeding, final merge.
4. Update tests: planner semantic decomposition, multi-commander parallel/ordered, dependency feeding, cycle rejection, final streaming.
5. Rollback: a single-transaction plan reproduces current single-commander behavior.

## Open Questions

- Should a Plan-and-Execute transaction's Commander run a tiny ReAct loop internally when the goal is vague, or always single-dispatch? Currently single-dispatch; revisit if users need both.
- Where exactly do dependency outputs enter the downstream goal — appended as "context" or as an explicit field in `Transaction`? Implementation detail; the contract is that they are present.

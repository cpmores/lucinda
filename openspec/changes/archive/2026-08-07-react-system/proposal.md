## Why

The ReAct loop is partially wired (commander can reason → act → observe → terminate, proven by `TestReActLoop`), but the system is not complete: the planner always produces Plan-Execute plans, the final answer is returned one-shot instead of streamed, reasoning steps are invisible to the user, and the loop lacks hardening (max-steps fallback, decision-task failure). Proposal-1's 7-step pipeline (wrapper → planner → commander → executor → commander → planner → wrapper) cannot be considered finished until the ReAct architecture is end-to-end.

## What Changes

- **Planner produces ReAct plans**: decide the execution architecture (config-driven, defaulting to Plan-Execute) and set `TaskPlan.Architecture` + `Goal` accordingly. **BREAKING**: `TaskPlan.Architecture` becomes the authoritative execution model the commander branches on.
- **Final-answer streaming**: the commander's `done` decision streams its answer via `provider.Stream` → `stream_router` → `TaskMonitor` → SSE, replacing the one-shot `Generate` synthesis. Only the final answer is streamed; reasoning steps stay structured.
- **Reasoning telemetry**: each ReAct iteration emits a user-visible telemetry frame so clients see "thinking step N / acting on …" live.
- **Loop hardening**: max-steps cap, LLM-decision failure → graceful fallback, and a failed decision-task terminates the plan with a clear error.
- **Complete e2e**: `/chat` with a ReAct plan yields status → step_result → **stream** → done SSE frames, proving the whole pipeline.

## Capabilities

### New Capabilities

- `react-planning`: the planner selects the execution architecture (ReAct vs Plan-Execute) from configuration and produces a plan carrying `Architecture` + `Goal`, so the commander knows which mode to run.
- `react-loop`: the commander's complete ReAct loop — reasoning, dynamic task issuance through the board, observation via the tracer, reasoning telemetry, max-steps cap, LLM-failure fallback, and decision-task-failure handling.
- `answer-streaming`: the final ReAct answer streams over the dedicated transport protocol (never the EventBus) to the SSE client, with the monitor reconstructing a local chunk channel.

### Modified Capabilities

<!-- No existing specs in openspec/specs/ are being changed; all capabilities above are new. -->

## Impact

- `api/v1/domain/task` — `Architecture`/`Goal` already on `TaskPlan`; architecture config type may be added.
- `internal/task_workflow_layer/task_planner` — architecture selection; sets `ArchReAct` + `Goal` for ReAct plans.
- `internal/task_workflow_layer/task_commander` — stream the `done` answer; emit reasoning telemetry; harden the loop.
- `internal/task_management_layer/stream_router` — reused as-is for chunk transport.
- `internal/user_server` — SSE already emits `stream` frames; no change expected beyond verification.
- `configs/server/config.yaml` — architecture decision config.
- Test scaffolding (`testutil.MockProvider`) — streaming mock for the final answer.

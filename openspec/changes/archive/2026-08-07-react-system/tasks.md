## 1. Request-driven Architecture (react-planning)

- [x] 1.1 Add `Agent` field to `ChatRequest` (values `plan_execute` | `react`, default `plan_execute`)
- [x] 1.2 TaskWrapper propagates `Agent` into the raw task's `TaskPlan.Architecture`, validating/falling back to `plan_execute`
- [x] 1.3 TaskPlanner inherits the raw task's architecture into the produced plan and keeps `Goal` set
- [x] 1.4 Test: a `react` request yields a plan with `Architecture == react` and `Goal == prompt`; a request without `Agent` yields `plan_execute`

## 2. ReAct Loop Hardening (react-loop)

- [x] 2.1 Reasoning telemetry: emit a status frame per iteration (iteration number + action summary) via the existing telemetry path
- [x] 2.2 Verify/formalize max-steps: reaching `maxSteps` finalizes with collected outputs
- [x] 2.3 Verify/formalize LLM-failure fallback: decision error or invalid JSON finalizes with collected outputs
- [x] 2.4 Verify decision-task failure: a `TaskTraced Failed` on a ReAct action terminates the plan with `PlanError`
- [x] 2.5 Unit test each hardening scenario (max-steps, fallback, action failure)

## 3. Final-Answer Streaming (answer-streaming)

- [x] 3.1 Commander `DependsOn` the StreamRouter
- [x] 3.2 `streamAnswer`: on `done`, call `provider.Stream` with goal + trajectory, route `StreamChunkMsg` chunks through the router, accumulate text
- [x] 3.3 Fallback chain: streaming fails → use `decision.answer` → concatenated outputs (one-shot done frame)
- [x] 3.4 Ensure reasoning/actions emit no stream frames (only the terminal answer streams)
- [x] 3.5 Mock provider streams chunks for the synthesis prompt

## 4. Verification

- [x] 4.1 ReAct e2e: `/chat` with `Agent: react` yields SSE `status → step_result → stream → done` frames, exactly one `done`
- [x] 4.2 Plan-Execute e2e still yields `status → step_result → done` (no stream frames), regression
- [x] 4.3 Full `-race` suite green

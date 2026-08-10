## 1. Task Kind Model

- [x] 1.1 Add `TaskSpec.Kind` (`reason` | `execute` | `synthesize`, default `execute`)
- [x] 1.2 Keep existing execute behavior unchanged (default path)

## 2. Executor Task-Kind Dispatch

- [x] 2.1 Executor gains a `StreamRouter` dependency
- [x] 2.2 Dispatch on `Kind`: `reason` and `execute` → provider.Generate; `synthesize` → provider.Stream
- [x] 2.3 Implement the synthesize path: route each `StreamChunkMsg` chunk through the router toward the plan owner, report completion
- [x] 2.4 Reason/execute outputs still flow back as the task result via the tracer
- [x] 2.5 Board: per-kind bid window — `reason` tasks use a short window (e.g. 50ms), `execute`/`synthesize` keep the normal 150ms

## 3. Planner Becomes Provider-Free

- [x] 3.1 Remove `ProviderController` from `TaskPlanner.DependsOn` and `DependsEnable`
- [x] 3.2 Issue decomposition as a board task: a reason-marked `TaskReady` whose spec carries the decomposition prompt
- [x] 3.3 Build the plan asynchronously: watch the decomposition task's completion, parse the transactions JSON, publish `TaskPlanned`
- [x] 3.4 Remove the planner's direct `generate`/`parseSemantic` provider calls; keep the fallback (single transaction) on failure

## 4. Commander Becomes Provider-Free

- [x] 4.1 Remove `ProviderController` and `StreamRouter` from `TaskCommander.DependsOn` and `DependsEnable`
- [x] 4.2 Issue reasoning as a board task: a reason-marked `TaskReady` whose spec carries the decision prompt; parse the decision from its completed output
- [x] 4.3 Rework `onTraced` to dispatch by the completed task's `Kind`: reason → parse + act; execute → record + reason again; synthesize → finish the transaction
- [x] 4.4 The ReAct loop alternates reason-task ↔ execution-task until `done`, then issues a synthesize task
- [x] 4.5 The synthesize task streams the answer from the executing node (via the executor's new path); the commander no longer streams directly
- [x] 4.6 Remove the commander's direct `reactDecide`/`streamSynthesis` provider calls

## 5. Verification

- [x] 5.1 Test: the planner module no longer resolves a ProviderController
- [x] 5.2 Test: the commander module no longer resolves a ProviderController or StreamRouter
- [x] 5.3 Cross-node test: a reason task is served by a remote node's provider and its decision reaches the commander
- [x] 5.4 Test: a synthesize task streams chunks from the executing node to the owner
- [x] 5.5 Regression: `/chat → /stream` e2e (plan_execute and react) still yields the expected frame sequence
- [x] 5.6 Full `-race` suite green

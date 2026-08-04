# Development Log

## Current Branch: `fix/sse-goroutine-leak`

### Done on this branch

- [x] `PlanResult` type — replaces plain `string` on Notify channel
  - `api/v1/task/task_plan.go`: `PlanResult{Status, Text}`, four statuses (`ok`, `error`, `timeout`, `cancelled`)
  - `internal/.../manager.go`: `Complete()` sends `PlanResult{Status: PlanOK, ...}`
  - `internal/task_wrapper/wrapper.go`: signature `chan<- PlanResult`
  - `internal/user_server/server.go`: SSE handler switches on `result.Status`, uses `r.Context().Done()` to prevent goroutine leak, `streams` map guarded with `sync.Mutex`

- [x] Deadline enforcement
  - `internal/.../manager.go`: `startDeadlineWatch()` goroutine launched on `Ingest()`. Sleeps until deadline, checks `allDone`, sends `PlanResult{Status: PlanTimeout}` and disposes remaining nodes if not done.

### To finish on this branch

- [ ] Notify send in `Complete()` should use goroutine — currently blocks under `m.Lock()`
  - `manager.go:226`: `go func() { plan.Notify <- PlanResult{...} }()`

- [ ] Provider model index in `ProviderController`
  - Add `map[string]Provider` to `controller` struct
  - Populate on `Register()`
  - Use O(1) lookup in `executor.go` instead of double-loop

- [ ] Delete dead code
  - `api/v1/other/` — all files
  - `cmd/node/` — entire directory (fully commented-out main.go)

### After this branch merges

- [ ] `fix/tracer-gc` — Tracer memory cleanup
  - `local` and `assigned` maps grow unbounded
  - Add `RemoveByPlan(planID)` or call `Dispose()` cleanup

- [ ] `fix/notify-error-path` — Notify on all termination paths
  - `Dispose()` should send `PlanResult{Status: PlanCancelled}`
  - Planner generate failure should send `PlanResult{Status: PlanError}`
  - Executor permanent failure should send `PlanResult{Status: PlanError}`

- [ ] `feat/context-manager` — Phase A of roadmap
  - Session creation, message append, window trimming
  - `SessionID` on `TaskSpec` for KV cache affinity
  - `SessionAffinity` on `CapabilityCV` for TaskBoard scoring

### Previously completed (on main)

- [x] Cross-node TaskAssign delivery + TaskDone result routing
- [x] Parallel executor goroutines (prevent lease expiry on queued nodes)
- [x] Tool check guard in `CapabilityCV.Match` (skip when CV has no tools)
- [x] Board GPU timeout (2s) in `buildCV()` to prevent event-loop stall
- [x] Planner fallback when LLM unavailable
- [x] Reducer context truncation + fallback to raw concatenation
- [x] Reduce prompt fix (don't mention sub-tasks)
- [x] `MaxContextTokens` on Provider interface, configurable per-model
- [x] Config-driven bootstrap (`cmd/pc/main.go` + viper + YAML)
- [x] Graceful HTTP shutdown
- [x] Delete `internel/others/` (legacy code with broken imports)
- [x] E2E test script with stale-process cleanup and non-empty validation

### Future (roadmap)

See `docs/roadmap.md` for full plan:
- **Phase A:** ContextManager
- **Phase B:** Toolbox + MCP Bridge + Local Toolbox
- **Phase C:** Multi-Strategy Planner (ReAct + Plan-and-Solve + MetaRouter)
- **Phase D:** ToolCall plumbing, function calling, multi-agent

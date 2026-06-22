# Lucinda Roadmap

## Current State

```
Done ✅

  Layer 1   EventBus + Transport (libp2p) + HardwareMonitor + ModuleManager + ProviderController
  Layer 2   TaskStateManager + TaskBoard (Publish-Lease + cross-node routing) + TaskPostman + TaskTracer + CapabilityCV
  Layer 3   TaskPlanner (Plan-and-Solve) + TaskExecutor (parallel goroutines) + TaskReducer (synthesis with truncation)
  Layer 4   TaskWrapper + HTTP Server (chat / SSE stream / healthz) + graceful shutdown

  12 test packages passing, e2e pipeline functional, supports 9+ node DAGs.
```

---

## Vision

Lucinda's long-term goal is a **hardware-aware, multi-strategy distributed agent runtime**. When a task arrives, the system automatically decides whether to use ReAct step-by-step reasoning or Plan-and-Solve one-shot decomposition. Tool calls route transparently between MCP servers and the local toolbox. Cross-node scheduling is driven by real-time KV cache affinity and hardware telemetry.

```
                         ┌─────────────────────────┐
                         │     MetaRouter           │
                         │  task complexity →       │
                         │  strategy selection      │
                         └───────────┬───────────────┘
                  ┌──────────────────┼──────────────────┐
          ┌───────▼───────┐  ┌───────▼───────┐  ┌───────▼───────┐
          │ Plan-and-Solve│  │     ReAct      │  │    Hybrid     │
          │ one-shot DAG  │  │ Thought→Act   │  │ plan then     │
          │ parallel exec │  │ iterative loop │  │ ReAct per step│
          └───────┬───────┘  └───────┬───────┘  └───────┬───────┘
                  └──────────────────┼──────────────────┘
                                     │
                        ┌────────────▼────────────┐
                        │  Publish-Lease TaskBoard │
                        │  SessionAffinity scoring │
                        │  + CapabilityCV matching │
                        └────────────┬────────────┘
                                     │
                  ┌──────────────────┼──────────────────┐
          ┌───────▼───────┐  ┌───────▼───────┐  ┌───────▼───────┐
          │ ContextManager │  │    Toolbox     │  │   Provider    │
          │ session + KV   │  │ MCP + local    │  │ vLLM / Ollama │
          │ affinity data  │  │ tools          │  │               │
          └───────────────┘  └───────────────┘  └───────────────┘
```

---

## Phase A: Foundation — ContextManager

**Why first:** Without context management, tool results have nowhere to go, ReAct steps have no memory between iterations, and multi-turn conversations don't work.

### A.1 Session management

```
ContextManager
├── CreateSession(model, maxTokens) → SessionID
├── AppendMessage(sessionID, role, content)
├── AppendToolResult(sessionID, toolName, result)
├── GetSession(sessionID) → Session
└── In-memory storage (SQLite persistence later)
```

`Session.Messages[]` is where every ReAct step's Thought-Action-Observation accumulates.

### A.2 Window trimming

Providers report `MaxContextTokens`. ContextManager trims to fit:

```
PrepareContext(sessionID) → returns a message list sized for the LLM:
  1. System prompt (never trimmed)
  2. [optional] Early conversation summary (LLM-compressed)
  3. Last N full turns (user + assistant pairs with tool calls)
```

### A.3 KV cache affinity

`TaskSpec` gains a `SessionID` field. `CapabilityCV` gains `SessionAffinity`:

```go
type SessionAffinity struct {
    SessionID   string
    Model       string
    ContextLen  int
    LastStepAt  int64
}
```

TaskBoard's `Match()` gives an affinity bonus (+50) to a node that already holds the session's KV cache, making ReAct consecutive steps favor the same node. When a node switch is unavoidable (crash, overload), `IsContinue: false` triggers a full prefill on the new node.

---

## Phase B: Tool System — Toolbox + MCP

**Why second:** ContextManager is in place to store results. ReAct Action steps need real tools to execute.

### B.1 Unified Tool interface

```go
type Tool interface {
    Name()        string
    Description() string
    Schema()      json.RawMessage   // JSON Schema for LLM function calling
    Execute(ctx context.Context, input json.RawMessage) (string, error)
}
```

### B.2 MCP Bridge

```
MCPBridge
├── Connect to multiple MCP servers (brave_search, github, postgres, ...)
├── mcp.ListTools() — dynamic discovery of available tools
├── mcp.CallTool(name, input) — invoke a tool
└── Configured via YAML: command, args, env per server
```

### B.3 Local Toolbox

```
LocalToolbox
├── file_read / file_write    (filesystem-bound)
├── shell_exec                (sandboxed command execution)
├── python_run                (Python code execution)
└── gpu_compute               (only registered on GPU nodes)
```

Same registry pattern as Provider drivers — tools register via `init()` or explicit calls.

### B.4 Tool selector

MCP servers can expose dozens of tools. Only the relevant subset should be sent to the LLM:

```
ToolSelector.Select(taskDescription)
  ├── Tag pre-filter (zero cost)     ← tool tags: "search", "code", "file", "database"
  ├── Embedding similarity rank     ← task description vs. tool descriptions
  └── Return top-K relevant tools
```

### B.5 CapabilityCV integration

`CapabilityCV.Tools` and `CapabilityCV.ToolTags` are populated from the real toolbox. TaskBoard `Match()` can now do precise tool-based node matching — a task requiring `["web_search", "file_write"]` only matches nodes that have both.

---

## Phase C: Multi-Strategy Planning

**Why third:** Toolbox and ContextManager are both ready. ReAct Action steps have real tools to call and results go into context.

### C.1 Strategy interface

```go
type Planner interface {
    Plan(ctx context.Context, task *Task) (*TaskPlan, error)
}
```

Two implementations: `PlanAndSolvePlanner` (current behavior) and `ReActPlanner` (new).

### C.2 MetaRouter

```
User request → MetaRouter decides:
  ├── Simple ("what is 2+2")               → ReAct (2-3 steps, no pre-planning needed)
  ├── Medium ("look up France GDP, compute") → ReAct (with tools, step-by-step reasoning)
  └── Complex ("write a thermodynamics paper") → Plan-and-Solve (decompose DAG, parallel exec)
```

Routing signals: prompt length and complexity, presence of tool requirements, need for cross-domain knowledge. A single lightweight LLM call classifies the task.

### C.3 ReAct strategy

```
Loop:
  Thought: Planner reasons about what to do next
  Action:  TaskBoard assigns → Executor calls tool or LLM
  Observation: ContextManager appends result → feeds back to Planner
  Judge: stop condition? (final_answer / maxSteps / deadline)
```

Each step is a TaskNode. KV cache affinity keeps consecutive steps on the same node.

### C.4 Plan-and-Solve strategy

The existing behavior preserved as `PlanAndSolvePlanner` — one-shot LLM decomposition → DAG → parallel execution → Reduce synthesis.

---

## Phase D: Deep ToolUse Integration

**Why last:** Requires all three prior phases.

### D.1 ToolCall as first-class citizen

```go
type TaskSpec struct {
    Prompt     string      `json:"prompt"`
    Model      string      `json:"model"`
    ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // Planner → Executor
}

type ToolCall struct {
    Name   string          `json:"name"`    // "web_search"
    Input  json.RawMessage `json:"input"`   // {"query": "France GDP 2024"}
}
```

Executor sees non-empty `ToolCalls` → routes to `Toolbox.Execute()` instead of `Provider.Generate()`.

### D.2 Provider-native function calling

vLLM/Ollama OpenAI-compatible APIs support a `tools` parameter. The Planner embeds the available tool list in the LLM request, and the LLM returns `tool_calls` directly — no manual ReAct format parsing needed.

### D.3 Multi-agent collaboration

Nodes with different specializations (researcher, coder, analyst) register different tool sets. The TaskBoard's Publish-Lease protocol naturally supports cross-role scheduling — one agent's output is another agent's input.

---

## Supplementary Improvements (interleaved across phases)

| Improvement | Phase | Impact |
|-------------|-------|--------|
| SSE `context.Done()` cancellation | A | Prevents goroutine leak on client disconnect |
| Notify unlock (goroutine send) | A | `go func() { plan.Notify <- output }()` — doesn't hold StateManager lock |
| Provider model index | A | `map[model]Provider`, O(1) lookup instead of double-loop |
| Plan deadline enforcement | A | Heartbeat checks Deadline, calls Dispose + notifies on timeout |
| Tracer GC | A | `Dispose(planID)` cleans up completed plan trace data |
| Delete `api/v1/other/` + `cmd/node/` | A | Remove dead code |
| Node overload protection | C | Node temporarily opts out of bidding when overloaded |
| Cross-node streaming | D | LLM token streams over Transport, bypassing EventBus |
| Benchmark suite | D | End-to-end latency, scheduling overhead, recovery time |

---

## Dependency Graph

```
                     ContextManager (A)
                    /                \
                   /                  \
           Toolbox + MCP (B)    TaskSpec.SessionID
                   │             SessionAffinity
                   │             KV cache affinity
                   │                  │
                   └──────┬───────────┘
                          │
                   Multi-Strategy (C)
                   ├── ReAct
                   └── Plan-and-Solve
                          │
                   ToolUse Integration (D)
                   ├── ToolCall plumbing
                   ├── Function calling
                   └── Multi-agent
```

## Build Order

```
Now (immediate payoff):
  1. SSE goroutine leak fix              ~1 hour
  2. Delete dead code                    ~30 min
  3. Notify unlock (goroutine send)      ~10 min

This iteration:
  4. Plan deadline enforcement           ~2 hours
  5. Tracer GC                           ~1 hour
  6. Provider model index                ~30 min

Next major iteration:
  7. ContextManager (Phase A)            ~1-2 weeks
  8. Toolbox + MCP (Phase B)             ~1-2 weeks
  9. Multi-Strategy Planner (Phase C)    ~1 week
  10. Deep ToolUse Integration (Phase D)  ~1-2 weeks
```

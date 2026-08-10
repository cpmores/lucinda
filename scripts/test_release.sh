#!/bin/bash
# Smoke the two-layer failure flow: without a reachable provider, a task
# fails → the executor emits TaskTraced{Released} → the board reassigns (or
# gives up when candidates are exhausted) → the plan ends with done(error).
#
# It asserts the sequence from the server log, so the server must be started
# by this script (output captured to /tmp/lucinda-release.log).
#
# Env overrides: PORT (default 9090), TIMEOUT (default 60), LOG, CFG.
set -e

PORT="${PORT:-9090}"
URL="http://localhost:$PORT"
TIMEOUT="${TIMEOUT:-60}"
LOG="${LOG:-/tmp/lucinda-release.log}"
BIN="${BIN:-/tmp/lucinda-server}"
CFG="${CFG:-/tmp/lucinda-release-config.yaml}"

cd "$(dirname "$0")/.."

# Clean any stale instance, build once, start with log capture. The manifest
# points at a dead provider port so every task fails → Released → give-up.
fuser -k "$PORT"/tcp >/dev/null 2>&1 || true
sleep 1

cat > "$CFG" << 'EOF'
apiVersion: lucinda.dev/v1
kind: NodeConfig
metadata:
  name: release-node
spec:
  http:
    port: 9090
  providers:
    - id: dead-vllm
      driver: vllm
      host: localhost
      port: 9999
      models:
        - id: qwen-2.5-gptq
          labels:
            employer: "TaskPlanner,TaskCommander,TaskExecutor"
EOF

echo "== Building Lucinda =="
go build -o "$BIN" ./cmd/pc/
"$BIN" -config "$CFG" > "$LOG" 2>&1 &
SRV=$!
sleep 3

cleanup() {
    kill "$SRV" 2>/dev/null || true
    fuser -k "$PORT"/tcp >/dev/null 2>&1 || true
    rm -f "$CFG"
}
trap cleanup EXIT

if ! curl -s "$URL/healthz" >/dev/null 2>&1; then
    echo "FAIL: server not reachable"
    exit 1
fi
echo "== server OK =="

# Send a request. Without a reachable provider the task will fail and go
# through the Released → give-up flow.
echo "== sending request =="
RESP=$(curl -s -X POST "$URL/chat" -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":[{"type":"text","text":"test the release flow"}]}]}')
PLAN=$(echo "$RESP" | grep -o '"plan_id":"[^"]*"' | cut -d'"' -f4)
if [ -z "$PLAN" ]; then
    echo "FAIL: no plan_id in response: $RESP"
    exit 1
fi
echo "plan_id: $PLAN"

STREAM=$(timeout "$TIMEOUT" curl -s -N "$URL/stream?plan=$PLAN" 2>/dev/null || true)
if [ -z "$STREAM" ]; then
    echo "FAIL: empty stream"
    exit 1
fi

DONE_STATUS=$(echo "$STREAM" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "done status: $DONE_STATUS"

echo "== release markers in server log =="
grep -a -E "releasing back|self-assigned \(retry\)|assigned to peer \(retry\)|no more candidates|failing plan" "$LOG" | tail -10

# Validate: exactly one done frame.
if [ "$(echo "$STREAM" | grep -c '"event":"done"')" != "1" ]; then
    echo "FAIL: expected exactly one done frame"
    exit 1
fi

# Validate the executor emitted Released.
if ! grep -a -q "releasing back" "$LOG"; then
    echo "FAIL: executor never released the task (no 'releasing back')"
    exit 1
fi
# Validate the board eventually gave up (candidates exhausted → Failed).
if ! grep -a -q "no more candidates" "$LOG"; then
    echo "FAIL: board never gave up (no 'no more candidates')"
    exit 1
fi

echo "== PASS: Released → board give-up → done(${DONE_STATUS:-?}) =="

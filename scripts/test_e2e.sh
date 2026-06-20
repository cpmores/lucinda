#!/bin/bash
set -e

PORT=9090
URL="http://localhost:$PORT"

echo "=== Starting Lucinda ==="

# Kill any stale instance from a previous run
fuser -k $PORT/tcp 2>/dev/null || true
sleep 1

go run ./cmd/pc/ >/tmp/lucinda_e2e.log 2>&1 &
PID=$!
sleep 4

# Check server is up
if ! curl -s "$URL/healthz" >/dev/null 2>&1; then
    echo "FAIL: server not reachable"
    kill $PID 2>/dev/null
    exit 1
fi
echo " Server OK"

# Send chat request
echo "=== Sending request ==="
RESP=$(curl -s -X POST "$URL/chat" \
    -H "Content-Type: application/json" \
    -d '{"prompt":"Explain the three laws of thermodynamics, compare them to the laws of motion, and give a real-world example of each."}')
TRACKING_ID=$(echo "$RESP" | grep -o '"tracking_id":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TRACKING_ID" ]; then
    echo "FAIL: no tracking_id in response: $RESP"
    kill $PID 2>/dev/null
    exit 1
fi
echo "Tracking ID: $TRACKING_ID"

# Wait for vLLM + cascade (up to 30s)
echo "=== Waiting for result (up to 60s) ==="
STREAM=$(timeout 1000 curl -s -N "$URL/stream?plan=$TRACKING_ID" 2>/dev/null || true)

if [ -z "$STREAM" ]; then
    echo "FAIL: no stream response after 30s"
    echo "=== Server log (last 20 lines) ==="
    tail -20 /tmp/lucinda_e2e.log
    kill $PID 2>/dev/null
    exit 1
fi

echo "=== Result ==="
echo "$STREAM"

# Check server log (use -a to treat as text since libp2p may write binary)
echo ""
echo "=== Server activity ==="
grep -a -E "planner.*->|submitted bid|awarded|executor|TaskDone|Complete|Notify|error|Error" /tmp/lucinda_e2e.log | head -20

kill $PID 2>/dev/null
echo ""

# Validate the output is non-empty
if echo "$STREAM" | grep -q '"text":""'; then
    echo "=== FAIL: result text is empty ==="
    exit 1
fi

echo "=== PASS ==="

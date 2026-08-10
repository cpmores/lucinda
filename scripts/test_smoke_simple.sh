#!/bin/bash
# Simple smoke: one plan-and-execute request, validating status → step_result
# → done frames (no streaming; that is the complex test_smoke.sh's job).
#
# Usage:
#   scripts/test_smoke_simple.sh           # server must already be running
#   scripts/test_smoke_simple.sh --start   # start it first (via start_lucinda.sh)
#
# Env overrides: PORT (default 9090), TIMEOUT (default 60).
set -e

PORT="${PORT:-9090}"
URL="http://localhost:$PORT"
TIMEOUT="${TIMEOUT:-60}"

if [ "$1" = "--start" ]; then
    scripts/start_lucinda.sh
fi

if ! curl -s "$URL/healthz" >/dev/null 2>&1; then
    echo "FAIL: server not reachable at $URL (run scripts/start_lucinda.sh first)"
    exit 1
fi
echo "== server OK =="

BODY='{"messages":[{"role":"user","content":[{"type":"text","text":"写一首关于大海的诗"}]}]}'

echo "== sending request (plan_execute) =="
RESP=$(curl -s -X POST "$URL/chat" -H 'Content-Type: application/json' -d "$BODY")
PLAN=$(echo "$RESP" | grep -o '"plan_id":"[^"]*"' | cut -d'"' -f4)
if [ -z "$PLAN" ]; then
    echo "FAIL: no plan_id in response: $RESP"
    exit 1
fi
echo "plan_id: $PLAN"

echo "== streaming (timeout ${TIMEOUT}s) =="
STREAM=$(timeout "$TIMEOUT" curl -s -N "$URL/stream?plan=$PLAN" 2>/dev/null || true)
if [ -z "$STREAM" ]; then
    echo "FAIL: empty stream after ${TIMEOUT}s"
    exit 1
fi

echo "== frame summary =="
echo "$STREAM" | awk '
    /"event":"/ {
        line=$0
        if (match(line, /"event":"[^"]*"/)) {
            ev=substr(line, RSTART+9, RLENGTH-10)
            count[ev]++
        }
    }
    END {
        printf "  status:      %d\n", count["status"] + 0
        printf "  step_result: %d\n", count["step_result"] + 0
        printf "  stream:      %d\n", count["stream"] + 0
        printf "  done:        %d\n", count["done"] + 0
    }
'

DONE_LINE=$(echo "$STREAM" | sed -n '/"event":"done"/,/^$/p' | tr -d '\n')
DONE_STATUS=$(echo "$DONE_LINE" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)
DONE_TEXT=$(echo "$DONE_LINE" | grep -o '"text":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "done status: $DONE_STATUS"
echo "done text length: ${#DONE_TEXT}"

echo "== final answer =="
echo "$DONE_TEXT"

# VERBOSE=1 dumps every raw SSE frame.
if [ "${VERBOSE:-0}" = "1" ]; then
    echo "== raw frames =="
    echo "$STREAM"
fi

if [ "$(echo "$STREAM" | grep -c '"event":"done"')" != "1" ]; then
    echo "FAIL: expected exactly one done frame"
    exit 1
fi

if [ "$DONE_STATUS" = "ok" ]; then
    for ev in status step_result; do
        if ! echo "$STREAM" | grep -q "\"event\":\"$ev\""; then
            echo "FAIL: ok plan is missing $ev frames"
            exit 1
        fi
    done
    if [ -z "$DONE_TEXT" ]; then
        echo "FAIL: ok plan has empty done text"
        exit 1
    fi
    echo "== PASS (ok): status → step_result → done =="
else
    echo "== PASS (pipeline up, status=$DONE_STATUS) — is vLLM running? =="
fi

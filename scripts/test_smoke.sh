#!/bin/bash
# Smoke test: /chat → /stream, exercising a ReAct reasoning loop so the
# stream carries status / step_result / stream / done frames.
#
# Usage:
#   scripts/test_smoke.sh                 # server must already be running
#   scripts/test_smoke.sh --start         # start it first (via start_lucinda.sh)
#   AGENT=plan_execute scripts/test_smoke.sh  # simpler plan-and-execute agent
#
# Env overrides: PORT (default 9090), AGENT (default react), TIMEOUT (default 180).
set -e

PORT="${PORT:-9090}"
URL="http://localhost:$PORT"
AGENT="${AGENT:-react}"
TIMEOUT="${TIMEOUT:-180}"

if [ "$1" = "--start" ]; then
    scripts/start_lucinda.sh
fi

if ! curl -s "$URL/healthz" >/dev/null 2>&1; then
    echo "FAIL: server not reachable at $URL (run scripts/start_lucinda.sh first)"
    exit 1
fi
echo "== server OK =="

# A prompt that invites multiple reasoning steps (search, then create).
BODY="{\"agent\":\"$AGENT\",\"messages\":[{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"研究一下当前流行音乐，然后基于它创作一首类似风格的新歌\"}]}]}"

echo "== sending request (agent=$AGENT) =="
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

# Per-type frame tally from the SSE data payloads.
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

# Terminal frame's status + text.
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

# Validation.
if [ "$(echo "$STREAM" | grep -c '"event":"done"')" != "1" ]; then
    echo "FAIL: expected exactly one done frame"
    exit 1
fi

if [ "$DONE_STATUS" = "ok" ]; then
    # Happy path: the full sequence must be present.
    for ev in status step_result stream; do
        if ! echo "$STREAM" | grep -q "\"event\":\"$ev\""; then
            echo "FAIL: ok plan is missing $ev frames"
            exit 1
        fi
    done
    if [ -z "$DONE_TEXT" ]; then
        echo "FAIL: ok plan has empty done text"
        exit 1
    fi
    echo "== PASS (ok): status → step_result → stream → done =="
else
    echo "== PASS (pipeline up, status=$DONE_STATUS) — is vLLM running? =="
fi

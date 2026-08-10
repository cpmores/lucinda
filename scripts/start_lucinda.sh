#!/bin/bash
# Start / stop Lucinda and wait until it is healthy.
#
# Usage:
#   scripts/start_lucinda.sh            # background, pid written to /tmp/lucinda.pid
#   scripts/start_lucinda.sh stop       # stop the running server (by pid file + port)
#   BG=0 scripts/start_lucinda.sh       # foreground (Ctrl-C stops the server)
#
# Env overrides: PORT (default 9090), BIN (default /tmp/lucinda-server).
set -e

PORT="${PORT:-9090}"
BIN="${BIN:-/tmp/lucinda-server}"
URL="http://localhost:$PORT"
BG="${BG:-1}"

# Run from the repo root so configs/server/config.yaml resolves.
cd "$(dirname "$0")/.."

# stop: kill by pid file first, then anything still holding the port.
if [ "$1" = "stop" ]; then
    if [ -f /tmp/lucinda.pid ]; then
        kill "$(cat /tmp/lucinda.pid)" 2>/dev/null || true
        rm -f /tmp/lucinda.pid
    fi
    fuser -k "$PORT"/tcp >/dev/null 2>&1 || true
    echo "== Lucinda stopped =="
    exit 0
fi

# Kill any stale instance holding the port.
fuser -k "$PORT"/tcp >/dev/null 2>&1 || true
sleep 1

# Build once to a fixed binary so we can kill exactly this process later
# (go run spawns a child that survives killing the `go run` pid).
echo "== Building Lucinda =="
go build -o "$BIN" ./cmd/pc/

if [ "$BG" = "1" ]; then
    "$BIN"
    #"$BIN" > /tmp/lucinda.log 2>&1 &
    SRV=$!
    echo "$SRV" >/tmp/lucinda.pid
    # echo "== Lucinda started (pid $SRV), log at /tmp/lucinda.log =="
    echo "== Lucinda started (pid $SRV) =="
else
    exec "$BIN"
fi

# # Wait for the health endpoint.
# echo "== Waiting for $URL =="
# # for _ in $(seq 1 30); do
# #     if curl -s "$URL/healthz" >/dev/null 2>&1; then
# #         echo "== Lucinda ready at $URL =="
# #         exit 0
# #     fi
# #     sleep 1
# # done
#
# echo "FAIL: server not reachable at $URL"
# echo "=== log tail ==="
# tail -20 /tmp/lucinda.log
exit 1

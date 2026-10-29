#!/usr/bin/env bash
# 本地开发后端：编译固定二进制后重启，避免 go run 的包装进程被回收导致服务停掉。
# 用法：scripts/restart-local-backend.sh
set -euo pipefail
cd "$(dirname "$0")/.."

PORT="${CANVAS_BACKEND_PORT:-8080}"
DATA_DIR="${CANVAS_BACKEND_DATA_DIR:-.local/project-workbench-debug}"
BINARY=".local/canvas-server"
LOG=".local/backend-dev.log"

# 先编译，编译失败时不动正在运行的服务。
(cd backend && go build -o "../$BINARY" ./cmd/server)

for pid in $(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN -t 2>/dev/null | sort -u); do
    kill "$pid" 2>/dev/null || true
done
for _ in $(seq 1 20); do
    lsof -nP -iTCP:"$PORT" -sTCP:LISTEN -t >/dev/null 2>&1 || break
    sleep 0.25
done

CANVAS_BACKEND_DATA_DIR="$DATA_DIR" nohup "$BINARY" >"$LOG" 2>&1 &

for _ in $(seq 1 40); do
    if curl -fsS -o /dev/null "http://localhost:$PORT/api/health" 2>/dev/null; then
        echo "backend ready: http://localhost:$PORT (pid $(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN -t | head -1))"
        exit 0
    fi
    sleep 0.5
done

echo "backend failed to start on :$PORT, see $LOG" >&2
tail -5 "$LOG" >&2
exit 1

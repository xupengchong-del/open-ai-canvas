#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WEB_DIR="${REPO_DIR}/web"
LOG_DIR="${REPO_DIR}/.local/cache"
LOG_FILE="${LOG_DIR}/web.log"
PORT=3050

mkdir -p "${LOG_DIR}"

echo "=================================================="
echo "Checking and starting frontend on port ${PORT}..."
echo "=================================================="

# Check if port is already in use
OLD_PID=$(lsof -nP -iTCP:${PORT} -sTCP:LISTEN 2>/dev/null | tail -1 | awk '{print $2}' || true)
if [ -n "${OLD_PID}" ]; then
    echo "Port ${PORT} is occupied by PID ${OLD_PID}, stopping it..."
    kill -9 "${OLD_PID}" 2>/dev/null || true
    sleep 1
    echo "Old process stopped."
fi

if ! command -v bun &>/dev/null; then
    echo "Error: bun is not installed or not in PATH."
    exit 1
fi

cd "${WEB_DIR}"
echo "Starting Vite in background..."
nohup bun run vite --host 0.0.0.0 --port ${PORT} >> "${LOG_FILE}" 2>&1 &
NEW_PID=$!

echo "Waiting for port ${PORT} to be ready..."
for i in {1..15}; do
    if lsof -nP -iTCP:${PORT} -sTCP:LISTEN >/dev/null 2>&1; then
        echo "=================================================="
        echo "Frontend started successfully!"
        echo "URL: http://localhost:${PORT}"
        echo "PID: ${NEW_PID}"
        echo "Log: ${LOG_FILE}"
        echo "=================================================="
        exit 0
    fi
    sleep 1
done

echo "Failed to start frontend within 15 seconds. Last logs:"
tail -20 "${LOG_FILE}"
exit 1

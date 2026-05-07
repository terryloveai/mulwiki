#!/bin/bash
# scripts/dev.sh — Start Mulwiki in development mode
set -e

echo "=== Mulwiki Dev ==="
echo ""

# Start Go server
echo "[1/2] Starting Go backend on :8080..."
cd "$(dirname "$0")/../server"
go run ./cmd/server &
GO_PID=$!

# Start Next.js dev
echo "[2/2] Starting Web frontend on :3000..."
cd "$(dirname "$0")/../apps/web"
pnpm dev &
WEB_PID=$!

echo ""
echo "Go backend PID: $GO_PID"
echo "Web frontend PID: $WEB_PID"
echo ""
echo "Press Ctrl+C to stop both."
trap "kill $GO_PID $WEB_PID 2>/dev/null; exit" INT TERM
wait

#!/usr/bin/env bash
# 关闭数字分身前后端(不动数据库)
# 用法: scripts/shutdown.sh
set -e
cd "$(dirname "$0")/.."

ROOT="$(pwd)"
BACKEND_PORT="${AVATAR_BACKEND_PORT:-8080}"
FRONTEND_PORT="${AVATAR_FRONTEND_PORT:-5173}"

kill_port() {
  local port="$1"
  local label="$2"
  local pids
  pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -z "$pids" ]; then
    echo "==> $label (:$port) 未在监听,跳过"
    return 0
  fi
  echo "==> 关闭 $label (:$port) pid: $pids"
  # 先温和再强制
  kill $pids 2>/dev/null || true
  sleep 1
  pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -n "$pids" ]; then
    kill -9 $pids 2>/dev/null || true
  fi
}

# 顺带清掉本仓库里残留的 go run / vite 子进程(端口已被占时仍有用)
kill_pattern() {
  local pattern="$1"
  local label="$2"
  local pids
  pids="$(pgrep -f "$pattern" 2>/dev/null || true)"
  if [ -z "$pids" ]; then
    return 0
  fi
  echo "==> 清理 $label 残留进程: $pids"
  kill $pids 2>/dev/null || true
  sleep 0.5
  pids="$(pgrep -f "$pattern" 2>/dev/null || true)"
  if [ -n "$pids" ]; then
    kill -9 $pids 2>/dev/null || true
  fi
}

echo "==> 关闭同事分身前后端(数据库不动)"
kill_port "$BACKEND_PORT" "Go 后端"
kill_port "$FRONTEND_PORT" "Vite 前端"
kill_pattern "$ROOT/server.*(go run|colleague-avatar)" "Go 后端"
kill_pattern "$ROOT/web.*vite" "Vite 前端"

echo "==> 完成. MySQL/docker 未改动."

#!/usr/bin/env bash
# 启动数字分身: MySQL(docker) + Go 后端 + React 前端
# 用法: scripts/start.sh [--skip-db]
set -e
cd "$(dirname "$0")/.."

# 加载仓库根 .env（密钥等，勿提交）
if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

MODE="${1:-all}"

if [ "$MODE" != "--skip-db" ]; then
  echo "==> 启动 MySQL (docker compose)"
  docker compose up -d mysql
else
  echo "==> 跳过 MySQL (假定已启动)"
fi

echo "==> 等待 MySQL 就绪"
for i in {1..30}; do
  if docker exec colleague-avatar-mysql mysqladmin ping -uroot -proot --silent >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "==> 启动 Go 后端 (端口 8080)"
cd server
export AVATAR_ADDR="${AVATAR_ADDR:-:8080}"
export AVATAR_DB_DSN="${AVATAR_DB_DSN:-root:root@tcp(127.0.0.1:3307)/colleague_avatar?parseTime=true&charset=utf8mb4&loc=Local}"
export AVATAR_CLAUDE_BIN="${AVATAR_CLAUDE_BIN:-claude}"
# 局域网 IP 白名单: 留空=放行全部; 填逗号分隔的 IP,如 AVATAR_ALLOWED_IPS=192.168.1.10,127.0.0.1
export AVATAR_ALLOWED_IPS="${AVATAR_ALLOWED_IPS:-127.0.0.1}"
export AVATAR_WORKSPACE_ROOT="${AVATAR_WORKSPACE_ROOT:-/Users/tianhaowen/Desktop/code_work_space}"
# 分身单次回答超时(秒),默认 28800s=8h(v0.2.4；避免约 15min signal: killed)
export AVATAR_TIMEOUT_SEC="${AVATAR_TIMEOUT_SEC:-28800}"
# 数据查询库 DSN(只读账号): 默认不启用,需要时由调用环境显式注入。
export AVATAR_DATA_DB_DSN="${AVATAR_DATA_DB_DSN:-}"
# 工具授权代理: 用户点同意的等待秒数 + hook 可执行文件
export AVATAR_PERMISSION_ENABLED="${AVATAR_PERMISSION_ENABLED:-1}"
export AVATAR_PERMISSION_WAIT_SEC="${AVATAR_PERMISSION_WAIT_SEC:-120}"
# AI 自动审核单次调用超时(秒)
export AVATAR_REVIEW_TIMEOUT_SEC="${AVATAR_REVIEW_TIMEOUT_SEC:-20}"
export AVATAR_HOOK_BIN="${AVATAR_HOOK_BIN:-$(pwd)/bin/avatar-hook}"
# 待授权 Bark 提醒脚本在仓库根 scripts/(当前 cwd 为 server/)
export AVATAR_NOTIFY_SCRIPT="${AVATAR_NOTIFY_SCRIPT:-$(cd .. && pwd)/scripts/notify-permission.sh}"
export AVATAR_BARK_NOTIFY="${AVATAR_BARK_NOTIFY:-1}"
chmod +x "$AVATAR_NOTIFY_SCRIPT" 2>/dev/null || true

echo "==> 构建 PreToolUse hook (bin/avatar-hook)"
mkdir -p bin
go build -o bin/avatar-hook ./cmd/avatar-hook || { echo "hook 构建失败,授权代理将不可用"; export AVATAR_PERMISSION_ENABLED=0; }

# 从常见 shell 配置注入 BARK_KEY(若当前环境未设置)
if [[ -z "${BARK_KEY:-}" ]]; then
  for f in "$HOME/.zshrc" "$HOME/.zprofile" "$HOME/.bashrc" "$HOME/.profile"; do
    if [[ -f "$f" ]]; then
      k=$(grep -E '^\s*(export\s+)?BARK_KEY=' "$f" 2>/dev/null | tail -1 | sed -E 's/.*BARK_KEY=//; s/^["'\'']//; s/["'\''].*$//; s/[[:space:]]*$//')
      if [[ -n "${k:-}" ]]; then
        export BARK_KEY="$k"
        break
      fi
    fi
  done
fi

go run . &
SERVER_PID=$!
trap "kill $SERVER_PID 2>/dev/null" EXIT

sleep 2
echo "==> 启动 React 前端 (vite, 局域网可访问)"
cd ../web
npm run dev

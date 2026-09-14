#!/usr/bin/env bash
# 重启数字分身前后端(不动数据库)
# 用法: scripts/restart.sh
set -e
cd "$(dirname "$0")/.."

echo "==> 重启前后端(跳过数据库)"
"$(dirname "$0")/shutdown.sh"
sleep 1
exec "$(dirname "$0")/start.sh" --skip-db

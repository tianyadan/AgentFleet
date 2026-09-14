#!/usr/bin/env bash
# 智能体待授权 / 通用 Bark 提醒(薄封装 ~/bin/ai-notify.sh)。
#
# 用法:
#   scripts/notify-permission.sh "智能体#3 待授权: ls -la /tmp"
#   scripts/notify-permission.sh "摘要" [agent名]
#
# 环境:
#   BARK_KEY          必填(与 ai-notify.sh 相同)
#   AVATAR_NOTIFY_BIN 可选,覆盖 ai-notify 路径
#   AVATAR_BARK_NOTIFY=0 时调用方不应执行本脚本;脚本自身不拦截
set -euo pipefail

MSG="${1:-有命令等待授权执行}"
AGENT="${2:-avatar}"
NOTIFY_BIN="${AVATAR_NOTIFY_BIN:-$HOME/bin/ai-notify.sh}"

if [[ ! -x "$NOTIFY_BIN" ]]; then
  echo "ERROR: notify bin missing or not executable: $NOTIFY_BIN" >&2
  exit 1
fi

if [[ -z "${BARK_KEY:-}" ]]; then
  # 尝试从常见 shell 配置加载(后端子进程可能没有 interactive env)
  for f in "$HOME/.zshrc" "$HOME/.zprofile" "$HOME/.bashrc" "$HOME/.profile"; do
    if [[ -f "$f" ]]; then
      # shellcheck disable=SC1090
      set +e
      # 只抽取 BARK_KEY,避免执行整份 rc
      k=$(grep -E '^\s*(export\s+)?BARK_KEY=' "$f" | tail -1 | sed -E 's/.*(BARK_KEY=)//; s/^["'\'']//; s/["'\'']$//; s/\s*$//')
      set -e
      if [[ -n "${k:-}" ]]; then
        export BARK_KEY="$k"
        break
      fi
    fi
  done
fi

exec "$NOTIFY_BIN" "$AGENT" "warning" "$MSG" "$(date +%s)" ""

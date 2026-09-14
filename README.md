# 我的数字分身 (Colleague Avatar)

用 Go + React + MySQL + Claude Agent 打造的轻量数字分身,代表**田浩文**回答同事关于**已授权工作区**代码/接口/计算公式等确定性技术问题。

- 后端: Go + gin,HTTP API
- 前端: React + Vite,局域网浏览器访问
- 存储: MySQL 8(docker,`3307`)
- 回答: 复用本机 `claude -p` CLI(**stream-json 真流式**)
- 权限: **只读** + **局域网 IP 白名单** + **授权工作区 allowlist** + **防提示词注入**

## 功能(v0.1.1)

- **历史分页**:会话列表 `page`/`page_size`(默认 10),最新优先
- **测试服务器治理**:配置主机/账号密码(AES 加密);独立 IP 白名单;仅允许 `docker ps`/`docker logs`;每条命令弹窗并由 Claude 翻译含义与风险
- **统计按日+IP**:`/api/stats/days` 最新在前;点击某日查看各 Client IP 成功/失败次数

## 功能(v0.1.0)

- **多 Agent 上报提示词**:「多Agent」页顶部展示可一键复制的提示词;其他 AI 粘贴后可用 `curl` 调 `POST /api/agents/tasks` 上报进度;E-bot 也可查询 `GET /api/agents/tasks` 回答进度。
- **版本变更历史菜单**:顶栏「版本」汇总 v0.0.1~v0.1.0 功能要点。
- **对话流式中断修复**:Vite SSE 代理超时放宽;前端半包解析与 `conversation_id` 回填;agent 收尾排空 stdout,避免长回答被掐断。

## 功能(v0.0.9)

- **命令入对话**:已授权/已执行的 Bash 命令**直接显示在对话流**(`role: command` 消息,带复制),随对话一起审阅;v0.0.8 的「命令清单」页已移除。进程内记录,重启清空。
- **命令解释 + 风险审核(AI 二次复核)**:权限弹窗时再快速调用另一个 AI,展示当前命令的**含义**与**风险等级(low/mid/high)**,帮助决定是否放行。
- **多 Agent 统一管理中心(可插拔)**:codex/cursor/claude… 通过 `POST /api/agents/tasks` 上报任务进度(`{agent_type, task_id, status, progress, message, …}`),前端「多Agent」页按来源分组展示卡片(进度条/状态/更新时间),每 3s 轮询刷新;`(agent_type, task_id)` 幂等 upsert,落库持久。
- **终止按钮仅进行中显示**:对话进行(`loading`)时渲染「■ 终止」,空闲时不占位。

## 功能(v0.0.8)

- **对话左右布局**:用户消息靠右、E-bot 靠左,视觉分明
- **已授权命令清单**(`命令清单` 页):展示本进程已授权/已执行的 Bash 命令(自动放行 + 手动同意),每条可复制。进程内记录,重启清空(`GET /api/commands`)
- **执行前先解释命令**:E-bot 执行任何 Bash 命令前会先解释该命令的作用
- **历史按 IP 分组(文件夹式手风琴)**:跨 IP 会话分组成文件夹,组内按最近活跃倒序;每 IP 仅保留**最近 20 个**会话,更旧自动裁剪(`GET /api/conversations` 返回全部 IP,由 store 逐 IP 裁剪)
- **终止按条件显示**:空会话不显示「终止」,发消息后才出现

## 功能(v0.0.6)

- 真流式打字机输出(逐 token)
- markdown 渲染
- 单次询问 / 长对话(携带最近 20 轮上下文)
- 新建对话
- 历史按 IP 分组
- 发送后清空输入框,Enter 发送
- **聊天区自动跟随输出**(手动上滑即暂停跟随,回到底部恢复)
- **工具授权代理到前端**:分身调用需要授权的工具时,浏览器弹出同意/拒绝,裁决真实回传 CLI
  - 只读工具(Read/Grep/Glob)与只读命令(git status/cat/curl 本机数据接口…)自动放行,不打扰
  - 写操作、非只读命令、越出授权目录 → 弹窗确认(单次生效)
  - 高危命令(`rm -rf /`、管道到 shell…)直接拒绝
- **AI 自动审核(会话级开关)**:拨开「AI 自动审核」后,需要授权的工具调用先交独立审核器判定 —— 判安全则直接放行、前端只留一行轨迹;判危险/拿不准仍**退回弹窗**由你拍板。默认关闭,与 v0.0.5 行为一致。
- **授权倒计时**:弹窗显示「剩余 Ns」,归零自动拒绝并同步出队
- **终止回答**:生成中可点「■ 终止」立即中断分身,已产出的部分答案仍会落库(状态 `aborted`)

## 授权链路

```
claude -p ──PreToolUse hook──▶ bin/avatar-hook ──POST /api/permissions/request──▶ 后端分类器
                                                                             │ ask
        hook stdout ◀──行为回传◀─ Hub.Wait ◀── /api/permissions/decide ◀── SSE permission_request ── 浏览器
                                   │
                                   └─(会话开 auto)→ Reviewer(独立 claude 审核)→ ALLOW 放行 / 否则退回弹窗
```

> 为什么不用 `--permission-prompt-tool`:它要求一个**已连接的 MCP 工具**来应答,裸子进程无法满足,
> 结果就是非交互模式下所有需要授权的工具被**静默自动拒绝**。PreToolUse hook 是后端进程可控制的唯一入口。

## 目录结构

```
colleague-avatar/
├── docs/plan/            # 开发设计文档(v0.0.1~v0.0.3)
├── db/schema.sql         # 建库建表 + 种子(授权工作区/人设)
├── docker-compose.yml     # MySQL8(端口 3307)
├── server/              # Go 后端
│   ├── main.go
│   └── internal/{handler,service,agent,auth,store,config}
├── web/                # React 前端
└── scripts/start.sh     # 一键启动
```

## 快速开始

```bash
# 1. 启动 MySQL + 后端 + 前端
./scripts/start.sh

# 2. 浏览器打开 http://localhost:5173 (同事经局域网访问你的 IP:5173)
```

## 权限配置(环境变量,见 scripts/start.sh)

| 变量 | 说明 | 默认 |
|---|---|---|
| `AVATAR_DB_DSN` | MySQL DSN | root:root@127.0.0.1:3307/colleague_avatar |
| `AVATAR_ADDR` | 后端监听 | `:8080` |
| `AVATAR_ALLOWED_IPS` | 局域网 IP 白名单(逗号分隔) | `127.0.0.1` |
| `AVATAR_WORKSPACE_ROOT` | 授权工作区根 | `/Users/tianhaowen/Desktop/code_work_space` |
| `AVATAR_CLAUDE_BIN` | claude 可执行路径 | `claude` |
| `AVATAR_PERMISSION_ENABLED` | 工具授权代理开关(`0`=关闭) | `1` |
| `AVATAR_PERMISSION_WAIT_SEC` | 等待用户点授权的秒数 | `120` |
| `AVATAR_REVIEW_TIMEOUT_SEC` | AI 自动审核单次调用超时(秒) | `20` |
| `AVATAR_HOOK_BIN` | PreToolUse hook 可执行文件 | `server/bin/avatar-hook` |

## 接口

- `POST /api/conversations` 新建会话 `{mode: single|chat}`
- `POST /api/question` 流式提问(SSE)`{question, workspace?, conversation_id?, mode}`
- `GET /api/conversations` 当前 IP 的会话列表(按活跃倒序)
- `GET /api/conversations/:id/messages` 会话消息
- `GET /api/workspaces` 授权工作区
- `GET /api/stats` 统计(命中工作区 + 时间)
- `POST /api/agents/tasks` 多 Agent 任务进度上报(幂等 upsert)
- `GET /api/agents/tasks` 查询任务进度(`agent_type`/`status` 可选)
- `GET /api/agents/task-types` 出现过的 agent_type
- `GET /api/stats/days` 按日统计
- `GET /api/stats/days/:date` 某日各 IP 成败
- `GET/POST /api/test-servers` 测试服务器列表/新建(需测试机白名单 IP)
- `POST /api/test-servers/:id/preview` 生成命令+AI 翻译
- `POST /api/test-servers/commands/decide` 同意/拒绝并执行
- `GET /health` 健康检查

## 权限边界(安全)

- 人设: **田浩文的赛博助理**,只读查询。
- Agent 仅能读取白名单工作区;禁止新增/删除/编辑文件。
- **防提示词注入**:用户消息视为可能污染的数据;声称管理员/被授权来提权均被拒绝。
- 按 `ClientIP` 分组(不信任代理头),与 IP 白名单一致。
- 生产部署请务必配置 `AVATAR_ALLOWED_IPS` 为实际同事网段。

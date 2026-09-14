// 多 Agent 进度上报提示词(给其他 AI 粘贴执行)。BASE 用相对 /api,局域网请替换为实际 origin。
export function buildAgentPrompt(baseUrl = '') {
  const api = (baseUrl || (typeof window !== 'undefined' ? window.location.origin : 'http://127.0.0.1:5173')).replace(/\/$/, '')
  return `# 赛博助手 · 多 Agent 任务进度上报

你正在协助田浩文的赛博助手(E-bot)。请在执行任务过程中,把进度上报到统一管理中心,便于用户在「多Agent」页或向 E-bot 查询。

## 上报接口
- Method: POST
- URL: ${api}/api/agents/tasks
- Content-Type: application/json
- 幂等键: (agent_type, task_id) — 同一任务重复 POST 会更新进度

## JSON 字段
| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| agent_type | string | 是 | 来源标识,如 cursor / codex / claude / grok |
| agent_id | string | 否 | 你的实例/会话 id |
| task_id | string | 是 | 任务唯一 id(建议 slug 或 UUID) |
| task_name | string | 否 | 人类可读任务名 |
| status | string | 是 | running / done / failed / waiting |
| progress | number | 否 | 0-100 |
| message | string | 否 | 当前进度说明 |
| payload | object | 否 | 任意附加结构化信息 |

## curl 示例

开始任务:
\`\`\`bash
curl -sS -X POST '${api}/api/agents/tasks' \\
  -H 'Content-Type: application/json' \\
  -d '{
    "agent_type": "cursor",
    "agent_id": "session-1",
    "task_id": "feat-login-20260911",
    "task_name": "实现登录页",
    "status": "running",
    "progress": 10,
    "message": "已读代码,开始改登录表单"
  }'
\`\`\`

推进进度:
\`\`\`bash
curl -sS -X POST '${api}/api/agents/tasks' \\
  -H 'Content-Type: application/json' \\
  -d '{
    "agent_type": "cursor",
    "task_id": "feat-login-20260911",
    "task_name": "实现登录页",
    "status": "running",
    "progress": 60,
    "message": "表单与校验已完成,联调中"
  }'
\`\`\`

完成:
\`\`\`bash
curl -sS -X POST '${api}/api/agents/tasks' \\
  -H 'Content-Type: application/json' \\
  -d '{
    "agent_type": "cursor",
    "task_id": "feat-login-20260911",
    "task_name": "实现登录页",
    "status": "done",
    "progress": 100,
    "message": "已完成并通过本地检查"
  }'
\`\`\`

## 查询进度
\`\`\`bash
curl -sS '${api}/api/agents/tasks'
# 可选过滤: ?agent_type=cursor&status=running
\`\`\`

## 行为要求
1. 接到任务后立刻上报一条 status=running, progress≈5~10。
2. 每完成一个有意义的步骤再上报(不必每秒刷)。
3. 结束时必须上报 done 或 failed,并写清 message。
4. 不要修改赛博助手本体代码;只通过上述 HTTP 接口写进度。
5. 若当前环境不能访问 ${api},请把同等 JSON 交给用户手动 curl。
`
}

export const AGENT_PROMPT_STATIC = buildAgentPrompt('http://127.0.0.1:5173')

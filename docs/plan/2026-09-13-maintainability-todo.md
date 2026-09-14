# colleague-avatar 可维护性优化 TODO

| 序号 | 模块 | 当前问题 | 优化动作 | 验收方式 | 状态 |
|---|---|---|---|---|---|
| 1 | 后端 Handler | `handler.go` 仍同时承载路由、对话、历史统计、任务进度、DB 查询、授权代理等职责 | 按业务域拆出 `chat`、`stats`、`agent_tasks`、`db`、`permissions` 等 handler 文件,`Register` 仅保留路由聚合 | `go test ./...` 通过,公开 API 路由不变 | 部分完成:已拆 `agent_tasks`、`db` |
| 2 | 后端 Service | `service.go` 同时包含顶层编排、权限审核、命令审计、系统提示词、问答流程 | 保留 `Service` 门面,将权限审核/提示词/问答流程拆到独立文件 | 单元测试通过,调用方无需改动 | 部分完成:已拆 `prompt` |
| 3 | 后端 Store | `store.go` 聚合迁移、工作区、会话、消息、统计等 SQL | 按数据域拆成 `migration`、`workspace`、`conversation`、`message`、`stats` | Store 测试通过,SQL 行为不变 | 待执行 |
| 4 | 配置安全 | 远端数据查询 DSN 存在代码默认值和启动脚本默认值 | 默认不启用远端数据查询,必须通过 `AVATAR_DATA_DB_DSN` 显式注入 | 未配置时 `/api/db/*` 返回未配置,主流程可启动 | 已完成 |
| 5 | 前端 App | `App.jsx` 仍超过 1000 行,状态、SSE、历史、权限弹窗混在一起 | 抽 `useAuth`、`useChatStream`、`usePermissionQueue` 等 hook,再拆展示组件 | `npm run lint` 通过,核心聊天流程可用 | 待执行 |
| 6 | 前端 ManagedAgents | `ManagedAgents.jsx` 超过 1300 行,管理表单、会话流、授权状态、任务列表耦合 | 拆出列表、编辑面板、会话面板、任务/审计面板与请求层 | `npm run lint` 通过,管理台功能不回退 | 部分完成:已清理 lint warning |
| 7 | 注释与命名 | 部分注释记录历史版本背景较多,新读者不易区分当前规则与历史原因 | 保留关键业务约束注释,弱化过期版本叙述 | 代码扫描无明显误导性 TODO | 待执行 |

## 本轮执行范围

| 优先级 | 本轮动作 | 说明 |
|---|---|---|
| P0 | 配置安全收敛 | 已移除远端 DB DSN 默认值,避免新环境默认连真实库 |
| P1 | Handler 第一轮拆分 | 已迁移任务进度与 DB 查询这两组低耦合接口 |
| P1 | 验证 | 执行 `go test ./...`、`npm run lint`、`git diff --check` |

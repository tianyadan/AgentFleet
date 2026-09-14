# 管理台登录与内外边界 — 设计文档

日期：2026-09-11  
状态：待实现  
范围：本阶段只做管理员登录 + 对话/管理双通道拆分；管理业务页除「历史」「统计」迁入外均为占位。

## 背景

Colleague Avatar（E-bot）当前以对外对话接口为主：提问、历史、统计同处一个前端壳。现需划清边界：

- **对外**：未登录访客只能使用对话（询问已有相关能力）。
- **对内**：登录后进入管理台，后续可监控 Agent、任务计划、备忘录、常用密码、服务器指标、数据分析等。

## 目标

1. 右上角登录；账号默认 `tianhaowen` / `12345678`（可由环境变量覆盖）。
2. 未登录对话可正常使用。
3. 登录成功后进入左侧菜单 + 右侧内容的管理布局。
4. 「历史」「统计」从公开顶栏迁入管理台，并对相关 API 做 JWT 保护。

## 非目标（本阶段不做）

- 用户表、注册、改密、多用户、token 黑名单、刷新 token
- HttpOnly Cookie 会话
- Agent / 任务 / 备忘录 / 密码 / 服务器 / 数据分析等真实业务页
- 对外对话提示词收窄（「只能问已有功能」另开任务）

## 方案选择

采用 **方案 1：单管理员 + JWT + localStorage + 管理向路由中间件**。

| 决策 | 选择 |
|------|------|
| 鉴权深度 | 前后端联调（非纯前端门禁） |
| 凭证存放 | JWT 存 `localStorage`，请求头 `Authorization: Bearer <token>` |
| 登录后 | 立即进入管理布局 |
| 历史/统计 | 迁入管理台；公开顶栏仅保留提问 |

## 架构

两条通道：

| 通道 | 谁用 | 前端 | 后端 |
|------|------|------|------|
| 对外对话 | 未登录访客 | 提问页（无历史/统计 Tab） | question / 建会话 / workspaces / permissions 公开 |
| 对内管理 | 管理员 | 登录后左菜单 + 右内容 | JWT 保护历史、统计及管理向接口 |

鉴权流：

1. 右上角「登录」→ 弹窗输入账号密码
2. `POST /api/auth/login` → 返回 JWT
3. 前端写入 `localStorage`（key: `avatar_admin_token`），后续管理请求带 Bearer
4. 登录成功立刻切到管理布局；「退出」清 token 并回对话页
5. 刷新时若有 token，先 `GET /api/auth/me`；有效进管理台，无效清本地并留在对话页
6. 已持有有效 token 时，对话页右上角显示「管理台」入口（而非「登录」）

## 配置

环境变量（带默认值）：

| 变量 | 默认 | 说明 |
|------|------|------|
| `AVATAR_ADMIN_USER` | `tianhaowen` | 管理员用户名 |
| `AVATAR_ADMIN_PASS` | `12345678` | 管理员密码（明文比对；单用户起步） |
| `AVATAR_JWT_SECRET` | `colleague-avatar-jwt-dev-secret-change-me` | HS256 签名密钥（生产务必覆盖） |
| `AVATAR_JWT_TTL_HOURS` | `168`（7 天） | Token 有效期 |

落点：`server/config/config.go` 扩展字段并由 `Load()` 读取。

## 后端接口

### 新增

| 方法 | 路径 | 鉴权 | 行为 |
|------|------|------|------|
| `POST` | `/api/auth/login` | 无 | body `{username,password}` → `{token,username}`；失败 401 |
| `GET` | `/api/auth/me` | Bearer JWT | → `{username}`；无效/过期 401 |
| `POST` | `/api/auth/logout` | 可选 | 返回成功；服务端无黑名单，前端清 localStorage |

### 中间件 `RequireAdminJWT`

- 解析 `Authorization: Bearer …`，校验签名与 `exp`
- 失败统一 `401 {"error":"unauthorized"}`
- 挂到管理向路由组

### 路由分组

**公开（对话必需）**

- `POST /api/question`
- `POST /api/conversations`
- `GET /api/workspaces`
- `POST|GET /api/permissions/*`
- `GET /health`

**需 JWT**

- `GET /api/conversations`
- `GET /api/conversations/:id/messages`
- `GET /api/stats`、`/api/stats/days`、`/api/stats/days/:date`
- `GET /api/commands`
- `/api/test-servers/*`（可与原有 IP 限制叠加）

**保持公开（Agent/分身经 curl 调用，本阶段不加 JWT）**

- `/api/db/*`
- `GET|POST /api/agents/*`

说明：访客仍可提问并创建会话，不能拉全局历史/统计。管理台点开某条历史时：带 JWT 拉取消息，并切回对话页载入该会话（token 保留，便于继续聊）；列表与统计数据仅在管理台展示。

### 代码落点

- `internal/auth`：JWT 签发与校验（如 `github.com/golang-jwt/jwt/v5`）
- `internal/handler`：login / me / logout；`Register` 中分组挂中间件
- 单测：错误密码 401、正确签发、篡改/过期 token 拒绝

## 前端

### 对话页顶栏

- 左：`E-bot`
- 中：仅「提问」（移除历史、统计 Tab）
- 右：未登录「登录」；已登录「管理台」

### 登录弹窗

- 账号、密码、提交/取消
- 错误提示；提交中禁用防连点

### 管理布局

```
┌─────────────────────────────────────────┐
│ E-bot 管理台          [返回对话] [退出] │
├──────────┬──────────────────────────────┤
│ 历史     │  右侧：对应页面 / 占位       │
│ 统计     │                              │
│ ───      │                              │
│ Agent状态│  占位：「即将开放」          │
│ 任务计划 │                              │
│ 备忘录   │                              │
│ 常用密码 │                              │
│ 服务器   │                              │
│ 数据分析 │                              │
└──────────┴──────────────────────────────┘
```

- **历史 / 统计**：迁移现有 UI，请求带 Bearer
- **其余菜单**：占位页
- **返回对话**：`view=chat`，token 可保留
- **退出**：清 token → 对话页

### 状态

- `authToken` / `username`：内存 + `localStorage`
- `view`: `'chat' | 'admin'`
- `adminMenu`：当前左侧选中项
- 启动鉴权：有 token → `/api/auth/me`

### 样式

沿用现有深色主题；管理壳用 flex 左右栏；移动端侧栏可折叠或顶置简版。本阶段不新引 Ant Design（保持与现有 JSX 栈一致）。

## 错误处理

| 场景 | 行为 |
|------|------|
| 登录失败 | 弹窗内「账号或密码错误」 |
| Token 过期/非法 | 401 → 清 localStorage，提示「登录已过期」，回对话页 |
| 管理 API 无 token | 后端 401 |
| 网络失败 | 登录/me 显示简短网络错误 |

## 验收清单

1. 未登录可正常提问；顶栏无历史/统计
2. 正确账号进入管理台；错误账号不进入
3. 管理台左侧含历史、统计（可用）+ 其余占位
4. 历史/统计 API 无 JWT → 401；有 JWT → 正常
5. 刷新后 token 有效则仍在管理台
6. 「返回对话」可提问；「退出」后需重新登录
7. 后端登录相关单测通过

## 后续（不在本规格）

- 管理菜单各业务页实现
- 密码改为哈希存储 / 用户表
- 对外对话能力边界（提示词/工具白名单）
- 可选迁移至 HttpOnly Cookie

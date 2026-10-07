# 前台助理与数字人管理改造

## 目标

取消首页长短对话模式；指定唯一前台助理接待来访者；访客 Cookie 登记；后台「数字人」含子菜单「对话 / 数字人管理」。

## 数据

- `managed_agents.is_receptionist`：全表最多一个为 1
- `visitors`：`public_id`、姓名、`current_conversation_id`、时间戳
- Cookie `avatar_visitor`：HttpOnly，签 `public_id`，7 天，Ask 成功续期
- `conversations.visitor_id`；前台会话不复用数字人主会话
- `visitor_ask_logs`：visitor / conv / agent / ip / ua

## 公开 API

- `GET /api/public/receptionist`
- `GET /api/public/visitor/me` → `{registered,name?}`
- `POST /api/public/visitor/register` `{name}`
- `POST /api/public/ask` SSE，前端 fetch stream
- `POST /api/public/conversations/new`：新建会话行，更新 `current_conversation_id`，不删旧行
- 前台 Gateway：规则 + JEVOS，Human 直接拒绝，无弹窗

## 后台

- 菜单「数字人」→「对话」（现 ManagedAgents）、「数字人管理」（卡片）
- 登录默认进「对话」
- 卡片：头像/名/分组、设置/删除、多选解聘、单选前台助理黄标

## 首页 UI

- Kimi 风黑白单栏；输入区内上下文占用；轻动画
- 无模型切换、无底部工具条、无长短模式

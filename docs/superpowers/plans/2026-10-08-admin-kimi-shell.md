# Admin Kimi Shell Implementation Plan

> **For agentic workers:** Execute task-by-task. Steps use checkbox syntax.

**Goal:** 管理台全宽紧凑壳 + Kimi 黑白色板；产品名 agentFleet；后端人设去掉 E-bot，自称「{名字} 数字员工」。

**Architecture:** CSS 变量挂在 `.app-admin`；壳层布局改 `App.jsx`/`App.css`；`agent.SystemPrompt` 用 persona 名拼「数字员工」；用户可见 E-bot 文案替换。

**Tech Stack:** React/Vite CSS，Go agent prompt

## Global Constraints

- 前台 `PublicHome` 布局/交互不动
- 历史 changelog / 旧 plan 不强制改 E-bot
- 展示品牌：`agentFleet`

## Task 1: 后端人设

- [ ] 改 `agent.SystemPrompt`：去掉硬编码 E-bot；展示名为 `{Name} 数字员工`（Name 已含则不重复）
- [ ] 改 `service.SystemPrompt` 默认 persona Name（不再用 E-bot）
- [ ] 加/改单测并 `go test`

## Task 2: 品牌文案

- [ ] `web/index.html` title、`App.jsx` 顶栏/开场白、`agentPrompt.js`、`README.md` 用户可见 E-bot → agentFleet / 数字员工表述

## Task 3: 管理台壳 + 主题

- [ ] `.app-admin` 全宽、紧凑侧栏、薄顶栏、Kimi token
- [ ] 业务页硬编码 slate → 映射到中性黑白（不影响 `.ph-*`）
- [ ] `vite build` + 相关 go test
---

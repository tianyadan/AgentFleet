-- v0.3.5 数字员工独立上下文窗口配置
-- 已有 200K 窗口作为历史默认显示分母废弃；已有员工默认迁移到 1M。
-- 执行一次即可（重复运行会提示 Duplicate column）。
USE colleague_avatar;
ALTER TABLE managed_agents
  ADD COLUMN context_window_tokens BIGINT NOT NULL DEFAULT 1000000 AFTER engine;
-- 可选：针对不支持 1M 的实际模型，在后台编辑该数字员工为 256K / 512K。
-- 本配置仅影响占用率分母，不会修改 Claude CLI 模型自身的最大上下文。

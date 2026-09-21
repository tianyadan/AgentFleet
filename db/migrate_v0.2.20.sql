-- v0.2.20: 数字员工任务规划开关（默认开）
ALTER TABLE managed_agents
  ADD COLUMN task_plan_enabled TINYINT NOT NULL DEFAULT 1 AFTER auto_review;

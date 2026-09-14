-- v0.1.9: 智能体 AI 自动审核开关持久化
ALTER TABLE managed_agents
  ADD COLUMN auto_review TINYINT NOT NULL DEFAULT 0 AFTER rules_prompt;

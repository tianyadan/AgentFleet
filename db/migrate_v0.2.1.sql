-- v0.2.1: 智能体权限 / 工作区 / 定时
ALTER TABLE managed_agents
  ADD COLUMN allow_write TINYINT NOT NULL DEFAULT 1 AFTER auto_review,
  ADD COLUMN allow_network TINYINT NOT NULL DEFAULT 1 AFTER allow_write,
  ADD COLUMN allow_rm TINYINT NOT NULL DEFAULT 0 AFTER allow_network,
  ADD COLUMN workspace_path VARCHAR(1024) NOT NULL DEFAULT '' AFTER allow_rm,
  ADD COLUMN schedule_enabled TINYINT NOT NULL DEFAULT 0 AFTER workspace_path,
  ADD COLUMN schedule_cron VARCHAR(128) NOT NULL DEFAULT '' AFTER schedule_enabled,
  ADD COLUMN schedule_label VARCHAR(128) NOT NULL DEFAULT '' AFTER schedule_cron;

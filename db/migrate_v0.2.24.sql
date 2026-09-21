-- v0.2.24: 复制数字员工（头像预留 + 来源员工）
ALTER TABLE managed_agents
  ADD COLUMN avatar_url VARCHAR(512) NULL AFTER name,
  ADD COLUMN cloned_from_id BIGINT NULL;

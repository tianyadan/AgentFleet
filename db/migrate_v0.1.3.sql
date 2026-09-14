-- v0.1.3 migration: managed agents + conversations.agent_id
USE colleague_avatar;

CREATE TABLE IF NOT EXISTS managed_agents (
  id              BIGINT         NOT NULL AUTO_INCREMENT,
  name            VARCHAR(128)   NOT NULL,
  engine          VARCHAR(32)    NOT NULL,
  bin_path        VARCHAR(512)   NOT NULL DEFAULT '',
  rules_prompt    LONGTEXT       NULL,
  status          VARCHAR(32)    NOT NULL DEFAULT 'idle',
  conversation_id BIGINT         NULL,
  last_error      TEXT           NULL,
  last_run_ms     INT            NULL,
  created_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_engine (engine),
  KEY idx_status (status)
) ENGINE=InnoDB;

-- conversations.agent_id(可空;管理型智能体会话挂靠)
SET @col_exists := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = 'colleague_avatar'
    AND TABLE_NAME = 'conversations'
    AND COLUMN_NAME = 'agent_id'
);
SET @sql := IF(@col_exists = 0,
  'ALTER TABLE conversations ADD COLUMN agent_id BIGINT NULL, ADD KEY idx_agent_id (agent_id)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

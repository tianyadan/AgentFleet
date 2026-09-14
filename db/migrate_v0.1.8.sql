-- v0.1.8: agent_tasks(若缺失) + command_audits 命令审批审计
CREATE TABLE IF NOT EXISTS agent_tasks (
  id         BIGINT         NOT NULL AUTO_INCREMENT,
  agent_type  VARCHAR(64)    NOT NULL,
  agent_id    VARCHAR(128)   DEFAULT '',
  task_id     VARCHAR(128)   NOT NULL,
  task_name   VARCHAR(512)   DEFAULT '',
  status     VARCHAR(32)    NOT NULL,
  progress  INT            DEFAULT 0,
  message   TEXT            NULL,
  payload   JSON            NULL,
  created_at DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_agent_task (agent_type, task_id),
  KEY idx_agent (agent_type, updated_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS command_audits (
  id              BIGINT        NOT NULL AUTO_INCREMENT,
  conversation_id BIGINT        NOT NULL DEFAULT 0,
  agent_id        BIGINT        NOT NULL DEFAULT 0,
  tool_name       VARCHAR(64)   NOT NULL DEFAULT '',
  command_text    TEXT          NOT NULL,
  decision        VARCHAR(16)   NOT NULL,  -- allow | deny
  decided_by      VARCHAR(32)   NOT NULL,  -- user | ai | system | timeout | disconnect
  risk            VARCHAR(16)   DEFAULT '',
  meaning         TEXT          NULL,
  note            TEXT          NULL,
  created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_conv_time (conversation_id, created_at),
  KEY idx_agent_time (agent_id, created_at)
) ENGINE=InnoDB;

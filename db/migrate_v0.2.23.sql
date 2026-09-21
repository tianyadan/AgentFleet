-- v0.2.23: 项目协作「项目 × 数字员工」持久会话
CREATE TABLE IF NOT EXISTS workflow_agent_sessions (
  id              BIGINT   NOT NULL AUTO_INCREMENT,
  definition_id   BIGINT   NOT NULL,
  agent_id        BIGINT   NOT NULL,
  conversation_id BIGINT   NOT NULL,
  created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_def_agent (definition_id, agent_id),
  KEY idx_agent (agent_id),
  CONSTRAINT fk_was_def FOREIGN KEY (definition_id) REFERENCES workflow_definitions(id) ON DELETE CASCADE
) ENGINE=InnoDB;

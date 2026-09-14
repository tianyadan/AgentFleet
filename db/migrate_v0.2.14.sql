-- v0.2.14: 智能体文件夹
CREATE TABLE IF NOT EXISTS agent_folders (
  id         BIGINT       NOT NULL AUTO_INCREMENT,
  name       VARCHAR(128) NOT NULL,
  sort_order INT          NOT NULL DEFAULT 0,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id)
) ENGINE=InnoDB;

ALTER TABLE managed_agents
  ADD COLUMN folder_id BIGINT NULL AFTER name;

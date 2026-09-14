-- v0.2.7: 团队编排 / 占用 / 长期记忆
USE colleague_avatar;

CREATE TABLE IF NOT EXISTS workflow_definitions (
  id           BIGINT        NOT NULL AUTO_INCREMENT,
  name         VARCHAR(255)  NOT NULL,
  description  TEXT          NULL,
  graph_json   JSON          NOT NULL,
  version      INT           NOT NULL DEFAULT 1,
  created_at   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_updated (updated_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS workflow_runs (
  id                   BIGINT        NOT NULL AUTO_INCREMENT,
  definition_id        BIGINT        NOT NULL,
  status               VARCHAR(32)   NOT NULL DEFAULT 'pending',
  -- pending|running|waiting|success|failed|stopped|interrupted
  input_prompt         LONGTEXT      NULL,
  current_node_ids_json JSON         NULL,
  parallel_state_json  JSON          NULL,
  fail_node_id         VARCHAR(64)   NOT NULL DEFAULT '',
  fail_reason          TEXT          NULL,
  progress_json        JSON          NULL,
  resume_cursor_json   JSON          NULL,
  started_at           DATETIME      NULL,
  finished_at          DATETIME      NULL,
  created_at           DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_def (definition_id, created_at),
  KEY idx_status (status),
  CONSTRAINT fk_run_def FOREIGN KEY (definition_id) REFERENCES workflow_definitions(id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS node_executions (
  id                BIGINT        NOT NULL AUTO_INCREMENT,
  run_id            BIGINT        NOT NULL,
  node_id           VARCHAR(64)   NOT NULL,
  attempt           INT           NOT NULL DEFAULT 1,
  node_type         VARCHAR(32)   NOT NULL,
  agent_id          BIGINT        NOT NULL DEFAULT 0,
  status            VARCHAR(32)   NOT NULL DEFAULT 'pending',
  conversation_id   BIGINT        NOT NULL DEFAULT 0,
  input_json        JSON          NULL,
  output_json       JSON          NULL,
  error_text        TEXT          NULL,
  parent_attempt_id BIGINT        NULL,
  started_at        DATETIME      NULL,
  finished_at       DATETIME      NULL,
  created_at        DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_run_node (run_id, node_id, attempt),
  KEY idx_run_status (run_id, status),
  CONSTRAINT fk_ne_run FOREIGN KEY (run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS agent_occupancy (
  agent_id            BIGINT        NOT NULL,
  source_type         VARCHAR(32)   NOT NULL,
  source_id           VARCHAR(128)  NOT NULL DEFAULT '',
  source_name         VARCHAR(255)  NOT NULL DEFAULT '',
  workflow_run_id     BIGINT        NOT NULL DEFAULT 0,
  node_execution_id   BIGINT        NOT NULL DEFAULT 0,
  task_name           VARCHAR(512)  NOT NULL DEFAULT '',
  started_at          DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (agent_id),
  KEY idx_source (source_type, source_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS agent_memories (
  id             BIGINT        NOT NULL AUTO_INCREMENT,
  agent_id       BIGINT        NOT NULL,
  source_type    VARCHAR(32)   NOT NULL DEFAULT '',
  source_id      VARCHAR(128)  NOT NULL DEFAULT '',
  workflow_id    BIGINT        NOT NULL DEFAULT 0,
  workspace      VARCHAR(1024) NOT NULL DEFAULT '',
  title          VARCHAR(512)  NOT NULL DEFAULT '',
  summary        TEXT          NOT NULL,
  metadata_json  JSON          NULL,
  created_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_agent_time (agent_id, created_at),
  KEY idx_agent_source (agent_id, source_type, source_id)
) ENGINE=InnoDB;

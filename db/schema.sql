-- colleague_avatar schema v0.0.3
-- MySQL 8 (本地 docker,端口 3307)
-- v0.0.3: 引入会话模型 conversations + messages;废弃 questions 表。

CREATE DATABASE IF NOT EXISTS colleague_avatar
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;

USE colleague_avatar;

-- 授权工作区 allowlist
CREATE TABLE IF NOT EXISTS authorized_workspaces (
  id         BIGINT        NOT NULL AUTO_INCREMENT,
  name       VARCHAR(255)  NOT NULL UNIQUE,
  path       VARCHAR(1024) NOT NULL,
  type       VARCHAR(16)    NOT NULL DEFAULT 'code',  -- code / db
  enabled    TINYINT       NOT NULL DEFAULT 1,
  created_at DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_path (path(255))
) ENGINE=InnoDB;

-- 分身配置(预留人设/语气等,key-value)
CREATE TABLE IF NOT EXISTS agent_config (
  id         BIGINT        NOT NULL AUTO_INCREMENT,
  config_key VARCHAR(128) NOT NULL UNIQUE,
  config_value JSON         NULL,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id)
) ENGINE=InnoDB;

-- 会话(按 user_ip 分组;管理型智能体可挂 agent_id)
CREATE TABLE IF NOT EXISTS conversations (
  id         BIGINT        NOT NULL AUTO_INCREMENT,
  user_ip     VARCHAR(64)   NOT NULL,
  mode       VARCHAR(16)    NOT NULL DEFAULT 'single',  -- single / chat
  agent_id   BIGINT        NULL,
  created_at DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ip (user_ip, updated_at),
  KEY idx_agent_id (agent_id)
) ENGINE=InnoDB;

-- v0.1.3: 管理台智能体注册表
CREATE TABLE IF NOT EXISTS agent_folders (
  id         BIGINT       NOT NULL AUTO_INCREMENT,
  name       VARCHAR(128) NOT NULL,
  sort_order INT          NOT NULL DEFAULT 0,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS managed_agents (
  id              BIGINT         NOT NULL AUTO_INCREMENT,
  name            VARCHAR(128)   NOT NULL,
  folder_id       BIGINT         NULL,
  engine          VARCHAR(32)    NOT NULL,           -- claude | codex | agent
  bin_path        VARCHAR(512)   NOT NULL DEFAULT '',
  rules_prompt    LONGTEXT       NULL,
  auto_review     TINYINT        NOT NULL DEFAULT 0,
  allow_write     TINYINT        NOT NULL DEFAULT 1,
  allow_network   TINYINT        NOT NULL DEFAULT 1,
  allow_rm        TINYINT        NOT NULL DEFAULT 0,
  allow_browser   TINYINT        NOT NULL DEFAULT 0,
  workspace_path  VARCHAR(1024)  NOT NULL DEFAULT '',
  schedule_enabled TINYINT       NOT NULL DEFAULT 0,
  schedule_cron   VARCHAR(128)   NOT NULL DEFAULT '',
  schedule_label  VARCHAR(128)   NOT NULL DEFAULT '',
  status          VARCHAR(32)    NOT NULL DEFAULT 'idle', -- idle|running|waiting|error
  conversation_id BIGINT         NULL,
  last_error      TEXT           NULL,
  last_run_ms     INT            NULL,
  run_started_at  DATETIME       NULL,
  created_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_engine (engine),
  KEY idx_status (status),
  KEY idx_folder (folder_id)
) ENGINE=InnoDB;

-- 消息
CREATE TABLE IF NOT EXISTS messages (
  id              BIGINT         NOT NULL AUTO_INCREMENT,
  conversation_id BIGINT         NOT NULL,
  role           VARCHAR(16)     NOT NULL,            -- user / assistant / invoke_quote / invoke_divider
  content        LONGTEXT        NULL,
  status         VARCHAR(20)      NOT NULL DEFAULT 'ok', -- ok / rejected / error
  workspace_path VARCHAR(1024)   NULL,
  reply_ms       INT             NULL,
  exclude_from_context TINYINT   NOT NULL DEFAULT 0,
  meta_json      JSON            NULL,
  created_at     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_conv (conversation_id, id),
  KEY idx_ws (workspace_path(191)),
  CONSTRAINT fk_msg_conv FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
) ENGINE=InnoDB;

-- 多 Agent 统一管理中心: 各 agent(可插拔,agent_type 区分)上报任务进度
CREATE TABLE IF NOT EXISTS agent_tasks (
  id         BIGINT         NOT NULL AUTO_INCREMENT,
  agent_type  VARCHAR(64)    NOT NULL,   -- codex / cursor / claude / ...
  agent_id    VARCHAR(128)   DEFAULT '',
  task_id     VARCHAR(128)   NOT NULL,   -- agent 侧任务ID(与 agent_type 组成幂等键)
  task_name   VARCHAR(512)   DEFAULT '',
  status     VARCHAR(32)    NOT NULL,   -- running / done / failed / waiting
  progress  INT            DEFAULT 0,    -- 0-100
  message   TEXT            NULL,         -- 进度说明/汇报
  payload   JSON            NULL,         -- 附加结构化信息(可插拔扩展)
  created_at DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_agent_task (agent_type, task_id),
  KEY idx_agent (agent_type, updated_at)
) ENGINE=InnoDB;

-- v0.1.8: 命令审批审计
CREATE TABLE IF NOT EXISTS command_audits (
  id              BIGINT        NOT NULL AUTO_INCREMENT,
  conversation_id BIGINT        NOT NULL DEFAULT 0,
  agent_id        BIGINT        NOT NULL DEFAULT 0,
  tool_name       VARCHAR(64)   NOT NULL DEFAULT '',
  command_text    TEXT          NOT NULL,
  decision        VARCHAR(16)   NOT NULL,
  decided_by      VARCHAR(32)   NOT NULL,
  risk            VARCHAR(16)   DEFAULT '',
  meaning         TEXT          NULL,
  note            TEXT          NULL,
  created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_conv_time (conversation_id, created_at),
  KEY idx_agent_time (agent_id, created_at)
) ENGINE=InnoDB;

-- 种子: 授权工作区(code 类型 + 数据查询 db 项目)
INSERT INTO authorized_workspaces (name, path, type, enabled) VALUES
  ('kfi-cloud-api',            '/Users/tianhaowen/Desktop/code_work_space/kfi-cloud-api',            'code', 1),
  ('kfi-cloud-admin',         '/Users/tianhaowen/Desktop/code_work_space/kfi-cloud-admin',         'code', 1),
  ('kesong-sales-dashboard',  '/Users/tianhaowen/Desktop/code_work_space/kesong-sales-dashboard', 'code', 1),
  ('数据查询',                'db://data-query',                                                   'db',   1)
ON DUPLICATE KEY UPDATE enabled = VALUES(enabled), type = VALUES(type);

-- 种子: 人设(田浩文的赛博助理)
INSERT INTO agent_config (config_key, config_value) VALUES
  ('persona', JSON_OBJECT('name','田浩文的赛博助理','style','严谨、简洁、专业'))
ON DUPLICATE KEY UPDATE config_value = VALUES(config_value);

-- v0.1.1: 测试服务器治理
CREATE TABLE IF NOT EXISTS test_servers (
  id            BIGINT        NOT NULL AUTO_INCREMENT,
  name          VARCHAR(128)  NOT NULL DEFAULT '',
  host          VARCHAR(255)  NOT NULL,
  port          INT           NOT NULL DEFAULT 22,
  username      VARCHAR(128)  NOT NULL,
  password_enc  TEXT          NOT NULL,
  enabled       TINYINT       NOT NULL DEFAULT 1,
  created_at    DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_host_user (host, username)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS test_server_allowed_ips (
  id         BIGINT       NOT NULL AUTO_INCREMENT,
  ip         VARCHAR(64)  NOT NULL,
  note       VARCHAR(255) DEFAULT '',
  enabled    TINYINT      NOT NULL DEFAULT 1,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ip (ip)
) ENGINE=InnoDB;

INSERT INTO test_server_allowed_ips (ip, note, enabled) VALUES
  ('127.0.0.1', '本机调试', 1)
ON DUPLICATE KEY UPDATE enabled = VALUES(enabled);

-- v0.2.7: 团队编排 / 占用 / 长期记忆
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
  events_json       JSON          NULL,
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

-- v0.1.1 migration: test servers + allowlist
USE colleague_avatar;

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

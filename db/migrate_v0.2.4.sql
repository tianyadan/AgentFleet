-- v0.2.4: 允许操作浏览器
ALTER TABLE managed_agents
  ADD COLUMN allow_browser TINYINT NOT NULL DEFAULT 0 AFTER allow_rm;

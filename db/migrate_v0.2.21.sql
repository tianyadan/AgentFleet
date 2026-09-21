-- v0.2.21: 引擎压缩后下一轮重带系统提示
ALTER TABLE conversations
  ADD COLUMN needs_system_reinject TINYINT NOT NULL DEFAULT 0;

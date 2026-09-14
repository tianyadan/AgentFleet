-- v0.2.2: 跨智能体调用消息元数据 + 上下文排除
ALTER TABLE messages
  ADD COLUMN exclude_from_context TINYINT NOT NULL DEFAULT 0 AFTER reply_ms,
  ADD COLUMN meta_json JSON NULL AFTER exclude_from_context;

-- status 语义扩展: idle|running|waiting|error（应用层约定，无需改列类型）

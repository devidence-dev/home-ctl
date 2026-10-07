ALTER TABLE flow_state ADD COLUMN message_id INTEGER;
CREATE INDEX IF NOT EXISTS idx_flow_state_message ON flow_state(chat_id, thread_id, message_id);

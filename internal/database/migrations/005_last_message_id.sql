-- Add last_message_id column to reminders table for tracking sent messages
-- This allows deleting old messages before resending to avoid flooding

ALTER TABLE reminders ADD COLUMN IF NOT EXISTS last_message_id BIGINT;

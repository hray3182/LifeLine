-- Migration: 006_user_settings
-- Description: Add user settings for todo reminders

-- user_settings table
CREATE TABLE IF NOT EXISTS user_settings (
    user_id BIGINT PRIMARY KEY REFERENCES "user"(user_id) ON DELETE CASCADE,
    max_daily_reminders INTEGER DEFAULT 10,
    quiet_start TIME DEFAULT '22:00',
    quiet_end TIME DEFAULT '08:00',
    timezone VARCHAR(50) DEFAULT 'Asia/Taipei',
    reminder_intervals JSONB DEFAULT '{"overdue": 30, "urgent": 30, "soon": 120, "normal": 480}'::jsonb,
    todo_reminders_enabled BOOLEAN DEFAULT TRUE,
    last_todo_message_id BIGINT,
    daily_summary_enabled BOOLEAN DEFAULT TRUE,
    daily_summary_time TIME DEFAULT '08:00',
    last_daily_summary_date DATE,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add last_notified_at to todo table for tracking notification timing
ALTER TABLE todo ADD COLUMN IF NOT EXISTS last_notified_at TIMESTAMP;

-- Daily reminder count table for rate limiting
CREATE TABLE IF NOT EXISTS daily_reminder_count (
    user_id BIGINT REFERENCES "user"(user_id) ON DELETE CASCADE,
    date DATE NOT NULL,
    count INTEGER DEFAULT 0,
    PRIMARY KEY (user_id, date)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_todo_last_notified ON todo(last_notified_at);
CREATE INDEX IF NOT EXISTS idx_daily_reminder_count_date ON daily_reminder_count(date);

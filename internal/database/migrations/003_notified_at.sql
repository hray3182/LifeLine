-- Migration: 003_notified_at
-- Description: Add notified_at column to track notification state and prevent duplicate notifications

-- Add notified_at to event table
ALTER TABLE event ADD COLUMN IF NOT EXISTS notified_at TIMESTAMP;

-- Add notified_at to reminders table
ALTER TABLE reminders ADD COLUMN IF NOT EXISTS notified_at TIMESTAMP;

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_event_notified_at ON event(notified_at);
CREATE INDEX IF NOT EXISTS idx_reminders_notified_at ON reminders(notified_at);

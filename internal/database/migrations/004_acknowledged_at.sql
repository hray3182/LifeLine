-- Migration: 004_acknowledged_at
-- Description: Add acknowledged_at column to track when user confirmed the reminder

-- Add acknowledged_at to reminders table
ALTER TABLE reminders ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMP;

-- Create index for efficient querying
CREATE INDEX IF NOT EXISTS idx_reminders_acknowledged_at ON reminders(acknowledged_at);

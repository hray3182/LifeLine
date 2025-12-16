-- Migration: 002_rrule_support
-- Description: Add RFC 5545 RRULE support for reminders and events

-- Update reminders table: add dtstart column
ALTER TABLE reminders ADD COLUMN IF NOT EXISTS dtstart TIMESTAMP;

-- Update event table: migrate from start_time/end_time to dtstart/duration/next_occurrence
ALTER TABLE event ADD COLUMN IF NOT EXISTS dtstart TIMESTAMP;
ALTER TABLE event ADD COLUMN IF NOT EXISTS duration INTEGER DEFAULT 60;
ALTER TABLE event ADD COLUMN IF NOT EXISTS next_occurrence TIMESTAMP;

-- Migrate existing data: copy start_time to dtstart and next_occurrence
UPDATE event SET dtstart = start_time, next_occurrence = start_time WHERE start_time IS NOT NULL AND dtstart IS NULL;

-- Drop old columns from event table
ALTER TABLE event DROP COLUMN IF EXISTS start_time;
ALTER TABLE event DROP COLUMN IF EXISTS end_time;
ALTER TABLE event DROP COLUMN IF EXISTS frequency;
ALTER TABLE event DROP COLUMN IF EXISTS interval;
ALTER TABLE event DROP COLUMN IF EXISTS by_day;
ALTER TABLE event DROP COLUMN IF EXISTS until;

-- Update indexes
DROP INDEX IF EXISTS idx_event_start_time;
CREATE INDEX IF NOT EXISTS idx_event_next_occurrence ON event(next_occurrence);
CREATE INDEX IF NOT EXISTS idx_event_dtstart ON event(dtstart);

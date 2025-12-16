-- Migration: 001_init
-- Description: Initialize all tables

-- User table
CREATE TABLE IF NOT EXISTS "user" (
    user_id BIGINT PRIMARY KEY,
    user_name VARCHAR(255)
);

-- Memo table
CREATE TABLE IF NOT EXISTS memo (
    memo_id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    content TEXT,
    tags VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Todo table
CREATE TABLE IF NOT EXISTS todo (
    todo_id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    title VARCHAR(255),
    priority INTEGER DEFAULT 0,
    description TEXT,
    due_time TIMESTAMP,
    completed_at TIMESTAMP,
    tags VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Reminders table
CREATE TABLE IF NOT EXISTS reminders (
    reminders_id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT TRUE,
    recurrence_rule TEXT,
    messages TEXT,
    remind_at TIMESTAMP,
    description TEXT,
    tags VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Category table
CREATE TABLE IF NOT EXISTS category (
    category_id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    category_name VARCHAR(255),
    usage_count INTEGER DEFAULT 0
);

-- Subcategory table
CREATE TABLE IF NOT EXISTS subcategory (
    subcategory_id SERIAL PRIMARY KEY,
    category_id INTEGER NOT NULL REFERENCES category(category_id) ON DELETE CASCADE,
    subcategory_name VARCHAR(255),
    usage_count INTEGER DEFAULT 0
);

-- Transaction table
CREATE TABLE IF NOT EXISTS transaction (
    transaction_id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    category_id INTEGER REFERENCES category(category_id) ON DELETE SET NULL,
    type VARCHAR(50),
    amount DECIMAL(15, 2),
    description TEXT,
    transaction_date DATE,
    tags VARCHAR(255),
    recurrence_rule TEXT,
    frequency VARCHAR(50),
    interval INTEGER,
    by_day VARCHAR(50),
    until DATE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Event table
CREATE TABLE IF NOT EXISTS event (
    event_id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    title VARCHAR(255),
    description TEXT,
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    notification_minutes INTEGER DEFAULT 30,
    recurrence_rule TEXT,
    frequency VARCHAR(50),
    interval INTEGER,
    by_day VARCHAR(50),
    until DATE,
    tags VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_memo_user_id ON memo(user_id);
CREATE INDEX IF NOT EXISTS idx_todo_user_id ON todo(user_id);
CREATE INDEX IF NOT EXISTS idx_todo_due_time ON todo(due_time);
CREATE INDEX IF NOT EXISTS idx_reminders_user_id ON reminders(user_id);
CREATE INDEX IF NOT EXISTS idx_reminders_remind_at ON reminders(remind_at);
CREATE INDEX IF NOT EXISTS idx_category_user_id ON category(user_id);
CREATE INDEX IF NOT EXISTS idx_transaction_user_id ON transaction(user_id);
CREATE INDEX IF NOT EXISTS idx_transaction_date ON transaction(transaction_date);
CREATE INDEX IF NOT EXISTS idx_event_user_id ON event(user_id);
CREATE INDEX IF NOT EXISTS idx_event_start_time ON event(start_time);

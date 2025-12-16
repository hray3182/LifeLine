package models

import "time"

type Reminder struct {
	ReminderID     int        `json:"reminders_id"`
	UserID         int64      `json:"user_id"`
	Enabled        bool       `json:"enabled"`
	RecurrenceRule string     `json:"recurrence_rule"`
	Messages       string     `json:"messages"`
	RemindAt       *time.Time `json:"remind_at"`
	Description    string     `json:"description"`
	Tags           string     `json:"tags"`
	CreatedAt      time.Time  `json:"created_at"`
}

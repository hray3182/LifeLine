package models

import "time"

type Reminder struct {
	ReminderID     int        `json:"reminders_id"`
	UserID         int64      `json:"user_id"`
	Enabled        bool       `json:"enabled"`
	RecurrenceRule string     `json:"recurrence_rule"` // RFC 5545 RRULE
	Dtstart        *time.Time `json:"dtstart"`         // First occurrence (for RRULE calculation)
	Messages       string     `json:"messages"`
	RemindAt       *time.Time `json:"remind_at"` // Next scheduled reminder time
	Description    string     `json:"description"`
	Tags           string     `json:"tags"`
	NotifiedAt     *time.Time `json:"notified_at"`     // Last notification time for this reminder
	AcknowledgedAt *time.Time `json:"acknowledged_at"` // When user confirmed the reminder
	LastMessageID  *int       `json:"last_message_id"` // Last sent message ID for deletion before resend
	CreatedAt      time.Time  `json:"created_at"`
}

// IsRecurring returns true if this reminder has a recurrence rule
func (r *Reminder) IsRecurring() bool {
	return r.RecurrenceRule != ""
}

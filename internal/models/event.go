package models

import "time"

type Event struct {
	EventID             int        `json:"event_id"`
	UserID              int64      `json:"user_id"`
	Title               string     `json:"title"`
	Description         string     `json:"description"`
	Dtstart             *time.Time `json:"dtstart"`              // First occurrence (for RRULE calculation)
	Duration            int        `json:"duration"`             // Duration in minutes
	NextOccurrence      *time.Time `json:"next_occurrence"`      // Next scheduled occurrence
	NotificationMinutes int        `json:"notification_minutes"` // Minutes before to notify
	RecurrenceRule      string     `json:"recurrence_rule"`      // RFC 5545 RRULE
	Tags                string     `json:"tags"`
	CreatedAt           time.Time  `json:"created_at"`
}

// IsRecurring returns true if this event has a recurrence rule
func (e *Event) IsRecurring() bool {
	return e.RecurrenceRule != ""
}

// GetEndTime calculates end time based on dtstart and duration
func (e *Event) GetEndTime() *time.Time {
	if e.Dtstart == nil || e.Duration == 0 {
		return nil
	}
	endTime := e.Dtstart.Add(time.Duration(e.Duration) * time.Minute)
	return &endTime
}

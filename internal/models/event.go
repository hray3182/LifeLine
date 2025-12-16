package models

import "time"

type Event struct {
	EventID             int        `json:"event_id"`
	UserID              int64      `json:"user_id"`
	Title               string     `json:"title"`
	Description         string     `json:"description"`
	StartTime           *time.Time `json:"start_time"`
	EndTime             *time.Time `json:"end_time"`
	NotificationMinutes int        `json:"notification_minutes"`
	RecurrenceRule      string     `json:"recurrence_rule"`
	Frequency           string     `json:"frequency"`
	Interval            int        `json:"interval"`
	ByDay               string     `json:"by_day"`
	Until               *time.Time `json:"until"`
	Tags                string     `json:"tags"`
	CreatedAt           time.Time  `json:"created_at"`
}

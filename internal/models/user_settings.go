package models

import (
	"encoding/json"
	"time"
)

// ReminderIntervals defines the notification intervals for different time zones
type ReminderIntervals struct {
	Overdue int `json:"overdue"` // minutes, for overdue todos
	Urgent  int `json:"urgent"`  // minutes, for todos due within 2 hours
	Soon    int `json:"soon"`    // minutes, for todos due within 24 hours
	Normal  int `json:"normal"`  // minutes, for todos due within 7 days
}

// DefaultReminderIntervals returns the default reminder intervals
func DefaultReminderIntervals() ReminderIntervals {
	return ReminderIntervals{
		Overdue: 30,  // 30 minutes
		Urgent:  30,  // 30 minutes
		Soon:    120, // 2 hours
		Normal:  480, // 8 hours
	}
}

// UserSettings represents user-specific settings for todo reminders
type UserSettings struct {
	UserID               int64             `json:"user_id"`
	MaxDailyReminders    int               `json:"max_daily_reminders"`
	QuietStart           string            `json:"quiet_start"` // HH:MM format
	QuietEnd             string            `json:"quiet_end"`   // HH:MM format
	Timezone             string            `json:"timezone"`
	ReminderIntervals    ReminderIntervals `json:"reminder_intervals"`
	TodoRemindersEnabled bool              `json:"todo_reminders_enabled"`
	LastTodoMessageID    *int              `json:"last_todo_message_id"`
	DailySummaryEnabled  bool              `json:"daily_summary_enabled"`
	DailySummaryTime     string            `json:"daily_summary_time"` // HH:MM format
	LastDailySummaryDate *time.Time        `json:"last_daily_summary_date"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// NewDefaultUserSettings creates a new UserSettings with default values
func NewDefaultUserSettings(userID int64) *UserSettings {
	return &UserSettings{
		UserID:               userID,
		MaxDailyReminders:    10,
		QuietStart:           "22:00",
		QuietEnd:             "08:00",
		Timezone:             "Asia/Taipei",
		ReminderIntervals:    DefaultReminderIntervals(),
		TodoRemindersEnabled: true,
		LastTodoMessageID:    nil,
		DailySummaryEnabled:  true,
		DailySummaryTime:     "08:00",
		LastDailySummaryDate: nil,
		UpdatedAt:            time.Now(),
	}
}

// ShouldSendDailySummary checks if it's time to send the daily summary
func (s *UserSettings) ShouldSendDailySummary(now time.Time) bool {
	if !s.DailySummaryEnabled {
		return false
	}

	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		loc = time.Local
	}

	localNow := now.In(loc)
	today := localNow.Truncate(24 * time.Hour)

	// Check if already sent today
	if s.LastDailySummaryDate != nil {
		lastDate := s.LastDailySummaryDate.In(loc).Truncate(24 * time.Hour)
		if !lastDate.Before(today) {
			return false
		}
	}

	// Check if current time is past the summary time
	summaryHour, summaryMin := parseTimeString(s.DailySummaryTime)
	summaryTime := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), summaryHour, summaryMin, 0, 0, loc)

	return localNow.After(summaryTime) || localNow.Equal(summaryTime)
}

// IsQuietHours checks if the given time is within quiet hours
func (s *UserSettings) IsQuietHours(t time.Time) bool {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		loc = time.Local
	}

	localTime := t.In(loc)
	currentMinutes := localTime.Hour()*60 + localTime.Minute()

	startHour, startMin := parseTimeString(s.QuietStart)
	endHour, endMin := parseTimeString(s.QuietEnd)

	startMinutes := startHour*60 + startMin
	endMinutes := endHour*60 + endMin

	// Handle overnight quiet hours (e.g., 22:00 - 08:00)
	if startMinutes > endMinutes {
		// Quiet hours span midnight
		return currentMinutes >= startMinutes || currentMinutes < endMinutes
	}

	// Same day quiet hours (e.g., 13:00 - 15:00)
	return currentMinutes >= startMinutes && currentMinutes < endMinutes
}

// parseTimeString parses "HH:MM" format to hours and minutes
func parseTimeString(timeStr string) (hour, min int) {
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return 0, 0
	}
	return t.Hour(), t.Minute()
}

// GetIntervalMinutes returns the reminder interval in minutes for a given time zone
func (s *UserSettings) GetIntervalMinutes(zone string) int {
	switch zone {
	case "overdue":
		return s.ReminderIntervals.Overdue
	case "urgent":
		return s.ReminderIntervals.Urgent
	case "soon":
		return s.ReminderIntervals.Soon
	case "normal":
		return s.ReminderIntervals.Normal
	default:
		return 0
	}
}

// MarshalIntervalsJSON converts ReminderIntervals to JSON for database storage
func (r ReminderIntervals) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int{
		"overdue": r.Overdue,
		"urgent":  r.Urgent,
		"soon":    r.Soon,
		"normal":  r.Normal,
	})
}

// UnmarshalIntervalsJSON parses JSON from database into ReminderIntervals
func (r *ReminderIntervals) UnmarshalJSON(data []byte) error {
	var m map[string]int
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	r.Overdue = m["overdue"]
	r.Urgent = m["urgent"]
	r.Soon = m["soon"]
	r.Normal = m["normal"]
	return nil
}

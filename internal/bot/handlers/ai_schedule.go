package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
)

func (h *Handlers) handleQueryScheduleResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	dateStr := params["date"]
	startDateStr := params["start_date"]
	endDateStr := params["end_date"]

	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Determine date range
	var startTime, endTime time.Time
	if dateStr != "" {
		// Specific date
		if parsed := parseDateTime(dateStr); parsed != nil {
			startTime = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
			endTime = startTime.Add(24 * time.Hour)
		} else {
			startTime = today
			endTime = startTime.Add(24 * time.Hour)
		}
	} else if startDateStr != "" || endDateStr != "" {
		// Date range
		if startDateStr != "" {
			if parsed := parseDateTime(startDateStr); parsed != nil {
				startTime = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
			} else {
				startTime = today
			}
		} else {
			startTime = today
		}
		if endDateStr != "" {
			if parsed := parseDateTime(endDateStr); parsed != nil {
				endTime = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, loc)
			} else {
				endTime = startTime.Add(24 * time.Hour)
			}
		} else {
			endTime = startTime.Add(24 * time.Hour)
		}
	} else {
		// Default: today
		startTime = today
		endTime = startTime.Add(24 * time.Hour)
	}

	// Ensure start_date is not before today (don't show past events)
	if startTime.Before(today) {
		startTime = today
	}

	// Check if query spans multiple days
	isMultiDay := endTime.Sub(startTime) > 24*time.Hour

	// Helper function to format time with optional date
	formatEventTime := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		if isMultiDay {
			return t.Format("01/02 15:04")
		}
		return t.Format("15:04")
	}

	// Collect data from multiple sources
	var sb strings.Builder

	// 1. Events in date range
	events, err := h.repos.Event.GetByDateRange(ctx, msg.From.ID, startTime, endTime)
	if err == nil && len(events) > 0 {
		sb.WriteString("【事件】\n")
		for _, e := range events {
			var eventTime *time.Time
			if e.NextOccurrence != nil {
				eventTime = e.NextOccurrence
			} else if e.Dtstart != nil {
				eventTime = e.Dtstart
			}
			timeStr := formatEventTime(eventTime)
			sb.WriteString(fmt.Sprintf("• [#%d] %s", e.EventID, e.Title))
			if timeStr != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", timeStr))
			}
			if e.Duration > 0 && e.Duration != 60 {
				sb.WriteString(fmt.Sprintf(" [%d分鐘]", e.Duration))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// 2. Todos with due date in range
	todos, err := h.repos.Todo.GetByUserID(ctx, msg.From.ID, false)
	if err == nil {
		var relevantTodos []*models.Todo
		for _, t := range todos {
			if t.DueTime != nil && !t.DueTime.Before(startTime) && t.DueTime.Before(endTime) {
				relevantTodos = append(relevantTodos, t)
			}
		}
		if len(relevantTodos) > 0 {
			sb.WriteString("【待辦事項】\n")
			for _, t := range relevantTodos {
				timeStr := formatEventTime(t.DueTime)
				sb.WriteString(fmt.Sprintf("• [#%d] %s", t.TodoID, t.Title))
				if timeStr != "" {
					sb.WriteString(fmt.Sprintf(" (截止: %s)", timeStr))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	// 3. Reminders in date range
	reminders, err := h.repos.Reminder.GetByUserID(ctx, msg.From.ID)
	if err == nil {
		var relevantReminders []*models.Reminder
		for _, r := range reminders {
			if r.Enabled && r.RemindAt != nil && !r.RemindAt.Before(startTime) && r.RemindAt.Before(endTime) {
				relevantReminders = append(relevantReminders, r)
			}
		}
		if len(relevantReminders) > 0 {
			sb.WriteString("【提醒】\n")
			for _, r := range relevantReminders {
				timeStr := formatEventTime(r.RemindAt)
				sb.WriteString(fmt.Sprintf("• [#%d] %s (%s)\n", r.ReminderID, r.Messages, timeStr))
			}
			sb.WriteString("\n")
		}
	}

	// Build result
	scheduleData := sb.String()
	if scheduleData == "" {
		scheduleData = "這段時間沒有安排任何事項。"
	}

	// Format date range for display
	dateRangeStr := ""
	if startTime.Format("2006-01-02") == endTime.Add(-time.Second).Format("2006-01-02") {
		dateRangeStr = startTime.Format("2006-01-02")
	} else {
		dateRangeStr = fmt.Sprintf("%s ~ %s", startTime.Format("2006-01-02"), endTime.Add(-time.Second).Format("2006-01-02"))
	}

	// Use AI to generate a friendly response
	if h.ai != nil {
		response, err := h.ai.FormatQueryResult(ctx, "行程查詢", dateRangeStr, scheduleData)
		if err == nil && response != "" {
			if sendMsg {
				h.sendMessage(msg.Chat.ID, response)
			}
			return response
		}
	}

	// Fallback: return raw data
	result := fmt.Sprintf("%s %s 的行程：\n\n%s", getDateEmoji(startTime), dateRangeStr, scheduleData)
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

// getDateEmoji returns a calendar emoji for the day of month (1-31)
func getDateEmoji(t time.Time) string {
	// Try to use keycap number emojis for the day
	day := t.Day()
	numberEmojis := []string{"0️⃣", "1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣"}

	if day < 10 {
		return numberEmojis[day]
	}
	// For 10-31, combine two digits
	tens := day / 10
	ones := day % 10
	return numberEmojis[tens] + numberEmojis[ones]
}

func parseDateTime(s string) *time.Time {
	now := time.Now()
	loc := now.Location()

	// Try various formats
	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02",
		"01-02 15:04",
		"15:04",
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, s, loc); err == nil {
			// Adjust year/month/day if not specified
			if format == "15:04" {
				t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
				if t.Before(now) {
					t = t.Add(24 * time.Hour)
				}
			} else if format == "01-02 15:04" {
				t = time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
			}
			return &t
		}
	}

	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

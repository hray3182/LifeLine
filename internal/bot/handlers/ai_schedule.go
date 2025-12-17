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
		var upcomingTodos []*models.Todo // 即將到期但不在查詢範圍內的 todo

		for _, t := range todos {
			if t.DueTime != nil {
				if !t.DueTime.Before(startTime) && t.DueTime.Before(endTime) {
					// 截止時間在查詢範圍內
					relevantTodos = append(relevantTodos, t)
				} else if !t.DueTime.Before(endTime) && t.DueTime.Before(endTime.Add(48*time.Hour)) {
					// 截止時間在查詢範圍結束後 48 小時內（即將到期）
					// 只顯示優先級 >= 4 的重要事項，過濾掉日常提醒類的低優先級 todo
					if t.Priority >= 4 {
						upcomingTodos = append(upcomingTodos, t)
					}
				}
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

		// 顯示即將到期的待辦事項
		if len(upcomingTodos) > 0 {
			sb.WriteString("【即將到期的待辦】\n")
			for _, t := range upcomingTodos {
				timeLeftStr := formatTimeLeft(t.DueTime, now)
				sb.WriteString(fmt.Sprintf("• [#%d] %s (%s)\n", t.TodoID, t.Title, timeLeftStr))
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

// formatTimeLeft formats the remaining time until deadline in a human-readable way
func formatTimeLeft(dueTime *time.Time, now time.Time) string {
	if dueTime == nil {
		return "無截止時間"
	}

	duration := dueTime.Sub(now)
	if duration < 0 {
		return "已過期"
	}

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60

	if hours >= 24 {
		days := hours / 24
		remainingHours := hours % 24
		if remainingHours > 0 {
			return fmt.Sprintf("剩餘 %d 天 %d 小時", days, remainingHours)
		}
		return fmt.Sprintf("剩餘 %d 天", days)
	} else if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("剩餘 %d 小時 %d 分鐘", hours, minutes)
		}
		return fmt.Sprintf("剩餘 %d 小時", hours)
	} else if minutes > 0 {
		return fmt.Sprintf("剩餘 %d 分鐘", minutes)
	}
	return "即將到期"
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

// handleFindFreeTime finds free time slots on a given date
func (h *Handlers) handleFindFreeTime(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	dateStr := params["date"]

	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Parse target date
	var targetDate time.Time
	if dateStr != "" {
		if parsed := parseDateTime(dateStr); parsed != nil {
			targetDate = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
		} else {
			targetDate = today
		}
	} else {
		targetDate = today
	}

	// Define working hours (08:00 - 22:00)
	dayStart := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 8, 0, 0, 0, loc)
	dayEnd := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 22, 0, 0, 0, loc)

	// If target date is today and current time is past 8:00, start from now (rounded to next 30 min)
	if targetDate.Equal(today) && now.After(dayStart) {
		// Round up to next 30 minutes
		minutes := now.Minute()
		if minutes > 30 {
			dayStart = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, loc)
		} else if minutes > 0 {
			dayStart = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 30, 0, 0, loc)
		} else {
			dayStart = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, loc)
		}
	}

	// Collect busy time slots
	type timeSlot struct {
		start time.Time
		end   time.Time
		title string
	}
	var busySlots []timeSlot

	// Get events for the day
	events, err := h.repos.Event.GetByDateRange(ctx, msg.From.ID, targetDate, targetDate.Add(24*time.Hour))
	if err == nil {
		for _, e := range events {
			var eventStart *time.Time
			if e.NextOccurrence != nil {
				eventStart = e.NextOccurrence
			} else if e.Dtstart != nil {
				eventStart = e.Dtstart
			}
			if eventStart != nil {
				duration := e.Duration
				if duration == 0 {
					duration = 60 // default 60 minutes
				}
				busySlots = append(busySlots, timeSlot{
					start: *eventStart,
					end:   eventStart.Add(time.Duration(duration) * time.Minute),
					title: e.Title,
				})
			}
		}
	}

	// Sort busy slots by start time
	for i := 0; i < len(busySlots)-1; i++ {
		for j := i + 1; j < len(busySlots); j++ {
			if busySlots[j].start.Before(busySlots[i].start) {
				busySlots[i], busySlots[j] = busySlots[j], busySlots[i]
			}
		}
	}

	// Find free slots
	var freeSlots []timeSlot
	currentTime := dayStart

	for _, busy := range busySlots {
		// Skip if busy slot is outside our time range
		if busy.end.Before(dayStart) || busy.start.After(dayEnd) {
			continue
		}

		// Adjust busy slot to our time range
		busyStart := busy.start
		if busyStart.Before(dayStart) {
			busyStart = dayStart
		}

		// If there's a gap before this busy slot, it's free time
		if currentTime.Before(busyStart) {
			freeSlots = append(freeSlots, timeSlot{
				start: currentTime,
				end:   busyStart,
			})
		}

		// Move current time to end of busy slot
		if busy.end.After(currentTime) {
			currentTime = busy.end
		}
	}

	// Add remaining time until end of day
	if currentTime.Before(dayEnd) {
		freeSlots = append(freeSlots, timeSlot{
			start: currentTime,
			end:   dayEnd,
		})
	}

	// Build result
	var sb strings.Builder
	dateLabel := targetDate.Format("2006-01-02")
	if targetDate.Equal(today) {
		dateLabel = "今天 (" + targetDate.Format("01/02") + ")"
	} else if targetDate.Equal(today.Add(24*time.Hour)) {
		dateLabel = "明天 (" + targetDate.Format("01/02") + ")"
	}

	sb.WriteString(fmt.Sprintf("【%s 的空閒時段】\n\n", dateLabel))

	if len(freeSlots) == 0 {
		sb.WriteString("這天沒有空閒時間。\n")
	} else {
		for _, slot := range freeSlots {
			duration := slot.end.Sub(slot.start)
			hours := int(duration.Hours())
			minutes := int(duration.Minutes()) % 60

			durationStr := ""
			if hours > 0 && minutes > 0 {
				durationStr = fmt.Sprintf("%d小時%d分鐘", hours, minutes)
			} else if hours > 0 {
				durationStr = fmt.Sprintf("%d小時", hours)
			} else {
				durationStr = fmt.Sprintf("%d分鐘", minutes)
			}

			sb.WriteString(fmt.Sprintf("• %s - %s (%s)\n",
				slot.start.Format("15:04"),
				slot.end.Format("15:04"),
				durationStr))
		}
	}

	if len(busySlots) > 0 {
		sb.WriteString("\n【已安排的事項】\n")
		for _, busy := range busySlots {
			sb.WriteString(fmt.Sprintf("• %s - %s: %s\n",
				busy.start.Format("15:04"),
				busy.end.Format("15:04"),
				busy.title))
		}
	}

	return sb.String()
}

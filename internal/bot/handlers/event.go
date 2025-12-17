package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
	"github.com/hray3182/LifeLine/internal/rrule"
)

func (h *Handlers) handleEvent(ctx context.Context, msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		h.sendMessage(msg.Chat.ID, "請提供事件標題\n用法: /event <標題> [時間]\n例如: /event 開會 15:30")
		return
	}

	// Parse: title and optional time
	parts := strings.Fields(args)
	title := parts[0]
	var dtstart *time.Time

	if len(parts) > 1 {
		// Try to parse the last part as time
		lastPart := parts[len(parts)-1]
		if t, err := parseTimeToday(lastPart); err == nil {
			dtstart = &t
			title = strings.Join(parts[:len(parts)-1], " ")
		} else {
			title = args
		}
	}

	event := &models.Event{
		UserID:              msg.From.ID,
		Title:               title,
		Dtstart:             dtstart,
		NextOccurrence:      dtstart,
		Duration:            60, // Default 60 minutes
		NotificationMinutes: 30,
	}

	if err := h.repos.Event.Create(ctx, event); err != nil {
		h.sendMessage(msg.Chat.ID, "建立事件失敗，請稍後再試")
		return
	}

	h.notifyScheduler()
	timeStr := "未設定"
	if dtstart != nil {
		timeStr = dtstart.Format("2006-01-02 15:04")
	}

	h.sendMessage(msg.Chat.ID, fmt.Sprintf("📅 事件已建立\n標題: %s\n時間: %s", title, timeStr))
}

func (h *Handlers) handleEventList(ctx context.Context, msg *tgbotapi.Message) {
	// Get all events for the user
	events, err := h.repos.Event.GetByUserID(ctx, msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "取得事件列表失敗，請稍後再試")
		return
	}

	if len(events) == 0 {
		h.sendMessage(msg.Chat.ID, "📅 目前沒有事件")
		return
	}

	var sb strings.Builder
	sb.WriteString("📅 **近期事件**\n")

	// 按日期分組
	currentDate := ""
	now := time.Now()
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")

	for _, event := range events {
		var eventDate string
		var eventTime *time.Time

		if event.NextOccurrence != nil {
			eventTime = event.NextOccurrence
		} else if event.Dtstart != nil {
			eventTime = event.Dtstart
		}

		if eventTime != nil {
			eventDate = eventTime.Format("2006-01-02")
		} else {
			eventDate = "未設定"
		}

		// 如果日期改變，顯示日期標題
		if eventDate != currentDate {
			currentDate = eventDate
			var dateLabel string
			if eventDate == today {
				dateLabel = "今天"
			} else if eventDate == tomorrow {
				dateLabel = "明天"
			} else if eventDate == "未設定" {
				dateLabel = "未設定時間"
			} else if eventTime != nil {
				dateLabel = eventTime.Format("01/02 (Mon)")
			}
			sb.WriteString(fmt.Sprintf("\n━━━ **%s** ━━━\n", dateLabel))
		}

		// 顯示事件
		timeStr := ""
		if eventTime != nil {
			timeStr = eventTime.Format("15:04")
		}

		if timeStr != "" {
			sb.WriteString(fmt.Sprintf("🕐 %s  %s\n", timeStr, event.Title))
		} else {
			sb.WriteString(fmt.Sprintf("• %s\n", event.Title))
		}

		if event.IsRecurring() {
			sb.WriteString(fmt.Sprintf("   🔄 %s\n", rrule.HumanReadableChinese(event.RecurrenceRule)))
		}
		if event.Description != "" {
			desc := event.Description
			if len(desc) > 30 {
				desc = desc[:30] + "..."
			}
			sb.WriteString(fmt.Sprintf("   📝 %s\n", desc))
		}
	}

	h.sendMessage(msg.Chat.ID, sb.String())
}

func (h *Handlers) CreateEvent(ctx context.Context, userID int64, title, description string, dtstart *time.Time, duration int, notificationMinutes int, recurrenceRule string, tags string) (*models.Event, error) {
	if notificationMinutes == 0 {
		notificationMinutes = 30
	}
	if duration == 0 {
		duration = 60 // Default 60 minutes
	}

	event := &models.Event{
		UserID:              userID,
		Title:               title,
		Description:         description,
		Dtstart:             dtstart,
		Duration:            duration,
		NotificationMinutes: notificationMinutes,
		RecurrenceRule:      recurrenceRule,
		Tags:                tags,
	}

	// Calculate NextOccurrence
	if dtstart != nil {
		now := time.Now()
		if recurrenceRule != "" {
			// For recurring events, calculate the next occurrence
			if dtstart.After(now) {
				event.NextOccurrence = dtstart
			} else {
				// dtstart is in the past, find next occurrence
				next, err := rrule.NextOccurrence(recurrenceRule, *dtstart, now)
				if err != nil {
					// Fallback to dtstart if RRULE parsing fails
					event.NextOccurrence = dtstart
				} else {
					event.NextOccurrence = next
				}
			}
		} else {
			// One-time event
			event.NextOccurrence = dtstart
		}
	}

	err := h.repos.Event.Create(ctx, event)
	if err == nil {
		h.notifyScheduler()
	}
	return event, err
}

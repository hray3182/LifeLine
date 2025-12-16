package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
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
	var startTime *time.Time

	if len(parts) > 1 {
		// Try to parse the last part as time
		lastPart := parts[len(parts)-1]
		if t, err := parseTimeToday(lastPart); err == nil {
			startTime = &t
			title = strings.Join(parts[:len(parts)-1], " ")
		} else {
			title = args
		}
	}

	event := &models.Event{
		UserID:              msg.From.ID,
		Title:               title,
		StartTime:           startTime,
		NotificationMinutes: 30,
	}

	if err := h.repos.Event.Create(ctx, event); err != nil {
		h.sendMessage(msg.Chat.ID, "建立事件失敗，請稍後再試")
		return
	}

	timeStr := "未設定"
	if startTime != nil {
		timeStr = startTime.Format("2006-01-02 15:04")
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
	sb.WriteString("📅 *近期事件*\n\n")
	for _, event := range events {
		timeStr := "未設定時間"
		if event.StartTime != nil {
			timeStr = event.StartTime.Format("01/02 15:04")
		}

		sb.WriteString(fmt.Sprintf("*%d.* %s\n", event.EventID, event.Title))
		sb.WriteString(fmt.Sprintf("   🕐 %s\n", timeStr))
		if event.Description != "" {
			desc := event.Description
			if len(desc) > 30 {
				desc = desc[:30] + "..."
			}
			sb.WriteString(fmt.Sprintf("   📝 %s\n", desc))
		}
		sb.WriteString("\n")
	}

	h.sendMessage(msg.Chat.ID, sb.String())
}

func (h *Handlers) CreateEvent(ctx context.Context, userID int64, title, description string, startTime, endTime *time.Time, notificationMinutes int, tags string) (*models.Event, error) {
	if notificationMinutes == 0 {
		notificationMinutes = 30
	}

	event := &models.Event{
		UserID:              userID,
		Title:               title,
		Description:         description,
		StartTime:           startTime,
		EndTime:             endTime,
		NotificationMinutes: notificationMinutes,
		Tags:                tags,
	}
	err := h.repos.Event.Create(ctx, event)
	return event, err
}

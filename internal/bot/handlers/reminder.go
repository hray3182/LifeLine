package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
)

func (h *Handlers) handleReminder(ctx context.Context, msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		h.sendMessage(msg.Chat.ID, "請提供提醒時間和訊息\n用法: /remind <時間> <訊息>\n例如: /remind 15:30 開會")
		return
	}

	// Simple parsing: first word is time, rest is message
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		h.sendMessage(msg.Chat.ID, "請提供提醒時間和訊息\n例如: /remind 15:30 開會")
		return
	}

	timeStr := parts[0]
	message := parts[1]

	// Parse time (HH:MM format for today)
	remindTime, err := parseTimeToday(timeStr)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "時間格式錯誤，請使用 HH:MM 格式 (例如 15:30)")
		return
	}

	reminder := &models.Reminder{
		UserID:   msg.From.ID,
		Enabled:  true,
		Messages: message,
		RemindAt: &remindTime,
	}

	if err := h.repos.Reminder.Create(ctx, reminder); err != nil {
		h.sendMessage(msg.Chat.ID, "建立提醒失敗，請稍後再試")
		return
	}

	h.sendMessage(msg.Chat.ID, fmt.Sprintf("⏰ 提醒已設定\n時間: %s\n訊息: %s",
		remindTime.Format("2006-01-02 15:04"), message))
}

func (h *Handlers) handleReminderList(ctx context.Context, msg *tgbotapi.Message) {
	reminders, err := h.repos.Reminder.GetByUserID(ctx, msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "取得提醒列表失敗，請稍後再試")
		return
	}

	if len(reminders) == 0 {
		h.sendMessage(msg.Chat.ID, "⏰ 目前沒有提醒")
		return
	}

	var sb strings.Builder
	sb.WriteString("⏰ *提醒列表*\n\n")
	for _, r := range reminders {
		status := "✅"
		if !r.Enabled {
			status = "❌"
		}

		timeStr := "未設定"
		if r.RemindAt != nil {
			timeStr = r.RemindAt.Format("2006-01-02 15:04")
		}

		sb.WriteString(fmt.Sprintf("%s *%d.* %s\n", status, r.ReminderID, r.Messages))
		sb.WriteString(fmt.Sprintf("   📅 %s\n\n", timeStr))
	}

	h.sendMessage(msg.Chat.ID, sb.String())
}

func parseTimeToday(timeStr string) (time.Time, error) {
	now := time.Now()
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return time.Time{}, err
	}

	result := time.Date(now.Year(), now.Month(), now.Day(),
		t.Hour(), t.Minute(), 0, 0, now.Location())

	// If time already passed today, set for tomorrow
	if result.Before(now) {
		result = result.Add(24 * time.Hour)
	}

	return result, nil
}

func (h *Handlers) CreateReminder(ctx context.Context, userID int64, message string, remindAt *time.Time, recurrenceRule string) (*models.Reminder, error) {
	reminder := &models.Reminder{
		UserID:         userID,
		Enabled:        true,
		Messages:       message,
		RemindAt:       remindAt,
		RecurrenceRule: recurrenceRule,
	}
	err := h.repos.Reminder.Create(ctx, reminder)
	return reminder, err
}

package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
	"github.com/hray3182/LifeLine/internal/rrule"
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

	h.notifyScheduler()
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
	sb.WriteString("⏰ **提醒列表**\n\n")
	for _, r := range reminders {
		status := "✅"
		if !r.Enabled {
			status = "❌"
		}

		timeStr := "未設定"
		if r.RemindAt != nil {
			timeStr = r.RemindAt.Format("2006-01-02 15:04")
		}

		sb.WriteString(fmt.Sprintf("%s **%d.** %s\n", status, r.ReminderID, r.Messages))
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

func (h *Handlers) handleReminderAcknowledge(ctx context.Context, callback *tgbotapi.CallbackQuery, reminderIDStr string) {
	reminderID, err := strconv.Atoi(reminderIDStr)
	if err != nil {
		h.debug("handleReminderAcknowledge: invalid reminder ID", "error", err)
		return
	}

	// Get reminder
	reminder, err := h.repos.Reminder.GetByIDOnly(ctx, reminderID)
	if err != nil {
		h.debug("handleReminderAcknowledge: reminder not found", "error", err)
		h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "⚠️ 找不到此提醒")
		return
	}

	// Verify the callback is from the correct user
	if callback.From.ID != reminder.UserID {
		h.answerCallbackWithAlert(callback.ID, "這不是你的提醒")
		return
	}

	now := time.Now()

	// Mark as acknowledged
	if err := h.repos.Reminder.SetAcknowledgedAt(ctx, reminderID, &now); err != nil {
		h.debug("handleReminderAcknowledge: failed to set acknowledged_at", "error", err)
		return
	}
	h.debug("handleReminderAcknowledge: acknowledged", "reminder_id", reminderID)

	// Handle recurrence: calculate next occurrence
	if reminder.IsRecurring() && reminder.Dtstart != nil {
		// Use strict version to get the next occurrence after now
		next, err := rrule.NextOccurrenceStrict(reminder.RecurrenceRule, *reminder.Dtstart, now)
		h.debug("handleReminderAcknowledge: recurring", "next", next, "err", err)
		if err != nil || next == nil {
			// No more occurrences, disable it
			h.repos.Reminder.SetEnabled(ctx, reminderID, reminder.UserID, false)
			h.debug("handleReminderAcknowledge: disabled (no more occurrences)")
		} else {
			// Update remind_at to next occurrence (this clears acknowledged_at and notified_at)
			h.repos.Reminder.UpdateRemindAt(ctx, reminderID, next)
			h.debug("handleReminderAcknowledge: scheduled next", "next", next.Format("2006-01-02 15:04"))
		}
	} else {
		// One-time reminder, disable it
		h.repos.Reminder.SetEnabled(ctx, reminderID, reminder.UserID, false)
		h.debug("handleReminderAcknowledge: disabled (one-time)")
	}

	// Update message to show acknowledged
	h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID,
		fmt.Sprintf("✅ 已確認提醒\n\n%s", reminder.Messages))
}

func (h *Handlers) CreateReminder(ctx context.Context, userID int64, message string, dtstart *time.Time, recurrenceRule string) (*models.Reminder, error) {
	reminder := &models.Reminder{
		UserID:         userID,
		Enabled:        true,
		Messages:       message,
		Dtstart:        dtstart,
		RecurrenceRule: recurrenceRule,
	}

	// Calculate first remind_at time
	if dtstart != nil {
		if recurrenceRule != "" {
			// For recurring reminders, calculate the first occurrence that is in the future
			now := time.Now()
			if dtstart.After(now) {
				reminder.RemindAt = dtstart
			} else {
				// dtstart is in the past, find next occurrence
				next, err := rrule.NextOccurrence(recurrenceRule, *dtstart, now)
				if err != nil {
					// Fallback to dtstart if RRULE parsing fails
					reminder.RemindAt = dtstart
				} else if next != nil {
					reminder.RemindAt = next
				} else {
					// No more occurrences
					reminder.RemindAt = nil
					reminder.Enabled = false
				}
			}
		} else {
			// One-time reminder
			reminder.RemindAt = dtstart
		}
	}

	err := h.repos.Reminder.Create(ctx, reminder)
	if err == nil {
		h.notifyScheduler()
	}
	return reminder, err
}

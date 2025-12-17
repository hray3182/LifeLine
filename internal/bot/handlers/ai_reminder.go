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

func (h *Handlers) handleAIListReminder(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIListReminderResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIListReminderResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	keyword := params["keyword"]
	var reminders []*models.Reminder
	var err error

	if keyword != "" {
		reminders, err = h.repos.Reminder.Search(ctx, msg.From.ID, keyword)
	} else {
		reminders, err = h.repos.Reminder.GetByUserID(ctx, msg.From.ID)
	}

	if err != nil {
		result := "取得提醒列表失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if len(reminders) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("找不到包含「%s」的提醒", keyword)
		} else {
			result = "目前沒有提醒"
		}
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("提醒搜尋結果 (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("提醒列表\n\n")
	}
	for _, r := range reminders {
		status := "啟用"
		if !r.Enabled {
			status = "停用"
		}

		timeStr := "未設定"
		if r.RemindAt != nil {
			timeStr = r.RemindAt.Format("2006-01-02 15:04")
		}

		sb.WriteString(fmt.Sprintf("[%s] %d. %s\n", status, r.ReminderID, r.Messages))
		sb.WriteString(fmt.Sprintf("   時間: %s\n\n", timeStr))
	}

	result := sb.String()
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAICreateReminder(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAICreateReminderResult(ctx, msg, params, true)
}

func (h *Handlers) handleAICreateReminderResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	message := params["message"]
	if message == "" {
		message = params["content"]
	}
	if message == "" {
		result := "請提供提醒訊息"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	// Parse dtstart (first occurrence time)
	var dtstart *time.Time
	if dt, ok := params["dtstart"]; ok && dt != "" {
		dtstart = parseDateTime(dt)
	}
	// Fallback to remind_at or time for backward compatibility
	if dtstart == nil {
		if dt, ok := params["remind_at"]; ok && dt != "" {
			dtstart = parseDateTime(dt)
		}
	}
	if dtstart == nil {
		if dt, ok := params["time"]; ok && dt != "" {
			dtstart = parseDateTime(dt)
		}
	}

	// Get RRULE
	rruleStr := params["rrule"]

	reminder, err := h.CreateReminder(ctx, msg.From.ID, message, dtstart, rruleStr)
	if err != nil {
		result := "建立提醒失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("提醒已設定 (ID: %d)\n訊息: %s", reminder.ReminderID, message)
	if dtstart != nil {
		result += fmt.Sprintf("\n首次提醒: %s", dtstart.Format("2006-01-02 15:04"))
	}
	if rruleStr != "" {
		result += fmt.Sprintf("\n重複: %s", rrule.HumanReadableChinese(rruleStr))
	}
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAIDeleteReminder(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIDeleteReminderResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIDeleteReminderResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的提醒編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if err := h.repos.Reminder.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除提醒失敗，請確認編號是否正確"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("提醒 #%d 已刪除", id)
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

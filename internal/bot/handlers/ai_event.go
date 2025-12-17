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

func (h *Handlers) handleAIListEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIListEventResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIListEventResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	keyword := params["keyword"]
	dateStr := params["date"]         // specific date: YYYY-MM-DD
	startDate := params["start_date"] // range start
	endDate := params["end_date"]     // range end

	var events []*models.Event
	var err error

	if dateStr != "" {
		// Search by specific date
		date := parseDateTime(dateStr)
		if date != nil {
			// Get start and end of day
			startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
			endOfDay := startOfDay.Add(24 * time.Hour)
			events, err = h.repos.Event.GetByDateRange(ctx, msg.From.ID, startOfDay, endOfDay)
		} else {
			events, err = h.repos.Event.GetByUserID(ctx, msg.From.ID)
		}
	} else if startDate != "" || endDate != "" {
		// Search by date range
		now := time.Now()
		start := now
		end := now.AddDate(1, 0, 0) // default: 1 year ahead

		if startDate != "" {
			if parsed := parseDateTime(startDate); parsed != nil {
				start = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, parsed.Location())
			}
		}
		if endDate != "" {
			if parsed := parseDateTime(endDate); parsed != nil {
				end = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, parsed.Location())
			}
		}
		events, err = h.repos.Event.GetByDateRange(ctx, msg.From.ID, start, end)
	} else if keyword != "" {
		events, err = h.repos.Event.Search(ctx, msg.From.ID, keyword)
	} else {
		events, err = h.repos.Event.GetByUserID(ctx, msg.From.ID)
	}

	if err != nil {
		result := "取得事件列表失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if len(events) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("找不到包含「%s」的事件", keyword)
		} else {
			result = "目前沒有事件"
		}
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("事件搜尋結果 (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("事件列表\n\n")
	}
	for _, event := range events {
		timeStr := "未設定時間"
		if event.NextOccurrence != nil {
			timeStr = event.NextOccurrence.Format("01/02 15:04")
		} else if event.Dtstart != nil {
			timeStr = event.Dtstart.Format("01/02 15:04")
		}

		sb.WriteString(fmt.Sprintf("%d. %s\n", event.EventID, event.Title))
		sb.WriteString(fmt.Sprintf("   時間: %s\n", timeStr))
		if event.Duration > 0 {
			sb.WriteString(fmt.Sprintf("   時長: %d 分鐘\n", event.Duration))
		}
		if event.IsRecurring() {
			sb.WriteString(fmt.Sprintf("   重複: %s\n", rrule.HumanReadableChinese(event.RecurrenceRule)))
		}
		if event.Description != "" {
			desc := event.Description
			if len(desc) > 30 {
				desc = desc[:30] + "..."
			}
			sb.WriteString(fmt.Sprintf("   描述: %s\n", desc))
		}
		sb.WriteString("\n")
	}

	result := sb.String()
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAICreateEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAICreateEventResult(ctx, msg, params, true)
}

func (h *Handlers) handleAICreateEventResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	title := params["title"]
	if title == "" {
		result := "請提供事件標題"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	description := params["description"]
	tags := params["tags"]

	// Parse dtstart (first occurrence time)
	var dtstart *time.Time
	if dt, ok := params["dtstart"]; ok && dt != "" {
		dtstart = parseDateTime(dt)
	}
	// Fallback to start_time for backward compatibility
	if dtstart == nil {
		if dt, ok := params["start_time"]; ok && dt != "" {
			dtstart = parseDateTime(dt)
		}
	}

	// Parse duration (minutes)
	duration := 60 // Default
	if d, ok := params["duration"]; ok && d != "" {
		if parsed, err := strconv.Atoi(d); err == nil {
			duration = parsed
		}
	}

	// Get RRULE
	rruleStr := params["rrule"]

	event, err := h.CreateEvent(ctx, msg.From.ID, title, description, dtstart, duration, 30, rruleStr, tags)
	if err != nil {
		result := "建立事件失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("事件已建立 (ID: %d)\n標題: %s", event.EventID, title)
	if dtstart != nil {
		result += fmt.Sprintf("\n首次時間: %s", dtstart.Format("2006-01-02 15:04"))
	}
	if duration > 0 {
		result += fmt.Sprintf("\n時長: %d 分鐘", duration)
	}
	if rruleStr != "" {
		result += fmt.Sprintf("\n重複: %s", rrule.HumanReadableChinese(rruleStr))
	}
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAIDeleteEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIDeleteEventResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIDeleteEventResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的事件編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if err := h.repos.Event.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除事件失敗，請確認編號是否正確"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("事件 #%d 已刪除", id)
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAIUpdateEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIUpdateEventResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIUpdateEventResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的事件編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	event, err := h.repos.Event.GetByID(ctx, id, msg.From.ID)
	if err != nil {
		result := "找不到該事件"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	// Update fields if provided
	if title, ok := params["title"]; ok && title != "" {
		event.Title = title
	}
	if desc, ok := params["description"]; ok {
		event.Description = desc
	}
	if dt, ok := params["dtstart"]; ok && dt != "" {
		event.Dtstart = parseDateTime(dt)
	}
	// Fallback to start_time for backward compatibility
	if dt, ok := params["start_time"]; ok && dt != "" && event.Dtstart == nil {
		event.Dtstart = parseDateTime(dt)
	}
	if d, ok := params["duration"]; ok && d != "" {
		if parsed, err := strconv.Atoi(d); err == nil {
			event.Duration = parsed
		}
	}
	if rruleStr, ok := params["rrule"]; ok {
		event.RecurrenceRule = rruleStr
	}
	if tags, ok := params["tags"]; ok {
		event.Tags = tags
	}

	// Recalculate NextOccurrence if dtstart or rrule changed
	if event.Dtstart != nil {
		now := time.Now()
		if event.RecurrenceRule != "" {
			if event.Dtstart.After(now) {
				event.NextOccurrence = event.Dtstart
			} else {
				next, err := rrule.NextOccurrence(event.RecurrenceRule, *event.Dtstart, now)
				if err != nil {
					event.NextOccurrence = event.Dtstart
				} else {
					event.NextOccurrence = next
				}
			}
		} else {
			event.NextOccurrence = event.Dtstart
		}
	}

	if err := h.repos.Event.Update(ctx, event); err != nil {
		result := "更新事件失敗"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("事件 #%d 已更新", id)
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

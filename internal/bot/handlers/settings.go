package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/format"
)

// handleSettings shows the settings menu
func (h *Handlers) handleSettings(ctx context.Context, msg *tgbotapi.Message) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, msg.From.ID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		h.sendMessage(msg.Chat.ID, "無法取得設定，請稍後再試")
		return
	}

	text := h.buildSettingsMainText(settings.TodoRemindersEnabled, settings.DailySummaryEnabled, settings.DailySummaryTime)
	keyboard := h.buildSettingsMainKeyboard()

	parsed := format.ParseMarkdown(text)
	reply := tgbotapi.NewMessage(msg.Chat.ID, parsed.Text)
	reply.Entities = parsed.Entities
	reply.ReplyMarkup = keyboard

	if _, err := h.api.Send(reply); err != nil {
		log.Printf("Failed to send settings menu: %v", err)
	}
}

// handleSettingsCallback handles settings-related callbacks
func (h *Handlers) handleSettingsCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string) {
	if len(parts) == 0 {
		return
	}

	userID := callback.From.ID
	chatID := callback.Message.Chat.ID
	messageID := callback.Message.MessageID

	action := parts[0]

	switch action {
	case "main":
		h.showSettingsMain(ctx, chatID, messageID, userID)

	case "todo":
		if len(parts) > 1 && parts[1] == "toggle" {
			h.toggleTodoReminders(ctx, chatID, messageID, userID)
		} else {
			h.showTodoSettings(ctx, chatID, messageID, userID)
		}

	case "summary":
		if len(parts) > 1 {
			switch parts[1] {
			case "toggle":
				h.toggleDailySummary(ctx, chatID, messageID, userID)
			case "time":
				if len(parts) > 2 {
					h.setDailySummaryTime(ctx, chatID, messageID, userID, parts[2])
				} else {
					h.showSummaryTimePicker(ctx, chatID, messageID)
				}
			}
		} else {
			h.showSummarySettings(ctx, chatID, messageID, userID)
		}

	case "quiet":
		if len(parts) > 1 {
			switch parts[1] {
			case "menu":
				h.showQuietSettings(ctx, chatID, messageID, userID)
			case "start":
				if len(parts) > 2 {
					h.setQuietStart(ctx, chatID, messageID, userID, parts[2])
				} else {
					h.showQuietStartPicker(ctx, chatID, messageID)
				}
			case "end":
				if len(parts) > 2 {
					h.setQuietEnd(ctx, chatID, messageID, userID, parts[2])
				} else {
					h.showQuietEndPicker(ctx, chatID, messageID)
				}
			case "disable":
				h.disableQuietHours(ctx, chatID, messageID, userID)
			}
		}

	case "limit":
		if len(parts) > 1 {
			h.setDailyLimit(ctx, chatID, messageID, userID, parts[1])
		} else {
			h.showLimitSettings(ctx, chatID, messageID, userID)
		}

	case "interval":
		if len(parts) > 1 {
			switch parts[1] {
			case "menu":
				h.showIntervalSettings(ctx, chatID, messageID, userID)
			case "reset":
				h.resetIntervals(ctx, chatID, messageID, userID)
			default:
				// Format: interval:zone:minutes
				if len(parts) > 2 {
					h.setInterval(ctx, chatID, messageID, userID, parts[1], parts[2])
				} else {
					h.showIntervalZonePicker(ctx, chatID, messageID, parts[1])
				}
			}
		}

	case "close":
		h.deleteMessage(chatID, messageID)
	}
}

// --- Main Menu ---

func (h *Handlers) buildSettingsMainText(todoEnabled, summaryEnabled bool, summaryTime string) string {
	todoStatus := "✅ 已開啟"
	if !todoEnabled {
		todoStatus = "❌ 已關閉"
	}
	summaryStatus := "✅ 已開啟"
	if !summaryEnabled {
		summaryStatus = "❌ 已關閉"
	}
	return fmt.Sprintf("⚙️ **設定選單**\n\n📋 Todo 提醒: %s\n☀️ 每日摘要: %s (%s)", todoStatus, summaryStatus, summaryTime)
}

func (h *Handlers) buildSettingsMainKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Todo 提醒", "settings:todo"),
			tgbotapi.NewInlineKeyboardButtonData("☀️ 每日摘要", "settings:summary"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔕 勿擾時段", "settings:quiet:menu"),
			tgbotapi.NewInlineKeyboardButtonData("📊 每日上限", "settings:limit"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏱ 提醒頻率", "settings:interval:menu"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ 關閉", "settings:close"),
		),
	)
}

func (h *Handlers) showSettingsMain(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	text := h.buildSettingsMainText(settings.TodoRemindersEnabled, settings.DailySummaryEnabled, settings.DailySummaryTime)
	keyboard := h.buildSettingsMainKeyboard()

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

// --- Todo Settings ---

func (h *Handlers) showTodoSettings(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	status := "✅ 已開啟"
	if !settings.TodoRemindersEnabled {
		status = "❌ 已關閉"
	}

	text := fmt.Sprintf("📋 **Todo 提醒設定**\n\n目前狀態: %s", status)

	toggleLabel := "❌ 關閉"
	if !settings.TodoRemindersEnabled {
		toggleLabel = "✅ 開啟"
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleLabel, "settings:todo:toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:main"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) toggleTodoReminders(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	newEnabled := !settings.TodoRemindersEnabled
	if err := h.repos.UserSettings.SetTodoRemindersEnabled(ctx, userID, newEnabled); err != nil {
		log.Printf("Failed to toggle todo reminders: %v", err)
		return
	}

	h.showTodoSettings(ctx, chatID, messageID, userID)
}

// --- Daily Summary Settings ---

func (h *Handlers) showSummarySettings(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	status := "✅ 已開啟"
	if !settings.DailySummaryEnabled {
		status = "❌ 已關閉"
	}

	text := fmt.Sprintf("☀️ **每日摘要設定**\n\n目前狀態: %s\n發送時間: %s\n\n每天在指定時間發送今日行程和待辦事項摘要", status, settings.DailySummaryTime)

	toggleLabel := "❌ 關閉"
	if !settings.DailySummaryEnabled {
		toggleLabel = "✅ 開啟"
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleLabel, "settings:summary:toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏰ 設定發送時間", "settings:summary:time"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:main"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) toggleDailySummary(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	newEnabled := !settings.DailySummaryEnabled
	if err := h.repos.UserSettings.SetDailySummaryEnabled(ctx, userID, newEnabled); err != nil {
		log.Printf("Failed to toggle daily summary: %v", err)
		return
	}

	h.showSummarySettings(ctx, chatID, messageID, userID)
}

func (h *Handlers) showSummaryTimePicker(ctx context.Context, chatID int64, messageID int) {
	text := "☀️ **選擇每日摘要發送時間**"

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("06:00", "settings:summary:time:06:00"),
			tgbotapi.NewInlineKeyboardButtonData("07:00", "settings:summary:time:07:00"),
			tgbotapi.NewInlineKeyboardButtonData("08:00", "settings:summary:time:08:00"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("09:00", "settings:summary:time:09:00"),
			tgbotapi.NewInlineKeyboardButtonData("10:00", "settings:summary:time:10:00"),
			tgbotapi.NewInlineKeyboardButtonData("12:00", "settings:summary:time:12:00"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:summary"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) setDailySummaryTime(ctx context.Context, chatID int64, messageID int, userID int64, timeStr string) {
	if err := h.repos.UserSettings.SetDailySummaryTime(ctx, userID, timeStr); err != nil {
		log.Printf("Failed to set daily summary time: %v", err)
		return
	}

	h.showSummarySettings(ctx, chatID, messageID, userID)
}

// --- Quiet Hours Settings ---

func (h *Handlers) showQuietSettings(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	text := fmt.Sprintf("🔕 **勿擾時段**\n\n目前設定: %s - %s\n\n在此時段內不會發送 Todo 提醒",
		settings.QuietStart, settings.QuietEnd)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("設定開始時間", "settings:quiet:start"),
			tgbotapi.NewInlineKeyboardButtonData("設定結束時間", "settings:quiet:end"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:main"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) showQuietStartPicker(ctx context.Context, chatID int64, messageID int) {
	text := "🔕 **選擇勿擾開始時間**"

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("20:00", "settings:quiet:start:20:00"),
			tgbotapi.NewInlineKeyboardButtonData("21:00", "settings:quiet:start:21:00"),
			tgbotapi.NewInlineKeyboardButtonData("22:00", "settings:quiet:start:22:00"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("23:00", "settings:quiet:start:23:00"),
			tgbotapi.NewInlineKeyboardButtonData("00:00", "settings:quiet:start:00:00"),
			tgbotapi.NewInlineKeyboardButtonData("01:00", "settings:quiet:start:01:00"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:quiet:menu"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) showQuietEndPicker(ctx context.Context, chatID int64, messageID int) {
	text := "🔕 **選擇勿擾結束時間**"

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("06:00", "settings:quiet:end:06:00"),
			tgbotapi.NewInlineKeyboardButtonData("07:00", "settings:quiet:end:07:00"),
			tgbotapi.NewInlineKeyboardButtonData("08:00", "settings:quiet:end:08:00"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("09:00", "settings:quiet:end:09:00"),
			tgbotapi.NewInlineKeyboardButtonData("10:00", "settings:quiet:end:10:00"),
			tgbotapi.NewInlineKeyboardButtonData("11:00", "settings:quiet:end:11:00"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:quiet:menu"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) setQuietStart(ctx context.Context, chatID int64, messageID int, userID int64, timeStr string) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	if err := h.repos.UserSettings.SetQuietHours(ctx, userID, timeStr, settings.QuietEnd); err != nil {
		log.Printf("Failed to set quiet start: %v", err)
		return
	}

	h.showQuietSettings(ctx, chatID, messageID, userID)
}

func (h *Handlers) setQuietEnd(ctx context.Context, chatID int64, messageID int, userID int64, timeStr string) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	if err := h.repos.UserSettings.SetQuietHours(ctx, userID, settings.QuietStart, timeStr); err != nil {
		log.Printf("Failed to set quiet end: %v", err)
		return
	}

	h.showQuietSettings(ctx, chatID, messageID, userID)
}

func (h *Handlers) disableQuietHours(ctx context.Context, chatID int64, messageID int, userID int64) {
	// Set both to same time to effectively disable
	if err := h.repos.UserSettings.SetQuietHours(ctx, userID, "00:00", "00:00"); err != nil {
		log.Printf("Failed to disable quiet hours: %v", err)
		return
	}

	h.showQuietSettings(ctx, chatID, messageID, userID)
}

// --- Daily Limit Settings ---

func (h *Handlers) showLimitSettings(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	limitText := fmt.Sprintf("%d 則", settings.MaxDailyReminders)
	if settings.MaxDailyReminders == 0 {
		limitText = "無限制"
	}

	text := fmt.Sprintf("📊 **每日提醒上限**\n\n目前設定: %s\n\n達到上限後當天不再發送提醒", limitText)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("5", "settings:limit:5"),
			tgbotapi.NewInlineKeyboardButtonData("10", "settings:limit:10"),
			tgbotapi.NewInlineKeyboardButtonData("15", "settings:limit:15"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("20", "settings:limit:20"),
			tgbotapi.NewInlineKeyboardButtonData("無限制", "settings:limit:0"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:main"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) setDailyLimit(ctx context.Context, chatID int64, messageID int, userID int64, limitStr string) {
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		log.Printf("Invalid limit value: %v", err)
		return
	}

	if err := h.repos.UserSettings.SetMaxDailyReminders(ctx, userID, limit); err != nil {
		log.Printf("Failed to set daily limit: %v", err)
		return
	}

	h.showLimitSettings(ctx, chatID, messageID, userID)
}

// --- Interval Settings ---

func (h *Handlers) showIntervalSettings(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	text := fmt.Sprintf(`⏱ **提醒頻率設定**

**已過期** (overdue): 每 %d 分鐘
**緊急** (< 2小時): 每 %d 分鐘
**即將到期** (< 24小時): 每 %d 分鐘
**一般** (< 7天): 每 %d 分鐘

💡 高優先級的待辦會更頻繁提醒`,
		settings.ReminderIntervals.Overdue,
		settings.ReminderIntervals.Urgent,
		settings.ReminderIntervals.Soon,
		settings.ReminderIntervals.Normal,
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("調整 已過期", "settings:interval:overdue"),
			tgbotapi.NewInlineKeyboardButtonData("調整 緊急", "settings:interval:urgent"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("調整 即將到期", "settings:interval:soon"),
			tgbotapi.NewInlineKeyboardButtonData("調整 一般", "settings:interval:normal"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 重設為預設", "settings:interval:reset"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:main"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) showIntervalZonePicker(ctx context.Context, chatID int64, messageID int, zone string) {
	zoneName := map[string]string{
		"overdue": "已過期",
		"urgent":  "緊急",
		"soon":    "即將到期",
		"normal":  "一般",
	}[zone]

	text := fmt.Sprintf("⏱ **設定「%s」提醒間隔**", zoneName)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("15 分鐘", fmt.Sprintf("settings:interval:%s:15", zone)),
			tgbotapi.NewInlineKeyboardButtonData("30 分鐘", fmt.Sprintf("settings:interval:%s:30", zone)),
			tgbotapi.NewInlineKeyboardButtonData("1 小時", fmt.Sprintf("settings:interval:%s:60", zone)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("2 小時", fmt.Sprintf("settings:interval:%s:120", zone)),
			tgbotapi.NewInlineKeyboardButtonData("4 小時", fmt.Sprintf("settings:interval:%s:240", zone)),
			tgbotapi.NewInlineKeyboardButtonData("8 小時", fmt.Sprintf("settings:interval:%s:480", zone)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", "settings:interval:menu"),
		),
	)

	h.editMessageWithKeyboard(chatID, messageID, text, keyboard)
}

func (h *Handlers) setInterval(ctx context.Context, chatID int64, messageID int, userID int64, zone, minutesStr string) {
	minutes, err := strconv.Atoi(minutesStr)
	if err != nil {
		log.Printf("Invalid interval value: %v", err)
		return
	}

	if err := h.repos.UserSettings.SetReminderInterval(ctx, userID, zone, minutes); err != nil {
		log.Printf("Failed to set interval: %v", err)
		return
	}

	h.showIntervalSettings(ctx, chatID, messageID, userID)
}

func (h *Handlers) resetIntervals(ctx context.Context, chatID int64, messageID int, userID int64) {
	settings, err := h.repos.UserSettings.GetOrCreate(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings: %v", err)
		return
	}

	// Reset to defaults
	settings.ReminderIntervals.Overdue = 30
	settings.ReminderIntervals.Urgent = 30
	settings.ReminderIntervals.Soon = 120
	settings.ReminderIntervals.Normal = 480

	if err := h.repos.UserSettings.Update(ctx, settings); err != nil {
		log.Printf("Failed to reset intervals: %v", err)
		return
	}

	h.showIntervalSettings(ctx, chatID, messageID, userID)
}

// --- Helper Functions ---

func (h *Handlers) editMessageWithKeyboard(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	parsed := format.ParseMarkdown(text)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, parsed.Text)
	edit.Entities = parsed.Entities
	edit.ReplyMarkup = &keyboard
	if _, err := h.api.Send(edit); err != nil {
		log.Printf("Failed to edit message with keyboard: %v", err)
	}
}

func (h *Handlers) deleteMessage(chatID int64, messageID int) {
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
	if _, err := h.api.Request(deleteMsg); err != nil {
		log.Printf("Failed to delete message: %v", err)
	}
}

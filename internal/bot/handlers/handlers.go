package handlers

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/ai"
	"github.com/hray3182/LifeLine/internal/format"
	"github.com/hray3182/LifeLine/internal/repository"
)

type Repositories struct {
	User         *repository.UserRepository
	Memo         *repository.MemoRepository
	Todo         *repository.TodoRepository
	Reminder     *repository.ReminderRepository
	Category     *repository.CategoryRepository
	Transaction  *repository.TransactionRepository
	Event        *repository.EventRepository
	UserSettings *repository.UserSettingsRepository
}

type Handlers struct {
	api             *tgbotapi.BotAPI
	repos           *Repositories
	ai              *ai.Client
	devMode         bool
	logger          *slog.Logger
	schedulerNotify func()
}

func New(api *tgbotapi.BotAPI, repos *Repositories, aiClient *ai.Client, devMode bool) *Handlers {
	// Setup logger based on devMode
	var logger *slog.Logger
	if devMode {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	return &Handlers{
		api:     api,
		repos:   repos,
		ai:      aiClient,
		devMode: devMode,
		logger:  logger,
	}
}

// SetSchedulerNotify sets the scheduler notification function
func (h *Handlers) SetSchedulerNotify(fn func()) {
	h.schedulerNotify = fn
}

// notifyScheduler triggers the scheduler to check for pending items
func (h *Handlers) notifyScheduler() {
	if h.schedulerNotify != nil {
		h.schedulerNotify()
	}
}

// debug logs at debug level (only shown in dev mode)
func (h *Handlers) debug(msg string, args ...any) {
	h.logger.Debug(msg, args...)
}

func (h *Handlers) HandleCommand(ctx context.Context, msg *tgbotapi.Message) {
	// Ensure user exists
	_, err := h.repos.User.GetOrCreate(ctx, msg.From.ID, msg.From.UserName)
	if err != nil {
		log.Printf("Failed to get/create user: %v", err)
		return
	}

	switch msg.Command() {
	case "start":
		h.handleStart(ctx, msg)
	case "help":
		h.handleHelp(ctx, msg)
	case "memo":
		h.handleMemo(ctx, msg)
	case "memos":
		h.handleMemoList(ctx, msg)
	case "todo":
		h.handleTodo(ctx, msg)
	case "todos":
		h.handleTodoList(ctx, msg)
	case "done":
		h.handleTodoDone(ctx, msg)
	case "remind":
		h.handleReminder(ctx, msg)
	case "reminders":
		h.handleReminderList(ctx, msg)
	case "expense":
		h.handleExpense(ctx, msg)
	case "income":
		h.handleIncome(ctx, msg)
	case "balance":
		h.handleBalance(ctx, msg)
	case "event":
		h.handleEvent(ctx, msg)
	case "events":
		h.handleEventList(ctx, msg)
	case "settings":
		h.handleSettings(ctx, msg)
	default:
		h.sendMessage(msg.Chat.ID, "未知指令，請使用 /help 查看可用指令")
	}
}

func (h *Handlers) HandleMessage(ctx context.Context, msg *tgbotapi.Message) {
	// Ensure user exists
	_, err := h.repos.User.GetOrCreate(ctx, msg.From.ID, msg.From.UserName)
	if err != nil {
		log.Printf("Failed to get/create user: %v", err)
		return
	}

	// Process with AI
	h.handleAIMessage(ctx, msg)
}

func (h *Handlers) HandleCallbackQuery(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	h.debug("HandleCallbackQuery received", "data", callback.Data, "user_id", callback.From.ID)

	// Answer callback to remove loading state
	answer := tgbotapi.NewCallback(callback.ID, "")
	if _, err := h.api.Request(answer); err != nil {
		log.Printf("Failed to answer callback: %v", err)
	}

	// Parse callback data: "confirm:userID", "cancel:userID", "option:userID:index", or "remind_ack:reminderID"
	parts := strings.Split(callback.Data, ":")
	if len(parts) < 2 {
		h.debug("HandleCallbackQuery: invalid callback data format", "parts", len(parts))
		return
	}

	action := parts[0]

	// Handle reminder acknowledgement separately (different format)
	if action == "remind_ack" {
		h.handleReminderAcknowledge(ctx, callback, parts[1])
		return
	}

	// Handle settings callbacks (different format: settings:action:...)
	if action == "settings" {
		h.handleSettingsCallback(ctx, callback, parts[1:])
		return
	}

	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		h.debug("HandleCallbackQuery: failed to parse userID", "error", err)
		return
	}

	h.debug("HandleCallbackQuery parsed", "action", action, "target_user_id", userID)

	// Verify the callback is from the correct user
	if callback.From.ID != userID {
		h.debug("HandleCallbackQuery: user mismatch", "from_id", callback.From.ID, "target_id", userID)
		h.answerCallbackWithAlert(callback.ID, "這不是你的操作")
		return
	}

	// Get pending confirmation
	pendingMutex.RLock()
	pending, exists := pendingConfirmations[userID]
	pendingMutex.RUnlock()

	h.debug("HandleCallbackQuery: pending check", "exists", exists)

	if !exists || time.Now().After(pending.ExpiresAt) {
		h.debug("HandleCallbackQuery: confirmation expired or not found", "exists", exists)
		if exists {
			pendingMutex.Lock()
			delete(pendingConfirmations, userID)
			pendingMutex.Unlock()
		}
		h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "⏰ 確認已過期")
		return
	}

	h.debug("HandleCallbackQuery: found valid pending confirmation", "intent_action", pending.Intent.Action)

	// Clear pending
	pendingMutex.Lock()
	delete(pendingConfirmations, userID)
	pendingMutex.Unlock()

	// Create a fake message for executeIntent
	fakeMsg := &tgbotapi.Message{
		Chat: callback.Message.Chat,
		From: callback.From,
	}

	switch action {
	case "confirm":
		h.debug("HandleCallbackQuery: executing confirm action")
		h.executeAfterConfirmation(ctx, fakeMsg, callback.Message.Chat.ID, callback.Message.MessageID, pending.Intent, "已確認")
	case "cancel":
		h.debug("HandleCallbackQuery: executing cancel action")
		h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "❌ 已取消操作")
	case "option":
		h.debug("HandleCallbackQuery: processing option selection")
		// Parse option index
		if len(parts) != 3 {
			h.debug("HandleCallbackQuery: invalid option format", "parts", len(parts))
			return
		}
		optionIndex, err := strconv.Atoi(parts[2])
		if err != nil || optionIndex < 0 || optionIndex >= len(pending.Intent.ConfirmationOptions) {
			h.debug("HandleCallbackQuery: invalid option index", "index", parts[2], "error", err)
			h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "❌ 無效的選項")
			return
		}

		// Get selected option and merge parameters
		selectedOption := pending.Intent.ConfirmationOptions[optionIndex]
		h.debug("HandleCallbackQuery: selected option", "label", selectedOption.Label, "params", selectedOption.Parameters)
		if pending.Intent.Parameters == nil {
			pending.Intent.Parameters = make(map[string]string)
		}
		for key, value := range selectedOption.Parameters {
			pending.Intent.Parameters[key] = value
		}

		h.debug("HandleCallbackQuery: executing option action", "merged_params", pending.Intent.Parameters)
		h.executeAfterConfirmation(ctx, fakeMsg, callback.Message.Chat.ID, callback.Message.MessageID, pending.Intent, fmt.Sprintf("已選擇「%s」", selectedOption.Label))
	}
}

// executeAfterConfirmation handles execution after user confirmation, supporting ReturnResultToAI flow
func (h *Handlers) executeAfterConfirmation(ctx context.Context, fakeMsg *tgbotapi.Message, chatID int64, messageID int, intent *ai.Intent, confirmText string) {
	h.debug("executeAfterConfirmation", "action", intent.Action, "return_result_to_ai", intent.ReturnResultToAI)

	var result string
	// Handle multi_action specially
	if intent.Action == "multi_action" && len(intent.Actions) > 0 {
		h.debug("executeAfterConfirmation: handling multi_action", "action_count", len(intent.Actions))
		var results []string
		for i, action := range intent.Actions {
			h.debug("executeAfterConfirmation: executing sub-action", "index", i, "action", action.Action)
			actionResult := h.executeSingleAction(ctx, fakeMsg, action.Action, action.Parameters, false)
			results = append(results, fmt.Sprintf("[%d] %s", i+1, actionResult))
		}
		result = strings.Join(results, "\n")
	} else {
		result = h.executeSingleAction(ctx, fakeMsg, intent.Action, intent.Parameters, false)
	}
	h.debug("Tool result (confirmation)", "result", result)

	// If ReturnResultToAI is set, let AI process the result
	if intent.ReturnResultToAI && h.ai != nil {
		h.debug("ReturnResultToAI flow after confirmation")

		// Build conversation history with the tool result
		history := []ai.Message{
			{Role: "assistant", Content: "[工具執行結果]\n" + result},
		}

		// Let AI decide next action
		nextIntent, err := h.ai.ParseIntentWithHistory(ctx, history)
		if err != nil {
			log.Printf("Failed to parse next intent after confirmation: %v", err)
			h.editMessageText(chatID, messageID, fmt.Sprintf("✅ %s\n\n%s", confirmText, result))
			return
		}

		h.debug("Next intent after confirmation",
			"action", nextIntent.Action,
			"needs_confirmation", nextIntent.NeedsConfirmation,
			"ai_message", nextIntent.AIMessage,
			"raw", nextIntent.RawResponse)

		// If AI needs another confirmation (e.g., for delete)
		if nextIntent.NeedsConfirmation {
			h.editMessageText(chatID, messageID, fmt.Sprintf("✅ %s", confirmText))
			h.requestConfirmation(chatID, fakeMsg.From.ID, nextIntent)
			return
		}

		// If AI just wants to send a message
		if nextIntent.AIMessage != "" {
			h.editMessageText(chatID, messageID, fmt.Sprintf("✅ %s\n\n%s", confirmText, nextIntent.AIMessage))
			return
		}

		// Execute the next action if needed
		if nextIntent.Action != "unknown" && nextIntent.Action != "" {
			nextResult := h.executeSingleAction(ctx, fakeMsg, nextIntent.Action, nextIntent.Parameters, false)
			h.editMessageText(chatID, messageID, fmt.Sprintf("✅ %s\n\n%s", confirmText, nextResult))
			return
		}
	}

	// Default: just show the result
	h.editMessageText(chatID, messageID, fmt.Sprintf("✅ %s\n\n%s", confirmText, result))
}

func (h *Handlers) answerCallbackWithAlert(callbackID string, text string) {
	answer := tgbotapi.NewCallbackWithAlert(callbackID, text)
	if _, err := h.api.Request(answer); err != nil {
		log.Printf("Failed to answer callback with alert: %v", err)
	}
}

func (h *Handlers) editMessageText(chatID int64, messageID int, text string) {
	parsed := format.ParseMarkdown(text)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, parsed.Text)
	edit.Entities = parsed.Entities
	if _, err := h.api.Send(edit); err != nil {
		log.Printf("Failed to edit message: %v", err)
	}
}

func (h *Handlers) sendMessage(chatID int64, text string) {
	// 確保文字是有效的 UTF-8
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}
	parsed := format.ParseMarkdown(text)
	msg := tgbotapi.NewMessage(chatID, parsed.Text)
	msg.Entities = parsed.Entities
	if _, err := h.api.Send(msg); err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}

func (h *Handlers) handleStart(ctx context.Context, msg *tgbotapi.Message) {
	text := fmt.Sprintf(`👋 你好 %s！

我是 LifeLine，你的個人生活助理機器人。

我可以幫你：
📝 管理備忘錄
✅ 追蹤待辦事項（自動提醒快到期的任務）
⏰ 設定提醒
💰 記錄收支
📅 管理行事曆
☀️ 每日摘要（每天早上發送今日行程）

你可以直接用自然語言告訴我你想做什麼，例如：
• "幫我記一下明天要開會"
• "新增待辦：完成報告，截止週五"
• "提醒我下午 3 點喝水"
• "午餐花了 150 元"

使用 /help 查看所有指令
使用 /settings 調整提醒設定`, msg.From.FirstName)
	h.sendMessage(msg.Chat.ID, text)
}

func (h *Handlers) handleHelp(ctx context.Context, msg *tgbotapi.Message) {
	text := `📖 **指令列表**

**備忘錄**
/memo <內容> - 新增備忘錄
/memos - 查看備忘錄列表

**待辦事項**
/todo <標題> - 新增待辦
/todos - 查看待辦列表
/done <編號> - 完成待辦
• 設定截止時間的待辦會自動提醒

**提醒**
/remind <時間> <訊息> - 設定提醒
/reminders - 查看提醒列表

**記帳**
/expense <金額> <說明> - 記錄支出
/income <金額> <說明> - 記錄收入
/balance - 查看收支統計

**行事曆**
/event <標題> <時間> - 新增事件
/events - 查看近期事件

**設定**
/settings - 調整提醒設定
• Todo 提醒開關與頻率
• 每日摘要時間
• 勿擾時段

💡 你也可以直接用自然語言告訴我！`
	h.sendMessage(msg.Chat.ID, text)
}

package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/ai"
	"github.com/hray3182/LifeLine/internal/models"
	"github.com/hray3182/LifeLine/internal/rrule"
)

// PendingConfirmation stores intent waiting for user confirmation
type PendingConfirmation struct {
	Intent    *ai.Intent
	ExpiresAt time.Time
}

// ConversationSession stores multi-turn conversation state
type ConversationSession struct {
	History   []ai.Message
	ExpiresAt time.Time
}

var (
	pendingConfirmations = make(map[int64]*PendingConfirmation) // userID -> pending
	pendingMutex         sync.RWMutex

	conversationSessions = make(map[int64]*ConversationSession) // userID -> session
	sessionMutex         sync.RWMutex
)

const (
	sessionTimeout = 5 * time.Minute
	maxHistoryLen  = 10
)

func (h *Handlers) handleAIMessage(ctx context.Context, msg *tgbotapi.Message) {
	if h.ai == nil {
		h.sendMessage(msg.Chat.ID, "AI 功能尚未啟用")
		return
	}

	// DEV mode: log incoming message
	if h.devMode {
		log.Printf("[DEV] Incoming message from %s (@%s): %s",
			msg.From.FirstName, msg.From.UserName, msg.Text)
		if msg.ReplyToMessage != nil {
			log.Printf("[DEV] ReplyToMessage: %s", msg.ReplyToMessage.Text)
		}
	}

	// Check if user is confirming a pending action
	if h.handleConfirmationResponse(ctx, msg) {
		return
	}

	// Get or create conversation session
	session := h.getOrCreateSession(msg.From.ID)

	// If user is replying to a message, add it as context
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.Text != "" {
		// Check if the replied message is from the bot (our previous response)
		if msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.IsBot {
			session.History = append(session.History, ai.Message{
				Role:    "assistant",
				Content: msg.ReplyToMessage.Text,
			})
			if h.devMode {
				log.Printf("[DEV] Added ReplyToMessage to context as assistant message")
			}
		}
	}

	// Add user message to history
	session.History = append(session.History, ai.Message{
		Role:    "user",
		Content: msg.Text,
	})

	// Trim history if too long
	if len(session.History) > maxHistoryLen {
		session.History = session.History[len(session.History)-maxHistoryLen:]
	}

	// DEV mode: log conversation history
	if h.devMode {
		log.Printf("[DEV] Conversation history (%d messages):", len(session.History))
		for i, m := range session.History {
			log.Printf("[DEV]   [%d] %s: %s", i, m.Role, truncateString(m.Content, 100))
		}
	}

	// Parse intent with conversation history
	intent, err := h.ai.ParseIntentWithHistory(ctx, session.History)
	if err != nil {
		log.Printf("Failed to parse intent: %v", err)
		h.sendMessage(msg.Chat.ID, "抱歉，我無法理解你的訊息。請試著用更清楚的方式描述，或使用 /help 查看可用指令。")
		return
	}

	// DEV mode: log parsed intent
	if h.devMode {
		log.Printf("[DEV] Parsed intent: action=%s, entity=%s, confidence=%.2f, needs_confirmation=%v, need_more_info=%v",
			intent.Action, intent.Entity, intent.Confidence, intent.NeedsConfirmation, intent.NeedMoreInfo)
		log.Printf("[DEV] Parameters: %v", intent.Parameters)
		if intent.AIMessage != "" {
			log.Printf("[DEV] AI Message: %s", intent.AIMessage)
		}
		log.Printf("[DEV] Raw response: %s", intent.RawResponse)
	} else {
		log.Printf("Parsed intent: action=%s, entity=%s, confidence=%.2f, needs_confirmation=%v, need_more_info=%v",
			intent.Action, intent.Entity, intent.Confidence, intent.NeedsConfirmation, intent.NeedMoreInfo)
	}

	// Handle low confidence
	if intent.Confidence < 0.5 {
		response := "我不太確定你想做什麼，可以說得更清楚一點嗎？"
		if intent.AIMessage != "" {
			response = intent.AIMessage
		}
		h.sendMessage(msg.Chat.ID, response)
		// Add AI response to history
		session.History = append(session.History, ai.Message{
			Role:    "assistant",
			Content: response,
		})
		h.saveSession(msg.From.ID, session)
		return
	}

	// Handle need more info (multi-turn)
	if intent.NeedMoreInfo {
		response := intent.FollowUpPrompt
		if response == "" {
			response = intent.AIMessage
		}
		if response == "" {
			response = "請提供更多資訊"
		}
		h.sendMessage(msg.Chat.ID, response)
		// Add AI response to history
		session.History = append(session.History, ai.Message{
			Role:    "assistant",
			Content: response,
		})
		h.saveSession(msg.From.ID, session)
		return
	}

	// Check if confirmation is needed
	if intent.NeedsConfirmation {
		h.requestConfirmation(msg.Chat.ID, msg.From.ID, intent)
		// Clear session after confirmation request since we store intent separately
		h.clearSession(msg.From.ID)
		return
	}

	// Execute intent and get result
	result := h.executeIntentWithResult(ctx, msg, intent)

	// Add execution result to history for AI to process
	if result != "" {
		session.History = append(session.History, ai.Message{
			Role:    "assistant",
			Content: result,
		})
	}

	// Clear session after successful action (unless it's a list/query action)
	if !strings.HasPrefix(intent.Action, "list_") && intent.Action != "get_balance" {
		h.clearSession(msg.From.ID)
	} else {
		h.saveSession(msg.From.ID, session)
	}
}

func (h *Handlers) getOrCreateSession(userID int64) *ConversationSession {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	session, exists := conversationSessions[userID]
	if !exists || time.Now().After(session.ExpiresAt) {
		session = &ConversationSession{
			History:   []ai.Message{},
			ExpiresAt: time.Now().Add(sessionTimeout),
		}
		conversationSessions[userID] = session
	} else {
		// Refresh expiry
		session.ExpiresAt = time.Now().Add(sessionTimeout)
	}
	return session
}

func (h *Handlers) saveSession(userID int64, session *ConversationSession) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	session.ExpiresAt = time.Now().Add(sessionTimeout)
	conversationSessions[userID] = session
}

func (h *Handlers) clearSession(userID int64) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	delete(conversationSessions, userID)
}

func (h *Handlers) handleConfirmationResponse(ctx context.Context, msg *tgbotapi.Message) bool {
	text := msg.Text

	pendingMutex.RLock()
	pending, exists := pendingConfirmations[msg.From.ID]
	pendingMutex.RUnlock()

	if !exists || time.Now().After(pending.ExpiresAt) {
		if exists {
			pendingMutex.Lock()
			delete(pendingConfirmations, msg.From.ID)
			pendingMutex.Unlock()
		}
		return false
	}

	// Check for confirmation keywords
	isConfirm := text == "是" || text == "確認" || text == "對" || text == "好" || text == "yes" || text == "y" || text == "Y"
	isCancel := text == "否" || text == "取消" || text == "不" || text == "no" || text == "n" || text == "N"

	if !isConfirm && !isCancel {
		return false
	}

	// Clear pending
	pendingMutex.Lock()
	delete(pendingConfirmations, msg.From.ID)
	pendingMutex.Unlock()

	if isCancel {
		h.sendMessage(msg.Chat.ID, "已取消操作")
		return true
	}

	// Execute the confirmed intent
	h.executeIntent(ctx, msg, pending.Intent)
	return true
}

func (h *Handlers) requestConfirmation(chatID int64, userID int64, intent *ai.Intent) {
	// Store pending confirmation (expires in 2 minutes)
	pendingMutex.Lock()
	pendingConfirmations[userID] = &PendingConfirmation{
		Intent:    intent,
		ExpiresAt: time.Now().Add(2 * time.Minute),
	}
	pendingMutex.Unlock()

	// Build confirmation message - prefer ai_message, fallback to confirmation_reason
	var confirmMsg string
	if intent.AIMessage != "" {
		confirmMsg = intent.AIMessage
	} else if intent.ConfirmationReason != "" {
		confirmMsg = intent.ConfirmationReason
	} else {
		confirmMsg = fmt.Sprintf("確認執行 %s 操作？", intent.Action)
	}

	// Create inline keyboard
	var keyboard tgbotapi.InlineKeyboardMarkup
	if len(intent.ConfirmationOptions) > 0 {
		// Use custom options from AI
		var buttons []tgbotapi.InlineKeyboardButton
		for i, opt := range intent.ConfirmationOptions {
			// callback data format: "option:<userID>:<index>"
			callbackData := fmt.Sprintf("option:%d:%d", userID, i)
			buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(opt.Label, callbackData))
		}
		// Add cancel button
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("❌ 取消", fmt.Sprintf("cancel:%d", userID)))

		// Split into rows of 2-3 buttons
		var rows [][]tgbotapi.InlineKeyboardButton
		for i := 0; i < len(buttons); i += 2 {
			end := i + 2
			if end > len(buttons) {
				end = len(buttons)
			}
			rows = append(rows, buttons[i:end])
		}
		keyboard = tgbotapi.NewInlineKeyboardMarkup(rows...)
	} else {
		// Default confirm/cancel buttons
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ 確認", fmt.Sprintf("confirm:%d", userID)),
				tgbotapi.NewInlineKeyboardButtonData("❌ 取消", fmt.Sprintf("cancel:%d", userID)),
			),
		)
	}

	msg := tgbotapi.NewMessage(chatID, confirmMsg)
	msg.ReplyMarkup = keyboard

	if _, err := h.api.Send(msg); err != nil {
		log.Printf("Failed to send confirmation message: %v", err)
	}
}

// escapeHTML escapes special HTML characters
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// executeIntent is kept for confirmation flow compatibility
func (h *Handlers) executeIntent(ctx context.Context, msg *tgbotapi.Message, intent *ai.Intent) {
	h.executeIntentWithResult(ctx, msg, intent)
}

// executeIntentWithResult executes the intent and returns the result message
func (h *Handlers) executeIntentWithResult(ctx context.Context, msg *tgbotapi.Message, intent *ai.Intent) string {
	// Handle multi-action
	if intent.Action == "multi_action" && len(intent.Actions) > 0 {
		var results []string
		for i, action := range intent.Actions {
			result := h.executeSingleAction(ctx, msg, action.Action, action.Parameters, false)
			results = append(results, fmt.Sprintf("[%d] %s: %s", i+1, action.Action, result))
		}
		combinedResult := strings.Join(results, "\n")
		h.sendMessage(msg.Chat.ID, combinedResult)
		return combinedResult
	}

	// Single action (backward compatible)
	return h.executeSingleAction(ctx, msg, intent.Action, intent.Parameters, true)
}

// executeSingleAction executes a single action and returns the result
func (h *Handlers) executeSingleAction(ctx context.Context, msg *tgbotapi.Message, action string, params map[string]string, sendMsg bool) string {
	var result string
	switch action {
	case "create_memo":
		result = h.handleAICreateMemoResult(ctx, msg, params, sendMsg)
	case "list_memo":
		result = h.handleAIListMemoResult(ctx, msg, params, sendMsg)
	case "delete_memo":
		result = h.handleAIDeleteMemoResult(ctx, msg, params, sendMsg)
	case "create_todo":
		result = h.handleAICreateTodoResult(ctx, msg, params, sendMsg)
	case "list_todo":
		result = h.handleAIListTodoResult(ctx, msg, params, sendMsg)
	case "complete_todo":
		result = h.handleAICompleteTodoResult(ctx, msg, params, sendMsg)
	case "delete_todo":
		result = h.handleAIDeleteTodoResult(ctx, msg, params, sendMsg)
	case "update_todo":
		result = h.handleAIUpdateTodoResult(ctx, msg, params, sendMsg)
	case "create_reminder":
		result = h.handleAICreateReminderResult(ctx, msg, params, sendMsg)
	case "list_reminder":
		result = h.handleAIListReminderResult(ctx, msg, params, sendMsg)
	case "delete_reminder":
		result = h.handleAIDeleteReminderResult(ctx, msg, params, sendMsg)
	case "create_expense":
		result = h.handleAICreateTransactionResult(ctx, msg, params, models.TransactionTypeExpense, sendMsg)
	case "create_income":
		result = h.handleAICreateTransactionResult(ctx, msg, params, models.TransactionTypeIncome, sendMsg)
	case "list_transaction":
		result = h.handleAIListTransactionResult(ctx, msg, params, sendMsg)
	case "delete_transaction":
		result = h.handleAIDeleteTransactionResult(ctx, msg, params, sendMsg)
	case "get_balance":
		result = h.handleBalanceWithResult(ctx, msg)
	case "create_event":
		result = h.handleAICreateEventResult(ctx, msg, params, sendMsg)
	case "list_event":
		result = h.handleAIListEventResult(ctx, msg, params, sendMsg)
	case "delete_event":
		result = h.handleAIDeleteEventResult(ctx, msg, params, sendMsg)
	case "update_event":
		result = h.handleAIUpdateEventResult(ctx, msg, params, sendMsg)
	case "unknown":
		result = "無法識別的操作"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
	default:
		result = "抱歉，我不確定你想做什麼。請使用 /help 查看可用指令。"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
	}
	return result
}

// List handlers with keyword search

func (h *Handlers) handleAIListMemo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIListMemoResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIListMemoResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	keyword := params["keyword"]
	var memos []*models.Memo
	var err error

	if keyword != "" {
		memos, err = h.repos.Memo.Search(ctx, msg.From.ID, keyword)
	} else {
		memos, err = h.repos.Memo.GetByUserID(ctx, msg.From.ID, 10, 0)
	}

	if err != nil {
		result := "取得備忘錄失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if len(memos) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("找不到包含「%s」的備忘錄", keyword)
		} else {
			result = "目前沒有備忘錄"
		}
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("備忘錄搜尋結果 (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("備忘錄列表\n\n")
	}
	for _, memo := range memos {
		content := memo.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n", memo.MemoID, content))
		sb.WriteString(fmt.Sprintf("   建立於 %s\n\n", memo.CreatedAt.Format("2006-01-02 15:04")))
	}

	result := sb.String()
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAIListTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIListTodoResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIListTodoResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	keyword := params["keyword"]
	var todos []*models.Todo
	var err error

	if keyword != "" {
		todos, err = h.repos.Todo.Search(ctx, msg.From.ID, keyword, false)
	} else {
		todos, err = h.repos.Todo.GetByUserID(ctx, msg.From.ID, false)
	}

	if err != nil {
		result := "取得待辦事項失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if len(todos) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("找不到包含「%s」的待辦事項", keyword)
		} else {
			result = "目前沒有待辦事項"
		}
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("待辦事項搜尋結果 (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("待辦事項列表\n\n")
	}
	for _, todo := range todos {
		status := "[ ]"
		if todo.IsCompleted() {
			status = "[x]"
		}

		title := todo.Title
		if len(title) > 40 {
			title = title[:40] + "..."
		}

		sb.WriteString(fmt.Sprintf("%s %d. %s", status, todo.TodoID, title))
		if todo.DueTime != nil {
			sb.WriteString(fmt.Sprintf("\n   截止: %s", todo.DueTime.Format("2006-01-02 15:04")))
		}
		if todo.Priority > 0 {
			sb.WriteString(fmt.Sprintf(" | 優先級: %d", todo.Priority))
		}
		sb.WriteString("\n\n")
	}

	result := sb.String()
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

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

func (h *Handlers) handleAIListTransaction(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIListTransactionResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIListTransactionResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	keyword := params["keyword"]
	var transactions []*models.Transaction
	var err error

	if keyword != "" {
		transactions, err = h.repos.Transaction.Search(ctx, msg.From.ID, keyword)
	} else {
		transactions, err = h.repos.Transaction.GetByUserID(ctx, msg.From.ID, 20, 0)
	}

	if err != nil {
		result := "取得交易記錄失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if len(transactions) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("找不到包含「%s」的交易記錄", keyword)
		} else {
			result = "目前沒有交易記錄"
		}
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("交易記錄搜尋結果 (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("交易記錄\n\n")
	}
	for _, tx := range transactions {
		typeStr := "支出"
		if tx.Type == models.TransactionTypeIncome {
			typeStr = "收入"
		}

		dateStr := ""
		if tx.TransactionDate != nil {
			dateStr = tx.TransactionDate.Format("01/02")
		}

		sb.WriteString(fmt.Sprintf("[%s] %d. %.2f", typeStr, tx.TransactionID, tx.Amount))
		if tx.Description != "" {
			desc := tx.Description
			if len(desc) > 20 {
				desc = desc[:20] + "..."
			}
			sb.WriteString(fmt.Sprintf(" - %s", desc))
		}
		if dateStr != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", dateStr))
		}
		sb.WriteString("\n")
	}

	result := sb.String()
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAIListEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIListEventResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIListEventResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	keyword := params["keyword"]
	var events []*models.Event
	var err error

	if keyword != "" {
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

// Create handlers

func (h *Handlers) handleAICreateMemo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAICreateMemoResult(ctx, msg, params, true)
}

func (h *Handlers) handleAICreateMemoResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	content := params["content"]
	if content == "" {
		content = msg.Text
	}

	tags := params["tags"]
	memo, err := h.CreateMemo(ctx, msg.From.ID, content, tags)
	if err != nil {
		result := "建立備忘錄失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("備忘錄已建立 (ID: %d)\n內容: %s", memo.MemoID, content)
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAICreateTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAICreateTodoResult(ctx, msg, params, true)
}

func (h *Handlers) handleAICreateTodoResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	title := params["title"]
	if title == "" {
		result := "請提供待辦事項標題"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	description := params["description"]
	tags := params["tags"]

	var priority int
	if p, ok := params["priority"]; ok {
		priority, _ = strconv.Atoi(p)
	}

	var dueTime *time.Time
	if dt, ok := params["due_time"]; ok && dt != "" {
		t := parseDateTime(dt)
		if t != nil {
			dueTime = t
		}
	}

	todo, err := h.CreateTodo(ctx, msg.From.ID, title, description, priority, dueTime, tags)
	if err != nil {
		result := "建立待辦事項失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("待辦事項已建立 (ID: %d)\n標題: %s", todo.TodoID, title)
	if dueTime != nil {
		result += fmt.Sprintf("\n截止時間: %s", dueTime.Format("2006-01-02 15:04"))
	}
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAICompleteTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAICompleteTodoResult(ctx, msg, params, true)
}

func (h *Handlers) handleAICompleteTodoResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	idStr := params["id"]
	if idStr == "" {
		result := "請提供待辦事項編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	todoID, err := strconv.Atoi(idStr)
	if err != nil {
		result := "無效的編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if err := h.repos.Todo.Complete(ctx, todoID, msg.From.ID); err != nil {
		result := "完成待辦事項失敗，請確認編號是否正確"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("待辦事項 #%d 已完成", todoID)
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

func (h *Handlers) handleAICreateTransaction(ctx context.Context, msg *tgbotapi.Message, params map[string]string, txType models.TransactionType) string {
	return h.handleAICreateTransactionResult(ctx, msg, params, txType, true)
}

func (h *Handlers) handleAICreateTransactionResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, txType models.TransactionType, sendMsg bool) string {
	amountStr := params["amount"]
	if amountStr == "" {
		result := "請提供金額"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		result := "無效的金額"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	description := params["description"]
	if description == "" {
		description = params["item"]
	}
	category := params["category"]

	tx, err := h.CreateTransaction(ctx, msg.From.ID, txType, amount, description, category, nil)
	if err != nil {
		result := "記錄失敗，請稍後再試"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	typeStr := "支出"
	if txType == models.TransactionTypeIncome {
		typeStr = "收入"
	}

	result := fmt.Sprintf("%s已記錄 (ID: %d)\n金額: %.2f", typeStr, tx.TransactionID, amount)
	if description != "" {
		result += fmt.Sprintf("\n說明: %s", description)
	}
	if category != "" {
		result += fmt.Sprintf("\n分類: %s", category)
	}
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

// Delete handlers

func (h *Handlers) handleAIDeleteMemo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIDeleteMemoResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIDeleteMemoResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的備忘錄編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if err := h.repos.Memo.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除備忘錄失敗，請確認編號是否正確"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("備忘錄 #%d 已刪除", id)
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAIDeleteTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIDeleteTodoResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIDeleteTodoResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的待辦事項編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if err := h.repos.Todo.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除待辦事項失敗，請確認編號是否正確"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("待辦事項 #%d 已刪除", id)
	if sendMsg {
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

func (h *Handlers) handleAIUpdateTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIUpdateTodoResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIUpdateTodoResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的待辦事項編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	todo, err := h.repos.Todo.GetByID(ctx, id, msg.From.ID)
	if err != nil {
		result := "找不到該待辦事項"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	// Update fields if provided
	if title, ok := params["title"]; ok && title != "" {
		todo.Title = title
	}
	if desc, ok := params["description"]; ok {
		todo.Description = desc
	}
	if p, ok := params["priority"]; ok && p != "" {
		todo.Priority, _ = strconv.Atoi(p)
	}
	if dt, ok := params["due_time"]; ok && dt != "" {
		todo.DueTime = parseDateTime(dt)
	}
	if tags, ok := params["tags"]; ok {
		todo.Tags = tags
	}

	if err := h.repos.Todo.Update(ctx, todo); err != nil {
		result := "更新待辦事項失敗"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("待辦事項 #%d 已更新", id)
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

func (h *Handlers) handleAIDeleteTransaction(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	return h.handleAIDeleteTransactionResult(ctx, msg, params, true)
}

func (h *Handlers) handleAIDeleteTransactionResult(ctx context.Context, msg *tgbotapi.Message, params map[string]string, sendMsg bool) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的交易記錄編號"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	if err := h.repos.Transaction.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除交易記錄失敗，請確認編號是否正確"
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
		return result
	}

	result := fmt.Sprintf("交易記錄 #%d 已刪除", id)
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

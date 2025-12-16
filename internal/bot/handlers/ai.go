package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/ai"
	"github.com/hray3182/LifeLine/internal/models"
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

	// Check if user is confirming a pending action
	if h.handleConfirmationResponse(ctx, msg) {
		return
	}

	// Get or create conversation session
	session := h.getOrCreateSession(msg.From.ID)

	// Add user message to history
	session.History = append(session.History, ai.Message{
		Role:    "user",
		Content: msg.Text,
	})

	// Trim history if too long
	if len(session.History) > maxHistoryLen {
		session.History = session.History[len(session.History)-maxHistoryLen:]
	}

	// Parse intent with conversation history
	intent, err := h.ai.ParseIntentWithHistory(ctx, session.History)
	if err != nil {
		log.Printf("Failed to parse intent: %v", err)
		h.sendMessage(msg.Chat.ID, "抱歉，我無法理解你的訊息。請試著用更清楚的方式描述，或使用 /help 查看可用指令。")
		return
	}

	log.Printf("Parsed intent: action=%s, entity=%s, confidence=%.2f, needs_confirmation=%v, need_more_info=%v",
		intent.Action, intent.Entity, intent.Confidence, intent.NeedsConfirmation, intent.NeedMoreInfo)

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

	// Build confirmation message
	var confirmMsg string
	if intent.ConfirmationReason != "" {
		confirmMsg = fmt.Sprintf("⚠️ *需要確認*\n\n%s\n\n", intent.ConfirmationReason)
	} else {
		confirmMsg = fmt.Sprintf("⚠️ *需要確認*\n\n確認執行 %s 操作？\n\n", intent.Action)
	}

	// Show action details
	if len(intent.Parameters) > 0 {
		confirmMsg += "*操作詳情:*\n"
		paramsJSON, _ := json.MarshalIndent(intent.Parameters, "", "  ")
		confirmMsg += "```\n" + string(paramsJSON) + "\n```\n\n"
	}

	confirmMsg += "回覆「*是*」確認，或「*否*」取消"

	h.sendMessage(chatID, confirmMsg)
}

// executeIntent is kept for confirmation flow compatibility
func (h *Handlers) executeIntent(ctx context.Context, msg *tgbotapi.Message, intent *ai.Intent) {
	h.executeIntentWithResult(ctx, msg, intent)
}

// executeIntentWithResult executes the intent and returns the result message
func (h *Handlers) executeIntentWithResult(ctx context.Context, msg *tgbotapi.Message, intent *ai.Intent) string {
	var result string
	switch intent.Action {
	case "create_memo":
		result = h.handleAICreateMemo(ctx, msg, intent.Parameters)
	case "list_memo":
		result = h.handleAIListMemo(ctx, msg, intent.Parameters)
	case "delete_memo":
		result = h.handleAIDeleteMemo(ctx, msg, intent.Parameters)
	case "create_todo":
		result = h.handleAICreateTodo(ctx, msg, intent.Parameters)
	case "list_todo":
		result = h.handleAIListTodo(ctx, msg, intent.Parameters)
	case "complete_todo":
		result = h.handleAICompleteTodo(ctx, msg, intent.Parameters)
	case "delete_todo":
		result = h.handleAIDeleteTodo(ctx, msg, intent.Parameters)
	case "update_todo":
		result = h.handleAIUpdateTodo(ctx, msg, intent.Parameters)
	case "create_reminder":
		result = h.handleAICreateReminder(ctx, msg, intent.Parameters)
	case "list_reminder":
		result = h.handleAIListReminder(ctx, msg, intent.Parameters)
	case "delete_reminder":
		result = h.handleAIDeleteReminder(ctx, msg, intent.Parameters)
	case "create_expense":
		result = h.handleAICreateTransaction(ctx, msg, intent.Parameters, models.TransactionTypeExpense)
	case "create_income":
		result = h.handleAICreateTransaction(ctx, msg, intent.Parameters, models.TransactionTypeIncome)
	case "list_transaction":
		result = h.handleAIListTransaction(ctx, msg, intent.Parameters)
	case "delete_transaction":
		result = h.handleAIDeleteTransaction(ctx, msg, intent.Parameters)
	case "get_balance":
		result = h.handleBalanceWithResult(ctx, msg)
	case "create_event":
		result = h.handleAICreateEvent(ctx, msg, intent.Parameters)
	case "list_event":
		result = h.handleAIListEvent(ctx, msg, intent.Parameters)
	case "delete_event":
		result = h.handleAIDeleteEvent(ctx, msg, intent.Parameters)
	case "update_event":
		result = h.handleAIUpdateEvent(ctx, msg, intent.Parameters)
	case "unknown":
		// Handle unknown/chat with AI message
		if intent.AIMessage != "" {
			result = intent.AIMessage
			h.sendMessage(msg.Chat.ID, result)
		} else {
			result = "抱歉，我不確定你想做什麼。請使用 /help 查看可用指令。"
			h.sendMessage(msg.Chat.ID, result)
		}
	default:
		result = "抱歉，我不確定你想做什麼。請使用 /help 查看可用指令。"
		h.sendMessage(msg.Chat.ID, result)
	}
	return result
}

// List handlers with keyword search

func (h *Handlers) handleAIListMemo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
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
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if len(memos) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("📝 找不到包含「%s」的備忘錄", keyword)
		} else {
			result = "📝 目前沒有備忘錄"
		}
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("📝 *備忘錄搜尋結果* (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("📝 *備忘錄列表*\n\n")
	}
	for _, memo := range memos {
		content := memo.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		sb.WriteString(fmt.Sprintf("*%d.* %s\n", memo.MemoID, content))
		sb.WriteString(fmt.Sprintf("   _建立於 %s_\n\n", memo.CreatedAt.Format("2006-01-02 15:04")))
	}

	result := sb.String()
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIListTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
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
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if len(todos) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("📋 找不到包含「%s」的待辦事項", keyword)
		} else {
			result = "✅ 目前沒有待辦事項"
		}
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("📋 *待辦事項搜尋結果* (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("📋 *待辦事項列表*\n\n")
	}
	for _, todo := range todos {
		status := "⬜"
		if todo.IsCompleted() {
			status = "✅"
		}

		title := todo.Title
		if len(title) > 40 {
			title = title[:40] + "..."
		}

		sb.WriteString(fmt.Sprintf("%s *%d.* %s", status, todo.TodoID, title))
		if todo.DueTime != nil {
			sb.WriteString(fmt.Sprintf("\n   📅 %s", todo.DueTime.Format("2006-01-02 15:04")))
		}
		if todo.Priority > 0 {
			sb.WriteString(fmt.Sprintf(" | 優先級: %d", todo.Priority))
		}
		sb.WriteString("\n\n")
	}

	result := sb.String()
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIListReminder(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
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
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if len(reminders) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("⏰ 找不到包含「%s」的提醒", keyword)
		} else {
			result = "⏰ 目前沒有提醒"
		}
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("⏰ *提醒搜尋結果* (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("⏰ *提醒列表*\n\n")
	}
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

	result := sb.String()
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIListTransaction(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
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
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if len(transactions) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("💰 找不到包含「%s」的交易記錄", keyword)
		} else {
			result = "💰 目前沒有交易記錄"
		}
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("💰 *交易記錄搜尋結果* (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("💰 *交易記錄*\n\n")
	}
	for _, tx := range transactions {
		emoji := "💸"
		if tx.Type == models.TransactionTypeIncome {
			emoji = "💰"
		}

		dateStr := ""
		if tx.TransactionDate != nil {
			dateStr = tx.TransactionDate.Format("01/02")
		}

		sb.WriteString(fmt.Sprintf("%s *%d.* %.2f", emoji, tx.TransactionID, tx.Amount))
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
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIListEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
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
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if len(events) == 0 {
		var result string
		if keyword != "" {
			result = fmt.Sprintf("📅 找不到包含「%s」的事件", keyword)
		} else {
			result = "📅 目前沒有事件"
		}
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	var sb strings.Builder
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("📅 *事件搜尋結果* (關鍵字: %s)\n\n", keyword))
	} else {
		sb.WriteString("📅 *事件列表*\n\n")
	}
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

	result := sb.String()
	h.sendMessage(msg.Chat.ID, result)
	return result
}

// Create handlers

func (h *Handlers) handleAICreateMemo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	content := params["content"]
	if content == "" {
		content = msg.Text
	}

	tags := params["tags"]
	memo, err := h.CreateMemo(ctx, msg.From.ID, content, tags)
	if err != nil {
		result := "建立備忘錄失敗，請稍後再試"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("📝 備忘錄已建立 (ID: %d)\n內容: %s", memo.MemoID, content)
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAICreateTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	title := params["title"]
	if title == "" {
		result := "請提供待辦事項標題"
		h.sendMessage(msg.Chat.ID, result)
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
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("✅ 待辦事項已建立 (ID: %d)\n標題: %s", todo.TodoID, title)
	if dueTime != nil {
		result += fmt.Sprintf("\n截止時間: %s", dueTime.Format("2006-01-02 15:04"))
	}
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAICompleteTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	idStr := params["id"]
	if idStr == "" {
		result := "請提供待辦事項編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	todoID, err := strconv.Atoi(idStr)
	if err != nil {
		result := "無效的編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if err := h.repos.Todo.Complete(ctx, todoID, msg.From.ID); err != nil {
		result := "完成待辦事項失敗，請確認編號是否正確"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("✅ 待辦事項 #%d 已完成！", todoID)
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAICreateReminder(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	message := params["message"]
	if message == "" {
		message = params["content"]
	}
	if message == "" {
		result := "請提供提醒訊息"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	var remindAt *time.Time
	if dt, ok := params["remind_at"]; ok && dt != "" {
		remindAt = parseDateTime(dt)
	}
	if remindAt == nil {
		if dt, ok := params["time"]; ok && dt != "" {
			remindAt = parseDateTime(dt)
		}
	}

	reminder, err := h.CreateReminder(ctx, msg.From.ID, message, remindAt, "")
	if err != nil {
		result := "建立提醒失敗，請稍後再試"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("⏰ 提醒已設定 (ID: %d)\n訊息: %s", reminder.ReminderID, message)
	if remindAt != nil {
		result += fmt.Sprintf("\n時間: %s", remindAt.Format("2006-01-02 15:04"))
	}
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAICreateTransaction(ctx context.Context, msg *tgbotapi.Message, params map[string]string, txType models.TransactionType) string {
	amountStr := params["amount"]
	if amountStr == "" {
		result := "請提供金額"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		result := "無效的金額"
		h.sendMessage(msg.Chat.ID, result)
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
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	emoji := "💸"
	typeStr := "支出"
	if txType == models.TransactionTypeIncome {
		emoji = "💰"
		typeStr = "收入"
	}

	result := fmt.Sprintf("%s %s已記錄 (ID: %d)\n金額: %.2f", emoji, typeStr, tx.TransactionID, amount)
	if description != "" {
		result += fmt.Sprintf("\n說明: %s", description)
	}
	if category != "" {
		result += fmt.Sprintf("\n分類: %s", category)
	}
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAICreateEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	title := params["title"]
	if title == "" {
		result := "請提供事件標題"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	description := params["description"]
	tags := params["tags"]

	var startTime, endTime *time.Time
	if dt, ok := params["start_time"]; ok && dt != "" {
		startTime = parseDateTime(dt)
	}
	if dt, ok := params["end_time"]; ok && dt != "" {
		endTime = parseDateTime(dt)
	}

	event, err := h.CreateEvent(ctx, msg.From.ID, title, description, startTime, endTime, 30, tags)
	if err != nil {
		result := "建立事件失敗，請稍後再試"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("📅 事件已建立 (ID: %d)\n標題: %s", event.EventID, title)
	if startTime != nil {
		result += fmt.Sprintf("\n開始時間: %s", startTime.Format("2006-01-02 15:04"))
	}
	h.sendMessage(msg.Chat.ID, result)
	return result
}

// Delete handlers

func (h *Handlers) handleAIDeleteMemo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的備忘錄編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if err := h.repos.Memo.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除備忘錄失敗，請確認編號是否正確"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("🗑️ 備忘錄 #%d 已刪除", id)
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIDeleteTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的待辦事項編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if err := h.repos.Todo.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除待辦事項失敗，請確認編號是否正確"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("🗑️ 待辦事項 #%d 已刪除", id)
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIUpdateTodo(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的待辦事項編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	todo, err := h.repos.Todo.GetByID(ctx, id, msg.From.ID)
	if err != nil {
		result := "找不到該待辦事項"
		h.sendMessage(msg.Chat.ID, result)
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
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("✏️ 待辦事項 #%d 已更新", id)
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIDeleteReminder(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的提醒編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if err := h.repos.Reminder.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除提醒失敗，請確認編號是否正確"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("🗑️ 提醒 #%d 已刪除", id)
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIDeleteTransaction(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的交易記錄編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if err := h.repos.Transaction.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除交易記錄失敗，請確認編號是否正確"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("🗑️ 交易記錄 #%d 已刪除", id)
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIDeleteEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的事件編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	if err := h.repos.Event.Delete(ctx, id, msg.From.ID); err != nil {
		result := "刪除事件失敗，請確認編號是否正確"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("🗑️ 事件 #%d 已刪除", id)
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) handleAIUpdateEvent(ctx context.Context, msg *tgbotapi.Message, params map[string]string) string {
	id, err := strconv.Atoi(params["id"])
	if err != nil {
		result := "請提供有效的事件編號"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	event, err := h.repos.Event.GetByID(ctx, id, msg.From.ID)
	if err != nil {
		result := "找不到該事件"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	// Update fields if provided
	if title, ok := params["title"]; ok && title != "" {
		event.Title = title
	}
	if desc, ok := params["description"]; ok {
		event.Description = desc
	}
	if dt, ok := params["start_time"]; ok && dt != "" {
		event.StartTime = parseDateTime(dt)
	}
	if dt, ok := params["end_time"]; ok && dt != "" {
		event.EndTime = parseDateTime(dt)
	}
	if tags, ok := params["tags"]; ok {
		event.Tags = tags
	}

	if err := h.repos.Event.Update(ctx, event); err != nil {
		result := "更新事件失敗"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	result := fmt.Sprintf("✏️ 事件 #%d 已更新", id)
	h.sendMessage(msg.Chat.ID, result)
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

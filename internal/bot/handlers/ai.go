package handlers

import (
	"context"
	"fmt"
	"log"
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

	h.debug("Incoming message", "from", msg.From.FirstName, "username", msg.From.UserName, "text", msg.Text)
	if msg.ReplyToMessage != nil {
		h.debug("ReplyToMessage", "text", msg.ReplyToMessage.Text)
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
			h.debug("Added ReplyToMessage to context as assistant message")
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

	h.debug("Conversation history", "count", len(session.History))
	for i, m := range session.History {
		h.debug("History item", "index", i, "role", m.Role, "content", truncateString(m.Content, 100))
	}

	// Parse intent with conversation history
	intent, err := h.ai.ParseIntentWithHistory(ctx, session.History)
	if err != nil {
		log.Printf("Failed to parse intent: %v", err)
		h.sendMessage(msg.Chat.ID, "抱歉，我無法理解你的訊息。請試著用更清楚的方式描述，或使用 /help 查看可用指令。")
		return
	}

	h.debug("Parsed intent",
		"action", intent.Action,
		"entity", intent.Entity,
		"confidence", intent.Confidence,
		"needs_confirmation", intent.NeedsConfirmation,
		"need_more_info", intent.NeedMoreInfo,
		"return_result_to_ai", intent.ReturnResultToAI,
		"params", intent.Parameters,
		"ai_message", intent.AIMessage,
		"raw", intent.RawResponse)

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

	// Handle return_result_to_ai flow: execute tool and let AI process the result
	if intent.ReturnResultToAI {
		h.debug("ReturnResultToAI flow", "action", intent.Action)

		// Execute but don't send to user
		result := h.executeIntentWithResult(ctx, msg, intent)
		h.debug("Tool result", "result", truncateString(result, 200))

		// Add result to history for AI to process
		session.History = append(session.History, ai.Message{
			Role:    "assistant",
			Content: "[工具執行結果]\n" + result,
		})
		h.saveSession(msg.From.ID, session)

		h.debug("Sending tool result to AI for next action")

		// Let AI decide next action based on result
		nextIntent, err := h.ai.ParseIntentWithHistory(ctx, session.History)
		if err != nil {
			log.Printf("Failed to parse next intent: %v", err)
			h.sendMessage(msg.Chat.ID, "處理失敗，請稍後再試")
			return
		}

		h.debug("Next intent after tool result",
			"action", nextIntent.Action,
			"entity", nextIntent.Entity,
			"confidence", nextIntent.Confidence,
			"needs_confirmation", nextIntent.NeedsConfirmation,
			"ai_message", truncateString(nextIntent.AIMessage, 100),
			"raw", nextIntent.RawResponse)

		// Process the next intent (but prevent infinite loop - nextIntent should not have ReturnResultToAI=true)
		if nextIntent.NeedsConfirmation {
			h.requestConfirmation(msg.Chat.ID, msg.From.ID, nextIntent)
			h.clearSession(msg.From.ID)
			return
		}

		// If AI just wants to send a message (unknown action with AIMessage)
		if nextIntent.Action == "unknown" && nextIntent.AIMessage != "" {
			h.sendMessage(msg.Chat.ID, nextIntent.AIMessage)
			h.clearSession(msg.From.ID)
			return
		}

		// Execute the next intent normally
		intent = nextIntent
	}

	// Execute intent and get result
	h.debug("Executing action", "action", intent.Action, "params", intent.Parameters)
	result := h.executeIntentWithResult(ctx, msg, intent)
	h.debug("Action result", "result", truncateString(result, 200))

	// Add execution result to history for AI to process
	if result != "" {
		session.History = append(session.History, ai.Message{
			Role:    "assistant",
			Content: result,
		})
	}

	// Clear session after successful action (unless it's a list/query action)
	if !strings.HasPrefix(intent.Action, "list_") && intent.Action != "get_balance" && intent.Action != "query_schedule" {
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
	h.debug("executeSingleAction", "action", action, "params", params, "sendMsg", sendMsg)
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
	case "query_schedule":
		result = h.handleQueryScheduleResult(ctx, msg, params, sendMsg)
	case "find_free_time":
		result = h.handleFindFreeTime(ctx, msg, params)
		if sendMsg {
			h.sendMessage(msg.Chat.ID, result)
		}
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

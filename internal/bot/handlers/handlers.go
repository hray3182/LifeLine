package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/ai"
	"github.com/hray3182/LifeLine/internal/repository"
)

type Repositories struct {
	User        *repository.UserRepository
	Memo        *repository.MemoRepository
	Todo        *repository.TodoRepository
	Reminder    *repository.ReminderRepository
	Category    *repository.CategoryRepository
	Transaction *repository.TransactionRepository
	Event       *repository.EventRepository
}

type Handlers struct {
	api     *tgbotapi.BotAPI
	repos   *Repositories
	ai      *ai.Client
	devMode bool
}

func New(api *tgbotapi.BotAPI, repos *Repositories, aiClient *ai.Client, devMode bool) *Handlers {
	return &Handlers{
		api:     api,
		repos:   repos,
		ai:      aiClient,
		devMode: devMode,
	}
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
	// Answer callback to remove loading state
	answer := tgbotapi.NewCallback(callback.ID, "")
	if _, err := h.api.Request(answer); err != nil {
		log.Printf("Failed to answer callback: %v", err)
	}

	// Parse callback data: "confirm:userID", "cancel:userID", or "option:userID:index"
	parts := strings.Split(callback.Data, ":")
	if len(parts) < 2 {
		return
	}

	action := parts[0]
	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}

	// Verify the callback is from the correct user
	if callback.From.ID != userID {
		h.answerCallbackWithAlert(callback.ID, "這不是你的操作")
		return
	}

	// Get pending confirmation
	pendingMutex.RLock()
	pending, exists := pendingConfirmations[userID]
	pendingMutex.RUnlock()

	if !exists || time.Now().After(pending.ExpiresAt) {
		if exists {
			pendingMutex.Lock()
			delete(pendingConfirmations, userID)
			pendingMutex.Unlock()
		}
		h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "⏰ 確認已過期")
		return
	}

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
		result := h.executeIntentWithResult(ctx, fakeMsg, pending.Intent)
		h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "✅ 已確認\n\n"+result)
	case "cancel":
		h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "❌ 已取消操作")
	case "option":
		// Parse option index
		if len(parts) != 3 {
			return
		}
		optionIndex, err := strconv.Atoi(parts[2])
		if err != nil || optionIndex < 0 || optionIndex >= len(pending.Intent.ConfirmationOptions) {
			h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "❌ 無效的選項")
			return
		}

		// Get selected option and merge parameters
		selectedOption := pending.Intent.ConfirmationOptions[optionIndex]
		for key, value := range selectedOption.Parameters {
			pending.Intent.Parameters[key] = value
		}

		result := h.executeIntentWithResult(ctx, fakeMsg, pending.Intent)
		h.editMessageText(callback.Message.Chat.ID, callback.Message.MessageID, fmt.Sprintf("✅ 已選擇「%s」\n\n%s", selectedOption.Label, result))
	}
}

func (h *Handlers) answerCallbackWithAlert(callbackID string, text string) {
	answer := tgbotapi.NewCallbackWithAlert(callbackID, text)
	if _, err := h.api.Request(answer); err != nil {
		log.Printf("Failed to answer callback with alert: %v", err)
	}
}

func (h *Handlers) editMessageText(chatID int64, messageID int, text string) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "Markdown"
	if _, err := h.api.Send(edit); err != nil {
		log.Printf("Failed to edit message: %v", err)
	}
}

func (h *Handlers) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := h.api.Send(msg); err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}

func (h *Handlers) handleStart(ctx context.Context, msg *tgbotapi.Message) {
	text := fmt.Sprintf(`👋 你好 %s！

我是 LifeLine，你的個人生活助理機器人。

我可以幫你：
📝 管理備忘錄
✅ 追蹤待辦事項
⏰ 設定提醒
💰 記錄收支
📅 管理行事曆

你可以直接用自然語言告訴我你想做什麼，例如：
• "記一下明天要開會"
• "幫我記帳 午餐 150 元"
• "提醒我下午 3 點喝水"

使用 /help 查看所有指令`, msg.From.FirstName)
	h.sendMessage(msg.Chat.ID, text)
}

func (h *Handlers) handleHelp(ctx context.Context, msg *tgbotapi.Message) {
	text := `📖 *指令列表*

*備忘錄*
/memo <內容> - 新增備忘錄
/memos - 查看備忘錄列表

*待辦事項*
/todo <標題> - 新增待辦
/todos - 查看待辦列表
/done <編號> - 完成待辦

*提醒*
/remind <時間> <訊息> - 設定提醒
/reminders - 查看提醒列表

*記帳*
/expense <金額> <說明> - 記錄支出
/income <金額> <說明> - 記錄收入
/balance - 查看收支統計

*行事曆*
/event <標題> <時間> - 新增事件
/events - 查看近期事件

💡 你也可以直接用自然語言告訴我！`
	h.sendMessage(msg.Chat.ID, text)
}

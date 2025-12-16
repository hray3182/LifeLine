package handlers

import (
	"context"
	"fmt"
	"log"

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
	api   *tgbotapi.BotAPI
	repos *Repositories
	ai    *ai.Client
}

func New(api *tgbotapi.BotAPI, repos *Repositories, aiClient *ai.Client) *Handlers {
	return &Handlers{
		api:   api,
		repos: repos,
		ai:    aiClient,
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

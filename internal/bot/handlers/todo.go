package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
)

func (h *Handlers) handleTodo(ctx context.Context, msg *tgbotapi.Message) {
	title := strings.TrimSpace(msg.CommandArguments())
	if title == "" {
		h.sendMessage(msg.Chat.ID, "請提供待辦事項標題\n用法: /todo <標題>")
		return
	}

	todo := &models.Todo{
		UserID: msg.From.ID,
		Title:  title,
	}

	if err := h.repos.Todo.Create(ctx, todo); err != nil {
		h.sendMessage(msg.Chat.ID, "建立待辦事項失敗，請稍後再試")
		return
	}

	h.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ 待辦事項已建立 (ID: %d)", todo.TodoID))
}

func (h *Handlers) handleTodoList(ctx context.Context, msg *tgbotapi.Message) {
	todos, err := h.repos.Todo.GetByUserID(ctx, msg.From.ID, false)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "取得待辦事項失敗，請稍後再試")
		return
	}

	if len(todos) == 0 {
		h.sendMessage(msg.Chat.ID, "✅ 目前沒有待辦事項")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 **待辦事項列表**\n\n")
	for _, todo := range todos {
		status := "⬜"
		if todo.IsCompleted() {
			status = "✅"
		}

		title := todo.Title
		if len(title) > 40 {
			title = title[:40] + "..."
		}

		sb.WriteString(fmt.Sprintf("%s **%d.** %s", status, todo.TodoID, title))

		if todo.DueTime != nil {
			sb.WriteString(fmt.Sprintf("\n   📅 %s", todo.DueTime.Format("2006-01-02 15:04")))
		}
		if todo.Priority > 0 {
			sb.WriteString(fmt.Sprintf(" | 優先級: %d", todo.Priority))
		}
		sb.WriteString("\n\n")
	}

	h.sendMessage(msg.Chat.ID, sb.String())
}

func (h *Handlers) handleTodoDone(ctx context.Context, msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		h.sendMessage(msg.Chat.ID, "請提供待辦事項編號\n用法: /done <編號>")
		return
	}

	todoID, err := strconv.Atoi(args)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "無效的編號")
		return
	}

	if err := h.repos.Todo.Complete(ctx, todoID, msg.From.ID); err != nil {
		h.sendMessage(msg.Chat.ID, "完成待辦事項失敗，請確認編號是否正確")
		return
	}

	h.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ 待辦事項 #%d 已完成！", todoID))
}

func (h *Handlers) CreateTodo(ctx context.Context, userID int64, title, description string, priority int, dueTime *time.Time, tags string) (*models.Todo, error) {
	todo := &models.Todo{
		UserID:      userID,
		Title:       title,
		Description: description,
		Priority:    priority,
		DueTime:     dueTime,
		Tags:        tags,
	}
	err := h.repos.Todo.Create(ctx, todo)
	return todo, err
}

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

	result := fmt.Sprintf("待辦事項已建立 (ID: %d)\n標題: %s\n優先級: %d", todo.TodoID, title, todo.Priority)
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

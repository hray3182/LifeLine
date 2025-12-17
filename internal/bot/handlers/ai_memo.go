package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
)

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

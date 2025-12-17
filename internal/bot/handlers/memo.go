package handlers

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
)

func (h *Handlers) handleMemo(ctx context.Context, msg *tgbotapi.Message) {
	content := strings.TrimSpace(msg.CommandArguments())
	if content == "" {
		h.sendMessage(msg.Chat.ID, "請提供備忘錄內容\n用法: /memo <內容>")
		return
	}

	memo := &models.Memo{
		UserID:  msg.From.ID,
		Content: content,
	}

	if err := h.repos.Memo.Create(ctx, memo); err != nil {
		h.sendMessage(msg.Chat.ID, "建立備忘錄失敗，請稍後再試")
		return
	}

	h.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ 備忘錄已建立 (ID: %d)", memo.MemoID))
}

func (h *Handlers) handleMemoList(ctx context.Context, msg *tgbotapi.Message) {
	memos, err := h.repos.Memo.GetByUserID(ctx, msg.From.ID, 10, 0)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "取得備忘錄失敗，請稍後再試")
		return
	}

	if len(memos) == 0 {
		h.sendMessage(msg.Chat.ID, "📝 目前沒有備忘錄")
		return
	}

	var sb strings.Builder
	sb.WriteString("📝 **備忘錄列表**\n\n")
	for _, memo := range memos {
		content := memo.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		sb.WriteString(fmt.Sprintf("**%d.** %s\n", memo.MemoID, content))
		sb.WriteString(fmt.Sprintf("   _建立於 %s_\n\n", memo.CreatedAt.Format("2006-01-02 15:04")))
	}

	h.sendMessage(msg.Chat.ID, sb.String())
}

func (h *Handlers) CreateMemo(ctx context.Context, userID int64, content string, tags string) (*models.Memo, error) {
	memo := &models.Memo{
		UserID:  userID,
		Content: content,
		Tags:    tags,
	}
	err := h.repos.Memo.Create(ctx, memo)
	return memo, err
}

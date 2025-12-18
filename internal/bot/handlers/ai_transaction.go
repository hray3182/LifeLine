package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/models"
)

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

		sb.WriteString(fmt.Sprintf("[%s] %d. %.0f", typeStr, tx.TransactionID, tx.Amount))
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

	result := fmt.Sprintf("%s已記錄 (ID: %d)\n金額: %.0f", typeStr, tx.TransactionID, amount)
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

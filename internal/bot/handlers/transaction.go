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

func (h *Handlers) handleExpense(ctx context.Context, msg *tgbotapi.Message) {
	h.handleTransaction(ctx, msg, models.TransactionTypeExpense)
}

func (h *Handlers) handleIncome(ctx context.Context, msg *tgbotapi.Message) {
	h.handleTransaction(ctx, msg, models.TransactionTypeIncome)
}

func (h *Handlers) handleTransaction(ctx context.Context, msg *tgbotapi.Message, txType models.TransactionType) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		typeStr := "支出"
		cmd := "expense"
		if txType == models.TransactionTypeIncome {
			typeStr = "收入"
			cmd = "income"
		}
		h.sendMessage(msg.Chat.ID, fmt.Sprintf("請提供金額和說明\n用法: /%s <金額> <說明>", cmd))
		_ = typeStr
		return
	}

	parts := strings.SplitN(args, " ", 2)
	amount, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "無效的金額")
		return
	}

	description := ""
	if len(parts) > 1 {
		description = parts[1]
	}

	now := time.Now()
	tx := &models.Transaction{
		UserID:          msg.From.ID,
		Type:            txType,
		Amount:          amount,
		Description:     description,
		TransactionDate: &now,
	}

	if err := h.repos.Transaction.Create(ctx, tx); err != nil {
		h.sendMessage(msg.Chat.ID, "記錄失敗，請稍後再試")
		return
	}

	emoji := "💸"
	typeStr := "支出"
	if txType == models.TransactionTypeIncome {
		emoji = "💰"
		typeStr = "收入"
	}

	h.sendMessage(msg.Chat.ID, fmt.Sprintf("%s %s已記錄\n金額: %.0f\n說明: %s",
		emoji, typeStr, amount, description))
}

func (h *Handlers) handleBalance(ctx context.Context, msg *tgbotapi.Message) {
	h.handleBalanceWithResult(ctx, msg)
}

func (h *Handlers) handleBalanceWithResult(ctx context.Context, msg *tgbotapi.Message) string {
	// Get this month's summary
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

	income, err := h.repos.Transaction.GetTotalByType(ctx, msg.From.ID, startOfMonth, endOfMonth, models.TransactionTypeIncome)
	if err != nil {
		result := "取得統計失敗，請稍後再試"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	expense, err := h.repos.Transaction.GetTotalByType(ctx, msg.From.ID, startOfMonth, endOfMonth, models.TransactionTypeExpense)
	if err != nil {
		result := "取得統計失敗，請稍後再試"
		h.sendMessage(msg.Chat.ID, result)
		return result
	}

	balance := income - expense

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 **%d年%d月 收支統計**\n\n", now.Year(), now.Month()))
	sb.WriteString(fmt.Sprintf("💰 收入: %.0f\n", income))
	sb.WriteString(fmt.Sprintf("💸 支出: %.0f\n", expense))
	sb.WriteString(fmt.Sprintf("━━━━━━━━━━\n"))

	balanceEmoji := "📈"
	if balance < 0 {
		balanceEmoji = "📉"
	}
	sb.WriteString(fmt.Sprintf("%s 結餘: %.0f", balanceEmoji, balance))

	result := sb.String()
	h.sendMessage(msg.Chat.ID, result)
	return result
}

func (h *Handlers) CreateTransaction(ctx context.Context, userID int64, txType models.TransactionType, amount float64, description string, categoryName string, date *time.Time) (*models.Transaction, error) {
	var categoryID *int
	if categoryName != "" {
		cat, err := h.repos.Category.GetOrCreateByName(ctx, userID, categoryName)
		if err == nil {
			categoryID = &cat.CategoryID
			h.repos.Category.IncrementUsage(ctx, cat.CategoryID)
		}
	}

	if date == nil {
		now := time.Now()
		date = &now
	}

	tx := &models.Transaction{
		UserID:          userID,
		CategoryID:      categoryID,
		Type:            txType,
		Amount:          amount,
		Description:     description,
		TransactionDate: date,
	}
	err := h.repos.Transaction.Create(ctx, tx)
	return tx, err
}

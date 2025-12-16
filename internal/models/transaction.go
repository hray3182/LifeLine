package models

import "time"

type TransactionType string

const (
	TransactionTypeIncome  TransactionType = "income"
	TransactionTypeExpense TransactionType = "expense"
)

type Transaction struct {
	TransactionID   int             `json:"transaction_id"`
	UserID          int64           `json:"user_id"`
	CategoryID      *int            `json:"category_id"`
	Type            TransactionType `json:"type"`
	Amount          float64         `json:"amount"`
	Description     string          `json:"description"`
	TransactionDate *time.Time      `json:"transaction_date"`
	Tags            string          `json:"tags"`
	RecurrenceRule  string          `json:"recurrence_rule"`
	Frequency       string          `json:"frequency"`
	Interval        int             `json:"interval"`
	ByDay           string          `json:"by_day"`
	Until           *time.Time      `json:"until"`
	CreatedAt       time.Time       `json:"created_at"`
}

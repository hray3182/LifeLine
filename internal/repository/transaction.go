package repository

import (
	"context"
	"time"

	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/models"
)

type TransactionRepository struct {
	db *database.DB
}

func NewTransactionRepository(db *database.DB) *TransactionRepository {
	return &TransactionRepository{db: db}
}

func (r *TransactionRepository) Create(ctx context.Context, tx *models.Transaction) error {
	return r.db.Pool.QueryRow(ctx,
		`INSERT INTO transaction (user_id, category_id, type, amount, description, transaction_date, tags,
		 recurrence_rule, frequency, interval, by_day, until)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING transaction_id, created_at`,
		tx.UserID, tx.CategoryID, tx.Type, tx.Amount, tx.Description, tx.TransactionDate, tx.Tags,
		tx.RecurrenceRule, tx.Frequency, tx.Interval, tx.ByDay, tx.Until,
	).Scan(&tx.TransactionID, &tx.CreatedAt)
}

func (r *TransactionRepository) GetByUserID(ctx context.Context, userID int64, limit, offset int) ([]*models.Transaction, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT transaction_id, user_id, category_id, type, amount, description, transaction_date, tags,
		 recurrence_rule, frequency, interval, by_day, until, created_at
		 FROM transaction WHERE user_id = $1
		 ORDER BY transaction_date DESC NULLS LAST, created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

func (r *TransactionRepository) GetByID(ctx context.Context, transactionID int, userID int64) (*models.Transaction, error) {
	tx := &models.Transaction{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT transaction_id, user_id, category_id, type, amount, description, transaction_date, tags,
		 recurrence_rule, frequency, interval, by_day, until, created_at
		 FROM transaction WHERE transaction_id = $1 AND user_id = $2`,
		transactionID, userID,
	).Scan(&tx.TransactionID, &tx.UserID, &tx.CategoryID, &tx.Type, &tx.Amount, &tx.Description,
		&tx.TransactionDate, &tx.Tags, &tx.RecurrenceRule, &tx.Frequency, &tx.Interval, &tx.ByDay, &tx.Until, &tx.CreatedAt)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (r *TransactionRepository) GetByDateRange(ctx context.Context, userID int64, start, end time.Time) ([]*models.Transaction, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT transaction_id, user_id, category_id, type, amount, description, transaction_date, tags,
		 recurrence_rule, frequency, interval, by_day, until, created_at
		 FROM transaction WHERE user_id = $1 AND transaction_date >= $2 AND transaction_date <= $3
		 ORDER BY transaction_date DESC`,
		userID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

func (r *TransactionRepository) Update(ctx context.Context, tx *models.Transaction) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE transaction SET category_id = $1, type = $2, amount = $3, description = $4,
		 transaction_date = $5, tags = $6, recurrence_rule = $7, frequency = $8, interval = $9, by_day = $10, until = $11
		 WHERE transaction_id = $12 AND user_id = $13`,
		tx.CategoryID, tx.Type, tx.Amount, tx.Description, tx.TransactionDate, tx.Tags,
		tx.RecurrenceRule, tx.Frequency, tx.Interval, tx.ByDay, tx.Until, tx.TransactionID, tx.UserID,
	)
	return err
}

func (r *TransactionRepository) Delete(ctx context.Context, transactionID int, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`DELETE FROM transaction WHERE transaction_id = $1 AND user_id = $2`,
		transactionID, userID,
	)
	return err
}

func (r *TransactionRepository) GetSummaryByCategory(ctx context.Context, userID int64, start, end time.Time, txType models.TransactionType) (map[int]float64, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT category_id, SUM(amount) as total
		 FROM transaction
		 WHERE user_id = $1 AND type = $2 AND transaction_date >= $3 AND transaction_date <= $4
		 GROUP BY category_id`,
		userID, txType, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := make(map[int]float64)
	for rows.Next() {
		var categoryID *int
		var total float64
		if err := rows.Scan(&categoryID, &total); err != nil {
			return nil, err
		}
		if categoryID != nil {
			summary[*categoryID] = total
		}
	}
	return summary, nil
}

func (r *TransactionRepository) GetTotalByType(ctx context.Context, userID int64, start, end time.Time, txType models.TransactionType) (float64, error) {
	var total float64
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)
		 FROM transaction
		 WHERE user_id = $1 AND type = $2 AND transaction_date >= $3 AND transaction_date <= $4`,
		userID, txType, start, end,
	).Scan(&total)
	return total, err
}

func (r *TransactionRepository) Search(ctx context.Context, userID int64, keyword string) ([]*models.Transaction, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT transaction_id, user_id, category_id, type, amount, description, transaction_date, tags,
		 recurrence_rule, frequency, interval, by_day, until, created_at
		 FROM transaction WHERE user_id = $1 AND (description ILIKE $2 OR tags ILIKE $2)
		 ORDER BY transaction_date DESC NULLS LAST, created_at DESC`,
		userID, "%"+keyword+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

func (r *TransactionRepository) scanTransactions(rows interface {
	Next() bool
	Scan(dest ...any) error
}) ([]*models.Transaction, error) {
	var transactions []*models.Transaction
	for rows.Next() {
		tx := &models.Transaction{}
		if err := rows.Scan(&tx.TransactionID, &tx.UserID, &tx.CategoryID, &tx.Type, &tx.Amount,
			&tx.Description, &tx.TransactionDate, &tx.Tags, &tx.RecurrenceRule, &tx.Frequency,
			&tx.Interval, &tx.ByDay, &tx.Until, &tx.CreatedAt); err != nil {
			return nil, err
		}
		transactions = append(transactions, tx)
	}
	return transactions, nil
}

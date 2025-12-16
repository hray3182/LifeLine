package repository

import (
	"context"

	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/models"
)

type MemoRepository struct {
	db *database.DB
}

func NewMemoRepository(db *database.DB) *MemoRepository {
	return &MemoRepository{db: db}
}

func (r *MemoRepository) Create(ctx context.Context, memo *models.Memo) error {
	return r.db.Pool.QueryRow(ctx,
		`INSERT INTO memo (user_id, content, tags) VALUES ($1, $2, $3)
		 RETURNING memo_id, created_at`,
		memo.UserID, memo.Content, memo.Tags,
	).Scan(&memo.MemoID, &memo.CreatedAt)
}

func (r *MemoRepository) GetByUserID(ctx context.Context, userID int64, limit, offset int) ([]*models.Memo, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT memo_id, user_id, content, tags, created_at
		 FROM memo WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memos []*models.Memo
	for rows.Next() {
		memo := &models.Memo{}
		if err := rows.Scan(&memo.MemoID, &memo.UserID, &memo.Content, &memo.Tags, &memo.CreatedAt); err != nil {
			return nil, err
		}
		memos = append(memos, memo)
	}
	return memos, nil
}

func (r *MemoRepository) GetByID(ctx context.Context, memoID int, userID int64) (*models.Memo, error) {
	memo := &models.Memo{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT memo_id, user_id, content, tags, created_at
		 FROM memo WHERE memo_id = $1 AND user_id = $2`,
		memoID, userID,
	).Scan(&memo.MemoID, &memo.UserID, &memo.Content, &memo.Tags, &memo.CreatedAt)
	if err != nil {
		return nil, err
	}
	return memo, nil
}

func (r *MemoRepository) Update(ctx context.Context, memo *models.Memo) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE memo SET content = $1, tags = $2 WHERE memo_id = $3 AND user_id = $4`,
		memo.Content, memo.Tags, memo.MemoID, memo.UserID,
	)
	return err
}

func (r *MemoRepository) Delete(ctx context.Context, memoID int, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`DELETE FROM memo WHERE memo_id = $1 AND user_id = $2`,
		memoID, userID,
	)
	return err
}

func (r *MemoRepository) Search(ctx context.Context, userID int64, keyword string) ([]*models.Memo, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT memo_id, user_id, content, tags, created_at
		 FROM memo WHERE user_id = $1 AND (content ILIKE $2 OR tags ILIKE $2)
		 ORDER BY created_at DESC`,
		userID, "%"+keyword+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memos []*models.Memo
	for rows.Next() {
		memo := &models.Memo{}
		if err := rows.Scan(&memo.MemoID, &memo.UserID, &memo.Content, &memo.Tags, &memo.CreatedAt); err != nil {
			return nil, err
		}
		memos = append(memos, memo)
	}
	return memos, nil
}

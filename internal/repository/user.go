package repository

import (
	"context"

	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/models"
)

type UserRepository struct {
	db *database.DB
}

func NewUserRepository(db *database.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetOrCreate(ctx context.Context, userID int64, userName string) (*models.User, error) {
	user := &models.User{}
	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO "user" (user_id, user_name) VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET user_name = EXCLUDED.user_name
		 RETURNING user_id, user_name`,
		userID, userName,
	).Scan(&user.UserID, &user.UserName)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, userID int64) (*models.User, error) {
	user := &models.User{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT user_id, user_name FROM "user" WHERE user_id = $1`,
		userID,
	).Scan(&user.UserID, &user.UserName)
	if err != nil {
		return nil, err
	}
	return user, nil
}

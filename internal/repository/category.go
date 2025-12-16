package repository

import (
	"context"

	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/models"
)

type CategoryRepository struct {
	db *database.DB
}

func NewCategoryRepository(db *database.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) Create(ctx context.Context, category *models.Category) error {
	return r.db.Pool.QueryRow(ctx,
		`INSERT INTO category (user_id, category_name, usage_count) VALUES ($1, $2, $3)
		 RETURNING category_id`,
		category.UserID, category.CategoryName, category.UsageCount,
	).Scan(&category.CategoryID)
}

func (r *CategoryRepository) GetByUserID(ctx context.Context, userID int64) ([]*models.Category, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT category_id, user_id, category_name, usage_count
		 FROM category WHERE user_id = $1 ORDER BY usage_count DESC, category_name ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []*models.Category
	for rows.Next() {
		cat := &models.Category{}
		if err := rows.Scan(&cat.CategoryID, &cat.UserID, &cat.CategoryName, &cat.UsageCount); err != nil {
			return nil, err
		}
		categories = append(categories, cat)
	}
	return categories, nil
}

func (r *CategoryRepository) GetByID(ctx context.Context, categoryID int, userID int64) (*models.Category, error) {
	cat := &models.Category{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT category_id, user_id, category_name, usage_count
		 FROM category WHERE category_id = $1 AND user_id = $2`,
		categoryID, userID,
	).Scan(&cat.CategoryID, &cat.UserID, &cat.CategoryName, &cat.UsageCount)
	if err != nil {
		return nil, err
	}
	return cat, nil
}

func (r *CategoryRepository) Update(ctx context.Context, category *models.Category) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE category SET category_name = $1 WHERE category_id = $2 AND user_id = $3`,
		category.CategoryName, category.CategoryID, category.UserID,
	)
	return err
}

func (r *CategoryRepository) Delete(ctx context.Context, categoryID int, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`DELETE FROM category WHERE category_id = $1 AND user_id = $2`,
		categoryID, userID,
	)
	return err
}

func (r *CategoryRepository) IncrementUsage(ctx context.Context, categoryID int) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE category SET usage_count = usage_count + 1 WHERE category_id = $1`,
		categoryID,
	)
	return err
}

func (r *CategoryRepository) GetOrCreateByName(ctx context.Context, userID int64, name string) (*models.Category, error) {
	cat := &models.Category{}
	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO category (user_id, category_name, usage_count)
		 VALUES ($1, $2, 0)
		 ON CONFLICT DO NOTHING
		 RETURNING category_id, user_id, category_name, usage_count`,
		userID, name,
	).Scan(&cat.CategoryID, &cat.UserID, &cat.CategoryName, &cat.UsageCount)

	if err != nil {
		// Category already exists, fetch it
		err = r.db.Pool.QueryRow(ctx,
			`SELECT category_id, user_id, category_name, usage_count
			 FROM category WHERE user_id = $1 AND category_name = $2`,
			userID, name,
		).Scan(&cat.CategoryID, &cat.UserID, &cat.CategoryName, &cat.UsageCount)
		if err != nil {
			return nil, err
		}
	}
	return cat, nil
}

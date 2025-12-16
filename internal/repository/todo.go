package repository

import (
	"context"
	"time"

	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/models"
)

type TodoRepository struct {
	db *database.DB
}

func NewTodoRepository(db *database.DB) *TodoRepository {
	return &TodoRepository{db: db}
}

func (r *TodoRepository) Create(ctx context.Context, todo *models.Todo) error {
	return r.db.Pool.QueryRow(ctx,
		`INSERT INTO todo (user_id, title, priority, description, due_time, tags)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING todo_id, created_at`,
		todo.UserID, todo.Title, todo.Priority, todo.Description, todo.DueTime, todo.Tags,
	).Scan(&todo.TodoID, &todo.CreatedAt)
}

func (r *TodoRepository) GetByUserID(ctx context.Context, userID int64, includeCompleted bool) ([]*models.Todo, error) {
	query := `SELECT todo_id, user_id, title, priority, description, due_time, completed_at, tags, created_at
		 FROM todo WHERE user_id = $1`
	if !includeCompleted {
		query += ` AND completed_at IS NULL`
	}
	query += ` ORDER BY priority DESC, due_time ASC NULLS LAST, created_at DESC`

	rows, err := r.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []*models.Todo
	for rows.Next() {
		todo := &models.Todo{}
		if err := rows.Scan(&todo.TodoID, &todo.UserID, &todo.Title, &todo.Priority,
			&todo.Description, &todo.DueTime, &todo.CompletedAt, &todo.Tags, &todo.CreatedAt); err != nil {
			return nil, err
		}
		todos = append(todos, todo)
	}
	return todos, nil
}

func (r *TodoRepository) GetByID(ctx context.Context, todoID int, userID int64) (*models.Todo, error) {
	todo := &models.Todo{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT todo_id, user_id, title, priority, description, due_time, completed_at, tags, created_at
		 FROM todo WHERE todo_id = $1 AND user_id = $2`,
		todoID, userID,
	).Scan(&todo.TodoID, &todo.UserID, &todo.Title, &todo.Priority,
		&todo.Description, &todo.DueTime, &todo.CompletedAt, &todo.Tags, &todo.CreatedAt)
	if err != nil {
		return nil, err
	}
	return todo, nil
}

func (r *TodoRepository) Update(ctx context.Context, todo *models.Todo) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE todo SET title = $1, priority = $2, description = $3, due_time = $4, tags = $5
		 WHERE todo_id = $6 AND user_id = $7`,
		todo.Title, todo.Priority, todo.Description, todo.DueTime, todo.Tags, todo.TodoID, todo.UserID,
	)
	return err
}

func (r *TodoRepository) Complete(ctx context.Context, todoID int, userID int64) error {
	now := time.Now()
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE todo SET completed_at = $1 WHERE todo_id = $2 AND user_id = $3`,
		now, todoID, userID,
	)
	return err
}

func (r *TodoRepository) Uncomplete(ctx context.Context, todoID int, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE todo SET completed_at = NULL WHERE todo_id = $1 AND user_id = $2`,
		todoID, userID,
	)
	return err
}

func (r *TodoRepository) Delete(ctx context.Context, todoID int, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`DELETE FROM todo WHERE todo_id = $1 AND user_id = $2`,
		todoID, userID,
	)
	return err
}

func (r *TodoRepository) GetDueSoon(ctx context.Context, userID int64, within time.Duration) ([]*models.Todo, error) {
	deadline := time.Now().Add(within)
	rows, err := r.db.Pool.Query(ctx,
		`SELECT todo_id, user_id, title, priority, description, due_time, completed_at, tags, created_at
		 FROM todo WHERE user_id = $1 AND completed_at IS NULL AND due_time IS NOT NULL AND due_time <= $2
		 ORDER BY due_time ASC`,
		userID, deadline,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []*models.Todo
	for rows.Next() {
		todo := &models.Todo{}
		if err := rows.Scan(&todo.TodoID, &todo.UserID, &todo.Title, &todo.Priority,
			&todo.Description, &todo.DueTime, &todo.CompletedAt, &todo.Tags, &todo.CreatedAt); err != nil {
			return nil, err
		}
		todos = append(todos, todo)
	}
	return todos, nil
}

func (r *TodoRepository) Search(ctx context.Context, userID int64, keyword string, includeCompleted bool) ([]*models.Todo, error) {
	query := `SELECT todo_id, user_id, title, priority, description, due_time, completed_at, tags, created_at
		 FROM todo WHERE user_id = $1 AND (title ILIKE $2 OR description ILIKE $2 OR tags ILIKE $2)`
	if !includeCompleted {
		query += ` AND completed_at IS NULL`
	}
	query += ` ORDER BY priority DESC, due_time ASC NULLS LAST, created_at DESC`

	rows, err := r.db.Pool.Query(ctx, query, userID, "%"+keyword+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []*models.Todo
	for rows.Next() {
		todo := &models.Todo{}
		if err := rows.Scan(&todo.TodoID, &todo.UserID, &todo.Title, &todo.Priority,
			&todo.Description, &todo.DueTime, &todo.CompletedAt, &todo.Tags, &todo.CreatedAt); err != nil {
			return nil, err
		}
		todos = append(todos, todo)
	}
	return todos, nil
}

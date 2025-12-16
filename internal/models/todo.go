package models

import "time"

type Todo struct {
	TodoID      int        `json:"todo_id"`
	UserID      int64      `json:"user_id"`
	Title       string     `json:"title"`
	Priority    int        `json:"priority"`
	Description string     `json:"description"`
	DueTime     *time.Time `json:"due_time"`
	CompletedAt *time.Time `json:"completed_at"`
	Tags        string     `json:"tags"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (t *Todo) IsCompleted() bool {
	return t.CompletedAt != nil
}

package models

import "time"

type Memo struct {
	MemoID    int       `json:"memo_id"`
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	Tags      string    `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

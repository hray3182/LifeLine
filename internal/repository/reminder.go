package repository

import (
	"context"
	"time"

	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/models"
)

type ReminderRepository struct {
	db *database.DB
}

func NewReminderRepository(db *database.DB) *ReminderRepository {
	return &ReminderRepository{db: db}
}

func (r *ReminderRepository) Create(ctx context.Context, reminder *models.Reminder) error {
	return r.db.Pool.QueryRow(ctx,
		`INSERT INTO reminders (user_id, enabled, recurrence_rule, dtstart, messages, remind_at, description, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING reminders_id, created_at`,
		reminder.UserID, reminder.Enabled, reminder.RecurrenceRule, reminder.Dtstart, reminder.Messages,
		reminder.RemindAt, reminder.Description, reminder.Tags,
	).Scan(&reminder.ReminderID, &reminder.CreatedAt)
}

func (r *ReminderRepository) GetByUserID(ctx context.Context, userID int64) ([]*models.Reminder, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT reminders_id, user_id, enabled, recurrence_rule, dtstart, messages, remind_at, description, tags, created_at
		 FROM reminders WHERE user_id = $1 ORDER BY remind_at ASC NULLS LAST`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []*models.Reminder
	for rows.Next() {
		reminder := &models.Reminder{}
		if err := rows.Scan(&reminder.ReminderID, &reminder.UserID, &reminder.Enabled, &reminder.RecurrenceRule,
			&reminder.Dtstart, &reminder.Messages, &reminder.RemindAt, &reminder.Description, &reminder.Tags, &reminder.CreatedAt); err != nil {
			return nil, err
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

func (r *ReminderRepository) GetByID(ctx context.Context, reminderID int, userID int64) (*models.Reminder, error) {
	reminder := &models.Reminder{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT reminders_id, user_id, enabled, recurrence_rule, dtstart, messages, remind_at, description, tags, created_at
		 FROM reminders WHERE reminders_id = $1 AND user_id = $2`,
		reminderID, userID,
	).Scan(&reminder.ReminderID, &reminder.UserID, &reminder.Enabled, &reminder.RecurrenceRule,
		&reminder.Dtstart, &reminder.Messages, &reminder.RemindAt, &reminder.Description, &reminder.Tags, &reminder.CreatedAt)
	if err != nil {
		return nil, err
	}
	return reminder, nil
}

func (r *ReminderRepository) Update(ctx context.Context, reminder *models.Reminder) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE reminders SET enabled = $1, recurrence_rule = $2, dtstart = $3, messages = $4, remind_at = $5, description = $6, tags = $7
		 WHERE reminders_id = $8 AND user_id = $9`,
		reminder.Enabled, reminder.RecurrenceRule, reminder.Dtstart, reminder.Messages, reminder.RemindAt,
		reminder.Description, reminder.Tags, reminder.ReminderID, reminder.UserID,
	)
	return err
}

func (r *ReminderRepository) UpdateRemindAt(ctx context.Context, reminderID int, remindAt *time.Time) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE reminders SET remind_at = $1 WHERE reminders_id = $2`,
		remindAt, reminderID,
	)
	return err
}

func (r *ReminderRepository) Delete(ctx context.Context, reminderID int, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`DELETE FROM reminders WHERE reminders_id = $1 AND user_id = $2`,
		reminderID, userID,
	)
	return err
}

func (r *ReminderRepository) GetPendingReminders(ctx context.Context, until time.Time) ([]*models.Reminder, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT reminders_id, user_id, enabled, recurrence_rule, dtstart, messages, remind_at, description, tags, created_at
		 FROM reminders WHERE enabled = true AND remind_at IS NOT NULL AND remind_at <= $1
		 ORDER BY remind_at ASC`,
		until,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []*models.Reminder
	for rows.Next() {
		reminder := &models.Reminder{}
		if err := rows.Scan(&reminder.ReminderID, &reminder.UserID, &reminder.Enabled, &reminder.RecurrenceRule,
			&reminder.Dtstart, &reminder.Messages, &reminder.RemindAt, &reminder.Description, &reminder.Tags, &reminder.CreatedAt); err != nil {
			return nil, err
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

func (r *ReminderRepository) SetEnabled(ctx context.Context, reminderID int, userID int64, enabled bool) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE reminders SET enabled = $1 WHERE reminders_id = $2 AND user_id = $3`,
		enabled, reminderID, userID,
	)
	return err
}

func (r *ReminderRepository) Search(ctx context.Context, userID int64, keyword string) ([]*models.Reminder, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT reminders_id, user_id, enabled, recurrence_rule, dtstart, messages, remind_at, description, tags, created_at
		 FROM reminders WHERE user_id = $1 AND (messages ILIKE $2 OR description ILIKE $2 OR tags ILIKE $2)
		 ORDER BY remind_at ASC NULLS LAST`,
		userID, "%"+keyword+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []*models.Reminder
	for rows.Next() {
		reminder := &models.Reminder{}
		if err := rows.Scan(&reminder.ReminderID, &reminder.UserID, &reminder.Enabled, &reminder.RecurrenceRule,
			&reminder.Dtstart, &reminder.Messages, &reminder.RemindAt, &reminder.Description, &reminder.Tags, &reminder.CreatedAt); err != nil {
			return nil, err
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

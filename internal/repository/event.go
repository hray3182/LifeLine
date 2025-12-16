package repository

import (
	"context"
	"time"

	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/models"
)

type EventRepository struct {
	db *database.DB
}

func NewEventRepository(db *database.DB) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) Create(ctx context.Context, event *models.Event) error {
	return r.db.Pool.QueryRow(ctx,
		`INSERT INTO event (user_id, title, description, start_time, end_time, notification_minutes,
		 recurrence_rule, frequency, interval, by_day, until, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING event_id, created_at`,
		event.UserID, event.Title, event.Description, event.StartTime, event.EndTime,
		event.NotificationMinutes, event.RecurrenceRule, event.Frequency, event.Interval,
		event.ByDay, event.Until, event.Tags,
	).Scan(&event.EventID, &event.CreatedAt)
}

func (r *EventRepository) GetByUserID(ctx context.Context, userID int64) ([]*models.Event, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, start_time, end_time, notification_minutes,
		 recurrence_rule, frequency, interval, by_day, until, tags, created_at
		 FROM event WHERE user_id = $1
		 ORDER BY start_time ASC NULLS LAST`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEvents(rows)
}

func (r *EventRepository) GetByID(ctx context.Context, eventID int, userID int64) (*models.Event, error) {
	event := &models.Event{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT event_id, user_id, title, description, start_time, end_time, notification_minutes,
		 recurrence_rule, frequency, interval, by_day, until, tags, created_at
		 FROM event WHERE event_id = $1 AND user_id = $2`,
		eventID, userID,
	).Scan(&event.EventID, &event.UserID, &event.Title, &event.Description, &event.StartTime,
		&event.EndTime, &event.NotificationMinutes, &event.RecurrenceRule, &event.Frequency,
		&event.Interval, &event.ByDay, &event.Until, &event.Tags, &event.CreatedAt)
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (r *EventRepository) GetByDateRange(ctx context.Context, userID int64, start, end time.Time) ([]*models.Event, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, start_time, end_time, notification_minutes,
		 recurrence_rule, frequency, interval, by_day, until, tags, created_at
		 FROM event WHERE user_id = $1 AND start_time >= $2 AND start_time <= $3
		 ORDER BY start_time ASC`,
		userID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEvents(rows)
}

func (r *EventRepository) GetUpcoming(ctx context.Context, userID int64, within time.Duration) ([]*models.Event, error) {
	now := time.Now()
	deadline := now.Add(within)
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, start_time, end_time, notification_minutes,
		 recurrence_rule, frequency, interval, by_day, until, tags, created_at
		 FROM event WHERE user_id = $1 AND start_time >= $2 AND start_time <= $3
		 ORDER BY start_time ASC`,
		userID, now, deadline,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEvents(rows)
}

func (r *EventRepository) Update(ctx context.Context, event *models.Event) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE event SET title = $1, description = $2, start_time = $3, end_time = $4,
		 notification_minutes = $5, recurrence_rule = $6, frequency = $7, interval = $8,
		 by_day = $9, until = $10, tags = $11
		 WHERE event_id = $12 AND user_id = $13`,
		event.Title, event.Description, event.StartTime, event.EndTime, event.NotificationMinutes,
		event.RecurrenceRule, event.Frequency, event.Interval, event.ByDay, event.Until, event.Tags,
		event.EventID, event.UserID,
	)
	return err
}

func (r *EventRepository) Delete(ctx context.Context, eventID int, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`DELETE FROM event WHERE event_id = $1 AND user_id = $2`,
		eventID, userID,
	)
	return err
}

func (r *EventRepository) GetPendingNotifications(ctx context.Context) ([]*models.Event, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, start_time, end_time, notification_minutes,
		 recurrence_rule, frequency, interval, by_day, until, tags, created_at
		 FROM event
		 WHERE start_time IS NOT NULL
		 AND start_time - (notification_minutes || ' minutes')::interval <= NOW()
		 AND start_time > NOW()
		 ORDER BY start_time ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEvents(rows)
}

func (r *EventRepository) Search(ctx context.Context, userID int64, keyword string) ([]*models.Event, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, start_time, end_time, notification_minutes,
		 recurrence_rule, frequency, interval, by_day, until, tags, created_at
		 FROM event WHERE user_id = $1 AND (title ILIKE $2 OR description ILIKE $2 OR tags ILIKE $2)
		 ORDER BY start_time ASC NULLS LAST`,
		userID, "%"+keyword+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEvents(rows)
}

func (r *EventRepository) scanEvents(rows interface {
	Next() bool
	Scan(dest ...any) error
}) ([]*models.Event, error) {
	var events []*models.Event
	for rows.Next() {
		event := &models.Event{}
		if err := rows.Scan(&event.EventID, &event.UserID, &event.Title, &event.Description,
			&event.StartTime, &event.EndTime, &event.NotificationMinutes, &event.RecurrenceRule,
			&event.Frequency, &event.Interval, &event.ByDay, &event.Until, &event.Tags, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

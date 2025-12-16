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
		`INSERT INTO event (user_id, title, description, dtstart, duration, next_occurrence,
		 notification_minutes, recurrence_rule, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING event_id, created_at`,
		event.UserID, event.Title, event.Description, event.Dtstart, event.Duration,
		event.NextOccurrence, event.NotificationMinutes, event.RecurrenceRule, event.Tags,
	).Scan(&event.EventID, &event.CreatedAt)
}

func (r *EventRepository) GetByUserID(ctx context.Context, userID int64) ([]*models.Event, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, dtstart, duration, next_occurrence,
		 notification_minutes, recurrence_rule, tags, created_at
		 FROM event WHERE user_id = $1
		 ORDER BY next_occurrence ASC NULLS LAST, dtstart ASC NULLS LAST`,
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
		`SELECT event_id, user_id, title, description, dtstart, duration, next_occurrence,
		 notification_minutes, recurrence_rule, tags, created_at
		 FROM event WHERE event_id = $1 AND user_id = $2`,
		eventID, userID,
	).Scan(&event.EventID, &event.UserID, &event.Title, &event.Description, &event.Dtstart,
		&event.Duration, &event.NextOccurrence, &event.NotificationMinutes, &event.RecurrenceRule,
		&event.Tags, &event.CreatedAt)
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (r *EventRepository) GetByDateRange(ctx context.Context, userID int64, start, end time.Time) ([]*models.Event, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, dtstart, duration, next_occurrence,
		 notification_minutes, recurrence_rule, tags, created_at
		 FROM event WHERE user_id = $1 AND next_occurrence >= $2 AND next_occurrence <= $3
		 ORDER BY next_occurrence ASC`,
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
		`SELECT event_id, user_id, title, description, dtstart, duration, next_occurrence,
		 notification_minutes, recurrence_rule, tags, created_at
		 FROM event WHERE user_id = $1 AND next_occurrence >= $2 AND next_occurrence <= $3
		 ORDER BY next_occurrence ASC`,
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
		`UPDATE event SET title = $1, description = $2, dtstart = $3, duration = $4,
		 next_occurrence = $5, notification_minutes = $6, recurrence_rule = $7, tags = $8
		 WHERE event_id = $9 AND user_id = $10`,
		event.Title, event.Description, event.Dtstart, event.Duration, event.NextOccurrence,
		event.NotificationMinutes, event.RecurrenceRule, event.Tags,
		event.EventID, event.UserID,
	)
	return err
}

func (r *EventRepository) UpdateNextOccurrence(ctx context.Context, eventID int, nextOccurrence *time.Time) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE event SET next_occurrence = $1 WHERE event_id = $2`,
		nextOccurrence, eventID,
	)
	return err
}

func (r *EventRepository) GetPassedEvents(ctx context.Context, before time.Time) ([]*models.Event, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, dtstart, duration, next_occurrence,
		 notification_minutes, recurrence_rule, tags, created_at
		 FROM event
		 WHERE next_occurrence IS NOT NULL AND next_occurrence <= $1
		 ORDER BY next_occurrence ASC`,
		before,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEvents(rows)
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
		`SELECT event_id, user_id, title, description, dtstart, duration, next_occurrence,
		 notification_minutes, recurrence_rule, tags, created_at
		 FROM event
		 WHERE next_occurrence IS NOT NULL
		 AND next_occurrence - (notification_minutes || ' minutes')::interval <= NOW()
		 AND next_occurrence > NOW()
		 ORDER BY next_occurrence ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEvents(rows)
}

func (r *EventRepository) Search(ctx context.Context, userID int64, keyword string) ([]*models.Event, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT event_id, user_id, title, description, dtstart, duration, next_occurrence,
		 notification_minutes, recurrence_rule, tags, created_at
		 FROM event WHERE user_id = $1 AND (title ILIKE $2 OR description ILIKE $2 OR tags ILIKE $2)
		 ORDER BY next_occurrence ASC NULLS LAST, dtstart ASC NULLS LAST`,
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
			&event.Dtstart, &event.Duration, &event.NextOccurrence, &event.NotificationMinutes,
			&event.RecurrenceRule, &event.Tags, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

package repository

import (
	"context"
	"time"

	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/models"
)

type UserSettingsRepository struct {
	db *database.DB
}

func NewUserSettingsRepository(db *database.DB) *UserSettingsRepository {
	return &UserSettingsRepository{db: db}
}

// GetOrCreate retrieves user settings, creating default settings if none exist
func (r *UserSettingsRepository) GetOrCreate(ctx context.Context, userID int64) (*models.UserSettings, error) {
	settings := &models.UserSettings{}
	var intervalsJSON []byte

	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO user_settings (user_id) VALUES ($1)
		 ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
		 RETURNING user_id, max_daily_reminders, quiet_start::text, quiet_end::text,
		           timezone, reminder_intervals, todo_reminders_enabled,
		           last_todo_message_id, daily_summary_enabled, daily_summary_time::text,
		           last_daily_summary_date, updated_at`,
		userID,
	).Scan(
		&settings.UserID,
		&settings.MaxDailyReminders,
		&settings.QuietStart,
		&settings.QuietEnd,
		&settings.Timezone,
		&intervalsJSON,
		&settings.TodoRemindersEnabled,
		&settings.LastTodoMessageID,
		&settings.DailySummaryEnabled,
		&settings.DailySummaryTime,
		&settings.LastDailySummaryDate,
		&settings.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse reminder intervals JSON
	if err := settings.ReminderIntervals.UnmarshalJSON(intervalsJSON); err != nil {
		settings.ReminderIntervals = models.DefaultReminderIntervals()
	}

	return settings, nil
}

// GetByUserID retrieves user settings by user ID
func (r *UserSettingsRepository) GetByUserID(ctx context.Context, userID int64) (*models.UserSettings, error) {
	settings := &models.UserSettings{}
	var intervalsJSON []byte

	err := r.db.Pool.QueryRow(ctx,
		`SELECT user_id, max_daily_reminders, quiet_start::text, quiet_end::text,
		        timezone, reminder_intervals, todo_reminders_enabled,
		        last_todo_message_id, daily_summary_enabled, daily_summary_time::text,
		        last_daily_summary_date, updated_at
		 FROM user_settings WHERE user_id = $1`,
		userID,
	).Scan(
		&settings.UserID,
		&settings.MaxDailyReminders,
		&settings.QuietStart,
		&settings.QuietEnd,
		&settings.Timezone,
		&intervalsJSON,
		&settings.TodoRemindersEnabled,
		&settings.LastTodoMessageID,
		&settings.DailySummaryEnabled,
		&settings.DailySummaryTime,
		&settings.LastDailySummaryDate,
		&settings.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := settings.ReminderIntervals.UnmarshalJSON(intervalsJSON); err != nil {
		settings.ReminderIntervals = models.DefaultReminderIntervals()
	}

	return settings, nil
}

// Update updates user settings
func (r *UserSettingsRepository) Update(ctx context.Context, settings *models.UserSettings) error {
	intervalsJSON, err := settings.ReminderIntervals.MarshalJSON()
	if err != nil {
		return err
	}

	_, err = r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET
		    max_daily_reminders = $1,
		    quiet_start = $2::time,
		    quiet_end = $3::time,
		    timezone = $4,
		    reminder_intervals = $5,
		    todo_reminders_enabled = $6,
		    updated_at = $7
		 WHERE user_id = $8`,
		settings.MaxDailyReminders,
		settings.QuietStart,
		settings.QuietEnd,
		settings.Timezone,
		intervalsJSON,
		settings.TodoRemindersEnabled,
		time.Now(),
		settings.UserID,
	)
	return err
}

// SetTodoRemindersEnabled toggles todo reminders on/off
func (r *UserSettingsRepository) SetTodoRemindersEnabled(ctx context.Context, userID int64, enabled bool) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET todo_reminders_enabled = $1, updated_at = $2 WHERE user_id = $3`,
		enabled, time.Now(), userID,
	)
	return err
}

// SetQuietHours updates quiet hours settings
func (r *UserSettingsRepository) SetQuietHours(ctx context.Context, userID int64, start, end string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET quiet_start = $1::time, quiet_end = $2::time, updated_at = $3 WHERE user_id = $4`,
		start, end, time.Now(), userID,
	)
	return err
}

// SetMaxDailyReminders updates max daily reminders limit
func (r *UserSettingsRepository) SetMaxDailyReminders(ctx context.Context, userID int64, max int) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET max_daily_reminders = $1, updated_at = $2 WHERE user_id = $3`,
		max, time.Now(), userID,
	)
	return err
}

// SetReminderInterval updates a specific reminder interval
func (r *UserSettingsRepository) SetReminderInterval(ctx context.Context, userID int64, zone string, minutes int) error {
	// Use jsonb_set to update specific interval
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings
		 SET reminder_intervals = jsonb_set(reminder_intervals, $1, $2::jsonb),
		     updated_at = $3
		 WHERE user_id = $4`,
		"{"+zone+"}", minutes, time.Now(), userID,
	)
	return err
}

// SetLastTodoMessageID updates the last todo message ID for a user
func (r *UserSettingsRepository) SetLastTodoMessageID(ctx context.Context, userID int64, messageID int) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET last_todo_message_id = $1 WHERE user_id = $2`,
		messageID, userID,
	)
	return err
}

// ClearLastTodoMessageID clears the last todo message ID
func (r *UserSettingsRepository) ClearLastTodoMessageID(ctx context.Context, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET last_todo_message_id = NULL WHERE user_id = $1`,
		userID,
	)
	return err
}

// GetDailyReminderCount gets the current day's reminder count for a user
func (r *UserSettingsRepository) GetDailyReminderCount(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(count, 0) FROM daily_reminder_count
		 WHERE user_id = $1 AND date = CURRENT_DATE`,
		userID,
	).Scan(&count)
	if err != nil {
		// No record means 0 reminders sent today
		return 0, nil
	}
	return count, nil
}

// IncrementDailyReminderCount increments the daily reminder count
func (r *UserSettingsRepository) IncrementDailyReminderCount(ctx context.Context, userID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`INSERT INTO daily_reminder_count (user_id, date, count) VALUES ($1, CURRENT_DATE, 1)
		 ON CONFLICT (user_id, date) DO UPDATE SET count = daily_reminder_count.count + 1`,
		userID,
	)
	return err
}

// GetAllUsersWithTodoRemindersEnabled returns all user IDs with todo reminders enabled
func (r *UserSettingsRepository) GetAllUsersWithTodoRemindersEnabled(ctx context.Context) ([]int64, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT user_id FROM user_settings WHERE todo_reminders_enabled = true`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, nil
}

// GetAllUsersWithDailySummaryEnabled returns all user IDs with daily summary enabled
func (r *UserSettingsRepository) GetAllUsersWithDailySummaryEnabled(ctx context.Context) ([]int64, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT user_id FROM user_settings WHERE daily_summary_enabled = true`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, nil
}

// SetDailySummaryEnabled toggles daily summary on/off
func (r *UserSettingsRepository) SetDailySummaryEnabled(ctx context.Context, userID int64, enabled bool) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET daily_summary_enabled = $1, updated_at = $2 WHERE user_id = $3`,
		enabled, time.Now(), userID,
	)
	return err
}

// SetDailySummaryTime updates the daily summary time
func (r *UserSettingsRepository) SetDailySummaryTime(ctx context.Context, userID int64, timeStr string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET daily_summary_time = $1::time, updated_at = $2 WHERE user_id = $3`,
		timeStr, time.Now(), userID,
	)
	return err
}

// SetLastDailySummaryDate updates the last daily summary date
func (r *UserSettingsRepository) SetLastDailySummaryDate(ctx context.Context, userID int64, date time.Time) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_settings SET last_daily_summary_date = $1 WHERE user_id = $2`,
		date, userID,
	)
	return err
}

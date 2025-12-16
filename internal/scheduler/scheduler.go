package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/repository"
	"github.com/hray3182/LifeLine/internal/rrule"
)

type Scheduler struct {
	api               *tgbotapi.BotAPI
	reminderRepo      *repository.ReminderRepository
	eventRepo         *repository.EventRepository
	todoRepo          *repository.TodoRepository
	checkInterval     time.Duration
	notifiedReminders map[int]bool
	notifiedEvents    map[int]bool
}

func New(
	api *tgbotapi.BotAPI,
	reminderRepo *repository.ReminderRepository,
	eventRepo *repository.EventRepository,
	todoRepo *repository.TodoRepository,
) *Scheduler {
	return &Scheduler{
		api:               api,
		reminderRepo:      reminderRepo,
		eventRepo:         eventRepo,
		todoRepo:          todoRepo,
		checkInterval:     1 * time.Minute,
		notifiedReminders: make(map[int]bool),
		notifiedEvents:    make(map[int]bool),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	log.Println("Scheduler started")
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	// Wait a bit for migrations to complete before first check
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}

	// Run first check
	s.check(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("Scheduler stopped")
			return
		case <-ticker.C:
			s.check(ctx)
		}
	}
}

func (s *Scheduler) check(ctx context.Context) {
	s.checkReminders(ctx)
	s.checkEvents(ctx)
	s.checkDueTodos(ctx)
}

func (s *Scheduler) checkReminders(ctx context.Context) {
	now := time.Now()
	reminders, err := s.reminderRepo.GetPendingReminders(ctx, now)
	if err != nil {
		log.Printf("Failed to get pending reminders: %v", err)
		return
	}

	for _, reminder := range reminders {
		// Skip if already notified
		if s.notifiedReminders[reminder.ReminderID] {
			continue
		}

		// Send notification
		text := "⏰ *提醒*\n\n" + reminder.Messages
		if reminder.Description != "" {
			text += "\n\n" + reminder.Description
		}
		if reminder.IsRecurring() {
			text += "\n\n🔄 " + rrule.HumanReadableChinese(reminder.RecurrenceRule)
		}

		msg := tgbotapi.NewMessage(reminder.UserID, text)
		msg.ParseMode = "Markdown"
		if _, err := s.api.Send(msg); err != nil {
			log.Printf("Failed to send reminder notification: %v", err)
			continue
		}

		s.notifiedReminders[reminder.ReminderID] = true
		log.Printf("Sent reminder %d to user %d", reminder.ReminderID, reminder.UserID)

		// Handle recurrence or disable
		if reminder.RecurrenceRule == "" || reminder.Dtstart == nil {
			// One-time reminder, disable it
			s.reminderRepo.SetEnabled(ctx, reminder.ReminderID, reminder.UserID, false)
		} else {
			// Calculate next occurrence based on recurrence rule
			next, err := rrule.NextOccurrence(reminder.RecurrenceRule, *reminder.Dtstart, now)
			if err != nil {
				log.Printf("Failed to calculate next occurrence for reminder %d: %v", reminder.ReminderID, err)
				s.reminderRepo.SetEnabled(ctx, reminder.ReminderID, reminder.UserID, false)
			} else if next == nil {
				// No more occurrences, disable it
				s.reminderRepo.SetEnabled(ctx, reminder.ReminderID, reminder.UserID, false)
			} else {
				// Update remind_at to next occurrence
				s.reminderRepo.UpdateRemindAt(ctx, reminder.ReminderID, next)
				// Clear the notified flag so it can be sent again
				delete(s.notifiedReminders, reminder.ReminderID)
				log.Printf("Scheduled next reminder %d at %s", reminder.ReminderID, next.Format("2006-01-02 15:04"))
			}
		}
	}
}

func (s *Scheduler) checkEvents(ctx context.Context) {
	now := time.Now()
	events, err := s.eventRepo.GetPendingNotifications(ctx)
	if err != nil {
		log.Printf("Failed to get pending event notifications: %v", err)
		return
	}

	for _, event := range events {
		// Skip if already notified
		if s.notifiedEvents[event.EventID] {
			continue
		}

		if event.NextOccurrence == nil {
			continue
		}

		// Calculate time until event
		timeUntil := time.Until(*event.NextOccurrence)
		minutesUntil := int(timeUntil.Minutes())

		text := "📅 *即將開始的事件*\n\n"
		text += "*" + event.Title + "*\n"
		text += "⏰ " + event.NextOccurrence.Format("15:04")

		if minutesUntil > 0 {
			text += " (約 " + formatDuration(timeUntil) + " 後)"
		}

		if event.Duration > 0 {
			text += fmt.Sprintf("\n⏱ %d 分鐘", event.Duration)
		}

		if event.IsRecurring() {
			text += "\n🔄 " + rrule.HumanReadableChinese(event.RecurrenceRule)
		}

		if event.Description != "" {
			text += "\n\n" + event.Description
		}

		msg := tgbotapi.NewMessage(event.UserID, text)
		msg.ParseMode = "Markdown"
		if _, err := s.api.Send(msg); err != nil {
			log.Printf("Failed to send event notification: %v", err)
			continue
		}

		s.notifiedEvents[event.EventID] = true
		log.Printf("Sent event notification %d to user %d", event.EventID, event.UserID)

		// For recurring events, schedule next occurrence after the current one passes
		// We need to wait until after the event starts before calculating the next one
		// This will be done in a separate check after the event time passes
	}

	// Check for events that have passed and need next occurrence calculated
	s.updateRecurringEvents(ctx, now)
}

func (s *Scheduler) updateRecurringEvents(ctx context.Context, now time.Time) {
	// Get events where next_occurrence has passed
	events, err := s.eventRepo.GetPassedEvents(ctx, now)
	if err != nil {
		log.Printf("Failed to get passed events: %v", err)
		return
	}

	for _, event := range events {
		// Event time has passed
		if event.RecurrenceRule == "" || event.Dtstart == nil {
			// One-time event, clear next_occurrence
			s.eventRepo.UpdateNextOccurrence(ctx, event.EventID, nil)
			delete(s.notifiedEvents, event.EventID)
		} else {
			// Calculate next occurrence
			next, err := rrule.NextOccurrence(event.RecurrenceRule, *event.Dtstart, now)
			if err != nil {
				log.Printf("Failed to calculate next occurrence for event %d: %v", event.EventID, err)
				s.eventRepo.UpdateNextOccurrence(ctx, event.EventID, nil)
			} else {
				s.eventRepo.UpdateNextOccurrence(ctx, event.EventID, next)
				delete(s.notifiedEvents, event.EventID)
				if next != nil {
					log.Printf("Scheduled next event %d at %s", event.EventID, next.Format("2006-01-02 15:04"))
				}
			}
		}
	}
}

func (s *Scheduler) checkDueTodos(ctx context.Context) {
	// Check for todos due within the next hour that haven't been notified
	// This is a simplified version - you might want to track notified todos in DB
	now := time.Now()

	// Get all users' todos that are due soon (within 1 hour)
	// For simplicity, we'll query by a specific time window
	// In production, you'd want to track notification state in the database

	_ = now // Placeholder for future implementation
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "不到 1 分鐘"
	}
	minutes := int(d.Minutes())
	if d < time.Hour {
		return fmt.Sprintf("%d 分鐘", minutes)
	}
	hours := int(d.Hours())
	mins := minutes % 60
	if mins == 0 {
		return fmt.Sprintf("%d 小時", hours)
	}
	return fmt.Sprintf("%d 小時 %d 分鐘", hours, mins)
}

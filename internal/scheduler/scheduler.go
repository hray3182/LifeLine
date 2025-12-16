package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/repository"
)

type Scheduler struct {
	api             *tgbotapi.BotAPI
	reminderRepo    *repository.ReminderRepository
	eventRepo       *repository.EventRepository
	todoRepo        *repository.TodoRepository
	checkInterval   time.Duration
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
		api:             api,
		reminderRepo:    reminderRepo,
		eventRepo:       eventRepo,
		todoRepo:        todoRepo,
		checkInterval:   1 * time.Minute,
		notifiedReminders: make(map[int]bool),
		notifiedEvents:    make(map[int]bool),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	log.Println("Scheduler started")
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	// Run immediately on start
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

		msg := tgbotapi.NewMessage(reminder.UserID, text)
		msg.ParseMode = "Markdown"
		if _, err := s.api.Send(msg); err != nil {
			log.Printf("Failed to send reminder notification: %v", err)
			continue
		}

		s.notifiedReminders[reminder.ReminderID] = true
		log.Printf("Sent reminder %d to user %d", reminder.ReminderID, reminder.UserID)

		// Handle recurrence or disable
		if reminder.RecurrenceRule == "" {
			// One-time reminder, disable it
			s.reminderRepo.SetEnabled(ctx, reminder.ReminderID, reminder.UserID, false)
		} else {
			// TODO: Calculate next occurrence based on recurrence rule
			s.reminderRepo.SetEnabled(ctx, reminder.ReminderID, reminder.UserID, false)
		}
	}
}

func (s *Scheduler) checkEvents(ctx context.Context) {
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

		// Calculate time until event
		timeUntil := time.Until(*event.StartTime)
		minutesUntil := int(timeUntil.Minutes())

		text := "📅 *即將開始的事件*\n\n"
		text += "*" + event.Title + "*\n"
		text += "⏰ " + event.StartTime.Format("15:04")

		if minutesUntil > 0 {
			text += " (約 " + formatDuration(timeUntil) + " 後)"
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

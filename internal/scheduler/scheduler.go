package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/format"
	"github.com/hray3182/LifeLine/internal/models"
	"github.com/hray3182/LifeLine/internal/repository"
	"github.com/hray3182/LifeLine/internal/rrule"
)

type Scheduler struct {
	api              *tgbotapi.BotAPI
	reminderRepo     *repository.ReminderRepository
	eventRepo        *repository.EventRepository
	todoRepo         *repository.TodoRepository
	userSettingsRepo *repository.UserSettingsRepository
	checkInterval    time.Duration
	notifyCh         chan struct{}
}

func New(
	api *tgbotapi.BotAPI,
	reminderRepo *repository.ReminderRepository,
	eventRepo *repository.EventRepository,
	todoRepo *repository.TodoRepository,
	userSettingsRepo *repository.UserSettingsRepository,
) *Scheduler {
	return &Scheduler{
		api:              api,
		reminderRepo:     reminderRepo,
		eventRepo:        eventRepo,
		todoRepo:         todoRepo,
		userSettingsRepo: userSettingsRepo,
		checkInterval:    1 * time.Minute,
		notifyCh:         make(chan struct{}, 1),
	}
}

// Notify triggers an immediate check. Non-blocking if a check is already pending.
func (s *Scheduler) Notify() {
	select {
	case s.notifyCh <- struct{}{}:
	default:
		// Channel already has a pending notification, skip
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
		case <-s.notifyCh:
			log.Println("Scheduler triggered by notification")
			s.check(ctx)
		}
	}
}

func (s *Scheduler) check(ctx context.Context) {
	s.checkReminders(ctx)
	s.checkEvents(ctx)
	s.checkDueTodos(ctx)
	s.checkDailySummary(ctx)
}

func (s *Scheduler) checkReminders(ctx context.Context) {
	now := time.Now()
	reminders, err := s.reminderRepo.GetPendingReminders(ctx, now)
	if err != nil {
		log.Printf("Failed to get pending reminders: %v", err)
		return
	}

	for _, reminder := range reminders {
		// Delete previous message if exists (to avoid flooding)
		if reminder.LastMessageID != nil {
			deleteMsg := tgbotapi.NewDeleteMessage(reminder.UserID, *reminder.LastMessageID)
			if _, err := s.api.Request(deleteMsg); err != nil {
				log.Printf("Failed to delete old reminder message %d: %v", *reminder.LastMessageID, err)
				// Continue anyway, the old message might have been deleted by user
			}
		}

		// Send notification
		text := "⏰ **提醒**\n\n" + reminder.Messages
		if reminder.Description != "" {
			text += "\n\n" + reminder.Description
		}
		if reminder.IsRecurring() {
			text += "\n\n🔄 " + rrule.HumanReadableChinese(reminder.RecurrenceRule)
		}

		parsed := format.ParseMarkdown(text)
		msg := tgbotapi.NewMessage(reminder.UserID, parsed.Text)
		msg.Entities = parsed.Entities

		// Add confirm button
		confirmButton := tgbotapi.NewInlineKeyboardButtonData(
			"✅ 確認",
			fmt.Sprintf("remind_ack:%d", reminder.ReminderID),
		)
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(confirmButton),
		)

		sentMsg, err := s.api.Send(msg)
		if err != nil {
			log.Printf("Failed to send reminder notification: %v", err)
			continue
		}

		// Save message ID and mark as notified in database
		s.reminderRepo.SetLastMessageID(ctx, reminder.ReminderID, sentMsg.MessageID)
		s.reminderRepo.SetNotifiedAt(ctx, reminder.ReminderID, &now)
		log.Printf("Sent reminder %d to user %d (msg_id=%d)", reminder.ReminderID, reminder.UserID, sentMsg.MessageID)
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
		if event.NextOccurrence == nil {
			continue
		}

		// Calculate time until event
		timeUntil := time.Until(*event.NextOccurrence)
		minutesUntil := int(timeUntil.Minutes())

		text := "📅 **即將開始的事件**\n\n"
		text += "**" + event.Title + "**\n"
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

		parsed := format.ParseMarkdown(text)
		msg := tgbotapi.NewMessage(event.UserID, parsed.Text)
		msg.Entities = parsed.Entities
		if _, err := s.api.Send(msg); err != nil {
			log.Printf("Failed to send event notification: %v", err)
			continue
		}

		// Mark as notified in database
		s.eventRepo.SetNotifiedAt(ctx, event.EventID, &now)
		log.Printf("Sent event notification %d to user %d", event.EventID, event.UserID)
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
			// One-time event, clear next_occurrence (this also clears notified_at)
			s.eventRepo.UpdateNextOccurrence(ctx, event.EventID, nil)
		} else {
			// Calculate next occurrence
			next, err := rrule.NextOccurrence(event.RecurrenceRule, *event.Dtstart, now)
			if err != nil {
				log.Printf("Failed to calculate next occurrence for event %d: %v", event.EventID, err)
				s.eventRepo.UpdateNextOccurrence(ctx, event.EventID, nil)
			} else {
				// Update next_occurrence (this also clears notified_at)
				s.eventRepo.UpdateNextOccurrence(ctx, event.EventID, next)
				if next != nil {
					log.Printf("Scheduled next event %d at %s", event.EventID, next.Format("2006-01-02 15:04"))
				}
			}
		}
	}
}

func (s *Scheduler) checkDueTodos(ctx context.Context) {
	now := time.Now()

	// Get all users with todo reminders enabled
	userIDs, err := s.userSettingsRepo.GetAllUsersWithTodoRemindersEnabled(ctx)
	if err != nil {
		log.Printf("Failed to get users with todo reminders enabled: %v", err)
		return
	}

	for _, userID := range userIDs {
		s.checkUserTodos(ctx, userID, now)
	}
}

func (s *Scheduler) checkUserTodos(ctx context.Context, userID int64, now time.Time) {
	// Get user settings
	settings, err := s.userSettingsRepo.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings for %d: %v", userID, err)
		return
	}

	// Check if in quiet hours
	if settings.IsQuietHours(now) {
		return
	}

	// Check daily reminder limit
	dailyCount, err := s.userSettingsRepo.GetDailyReminderCount(ctx, userID)
	if err != nil {
		log.Printf("Failed to get daily reminder count for %d: %v", userID, err)
		return
	}
	if settings.MaxDailyReminders > 0 && dailyCount >= settings.MaxDailyReminders {
		return
	}

	// Get todos that are due within 7 days
	todos, err := s.todoRepo.GetTodosForNotification(ctx, userID)
	if err != nil {
		log.Printf("Failed to get todos for notification for %d: %v", userID, err)
		return
	}

	// Filter todos that should be notified based on the algorithm
	var todosToNotify []*struct {
		todo     *models.Todo
		timeZone string
	}

	for _, todo := range todos {
		if shouldNotify, zone := s.shouldNotifyTodo(todo, settings, now); shouldNotify {
			todosToNotify = append(todosToNotify, &struct {
				todo     *models.Todo
				timeZone string
			}{todo: todo, timeZone: zone})
		}
	}

	if len(todosToNotify) == 0 {
		return
	}

	// Delete previous combined message if exists
	if settings.LastTodoMessageID != nil {
		deleteMsg := tgbotapi.NewDeleteMessage(userID, *settings.LastTodoMessageID)
		if _, err := s.api.Request(deleteMsg); err != nil {
			log.Printf("Failed to delete old todo reminder message %d: %v", *settings.LastTodoMessageID, err)
			// Continue anyway, the old message might have been deleted by user
		}
	}

	// Build combined notification message
	text := s.buildTodoNotificationText(todosToNotify, now)

	parsed := format.ParseMarkdown(text)
	msg := tgbotapi.NewMessage(userID, parsed.Text)
	msg.Entities = parsed.Entities

	sentMsg, err := s.api.Send(msg)
	if err != nil {
		log.Printf("Failed to send todo notification to %d: %v", userID, err)
		return
	}

	// Update last_notified_at for all notified todos
	todoIDs := make([]int, len(todosToNotify))
	for i, t := range todosToNotify {
		todoIDs[i] = t.todo.TodoID
	}
	if err := s.todoRepo.BatchSetLastNotifiedAt(ctx, todoIDs, &now); err != nil {
		log.Printf("Failed to update last_notified_at for todos: %v", err)
	}

	// Update last message ID for user
	if err := s.userSettingsRepo.SetLastTodoMessageID(ctx, userID, sentMsg.MessageID); err != nil {
		log.Printf("Failed to update last_todo_message_id for %d: %v", userID, err)
	}

	// Increment daily reminder count
	if err := s.userSettingsRepo.IncrementDailyReminderCount(ctx, userID); err != nil {
		log.Printf("Failed to increment daily reminder count for %d: %v", userID, err)
	}

	log.Printf("Sent todo reminder to user %d with %d items (msg_id=%d)", userID, len(todosToNotify), sentMsg.MessageID)
}

// shouldNotifyTodo determines if a todo should be notified based on time and priority
func (s *Scheduler) shouldNotifyTodo(todo *models.Todo, settings *models.UserSettings, now time.Time) (bool, string) {
	if todo.DueTime == nil {
		return false, ""
	}

	timeUntilDue := todo.DueTime.Sub(now)

	// Determine time zone and base interval
	var zone string
	var baseIntervalMinutes int

	switch {
	case timeUntilDue < 0: // Overdue
		zone = "overdue"
		baseIntervalMinutes = settings.ReminderIntervals.Overdue
	case timeUntilDue < 2*time.Hour: // Urgent: within 2 hours
		zone = "urgent"
		baseIntervalMinutes = settings.ReminderIntervals.Urgent
	case timeUntilDue < 24*time.Hour: // Soon: within 24 hours
		zone = "soon"
		baseIntervalMinutes = settings.ReminderIntervals.Soon
	case timeUntilDue < 7*24*time.Hour: // Normal: within 7 days
		zone = "normal"
		baseIntervalMinutes = settings.ReminderIntervals.Normal
	default:
		return false, "" // Too far away
	}

	if baseIntervalMinutes <= 0 {
		return false, ""
	}

	// Apply priority multiplier
	multiplier := getPriorityMultiplier(todo.Priority)
	intervalMinutes := int(float64(baseIntervalMinutes) * multiplier)
	if intervalMinutes < 1 {
		intervalMinutes = 1
	}
	interval := time.Duration(intervalMinutes) * time.Minute

	// Check if enough time has passed since last notification
	if todo.LastNotifiedAt == nil {
		return true, zone
	}

	return now.Sub(*todo.LastNotifiedAt) >= interval, zone
}

// getPriorityMultiplier returns the interval multiplier based on priority
func getPriorityMultiplier(priority int) float64 {
	switch priority {
	case 5:
		return 0.5 // Highest priority: half the interval
	case 4:
		return 0.7
	case 3:
		return 1.0 // Default
	case 2:
		return 1.5
	case 1:
		return 2.0 // Lowest priority: double the interval
	default:
		return 1.0
	}
}

// buildTodoNotificationText builds the combined notification message
func (s *Scheduler) buildTodoNotificationText(todos []*struct {
	todo     *models.Todo
	timeZone string
}, now time.Time) string {
	if len(todos) == 1 {
		todo := todos[0].todo
		text := "📋 **待辦提醒**\n\n"
		text += "**" + todo.Title + "**\n"
		text += "⏰ " + formatDueTime(todo.DueTime, now)
		if todo.Priority > 0 {
			text += fmt.Sprintf(" | ⭐%d", todo.Priority)
		}
		if todo.Description != "" {
			text += "\n\n" + todo.Description
		}
		return text
	}

	text := fmt.Sprintf("📋 **待辦提醒** (%d 項)\n\n", len(todos))
	for i, t := range todos {
		todo := t.todo
		text += fmt.Sprintf("%d. **%s**", i+1, todo.Title)
		text += " - " + formatDueTime(todo.DueTime, now)
		if todo.Priority > 0 {
			text += fmt.Sprintf(" ⭐%d", todo.Priority)
		}
		text += "\n"
	}
	return text
}

// formatDueTime formats the due time relative to now
func formatDueTime(dueTime *time.Time, now time.Time) string {
	if dueTime == nil {
		return ""
	}

	diff := dueTime.Sub(now)

	if diff < 0 {
		// Overdue
		overdue := -diff
		if overdue < time.Hour {
			return fmt.Sprintf("已逾期 %d 分鐘", int(overdue.Minutes()))
		}
		if overdue < 24*time.Hour {
			return fmt.Sprintf("已逾期 %d 小時", int(overdue.Hours()))
		}
		return fmt.Sprintf("已逾期 %d 天", int(overdue.Hours()/24))
	}

	if diff < time.Hour {
		return fmt.Sprintf("剩 %d 分鐘", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		mins := int(diff.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("剩 %d 小時 %d 分", hours, mins)
		}
		return fmt.Sprintf("剩 %d 小時", hours)
	}
	if diff < 48*time.Hour {
		return "明天 " + dueTime.Format("15:04")
	}
	return dueTime.Format("01/02 15:04")
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

// ==================== Daily Summary ====================

func (s *Scheduler) checkDailySummary(ctx context.Context) {
	now := time.Now()

	// Get all users with daily summary enabled
	userIDs, err := s.userSettingsRepo.GetAllUsersWithDailySummaryEnabled(ctx)
	if err != nil {
		log.Printf("Failed to get users with daily summary enabled: %v", err)
		return
	}

	for _, userID := range userIDs {
		s.sendDailySummaryIfNeeded(ctx, userID, now)
	}
}

func (s *Scheduler) sendDailySummaryIfNeeded(ctx context.Context, userID int64, now time.Time) {
	settings, err := s.userSettingsRepo.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user settings for daily summary %d: %v", userID, err)
		return
	}

	if !settings.ShouldSendDailySummary(now) {
		return
	}

	// Get today's events
	todayEvents, err := s.eventRepo.GetTodayEvents(ctx, userID)
	if err != nil {
		log.Printf("Failed to get today events for %d: %v", userID, err)
		todayEvents = nil
	}

	// Get incomplete todos (with due_time within 7 days or no due_time)
	todos, err := s.todoRepo.GetByUserID(ctx, userID, false)
	if err != nil {
		log.Printf("Failed to get todos for daily summary %d: %v", userID, err)
		todos = nil
	}

	// Build and send daily summary message
	text := s.buildDailySummaryText(todayEvents, todos, now, settings.Timezone)

	parsed := format.ParseMarkdown(text)
	msg := tgbotapi.NewMessage(userID, parsed.Text)
	msg.Entities = parsed.Entities

	if _, err := s.api.Send(msg); err != nil {
		log.Printf("Failed to send daily summary to %d: %v", userID, err)
		return
	}

	// Update last daily summary date
	if err := s.userSettingsRepo.SetLastDailySummaryDate(ctx, userID, now); err != nil {
		log.Printf("Failed to update last daily summary date for %d: %v", userID, err)
	}

	log.Printf("Sent daily summary to user %d", userID)
}

func (s *Scheduler) buildDailySummaryText(events []*models.Event, todos []*models.Todo, now time.Time, timezone string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.Local
	}
	localNow := now.In(loc)

	// Greeting based on time
	greeting := getGreeting(localNow.Hour())
	dateStr := localNow.Format("2006/01/02 (Mon)")

	text := fmt.Sprintf("☀️ **%s**\n\n📅 %s\n", greeting, dateStr)

	// Today's events
	text += "\n**今日行程**\n"
	if len(events) == 0 {
		text += "• 今天沒有行程安排\n"
	} else {
		for _, event := range events {
			timeStr := ""
			if event.NextOccurrence != nil {
				timeStr = event.NextOccurrence.In(loc).Format("15:04")
			} else if event.Dtstart != nil {
				timeStr = event.Dtstart.In(loc).Format("15:04")
			}
			text += fmt.Sprintf("• %s %s", timeStr, event.Title)
			if event.Duration > 0 {
				text += fmt.Sprintf(" (%d分鐘)", event.Duration)
			}
			text += "\n"
		}
	}

	// Todo list
	text += "\n**待辦事項**\n"
	if len(todos) == 0 {
		text += "• 沒有待辦事項\n"
	} else {
		// Show up to 10 todos
		count := len(todos)
		if count > 10 {
			count = 10
		}
		for i := 0; i < count; i++ {
			todo := todos[i]
			priority := ""
			if todo.Priority >= 4 {
				priority = " ⭐"
			}
			dueStr := ""
			if todo.DueTime != nil {
				if todo.DueTime.Before(now) {
					dueStr = " (已逾期)"
				} else if todo.DueTime.Before(now.Add(24 * time.Hour)) {
					dueStr = " (今天截止)"
				} else if todo.DueTime.Before(now.Add(48 * time.Hour)) {
					dueStr = " (明天截止)"
				}
			}
			text += fmt.Sprintf("• %s%s%s\n", todo.Title, priority, dueStr)
		}
		if len(todos) > 10 {
			text += fmt.Sprintf("• ...還有 %d 項\n", len(todos)-10)
		}
	}

	text += "\n祝你有美好的一天！ 💪"

	return text
}

func getGreeting(hour int) string {
	switch {
	case hour >= 5 && hour < 12:
		return "早安"
	case hour >= 12 && hour < 18:
		return "午安"
	default:
		return "晚安"
	}
}

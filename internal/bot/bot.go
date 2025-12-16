package bot

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/ai"
	"github.com/hray3182/LifeLine/internal/bot/handlers"
	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/repository"
)

type Bot struct {
	api      *tgbotapi.BotAPI
	handlers *handlers.Handlers
	ai       *ai.Client
}

func New(token string, db *database.DB, aiClient *ai.Client) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	repos := &handlers.Repositories{
		User:        repository.NewUserRepository(db),
		Memo:        repository.NewMemoRepository(db),
		Todo:        repository.NewTodoRepository(db),
		Reminder:    repository.NewReminderRepository(db),
		Category:    repository.NewCategoryRepository(db),
		Transaction: repository.NewTransactionRepository(db),
		Event:       repository.NewEventRepository(db),
	}

	return &Bot{
		api:      api,
		handlers: handlers.New(api, repos, aiClient),
		ai:       aiClient,
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	log.Printf("Authorized on account %s", b.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			go b.handleUpdate(ctx, update)
		}
	}
}

func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	// Handle commands
	if update.Message.IsCommand() {
		b.handlers.HandleCommand(ctx, update.Message)
		return
	}

	// Handle regular messages with AI
	b.handlers.HandleMessage(ctx, update.Message)
}

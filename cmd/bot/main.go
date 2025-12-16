package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hray3182/LifeLine/internal/ai"
	"github.com/hray3182/LifeLine/internal/bot"
	"github.com/hray3182/LifeLine/internal/config"
	"github.com/hray3182/LifeLine/internal/database"
	"github.com/hray3182/LifeLine/internal/repository"
	"github.com/hray3182/LifeLine/internal/scheduler"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate required config
	if cfg.DatabaseURI == "" {
		log.Fatal("DATABASE_URI is required")
	}
	if cfg.TelegramToken == "" {
		log.Fatal("TELEGRAM_TOKEN is required")
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to database
	db, err := database.New(ctx, cfg.DatabaseURI)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Connected to database")

	// Run migrations
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// Initialize AI client (optional)
	var aiClient *ai.Client
	if cfg.AIAPIKey != "" {
		aiClient = ai.New(cfg.AIAPIKey, cfg.AIBaseURL, cfg.AIModel)
		log.Printf("AI client initialized (model: %s)", cfg.AIModel)
	} else {
		log.Println("AI client not configured, natural language features disabled")
	}

	// Create Telegram API client for scheduler
	tgAPI, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("Failed to create Telegram API: %v", err)
	}

	// Create repositories for scheduler
	reminderRepo := repository.NewReminderRepository(db)
	eventRepo := repository.NewEventRepository(db)
	todoRepo := repository.NewTodoRepository(db)

	// Create and start scheduler
	sched := scheduler.New(tgAPI, reminderRepo, eventRepo, todoRepo)
	go sched.Start(ctx)

	// Create and start bot
	b, err := bot.New(cfg.TelegramToken, db, aiClient)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Handle graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	log.Println("Starting bot...")
	if err := b.Start(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Bot error: %v", err)
	}
}

package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURI   string
	TelegramToken string
	AIAPIKey      string
	AIBaseURL     string
	AIModel       string
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		// .env file is optional in production
	}

	return &Config{
		DatabaseURI:   os.Getenv("DATABASE_URI"),
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
		AIAPIKey:      os.Getenv("AI_API_KEY"),
		AIBaseURL:     getEnvOrDefault("AI_BASE_URL", "https://openrouter.ai/api/v1"),
		AIModel:       getEnvOrDefault("AI_MODEL", "openai/gpt-4o-mini"),
	}, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

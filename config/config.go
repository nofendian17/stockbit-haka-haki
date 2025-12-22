package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds application configuration
type Config struct {
	PlayerID     string
	Username     string
	Password     string
	TradingWSURL string

	// Database configuration
	DatabaseHost     string
	DatabasePort     string
	DatabaseName     string
	DatabaseUser     string
	DatabasePassword string

	// Redis configuration
	RedisHost     string
	RedisPassword string
	RedisPort     string

	// LLM configuration
	LLM LLMConfig
}

// LLMConfig holds LLM service configuration
type LLMConfig struct {
	Enabled  bool
	Endpoint string
	APIKey   string
	Model    string
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	// Load .env file if exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	return &Config{
		PlayerID:     os.Getenv("STOCKBIT_PLAYER_ID"),
		Username:     os.Getenv("STOCKBIT_USERNAME"),
		Password:     os.Getenv("STOCKBIT_PASSWORD"),
		TradingWSURL: getEnvOrDefault("TRADING_WS_URL", "wss://wss-trading.stockbit.com/ws"),

		// Database configuration
		DatabaseHost:     getEnvOrDefault("DB_HOST", "localhost"),
		DatabasePort:     getEnvOrDefault("DB_PORT", "5432"),
		DatabaseName:     getEnvOrDefault("DB_NAME", "stockbit_trades"),
		DatabaseUser:     getEnvOrDefault("DB_USER", "stockbit"),
		DatabasePassword: getEnvOrDefault("DB_PASSWORD", "stockbit123"),

		// Redis configuration
		RedisHost:     getEnvOrDefault("REDIS_HOST", "localhost"),
		RedisPort:     getEnvOrDefault("REDIS_PORT", "6379"),
		RedisPassword: getEnvOrDefault("REDIS_PASSWORD", ""),

		// LLM configuration
		LLM: LLMConfig{
			Enabled:  getEnvOrDefault("LLM_ENABLED", "false") == "true",
			Endpoint: getEnvOrDefault("LLM_ENDPOINT", "https://ai.onehub.biz.id/v1"),
			APIKey:   getEnvOrDefault("LLM_API_KEY", ""),
			Model:    getEnvOrDefault("LLM_MODEL", "qwen3-max"),
		},
	}
}

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// DefaultConfig returns default configuration (deprecated - use LoadFromEnv)
func DefaultConfig(playerID, email, password string) *Config {
	return &Config{
		PlayerID:     playerID,
		Username:     email,
		Password:     password,
		TradingWSURL: "wss://wss-trading.stockbit.com/ws",
	}
}

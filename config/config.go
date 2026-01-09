package config

import (
	"fmt"
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

	// Trading configuration
	Trading TradingConfig
}

// LLMConfig holds LLM service configuration
type LLMConfig struct {
	Enabled  bool
	Endpoint string
	APIKey   string
	Model    string
}

// TradingConfig holds trading parameters and thresholds
type TradingConfig struct {
	// Position Management
	MinSignalIntervalMinutes int
	MaxOpenPositions         int
	MaxPositionsPerSymbol    int
	SignalTimeWindowMinutes  int

	// Thresholds
	OrderFlowBuyThreshold       float64
	AggressiveBuyThreshold      float64
	MinBaselineSampleSize       int
	MinBaselineSampleSizeStrict int

	// Strategy Performance
	MinStrategySignals   int
	LowWinRateThreshold  float64 // Percent
	HighWinRateThreshold float64 // Percent

	// Fail-Safe
	RequireOrderFlow bool // If true, reject signals if order flow data is missing

	// Risk Management
	MaxHoldingLossPct float64 // Cut loss if held too long and loss exceeds this (positive value representing negative %)

	// ATR Multipliers
	StopLossATRMultiplier     float64
	TrailingStopATRMultiplier float64
	TakeProfit1ATRMultiplier  float64
	TakeProfit2ATRMultiplier  float64
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

		// Trading configuration
		Trading: TradingConfig{
			// Default values matching previous constants
			MinSignalIntervalMinutes: getEnvInt("TRADING_MIN_SIGNAL_INTERVAL", 15),
			MaxOpenPositions:         getEnvInt("TRADING_MAX_OPEN_POSITIONS", 10),
			MaxPositionsPerSymbol:    getEnvInt("TRADING_MAX_POSITIONS_PER_SYMBOL", 1),
			SignalTimeWindowMinutes:  getEnvInt("TRADING_SIGNAL_TIME_WINDOW", 5),

			OrderFlowBuyThreshold:       getEnvFloat("TRADING_ORDER_FLOW_THRESHOLD", 0.50), // Was 0.45
			AggressiveBuyThreshold:      getEnvFloat("TRADING_AGGRESSIVE_BUY_THRESHOLD", 55.0),
			MinBaselineSampleSize:       getEnvInt("TRADING_MIN_BASELINE_SAMPLE", 30),
			MinBaselineSampleSizeStrict: getEnvInt("TRADING_MIN_BASELINE_SAMPLE_STRICT", 50),

			MinStrategySignals:   getEnvInt("TRADING_MIN_STRATEGY_SIGNALS", 10),
			LowWinRateThreshold:  getEnvFloat("TRADING_LOW_WIN_RATE", 40.0), // Was 30.0
			HighWinRateThreshold: getEnvFloat("TRADING_HIGH_WIN_RATE", 65.0),

			RequireOrderFlow: getEnvOrDefault("TRADING_REQUIRE_ORDER_FLOW", "false") == "true", // Default false for now to match previous behavior (soft check), plan to enable later

			MaxHoldingLossPct: getEnvFloat("TRADING_MAX_HOLDING_LOSS_PCT", 5.0), // Relaxed from 1.5 to 5.0 to prevent premature cut loss

			StopLossATRMultiplier:     getEnvFloat("TRADING_SL_ATR_MULT", 2.0),
			TrailingStopATRMultiplier: getEnvFloat("TRADING_TS_ATR_MULT", 2.5),
			TakeProfit1ATRMultiplier:  getEnvFloat("TRADING_TP1_ATR_MULT", 4.0),
			TakeProfit2ATRMultiplier:  getEnvFloat("TRADING_TP2_ATR_MULT", 8.0),
		},
	}
}

// getEnvInt gets environment variable as int or returns default value
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var intValue int
	if _, err := fmt.Sscanf(value, "%d", &intValue); err != nil {
		return defaultValue
	}
	return intValue
}

// getEnvFloat gets environment variable as float64 or returns default value
func getEnvFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var floatValue float64
	if _, err := fmt.Sscanf(value, "%f", &floatValue); err != nil {
		return defaultValue
	}
	return floatValue
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

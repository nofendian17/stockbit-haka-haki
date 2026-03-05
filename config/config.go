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
	MinBaselineSampleSize       int
	MinBaselineSampleSizeStrict int

	// Strategy Performance
	MinStrategySignals   int
	LowWinRateThreshold  float64 // Percent
	HighWinRateThreshold float64 // Percent

	// Risk Management
	MaxHoldingLossPct    float64 // Cut loss if held too long and loss exceeds this (positive value representing negative %)
	MaxDailyLossPct      float64 // Maximum daily loss percentage before stopping trading
	MaxConsecutiveLosses int     // Maximum consecutive losses before circuit breaker

	// ATR Multipliers
	StopLossATRMultiplier     float64
	TrailingStopATRMultiplier float64
	TakeProfit1ATRMultiplier  float64
	TakeProfit2ATRMultiplier  float64

	// Breakeven Settings
	BreakevenTriggerPct float64 // Profit percentage to trigger breakeven stop
	BreakevenBufferPct  float64 // Buffer above entry price for breakeven stop

	// Swing Trading Configuration
	EnableSwingTrading   bool    // Enable swing trading mode
	SwingMinConfidence   float64 // Minimum confidence for swing signals (higher than day trading)
	SwingMaxHoldingDays  int     // Maximum holding period for swing (default 30 days)
	SwingATRMultiplier   float64 // ATR multiplier for swing (more lenient than day trading)
	SwingMinBaselineDays int     // Minimum baseline data in days for swing
	SwingPositionSizePct float64 // Position size as % of portfolio for swing
	SwingRequireTrend    bool    // Require strong trend confirmation for swing

	// Testing & Simulation
	MockTradingMode bool // Bypass strict market hours and trend checks for simulation
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

		// Trading configuration - Relaxed for mock trading / active signals
		Trading: TradingConfig{
			// Position Management - Allow more active testing
			MinSignalIntervalMinutes: getEnvInt("TRADING_MIN_SIGNAL_INTERVAL", 5), // Reduced for testing
			MaxOpenPositions:         getEnvInt("TRADING_MAX_OPEN_POSITIONS", 20),
			MaxPositionsPerSymbol:    getEnvInt("TRADING_MAX_POSITIONS_PER_SYMBOL", 3),
			SignalTimeWindowMinutes:  getEnvInt("TRADING_SIGNAL_TIME_WINDOW", 2),

			// Thresholds - Relaxed for mock testing
			MinBaselineSampleSize:       getEnvInt("TRADING_MIN_BASELINE_SAMPLE", 5),           // Dropped to 5 for quick mock
			MinBaselineSampleSizeStrict: getEnvInt("TRADING_MIN_BASELINE_SAMPLE_STRICT", 10),

			// Strategy Performance - Allow newer strategies to trade
			MinStrategySignals:   getEnvInt("TRADING_MIN_STRATEGY_SIGNALS", 0),  // 0 so new DB instances can start mock trading
			LowWinRateThreshold:  getEnvFloat("TRADING_LOW_WIN_RATE", 0.0),      // 0% to allow testing
			HighWinRateThreshold: getEnvFloat("TRADING_HIGH_WIN_RATE", 50.0),

			// Risk Management - Tighter to prevent large losses
			MaxHoldingLossPct:    getEnvFloat("TRADING_MAX_HOLDING_LOSS_PCT", 10.0), // Relaxed
			MaxDailyLossPct:      getEnvFloat("TRADING_MAX_DAILY_LOSS_PCT", 20.0),   // Relaxed
			MaxConsecutiveLosses: getEnvInt("TRADING_MAX_CONSECUTIVE_LOSSES", 10),  // Relaxed

			// ATR Multipliers - Optimized for risk/reward
			StopLossATRMultiplier:     getEnvFloat("TRADING_SL_ATR_MULT", 1.5), // Reduced from 2.0 for tighter stops
			TrailingStopATRMultiplier: getEnvFloat("TRADING_TS_ATR_MULT", 2.0), // Reduced from 2.5

			TakeProfit1ATRMultiplier: getEnvFloat("TRADING_TP1_ATR_MULT", 3.0), // Reduced from 4.0 for faster profits
			TakeProfit2ATRMultiplier: getEnvFloat("TRADING_TP2_ATR_MULT", 6.0), // Reduced from 8.0

			// Breakeven Settings - NEW
			BreakevenTriggerPct: getEnvFloat("TRADING_BREAKEVEN_TRIGGER_PCT", 1.0), // Trigger at 1% profit
			BreakevenBufferPct:  getEnvFloat("TRADING_BREAKEVEN_BUFFER_PCT", 0.15), // Set stop at +0.15% to cover fees

			// Swing Trading Configuration - NEW
			EnableSwingTrading:   getEnvOrDefault("SWING_TRADING_ENABLED", "true") == "false", // Disabled by default
			SwingMinConfidence:   getEnvFloat("SWING_MIN_CONFIDENCE", 0.75),                   // Higher threshold for swing
			SwingMaxHoldingDays:  getEnvInt("SWING_MAX_HOLDING_DAYS", 30),                     // Max 30 days
			SwingATRMultiplier:   getEnvFloat("SWING_ATR_MULTIPLIER", 3.0),                    // More lenient than day trading (1.5)
			SwingMinBaselineDays: getEnvInt("SWING_MIN_BASELINE_DAYS", 20),                    // Need 20 days of history
			SwingPositionSizePct: getEnvFloat("SWING_POSITION_SIZE_PCT", 5.0),                 // 5% of portfolio
			SwingRequireTrend:    getEnvOrDefault("SWING_REQUIRE_TREND", "true") == "true",    // Require trend confirmation

			// Testing & Simulation
			MockTradingMode: getEnvOrDefault("MOCK_TRADING_MODE", "true") == "true",
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

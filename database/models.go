// Package database provides database connection management for the stockbit-haka-haki trading analysis system.
//
// This package includes:
//   - Database connection management using GORM and PostgreSQL
//   - Support for TimescaleDB hypertables and continuous aggregates
//   - Comprehensive error handling and validation
//
// Key Concepts:
//   - TimescaleDB hypertables for time-series data optimization
//   - Continuous aggregates for pre-computed candle data
//   - Composite primary keys for hypertable compatibility
//   - Automatic retention policies for data lifecycle management
//
// Data Models:
//
//	All data models (Trade, Candle, WhaleAlert, etc.) are defined in the models_pkg package
//	to avoid circular import dependencies.
package database

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	models "stockbit-haka-haki/database/models_pkg"
)

// Database holds the GORM database connection and provides access to the underlying DB instance.
// It serves as the central connection point for all database operations in the application.
type Database struct {
	db *gorm.DB
}

// DB returns the underlying GORM database instance for direct access when needed.
// This method provides access to the raw GORM DB for advanced operations.
func (d *Database) DB() *gorm.DB {
	return d.db
}

// Connect establishes database connection using GORM
func Connect(host string, port int, dbname, user, password string) (*Database, error) {
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		host, port, dbname, user, password)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Silent logging for production
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Database{db: db}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	sqlDB, err := d.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ============================================================================
// Backward Compatibility Type Aliases
// ============================================================================

// These type aliases maintain backward compatibility with existing code
// that imports types from the database package directly.

// Core data models - type aliases for backward compatibility
type Trade = models.Trade
type Candle = models.Candle
type WhaleAlert = models.WhaleAlert
type WhaleWebhook = models.WhaleWebhook
type WhaleWebhookLog = models.WhaleWebhookLog
type TradingSignal = models.TradingSignal
type TradingSignalDB = models.TradingSignalDB
type SignalOutcome = models.SignalOutcome
type WhaleAlertFollowup = models.WhaleAlertFollowup
type OrderFlowImbalance = models.OrderFlowImbalance
type StatisticalBaseline = models.StatisticalBaseline
type MarketRegime = models.MarketRegime
type DetectedPattern = models.DetectedPattern
type StockCorrelation = models.StockCorrelation
type WhaleStats = models.WhaleStats

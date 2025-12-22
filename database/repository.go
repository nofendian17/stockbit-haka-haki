package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// TradeRepository handles database operations for trades
type TradeRepository struct {
	db *Database
}

// NewTradeRepository creates a new trade repository
func NewTradeRepository(db *Database) *TradeRepository {
	return &TradeRepository{db: db}
}

// InitSchema performs auto-migration and TimescaleDB setup
func (r *TradeRepository) InitSchema() error {
	// Drop continuous aggregate view if exists to allow table alterations
	// This is necessary because TimescaleDB/Postgres locks columns used in views
	if err := r.db.db.Exec("DROP MATERIALIZED VIEW IF EXISTS candle_1min CASCADE").Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to drop view candle_1min: %v\n", err)
	}

	// Create running_trades table manually if not exists (before converting to hypertable)
	// GORM AutoMigrate fails on Hypertables with Views, so we manage schema manually
	if err := r.db.db.Exec(`
		CREATE TABLE IF NOT EXISTS running_trades (
			id BIGSERIAL,
			timestamp TIMESTAMPTZ NOT NULL,
			stock_symbol VARCHAR(10) NOT NULL,
			action VARCHAR(10) NOT NULL,
			price DOUBLE PRECISION NOT NULL,
			volume BIGINT NOT NULL,
			volume_lot DOUBLE PRECISION NOT NULL,
			total_amount DOUBLE PRECISION NOT NULL,
			market_board VARCHAR(10) NOT NULL,
			change DOUBLE PRECISION,
			trade_number BIGINT,
			PRIMARY KEY (id, timestamp)
		)
	`).Error; err != nil {
		return fmt.Errorf("failed to create running_trades table: %w", err)
	}

	// Add trade_number column if it doesn't exist (migration for existing databases)
	r.db.db.Exec(`
		ALTER TABLE running_trades 
		ADD COLUMN IF NOT EXISTS trade_number BIGINT
	`)

	// Create unique index on (stock_symbol, trade_number, market_board, date)
	// Trade numbers reset daily in Stockbit system, so we need to include the date
	r.db.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_running_trades_unique_trade 
		ON running_trades (stock_symbol, trade_number, market_board, DATE(timestamp))
		WHERE trade_number IS NOT NULL
	`)

	// Create regular index on trade_number for faster lookups
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_running_trades_trade_number 
		ON running_trades (trade_number)
		WHERE trade_number IS NOT NULL
	`)

	// Auto-migrate other tables
	err := r.db.db.AutoMigrate(
		// &Trade{}, // Managed manually above
		// &Candle{}, // Managed as TimescaleDB Continuous Aggregate
		&WhaleAlert{},
		&WhaleWebhook{},
		&WhaleWebhookLog{},
	)
	if err != nil {
		return fmt.Errorf("auto-migration failed: %w", err)
	}

	// Create TimescaleDB extension and hypertables
	if err := r.setupTimescaleDB(); err != nil {
		return err
	}

	return nil
}

// setupTimescaleDB creates hypertables and policies
func (r *TradeRepository) setupTimescaleDB() error {
	// Enable TimescaleDB extension
	if err := r.db.db.Exec("CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE").Error; err != nil {
		return fmt.Errorf("failed to create TimescaleDB extension: %w", err)
	}

	// Create hypertable for running_trades
	r.db.db.Exec(`
		SELECT create_hypertable('running_trades', 'timestamp',  
			chunk_time_interval => INTERVAL '1 day',
			if_not_exists => TRUE
		)
	`)

	// Add retention policy: 3 months
	r.db.db.Exec(`
		SELECT add_retention_policy('running_trades', INTERVAL '3 months', if_not_exists => TRUE)
	`)

	// Create continuous aggregate for 1-minute candles
	r.db.db.Exec(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS candle_1min
		WITH (timescaledb.continuous) AS
		SELECT 
			time_bucket('1 minute', timestamp) AS bucket,
			stock_symbol,
			FIRST(price, timestamp) AS open,
			MAX(price) AS high,
			MIN(price) AS low,
			LAST(price, timestamp) AS close,
			SUM(volume) AS volume_shares,
			SUM(volume_lot) AS volume_lots,
			SUM(total_amount) AS total_value,
			COUNT(*) AS trade_count,
			MODE() WITHIN GROUP (ORDER BY market_board) AS market_board
		FROM running_trades
		GROUP BY bucket, stock_symbol
	`)

	// Add refresh policy for continuous aggregate
	r.db.db.Exec(`
		SELECT add_continuous_aggregate_policy('candle_1min',
			start_offset => INTERVAL '3 minutes',
			end_offset => INTERVAL '1 minute',
			schedule_interval => INTERVAL '1 minute',
			if_not_exists => TRUE
		)
	`)

	// Add 10-year retention for candles
	r.db.db.Exec(`
		SELECT add_retention_policy('candle_1min', INTERVAL '10 years', if_not_exists => TRUE)
	`)

	// Create hypertable for whale_alerts
	r.db.db.Exec(`
		SELECT create_hypertable('whale_alerts', 'detected_at',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`)

	// Add retention policy: 1 year
	r.db.db.Exec(`
		SELECT add_retention_policy('whale_alerts', INTERVAL '1 year', if_not_exists => TRUE)
	`)

	// Create hypertable for whale_webhook_logs
	r.db.db.Exec(`
		SELECT create_hypertable('whale_webhook_logs', 'triggered_at',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`)

	// Add retention policy: 30 days
	r.db.db.Exec(`
		SELECT add_retention_policy('whale_webhook_logs', INTERVAL '30 days', if_not_exists => TRUE)
	`)

	return nil
}

// SaveTrade saves a trade record using GORM
func (r *TradeRepository) SaveTrade(trade *Trade) error {
	return r.db.db.Create(trade).Error
}

// GetRecentTrades retrieves recent trades with filters
func (r *TradeRepository) GetRecentTrades(stockSymbol string, limit int, actionFilter string) ([]Trade, error) {
	var trades []Trade
	query := r.db.db.Order("timestamp DESC")

	if stockSymbol != "" {
		query = query.Where("stock_symbol = ?", stockSymbol)
	}

	if actionFilter != "" {
		query = query.Where("action = ?", actionFilter)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&trades).Error
	return trades, err
}

// GetCandles retrieves candle data with filters
func (r *TradeRepository) GetCandles(stockSymbol string, startTime, endTime time.Time, limit int) ([]Candle, error) {
	var candles []Candle
	query := r.db.db.Order("bucket DESC")

	if stockSymbol != "" {
		query = query.Where("stock_symbol = ?", stockSymbol)
	}

	if !startTime.IsZero() {
		query = query.Where("bucket >= ?", startTime)
	}

	if !endTime.IsZero() {
		query = query.Where("bucket <= ?", endTime)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&candles).Error
	return candles, err
}

// GetLatestCandle retrieves the most recent candle for a stock
func (r *TradeRepository) GetLatestCandle(stockSymbol string) (*Candle, error) {
	var candle Candle
	err := r.db.db.
		Where("stock_symbol = ?", stockSymbol).
		Order("bucket DESC").
		First(&candle).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}

	return &candle, err
}

// SaveWhaleAlert saves a whale alert using GORM
func (r *TradeRepository) SaveWhaleAlert(alert *WhaleAlert) error {
	return r.db.db.Create(alert).Error
}

// GetHistoricalWhales retrieves whale alerts with filters
func (r *TradeRepository) GetHistoricalWhales(stockSymbol string, startTime, endTime time.Time, alertType string, limit, offset int) ([]WhaleAlert, error) {
	var whales []WhaleAlert
	query := r.db.db.Order("detected_at DESC")

	if stockSymbol != "" {
		query = query.Where("stock_symbol = ?", stockSymbol)
	}

	if !startTime.IsZero() {
		query = query.Where("detected_at >= ?", startTime)
	}

	if !endTime.IsZero() {
		query = query.Where("detected_at <= ?", endTime)
	}

	if alertType != "" {
		query = query.Where("alert_type = ?", alertType)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&whales).Error
	return whales, err
}

// GetWhaleCount returns total count of whales matching filters
func (r *TradeRepository) GetWhaleCount(stockSymbol string, startTime, endTime time.Time, alertType string) (int64, error) {
	var count int64
	query := r.db.db.Model(&WhaleAlert{})

	if stockSymbol != "" {
		query = query.Where("stock_symbol = ?", stockSymbol)
	}

	if !startTime.IsZero() {
		query = query.Where("detected_at >= ?", startTime)
	}

	if !endTime.IsZero() {
		query = query.Where("detected_at <= ?", endTime)
	}

	if alertType != "" {
		query = query.Where("alert_type = ?", alertType)
	}

	err := query.Count(&count).Error
	return count, err
}

// GetActiveWebhooks retrieves all active webhooks
func (r *TradeRepository) GetActiveWebhooks() ([]WhaleWebhook, error) {
	var webhooks []WhaleWebhook
	err := r.db.db.Where("is_active = ?", true).Find(&webhooks).Error
	return webhooks, err
}

// SaveWebhookLog saves a new webhook delivery log
func (r *TradeRepository) SaveWebhookLog(log *WhaleWebhookLog) error {
	return r.db.db.Create(log).Error
}

// GetWhaleStats calculates aggregated statistics for whale alerts
func (r *TradeRepository) GetWhaleStats(stockSymbol string, startTime, endTime time.Time) (*WhaleStats, error) {
	var stats WhaleStats

	// Base selection columns for aggregation
	aggSelect := "count(*) as total_whale_trades, sum(trigger_value) as total_whale_value, " +
		"sum(case when action = 'BUY' then trigger_volume_lots else 0 end) as buy_volume_lots, " +
		"sum(case when action = 'SELL' then trigger_volume_lots else 0 end) as sell_volume_lots, " +
		"max(trigger_value) as largest_trade_value"

	var query *gorm.DB
	if stockSymbol != "" {
		// Specific stock: Select symbol and group by it
		query = r.db.db.Model(&WhaleAlert{}).Select("stock_symbol, "+aggSelect).Where("stock_symbol = ?", stockSymbol).Group("stock_symbol")
	} else {
		// Global stats: Select static 'ALL' as symbol, no grouping (aggregates entire filtered set)
		query = r.db.db.Model(&WhaleAlert{}).Select("'ALL' as stock_symbol, " + aggSelect)
	}

	if !startTime.IsZero() {
		query = query.Where("detected_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("detected_at <= ?", endTime)
	}

	err := query.Scan(&stats).Error
	return &stats, err
}

// Webhook Management methods

// GetWebhooks retrieves all webhooks (active and inactive)
func (r *TradeRepository) GetWebhooks() ([]WhaleWebhook, error) {
	var webhooks []WhaleWebhook
	err := r.db.db.Order("id ASC").Find(&webhooks).Error
	return webhooks, err
}

// GetWebhookByID retrieves a specific webhook
func (r *TradeRepository) GetWebhookByID(id int) (*WhaleWebhook, error) {
	var webhook WhaleWebhook
	err := r.db.db.First(&webhook, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &webhook, err
}

// SaveWebhook creates or updates a webhook
func (r *TradeRepository) SaveWebhook(webhook *WhaleWebhook) error {
	return r.db.db.Save(webhook).Error
}

// DeleteWebhook deletes a webhook
func (r *TradeRepository) DeleteWebhook(id int) error {
	return r.db.db.Delete(&WhaleWebhook{}, id).Error
}

// Statistical Analysis Methods

// StockStats holds aggregated statistical data for a stock
type StockStats struct {
	MeanVolumeLots float64
	StdDevVolume   float64
	MeanValue      float64
	StdDevValue    float64
	MeanPrice      float64 // New field
	SampleCount    int64
}

// GetStockStats calculates statistics based on recent history (e.g., last 60 minutes)
// Uses the candle_1min materialized view for efficient aggregation.
// Query optimizations:
// - Uses COALESCE to handle NULL values when no data exists
// - Aggregates from pre-computed candles instead of raw trades
// - Time-based filtering using parameterized lookback window
func (r *TradeRepository) GetStockStats(symbol string, lookbackMinutes int) (*StockStats, error) {
	var stats StockStats

	// Query candle_1min view for more efficient stats
	// We use coalesce to handle nulls if there's no data
	query := `
		SELECT 
			COALESCE(AVG(volume_lots), 0) as mean_volume_lots,
			COALESCE(STDDEV(volume_lots), 0) as std_dev_volume,
			COALESCE(AVG(total_value), 0) as mean_value,
			COALESCE(STDDEV(total_value), 0) as std_dev_value,
			COALESCE(AVG(close), 0) as mean_price, 
			COUNT(*) as sample_count
		FROM candle_1min
		WHERE stock_symbol = ? 
		AND bucket >= NOW() - INTERVAL '1 minute' * ?
	`

	err := r.db.db.Raw(query, symbol, lookbackMinutes).Scan(&stats).Error
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// AccumulationPattern represents detected accumulation/distribution pattern
type AccumulationPattern struct {
	StockSymbol    string
	Action         string
	AlertCount     int64
	TotalValue     float64
	FirstAlertTime time.Time
	LastAlertTime  time.Time
	AvgZScore      float64
}

// GetAccumulationPattern detects BUY/SELL sequences (accumulation/distribution)
// Identifies repeated whale activity grouped by stock and action.
// Query optimizations:
// - Groups by stock_symbol and action for pattern detection
// - Filters by time window and minimum alert threshold
// - Orders by total value to prioritize significant patterns
func (r *TradeRepository) GetAccumulationPattern(hoursBack int, minAlerts int) ([]AccumulationPattern, error) {
	var patterns []AccumulationPattern

	query := `
		SELECT 
			stock_symbol,
			action,
			COUNT(*) as alert_count,
			SUM(trigger_value) as total_value,
			MIN(detected_at) as first_alert_time,
			MAX(detected_at) as last_alert_time,
			AVG(COALESCE(z_score, 0)) as avg_z_score
		FROM whale_alerts
		WHERE detected_at >= NOW() - INTERVAL '1 hour' * ?
		GROUP BY stock_symbol, action
		HAVING COUNT(*) >= ?
		ORDER BY total_value DESC
	`

	err := r.db.db.Raw(query, hoursBack, minAlerts).Scan(&patterns).Error
	return patterns, err
}

// ExtremeAnomaly represents whale alerts with unusually high Z-scores
type ExtremeAnomaly struct {
	WhaleAlert
	DurationSinceDetection time.Duration
}

// GetExtremeAnomalies returns alerts with Z-Score > minZScore
func (r *TradeRepository) GetExtremeAnomalies(minZScore float64, hoursBack int) ([]WhaleAlert, error) {
	var anomalies []WhaleAlert

	err := r.db.db.Where("z_score >= ? AND detected_at >= NOW() - INTERVAL '1 hour' * ?", minZScore, hoursBack).
		Order("z_score DESC").
		Limit(50).
		Find(&anomalies).Error

	return anomalies, err
}

// TimeBasedStat represents whale activity statistics by time bucket
type TimeBasedStat struct {
	TimeBucket string
	AlertCount int64
	TotalValue float64
	AvgZScore  float64
	BuyCount   int64
	SellCount  int64
}

// GetTimeBasedStats returns whale activity distribution by hour
func (r *TradeRepository) GetTimeBasedStats(daysBack int) ([]TimeBasedStat, error) {
	var stats []TimeBasedStat

	query := `
		SELECT 
			EXTRACT(HOUR FROM (detected_at AT TIME ZONE 'Asia/Jakarta'))::TEXT as time_bucket,
			COUNT(*) as alert_count,
			SUM(trigger_value) as total_value,
			AVG(COALESCE(z_score, 0)) as avg_z_score,
			SUM(CASE WHEN action = 'BUY' THEN 1 ELSE 0 END) as buy_count,
			SUM(CASE WHEN action = 'SELL' THEN 1 ELSE 0 END) as sell_count
		FROM whale_alerts
		WHERE detected_at >= NOW() - INTERVAL '1 day' * ?
		GROUP BY EXTRACT(HOUR FROM (detected_at AT TIME ZONE 'Asia/Jakarta'))
		ORDER BY time_bucket
	`

	err := r.db.db.Raw(query, daysBack).Scan(&stats).Error
	return stats, err
}

// GetRecentAlertsBySymbol returns recent alerts for a specific stock (for LLM context)
func (r *TradeRepository) GetRecentAlertsBySymbol(symbol string, limit int) ([]WhaleAlert, error) {
	var alerts []WhaleAlert

	err := r.db.db.Where("stock_symbol = ?", symbol).
		Order("detected_at DESC").
		Limit(limit).
		Find(&alerts).Error

	return alerts, err
}

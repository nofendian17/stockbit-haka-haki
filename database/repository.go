package database

import (
	"errors"
	"fmt"
	"sort"
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
	fmt.Println("🔄 Starting database schema initialization...")

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
		// Phase 1 Enhancement Tables
		&TradingSignalDB{},
		&SignalOutcome{},
		&WhaleAlertFollowup{},
		&OrderFlowImbalance{},
		// Phase 2 Enhancement Tables
		&StatisticalBaseline{},
		&MarketRegime{},
		&DetectedPattern{},
		// Phase 3 Enhancement Tables
		&StockCorrelation{},
	)
	if err != nil {
		return fmt.Errorf("auto-migration failed: %w", err)
	}

	// Create strategy_performance_daily view immediately after AutoMigrate
	// This ensures the view is created before any hypertable operations
	fmt.Println("📊 Creating strategy_performance_daily materialized view...")
	if err := r.db.db.Exec(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS strategy_performance_daily AS
		SELECT
			date_trunc('day', so.entry_time) AS day,
			ts.strategy,
			so.stock_symbol,
			COUNT(*) AS total_signals,
			SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END) AS wins,
			SUM(CASE WHEN so.outcome_status = 'LOSS' THEN 1 ELSE 0 END) AS losses,
			AVG(so.profit_loss_pct) AS avg_profit_pct,
			SUM(so.profit_loss_pct) AS total_profit_pct,
			AVG(so.risk_reward_ratio) AS avg_risk_reward
		FROM signal_outcomes so
		JOIN trading_signals ts ON so.signal_id = ts.id
		GROUP BY day, ts.strategy, so.stock_symbol
	`).Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to create view strategy_performance_daily: %v\n", err)
	} else {
		fmt.Println("✅ strategy_performance_daily view created successfully")
	}

	// Manual migrations for whale_alert_followup columns (GORM sometimes struggles with Hypertables)
	r.db.db.Exec(`
		ALTER TABLE whale_alert_followup 
		ADD COLUMN IF NOT EXISTS price_1min_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS price_5min_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS price_15min_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS price_30min_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS price_60min_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS price_1day_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS change_1min_pct DECIMAL(10,4),
		ADD COLUMN IF NOT EXISTS change_5min_pct DECIMAL(10,4),
		ADD COLUMN IF NOT EXISTS change_15min_pct DECIMAL(10,4),
		ADD COLUMN IF NOT EXISTS change_30min_pct DECIMAL(10,4),
		ADD COLUMN IF NOT EXISTS change_60min_pct DECIMAL(10,4),
		ADD COLUMN IF NOT EXISTS change_1day_pct DECIMAL(10,4),
		ADD COLUMN IF NOT EXISTS volume_1min_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS volume_5min_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS volume_15min_later DECIMAL(15,2)
	`)

	// Create TimescaleDB extension and hypertables
	if err := r.setupTimescaleDB(); err != nil {
		return err
	}

	// Setup Phase 1 enhancement tables with TimescaleDB features
	if err := r.setupEnhancedTables(); err != nil {
		return err
	}

	fmt.Println("✅ Database schema initialization completed successfully")
	return nil
}

// setupTimescaleDB creates hypertables and policies
func (r *TradeRepository) setupTimescaleDB() error {
	fmt.Println("⏰ Setting up TimescaleDB extension and hypertables...")

	// Enable TimescaleDB extension
	if err := r.db.db.Exec("CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE").Error; err != nil {
		return fmt.Errorf("failed to create TimescaleDB extension: %w", err)
	}
	fmt.Println("✅ TimescaleDB extension enabled")

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

// setupEnhancedTables creates hypertables and policies for Phase 1 enhancement tables
func (r *TradeRepository) setupEnhancedTables() error {
	// Create hypertable for trading_signals
	// Create hypertable for trading_signals
	if err := r.db.db.Exec(`
		SELECT create_hypertable('trading_signals', 'generated_at',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`).Error; err != nil {
		// Log warning but continue
		fmt.Printf("⚠️ Warning: Failed to create hypertable for trading_signals: %v\n", err)
	}

	// Add retention policy: 2 years
	r.db.db.Exec(`
		SELECT add_retention_policy('trading_signals', INTERVAL '2 years', if_not_exists => TRUE)
	`)

	// Create indexes for trading_signals
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_signals_symbol_strategy 
		ON trading_signals(stock_symbol, strategy, generated_at DESC)
	`)
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_signals_decision 
		ON trading_signals(decision, confidence DESC)
	`)

	// Create hypertable for signal_outcomes
	// Note: We need to ensure Primary Key includes time column for Hypertables
	fmt.Println("🔄 Setting up signal_outcomes hypertable...")
	if err := r.db.db.Exec(`
		ALTER TABLE signal_outcomes DROP CONSTRAINT IF EXISTS signal_outcomes_pkey;
	`).Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to drop signal_outcomes primary key: %v\n", err)
	}

	if err := r.db.db.Exec(`
		ALTER TABLE signal_outcomes ADD PRIMARY KEY (id, entry_time);
	`).Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to add signal_outcomes composite primary key: %v\n", err)
	}

	if err := r.db.db.Exec(`
		SELECT create_hypertable('signal_outcomes', 'entry_time',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`).Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to create hypertable for signal_outcomes: %v\n", err)
	} else {
		fmt.Println("✅ signal_outcomes hypertable created successfully")
	}

	// Add retention policy: 2 years
	r.db.db.Exec(`
		SELECT add_retention_policy('signal_outcomes', INTERVAL '2 years', if_not_exists => TRUE)
	`)

	// Create indexes for signal_outcomes
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_outcomes_symbol 
		ON signal_outcomes(stock_symbol, outcome_status)
	`)
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_outcomes_performance 
		ON signal_outcomes(outcome_status, profit_loss_pct DESC)
	`)

	// Create hypertable for whale_alert_followup
	r.db.db.Exec(`
		SELECT create_hypertable('whale_alert_followup', 'alert_time',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`)

	// Add retention policy: 1 year
	r.db.db.Exec(`
		SELECT add_retention_policy('whale_alert_followup', INTERVAL '1 year', if_not_exists => TRUE)
	`)

	// Create indexes for whale_alert_followup
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_followup_impact 
		ON whale_alert_followup(immediate_impact, sustained_impact)
	`)

	// Create hypertable for order_flow_imbalance
	r.db.db.Exec(`
		SELECT create_hypertable('order_flow_imbalance', 'bucket',
			chunk_time_interval => INTERVAL '1 day',
			if_not_exists => TRUE
		)
	`)

	// Add retention policy: 3 months
	r.db.db.Exec(`
		SELECT add_retention_policy('order_flow_imbalance', INTERVAL '3 months', if_not_exists => TRUE)
	`)

	// Create index for order_flow_imbalance
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_flow_symbol_bucket 
		ON order_flow_imbalance(stock_symbol, bucket DESC)
	`)

	fmt.Println("✅ Phase 1 enhancement tables configured successfully")

	// ========================================================================
	// Phase 2 Enhancement Tables
	// ========================================================================

	// 1. Statistical Baselines
	r.db.db.Exec(`
		SELECT create_hypertable('statistical_baselines', 'calculated_at',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`)
	r.db.db.Exec(`
		SELECT add_retention_policy('statistical_baselines', INTERVAL '3 months', if_not_exists => TRUE)
	`)

	// 2. Market Regimes
	r.db.db.Exec(`
		SELECT create_hypertable('market_regimes', 'detected_at',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`)
	r.db.db.Exec(`
		SELECT add_retention_policy('market_regimes', INTERVAL '6 months', if_not_exists => TRUE)
	`)

	// 3. Detected Patterns
	r.db.db.Exec(`
		SELECT create_hypertable('detected_patterns', 'detected_at',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`)
	r.db.db.Exec(`
		SELECT add_retention_policy('detected_patterns', INTERVAL '1 year', if_not_exists => TRUE)
	`)

	// 4. Multi-Timeframe Candles (Continuous Aggregates)

	// 5-minute candles
	r.db.db.Exec(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS candle_5min
		WITH (timescaledb.continuous) AS
		SELECT
			time_bucket('5 minutes', timestamp) AS bucket,
			stock_symbol,
			FIRST(price, timestamp) AS open,
			MAX(price) AS high,
			MIN(price) AS low,
			LAST(price, timestamp) AS close,
			SUM(volume_lot) AS volume_lots,
			SUM(total_amount) AS total_value,
			COUNT(*) AS trade_count
		FROM running_trades
		GROUP BY bucket, stock_symbol
	`)
	r.db.db.Exec(`
		SELECT add_continuous_aggregate_policy('candle_5min',
			start_offset => INTERVAL '15 minutes',
			end_offset => INTERVAL '1 minute',
			schedule_interval => INTERVAL '5 minutes',
			if_not_exists => TRUE
		)
	`)

	// 15-minute candles
	r.db.db.Exec(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS candle_15min
		WITH (timescaledb.continuous) AS
		SELECT
			time_bucket('15 minutes', timestamp) AS bucket,
			stock_symbol,
			FIRST(price, timestamp) AS open,
			MAX(price) AS high,
			MIN(price) AS low,
			LAST(price, timestamp) AS close,
			SUM(volume_lot) AS volume_lots,
			SUM(total_amount) AS total_value,
			COUNT(*) AS trade_count
		FROM running_trades
		GROUP BY bucket, stock_symbol
	`)
	r.db.db.Exec(`
		SELECT add_continuous_aggregate_policy('candle_15min',
			start_offset => INTERVAL '45 minutes',
			end_offset => INTERVAL '1 minute',
			schedule_interval => INTERVAL '15 minutes',
			if_not_exists => TRUE
		)
	`)

	// 1-hour candles
	r.db.db.Exec(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS candle_1hour
		WITH (timescaledb.continuous) AS
		SELECT
			time_bucket('1 hour', timestamp) AS bucket,
			stock_symbol,
			FIRST(price, timestamp) AS open,
			MAX(price) AS high,
			MIN(price) AS low,
			LAST(price, timestamp) AS close,
			SUM(volume_lot) AS volume_lots,
			SUM(total_amount) AS total_value,
			COUNT(*) AS trade_count
		FROM running_trades
		GROUP BY bucket, stock_symbol
	`)
	r.db.db.Exec(`
		SELECT add_continuous_aggregate_policy('candle_1hour',
			start_offset => INTERVAL '3 hours',
			end_offset => INTERVAL '1 minute',
			schedule_interval => INTERVAL '1 hour',
			if_not_exists => TRUE
		)
	`)

	// 1-day candles
	r.db.db.Exec(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS candle_1day
		WITH (timescaledb.continuous) AS
		SELECT
			time_bucket('1 day', timestamp) AS bucket,
			stock_symbol,
			FIRST(price, timestamp) AS open,
			MAX(price) AS high,
			MIN(price) AS low,
			LAST(price, timestamp) AS close,
			SUM(volume_lot) AS volume_lots,
			SUM(total_amount) AS total_value,
			COUNT(*) AS trade_count
		FROM running_trades
		GROUP BY bucket, stock_symbol
	`)
	r.db.db.Exec(`
		SELECT add_continuous_aggregate_policy('candle_1day',
			start_offset => INTERVAL '3 days',
			end_offset => INTERVAL '1 hour',
			schedule_interval => INTERVAL '1 day',
			if_not_exists => TRUE
		)
	`)

	fmt.Println("✅ Phase 2 enhancement tables configured successfully")

	// ========================================================================
	// Phase 3 Enhancement Tables
	// ========================================================================

	// 1. Stock Correlations
	r.db.db.Exec(`
		SELECT create_hypertable('stock_correlations', 'calculated_at',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE
		)
	`)
	r.db.db.Exec(`
		SELECT add_retention_policy('stock_correlations', INTERVAL '6 months', if_not_exists => TRUE)
	`)

	// 2. Strategy Performance Daily - Create index for better query performance
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_strategy_performance_daily_lookup
		ON strategy_performance_daily(day, strategy, stock_symbol)
	`)
	fmt.Println("✅ strategy_performance_daily view and index created successfully")

	fmt.Println("✅ Phase 3 enhancement tables configured successfully")
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
func (r *TradeRepository) GetHistoricalWhales(stockSymbol string, startTime, endTime time.Time, alertType string, action string, board string, minAmount float64, limit, offset int) ([]WhaleAlert, error) {
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

	if action != "" {
		query = query.Where("action = ?", action)
	}

	if board != "" {
		query = query.Where("market_board = ?", board)
	}

	if minAmount > 0 {
		query = query.Where("trigger_value >= ?", minAmount)
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
func (r *TradeRepository) GetWhaleCount(stockSymbol string, startTime, endTime time.Time, alertType string, action string, board string, minAmount float64) (int64, error) {
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

	if action != "" {
		query = query.Where("action = ?", action)
	}

	if board != "" {
		query = query.Where("market_board = ?", board)
	}

	if minAmount > 0 {
		query = query.Where("trigger_value >= ?", minAmount)
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
	StockSymbol     string
	Action          string
	AlertCount      int64
	TotalValue      float64
	TotalVolumeLots float64 // Added for average price calculation
	FirstAlertTime  time.Time
	LastAlertTime   time.Time
	AvgZScore       float64
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
			SUM(trigger_volume_lots) as total_volume_lots,
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

// AccumulationDistributionSummary represents accumulation vs distribution summary per symbol
type AccumulationDistributionSummary struct {
	StockSymbol    string  `json:"stock_symbol"`
	BuyCount       int64   `json:"buy_count"`
	SellCount      int64   `json:"sell_count"`
	BuyValue       float64 `json:"buy_value"`
	SellValue      float64 `json:"sell_value"`
	TotalCount     int64   `json:"total_count"`
	TotalValue     float64 `json:"total_value"`
	BuyPercentage  float64 `json:"buy_percentage"`
	SellPercentage float64 `json:"sell_percentage"`
	Status         string  `json:"status"`
	NetValue       float64 `json:"net_value"`
}

// GetAccumulationDistributionSummary returns top 20 accumulation and top 20 distribution separately
// Data is calculated from startTime
func (r *TradeRepository) GetAccumulationDistributionSummary(startTime time.Time) (accumulation []AccumulationDistributionSummary, distribution []AccumulationDistributionSummary, err error) {
	// Default to 24 hours if zero
	if startTime.IsZero() {
		startTime = time.Now().Add(-24 * time.Hour)
	}

	baseQuery := `
		WITH symbol_stats AS (
			SELECT 
				stock_symbol,
				SUM(CASE WHEN action = 'BUY' THEN 1 ELSE 0 END) as buy_count,
				SUM(CASE WHEN action = 'SELL' THEN 1 ELSE 0 END) as sell_count,
				SUM(CASE WHEN action = 'BUY' THEN trigger_value ELSE 0 END) as buy_value,
				SUM(CASE WHEN action = 'SELL' THEN trigger_value ELSE 0 END) as sell_value,
				COUNT(*) as total_count,
				SUM(trigger_value) as total_value
			FROM whale_alerts
			WHERE detected_at >= ?
			GROUP BY stock_symbol
		)
		SELECT 
			stock_symbol,
			buy_count,
			sell_count,
			buy_value,
			sell_value,
			total_count,
			total_value,
			CASE 
				WHEN total_count > 0 THEN ROUND((buy_count::DECIMAL / total_count * 100), 2)
				ELSE 0 
			END as buy_percentage,
			CASE 
				WHEN total_count > 0 THEN ROUND((sell_count::DECIMAL / total_count * 100), 2)
				ELSE 0 
			END as sell_percentage,
			CASE 
				WHEN buy_count > sell_count AND (buy_count::DECIMAL / NULLIF(total_count, 0)) > 0.55 THEN 'ACCUMULATION'
				WHEN sell_count > buy_count AND (sell_count::DECIMAL / NULLIF(total_count, 0)) > 0.55 THEN 'DISTRIBUTION'
				ELSE 'NEUTRAL'
			END as status,
			buy_value - sell_value as net_value
		FROM symbol_stats
	`

	// Get top 20 ACCUMULATION (sorted by net_value DESC - highest positive)
	accumulationQuery := baseQuery + `
		WHERE (buy_count::DECIMAL / NULLIF(total_count, 0)) > 0.55 
		  AND buy_count > sell_count
		ORDER BY net_value DESC
		LIMIT 20
	`
	if err := r.db.db.Raw(accumulationQuery, startTime).Scan(&accumulation).Error; err != nil {
		return nil, nil, err
	}

	// Get top 20 DISTRIBUTION (sorted by net_value ASC - highest negative)
	distributionQuery := baseQuery + `
		WHERE (sell_count::DECIMAL / NULLIF(total_count, 0)) > 0.55 
		  AND sell_count > buy_count
		ORDER BY net_value ASC
		LIMIT 20
	`
	if err := r.db.db.Raw(distributionQuery, startTime).Scan(&distribution).Error; err != nil {
		return nil, nil, err
	}

	return accumulation, distribution, nil
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

// ZScoreData holds z-score calculations for price and volume
type ZScoreData struct {
	PriceZScore  float64
	VolumeZScore float64
	MeanPrice    float64
	StdDevPrice  float64
	MeanVolume   float64
	StdDevVolume float64
	SampleCount  int64
	PriceChange  float64
	VolumeChange float64
}

// GetPriceVolumeZScores calculates real-time z-scores for a stock
// Returns z-scores for current price and volume compared to historical baseline
func (r *TradeRepository) GetPriceVolumeZScores(symbol string, currentPrice, currentVolume float64, lookbackMinutes int) (*ZScoreData, error) {
	var result struct {
		MeanPrice    float64
		StdDevPrice  float64
		MeanVolume   float64
		StdDevVolume float64
		SampleCount  int64
		MinPrice     float64
		MaxPrice     float64
	}

	// Calculate statistics from candle_1min view
	query := `
		SELECT 
			COALESCE(AVG(close), 0) as mean_price,
			COALESCE(STDDEV(close), 0) as std_dev_price,
			COALESCE(AVG(volume_lots), 0) as mean_volume,
			COALESCE(STDDEV(volume_lots), 0) as std_dev_volume,
			COUNT(*) as sample_count,
			COALESCE(MIN(close), 0) as min_price,
			COALESCE(MAX(close), 0) as max_price
		FROM candle_1min
		WHERE stock_symbol = ? 
		AND bucket >= NOW() - INTERVAL '1 minute' * ?
	`

	err := r.db.db.Raw(query, symbol, lookbackMinutes).Scan(&result).Error
	if err != nil {
		return nil, err
	}

	// Calculate z-scores (handle zero standard deviation)
	var priceZScore, volumeZScore float64

	if result.StdDevPrice > 0 {
		priceZScore = (currentPrice - result.MeanPrice) / result.StdDevPrice
	}

	if result.StdDevVolume > 0 {
		volumeZScore = (currentVolume - result.MeanVolume) / result.StdDevVolume
	}

	// Calculate percentage changes
	priceChange := 0.0
	volumeChange := 0.0
	if result.MeanPrice > 0 {
		priceChange = ((currentPrice - result.MeanPrice) / result.MeanPrice) * 100
	}
	if result.MeanVolume > 0 {
		volumeChange = ((currentVolume - result.MeanVolume) / result.MeanVolume) * 100
	}

	return &ZScoreData{
		PriceZScore:  priceZScore,
		VolumeZScore: volumeZScore,
		MeanPrice:    result.MeanPrice,
		StdDevPrice:  result.StdDevPrice,
		MeanVolume:   result.MeanVolume,
		StdDevVolume: result.StdDevVolume,
		SampleCount:  result.SampleCount,
		PriceChange:  priceChange,
		VolumeChange: volumeChange,
	}, nil
}

// EvaluateVolumeBreakoutStrategy implements Volume Breakout Validation strategy
// Logic: Price increase (>2%) + explosive volume (z-score > 3) = BUY signal
func (r *TradeRepository) EvaluateVolumeBreakoutStrategy(alert *WhaleAlert, zscores *ZScoreData) *TradingSignal {
	signal := &TradingSignal{
		StockSymbol:  alert.StockSymbol,
		Timestamp:    alert.DetectedAt,
		Strategy:     "VOLUME_BREAKOUT",
		PriceZScore:  zscores.PriceZScore,
		VolumeZScore: zscores.VolumeZScore,
		Price:        alert.TriggerPrice,
		Volume:       alert.TriggerVolumeLots,
		Change:       zscores.PriceChange,
	}

	// Check conditions: change > 2% AND volume_z_score > 3
	if zscores.PriceChange > 2.0 && zscores.VolumeZScore > 3.0 {
		signal.Decision = "BUY"
		signal.Confidence = calculateConfidence(zscores.VolumeZScore, 3.0, 6.0)
		signal.Reason = fmt.Sprintf("Kenaikan harga %.2f%% didukung volume meledak (Z=%.2f). Entry valid ✓",
			zscores.PriceChange, zscores.VolumeZScore)
	} else if zscores.PriceChange > 2.0 && zscores.VolumeZScore <= 3.0 {
		signal.Decision = "WAIT"
		signal.Confidence = 0.3
		signal.Reason = fmt.Sprintf("Harga naik %.2f%% tapi volume biasa saja (Z=%.2f). Tunggu konfirmasi.",
			zscores.PriceChange, zscores.VolumeZScore)
	} else {
		signal.Decision = "NO_TRADE"
		signal.Confidence = 0.1
		signal.Reason = "Tidak ada breakout signifikan"
	}

	return signal
}

// EvaluateMeanReversionStrategy implements Mean Reversion (Contrarian) strategy
// Logic: Extreme price (z-score > 4) + declining volume = SELL signal (overbought)
func (r *TradeRepository) EvaluateMeanReversionStrategy(alert *WhaleAlert, zscores *ZScoreData, prevVolumeZScore float64) *TradingSignal {
	signal := &TradingSignal{
		StockSymbol:  alert.StockSymbol,
		Timestamp:    alert.DetectedAt,
		Strategy:     "MEAN_REVERSION",
		PriceZScore:  zscores.PriceZScore,
		VolumeZScore: zscores.VolumeZScore,
		Price:        alert.TriggerPrice,
		Volume:       alert.TriggerVolumeLots,
		Change:       zscores.PriceChange,
	}

	// Detect volume divergence
	volumeDeclining := zscores.VolumeZScore < prevVolumeZScore

	// Check conditions: price_z_score > 4 AND volume declining
	if zscores.PriceZScore > 4.0 && volumeDeclining {
		signal.Decision = "SELL"
		signal.Confidence = calculateConfidence(zscores.PriceZScore, 4.0, 7.0)
		signal.Reason = fmt.Sprintf("Harga overextended (Z=%.2f), volume menurun. Mean reversion imminent ⚠️",
			zscores.PriceZScore)
	} else if zscores.PriceZScore > 4.0 {
		signal.Decision = "WAIT"
		signal.Confidence = 0.5
		signal.Reason = fmt.Sprintf("Harga overbought (Z=%.2f) tapi volume masih kuat. Monitor divergence.",
			zscores.PriceZScore)
	} else if zscores.PriceZScore < -4.0 {
		signal.Decision = "BUY"
		signal.Confidence = calculateConfidence(-zscores.PriceZScore, 4.0, 7.0)
		signal.Reason = fmt.Sprintf("Harga oversold (Z=%.2f). Potential bounce 📈",
			zscores.PriceZScore)
	} else {
		signal.Decision = "NO_TRADE"
		signal.Confidence = 0.1
		signal.Reason = "Harga dalam range normal"
	}

	return signal
}

// EvaluateFakeoutFilterStrategy implements Fakeout Filter (Defense) strategy
// Logic: Price breakout + low volume (z-score < 1) = NO_TRADE (likely bull trap)
func (r *TradeRepository) EvaluateFakeoutFilterStrategy(alert *WhaleAlert, zscores *ZScoreData) *TradingSignal {
	signal := &TradingSignal{
		StockSymbol:  alert.StockSymbol,
		Timestamp:    alert.DetectedAt,
		Strategy:     "FAKEOUT_FILTER",
		PriceZScore:  zscores.PriceZScore,
		VolumeZScore: zscores.VolumeZScore,
		Price:        alert.TriggerPrice,
		Volume:       alert.TriggerVolumeLots,
		Change:       zscores.PriceChange,
	}

	// Detect potential breakout (price moving significantly)
	isBreakout := zscores.PriceChange > 3.0 || zscores.PriceZScore > 2.0

	// Check volume strength
	if isBreakout && zscores.VolumeZScore < 1.0 {
		signal.Decision = "NO_TRADE"
		signal.Confidence = 0.8 // High confidence to AVOID
		signal.Reason = fmt.Sprintf("⚠️ FAKEOUT DETECTED! Breakout tanpa volume (Z=%.2f). Bull trap!",
			zscores.VolumeZScore)
	} else if isBreakout && zscores.VolumeZScore >= 2.0 {
		signal.Decision = "BUY"
		signal.Confidence = calculateConfidence(zscores.VolumeZScore, 2.0, 5.0)
		signal.Reason = fmt.Sprintf("✓ Breakout valid dengan volume kuat (Z=%.2f). Safe entry.",
			zscores.VolumeZScore)
	} else if isBreakout {
		signal.Decision = "WAIT"
		signal.Confidence = 0.4
		signal.Reason = fmt.Sprintf("Breakout dengan volume moderate (Z=%.2f). Tunggu konfirmasi.",
			zscores.VolumeZScore)
	} else {
		signal.Decision = "NO_TRADE"
		signal.Confidence = 0.1
		signal.Reason = "Tidak ada breakout terdeteksi"
	}

	return signal
}

// GetStrategySignals evaluates recent whale alerts and generates trading signals
func (r *TradeRepository) GetStrategySignals(lookbackMinutes int, minConfidence float64, strategyFilter string) ([]TradingSignal, error) {
	// Get recent whale alerts
	var alerts []WhaleAlert
	err := r.db.db.Where("detected_at >= NOW() - INTERVAL '1 minute' * ?", lookbackMinutes).
		Order("detected_at DESC").
		Limit(50).
		Find(&alerts).Error

	if err != nil {
		return nil, err
	}

	var signals []TradingSignal

	// Track previous volume z-scores for divergence detection
	prevVolumeZScores := make(map[string]float64)

	for _, alert := range alerts {
		// PHASE 2 ENHANCEMENTS: Fetch statistical metrics
		baseline, _ := r.GetLatestBaseline(alert.StockSymbol)
		regime, _ := r.GetLatestRegime(alert.StockSymbol)
		patterns, _ := r.GetRecentPatterns(alert.StockSymbol, time.Now().Add(-2*time.Hour))

		// Calculate z-scores using persistent baseline if available
		volumeLots := alert.TriggerVolumeLots
		var zscores *ZScoreData
		var err error

		if baseline != nil && baseline.SampleSize > 30 {
			// Calculate Z-Score using persistent baseline (more accurate)
			priceZ := (alert.TriggerPrice - baseline.MeanPrice) / baseline.StdDevPrice
			volZ := (volumeLots - baseline.MeanVolumeLots) / baseline.StdDevVolume
			zscores = &ZScoreData{
				PriceZScore:  priceZ,
				VolumeZScore: volZ,
				SampleCount:  int64(baseline.SampleSize),
			}
		} else {
			// Fallback to real-time calculation
			zscores, err = r.GetPriceVolumeZScores(alert.StockSymbol, alert.TriggerPrice, volumeLots, 60)
			if err != nil || zscores.SampleCount < 10 {
				continue // Skip if insufficient data
			}
		}

		// Evaluate each strategy
		strategies := []string{"VOLUME_BREAKOUT", "MEAN_REVERSION", "FAKEOUT_FILTER"}
		if strategyFilter != "" && strategyFilter != "ALL" {
			strategies = []string{strategyFilter}
		}

		for _, strategy := range strategies {
			var signal *TradingSignal

			switch strategy {
			case "VOLUME_BREAKOUT":
				signal = r.EvaluateVolumeBreakoutStrategy(&alert, zscores)
				// Phase 2: Filter breakouts in RANGING markets
				if signal != nil && regime != nil && regime.Regime == "RANGING" && regime.Confidence > 0.6 {
					signal.Confidence *= 0.5 // Deprioritize
					signal.Reason += " (Filtered: Ranging Market)"
				}
			case "MEAN_REVERSION":
				prevZScore := prevVolumeZScores[alert.StockSymbol]
				signal = r.EvaluateMeanReversionStrategy(&alert, zscores, prevZScore)
				// Phase 2: Boost Mean Reversion in RANGING/VOLATILE markets
				if signal != nil && regime != nil && (regime.Regime == "RANGING" || regime.Regime == "VOLATILE") {
					signal.Confidence *= 1.2
					signal.Reason += " (Regime Boost)"
				}
			case "FAKEOUT_FILTER":
				signal = r.EvaluateFakeoutFilterStrategy(&alert, zscores)
			}

			// Phase 2: Pattern Confirmation
			if signal != nil && len(patterns) > 0 {
				for _, p := range patterns {
					if p.PatternType == "RANGE_BREAKOUT" && p.PatternDirection != nil {
						if *p.PatternDirection == signal.Decision {
							signal.Confidence *= 1.3 // Strong confirmation
							signal.Reason += fmt.Sprintf(" (Confirmed by %s)", p.PatternType)
							break
						}
					}
				}
			}

			// Only include signals meeting confidence threshold
			if signal != nil && signal.Confidence >= minConfidence && signal.Decision != "NO_TRADE" {
				signals = append(signals, *signal)

				// Persist signal to database for tracking (async, don't block on errors)
				go func(sig TradingSignal, alertID int64) {
					signalDB := &TradingSignalDB{
						GeneratedAt:       sig.Timestamp,
						StockSymbol:       sig.StockSymbol,
						Strategy:          sig.Strategy,
						Decision:          sig.Decision,
						Confidence:        sig.Confidence,
						TriggerPrice:      sig.Price,
						TriggerVolumeLots: sig.Volume,
						PriceZScore:       sig.PriceZScore,
						VolumeZScore:      sig.VolumeZScore,
						PriceChangePct:    sig.Change,
						Reason:            sig.Reason,
						WhaleAlertID:      &alertID,
					}

					if err := r.SaveTradingSignal(signalDB); err != nil {
						// Log error but don't fail signal generation
						fmt.Printf("⚠️ Failed to persist signal to DB: %v\n", err)
					}
				}(*signal, alert.ID)
			}
		}

		// Update previous volume z-score
		prevVolumeZScores[alert.StockSymbol] = zscores.VolumeZScore
	}

	// Sort signals by timestamp DESC (newest first), then by strategy name for consistency
	// This is necessary because multiple strategies per alert can create out-of-order results
	sort.Slice(signals, func(i, j int) bool {
		// First, sort by timestamp (newest first)
		if !signals[i].Timestamp.Equal(signals[j].Timestamp) {
			return signals[i].Timestamp.After(signals[j].Timestamp)
		}
		// If timestamps are equal, sort by strategy name alphabetically
		return signals[i].Strategy < signals[j].Strategy
	})

	return signals, nil
}

// calculateConfidence converts z-score range to confidence percentage
func calculateConfidence(value, minThreshold, maxThreshold float64) float64 {
	if value < minThreshold {
		return 0.0
	}
	if value >= maxThreshold {
		return 1.0
	}

	// Prevent division by zero
	denominator := maxThreshold - minThreshold
	if denominator <= 0 {
		return 0.5 // Return neutral confidence if thresholds are invalid
	}

	// Linear interpolation between min and max
	confidence := (value - minThreshold) / denominator

	// Clamp to [0, 1] range to prevent overflow
	if confidence < 0 {
		return 0.0
	}
	if confidence > 1 {
		return 1.0
	}

	return confidence
}

// ============================================================================
// Phase 1 Enhancement Repository Methods
// ============================================================================

// SaveTradingSignal persists a trading signal to the database
func (r *TradeRepository) SaveTradingSignal(signal *TradingSignalDB) error {
	return r.db.db.Create(signal).Error
}

// GetTradingSignals retrieves trading signals with filters
func (r *TradeRepository) GetTradingSignals(symbol string, strategy string, decision string, startTime, endTime time.Time, limit int) ([]TradingSignalDB, error) {
	var signals []TradingSignalDB
	query := r.db.db.Order("generated_at DESC")

	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
	}
	if strategy != "" {
		query = query.Where("strategy = ?", strategy)
	}
	if decision != "" {
		query = query.Where("decision = ?", decision)
	}
	if !startTime.IsZero() {
		query = query.Where("generated_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("generated_at <= ?", endTime)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&signals).Error
	return signals, err
}

// GetSignalByID retrieves a specific signal by ID
func (r *TradeRepository) GetSignalByID(id int64) (*TradingSignalDB, error) {
	var signal TradingSignalDB
	err := r.db.db.First(&signal, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &signal, err
}

// SaveSignalOutcome creates a new signal outcome record
func (r *TradeRepository) SaveSignalOutcome(outcome *SignalOutcome) error {
	return r.db.db.Create(outcome).Error
}

// UpdateSignalOutcome updates an existing signal outcome
func (r *TradeRepository) UpdateSignalOutcome(outcome *SignalOutcome) error {
	return r.db.db.Save(outcome).Error
}

// GetSignalOutcomes retrieves signal outcomes with filters
func (r *TradeRepository) GetSignalOutcomes(symbol string, status string, startTime, endTime time.Time, limit int) ([]SignalOutcome, error) {
	var outcomes []SignalOutcome
	query := r.db.db.Order("entry_time DESC")

	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
	}
	if status != "" {
		query = query.Where("outcome_status = ?", status)
	}
	if !startTime.IsZero() {
		query = query.Where("entry_time >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("entry_time <= ?", endTime)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&outcomes).Error
	return outcomes, err
}

// GetSignalOutcomeBySignalID retrieves outcome for a specific signal
func (r *TradeRepository) GetSignalOutcomeBySignalID(signalID int64) (*SignalOutcome, error) {
	var outcome SignalOutcome
	err := r.db.db.Where("signal_id = ?", signalID).First(&outcome).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &outcome, err
}

// PerformanceStats holds aggregated performance metrics
type PerformanceStats struct {
	Strategy       string  `json:"strategy"`
	StockSymbol    string  `json:"stock_symbol"`
	TotalSignals   int64   `json:"total_signals"`
	Wins           int64   `json:"wins"`
	Losses         int64   `json:"losses"`
	WinRate        float64 `json:"win_rate"`
	AvgProfitPct   float64 `json:"avg_profit_pct"`
	TotalProfitPct float64 `json:"total_profit_pct"`
	MaxWinPct      float64 `json:"max_win_pct"`
	MaxLossPct     float64 `json:"max_loss_pct"`
	AvgRiskReward  float64 `json:"avg_risk_reward"`
	Expectancy     float64 `json:"expectancy"`
}

// GetSignalPerformanceStats calculates performance statistics
func (r *TradeRepository) GetSignalPerformanceStats(strategy string, symbol string) (*PerformanceStats, error) {
	var stats PerformanceStats

	query := `
		SELECT 
			ts.strategy,
			ts.stock_symbol,
			COUNT(*) AS total_signals,
			SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END) AS wins,
			SUM(CASE WHEN so.outcome_status = 'LOSS' THEN 1 ELSE 0 END) AS losses,
			ROUND(
				(SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END)::DECIMAL / 
				 NULLIF(COUNT(*), 0)) * 100, 
				2
			) AS win_rate,
			COALESCE(AVG(so.profit_loss_pct), 0) AS avg_profit_pct,
			COALESCE(SUM(so.profit_loss_pct), 0) AS total_profit_pct,
			COALESCE(MAX(so.profit_loss_pct), 0) AS max_win_pct,
			COALESCE(MIN(so.profit_loss_pct), 0) AS max_loss_pct,
			COALESCE(AVG(so.risk_reward_ratio), 0) AS avg_risk_reward,
			(COALESCE(AVG(so.profit_loss_pct), 0) * 
			 (SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END)::DECIMAL / NULLIF(COUNT(*), 0))
			) AS expectancy
		FROM trading_signals ts
		JOIN signal_outcomes so ON ts.id = so.signal_id
		WHERE so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN')
	`

	if strategy != "" {
		query += " AND ts.strategy = @strategy"
	}
	if symbol != "" {
		query += " AND ts.stock_symbol = @symbol"
	}

	query += " GROUP BY ts.strategy, ts.stock_symbol"

	err := r.db.db.Raw(query, map[string]interface{}{
		"strategy": strategy,
		"symbol":   symbol,
	}).Scan(&stats).Error

	return &stats, err
}

// SaveWhaleFollowup creates a new whale alert followup record
func (r *TradeRepository) SaveWhaleFollowup(followup *WhaleAlertFollowup) error {
	return r.db.db.Create(followup).Error
}

// UpdateWhaleFollowup updates specific fields of a whale followup
func (r *TradeRepository) UpdateWhaleFollowup(alertID int64, updates map[string]interface{}) error {
	return r.db.db.Model(&WhaleAlertFollowup{}).
		Where("whale_alert_id = ?", alertID).
		Updates(updates).Error
}

// GetWhaleFollowup retrieves followup data for a specific whale alert
func (r *TradeRepository) GetWhaleFollowup(alertID int64) (*WhaleAlertFollowup, error) {
	var followup WhaleAlertFollowup
	err := r.db.db.Where("whale_alert_id = ?", alertID).First(&followup).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &followup, err
}

// GetPendingFollowups retrieves whale alerts that need followup updates
func (r *TradeRepository) GetPendingFollowups(maxAge time.Duration) ([]WhaleAlertFollowup, error) {
	var followups []WhaleAlertFollowup
	cutoffTime := time.Now().Add(-maxAge)

	// Get followups where latest price update is still pending
	err := r.db.db.Where("alert_time >= ?", cutoffTime).
		Where("price_1day_later IS NULL"). // Still tracking
		Order("alert_time ASC").
		Find(&followups).Error

	return followups, err
}

// SaveOrderFlowImbalance persists order flow data
func (r *TradeRepository) SaveOrderFlowImbalance(flow *OrderFlowImbalance) error {
	return r.db.db.Create(flow).Error
}

// GetOrderFlowImbalance retrieves order flow data with filters
func (r *TradeRepository) GetOrderFlowImbalance(symbol string, startTime, endTime time.Time, limit int) ([]OrderFlowImbalance, error) {
	var flows []OrderFlowImbalance
	query := r.db.db.Order("bucket DESC")

	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
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

	err := query.Find(&flows).Error
	return flows, err
}

// GetLatestOrderFlow retrieves the most recent order flow for a symbol
func (r *TradeRepository) GetLatestOrderFlow(symbol string) (*OrderFlowImbalance, error) {
	var flow OrderFlowImbalance
	err := r.db.db.Where("stock_symbol = ?", symbol).
		Order("bucket DESC").
		First(&flow).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &flow, err
}

// GetOpenSignals retrieves signals that don't have outcomes yet
func (r *TradeRepository) GetOpenSignals(limit int) ([]TradingSignalDB, error) {
	var signals []TradingSignalDB

	// Subquery to find signal IDs that already have outcomes
	subQuery := r.db.db.Model(&SignalOutcome{}).Select("signal_id")

	// Get signals NOT IN the subquery
	query := r.db.db.Where("id NOT IN (?)", subQuery).
		Where("decision IN ('BUY', 'SELL')"). // Only actionable signals
		Order("generated_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&signals).Error
	return signals, err
}

// ============================================================================
// Phase 2 Enhancement Methods
// ============================================================================

// SaveStatisticalBaseline persists a statistical baseline to the database
func (r *TradeRepository) SaveStatisticalBaseline(baseline *StatisticalBaseline) error {
	return r.db.db.Create(baseline).Error
}

// GetLatestBaseline retrieves the most recent statistical baseline for a symbol
func (r *TradeRepository) GetLatestBaseline(symbol string) (*StatisticalBaseline, error) {
	var baseline StatisticalBaseline
	err := r.db.db.Where("stock_symbol = ?", symbol).Order("calculated_at DESC").First(&baseline).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &baseline, nil
}

// SaveMarketRegime persists a market regime detection to the database
func (r *TradeRepository) SaveMarketRegime(regime *MarketRegime) error {
	return r.db.db.Create(regime).Error
}

// GetLatestRegime retrieves the most recent market regime for a symbol
func (r *TradeRepository) GetLatestRegime(symbol string) (*MarketRegime, error) {
	var regime MarketRegime
	err := r.db.db.Where("stock_symbol = ?", symbol).Order("detected_at DESC").First(&regime).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &regime, nil
}

// SaveDetectedPattern persists a detected chart pattern to the database
func (r *TradeRepository) SaveDetectedPattern(pattern *DetectedPattern) error {
	return r.db.db.Create(pattern).Error
}

// GetRecentPatterns retrieves recently detected patterns for a symbol
func (r *TradeRepository) GetRecentPatterns(symbol string, since time.Time) ([]DetectedPattern, error) {
	var patterns []DetectedPattern
	err := r.db.db.Where("stock_symbol = ? AND detected_at >= ?", symbol, since).Order("detected_at DESC").Find(&patterns).Error
	return patterns, err
}

// UpdatePatternOutcome updates the outcome of a detected pattern
func (r *TradeRepository) UpdatePatternOutcome(id int64, outcome string, breakout bool, maxMove float64) error {
	return r.db.db.Model(&DetectedPattern{}).Where("id = ?", id).Updates(map[string]interface{}{
		"outcome":         outcome,
		"actual_breakout": breakout,
		"max_move_pct":    maxMove,
	}).Error
}

// GetCandlesByTimeframe returns candles for a specific timeframe and symbol
func (r *TradeRepository) GetCandlesByTimeframe(timeframe string, symbol string, limit int) ([]map[string]interface{}, error) {
	var viewName string
	switch timeframe {
	case "1min", "1m":
		viewName = "candle_1min"
	case "5min", "5m":
		viewName = "candle_5min"
	case "15min", "15m":
		viewName = "candle_15min"
	case "1hour", "1h", "60min", "60m":
		viewName = "candle_1hour"
	case "1day", "1d", "daily":
		viewName = "candle_1day"
	default:
		return nil, fmt.Errorf("unsupported timeframe: %s (supported: 1min/1m, 5min/5m, 15min/15m, 1hour/1h, 1day/1d)", timeframe)
	}

	var results []map[string]interface{}
	err := r.db.db.Table(viewName).
		Where("stock_symbol = ?", symbol).
		Order("bucket DESC").
		Limit(limit).
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	// Rename fields for frontend compatibility
	for i := range results {
		if bucket, ok := results[i]["bucket"]; ok {
			results[i]["time"] = bucket
			delete(results[i], "bucket")
		}
		if volumeLots, ok := results[i]["volume_lots"]; ok {
			results[i]["volume"] = volumeLots
			delete(results[i], "volume_lots")
		}
	}

	return results, nil
}

// GetActiveSymbols retrieves symbols that had trades in the specified lookback duration
func (r *TradeRepository) GetActiveSymbols(since time.Time) ([]string, error) {
	var symbols []string
	err := r.db.db.Table("running_trades").
		Where("timestamp >= ?", since).
		Distinct("stock_symbol").
		Pluck("stock_symbol", &symbols).Error
	return symbols, err
}

// GetTradesByTimeRange retrieves trades for a symbol within a time range
func (r *TradeRepository) GetTradesByTimeRange(symbol string, startTime, endTime time.Time) ([]Trade, error) {
	var trades []Trade
	err := r.db.db.Where("stock_symbol = ? AND timestamp >= ? AND timestamp <= ?", symbol, startTime, endTime).
		Order("timestamp ASC").
		Find(&trades).Error
	return trades, err
}

// ============================================================================
// Phase 3 Enhancement Repository Methods
// ============================================================================

// SaveStockCorrelation persists a stock correlation record
func (r *TradeRepository) SaveStockCorrelation(correlation *StockCorrelation) error {
	return r.db.db.Create(correlation).Error
}

// GetStockCorrelations retrieves recent correlations for a symbol
func (r *TradeRepository) GetStockCorrelations(symbol string, limit int) ([]StockCorrelation, error) {
	var correlations []StockCorrelation
	err := r.db.db.Where("stock_a = ? OR stock_b = ?", symbol, symbol).
		Order("calculated_at DESC").
		Limit(limit).
		Find(&correlations).Error
	return correlations, err
}

// GetCorrelationsForPair retrieves historical correlations between two specific stocks
func (r *TradeRepository) GetCorrelationsForPair(stockA, stockB string) ([]StockCorrelation, error) {
	var correlations []StockCorrelation
	err := r.db.db.Where("(stock_a = ? AND stock_b = ?) OR (stock_a = ? AND stock_b = ?)", stockA, stockB, stockB, stockA).
		Order("calculated_at DESC").
		Find(&correlations).Error
	return correlations, err
}

// GetDailyStrategyPerformance retrieves daily aggregated performance data
func (r *TradeRepository) GetDailyStrategyPerformance(strategy, symbol string, limit int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	query := r.db.db.Table("strategy_performance_daily").Order("day DESC")

	if strategy != "" && strategy != "ALL" {
		query = query.Where("strategy = ?", strategy)
	}
	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&results).Error
	return results, err
}

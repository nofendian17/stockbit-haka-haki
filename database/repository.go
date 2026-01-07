package database

import (
	"fmt"
	"log"
	"stockbit-haka-haki/database/analytics"
	models "stockbit-haka-haki/database/models_pkg"
	"stockbit-haka-haki/database/signals"
	"stockbit-haka-haki/database/trades"
	"stockbit-haka-haki/database/types"
	"stockbit-haka-haki/database/whales"
	"time"

	"gorm.io/gorm"
)

// TradeRepository is a facade that delegates to domain-specific repositories
// This maintains backward compatibility while providing a cleaner architecture
type TradeRepository struct {
	db        *Database
	trades    *trades.Repository
	whales    *whales.Repository
	signals   *signals.Repository
	analytics *analytics.Repository
}

// NewTradeRepository creates a new trade repository facade
func NewTradeRepository(db *Database) *TradeRepository {
	analyticsRepo := analytics.NewRepository(db.db)
	signalsRepo := signals.NewRepository(db.db)
	signalsRepo.SetAnalyticsRepository(analyticsRepo)

	return &TradeRepository{
		db:        db,
		trades:    trades.NewRepository(db.db),
		whales:    whales.NewRepository(db.db),
		signals:   signalsRepo,
		analytics: analyticsRepo,
	}
}

// ============================================================================
// Schema Initialization (kept in main repository)
// ============================================================================

// InitSchema performs auto-migration and TimescaleDB setup
func (r *TradeRepository) InitSchema() error {
	fmt.Println("🔄 Starting database schema initialization...")

	// Drop continuous aggregate view if exists to allow table alterations
	if err := r.db.db.Exec("DROP MATERIALIZED VIEW IF EXISTS candle_1min CASCADE").Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to drop view candle_1min: %v\n", err)
	}

	// Create running_trades table manually if not exists
	if err := r.db.db.Exec(`
		CREATE TABLE IF NOT EXISTS running_trades (
			id BIGSERIAL,
			timestamp TIMESTAMPTZ NOT NULL,
			stock_symbol TEXT NOT NULL,
			action TEXT NOT NULL,
			price DOUBLE PRECISION NOT NULL,
			volume BIGINT NOT NULL,
			volume_lot DOUBLE PRECISION NOT NULL,
			total_amount DOUBLE PRECISION NOT NULL,
			market_board TEXT NOT NULL,
			change DOUBLE PRECISION,
			trade_number BIGINT,
			PRIMARY KEY (id, timestamp)
		)
	`).Error; err != nil {
		return fmt.Errorf("failed to create running_trades table: %w", err)
	}

	// Add trade_number column if it doesn't exist
	r.db.db.Exec(`
		ALTER TABLE running_trades
		ADD COLUMN IF NOT EXISTS trade_number BIGINT
	`)

	// Create unique index on (stock_symbol, trade_number, market_board, date)
	r.db.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_running_trades_unique_trade
		ON running_trades (stock_symbol, trade_number, market_board, (timestamp::DATE))
		WHERE trade_number IS NOT NULL
	`)

	// Create regular index on trade_number
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_running_trades_trade_number 
		ON running_trades (trade_number)
		WHERE trade_number IS NOT NULL
	`)

	// Create all hypertable tables
	if err := r.createHypertableTables(); err != nil {
		return err
	}

	// Create indexes
	if err := r.createIndexes(); err != nil {
		return err
	}

	// Auto-migrate remaining tables
	if err := r.db.db.AutoMigrate(&WhaleWebhook{}); err != nil {
		return fmt.Errorf("auto-migration failed: %w", err)
	}

	// Create strategy_performance_daily view
	if err := r.createPerformanceView(); err != nil {
		return err
	}

	// Manual migrations for whale_alert_followup columns
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
		ADD COLUMN IF NOT EXISTS volume_15min_later DECIMAL(15,2),
		ADD COLUMN IF NOT EXISTS analysis TEXT
	`)

	// Setup TimescaleDB extension and hypertables
	if err := r.setupTimescaleDB(); err != nil {
		return err
	}

	// Setup enhancement tables
	if err := r.setupEnhancedTables(); err != nil {
		return err
	}

	fmt.Println("✅ Database schema initialization completed successfully")
	return nil
}

// createHypertableTables creates all hypertable tables
func (r *TradeRepository) createHypertableTables() error {
	tables := []string{
		`whale_alerts (
			id BIGSERIAL,
			detected_at TIMESTAMPTZ NOT NULL,
			stock_symbol TEXT NOT NULL,
			alert_type TEXT NOT NULL,
			action TEXT NOT NULL,
			trigger_price DECIMAL(15,2),
			trigger_volume_lots DECIMAL(15,2),
			trigger_value DECIMAL(20,2),
			pattern_duration_sec INTEGER,
			pattern_trade_count INTEGER,
			total_pattern_volume DECIMAL(15,2),
			total_pattern_value DECIMAL(20,2),
			z_score DECIMAL(10,4),
			volume_vs_avg_pct DECIMAL(10,2),
			avg_price DECIMAL(15,2),
			confidence_score DECIMAL(5,2) NOT NULL,
			market_board TEXT,
			PRIMARY KEY (id, detected_at)
		)`,
		`whale_webhook_logs (
			id BIGSERIAL,
			webhook_id INTEGER NOT NULL,
			whale_alert_id BIGINT,
			triggered_at TIMESTAMPTZ NOT NULL,
			status TEXT,
			http_status_code INTEGER,
			response_body TEXT,
			error_message TEXT,
			retry_attempt INTEGER DEFAULT 0,
			PRIMARY KEY (id, triggered_at)
		)`,
		`trading_signals (
			id BIGSERIAL,
			generated_at TIMESTAMPTZ NOT NULL,
			stock_symbol TEXT NOT NULL,
			strategy TEXT NOT NULL,
			decision TEXT NOT NULL,
			confidence DECIMAL(5,2) NOT NULL,
			trigger_price DECIMAL(15,2),
			trigger_volume_lots DECIMAL(15,2),
			price_z_score DECIMAL(10,4),
			volume_z_score DECIMAL(10,4),
			price_change_pct DECIMAL(10,4),
			reason TEXT,
			market_regime TEXT,
			volume_imbalance_ratio DECIMAL(10,4),
			whale_alert_id BIGINT,
			PRIMARY KEY (id, generated_at)
		)`,
		`signal_outcomes (
			id BIGSERIAL,
			signal_id BIGINT NOT NULL,
			stock_symbol TEXT NOT NULL,
			entry_time TIMESTAMPTZ NOT NULL,
			entry_price DECIMAL(15,2) NOT NULL,
			entry_decision TEXT NOT NULL,
			exit_time TIMESTAMPTZ,
			exit_price DECIMAL(15,2),
			exit_reason TEXT,
			holding_period_minutes INTEGER,
			price_change_pct DECIMAL(10,4),
			profit_loss_pct DECIMAL(10,4),
			max_favorable_excursion DECIMAL(10,4),
			max_adverse_excursion DECIMAL(10,4),
			risk_reward_ratio DECIMAL(10,4),
			outcome_status TEXT,
			PRIMARY KEY (id, entry_time)
		)`,
		`whale_alert_followup (
			id BIGSERIAL,
			whale_alert_id BIGINT NOT NULL,
			stock_symbol TEXT NOT NULL,
			alert_time TIMESTAMPTZ NOT NULL,
			alert_price DECIMAL(15,2) NOT NULL,
			alert_action TEXT NOT NULL,
			price_1min_later DECIMAL(15,2),
			price_5min_later DECIMAL(15,2),
			price_15min_later DECIMAL(15,2),
			price_30min_later DECIMAL(15,2),
			price_60min_later DECIMAL(15,2),
			price_1day_later DECIMAL(15,2),
			change_1min_pct DECIMAL(10,4),
			change_5min_pct DECIMAL(10,4),
			change_15min_pct DECIMAL(10,4),
			change_30min_pct DECIMAL(10,4),
			change_60min_pct DECIMAL(10,4),
			change_1day_pct DECIMAL(10,4),
			volume_1min_later DECIMAL(15,2),
			volume_5min_later DECIMAL(15,2),
			volume_15min_later DECIMAL(15,2),
			immediate_impact TEXT,
			sustained_impact TEXT,
			reversal_detected BOOLEAN,
			reversal_time_minutes INTEGER,
			PRIMARY KEY (id, alert_time)
		)`,
		`order_flow_imbalance (
			id BIGSERIAL,
			bucket TIMESTAMPTZ NOT NULL,
			stock_symbol TEXT NOT NULL,
			buy_volume_lots DECIMAL(15,2) NOT NULL,
			sell_volume_lots DECIMAL(15,2) NOT NULL,
			buy_trade_count INTEGER NOT NULL,
			sell_trade_count INTEGER NOT NULL,
			buy_value DECIMAL(20,2),
			sell_value DECIMAL(20,2),
			volume_imbalance_ratio DECIMAL(10,4),
			value_imbalance_ratio DECIMAL(10,4),
			delta_volume DECIMAL(15,2),
			aggressive_buy_pct DECIMAL(5,2),
			aggressive_sell_pct DECIMAL(5,2),
			PRIMARY KEY (id, bucket),
			UNIQUE (bucket, stock_symbol)
		)`,
		`statistical_baselines (
			id BIGSERIAL,
			stock_symbol TEXT NOT NULL,
			calculated_at TIMESTAMPTZ NOT NULL,
			lookback_hours INTEGER NOT NULL,
			sample_size INTEGER,
			mean_price DECIMAL(15,2),
			std_dev_price DECIMAL(15,4),
			median_price DECIMAL(15,2),
			price_p25 DECIMAL(15,2),
			price_p75 DECIMAL(15,2),
			mean_volume_lots DECIMAL(15,2),
			std_dev_volume DECIMAL(15,4),
			median_volume_lots DECIMAL(15,2),
			volume_p25 DECIMAL(15,2),
			volume_p75 DECIMAL(15,2),
			mean_value DECIMAL(20,2),
			std_dev_value DECIMAL(20,4),
			PRIMARY KEY (id, calculated_at)
		)`,
		`market_regimes (
			id BIGSERIAL,
			stock_symbol TEXT NOT NULL,
			detected_at TIMESTAMPTZ NOT NULL,
			lookback_periods INTEGER NOT NULL,
			regime TEXT NOT NULL,
			confidence DECIMAL(5,4),
			adx DECIMAL(10,4),
			atr DECIMAL(15,4),
			bollinger_width DECIMAL(10,4),
			price_change_pct DECIMAL(10,4),
			volatility DECIMAL(10,4),
			PRIMARY KEY (id, detected_at)
		)`,
		`detected_patterns (
			id BIGSERIAL,
			stock_symbol TEXT NOT NULL,
			detected_at TIMESTAMPTZ NOT NULL,
			pattern_type TEXT NOT NULL,
			pattern_direction TEXT,
			confidence DECIMAL(5,4),
			pattern_start TIMESTAMPTZ,
			pattern_end TIMESTAMPTZ,
			price_range DECIMAL(15,2),
			volume_profile TEXT,
			breakout_level DECIMAL(15,2),
			target_price DECIMAL(15,2),
			stop_loss DECIMAL(15,2),
			outcome TEXT,
			actual_breakout BOOLEAN,
			max_move_pct DECIMAL(10,4),
			llm_analysis TEXT,
			PRIMARY KEY (id, detected_at)
		)`,
		`stock_correlations (
			id BIGSERIAL,
			stock_a TEXT NOT NULL,
			stock_b TEXT NOT NULL,
			calculated_at TIMESTAMPTZ NOT NULL,
			correlation_coefficient DOUBLE PRECISION,
			lookback_days INTEGER,
			period TEXT,
			PRIMARY KEY (id, calculated_at)
		)`,
	}

	for _, table := range tables {
		if err := r.db.db.Exec("CREATE TABLE IF NOT EXISTS " + table).Error; err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

// createIndexes creates all database indexes
func (r *TradeRepository) createIndexes() error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_whale_alerts_symbol ON whale_alerts(stock_symbol)",
		"CREATE INDEX IF NOT EXISTS idx_whale_alerts_detected ON whale_alerts(detected_at)",
		"CREATE INDEX IF NOT EXISTS idx_whale_webhook_logs_webhook ON whale_webhook_logs(webhook_id)",
		"CREATE INDEX IF NOT EXISTS idx_trading_signals_symbol ON trading_signals(stock_symbol, strategy, generated_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_trading_signals_decision ON trading_signals(decision, confidence DESC)",
		"CREATE INDEX IF NOT EXISTS idx_signal_outcomes_signal ON signal_outcomes(signal_id)",
		"CREATE INDEX IF NOT EXISTS idx_signal_outcomes_symbol ON signal_outcomes(stock_symbol, outcome_status)",
		"CREATE INDEX IF NOT EXISTS idx_whale_followup_alert ON whale_alert_followup(whale_alert_id)",
		"CREATE INDEX IF NOT EXISTS idx_baselines_symbol_time ON statistical_baselines(stock_symbol, calculated_at)",
		"CREATE INDEX IF NOT EXISTS idx_regimes_symbol_time ON market_regimes(stock_symbol, detected_at)",
		"CREATE INDEX IF NOT EXISTS idx_regimes_regime ON market_regimes(regime, confidence)",
		"CREATE INDEX IF NOT EXISTS idx_patterns_symbol_time ON detected_patterns(stock_symbol, detected_at, pattern_type)",
		"CREATE INDEX IF NOT EXISTS idx_patterns_outcome ON detected_patterns(outcome)",
		"CREATE INDEX IF NOT EXISTS idx_correlations_pair ON stock_correlations(stock_a, stock_b, calculated_at)",
	}

	for _, idx := range indexes {
		if err := r.db.db.Exec(idx).Error; err != nil {
			fmt.Printf("⚠️ Warning: Failed to create index: %v\n", err)
		}
	}

	return nil
}

// createPerformanceView creates the strategy performance materialized view
func (r *TradeRepository) createPerformanceView() error {
	fmt.Println("📊 Creating strategy_performance_daily materialized view...")

	// Drop existing view if it exists to recreate with proper schema
	r.db.db.Exec(`DROP MATERIALIZED VIEW IF EXISTS strategy_performance_daily`)

	if err := r.db.db.Exec(`
		CREATE MATERIALIZED VIEW strategy_performance_daily AS
		SELECT
			DATE(so.entry_time) AS day,
			so.stock_symbol,
			ts.strategy,
			COUNT(*) AS total_signals,
			SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END) AS wins,
			SUM(CASE WHEN so.outcome_status = 'LOSS' THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN so.outcome_status = 'BREAKEVEN' THEN 1 ELSE 0 END) AS breakeven,
			SUM(CASE WHEN so.outcome_status = 'OPEN' THEN 1 ELSE 0 END) AS open_positions,
			ROUND(
				(SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END)::DECIMAL /
				 NULLIF(SUM(CASE WHEN so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN') THEN 1 ELSE 0 END), 0)) * 100,
				2
			) AS win_rate,
			COALESCE(AVG(CASE WHEN so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN') THEN so.profit_loss_pct END), 0) AS avg_profit_pct,
			COALESCE(SUM(CASE WHEN so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN') THEN so.profit_loss_pct END), 0) AS total_profit_pct,
			COALESCE(MAX(so.profit_loss_pct), 0) AS best_trade_pct,
			COALESCE(MIN(so.profit_loss_pct), 0) AS worst_trade_pct,
			COALESCE(AVG(CASE WHEN so.outcome_status = 'WIN' THEN so.profit_loss_pct END), 0) AS avg_win_pct,
			COALESCE(AVG(CASE WHEN so.outcome_status = 'LOSS' THEN so.profit_loss_pct END), 0) AS avg_loss_pct,
			COALESCE(AVG(CASE WHEN so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN') THEN so.risk_reward_ratio END), 0) AS avg_risk_reward,
			COALESCE(AVG(so.entry_price), 0) AS avg_entry_price,
			COALESCE(AVG(CASE WHEN so.exit_price IS NOT NULL THEN so.exit_price END), 0) AS avg_exit_price,
			COALESCE(AVG(CASE WHEN so.holding_period_minutes IS NOT NULL THEN so.holding_period_minutes END), 0) AS avg_holding_minutes
		FROM signal_outcomes so
		JOIN trading_signals ts ON so.signal_id = ts.id
		WHERE so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN', 'OPEN')
		GROUP BY DATE(so.entry_time), so.stock_symbol, ts.strategy
		ORDER BY day DESC, so.stock_symbol, ts.strategy
	`).Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to create view strategy_performance_daily: %v\n", err)
		return err
	}

	fmt.Println("✅ strategy_performance_daily view created successfully")

	// Create indexes for faster queries
	fmt.Println("📑 Creating indexes for strategy_performance_daily...")
	r.db.db.Exec(`CREATE INDEX IF NOT EXISTS idx_strategy_performance_daily_day ON strategy_performance_daily(day DESC)`)
	r.db.db.Exec(`CREATE INDEX IF NOT EXISTS idx_strategy_performance_daily_symbol ON strategy_performance_daily(stock_symbol)`)
	r.db.db.Exec(`CREATE INDEX IF NOT EXISTS idx_strategy_performance_daily_strategy ON strategy_performance_daily(strategy)`)
	r.db.db.Exec(`CREATE INDEX IF NOT EXISTS idx_strategy_performance_daily_lookup ON strategy_performance_daily(day, strategy, stock_symbol)`)

	fmt.Println("🔄 Performing initial refresh of strategy_performance_daily...")
	if err := r.db.db.Exec(`REFRESH MATERIALIZED VIEW strategy_performance_daily`).Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to refresh view: %v (This is normal if there's no data yet)\n", err)
	} else {
		fmt.Println("✅ Initial refresh completed")
	}

	return nil
}

// setupTimescaleDB creates hypertables and policies
func (r *TradeRepository) setupTimescaleDB() error {
	fmt.Println("⏰ Setting up TimescaleDB extension and hypertables...")

	if err := r.db.db.Exec("CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE").Error; err != nil {
		return fmt.Errorf("failed to create TimescaleDB extension: %w", err)
	}
	fmt.Println("✅ TimescaleDB extension enabled")

	// Create hypertables
	hypertables := []struct {
		table      string
		timeColumn string
		chunk      string
		retention  string
	}{
		{"running_trades", "timestamp", "INTERVAL '1 day'", "INTERVAL '3 months'"},
		{"whale_alerts", "detected_at", "INTERVAL '7 days'", "INTERVAL '1 year'"},
		{"whale_webhook_logs", "triggered_at", "INTERVAL '7 days'", "INTERVAL '30 days'"},
	}

	for _, ht := range hypertables {
		if err := r.db.db.Exec(`
			SELECT create_hypertable('` + ht.table + `', '` + ht.timeColumn + `',
				chunk_time_interval => ` + ht.chunk + `,
				if_not_exists => TRUE,
				migrate_data => TRUE
			)
		`).Error; err != nil {
			fmt.Printf("⚠️ Warning: Failed to create hypertable for %s: %v\n", ht.table, err)
			continue
		}

		if err := r.db.db.Exec(`
			SELECT add_retention_policy('` + ht.table + `', ` + ht.retention + `, if_not_exists => TRUE)
		`).Error; err != nil {
			fmt.Printf("⚠️ Warning: Failed to add retention policy for %s: %v\n", ht.table, err)
		}
	}

	// Create continuous aggregate for 1-minute candles
	if err := r.db.db.Exec(`
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
	`).Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to create candle_1min view: %v\n", err)
	} else {
		r.db.db.Exec(`
			SELECT add_continuous_aggregate_policy('candle_1min',
				start_offset => INTERVAL '3 minutes',
				end_offset => INTERVAL '1 minute',
				schedule_interval => INTERVAL '1 minute',
				if_not_exists => TRUE
			)
		`)
		r.db.db.Exec(`
			SELECT add_retention_policy('candle_1min', INTERVAL '10 years', if_not_exists => TRUE)
		`)
	}

	return nil
}

// setupEnhancedTables creates hypertables and policies for enhancement tables
func (r *TradeRepository) setupEnhancedTables() error {
	// Phase 1 enhancement tables
	phase1Tables := []struct {
		table     string
		timeCol   string
		chunk     string
		retention string
	}{
		{"trading_signals", "generated_at", "INTERVAL '7 days'", "INTERVAL '2 years'"},
		{"signal_outcomes", "entry_time", "INTERVAL '7 days'", "INTERVAL '2 years'"},
		{"whale_alert_followup", "alert_time", "INTERVAL '7 days'", "INTERVAL '1 year'"},
		{"order_flow_imbalance", "bucket", "INTERVAL '1 day'", "INTERVAL '3 months'"},
	}

	for _, t := range phase1Tables {
		if err := r.db.db.Exec(`
			SELECT create_hypertable('` + t.table + `', '` + t.timeCol + `',
				chunk_time_interval => ` + t.chunk + `,
				if_not_exists => TRUE,
				migrate_data => TRUE
			)
		`).Error; err != nil {
			fmt.Printf("⚠️ Warning: Failed to create hypertable for %s: %v\n", t.table, err)
			continue
		}
		r.db.db.Exec(`
			SELECT add_retention_policy('` + t.table + `', ` + t.retention + `, if_not_exists => TRUE)
		`)
	}

	// Phase 2 enhancement tables
	phase2Tables := []struct {
		table     string
		timeCol   string
		chunk     string
		retention string
	}{
		{"statistical_baselines", "calculated_at", "INTERVAL '7 days'", "INTERVAL '3 months'"},
		{"market_regimes", "detected_at", "INTERVAL '7 days'", "INTERVAL '6 months'"},
		{"detected_patterns", "detected_at", "INTERVAL '7 days'", "INTERVAL '1 year'"},
	}

	for _, t := range phase2Tables {
		if err := r.db.db.Exec(`
			SELECT create_hypertable('` + t.table + `', '` + t.timeCol + `',
				chunk_time_interval => ` + t.chunk + `,
				if_not_exists => TRUE,
				migrate_data => TRUE
			)
		`).Error; err != nil {
			fmt.Printf("⚠️ Warning: Failed to create hypertable for %s: %v\n", t.table, err)
			continue
		}
		r.db.db.Exec(`
			SELECT add_retention_policy('` + t.table + `', ` + t.retention + `, if_not_exists => TRUE)
		`)
	}

	// Multi-timeframe candles
	timeframes := []struct {
		view     string
		start    string
		end      string
		schedule string
	}{
		{"candle_5min", "INTERVAL '15 minutes'", "INTERVAL '1 minute'", "INTERVAL '5 minutes'"},
		{"candle_15min", "INTERVAL '45 minutes'", "INTERVAL '1 minute'", "INTERVAL '15 minutes'"},
		{"candle_1hour", "INTERVAL '3 hours'", "INTERVAL '1 minute'", "INTERVAL '1 hour'"},
		{"candle_1day", "INTERVAL '3 days'", "INTERVAL '1 hour'", "INTERVAL '1 day'"},
	}

	for _, tf := range timeframes {
		if err := r.db.db.Exec(`
			CREATE MATERIALIZED VIEW IF NOT EXISTS ` + tf.view + `
			WITH (timescaledb.continuous) AS
			SELECT
				time_bucket('` + tf.view[7:] + `', timestamp) AS bucket,
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
		`).Error; err != nil {
			fmt.Printf("⚠️ Warning: Failed to create %s view: %v\n", tf.view, err)
			continue
		}

		r.db.db.Exec(`
			SELECT add_continuous_aggregate_policy('` + tf.view + `',
				start_offset => ` + tf.start + `,
				end_offset => ` + tf.end + `,
				schedule_interval => ` + tf.schedule + `,
				if_not_exists => TRUE
			)
		`)
	}

	// Phase 3 enhancement tables
	if err := r.db.db.Exec(`
		SELECT create_hypertable('stock_correlations', 'calculated_at',
			chunk_time_interval => INTERVAL '7 days',
			if_not_exists => TRUE,
			migrate_data => TRUE
		)
	`).Error; err != nil {
		fmt.Printf("⚠️ Warning: Failed to create hypertable for stock_correlations: %v\n", err)
	} else {
		r.db.db.Exec(`
			SELECT add_retention_policy('stock_correlations', INTERVAL '6 months', if_not_exists => TRUE)
		`)
	}

	// Create index for strategy_performance_daily
	r.db.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_strategy_performance_daily_lookup
		ON strategy_performance_daily(day, strategy, stock_symbol)
	`)

	fmt.Println("✅ All enhancement tables configured successfully")
	return nil
}

// ============================================================================
// Facade Methods - Delegate to domain repositories
// ============================================================================

// Trade methods
func (r *TradeRepository) SaveTrade(trade *Trade) error {
	return r.trades.SaveTrade(trade)
}

func (r *TradeRepository) GetRecentTrades(stockSymbol string, limit int, actionFilter string) ([]Trade, error) {
	return r.trades.GetRecentTrades(stockSymbol, limit, actionFilter)
}

func (r *TradeRepository) GetCandles(stockSymbol string, startTime, endTime time.Time, limit int) ([]Candle, error) {
	return r.trades.GetCandles(stockSymbol, startTime, endTime, limit)
}

func (r *TradeRepository) GetLatestCandle(stockSymbol string) (*Candle, error) {
	return r.trades.GetLatestCandle(stockSymbol)
}

func (r *TradeRepository) GetCandlesByTimeframe(timeframe string, symbol string, limit int) ([]map[string]interface{}, error) {
	return r.trades.GetCandlesByTimeframe(timeframe, symbol, limit)
}

func (r *TradeRepository) GetActiveSymbols(since time.Time) ([]string, error) {
	return r.trades.GetActiveSymbols(since)
}

func (r *TradeRepository) GetTradesByTimeRange(symbol string, startTime, endTime time.Time) ([]Trade, error) {
	return r.trades.GetTradesByTimeRange(symbol, startTime, endTime)
}

func (r *TradeRepository) GetStockStats(symbol string, lookbackMinutes int) (*types.StockStats, error) {
	return r.trades.GetStockStats(symbol, lookbackMinutes)
}

func (r *TradeRepository) GetPriceVolumeZScores(symbol string, currentPrice, currentVolume float64, lookbackMinutes int) (*types.ZScoreData, error) {
	return r.trades.GetPriceVolumeZScores(symbol, currentPrice, currentVolume, lookbackMinutes)
}

// Whale methods
func (r *TradeRepository) SaveWhaleAlert(alert *WhaleAlert) error {
	return r.whales.SaveWhaleAlert(alert)
}

func (r *TradeRepository) GetHistoricalWhales(stockSymbol string, startTime, endTime time.Time, alertType string, action string, board string, minAmount float64, limit, offset int) ([]WhaleAlert, error) {
	return r.whales.GetHistoricalWhales(stockSymbol, startTime, endTime, alertType, action, board, minAmount, limit, offset)
}

func (r *TradeRepository) GetWhaleAlertByID(id int64) (*WhaleAlert, error) {
	return r.whales.GetWhaleAlertByID(id)
}

func (r *TradeRepository) GetWhaleCount(stockSymbol string, startTime, endTime time.Time, alertType string, action string, board string, minAmount float64) (int64, error) {
	return r.whales.GetWhaleCount(stockSymbol, startTime, endTime, alertType, action, board, minAmount)
}

func (r *TradeRepository) GetWhaleStats(stockSymbol string, startTime, endTime time.Time) (*WhaleStats, error) {
	return r.whales.GetWhaleStats(stockSymbol, startTime, endTime)
}

func (r *TradeRepository) GetAccumulationPattern(hoursBack int, minAlerts int) ([]types.AccumulationPattern, error) {
	return r.whales.GetAccumulationPattern(hoursBack, minAlerts)
}

func (r *TradeRepository) GetAccumulationDistributionSummary(startTime time.Time) (accumulation []types.AccumulationDistributionSummary, distribution []types.AccumulationDistributionSummary, err error) {
	return r.whales.GetAccumulationDistributionSummary(startTime)
}

func (r *TradeRepository) GetExtremeAnomalies(minZScore float64, hoursBack int) ([]WhaleAlert, error) {
	return r.whales.GetExtremeAnomalies(minZScore, hoursBack)
}

func (r *TradeRepository) GetTimeBasedStats(daysBack int) ([]types.TimeBasedStat, error) {
	return r.whales.GetTimeBasedStats(daysBack)
}

func (r *TradeRepository) GetRecentAlertsBySymbol(symbol string, limit int) ([]WhaleAlert, error) {
	return r.whales.GetRecentAlertsBySymbol(symbol, limit)
}

func (r *TradeRepository) SaveWhaleFollowup(followup *WhaleAlertFollowup) error {
	return r.whales.SaveWhaleFollowup(followup)
}

func (r *TradeRepository) UpdateWhaleFollowup(alertID int64, updates map[string]interface{}) error {
	return r.whales.UpdateWhaleFollowup(alertID, updates)
}

func (r *TradeRepository) GetWhaleFollowup(alertID int64) (*WhaleAlertFollowup, error) {
	return r.whales.GetWhaleFollowup(alertID)
}

func (r *TradeRepository) GetPendingFollowups(maxAge time.Duration) ([]WhaleAlertFollowup, error) {
	return r.whales.GetPendingFollowups(maxAge)
}

func (r *TradeRepository) GetWhaleFollowups(symbol, status string, limit int) ([]WhaleAlertFollowup, error) {
	return r.whales.GetWhaleFollowups(symbol, status, limit)
}

func (r *TradeRepository) GetActiveWebhooks() ([]WhaleWebhook, error) {
	return r.whales.GetActiveWebhooks()
}

func (r *TradeRepository) SaveWebhookLog(log *WhaleWebhookLog) error {
	return r.whales.SaveWebhookLog(log)
}

// Signal methods
func (r *TradeRepository) SaveTradingSignal(signal *TradingSignalDB) error {
	return r.signals.SaveTradingSignal(signal)
}

func (r *TradeRepository) GetTradingSignals(symbol string, strategy string, decision string, startTime, endTime time.Time, limit int) ([]TradingSignalDB, error) {
	return r.signals.GetTradingSignals(symbol, strategy, decision, startTime, endTime, limit)
}

func (r *TradeRepository) GetSignalByID(id int64) (*TradingSignalDB, error) {
	return r.signals.GetSignalByID(id)
}

func (r *TradeRepository) SaveSignalOutcome(outcome *SignalOutcome) error {
	return r.signals.SaveSignalOutcome(outcome)
}

func (r *TradeRepository) UpdateSignalOutcome(outcome *SignalOutcome) error {
	return r.signals.UpdateSignalOutcome(outcome)
}

func (r *TradeRepository) GetSignalOutcomes(symbol string, status string, startTime, endTime time.Time, limit int) ([]SignalOutcome, error) {
	return r.signals.GetSignalOutcomes(symbol, status, startTime, endTime, limit)
}

func (r *TradeRepository) GetSignalOutcomeBySignalID(signalID int64) (*SignalOutcome, error) {
	return r.signals.GetSignalOutcomeBySignalID(signalID)
}

func (r *TradeRepository) GetOpenSignals(limit int) ([]TradingSignalDB, error) {
	return r.signals.GetOpenSignals(limit)
}

func (r *TradeRepository) GetSignalPerformanceStats(strategy string, symbol string) (*types.PerformanceStats, error) {
	return r.signals.GetSignalPerformanceStats(strategy, symbol)
}

func (r *TradeRepository) GetGlobalPerformanceStats() (*types.PerformanceStats, error) {
	return r.signals.GetGlobalPerformanceStats()
}

func (r *TradeRepository) GetDailyStrategyPerformance(strategy, symbol string, limit int) ([]map[string]interface{}, error) {
	return r.signals.GetDailyStrategyPerformance(strategy, symbol, limit)
}

func (r *TradeRepository) EvaluateVolumeBreakoutStrategy(alert *models.WhaleAlert, zscores *types.ZScoreData, vwap float64) *TradingSignal {
	signal := r.signals.EvaluateVolumeBreakoutStrategy(alert, zscores, nil, vwap)
	// Convert models.TradingSignal back to TradingSignal
	return &TradingSignal{
		StockSymbol:  signal.StockSymbol,
		Timestamp:    signal.Timestamp,
		Strategy:     signal.Strategy,
		Decision:     signal.Decision,
		PriceZScore:  signal.PriceZScore,
		VolumeZScore: signal.VolumeZScore,
		Price:        signal.Price,
		Volume:       signal.Volume,
		Change:       signal.Change,
		Confidence:   signal.Confidence,
		Reason:       signal.Reason,
	}
}

func (r *TradeRepository) EvaluateMeanReversionStrategy(alert *models.WhaleAlert, zscores *types.ZScoreData, prevVolumeZScore float64, vwap float64) *TradingSignal {
	signal := r.signals.EvaluateMeanReversionStrategy(alert, zscores, prevVolumeZScore, nil, vwap)
	// Convert models.TradingSignal back to TradingSignal
	return &TradingSignal{
		StockSymbol:  signal.StockSymbol,
		Timestamp:    signal.Timestamp,
		Strategy:     signal.Strategy,
		Decision:     signal.Decision,
		PriceZScore:  signal.PriceZScore,
		VolumeZScore: signal.VolumeZScore,
		Price:        signal.Price,
		Volume:       signal.Volume,
		Change:       signal.Change,
		Confidence:   signal.Confidence,
		Reason:       signal.Reason,
	}
}

func (r *TradeRepository) EvaluateFakeoutFilterStrategy(alert *models.WhaleAlert, zscores *types.ZScoreData, vwap float64) *TradingSignal {
	signal := r.signals.EvaluateFakeoutFilterStrategy(alert, zscores, nil, vwap)
	// Convert models.TradingSignal back to TradingSignal
	return &TradingSignal{
		StockSymbol:  signal.StockSymbol,
		Timestamp:    signal.Timestamp,
		Strategy:     signal.Strategy,
		Decision:     signal.Decision,
		PriceZScore:  signal.PriceZScore,
		VolumeZScore: signal.VolumeZScore,
		Price:        signal.Price,
		Volume:       signal.Volume,
		Change:       signal.Change,
		Confidence:   signal.Confidence,
		Reason:       signal.Reason,
	}
}

func (r *TradeRepository) GetStrategySignals(lookbackMinutes int, minConfidence float64, strategyFilter string) ([]TradingSignal, error) {
	// Get recent whale alerts
	var alerts []models.WhaleAlert
	err := r.db.db.Where("detected_at >= NOW() - INTERVAL '1 minute' * ?", lookbackMinutes).
		Where("market_board != 'NG' OR market_board IS NULL").
		Order("detected_at DESC").
		Limit(50).
		Find(&alerts).Error

	if err != nil {
		log.Printf("❌ Error fetching whale alerts: %v", err)
		return nil, err
	}

	log.Printf("📊 Found %d whale alerts in last %d minutes", len(alerts), lookbackMinutes)

	// Get signals from signals repository
	modelSignals, err := r.signals.GetStrategySignals(lookbackMinutes, minConfidence, strategyFilter, alerts)
	if err != nil {
		log.Printf("❌ Error generating strategy signals: %v", err)
		return nil, err
	}

	log.Printf("✅ Generated %d strategy signals (min confidence: %.2f)", len(modelSignals), minConfidence)

	// Convert models.TradingSignal to database.TradingSignal
	signals := make([]TradingSignal, len(modelSignals))
	for i, ms := range modelSignals {
		signals[i] = TradingSignal{
			StockSymbol:  ms.StockSymbol,
			Timestamp:    ms.Timestamp,
			Strategy:     ms.Strategy,
			Decision:     ms.Decision,
			PriceZScore:  ms.PriceZScore,
			VolumeZScore: ms.VolumeZScore,
			Price:        ms.Price,
			Volume:       ms.Volume,
			Change:       ms.Change,
			Confidence:   ms.Confidence,
			Reason:       ms.Reason,
		}
	}

	return signals, nil
}

// Analytics methods
func (r *TradeRepository) SaveStatisticalBaseline(baseline *models.StatisticalBaseline) error {
	return r.analytics.SaveStatisticalBaseline(baseline)
}

func (r *TradeRepository) GetLatestBaseline(symbol string) (*models.StatisticalBaseline, error) {
	return r.analytics.GetLatestBaseline(symbol)
}

func (r *TradeRepository) SaveMarketRegime(regime *models.MarketRegime) error {
	return r.analytics.SaveMarketRegime(regime)
}

func (r *TradeRepository) GetLatestRegime(symbol string) (*models.MarketRegime, error) {
	return r.analytics.GetLatestRegime(symbol)
}

func (r *TradeRepository) GetAggregateMarketRegime() (*models.MarketRegime, error) {
	return r.analytics.GetAggregateMarketRegime()
}

func (r *TradeRepository) SaveDetectedPattern(pattern *models.DetectedPattern) error {
	return r.analytics.SaveDetectedPattern(pattern)
}

func (r *TradeRepository) GetRecentPatterns(symbol string, since time.Time) ([]models.DetectedPattern, error) {
	return r.analytics.GetRecentPatterns(symbol, since)
}

func (r *TradeRepository) UpdatePatternOutcome(id int64, outcome string, breakout bool, maxMove float64) error {
	return r.analytics.UpdatePatternOutcome(id, outcome, breakout, maxMove)
}

func (r *TradeRepository) SaveStockCorrelation(correlation *models.StockCorrelation) error {
	return r.analytics.SaveStockCorrelation(correlation)
}

func (r *TradeRepository) GetStockCorrelations(symbol string, limit int) ([]models.StockCorrelation, error) {
	return r.analytics.GetStockCorrelations(symbol, limit)
}

func (r *TradeRepository) GetCorrelationsForPair(stockA, stockB string) ([]models.StockCorrelation, error) {
	return r.analytics.GetCorrelationsForPair(stockA, stockB)
}

func (r *TradeRepository) SaveOrderFlowImbalance(flow *models.OrderFlowImbalance) error {
	return r.analytics.SaveOrderFlowImbalance(flow)
}

func (r *TradeRepository) GetOrderFlowImbalance(symbol string, startTime, endTime time.Time, limit int) ([]models.OrderFlowImbalance, error) {
	return r.analytics.GetOrderFlowImbalance(symbol, startTime, endTime, limit)
}

func (r *TradeRepository) GetLatestOrderFlow(symbol string) (*models.OrderFlowImbalance, error) {
	return r.analytics.GetLatestOrderFlow(symbol)
}

// Webhook management methods (kept for backward compatibility)
func (r *TradeRepository) GetWebhooks() ([]models.WhaleWebhook, error) {
	var webhooks []models.WhaleWebhook
	err := r.db.db.Order("id ASC").Find(&webhooks).Error
	return webhooks, err
}

func (r *TradeRepository) GetWebhookByID(id int) (*models.WhaleWebhook, error) {
	var webhook models.WhaleWebhook
	err := r.db.db.First(&webhook, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &webhook, err
}

func (r *TradeRepository) SaveWebhook(webhook *models.WhaleWebhook) error {
	return r.db.db.Save(webhook).Error
}

func (r *TradeRepository) DeleteWebhook(id int) error {
	return r.db.db.Delete(&models.WhaleWebhook{}, id).Error
}

// GetRecentSignalsWithOutcomes retrieves recent persisted signals with their outcomes
func (r *TradeRepository) GetRecentSignalsWithOutcomes(lookbackMinutes int, minConfidence float64, strategyFilter string) ([]TradingSignal, error) {
	return r.signals.GetRecentSignalsWithOutcomes(lookbackMinutes, minConfidence, strategyFilter)
}

// GetMLTrainingData retrieves joined data for machine learning training
func (r *TradeRepository) GetMLTrainingData() ([]models.MLTrainingData, error) {
	var results []models.MLTrainingData

	// Query to join signals with outcomes and flatten result
	err := r.db.db.Table("trading_signals s").
		Select("s.generated_at, s.stock_symbol, s.strategy, s.confidence, s.analysis_data, o.result as outcome_result, o.profit_loss_pct, o.exit_reason").
		Joins("JOIN signal_outcomes o ON s.id = o.signal_id").
		Where("s.analysis_data IS NOT NULL AND s.analysis_data != ''").
		Order("s.generated_at DESC").
		Scan(&results).Error

	return results, err
}

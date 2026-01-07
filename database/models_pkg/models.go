package models

import "time"

// Trade represents a running trade record from the Stockbit platform.
// Each trade captures a single transaction with price, volume, and market information.
// Trades are stored in a hypertable for efficient time-series queries.
//
// Key Fields:
//   - Timestamp: When the trade occurred (indexed for time-based queries)
//   - StockSymbol: The stock ticker symbol (indexed for symbol-based queries)
//   - Action: BUY or SELL direction
//   - Volume: Number of shares traded
//   - VolumeLot: Number of lots (1 lot = 100 shares in Indonesian market)
//   - TotalAmount: Total transaction value (price × volume)
//   - MarketBoard: Market segment (RG=Regular, TN=Tuna, NG=Negotiation)
//   - TradeNumber: Unique daily identifier from Stockbit (resets daily)
//
// TimescaleDB Optimization:
//   - Stored in a hypertable partitioned by timestamp
//   - Indexed on timestamp and stock_symbol for fast queries
//   - Automatic chunking based on time intervals
type Trade struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Timestamp   time.Time `gorm:"index;not null" json:"timestamp"`
	StockSymbol string    `gorm:"size:10;index;not null" json:"stock_symbol"`
	Action      string    `gorm:"size:10;not null" json:"action"` // BUY, SELL
	Price       float64   `gorm:"type:decimal(15,2);not null" json:"price"`
	Volume      float64   `gorm:"type:decimal(15,2);not null" json:"volume"`       // in shares
	VolumeLot   float64   `gorm:"type:decimal(15,2);not null" json:"volume_lot"`   // in lots
	TotalAmount float64   `gorm:"type:decimal(20,2);not null" json:"total_amount"` // price * volume
	MarketBoard string    `gorm:"size:5;index" json:"market_board"`                // RG, TN, NG
	Change      *float64  `gorm:"type:decimal(10,4)" json:"change,omitempty"`
	TradeNumber *int64    `gorm:"index" json:"trade_number,omitempty"` // Unique trade identifier from Stockbit (resets daily)
}

// TableName specifies the table name for Trade
func (Trade) TableName() string {
	return "running_trades"
}

// Candle represents 1-minute OHLCV (Open, High, Low, Close, Volume) candle data.
// Candles are pre-computed aggregates of trade data stored in a continuous aggregate view.
//
// Key Fields:
//   - StockSymbol: The stock ticker symbol (part of composite primary key)
//   - Bucket: The 1-minute time bucket (part of composite primary key)
//   - Open/High/Low/Close: OHLC price data for the minute
//   - VolumeShares: Total volume in shares
//   - VolumeLots: Total volume in lots (1 lot = 100 shares)
//   - TotalValue: Total transaction value for the minute
//   - TradeCount: Number of individual trades in this candle
//
// TimescaleDB Optimization:
//   - Stored as a continuous aggregate view on running_trades
//   - Automatically refreshed every minute
//   - Composite primary key (StockSymbol, Bucket) for hypertable compatibility
//   - Enables fast queries for technical analysis and charting
type Candle struct {
	StockSymbol  string    `gorm:"size:10;not null;primaryKey" json:"stock_symbol"`
	Bucket       time.Time `gorm:"not null;primaryKey" json:"time"`
	Open         float64   `gorm:"type:decimal(15,2);not null" json:"open"`
	High         float64   `gorm:"type:decimal(15,2);not null" json:"high"`
	Low          float64   `gorm:"type:decimal(15,2);not null" json:"low"`
	Close        float64   `gorm:"type:decimal(15,2);not null" json:"close"`
	VolumeShares float64   `gorm:"type:decimal(20,2)" json:"volume_shares"`
	VolumeLots   float64   `gorm:"type:decimal(15,2)" json:"volume_lots"`
	TotalValue   float64   `gorm:"type:decimal(20,2)" json:"total_value"`
	TradeCount   int64     `json:"trade_count"`
	MarketBoard  string    `gorm:"size:5" json:"market_board"`
}

// TableName specifies the table name for Candle
func (Candle) TableName() string {
	return "candle_1min"
}

// WhaleAlert represents a detected whale trade with statistical significance.
// Whale alerts are generated when unusually large trades or patterns are detected.
//
// Key Fields:
//   - DetectedAt: When the whale activity was detected (indexed)
//   - StockSymbol: The stock ticker symbol (indexed)
//   - AlertType: Type of detection (SINGLE_TRADE, ACCUMULATION, DISTRIBUTION)
//   - Action: BUY or SELL direction
//   - TriggerPrice/VolumeLots/Value: The trade that triggered the alert
//   - ZScore: Statistical significance (how many standard deviations from mean)
//   - VolumeVsAvgPct: Volume as percentage of average
//   - ConfidenceScore: Algorithm confidence (0.0 to 1.0)
//
// Detection Logic:
//   - Single trades with volume > 3 standard deviations from mean
//   - Accumulation patterns (multiple large BUY trades in short time)
//   - Distribution patterns (multiple large SELL trades in short time)
//   - Confidence score based on volume, price impact, and pattern strength
type WhaleAlert struct {
	ID                 int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	DetectedAt         time.Time `gorm:"primaryKey;index;not null" json:"detected_at"`
	StockSymbol        string    `gorm:"type:text;index;not null" json:"stock_symbol"`
	AlertType          string    `gorm:"type:text;not null" json:"alert_type"` // SINGLE_TRADE, ACCUMULATION, etc.
	Action             string    `gorm:"type:text;not null" json:"action"`     // BUY, SELL
	TriggerPrice       float64   `gorm:"type:decimal(15,2)" json:"trigger_price"`
	TriggerVolumeLots  float64   `gorm:"type:decimal(15,2)" json:"trigger_volume_lots"`
	TriggerValue       float64   `gorm:"type:decimal(20,2)" json:"trigger_value"`
	PatternDurationSec *int      `json:"pattern_duration_sec,omitempty"`
	PatternTradeCount  *int      `json:"pattern_trade_count,omitempty"`
	TotalPatternVolume *float64  `gorm:"type:decimal(15,2)" json:"total_pattern_volume,omitempty"`
	TotalPatternValue  *float64  `gorm:"type:decimal(20,2)" json:"total_pattern_value,omitempty"`
	ZScore             *float64  `gorm:"type:decimal(10,4)" json:"z_score,omitempty"`
	VolumeVsAvgPct     *float64  `gorm:"type:decimal(10,2)" json:"volume_vs_avg_pct,omitempty"`
	AvgPrice           *float64  `gorm:"type:decimal(15,2)" json:"avg_price,omitempty"` // New field for average price context
	ConfidenceScore    float64   `gorm:"type:decimal(5,2);not null" json:"confidence_score"`
	MarketBoard        string    `gorm:"type:text" json:"market_board,omitempty"`
	AdaptiveThreshold  *float64  `gorm:"type:decimal(5,2)" json:"adaptive_threshold,omitempty"`
	VolatilityPct      *float64  `gorm:"type:decimal(5,2)" json:"volatility_pct,omitempty"`
}

// TableName specifies the table name for WhaleAlert
func (WhaleAlert) TableName() string {
	return "whale_alerts"
}

// WhaleWebhook holds webhook registration
type WhaleWebhook struct {
	ID                 int        `gorm:"primaryKey;autoIncrement" json:"id"`
	Name               string     `gorm:"size:100;not null" json:"name"`
	URL                string     `gorm:"not null" json:"url"`
	Method             string     `gorm:"size:10;default:POST" json:"method"`
	AuthType           string     `gorm:"size:20" json:"auth_type"`
	AuthHeader         string     `gorm:"size:100" json:"auth_header"`
	AuthValue          string     `json:"auth_value"`
	AlertTypes         string     `json:"alert_types"`   // Stored as JSON array
	StockSymbols       string     `json:"stock_symbols"` // Stored as JSON array
	MinConfidence      *float64   `gorm:"type:decimal(5,2)" json:"min_confidence,omitempty"`
	MinValue           *float64   `gorm:"type:decimal(20,2)" json:"min_value,omitempty"`
	IsActive           bool       `gorm:"default:true" json:"is_active"`
	RetryCount         int        `gorm:"default:3" json:"retry_count"`
	RetryDelaySeconds  int        `gorm:"default:5" json:"retry_delay_seconds"`
	TimeoutSeconds     int        `gorm:"default:10" json:"timeout_seconds"`
	MaxAlertsPerMinute int        `gorm:"default:10" json:"max_alerts_per_minute"`
	CustomHeaders      string     `json:"custom_headers"` // Stored as JSON
	LastTriggeredAt    *time.Time `json:"last_triggered_at,omitempty"`
	LastSuccessAt      *time.Time `json:"last_success_at,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
	TotalSent          int        `gorm:"default:0" json:"total_sent"`
	TotalFailed        int        `gorm:"default:0" json:"total_failed"`
	CreatedAt          time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the table name for WhaleWebhook
func (WhaleWebhook) TableName() string {
	return "whale_webhooks"
}

// WhaleWebhookLog holds webhook delivery logs
type WhaleWebhookLog struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	WebhookID      int       `gorm:"index;not null" json:"webhook_id"`
	WhaleAlertID   *int64    `json:"whale_alert_id,omitempty"`
	TriggeredAt    time.Time `gorm:"primaryKey;index;not null" json:"triggered_at"`
	Status         string    `gorm:"type:text" json:"status"` // SUCCESS, FAILED, TIMEOUT, RATE_LIMITED
	HTTPStatusCode *int      `json:"http_status_code,omitempty"`
	ResponseBody   string    `json:"response_body,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	RetryAttempt   int       `gorm:"default:0" json:"retry_attempt"`
}

// TradingSignal represents a generated trading strategy signal
type TradingSignal struct {
	StockSymbol   string    `json:"stock_symbol"`
	Timestamp     time.Time `json:"timestamp"`
	Strategy      string    `json:"strategy"` // "VOLUME_BREAKOUT", "MEAN_REVERSION", "FAKEOUT_FILTER"
	Decision      string    `json:"decision"` // "BUY", "SELL", "WAIT", "NO_TRADE"
	PriceZScore   float64   `json:"price_z_score"`
	VolumeZScore  float64   `json:"volume_z_score"`
	Price         float64   `json:"price"`
	Volume        float64   `json:"volume"`
	Change        float64   `json:"change"`
	Confidence    float64   `json:"confidence"`
	Reason        string    `json:"reason"`
	Outcome       string    `json:"outcome,omitempty"`        // WIN, LOSS, BREAKEVEN
	OutcomeStatus string    `json:"outcome_status,omitempty"` // OPEN, SKIPPED, or Outcome
	ProfitLossPct float64   `json:"profit_loss_pct,omitempty"`
}

// WhaleStats represents aggregated statistics for whale activity
type WhaleStats struct {
	StockSymbol       string  `json:"stock_symbol"`
	TotalWhaleTrades  int64   `json:"total_whale_trades"`
	TotalWhaleValue   float64 `json:"total_whale_value"`
	BuyVolumeLots     float64 `json:"buy_volume_lots"`
	SellVolumeLots    float64 `json:"sell_volume_lots"`
	LargestTradeValue float64 `json:"largest_trade_value"`
}

// TradingSignalDB represents a persisted trading signal in the database.
// Trading signals are generated by strategy algorithms and stored for analysis and tracking.
//
// Key Fields:
//   - GeneratedAt: When the signal was generated (indexed)
//   - StockSymbol: The stock ticker symbol (indexed)
//   - Strategy: Strategy type (VOLUME_BREAKOUT, MEAN_REVERSION, FAKEOUT_FILTER)
//   - Decision: Trading decision (BUY, SELL, WAIT, NO_TRADE)
//   - Confidence: Signal confidence score (0.0 to 1.0)
//   - PriceZScore/VolumeZScore: Statistical significance metrics
//   - WhaleAlertID: Optional reference to related whale alert
//
// Signal Generation:
//   - VOLUME_BREAKOUT: High volume with price movement
//   - MEAN_REVERSION: Price deviation from mean
//   - FAKEOUT_FILTER: Filter false breakouts using volume analysis
type TradingSignalDB struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	GeneratedAt          time.Time `gorm:"primaryKey;index:idx_signal_time;not null" json:"generated_at"`
	StockSymbol          string    `gorm:"type:text;index;index:idx_symbol_strategy,priority:1;not null" json:"stock_symbol"`
	Strategy             string    `gorm:"type:text;index:idx_symbol_strategy,priority:2;index:idx_strategy_time,priority:1;not null" json:"strategy"` // VOLUME_BREAKOUT, MEAN_REVERSION, FAKEOUT_FILTER
	Decision             string    `gorm:"type:text;not null" json:"decision"`                                                                         // BUY, SELL, WAIT, NO_TRADE
	Confidence           float64   `gorm:"type:decimal(5,2);not null" json:"confidence"`
	TriggerPrice         float64   `gorm:"type:decimal(15,2)" json:"trigger_price"`
	TriggerVolumeLots    float64   `gorm:"type:decimal(15,2)" json:"trigger_volume_lots"`
	PriceZScore          float64   `gorm:"type:decimal(10,4)" json:"price_z_score"`
	VolumeZScore         float64   `gorm:"type:decimal(10,4)" json:"volume_z_score"`
	PriceChangePct       float64   `gorm:"type:decimal(10,4)" json:"price_change_pct"`
	Reason               string    `gorm:"type:text" json:"reason"`
	MarketRegime         *string   `gorm:"type:text" json:"market_regime,omitempty"` // Future: TRENDING_UP, RANGING, etc.
	VolumeImbalanceRatio *float64  `gorm:"type:decimal(10,4)" json:"volume_imbalance_ratio,omitempty"`
	WhaleAlertID         *int64    `gorm:"index" json:"whale_alert_id,omitempty"`     // Reference to whale_alerts
	AnalysisData         string    `gorm:"type:jsonb" json:"analysis_data,omitempty"` // Features for ML (Scorecard, MTF)
}

// MLTrainingData represents a flattened record for ML training
// Joins Signal Features (Input) with Outcome (Target)
type MLTrainingData struct {
	GeneratedAt   time.Time `json:"generated_at"`
	StockSymbol   string    `json:"stock_symbol"`
	Strategy      string    `json:"strategy"`
	Confidence    float64   `json:"confidence"`
	AnalysisData  string    `json:"analysis_data"` // JSON feature vector
	OutcomeResult string    `json:"outcome_result"`
	ProfitLossPct float64   `json:"profit_loss_pct"`
	ExitReason    string    `json:"exit_reason"`
}

// TableName specifies the table name for TradingSignalDB
func (TradingSignalDB) TableName() string {
	return "trading_signals"
}

// SignalOutcome tracks the performance of a trading signal
type SignalOutcome struct {
	ID                    int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	SignalID              int64      `gorm:"index;not null" json:"signal_id"`
	StockSymbol           string     `gorm:"type:text;index;index:idx_outcome_symbol_status,priority:1;not null" json:"stock_symbol"`
	EntryTime             time.Time  `gorm:"primaryKey;index;not null" json:"entry_time"`
	EntryPrice            float64    `gorm:"type:decimal(15,2);not null" json:"entry_price"`
	EntryDecision         string     `gorm:"type:text;not null" json:"entry_decision"` // BUY or SELL
	ATRAtEntry            *float64   `gorm:"type:decimal(15,4)" json:"atr_at_entry,omitempty"`
	TrailingStopPrice     *float64   `gorm:"type:decimal(15,2)" json:"trailing_stop_price,omitempty"`
	ExitTime              *time.Time `gorm:"index" json:"exit_time,omitempty"`
	ExitPrice             *float64   `gorm:"type:decimal(15,2)" json:"exit_price,omitempty"`
	ExitReason            *string    `gorm:"type:text" json:"exit_reason,omitempty"` // TAKE_PROFIT, STOP_LOSS, TIME_BASED, REVERSE_SIGNAL
	HoldingPeriodMinutes  *int       `json:"holding_period_minutes,omitempty"`
	PriceChangePct        *float64   `gorm:"type:decimal(10,4)" json:"price_change_pct,omitempty"`                           // (exit - entry) / entry * 100
	ProfitLossPct         *float64   `gorm:"type:decimal(10,4)" json:"profit_loss_pct,omitempty"`                            // Adjusted for direction
	MaxFavorableExcursion *float64   `gorm:"type:decimal(10,4)" json:"max_favorable_excursion,omitempty"`                    // MFE: Best price reached
	MaxAdverseExcursion   *float64   `gorm:"type:decimal(10,4)" json:"max_adverse_excursion,omitempty"`                      // MAE: Worst price reached
	RiskRewardRatio       *float64   `gorm:"type:decimal(10,4)" json:"risk_reward_ratio,omitempty"`                          // MFE / MAE
	OutcomeStatus         string     `gorm:"size:20;index;index:idx_outcome_symbol_status,priority:2" json:"outcome_status"` // WIN, LOSS, BREAKEVEN, OPEN
}

// TableName specifies the table name for SignalOutcome
func (SignalOutcome) TableName() string {
	return "signal_outcomes"
}

// WhaleAlertFollowup tracks price movement after whale alert detection
type WhaleAlertFollowup struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	WhaleAlertID        int64     `gorm:"index;not null" json:"whale_alert_id"`
	StockSymbol         string    `gorm:"type:text;index;not null" json:"stock_symbol"`
	AlertTime           time.Time `gorm:"primaryKey;index;not null" json:"alert_time"`
	AlertPrice          float64   `gorm:"type:decimal(15,2);not null" json:"alert_price"`
	AlertAction         string    `gorm:"type:text;not null" json:"alert_action"` // BUY or SELL
	Price1MinLater      *float64  `gorm:"column:price_1min_later;type:decimal(15,2)" json:"price_1min_later,omitempty"`
	Price5MinLater      *float64  `gorm:"column:price_5min_later;type:decimal(15,2)" json:"price_5min_later,omitempty"`
	Price15MinLater     *float64  `gorm:"column:price_15min_later;type:decimal(15,2)" json:"price_15min_later,omitempty"`
	Price30MinLater     *float64  `gorm:"column:price_30min_later;type:decimal(15,2)" json:"price_30min_later,omitempty"`
	Price60MinLater     *float64  `gorm:"column:price_60min_later;type:decimal(15,2)" json:"price_60min_later,omitempty"`
	Price1DayLater      *float64  `gorm:"column:price_1day_later;type:decimal(15,2)" json:"price_1day_later,omitempty"`
	Change1MinPct       *float64  `gorm:"column:change_1min_pct;type:decimal(10,4)" json:"change_1min_pct,omitempty"`
	Change5MinPct       *float64  `gorm:"column:change_5min_pct;type:decimal(10,4)" json:"change_5min_pct,omitempty"`
	Change15MinPct      *float64  `gorm:"column:change_15min_pct;type:decimal(10,4)" json:"change_15min_pct,omitempty"`
	Change30MinPct      *float64  `gorm:"column:change_30min_pct;type:decimal(10,4)" json:"change_30min_pct,omitempty"`
	Change60MinPct      *float64  `gorm:"column:change_60min_pct;type:decimal(10,4)" json:"change_60min_pct,omitempty"`
	Change1DayPct       *float64  `gorm:"column:change_1day_pct;type:decimal(10,4)" json:"change_1day_pct,omitempty"`
	Volume1MinLater     *float64  `gorm:"column:volume_1min_later;type:decimal(15,2)" json:"volume_1min_later,omitempty"`
	Volume5MinLater     *float64  `gorm:"column:volume_5min_later;type:decimal(15,2)" json:"volume_5min_later,omitempty"`
	Volume15MinLater    *float64  `gorm:"column:volume_15min_later;type:decimal(15,2)" json:"volume_15min_later,omitempty"`
	ImmediateImpact     *string   `gorm:"type:text" json:"immediate_impact,omitempty"` // POSITIVE, NEGATIVE, NEUTRAL (5min)
	SustainedImpact     *string   `gorm:"type:text" json:"sustained_impact,omitempty"` // POSITIVE, NEGATIVE, NEUTRAL (1hr)
	ReversalDetected    *bool     `json:"reversal_detected,omitempty"`
	ReversalTimeMinutes *int      `json:"reversal_time_minutes,omitempty"`
	Analysis            *string   `gorm:"type:text" json:"analysis,omitempty"`
}

// TableName specifies the table name for WhaleAlertFollowup
func (WhaleAlertFollowup) TableName() string {
	return "whale_alert_followup"
}

// OrderFlowImbalance tracks buy vs sell pressure per minute
type OrderFlowImbalance struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Bucket               time.Time `gorm:"primaryKey;not null;uniqueIndex:idx_flow_bucket_symbol" json:"bucket"`
	StockSymbol          string    `gorm:"type:text;not null;uniqueIndex:idx_flow_bucket_symbol" json:"stock_symbol"`
	BuyVolumeLots        float64   `gorm:"type:decimal(15,2);not null" json:"buy_volume_lots"`
	SellVolumeLots       float64   `gorm:"type:decimal(15,2);not null" json:"sell_volume_lots"`
	BuyTradeCount        int       `gorm:"not null" json:"buy_trade_count"`
	SellTradeCount       int       `gorm:"not null" json:"sell_trade_count"`
	BuyValue             float64   `gorm:"type:decimal(20,2)" json:"buy_value"`
	SellValue            float64   `gorm:"type:decimal(20,2)" json:"sell_value"`
	VolumeImbalanceRatio float64   `gorm:"type:decimal(10,4)" json:"volume_imbalance_ratio"`
	ValueImbalanceRatio  float64   `gorm:"type:decimal(10,4)" json:"value_imbalance_ratio"`
	DeltaVolume          float64   `gorm:"type:decimal(15,2)" json:"delta_volume"`
	AggressiveBuyPct     *float64  `gorm:"type:decimal(5,2)" json:"aggressive_buy_pct,omitempty"`
	AggressiveSellPct    *float64  `gorm:"type:decimal(5,2)" json:"aggressive_sell_pct,omitempty"`
}

// TableName specifies the table name for OrderFlowImbalance
func (OrderFlowImbalance) TableName() string {
	return "order_flow_imbalance"
}

// StatisticalBaseline stores persistent rolling statistics
type StatisticalBaseline struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StockSymbol   string    `gorm:"type:text;not null;index:idx_baselines_symbol_time" json:"stock_symbol"`
	CalculatedAt  time.Time `gorm:"primaryKey;not null;index:idx_baselines_symbol_time" json:"calculated_at"`
	LookbackHours int       `gorm:"not null" json:"lookback_hours"`
	SampleSize    int       `json:"sample_size"`

	// Price Statistics
	MeanPrice   float64 `gorm:"type:decimal(15,2)" json:"mean_price"`
	StdDevPrice float64 `gorm:"type:decimal(15,4)" json:"std_dev_price"`
	MedianPrice float64 `gorm:"type:decimal(15,2)" json:"median_price"`
	PriceP25    float64 `gorm:"type:decimal(15,2)" json:"price_p25"`
	PriceP75    float64 `gorm:"type:decimal(15,2)" json:"price_p75"`

	// Volume Statistics
	MeanVolumeLots   float64 `gorm:"type:decimal(15,2)" json:"mean_volume_lots"`
	StdDevVolume     float64 `gorm:"type:decimal(15,4)" json:"std_dev_volume"`
	MedianVolumeLots float64 `gorm:"type:decimal(15,2)" json:"median_volume_lots"`
	VolumeP25        float64 `gorm:"type:decimal(15,2)" json:"volume_p25"`
	VolumeP75        float64 `gorm:"type:decimal(15,2)" json:"volume_p75"`

	// Value Statistics
	MeanValue   float64 `gorm:"type:decimal(20,2)" json:"mean_value"`
	StdDevValue float64 `gorm:"type:decimal(20,4)" json:"std_dev_value"`
}

func (StatisticalBaseline) TableName() string {
	return "statistical_baselines"
}

// MarketRegime classifies market conditions
type MarketRegime struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StockSymbol     string    `gorm:"type:text;not null;index:idx_regimes_symbol_time" json:"stock_symbol"`
	DetectedAt      time.Time `gorm:"primaryKey;not null;index:idx_regimes_symbol_time" json:"detected_at"`
	LookbackPeriods int       `gorm:"not null" json:"lookback_periods"`

	// Regime Classification: TRENDING_UP, TRENDING_DOWN, RANGING, VOLATILE
	Regime     string  `gorm:"type:text;not null;index:idx_regimes_regime" json:"regime"`
	Confidence float64 `gorm:"type:decimal(5,4);index:idx_regimes_regime" json:"confidence"`

	// Technical Indicators
	ADX            *float64 `gorm:"type:decimal(10,4)" json:"adx,omitempty"`
	ATR            *float64 `gorm:"type:decimal(15,4)" json:"atr,omitempty"`
	BollingerWidth *float64 `gorm:"type:decimal(10,4)" json:"bollinger_width,omitempty"`

	// Price Movement
	PriceChangePct *float64 `gorm:"type:decimal(10,4)" json:"price_change_pct,omitempty"`
	Volatility     *float64 `gorm:"type:decimal(10,4)" json:"volatility,omitempty"`
}

func (MarketRegime) TableName() string {
	return "market_regimes"
}

// DetectedPattern stores chart patterns
type DetectedPattern struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StockSymbol      string    `gorm:"type:text;not null;index:idx_patterns_symbol_time" json:"stock_symbol"`
	DetectedAt       time.Time `gorm:"primaryKey;not null;index:idx_patterns_symbol_time" json:"detected_at"`
	PatternType      string    `gorm:"type:text;not null;index:idx_patterns_symbol_time" json:"pattern_type"`
	PatternDirection *string   `gorm:"type:text" json:"pattern_direction,omitempty"`
	Confidence       float64   `gorm:"type:decimal(5,4)" json:"confidence"`

	// Pattern Metrics
	PatternStart  *time.Time `json:"pattern_start,omitempty"`
	PatternEnd    *time.Time `json:"pattern_end,omitempty"`
	PriceRange    *float64   `gorm:"type:decimal(15,2)" json:"price_range,omitempty"`
	VolumeProfile *string    `gorm:"type:text" json:"volume_profile,omitempty"`

	// Target Levels
	BreakoutLevel *float64 `gorm:"type:decimal(15,2)" json:"breakout_level,omitempty"`
	TargetPrice   *float64 `gorm:"type:decimal(15,2)" json:"target_price,omitempty"`
	StopLoss      *float64 `gorm:"type:decimal(15,2)" json:"stop_loss,omitempty"`

	// Outcome
	Outcome        *string  `gorm:"type:text;index:idx_patterns_outcome" json:"outcome,omitempty"`
	ActualBreakout *bool    `json:"actual_breakout,omitempty"`
	MaxMovePct     *float64 `gorm:"type:decimal(10,4)" json:"max_move_pct,omitempty"`

	// LLM Analysis
	LLMAnalysis *string `gorm:"type:text" json:"llm_analysis,omitempty"`
}

func (DetectedPattern) TableName() string {
	return "detected_patterns"
}

// StockCorrelation stores correlation coefficients between stock pairs
type StockCorrelation struct {
	ID                     int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StockA                 string    `gorm:"type:text;not null;index:idx_correlations_pair" json:"stock_a"`
	StockB                 string    `gorm:"type:text;not null;index:idx_correlations_pair" json:"stock_b"`
	CalculatedAt           time.Time `gorm:"primaryKey;not null;index:idx_correlations_pair" json:"calculated_at"`
	CorrelationCoefficient float64   `json:"correlation_coefficient"`
	LookbackDays           int       `json:"lookback_days"`
	Period                 string    `gorm:"type:text" json:"period"`
}

func (StockCorrelation) TableName() string {
	return "stock_correlations"
}

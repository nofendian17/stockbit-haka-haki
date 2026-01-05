package database

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Database holds the GORM database connection
type Database struct {
	db *gorm.DB
}

// GORM Models with proper tags

// Trade represents a running trade record
type Trade struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	Timestamp   time.Time `gorm:"index;not null"`
	StockSymbol string    `gorm:"size:10;index;not null"`
	Action      string    `gorm:"size:10;not null"` // BUY, SELL
	Price       float64   `gorm:"type:decimal(15,2);not null"`
	Volume      float64   `gorm:"type:decimal(15,2);not null"` // in shares
	VolumeLot   float64   `gorm:"type:decimal(15,2);not null"` // in lots
	TotalAmount float64   `gorm:"type:decimal(20,2);not null"` // price * volume
	MarketBoard string    `gorm:"size:5;index"`                // RG, TN, NG
	Change      *float64  `gorm:"type:decimal(10,4)"`
	TradeNumber *int64    `gorm:"index"` // Unique trade identifier from Stockbit (resets daily)
}

// TableName specifies the table name for Trade
func (Trade) TableName() string {
	return "running_trades"
}

// Candle represents 1-minute OHLCV candle data
type Candle struct {
	StockSymbol  string    `gorm:"size:10;not null;primaryKey"`
	Bucket       time.Time `gorm:"not null;primaryKey"`
	Open         float64   `gorm:"type:decimal(15,2);not null"`
	High         float64   `gorm:"type:decimal(15,2);not null"`
	Low          float64   `gorm:"type:decimal(15,2);not null"`
	Close        float64   `gorm:"type:decimal(15,2);not null"`
	VolumeShares float64   `gorm:"type:decimal(20,2)"`
	VolumeLots   float64   `gorm:"type:decimal(15,2)"`
	TotalValue   float64   `gorm:"type:decimal(20,2)"`
	TradeCount   int64
	MarketBoard  string `gorm:"size:5"`
}

// TableName specifies the table name for Candle
func (Candle) TableName() string {
	return "candle_1min"
}

// WhaleAlert represents a detected whale trade
type WhaleAlert struct {
	ID                 int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	DetectedAt         time.Time `gorm:"index;not null" json:"detected_at"`
	StockSymbol        string    `gorm:"size:10;index;not null" json:"stock_symbol"`
	AlertType          string    `gorm:"size:20;not null" json:"alert_type"` // SINGLE_TRADE, ACCUMULATION, etc.
	Action             string    `gorm:"size:10;not null" json:"action"`     // BUY, SELL
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
	MarketBoard        string    `gorm:"size:5" json:"market_board,omitempty"`
}

// TableName specifies the table name for WhaleAlert
func (WhaleAlert) TableName() string {
	return "whale_alerts"
}

// Webhook Management
// WhaleWebhook holds webhook registration

// WhaleWebhook holds webhook registration
type WhaleWebhook struct {
	ID                 int    `gorm:"primaryKey;autoIncrement"`
	Name               string `gorm:"size:100;not null"`
	URL                string `gorm:"not null"`
	Method             string `gorm:"size:10;default:POST"`
	AuthType           string `gorm:"size:20"`
	AuthHeader         string `gorm:"size:100"`
	AuthValue          string
	AlertTypes         string   // Stored as JSON array
	StockSymbols       string   // Stored as JSON array
	MinConfidence      *float64 `gorm:"type:decimal(5,2)"`
	MinValue           *float64 `gorm:"type:decimal(20,2)"`
	IsActive           bool     `gorm:"default:true"`
	RetryCount         int      `gorm:"default:3"`
	RetryDelaySeconds  int      `gorm:"default:5"`
	TimeoutSeconds     int      `gorm:"default:10"`
	MaxAlertsPerMinute int      `gorm:"default:10"`
	CustomHeaders      string   // Stored as JSON
	LastTriggeredAt    *time.Time
	LastSuccessAt      *time.Time
	LastError          string
	TotalSent          int       `gorm:"default:0"`
	TotalFailed        int       `gorm:"default:0"`
	CreatedAt          time.Time `gorm:"autoCreateTime"`
	UpdatedAt          time.Time `gorm:"autoUpdateTime"`
}

// TableName specifies the table name for WhaleWebhook
func (WhaleWebhook) TableName() string {
	return "whale_webhooks"
}

// WhaleWebhookLog holds webhook delivery logs
type WhaleWebhookLog struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	WebhookID      int   `gorm:"index;not null"`
	WhaleAlertID   *int64
	TriggeredAt    time.Time `gorm:"index;not null"`
	Status         string    `gorm:"size:20"` // SUCCESS, FAILED, TIMEOUT, RATE_LIMITED
	HTTPStatusCode *int
	ResponseBody   string
	ErrorMessage   string
	RetryAttempt   int `gorm:"default:0"`
}

// TradingSignal represents a generated trading strategy signal
type TradingSignal struct {
	StockSymbol  string    `json:"stock_symbol"`
	Timestamp    time.Time `json:"timestamp"`
	Strategy     string    `json:"strategy"` // "VOLUME_BREAKOUT", "MEAN_REVERSION", "FAKEOUT_FILTER"
	Decision     string    `json:"decision"` // "BUY", "SELL", "WAIT", "NO_TRADE"
	PriceZScore  float64   `json:"price_z_score"`
	VolumeZScore float64   `json:"volume_z_score"`
	Price        float64   `json:"price"`
	Volume       float64   `json:"volume"`
	Change       float64   `json:"change"`
	Confidence   float64   `json:"confidence"`
	Reason       string    `json:"reason"`
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

// Phase 1 Enhancement Models

// TradingSignalDB represents a persisted trading signal in the database
type TradingSignalDB struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement"`
	GeneratedAt          time.Time `gorm:"index;not null"`
	StockSymbol          string    `gorm:"size:10;index;not null"`
	Strategy             string    `gorm:"size:30;not null"` // VOLUME_BREAKOUT, MEAN_REVERSION, FAKEOUT_FILTER
	Decision             string    `gorm:"size:20;not null"` // BUY, SELL, WAIT, NO_TRADE
	Confidence           float64   `gorm:"type:decimal(5,2);not null"`
	TriggerPrice         float64   `gorm:"type:decimal(15,2)"`
	TriggerVolumeLots    float64   `gorm:"type:decimal(15,2)"`
	PriceZScore          float64   `gorm:"type:decimal(10,4)"`
	VolumeZScore         float64   `gorm:"type:decimal(10,4)"`
	PriceChangePct       float64   `gorm:"type:decimal(10,4)"`
	Reason               string    `gorm:"type:text"`
	MarketRegime         *string   `gorm:"size:20"` // Future: TRENDING_UP, RANGING, etc.
	VolumeImbalanceRatio *float64  `gorm:"type:decimal(10,4)"`
	WhaleAlertID         *int64    `gorm:"index"` // Reference to whale_alerts
}

// TableName specifies the table name for TradingSignalDB
func (TradingSignalDB) TableName() string {
	return "trading_signals"
}

// SignalOutcome tracks the performance of a trading signal
type SignalOutcome struct {
	ID                    int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	SignalID              int64      `gorm:"uniqueIndex;not null" json:"signal_id"`
	StockSymbol           string     `gorm:"type:varchar(10);index;not null" json:"stock_symbol"`
	EntryTime             time.Time  `gorm:"primaryKey;index;not null" json:"entry_time"`
	EntryPrice            float64    `gorm:"type:decimal(15,2);not null"`
	EntryDecision         string     `gorm:"size:10;not null"` // BUY or SELL
	ExitTime              *time.Time `gorm:"index"`
	ExitPrice             *float64   `gorm:"type:decimal(15,2)"`
	ExitReason            *string    `gorm:"size:50"` // TAKE_PROFIT, STOP_LOSS, TIME_BASED, REVERSE_SIGNAL
	HoldingPeriodMinutes  *int
	PriceChangePct        *float64 `gorm:"type:decimal(10,4)"` // (exit - entry) / entry * 100
	ProfitLossPct         *float64 `gorm:"type:decimal(10,4)"` // Adjusted for direction
	MaxFavorableExcursion *float64 `gorm:"type:decimal(10,4)"` // MFE: Best price reached
	MaxAdverseExcursion   *float64 `gorm:"type:decimal(10,4)"` // MAE: Worst price reached
	RiskRewardRatio       *float64 `gorm:"type:decimal(10,4)"` // MFE / MAE
	OutcomeStatus         string   `gorm:"size:20;index"`      // WIN, LOSS, BREAKEVEN, OPEN
}

// TableName specifies the table name for SignalOutcome
func (SignalOutcome) TableName() string {
	return "signal_outcomes"
}

// WhaleAlertFollowup tracks price movement after whale alert detection
type WhaleAlertFollowup struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement"`
	WhaleAlertID        int64     `gorm:"uniqueIndex;not null"`
	StockSymbol         string    `gorm:"size:10;index;not null"`
	AlertTime           time.Time `gorm:"index;not null"`
	AlertPrice          float64   `gorm:"type:decimal(15,2);not null"`
	AlertAction         string    `gorm:"size:10;not null"` // BUY or SELL
	Price1MinLater      *float64  `gorm:"column:price_1min_later;type:decimal(15,2)"`
	Price5MinLater      *float64  `gorm:"column:price_5min_later;type:decimal(15,2)"`
	Price15MinLater     *float64  `gorm:"column:price_15min_later;type:decimal(15,2)"`
	Price30MinLater     *float64  `gorm:"column:price_30min_later;type:decimal(15,2)"`
	Price60MinLater     *float64  `gorm:"column:price_60min_later;type:decimal(15,2)"`
	Price1DayLater      *float64  `gorm:"column:price_1day_later;type:decimal(15,2)"`
	Change1MinPct       *float64  `gorm:"column:change_1min_pct;type:decimal(10,4)"`
	Change5MinPct       *float64  `gorm:"column:change_5min_pct;type:decimal(10,4)"`
	Change15MinPct      *float64  `gorm:"column:change_15min_pct;type:decimal(10,4)"`
	Change30MinPct      *float64  `gorm:"column:change_30min_pct;type:decimal(10,4)"`
	Change60MinPct      *float64  `gorm:"column:change_60min_pct;type:decimal(10,4)"`
	Change1DayPct       *float64  `gorm:"column:change_1day_pct;type:decimal(10,4)"`
	Volume1MinLater     *float64  `gorm:"column:volume_1min_later;type:decimal(15,2)"`
	Volume5MinLater     *float64  `gorm:"column:volume_5min_later;type:decimal(15,2)"`
	Volume15MinLater    *float64  `gorm:"column:volume_15min_later;type:decimal(15,2)"`
	ImmediateImpact     *string   `gorm:"size:20"` // POSITIVE, NEGATIVE, NEUTRAL (5min)
	SustainedImpact     *string   `gorm:"size:20"` // POSITIVE, NEGATIVE, NEUTRAL (1hr)
	ReversalDetected    *bool
	ReversalTimeMinutes *int
}

// TableName specifies the table name for WhaleAlertFollowup
func (WhaleAlertFollowup) TableName() string {
	return "whale_alert_followup"
}

// OrderFlowImbalance tracks buy vs sell pressure per minute
type OrderFlowImbalance struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Bucket               time.Time `gorm:"not null;uniqueIndex:idx_flow_bucket_symbol" json:"bucket"`
	StockSymbol          string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_flow_bucket_symbol" json:"stock_symbol"`
	BuyVolumeLots        float64   `gorm:"type:decimal(15,2);not null" json:"buy_volume"`
	SellVolumeLots       float64   `gorm:"type:decimal(15,2);not null" json:"sell_volume"`
	BuyTradeCount        int       `gorm:"not null" json:"buy_trade_count"`
	SellTradeCount       int       `gorm:"not null" json:"sell_trade_count"`
	BuyValue             float64   `gorm:"type:decimal(20,2)" json:"buy_value"`
	SellValue            float64   `gorm:"type:decimal(20,2)" json:"sell_value"`
	VolumeImbalanceRatio float64   `gorm:"type:decimal(10,4)" json:"volume_imbalance"`
	ValueImbalanceRatio  float64   `gorm:"type:decimal(10,4)" json:"value_imbalance"`
	DeltaVolume          float64   `gorm:"type:decimal(15,2)" json:"delta_volume"`
	AggressiveBuyPct     *float64  `gorm:"type:decimal(5,2)" json:"aggressive_buy_pct,omitempty"`
	AggressiveSellPct    *float64  `gorm:"type:decimal(5,2)" json:"aggressive_sell_pct,omitempty"`
}

// TableName specifies the table name for OrderFlowImbalance
func (OrderFlowImbalance) TableName() string {
	return "order_flow_imbalance"
}

// Phase 2: Statistical Enhancements

// StatisticalBaseline stores persistent rolling statistics
type StatisticalBaseline struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StockSymbol   string    `gorm:"type:varchar(10);not null;index:idx_baselines_symbol_time" json:"stock_symbol"`
	CalculatedAt  time.Time `gorm:"not null;index:idx_baselines_symbol_time" json:"calculated_at"`
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
	StockSymbol     string    `gorm:"type:varchar(10);not null;index:idx_regimes_symbol_time" json:"stock_symbol"`
	DetectedAt      time.Time `gorm:"not null;index:idx_regimes_symbol_time" json:"detected_at"`
	LookbackPeriods int       `gorm:"not null" json:"lookback_periods"`

	// Regime Classification: TRENDING_UP, TRENDING_DOWN, RANGING, VOLATILE
	Regime     string  `gorm:"type:varchar(20);not null;index:idx_regimes_regime" json:"regime"`
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
	StockSymbol      string    `gorm:"type:varchar(10);not null;index:idx_patterns_symbol_time" json:"stock_symbol"`
	DetectedAt       time.Time `gorm:"not null;index:idx_patterns_symbol_time" json:"detected_at"`
	PatternType      string    `gorm:"type:varchar(50);not null;index:idx_patterns_symbol_time" json:"pattern_type"`
	PatternDirection *string   `gorm:"type:varchar(10)" json:"pattern_direction,omitempty"`
	Confidence       float64   `gorm:"type:decimal(5,4)" json:"confidence"`

	// Pattern Metrics
	PatternStart  *time.Time
	PatternEnd    *time.Time
	PriceRange    *float64 `gorm:"type:decimal(15,2)" json:"price_range,omitempty"`
	VolumeProfile *string  `gorm:"type:varchar(20)" json:"volume_profile,omitempty"`

	// Target Levels
	BreakoutLevel *float64 `gorm:"type:decimal(15,2)" json:"breakout_level,omitempty"`
	TargetPrice   *float64 `gorm:"type:decimal(15,2)" json:"target_price,omitempty"`
	StopLoss      *float64 `gorm:"type:decimal(15,2)" json:"stop_loss,omitempty"`

	// Outcome
	Outcome        *string  `gorm:"type:varchar(20);index:idx_patterns_outcome" json:"outcome,omitempty"`
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
	ID                     int64     `gorm:"primaryKey;autoIncrement"`
	StockA                 string    `gorm:"type:varchar(10);not null;index:idx_correlations_pair"`
	StockB                 string    `gorm:"type:varchar(10);not null;index:idx_correlations_pair"`
	CalculatedAt           time.Time `gorm:"not null;index:idx_correlations_pair"`
	CorrelationCoefficient float64
	LookbackDays           int
	Period                 string `gorm:"type:varchar(10)"`
}

func (StockCorrelation) TableName() string {
	return "stock_correlations"
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

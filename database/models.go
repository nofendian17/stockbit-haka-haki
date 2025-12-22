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
	ID                 int64     `gorm:"primaryKey;autoIncrement"`
	DetectedAt         time.Time `gorm:"index;not null"`
	StockSymbol        string    `gorm:"size:10;index;not null"`
	AlertType          string    `gorm:"size:20;not null"` // SINGLE_TRADE, ACCUMULATION, etc.
	Action             string    `gorm:"size:10;not null"` // BUY, SELL
	TriggerPrice       float64   `gorm:"type:decimal(15,2)"`
	TriggerVolumeLots  float64   `gorm:"type:decimal(15,2)"`
	TriggerValue       float64   `gorm:"type:decimal(20,2)"`
	PatternDurationSec *int
	PatternTradeCount  *int
	TotalPatternVolume *float64 `gorm:"type:decimal(15,2)"`
	TotalPatternValue  *float64 `gorm:"type:decimal(20,2)"`
	ZScore             *float64 `gorm:"type:decimal(10,4)"`
	VolumeVsAvgPct     *float64 `gorm:"type:decimal(10,2)"`
	AvgPrice           *float64 `gorm:"type:decimal(15,2)"` // New field for average price context
	ConfidenceScore    float64  `gorm:"type:decimal(5,2);not null"`
	MarketBoard        string   `gorm:"size:5"`
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

// WhaleStats represents aggregated statistics for whale activity
type WhaleStats struct {
	StockSymbol       string  `json:"stock_symbol"`
	TotalWhaleTrades  int64   `json:"total_whale_trades"`
	TotalWhaleValue   float64 `json:"total_whale_value"`
	BuyVolumeLots     float64 `json:"buy_volume_lots"`
	SellVolumeLots    float64 `json:"sell_volume_lots"`
	LargestTradeValue float64 `json:"largest_trade_value"`
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

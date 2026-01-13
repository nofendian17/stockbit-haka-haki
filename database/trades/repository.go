package trades

import (
	"fmt"
	"strings"
	"time"

	models "stockbit-haka-haki/database/models_pkg"
	"stockbit-haka-haki/database/types"

	"gorm.io/gorm"
)

// Repository handles database operations for trade data
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new trades repository
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// SaveTrade saves a trade record
// Handles duplicate trade numbers by catching and ignoring duplicate key errors
func (r *Repository) SaveTrade(trade *models.Trade) error {
	if err := r.db.Create(trade).Error; err != nil {
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") ||
			strings.Contains(err.Error(), "idx_running_trades_unique_trade") {
			// Ignore duplicate trade numbers
			return nil
		}
		return fmt.Errorf("SaveTrade: %w", err)
	}
	return nil
}

// BatchSaveTrades saves multiple trade records in a single transaction
// Handles duplicate trade numbers by catching and ignoring duplicate key errors
func (r *Repository) BatchSaveTrades(trades []*models.Trade) error {
	if len(trades) == 0 {
		return nil
	}

	// Use CreateInBatches and handle duplicate key errors
	// Process trades in smaller batches to avoid memory issues
	batchSize := 100
	for i := 0; i < len(trades); i += batchSize {
		end := i + batchSize
		if end > len(trades) {
			end = len(trades)
		}
		batch := trades[i:end]

		// Convert to interface slice for gorm
		interfaceBatch := make([]interface{}, len(batch))
		for j, trade := range batch {
			interfaceBatch[j] = trade
		}

		if err := r.db.CreateInBatches(interfaceBatch, len(batch)).Error; err != nil {
			// Check if it's a duplicate key error
			if strings.Contains(err.Error(), "duplicate key value violates unique constraint") ||
				strings.Contains(err.Error(), "idx_running_trades_unique_trade") {
				// Ignore duplicate trade numbers - continue with next batch
				continue
			}
			return fmt.Errorf("BatchSaveTrades batch %d: %w", i/batchSize, err)
		}
	}

	return nil
}

// GetRecentTrades retrieves recent trades with filters
func (r *Repository) GetRecentTrades(stockSymbol string, limit int, actionFilter string) ([]models.Trade, error) {
	var trades []models.Trade
	query := r.db.Order("timestamp DESC")

	if stockSymbol != "" {
		query = query.Where("stock_symbol = ?", stockSymbol)
	}

	if actionFilter != "" {
		query = query.Where("action = ?", actionFilter)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&trades).Error; err != nil {
		return nil, fmt.Errorf("GetRecentTrades: %w", err)
	}
	return trades, nil
}

// GetCandles retrieves candle data with filters
func (r *Repository) GetCandles(stockSymbol string, startTime, endTime time.Time, limit int) ([]models.Candle, error) {
	var candles []models.Candle
	query := r.db.Order("bucket DESC")

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

	if err := query.Find(&candles).Error; err != nil {
		return nil, fmt.Errorf("GetCandles: %w", err)
	}
	return candles, nil
}

// GetLatestCandle retrieves the most recent candle for a stock
func (r *Repository) GetLatestCandle(stockSymbol string) (*models.Candle, error) {
	var candle models.Candle
	err := r.db.
		Where("stock_symbol = ?", stockSymbol).
		Order("bucket DESC").
		First(&candle).Error

	if err == nil {
		return &candle, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("GetLatestCandle: %w", err)
	}

	// FALLBACK: If no candle found in materialized view (e.g., view not refreshed yet),
	// try to construct a pseudo-candle from the very latest running trade.
	// This ensures real-time price availability for critical functions like SignalTracker.
	var latestTrade models.Trade
	errTrade := r.db.Table("running_trades").
		Where("stock_symbol = ?", stockSymbol).
		Order("timestamp DESC").
		First(&latestTrade).Error

	if errTrade == gorm.ErrRecordNotFound {
		return nil, nil // No trades either, truly no data
	}

	if errTrade != nil {
		return nil, fmt.Errorf("GetLatestCandle (fallback): %w", errTrade)
	}

	// Construct pseudo-candle from latest trade
	pseudoCandle := &models.Candle{
		StockSymbol: latestTrade.StockSymbol,
		Bucket:      latestTrade.Timestamp, // precise time
		Open:        latestTrade.Price,
		High:        latestTrade.Price,
		Low:         latestTrade.Price,
		Close:       latestTrade.Price,
		VolumeLots:  latestTrade.VolumeLot,
		TotalValue:  latestTrade.TotalAmount,
		TradeCount:  1,
		MarketBoard: latestTrade.MarketBoard,
	}

	return pseudoCandle, nil
}

// GetCandlesByTimeframe returns candles for a specific timeframe and symbol
// Supported timeframes: 1min/1m, 5min/5m, 15min/15m, 1hour/1h, 1day/1d
func (r *Repository) GetCandlesByTimeframe(timeframe string, symbol string, limit int) ([]map[string]interface{}, error) {
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
	err := r.db.Table(viewName).
		Where("stock_symbol = ?", symbol).
		Order("bucket DESC").
		Limit(limit).
		Find(&results).Error

	if err != nil {
		return nil, fmt.Errorf("GetCandlesByTimeframe: %w", err)
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
func (r *Repository) GetActiveSymbols(since time.Time) ([]string, error) {
	var symbols []string
	err := r.db.Table("running_trades").
		Where("timestamp >= ?", since).
		Distinct("stock_symbol").
		Pluck("stock_symbol", &symbols).Error

	if err != nil {
		return nil, fmt.Errorf("GetActiveSymbols: %w", err)
	}
	return symbols, nil
}

// GetTradesByTimeRange retrieves trades for a symbol within a time range
func (r *Repository) GetTradesByTimeRange(symbol string, startTime, endTime time.Time) ([]models.Trade, error) {
	var trades []models.Trade
	err := r.db.Where("stock_symbol = ? AND timestamp >= ? AND timestamp <= ?", symbol, startTime, endTime).
		Order("timestamp ASC").
		Find(&trades).Error

	if err != nil {
		return nil, fmt.Errorf("GetTradesByTimeRange: %w", err)
	}
	return trades, nil
}

// GetStockStats calculates statistics based on recent history
// Uses the candle_1min materialized view for efficient aggregation
func (r *Repository) GetStockStats(symbol string, lookbackMinutes int) (*types.StockStats, error) {
	var stats types.StockStats

	// Query candle_1min view for more efficient stats
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

	err := r.db.Raw(query, symbol, lookbackMinutes).Scan(&stats).Error
	if err != nil {
		return nil, fmt.Errorf("GetStockStats: %w", err)
	}

	return &stats, nil
}

// GetPriceVolumeZScores calculates real-time z-scores for a stock
// Returns z-scores for current price and volume compared to historical baseline
func (r *Repository) GetPriceVolumeZScores(symbol string, currentPrice, currentVolume float64, lookbackMinutes int) (*types.ZScoreData, error) {
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

	err := r.db.Raw(query, symbol, lookbackMinutes).Scan(&result).Error
	if err != nil {
		return nil, fmt.Errorf("GetPriceVolumeZScores: %w", err)
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

	return &types.ZScoreData{
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

package database

import (
	"fmt"
	"time"
)

// Dashboard-specific data structures

// BuySellPressure represents buy/sell volume pressure metrics
type BuySellPressure struct {
	StockSymbol    string    `json:"stock_symbol"`
	TimeWindow     string    `json:"time_window"`
	BuyVolume      float64   `json:"buy_volume"`
	SellVolume     float64   `json:"sell_volume"`
	TotalVolume    float64   `json:"total_volume"`
	BuyPressurePct float64   `json:"buy_pressure_pct"`
	Sentiment      string    `json:"market_sentiment"`
	Timestamp      time.Time `json:"timestamp"`
}

// VolumeSpike represents a detected volume spike
type VolumeSpike struct {
	StockSymbol   string    `json:"stock_symbol"`
	CurrentVolume float64   `json:"current_volume"`
	AvgVolume     float64   `json:"avg_volume_5m"`
	SpikePct      float64   `json:"spike_pct"`
	DetectedAt    time.Time `json:"detected_at"`
	AlertLevel    string    `json:"alert_level"`
}

// PowerCandle represents a candle with significant price movement
type PowerCandle struct {
	Candle
	PriceChangePct float64 `json:"price_change_pct"`
	IsPowerCandle  bool    `json:"is_power_candle"`
}

// VWAPPoint represents a VWAP data point
type VWAPPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	StockSymbol string    `json:"stock_symbol"`
	VWAP        float64   `json:"vwap"`
	Price       float64   `json:"price"`
	Volume      float64   `json:"volume"`
}

// ZScoreRanking represents a stock ranked by Z-Score
type ZScoreRanking struct {
	StockSymbol  string    `json:"stock_symbol"`
	ZScore       float64   `json:"z_score"`
	Action       string    `json:"action"`
	TriggerValue float64   `json:"trigger_value"`
	DetectedAt   time.Time `json:"detected_at"`
	Rank         int       `json:"rank"`
}

// Dashboard Query Methods

// GetRecentTradesRG retrieves recent running trades filtered by Regular market (RG)
func (r *TradeRepository) GetRecentTradesRG(limit int, stockSymbol string, actionFilter string) ([]Trade, error) {
	var trades []Trade
	query := r.db.db.Where("market_board = ?", "RG").Order("timestamp DESC")

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

// GetBuySellPressure calculates buy/sell volume pressure for a time window
func (r *TradeRepository) GetBuySellPressure(stockSymbol string, windowMinutes int) (*BuySellPressure, error) {
	var result struct {
		BuyVolume  float64
		SellVolume float64
	}

	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN action = 'BUY' THEN volume_lot ELSE 0 END), 0) as buy_volume,
			COALESCE(SUM(CASE WHEN action = 'SELL' THEN volume_lot ELSE 0 END), 0) as sell_volume
		FROM running_trades
		WHERE market_board = 'RG'
		AND timestamp >= NOW() - INTERVAL '1 minute' * ?
	`

	args := []interface{}{windowMinutes}
	if stockSymbol != "" {
		query += " AND stock_symbol = ?"
		args = append(args, stockSymbol)
	}

	err := r.db.db.Raw(query, args...).Scan(&result).Error
	if err != nil {
		return nil, err
	}

	totalVolume := result.BuyVolume + result.SellVolume
	buyPressurePct := 0.0
	if totalVolume > 0 {
		buyPressurePct = (result.BuyVolume / totalVolume) * 100
	}

	sentiment := "NEUTRAL"
	if buyPressurePct > 60 {
		sentiment = "BULLISH"
	} else if buyPressurePct < 40 {
		sentiment = "BEARISH"
	}

	pressure := &BuySellPressure{
		StockSymbol:    stockSymbol,
		TimeWindow:     fmt.Sprintf("%dm", windowMinutes),
		BuyVolume:      result.BuyVolume,
		SellVolume:     result.SellVolume,
		TotalVolume:    totalVolume,
		BuyPressurePct: buyPressurePct,
		Sentiment:      sentiment,
		Timestamp:      time.Now(),
	}

	return pressure, nil
}

// GetTopSymbolsByActivity returns top N symbols by volume with their pressure data
func (r *TradeRepository) GetTopSymbolsByActivity(windowMinutes int, limit int) ([]BuySellPressure, error) {
	var results []struct {
		StockSymbol string
		BuyVolume   float64
		SellVolume  float64
		TotalVolume float64
	}

	// Use string formatting for interval instead of placeholder multiplication
	query := fmt.Sprintf(`
		SELECT 
			stock_symbol,
			COALESCE(SUM(CASE WHEN action = 'BUY' THEN volume_lot ELSE 0 END), 0) as buy_volume,
			COALESCE(SUM(CASE WHEN action = 'SELL' THEN volume_lot ELSE 0 END), 0) as sell_volume,
			COALESCE(SUM(volume_lot), 0) as total_volume
		FROM running_trades
		WHERE market_board = 'RG'
		AND timestamp >= NOW() - INTERVAL '%d minutes'
		GROUP BY stock_symbol
		HAVING COALESCE(SUM(volume_lot), 0) > 0
		ORDER BY 4 DESC
		LIMIT ?
	`, windowMinutes)

	err := r.db.db.Raw(query, limit).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	pressures := make([]BuySellPressure, 0, len(results))
	for _, result := range results {
		buyPressurePct := 0.0
		if result.TotalVolume > 0 {
			buyPressurePct = (result.BuyVolume / result.TotalVolume) * 100
		}

		sentiment := "NEUTRAL"
		if buyPressurePct > 60 {
			sentiment = "BULLISH"
		} else if buyPressurePct < 40 {
			sentiment = "BEARISH"
		}

		pressures = append(pressures, BuySellPressure{
			StockSymbol:    result.StockSymbol,
			TimeWindow:     fmt.Sprintf("%dm", windowMinutes),
			BuyVolume:      result.BuyVolume,
			SellVolume:     result.SellVolume,
			TotalVolume:    result.TotalVolume,
			BuyPressurePct: buyPressurePct,
			Sentiment:      sentiment,
			Timestamp:      time.Now(),
		})
	}

	return pressures, nil
}

// GetLargeTransactions retrieves transactions above a value threshold
func (r *TradeRepository) GetLargeTransactions(thresholdValue float64, limit int, hoursBack int) ([]Trade, error) {
	var trades []Trade

	query := r.db.db.Where("total_amount >= ?", thresholdValue).
		Where("market_board = ?", "RG").
		Where("timestamp >= NOW() - INTERVAL '1 hour' * ?", hoursBack).
		Order("timestamp DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&trades).Error
	return trades, err
}

// GetVolumeSpikes detects volume spikes compared to historical average
func (r *TradeRepository) GetVolumeSpikes(minSpikePct float64, hoursBack int) ([]VolumeSpike, error) {
	var spikes []VolumeSpike

	query := `
		WITH recent_volume AS (
			SELECT 
				stock_symbol,
				time_bucket('5 minutes', bucket) as time_bucket,
				SUM(volume_lots) as volume_5m
			FROM candle_1min
			WHERE bucket >= NOW() - INTERVAL '1 hour' * ?
			GROUP BY stock_symbol, time_bucket
		),
		avg_volume AS (
			SELECT 
				stock_symbol,
				AVG(volume_5m) as avg_volume,
				STDDEV(volume_5m) as stddev_volume
			FROM recent_volume
			WHERE time_bucket < (SELECT MAX(time_bucket) FROM recent_volume)
			GROUP BY stock_symbol
		),
		latest_volume AS (
			SELECT 
				stock_symbol,
				volume_5m as current_volume,
				time_bucket
			FROM recent_volume
			WHERE time_bucket = (SELECT MAX(time_bucket) FROM recent_volume)
		)
		SELECT 
			l.stock_symbol,
			l.current_volume,
			COALESCE(a.avg_volume, 0) as avg_volume,
			CASE 
				WHEN a.avg_volume > 0 THEN ((l.current_volume - a.avg_volume) / a.avg_volume * 100)
				ELSE 0
			END as spike_pct,
			l.time_bucket as detected_at,
			CASE 
				WHEN ((l.current_volume - COALESCE(a.avg_volume, 0)) / NULLIF(a.avg_volume, 0) * 100) >= 1000 THEN 'EXTREME'
				WHEN ((l.current_volume - COALESCE(a.avg_volume, 0)) / NULLIF(a.avg_volume, 0) * 100) >= 500 THEN 'HIGH'
				WHEN ((l.current_volume - COALESCE(a.avg_volume, 0)) / NULLIF(a.avg_volume, 0) * 100) >= 200 THEN 'MEDIUM'
				ELSE 'LOW'
			END as alert_level
		FROM latest_volume l
		LEFT JOIN avg_volume a ON l.stock_symbol = a.stock_symbol
		WHERE CASE 
			WHEN a.avg_volume > 0 THEN ((l.current_volume - a.avg_volume) / a.avg_volume * 100)
			ELSE 0
		END >= ?
		ORDER BY spike_pct DESC
		LIMIT 50
	`

	err := r.db.db.Raw(query, hoursBack, minSpikePct).Scan(&spikes).Error
	return spikes, err
}

// GetZScoreRanking returns stocks ranked by Z-Score
func (r *TradeRepository) GetZScoreRanking(minZScore float64, hoursBack int, limit int) ([]ZScoreRanking, error) {
	var rankings []ZScoreRanking

	query := `
		SELECT 
			stock_symbol,
			z_score,
			action,
			trigger_value,
			detected_at,
			ROW_NUMBER() OVER (ORDER BY z_score DESC) as rank
		FROM whale_alerts
		WHERE z_score >= ?
		AND detected_at >= NOW() - INTERVAL '1 hour' * ?
		ORDER BY z_score DESC
		LIMIT ?
	`

	err := r.db.db.Raw(query, minZScore, hoursBack, limit).Scan(&rankings).Error
	return rankings, err
}

// GetPowerCandles scans for candles with significant price changes
func (r *TradeRepository) GetPowerCandles(minChangePct float64, minVolume float64, hoursBack int) ([]PowerCandle, error) {
	var candles []PowerCandle

	query := `
		SELECT 
			*,
			((close - open) / NULLIF(open, 0) * 100) as price_change_pct,
			(
				ABS((close - open) / NULLIF(open, 0) * 100) >= ? 
				AND volume_lots >= ?
			) as is_power_candle
		FROM candle_1min
		WHERE bucket >= NOW() - INTERVAL '1 hour' * ?
		AND ABS((close - open) / NULLIF(open, 0) * 100) >= ?
		AND volume_lots >= ?
		ORDER BY ABS((close - open) / NULLIF(open, 0) * 100) DESC
		LIMIT 50
	`

	err := r.db.db.Raw(query, minChangePct, minVolume, hoursBack, minChangePct, minVolume).Scan(&candles).Error
	return candles, err
}

// CalculateVWAP calculates Volume Weighted Average Price for a stock
func (r *TradeRepository) CalculateVWAP(stockSymbol string, startTime time.Time) (*VWAPPoint, error) {
	var result struct {
		VWAP        float64
		TotalVolume float64
		LastPrice   float64
	}

	query := `
		SELECT 
			COALESCE(SUM(price * volume) / NULLIF(SUM(volume), 0), 0) as vwap,
			COALESCE(SUM(volume), 0) as total_volume,
			LAST(price, timestamp) as last_price
		FROM running_trades
		WHERE stock_symbol = ?
		AND timestamp >= ?
		AND market_board = 'RG'
	`

	err := r.db.db.Raw(query, stockSymbol, startTime).Scan(&result).Error
	if err != nil {
		return nil, err
	}

	vwapPoint := &VWAPPoint{
		Timestamp:   time.Now(),
		StockSymbol: stockSymbol,
		VWAP:        result.VWAP,
		Price:       result.LastPrice,
		Volume:      result.TotalVolume,
	}

	return vwapPoint, nil
}

// GetVWAPHistory retrieves VWAP data points over time
func (r *TradeRepository) GetVWAPHistory(stockSymbol string, startTime time.Time, intervalMinutes int) ([]VWAPPoint, error) {
	var vwapPoints []VWAPPoint

	query := `
		SELECT 
			time_bucket(INTERVAL '1 minute' * $1, timestamp) as timestamp,
			stock_symbol,
			SUM(price * volume) / NULLIF(SUM(volume), 0) as vwap,
			LAST(price, timestamp) as price,
			SUM(volume) as volume
		FROM running_trades
		WHERE stock_symbol = $2
		AND timestamp >= $3
		AND market_board = 'RG'
		GROUP BY time_bucket(INTERVAL '1 minute' * $1, timestamp), stock_symbol
		ORDER BY timestamp ASC
	`

	err := r.db.db.Raw(query, intervalMinutes, stockSymbol, startTime).Scan(&vwapPoints).Error
	return vwapPoints, err
}

// GetCandlesWithWhaleAlerts retrieves candles with associated whale alerts
func (r *TradeRepository) GetCandlesWithWhaleAlerts(stockSymbol string, startTime, endTime time.Time) (map[string]interface{}, error) {
	// Get candles
	candles, err := r.GetCandles(stockSymbol, startTime, endTime, 0)
	if err != nil {
		return nil, err
	}

	// Get whale alerts in the same time range
	whales, err := r.GetHistoricalWhales(stockSymbol, startTime, endTime, "", 0, 0)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"candles":      candles,
		"whale_alerts": whales,
		"stock_symbol": stockSymbol,
		"start_time":   startTime,
		"end_time":     endTime,
	}

	return result, nil
}

// GetAllPressureGauges retrieves pressure metrics for all active stocks
func (r *TradeRepository) GetAllPressureGauges(windowMinutes int, minVolume float64) ([]BuySellPressure, error) {
	var pressures []BuySellPressure

	query := `
		SELECT 
			stock_symbol,
			? as time_window,
			COALESCE(SUM(CASE WHEN action = 'BUY' THEN volume_lot ELSE 0 END), 0) as buy_volume,
			COALESCE(SUM(CASE WHEN action = 'SELL' THEN volume_lot ELSE 0 END), 0) as sell_volume,
			COALESCE(SUM(volume_lot), 0) as total_volume,
			CASE 
				WHEN SUM(volume_lot) > 0 THEN 
					(SUM(CASE WHEN action = 'BUY' THEN volume_lot ELSE 0 END) / SUM(volume_lot) * 100)
				ELSE 0
			END as buy_pressure_pct,
			CASE 
				WHEN (SUM(CASE WHEN action = 'BUY' THEN volume_lot ELSE 0 END) / NULLIF(SUM(volume_lot), 0) * 100) > 60 THEN 'BULLISH'
				WHEN (SUM(CASE WHEN action = 'BUY' THEN volume_lot ELSE 0 END) / NULLIF(SUM(volume_lot), 0) * 100) < 40 THEN 'BEARISH'
				ELSE 'NEUTRAL'
			END as sentiment,
			NOW() as timestamp
		FROM running_trades
		WHERE market_board = 'RG'
		AND timestamp >= NOW() - INTERVAL '1 minute' * ?
		GROUP BY stock_symbol
		HAVING SUM(volume_lot) >= ?
		ORDER BY total_volume DESC
	`

	timeWindow := fmt.Sprintf("%dm", windowMinutes)
	err := r.db.db.Raw(query, timeWindow, windowMinutes, minVolume).Scan(&pressures).Error
	return pressures, err
}

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleGetWhales(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()
	symbol := query.Get("symbol")
	alertType := query.Get("type")
	action := query.Get("action") // NEW: Filter for BUY/SELL
	board := query.Get("board")

	limitStr := query.Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			if val > 200 {
				val = 200 // Cap at 200
			}
			limit = val
		}
	}

	offsetStr := query.Get("offset")
	offset := 0
	if offsetStr != "" {
		if val, err := strconv.Atoi(offsetStr); err == nil && val >= 0 {
			offset = val
		}
	}

	// Parse min_value filter (frontend sends min_value, not min_amount)
	minAmount := 0.0
	if minValueStr := query.Get("min_value"); minValueStr != "" {
		if val, err := strconv.ParseFloat(minValueStr, 64); err == nil && val >= 0 {
			minAmount = val
		}
	}

	// Time range parsing (RFC3339)
	var startTime, endTime time.Time
	if startStr := query.Get("start"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	}
	if endStr := query.Get("end"); endStr != "" {
		endTime, _ = time.Parse(time.RFC3339, endStr)
	}

	whales, err := s.repo.GetHistoricalWhales(symbol, startTime, endTime, alertType, action, board, minAmount, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get total count for pagination metadata
	totalCount, err := s.repo.GetWhaleCount(symbol, startTime, endTime, alertType, action, board, minAmount)
	if err != nil {
		// If count fails, still return data but without total
		totalCount = 0
	}

	// Return response with pagination metadata
	response := map[string]interface{}{
		"data":     whales,
		"total":    totalCount,
		"limit":    limit,
		"offset":   offset,
		"has_more": int64(offset+len(whales)) < totalCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleGetWhaleStats(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()
	symbol := query.Get("symbol")

	// Time range parsing
	var startTime, endTime time.Time

	// Default to Today 8:00 AM WIB if no start time is provided
	if startStr := query.Get("start"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	} else {
		// Calculate start time based on 8 AM WIB daily reset
		loc := time.FixedZone("WIB", 7*60*60) // Asia/Jakarta timezone (UTC+7)
		now := time.Now().In(loc)

		// If current time is before 8 AM, use yesterday 8 AM
		// If current time is after 8 AM, use today 8 AM
		today8AM := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, loc)

		if now.Before(today8AM) {
			// Before 8 AM today, use yesterday 8 AM
			startTime = today8AM.AddDate(0, 0, -1)
		} else {
			// After 8 AM today, use today 8 AM
			startTime = today8AM
		}
	}

	if endStr := query.Get("end"); endStr != "" {
		endTime, _ = time.Parse(time.RFC3339, endStr)
	}

	stats, err := s.repo.GetWhaleStats(symbol, startTime, endTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch global performance stats (system win rate)
	globalStats, err := s.repo.GetGlobalPerformanceStats()
	var winRate float64
	var avgProfit float64

	if err == nil && globalStats != nil {
		winRate = globalStats.WinRate
		avgProfit = globalStats.AvgProfitPct
	}

	// Create merged response
	response := map[string]interface{}{
		"stock_symbol":        stats.StockSymbol,
		"total_whale_trades":  stats.TotalWhaleTrades,
		"total_whale_value":   stats.TotalWhaleValue,
		"buy_volume_lots":     stats.BuyVolumeLots,
		"sell_volume_lots":    stats.SellVolumeLots,
		"largest_trade_value": stats.LargestTradeValue,
		"win_rate":            winRate,   // Added field for frontend
		"avg_profit_pct":      avgProfit, // Added field for frontend
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetCandles returns candles for a specific timeframe with technical analysis
func (s *Server) handleGetCandles(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	symbol := query.Get("symbol")
	timeframe := query.Get("timeframe") // 1min, 5min, 15min, 1hour, 1day

	if symbol == "" || timeframe == "" {
		http.Error(w, "Symbol and timeframe are required", http.StatusBadRequest)
		return
	}

	limit := 100
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	candles, err := s.repo.GetCandlesByTimeframe(timeframe, symbol, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculate technical indicators
	analysis := calculateTechnicalAnalysis(candles)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"candles":    candles,
		"symbol":     symbol,
		"timeframe":  timeframe,
		"count":      len(candles),
		"indicators": analysis,
	})
}

// calculateTechnicalAnalysis computes RSI, SMA, trend, and momentum from candle data
func calculateTechnicalAnalysis(candles []map[string]interface{}) map[string]interface{} {
	if len(candles) < 20 {
		return map[string]interface{}{
			"trend":       "NEUTRAL",
			"momentum":    "NEUTRAL",
			"rsi":         50.0,
			"sma20":       nil,
			"sma50":       nil,
			"volumeRatio": 1.0,
		}
	}

	// Extract closing prices
	closes := make([]float64, len(candles))
	volumes := make([]float64, len(candles))
	for i, c := range candles {
		if close, ok := c["close"].(float64); ok {
			closes[i] = close
		}
		if vol, ok := c["volume"].(float64); ok {
			volumes[i] = vol
		}
	}

	// Calculate SMA 20 and SMA 50
	sma20 := calculateSMA(closes, 20)
	sma50 := calculateSMA(closes, 50)

	// Calculate RSI (14 periods)
	rsi := calculateRSI(closes, 14)

	// Calculate volume ratio (current avg vs historical avg)
	avgVolume := calculateAvgVolume(volumes, min(20, len(volumes)))
	volumeRatio := 1.0
	if avgVolume > 0 && len(volumes) >= 5 {
		recentAvg := calculateAvgVolume(volumes[:5], 5)
		volumeRatio = recentAvg / avgVolume
	}

	// Determine trend
	trend := "NEUTRAL"
	if len(closes) >= 20 && sma20 != nil && sma50 != nil {
		if *sma20 > *sma50 {
			trend = "BULLISH"
		} else if *sma50 > *sma20 {
			trend = "BEARISH"
		}
	}

	// Determine momentum
	momentum := "NEUTRAL"
	if rsi != nil {
		if *rsi > 60 {
			momentum = "BULLISH"
		} else if *rsi < 40 {
			momentum = "BEARISH"
		}
	}

	return map[string]interface{}{
		"trend":       trend,
		"momentum":    momentum,
		"rsi":         rsi,
		"sma20":       sma20,
		"sma50":       sma50,
		"volumeRatio": volumeRatio,
	}
}

func calculateSMA(data []float64, period int) *float64 {
	if len(data) < period {
		return nil
	}
	sum := 0.0
	for i := len(data) - period; i < len(data); i++ {
		sum += data[i]
	}
	val := sum / float64(period)
	return &val
}

func calculateRSI(data []float64, period int) *float64 {
	if len(data) < period+1 {
		return nil
	}

	gains := 0.0
	losses := 0.0

	// Calculate initial average gain/loss
	for i := len(data) - period; i < len(data)-1; i++ {
		change := data[i+1] - data[i]
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		val := 100.0
		return &val
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))
	return &rsi
}

func calculateAvgVolume(volumes []float64, period int) float64 {
	if len(volumes) == 0 || period <= 0 {
		return 0
	}
	p := min(period, len(volumes))
	sum := 0.0
	for i := len(volumes) - p; i < len(volumes); i++ {
		sum += volumes[i]
	}
	return sum / float64(p)
}

// handleGetWhaleFollowup returns followup data for a whale alert
func (s *Server) handleGetWhaleFollowup(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid whale alert ID", http.StatusBadRequest)
		return
	}

	followup, err := s.repo.GetWhaleFollowup(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if followup == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Followup not found",
		})
		return
	}

	// Calculate current_price from the most recent available price field
	currentPrice := followup.AlertPrice
	if followup.Price1MinLater != nil && *followup.Price1MinLater > 0 {
		currentPrice = *followup.Price1MinLater
	}
	if followup.Price5MinLater != nil && *followup.Price5MinLater > 0 {
		currentPrice = *followup.Price5MinLater
	}
	if followup.Price15MinLater != nil && *followup.Price15MinLater > 0 {
		currentPrice = *followup.Price15MinLater
	}
	if followup.Price30MinLater != nil && *followup.Price30MinLater > 0 {
		currentPrice = *followup.Price30MinLater
	}
	if followup.Price60MinLater != nil && *followup.Price60MinLater > 0 {
		currentPrice = *followup.Price60MinLater
	}
	if followup.Price1DayLater != nil && *followup.Price1DayLater > 0 {
		currentPrice = *followup.Price1DayLater
	}

	// Create response with current_price and detected_at fields
	response := map[string]interface{}{
		"id":                    followup.ID,
		"whale_alert_id":        followup.WhaleAlertID,
		"stock_symbol":          followup.StockSymbol,
		"alert_time":            followup.AlertTime,
		"detected_at":           followup.AlertTime, // Alias for frontend compatibility
		"alert_price":           followup.AlertPrice,
		"alert_action":          followup.AlertAction,
		"current_price":         currentPrice,
		"price_1min_later":      followup.Price1MinLater,
		"price_5min_later":      followup.Price5MinLater,
		"price_15min_later":     followup.Price15MinLater,
		"price_30min_later":     followup.Price30MinLater,
		"price_60min_later":     followup.Price60MinLater,
		"price_1day_later":      followup.Price1DayLater,
		"change_1min_pct":       followup.Change1MinPct,
		"change_5min_pct":       followup.Change5MinPct,
		"change_15min_pct":      followup.Change15MinPct,
		"change_30min_pct":      followup.Change30MinPct,
		"change_60min_pct":      followup.Change60MinPct,
		"change_1day_pct":       followup.Change1DayPct,
		"immediate_impact":      followup.ImmediateImpact,
		"sustained_impact":      followup.SustainedImpact,
		"reversal_detected":     followup.ReversalDetected,
		"reversal_time_minutes": followup.ReversalTimeMinutes,
		"analysis":              followup.Analysis,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetWhaleFollowups returns list of whale followups with filters
func (s *Server) handleGetWhaleFollowups(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	symbol := query.Get("symbol")
	status := query.Get("status") // active, completed, all

	limit := 50
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 200 {
				limit = 200
			}
		}
	}

	followups, err := s.repo.GetWhaleFollowups(symbol, status, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"followups": followups,
		"count":     len(followups),
	})
}

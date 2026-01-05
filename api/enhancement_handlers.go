package api

// Phase 1 Enhancement Handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

// handleGetSignalHistory returns historical trading signals with filters
func (s *Server) handleGetSignalHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	symbol := query.Get("symbol")
	strategy := query.Get("strategy")
	decision := query.Get("decision")

	limit := 100
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 500 {
				limit = 500
			}
		}
	}

	var startTime, endTime time.Time
	if start := query.Get("start"); start != "" {
		startTime, _ = time.Parse(time.RFC3339, start)
	}
	if end := query.Get("end"); end != "" {
		endTime, _ = time.Parse(time.RFC3339, end)
	}

	signals, err := s.repo.GetTradingSignals(symbol, strategy, decision, startTime, endTime, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"signals": signals,
		"count":   len(signals),
	})
}

// handleGetSignalPerformance returns performance statistics for strategies
func (s *Server) handleGetSignalPerformance(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	strategy := query.Get("strategy")
	symbol := query.Get("symbol")

	stats, err := s.repo.GetSignalPerformanceStats(strategy, symbol)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleGetSignalOutcome returns outcome for a specific signal
func (s *Server) handleGetSignalOutcome(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid signal ID", http.StatusBadRequest)
		return
	}

	outcome, err := s.repo.GetSignalOutcomeBySignalID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if outcome == nil {
		http.Error(w, "Outcome not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(outcome)
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

// handleGetOrderFlow returns order flow imbalance data
func (s *Server) handleGetOrderFlow(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	symbol := query.Get("symbol")

	limit := 100
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 500 {
				limit = 500
			}
		}
	}

	var startTime, endTime time.Time
	if start := query.Get("start"); start != "" {
		startTime, _ = time.Parse(time.RFC3339, start)
	}
	if end := query.Get("end"); end != "" {
		endTime, _ = time.Parse(time.RFC3339, end)
	}

	log.Printf("📊 Fetching order flow for symbol: %s (limit: %d)", symbol, limit)

	flows, err := s.repo.GetOrderFlowImbalance(symbol, startTime, endTime, limit)
	if err != nil {
		log.Printf("❌ Failed to fetch order flow for %s: %v", symbol, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Returning %d order flow records for %s", len(flows), symbol)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"flows": flows,
		"count": len(flows),
	})
}

// Phase 2 Enhancement Handlers

// handleGetStatisticalBaselines returns latest statistical baselines for a symbol
func (s *Server) handleGetStatisticalBaselines(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "Symbol is required", http.StatusBadRequest)
		return
	}

	baseline, err := s.repo.GetLatestBaseline(symbol)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(baseline)
}

// handleGetMarketRegimes returns latest market regimes for a symbol
func (s *Server) handleGetMarketRegimes(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "Symbol is required", http.StatusBadRequest)
		return
	}

	regime, err := s.repo.GetLatestRegime(symbol)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(regime)
}

// handleGetDetectedPatterns returns recently detected patterns
func (s *Server) handleGetDetectedPatterns(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	symbol := query.Get("symbol")

	since := time.Now().Add(-24 * time.Hour)
	if s := query.Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}

	patterns, err := s.repo.GetRecentPatterns(symbol, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"patterns": patterns,
		"count":    len(patterns),
	})
}

// handleGetCandles returns candles for a specific timeframe
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"candles":   candles,
		"symbol":    symbol,
		"timeframe": timeframe,
		"count":     len(candles),
	})
}

// Phase 3 Enhancement Handlers

// handleGetStockCorrelations returns correlations for a symbol
func (s *Server) handleGetStockCorrelations(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "Symbol is required", http.StatusBadRequest)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	log.Printf("📊 Fetching correlations for symbol: %s (limit: %d)", symbol, limit)

	correlations, err := s.repo.GetStockCorrelations(symbol, limit)
	if err != nil {
		log.Printf("❌ Failed to fetch correlations for %s: %v", symbol, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Returning %d correlations for %s", len(correlations), symbol)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"symbol":       symbol,
		"correlations": correlations,
		"count":        len(correlations),
	})
}

// handleGetDailyPerformance returns daily strategy performance analytics
func (s *Server) handleGetDailyPerformance(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	strategy := query.Get("strategy")
	symbol := query.Get("symbol")

	limit := 30
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	log.Printf("📈 Fetching daily performance (strategy: %s, symbol: %s, limit: %d)", strategy, symbol, limit)

	performance, err := s.repo.GetDailyStrategyPerformance(strategy, symbol, limit)
	if err != nil {
		log.Printf("❌ Failed to fetch daily performance: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Returning %d performance records", len(performance))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"performance": performance,
		"strategy":    strategy,
		"symbol":      symbol,
		"count":       len(performance),
	})
}

// handleGetOpenPositions returns currently open trading positions
func (s *Server) handleGetOpenPositions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	symbol := query.Get("symbol")
	strategy := query.Get("strategy")

	limit := 50
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 100 {
				limit = 100
			}
		}
	}

	log.Printf("📊 Fetching open positions (symbol: %s, strategy: %s, limit: %d)", symbol, strategy, limit)

	// Check if signal tracker is available
	if s.signalTracker == nil {
		log.Printf("⚠️ Signal tracker not initialized")
		http.Error(w, "Signal tracker not available", http.StatusServiceUnavailable)
		return
	}

	// Use case: Get open positions through signal tracker
	positions, err := s.signalTracker.GetOpenPositions(symbol, strategy, limit)
	if err != nil {
		log.Printf("❌ Failed to fetch open positions: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Found %d open positions", len(positions))

	// Enrich positions with signal details for UI
	enrichedPositions := make([]map[string]interface{}, 0, len(positions))
	for _, pos := range positions {
		// Get the signal details to include strategy and confidence
		signal, err := s.repo.GetSignalByID(pos.SignalID)
		if err != nil {
			log.Printf("⚠️ Failed to get signal %d: %v", pos.SignalID, err)
			continue
		}

		// Calculate current P&L percentage
		var currentPnL float64
		if pos.ProfitLossPct != nil {
			currentPnL = *pos.ProfitLossPct
		}

		// Calculate holding time in minutes
		holdingMins := 0
		if pos.HoldingPeriodMinutes != nil {
			holdingMins = *pos.HoldingPeriodMinutes
		}

		enrichedPos := map[string]interface{}{
			"id":             pos.ID,
			"signal_id":      pos.SignalID,
			"stock_symbol":   pos.StockSymbol,
			"strategy":       signal.Strategy,
			"entry_time":     pos.EntryTime,
			"entry_price":    pos.EntryPrice,
			"entry_decision": pos.EntryDecision,
			"current_pnl":    currentPnL,
			"holding_mins":   holdingMins,
			"mfe":            pos.MaxFavorableExcursion,
			"mae":            pos.MaxAdverseExcursion,
			"confidence":     signal.Confidence,
			"status":         pos.OutcomeStatus,
		}

		enrichedPositions = append(enrichedPositions, enrichedPos)
	}

	log.Printf("✅ Returning %d enriched open positions", len(enrichedPositions))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"positions": enrichedPositions,
		"count":     len(enrichedPositions),
	})
}

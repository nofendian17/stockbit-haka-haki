package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"stockbit-haka-haki/database"
	"strconv"
	"time"
)

// handleGetStrategySignals returns recent strategy signals in JSON format
func (s *Server) handleGetStrategySignals(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()

	lookbackMinutes := 60
	if l := query.Get("lookback"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			lookbackMinutes = parsed
		}
	}

	minConfidence := 0.3
	if c := query.Get("min_confidence"); c != "" {
		if parsed, err := strconv.ParseFloat(c, 64); err == nil {
			minConfidence = parsed
		}
	}

	strategyFilter := query.Get("strategy") // "VOLUME_BREAKOUT", "MEAN_REVERSION", "FAKEOUT_FILTER", or "ALL"

	log.Printf("📊 Fetching strategy signals (lookback: %d min, confidence: %.2f, strategy: %s)",
		lookbackMinutes, minConfidence, strategyFilter)

	// Get strategy signals
	signals, err := s.repo.GetStrategySignals(lookbackMinutes, minConfidence, strategyFilter)
	if err != nil {
		log.Printf("❌ Error fetching strategy signals: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure signals is never nil
	if signals == nil {
		signals = []database.TradingSignal{}
	}

	log.Printf("✅ Returning %d strategy signals", len(signals))

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"signals": signals,
		"count":   len(signals),
	}); err != nil {
		log.Printf("❌ Error encoding JSON response: %v", err)
	}
}

// handleStrategySignalsStream streams strategy signals via SSE
func (s *Server) handleStrategySignalsStream(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()

	strategyFilter := query.Get("strategy") // Filter by strategy type

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial connection message
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Create ticker for periodic signal evaluation (every 5 seconds)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Track sent signals to avoid duplicates
	sentSignals := make(map[string]bool)

	// Send signals periodically
	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			log.Println("Strategy SSE client disconnected")
			return

		case <-ticker.C:
			// Get recent signals (last 5 minutes for real-time updates only)
			signals, err := s.repo.GetStrategySignals(5, 0.3, strategyFilter)
			if err != nil {
				log.Printf("Error getting strategy signals: %v", err)
				continue
			}

			// Send new signals only
			for _, signal := range signals {
				// Create unique key for signal
				signalKey := fmt.Sprintf("%s-%s-%s-%d",
					signal.StockSymbol,
					signal.Strategy,
					signal.Decision,
					signal.Timestamp.Unix())

				// Skip if already sent
				if sentSignals[signalKey] {
					continue
				}

				// Marshal signal to JSON
				signalJSON, err := json.Marshal(signal)
				if err != nil {
					log.Printf("Error marshaling signal: %v", err)
					continue
				}

				// Send signal via SSE
				fmt.Fprintf(w, "event: signal\ndata: %s\n\n", signalJSON)
				flusher.Flush()

				// Mark as sent
				sentSignals[signalKey] = true
			}

			// Clean up old sent signals (keep last 100)
			if len(sentSignals) > 100 {
				sentSignals = make(map[string]bool)
			}
		}
	}
}

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
			"id":                      pos.ID,
			"signal_id":               pos.SignalID,
			"stock_symbol":            pos.StockSymbol,
			"strategy":                signal.Strategy,
			"entry_time":              pos.EntryTime,
			"entry_price":             pos.EntryPrice,
			"entry_decision":          pos.EntryDecision,
			"profit_loss_pct":         currentPnL,
			"holding_period_minutes":  holdingMins,
			"max_favorable_excursion": pos.MaxFavorableExcursion,
			"max_adverse_excursion":   pos.MaxAdverseExcursion,
			"confidence":              signal.Confidence,
			"outcome_status":          pos.OutcomeStatus,
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

// handleGetProfitLossHistory returns profit/loss history with status
func (s *Server) handleGetProfitLossHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	symbol := query.Get("symbol")
	strategy := query.Get("strategy")
	status := query.Get("status") // WIN, LOSS, BREAKEVEN, OPEN

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

	log.Printf("📊 Fetching P&L history (symbol: %s, strategy: %s, status: %s, limit: %d)",
		symbol, strategy, status, limit)

	outcomes, err := s.repo.GetSignalOutcomes(symbol, status, startTime, endTime, limit)
	if err != nil {
		log.Printf("❌ Failed to fetch P&L history: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Enrich with signal details
	enrichedOutcomes := make([]map[string]interface{}, 0, len(outcomes))
	for _, outcome := range outcomes {
		// Get signal details
		signal, err := s.repo.GetSignalByID(outcome.SignalID)
		if err != nil {
			log.Printf("⚠️ Failed to get signal %d: %v", outcome.SignalID, err)
			continue
		}

		// Filter by strategy if specified
		if strategy != "" && strategy != "ALL" && signal.Strategy != strategy {
			continue
		}

		// Calculate duration display
		durationStr := "N/A"
		if outcome.HoldingPeriodMinutes != nil {
			mins := *outcome.HoldingPeriodMinutes
			if mins < 60 {
				durationStr = fmt.Sprintf("%d menit", mins)
			} else if mins < 1440 {
				hours := mins / 60
				remainMins := mins % 60
				if remainMins > 0 {
					durationStr = fmt.Sprintf("%d jam %d menit", hours, remainMins)
				} else {
					durationStr = fmt.Sprintf("%d jam", hours)
				}
			} else {
				days := mins / 1440
				remainHours := (mins % 1440) / 60
				if remainHours > 0 {
					durationStr = fmt.Sprintf("%d hari %d jam", days, remainHours)
				} else {
					durationStr = fmt.Sprintf("%d hari", days)
				}
			}
		}

		enriched := map[string]interface{}{
			"id":                       outcome.ID,
			"signal_id":                outcome.SignalID,
			"stock_symbol":             outcome.StockSymbol,
			"strategy":                 signal.Strategy,
			"entry_time":               outcome.EntryTime,
			"entry_price":              outcome.EntryPrice,
			"entry_decision":           outcome.EntryDecision,
			"exit_time":                outcome.ExitTime,
			"exit_price":               outcome.ExitPrice,
			"exit_reason":              outcome.ExitReason,
			"holding_period_minutes":   outcome.HoldingPeriodMinutes,
			"holding_duration_display": durationStr,
			"price_change_pct":         outcome.PriceChangePct,
			"profit_loss_pct":          outcome.ProfitLossPct,
			"max_favorable_excursion":  outcome.MaxFavorableExcursion,
			"max_adverse_excursion":    outcome.MaxAdverseExcursion,
			"risk_reward_ratio":        outcome.RiskRewardRatio,
			"outcome_status":           outcome.OutcomeStatus,
			"confidence":               signal.Confidence,
		}

		enrichedOutcomes = append(enrichedOutcomes, enriched)
	}

	log.Printf("✅ Returning %d P&L history records", len(enrichedOutcomes))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"history": enrichedOutcomes,
		"count":   len(enrichedOutcomes),
	})
}

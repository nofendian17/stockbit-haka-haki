package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"stockbit-haka-haki/database"
	"stockbit-haka-haki/llm"
)

// Market hours constants (mirrored from app/signal_tracker.go to avoid import cycle)
const (
	marketOpenHour  = 9  // 09:00 WIB - market open
	marketCloseHour = 16 // 16:00 WIB - market close
	marketTimeZone  = "Asia/Jakarta"
	millionDivisor  = 1_000_000
)

// safeFloat64 safely dereferences a float64 pointer, returning defaultValue if nil
func safeFloat64(ptr *float64, defaultValue float64) float64 {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// handleAccumulationPattern returns accumulation patterns
func (s *Server) handleAccumulationPattern(w http.ResponseWriter, r *http.Request) {
	hoursBack := getIntParam(r, "hours", 24, nil, nil)
	minAlerts := getIntParam(r, "min_alerts", 3, nil, nil)

	patterns, err := s.repo.GetAccumulationPattern(hoursBack, minAlerts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"patterns":    patterns,
		"llm_enabled": s.llmEnabled,
	}

	// Add LLM insights if enabled
	if s.llmEnabled && s.llmClient != nil && len(patterns) > 0 {
		// Fetch regimes for all symbols in patterns
		regimes := make(map[string]database.MarketRegime)
		for _, p := range patterns {
			if r, err := s.repo.GetLatestRegime(p.StockSymbol); err == nil && r != nil {
				regimes[p.StockSymbol] = *r
			}
		}

		prompt := llm.FormatAccumulationPrompt(patterns, regimes)
		if insight, err := s.llmClient.Analyze(r.Context(), prompt); err == nil {
			response["llm_insight"] = insight
		} else {
			log.Printf("LLM analysis failed: %v", err)
			response["llm_error"] = err.Error()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleExtremeAnomalies returns extreme anomalies
func (s *Server) handleExtremeAnomalies(w http.ResponseWriter, r *http.Request) {
	minZScore := getFloatParam(r, "min_z", 5.0)
	hoursBack := getIntParam(r, "hours", 48, nil, nil)

	anomalies, err := s.repo.GetExtremeAnomalies(minZScore, hoursBack)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"anomalies":   anomalies,
		"min_z_score": minZScore,
		"llm_enabled": s.llmEnabled,
	}

	// Add LLM insights if enabled
	if s.llmEnabled && s.llmClient != nil && len(anomalies) > 0 {
		// Fetch regimes for all symbols in anomalies
		regimes := make(map[string]database.MarketRegime)
		for _, a := range anomalies {
			if _, ok := regimes[a.StockSymbol]; !ok {
				if r, err := s.repo.GetLatestRegime(a.StockSymbol); err == nil && r != nil {
					regimes[a.StockSymbol] = *r
				}
			}
		}

		prompt := llm.FormatAnomalyPrompt(anomalies, regimes)
		if insight, err := s.llmClient.Analyze(r.Context(), prompt); err == nil {
			response["llm_insight"] = insight
		} else {
			log.Printf("LLM analysis failed: %v", err)
			response["llm_error"] = err.Error()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTimeBasedStats returns time-based statistics
func (s *Server) handleTimeBasedStats(w http.ResponseWriter, r *http.Request) {
	daysBack := getIntParam(r, "days", 7, nil, nil)

	stats, err := s.repo.GetTimeBasedStats(daysBack)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"stats":       stats,
		"days_back":   daysBack,
		"llm_enabled": s.llmEnabled,
	}

	// Add LLM insights if enabled
	if s.llmEnabled && s.llmClient != nil && len(stats) > 0 {
		prompt := llm.FormatTimingPrompt(stats)
		if insight, err := s.llmClient.Analyze(r.Context(), prompt); err == nil {
			response["llm_insight"] = insight
		} else {
			log.Printf("LLM analysis failed: %v", err)
			response["llm_error"] = err.Error()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAccumulationPatternStream streams accumulation patterns via SSE
func (s *Server) handleAccumulationPatternStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query params
	hoursBack := getIntParam(r, "hours", 24, nil, nil)
	minAlerts := getIntParam(r, "min_alerts", 3, nil, nil)

	// Get patterns data
	patterns, err := s.repo.GetAccumulationPattern(hoursBack, minAlerts)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	if len(patterns) == 0 {
		http.Error(w, "No accumulation patterns found", http.StatusNotFound)
		return
	}

	// Set SSE headers
	flusher, ok := setupSSE(w)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "Streaming not supported", nil)
		return
	}

	// Fetch regimes for context
	regimes := make(map[string]database.MarketRegime)
	for _, p := range patterns {
		if r, err := s.repo.GetLatestRegime(p.StockSymbol); err == nil && r != nil {
			regimes[p.StockSymbol] = *r
		}
	}

	// Generate prompt
	prompt := llm.FormatAccumulationPrompt(patterns, regimes)

	// Stream LLM response
	err = s.llmClient.AnalyzeStream(r.Context(), prompt, func(chunk string) error {
		// Properly format multi-line chunks for SSE
		lines := strings.Split(chunk, "\n")
		for i, line := range lines {
			if i < len(lines)-1 {
				fmt.Fprintf(w, "data: %s\n", line)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
		}
		flusher.Flush()
		return nil
	})

	if err != nil {
		log.Printf("LLM streaming failed: %v", err)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send completion event
	fmt.Fprintf(w, "event: done\ndata: Stream completed\n\n")
	flusher.Flush()
}

// handleExtremeAnomaliesStream streams extreme anomalies via SSE
func (s *Server) handleExtremeAnomaliesStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query params
	minZScore := getFloatParam(r, "min_z", 5.0)
	hoursBack := getIntParam(r, "hours", 48, nil, nil)

	// Get anomalies data
	anomalies, err := s.repo.GetExtremeAnomalies(minZScore, hoursBack)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	if len(anomalies) == 0 {
		http.Error(w, "No extreme anomalies found", http.StatusNotFound)
		return
	}

	// Set SSE headers
	flusher, ok := setupSSE(w)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "Streaming not supported", nil)
		return
	}

	// Fetch regimes for context
	regimes := make(map[string]database.MarketRegime)
	for _, a := range anomalies {
		if _, ok := regimes[a.StockSymbol]; !ok {
			if r, err := s.repo.GetLatestRegime(a.StockSymbol); err == nil && r != nil {
				regimes[a.StockSymbol] = *r
			}
		}
	}

	// Generate prompt
	prompt := llm.FormatAnomalyPrompt(anomalies, regimes)

	// Stream LLM response
	err = s.llmClient.AnalyzeStream(r.Context(), prompt, func(chunk string) error {
		// Properly format multi-line chunks for SSE
		lines := strings.Split(chunk, "\n")
		for i, line := range lines {
			if i < len(lines)-1 {
				fmt.Fprintf(w, "data: %s\n", line)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
		}
		flusher.Flush()
		return nil
	})

	if err != nil {
		log.Printf("LLM streaming failed: %v", err)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send completion event
	fmt.Fprintf(w, "event: done\ndata: Stream completed\n\n")
	flusher.Flush()
}

// handleTimeBasedStatsStream streams time-based stats via SSE
func (s *Server) handleTimeBasedStatsStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query params
	daysBack := getIntParam(r, "days", 7, nil, nil)

	// Get time-based stats
	stats, err := s.repo.GetTimeBasedStats(daysBack)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	if len(stats) == 0 {
		http.Error(w, "No timing stats found", http.StatusNotFound)
		return
	}

	// Set SSE headers
	flusher, ok := setupSSE(w)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "Streaming not supported", nil)
		return
	}

	// Generate prompt
	prompt := llm.FormatTimingPrompt(stats)

	// Stream LLM response
	err = s.llmClient.AnalyzeStream(r.Context(), prompt, func(chunk string) error {
		// Properly format multi-line chunks for SSE
		lines := strings.Split(chunk, "\n")
		for i, line := range lines {
			if i < len(lines)-1 {
				fmt.Fprintf(w, "data: %s\n", line)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
		}
		flusher.Flush()
		return nil
	})

	if err != nil {
		log.Printf("LLM streaming failed: %v", err)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send completion event
	fmt.Fprintf(w, "event: done\ndata: Stream completed\n\n")
	flusher.Flush()
}

// handleSymbolAnalysisStream streams symbol analysis via SSE
func (s *Server) handleSymbolAnalysisStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Get symbol from query param
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "symbol parameter is required", http.StatusBadRequest)
		return
	}

	// Get limit (default 20, max 50)
	maxLimit := 50
	limit := getIntParam(r, "limit", 20, nil, &maxLimit)

	// Get recent alerts for symbol
	alerts, err := s.repo.GetRecentAlertsBySymbol(symbol, limit)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	if len(alerts) == 0 {
		http.Error(w, "No whale alerts found for this symbol", http.StatusNotFound)
		return
	}

	// Fetch enriched metadata for context
	regime, _ := s.repo.GetLatestRegime(symbol)
	baseline, _ := s.repo.GetLatestBaseline(symbol)
	orderFlow, _ := s.repo.GetLatestOrderFlow(symbol)

	// OPTIMIZATION: Use batch query to avoid N+1 problem
	var alertIDs []int64
	for _, a := range alerts {
		alertIDs = append(alertIDs, a.ID)
	}

	followups, err := s.repo.GetWhaleFollowupsByAlertIDs(alertIDs)
	if err != nil {
		log.Printf("Warning: failed to batch fetch followups: %v", err)
		// Non-fatal error, continue without followups
		followups = []database.WhaleAlertFollowup{}
	}

	// Set SSE headers
	flusher, ok := setupSSE(w)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "Streaming not supported", nil)
		return
	}

	// Generate prompt with enriched data
	prompt := llm.FormatSymbolAnalysisPrompt(symbol, alerts, regime, baseline, orderFlow, followups)

	// Stream LLM response
	err = s.llmClient.AnalyzeStream(r.Context(), prompt, func(chunk string) error {
		// Properly format multi-line chunks for SSE
		lines := strings.Split(chunk, "\n")
		for i, line := range lines {
			if i < len(lines)-1 {
				fmt.Fprintf(w, "data: %s\n", line)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
		}
		flusher.Flush()
		return nil
	})

	if err != nil {
		log.Printf("LLM streaming failed: %v", err)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send completion event
	fmt.Fprintf(w, "event: done\ndata: Stream completed\n\n")
	flusher.Flush()
}

// handleAccumulationSummary returns separate top 20 accumulation and distribution lists
// Uses market open time (09:00 WIB) as default for more accurate trading hours analysis
func (s *Server) handleAccumulationSummary(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()

	var startTime time.Time
	var hoursBack float64

	// Use market hours constants (mirrored from app/signal_tracker.go)
	loc, err := time.LoadLocation(marketTimeZone)
	if err != nil {
		loc = time.FixedZone("WIB", 7*60*60)
	}
	now := time.Now().In(loc)
	marketOpen := time.Date(now.Year(), now.Month(), now.Day(), marketOpenHour, 0, 0, 0, loc) // 09:00 WIB

	if h := query.Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil {
			hoursBack = float64(parsed)
			startTime = time.Now().Add(-time.Duration(parsed) * time.Hour)
		} else {
			// Fallback if parsing fails - use market open time
			startTime = marketOpen
			hoursBack = time.Since(startTime).Hours()
		}
	} else {
		// Default: use market open time (09:00 WIB) instead of midnight
		startTime = marketOpen
		hoursBack = time.Since(startTime).Hours()
	}

	// Log for debugging time range issues
	log.Printf("[handleAccumulationSummary] now=%s, startTime=%s, hoursBack=%.2f, marketOpenHour=%d",
		now.Format("2006-01-02 15:04:05"), startTime.Format("2006-01-02 15:04:05"),
		hoursBack, marketOpenHour)

	// Get accumulation/distribution summary (now returns 2 separate lists)
	accumulation, distribution, err := s.repo.GetAccumulationDistributionSummary(startTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accumulation":       accumulation,
		"distribution":       distribution,
		"accumulation_count": len(accumulation),
		"distribution_count": len(distribution),
		"hours_back":         hoursBack,
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

	var regime *database.MarketRegime
	var err error

	if symbol == "IHSG" || symbol == "COMPOSITE" {
		regime, err = s.repo.GetAggregateMarketRegime()
	} else {
		regime, err = s.repo.GetLatestRegime(symbol)
	}

	if err != nil {
		log.Printf("Error getting market regime for %s: %v", symbol, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Safety check if regime is still nil (should be handled by repo now, but good practice)
	if regime == nil {
		regime = &database.MarketRegime{
			StockSymbol: symbol,
			DetectedAt:  time.Now(),
			Regime:      "NEUTRAL",
			Confidence:  0.0,
		}
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

	// If no symbol specified, get patterns for all symbols
	var patterns []database.DetectedPattern
	var err error
	if symbol != "" {
		patterns, err = s.repo.GetRecentPatterns(symbol, since)
	} else {
		patterns, err = s.repo.GetAllRecentPatterns(since)
	}

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

// Phase 3 Enhancement Handlers

// handleGetStockCorrelations returns correlations for a symbol
func (s *Server) handleGetStockCorrelations(w http.ResponseWriter, r *http.Request) {
	// Symbol is optional for global correlations
	symbol := r.URL.Query().Get("symbol")

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

// handleMLDataStats returns statistics about ML training data availability
func (s *Server) handleMLDataStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.repo.GetMLTrainingDataStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleExportMLData returns a CSV of training data
func (s *Server) handleExportMLData(w http.ResponseWriter, r *http.Request) {
	data, err := s.repo.GetMLTrainingData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment;filename=training_data_%d.csv", time.Now().Unix()))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header
	writer.Write([]string{"generated_at", "symbol", "strategy", "confidence", "outcome", "profit_pct", "feature_vector"})

	// Rows
	for _, row := range data {
		writer.Write([]string{
			row.GeneratedAt.Format(time.RFC3339),
			row.StockSymbol,
			row.Strategy,
			fmt.Sprintf("%.2f", row.Confidence),
			row.OutcomeResult,
			fmt.Sprintf("%.2f", row.ProfitLossPct),
			row.AnalysisData,
		})
	}
}

// ============================================================================
// Signal Effectiveness Analysis Handlers
// ============================================================================

// handleGetStrategyEffectiveness returns multi-dimensional strategy effectiveness
// by market regime for adaptive strategy selection
func (s *Server) handleGetStrategyEffectiveness(w http.ResponseWriter, r *http.Request) {
	daysBack := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			daysBack = parsed
		}
	}

	effectiveness, err := s.repo.GetStrategyEffectivenessByRegime(daysBack)
	if err != nil {
		log.Printf("❌ Failed to get strategy effectiveness: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"effectiveness": effectiveness,
		"days_back":     daysBack,
		"count":         len(effectiveness),
	})
}

// handleGetOptimalThresholds returns optimal confidence thresholds per strategy
func (s *Server) handleGetOptimalThresholds(w http.ResponseWriter, r *http.Request) {
	daysBack := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			daysBack = parsed
		}
	}

	thresholds, err := s.repo.GetOptimalConfidenceThresholds(daysBack)
	if err != nil {
		log.Printf("❌ Failed to get optimal thresholds: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"thresholds": thresholds,
		"days_back":  daysBack,
	})
}

// handleGetTimeEffectiveness returns signal effectiveness by hour of day
func (s *Server) handleGetTimeEffectiveness(w http.ResponseWriter, r *http.Request) {
	daysBack := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			daysBack = parsed
		}
	}

	effectiveness, err := s.repo.GetTimeOfDayEffectiveness(daysBack)
	if err != nil {
		log.Printf("❌ Failed to get time effectiveness: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"time_effectiveness": effectiveness,
		"days_back":          daysBack,
		"count":              len(effectiveness),
	})
}

// handleGetExpectedValues returns expected value calculations for strategies
func (s *Server) handleGetExpectedValues(w http.ResponseWriter, r *http.Request) {
	daysBack := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			daysBack = parsed
		}
	}

	evs, err := s.repo.GetSignalExpectedValues(daysBack)
	if err != nil {
		log.Printf("❌ Failed to get expected values: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"expected_values": evs,
		"days_back":       daysBack,
	})
}

// handleCustomPromptStream streams AI analysis based on custom user prompt with database context
func (s *Server) handleCustomPromptStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse JSON request body
	var reqBody struct {
		Prompt      string   `json:"prompt"`
		Symbols     []string `json:"symbols"`      // optional: specific symbols to analyze
		HoursBack   int      `json:"hours_back"`   // hours of data to include
		IncludeData string   `json:"include_data"` // comma-separated: alerts,regimes,patterns,signals
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if reqBody.Prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	// Default values
	if reqBody.HoursBack <= 0 {
		reqBody.HoursBack = 24
	}
	if reqBody.IncludeData == "" {
		reqBody.IncludeData = "alerts,regimes"
	}

	// Set SSE headers
	flusher, ok := setupSSE(w)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "Streaming not supported", nil)
		return
	}

	// Build context data based on user selection
	var contextBuilder strings.Builder
	contextBuilder.WriteString("KONTEKS DATA DARI DATABASE:\n\n")

	includeTypes := strings.Split(reqBody.IncludeData, ",")

	for _, dataType := range includeTypes {
		dataType = strings.TrimSpace(dataType)

		switch dataType {
		case "alerts":
			// Get whale alerts
			var alerts []database.WhaleAlert

			if len(reqBody.Symbols) > 0 {
				// Get alerts for specific symbols
				for _, symbol := range reqBody.Symbols {
					symbolAlerts, e := s.repo.GetRecentAlertsBySymbol(symbol, 50)
					if e == nil {
						alerts = append(alerts, symbolAlerts...)
					}
				}
			} else {
				// Get recent alerts from accumulation patterns (top active stocks)
				patterns, e := s.repo.GetAccumulationPattern(reqBody.HoursBack, 2)
				if e == nil && len(patterns) > 0 {
					// Get alerts for top 10 most active symbols
					limit := 10
					if len(patterns) < limit {
						limit = len(patterns)
					}
					for i := 0; i < limit; i++ {
						symbolAlerts, ae := s.repo.GetRecentAlertsBySymbol(patterns[i].StockSymbol, 10)
						if ae == nil {
							alerts = append(alerts, symbolAlerts...)
						}
					}
				}
			}

			if len(alerts) > 0 {
				contextBuilder.WriteString("=== WHALE ALERTS (Transaksi Besar) ===\n")
				for i, a := range alerts {
					if i >= 20 { // Limit to 20 alerts
						break
					}
					zScore := safeFloat64(a.ZScore, 0.0)
					timeSince := time.Since(a.DetectedAt).Minutes()
					contextBuilder.WriteString(fmt.Sprintf(
						"- %s (%s): Rp %.1fM, Z-Score: %.2f, %.0f menit lalu\n",
						a.StockSymbol, a.Action, a.TriggerValue/millionDivisor, zScore, timeSince,
					))
				}
				contextBuilder.WriteString("\n")
			}

		case "regimes":
			// Get market regimes
			if len(reqBody.Symbols) > 0 {
				contextBuilder.WriteString("=== MARKET REGIME (Kondisi Pasar) ===\n")
				for _, symbol := range reqBody.Symbols {
					if regime, err := s.repo.GetLatestRegime(symbol); err == nil && regime != nil {
						volatility := safeFloat64(regime.Volatility, 0.0)
						contextBuilder.WriteString(fmt.Sprintf(
							"- %s: %s (Confidence: %.0f%%, Volatility: %.2f%%)\n",
							symbol, regime.Regime, regime.Confidence*100, volatility*100,
						))
					}
				}
				contextBuilder.WriteString("\n")
			}

		case "patterns":
			// Get accumulation patterns
			patterns, err := s.repo.GetAccumulationPattern(reqBody.HoursBack, 3)
			if err == nil && len(patterns) > 0 {
				contextBuilder.WriteString("=== POLA AKUMULASI/DISTRIBUSI ===\n")
				for i, p := range patterns {
					if i >= 10 {
						break
					}
					avgPrice := 0.0
					if p.TotalVolumeLots > 0 {
						avgPrice = p.TotalValue / (p.TotalVolumeLots * 100)
					}
					contextBuilder.WriteString(fmt.Sprintf(
						"- %s (%s): %d alerts, Total: Rp %.2fM, Avg Price: %.0f, Z-Score: %.2f\n",
						p.StockSymbol, p.Action, p.AlertCount,
						p.TotalValue/millionDivisor, avgPrice, p.AvgZScore,
					))
				}
				contextBuilder.WriteString("\n")
			}

		case "signals":
			// Get recent signals (lookback 24 hours * 60 minutes)
			signals, err := s.repo.GetRecentSignalsWithOutcomes(reqBody.HoursBack*60, 0.0, "")
			if err == nil && len(signals) > 0 {
				contextBuilder.WriteString("=== TRADING SIGNALS (AI) ===\n")
				for i, sig := range signals {
					if i >= 15 {
						break
					}
					result := "OPEN"
					if sig.Outcome != "" {
						result = sig.Outcome
					}
					contextBuilder.WriteString(fmt.Sprintf(
						"- %s (%s): %s, Price: %.0f, Confidence: %.0f%%, Result: %s\n",
						sig.StockSymbol, sig.Strategy, sig.Decision,
						sig.Price, sig.Confidence*100, result,
					))
				}
				contextBuilder.WriteString("\n")
			}
		}
	}

	contextBuilder.WriteString("=== PERTANYAAN USER ===\n")
	contextBuilder.WriteString(reqBody.Prompt)
	contextBuilder.WriteString("\n\nJawab berdasarkan DATA di atas. Jangan membuat asumsi atau data yang tidak ada. Fokus pada insight yang actionable.")

	fullPrompt := contextBuilder.String()

	// Stream LLM response
	err := s.llmClient.AnalyzeStream(r.Context(), fullPrompt, func(chunk string) error {
		// Properly format multi-line chunks for SSE
		lines := strings.Split(chunk, "\n")
		for i, line := range lines {
			if i < len(lines)-1 {
				fmt.Fprintf(w, "data: %s\n", line)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
		}
		flusher.Flush()
		return nil
	})

	if err != nil {
		log.Printf("LLM streaming failed: %v", err)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send completion event
	fmt.Fprintf(w, "event: done\ndata: Stream completed\n\n")
	flusher.Flush()
}

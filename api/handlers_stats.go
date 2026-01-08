package api

import (
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

// handleGetStatisticalBaselines returns latest statistical baselines for a symbol
func (s *Server) handleGetStatisticalBaselines(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "Symbol is required", http.StatusBadRequest)
		return
	}

	var baseline *database.StatisticalBaseline
	var err error

	if symbol == "IHSG" || symbol == "COMPOSITE" {
		baseline, err = s.repo.GetAggregateBaseline()
	} else {
		baseline, err = s.repo.GetLatestBaseline(symbol)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if baseline == nil {
		http.Error(w, "Baseline not found", http.StatusNotFound)
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

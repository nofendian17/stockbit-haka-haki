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

package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"stockbit-haka-haki/database"
	"stockbit-haka-haki/llm"
)

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

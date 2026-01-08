package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"stockbit-haka-haki/database"
	"stockbit-haka-haki/llm"
)

// safeFloat64 safely dereferences a float64 pointer, returning defaultValue if nil
func safeFloat64(ptr *float64, defaultValue float64) float64 {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
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
						a.StockSymbol, a.Action, a.TriggerValue/1000000.0, zScore, timeSince,
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
						p.TotalValue/1000000.0, avgPrice, p.AvgZScore,
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

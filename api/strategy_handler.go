package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

	// Get strategy signals
	signals, err := s.repo.GetStrategySignals(lookbackMinutes, minConfidence, strategyFilter)
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

// handleAccumulationSummary returns separate top 20 accumulation and distribution lists
func (s *Server) handleAccumulationSummary(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()

	hoursBack := 24 // default 24 hours (1 day)
	if h := query.Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil {
			hoursBack = parsed
		}
	}

	// Get accumulation/distribution summary (now returns 2 separate lists)
	accumulation, distribution, err := s.repo.GetAccumulationDistributionSummary(hoursBack)
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

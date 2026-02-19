package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

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

// handleGetStrategyEffectiveness returns strategy effectiveness analysis
func (s *Server) handleGetStrategyEffectiveness(w http.ResponseWriter, r *http.Request) {
	daysBack := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			daysBack = parsed
		}
	}

	effectiveness, err := s.repo.GetStrategyEffectiveness(daysBack)
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

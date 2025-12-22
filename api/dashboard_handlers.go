package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Dashboard API Handlers

// handleDashboardLiveTrades returns live running trades (RG market only)
func (s *Server) handleDashboardLiveTrades(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	stockSymbol := query.Get("symbol")
	actionFilter := query.Get("action")

	limitStr := query.Get("limit")
	limit := 100 // default
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			if val > 500 {
				val = 500 // Cap at 500
			}
			limit = val
		}
	}

	trades, err := s.repo.GetRecentTradesRG(limit, stockSymbol, actionFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"data":  trades,
		"count": len(trades),
		"limit": limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDashboardPressureGauge returns buy/sell pressure metrics
func (s *Server) handleDashboardPressureGauge(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	stockSymbol := query.Get("symbol")

	windowMinutesStr := query.Get("window")
	windowMinutes := 5 // default 5 minutes
	if windowMinutesStr != "" {
		if val, err := strconv.Atoi(windowMinutesStr); err == nil && val > 0 && val <= 60 {
			windowMinutes = val
		}
	}

	var pressure interface{}
	var err error

	if stockSymbol != "" {
		// Single stock pressure
		pressure, err = s.repo.GetBuySellPressure(stockSymbol, windowMinutes)
	} else {
		// Top N active stocks with their pressure (per-symbol)
		limitStr := query.Get("limit")
		limit := 10 // default top 10 symbols
		if limitStr != "" {
			if val, err := strconv.Atoi(limitStr); err == nil && val > 0 && val <= 50 {
				limit = val
			}
		}
		pressure, err = s.repo.GetTopSymbolsByActivity(windowMinutes, limit)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pressure)
}

// handleDashboardWhaleDetector returns large transactions
func (s *Server) handleDashboardWhaleDetector(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	thresholdStr := query.Get("threshold")
	threshold := 500000000.0 // default 500 million IDR
	if thresholdStr != "" {
		if val, err := strconv.ParseFloat(thresholdStr, 64); err == nil && val > 0 {
			threshold = val
		}
	}

	limitStr := query.Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			if val > 200 {
				val = 200
			}
			limit = val
		}
	}

	hoursBackStr := query.Get("hours")
	hoursBack := 24 // default 24 hours
	if hoursBackStr != "" {
		if val, err := strconv.Atoi(hoursBackStr); err == nil && val > 0 {
			hoursBack = val
		}
	}

	trades, err := s.repo.GetLargeTransactions(threshold, limit, hoursBack)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"data":      trades,
		"count":     len(trades),
		"threshold": threshold,
		"hours":     hoursBack,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDashboardVolumeSpikes returns detected volume spikes
func (s *Server) handleDashboardVolumeSpikes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	minSpikeStr := query.Get("min_spike")
	minSpike := 500.0 // default 500%
	if minSpikeStr != "" {
		if val, err := strconv.ParseFloat(minSpikeStr, 64); err == nil && val > 0 {
			minSpike = val
		}
	}

	hoursBackStr := query.Get("hours")
	hoursBack := 4 // default 4 hours for baseline
	if hoursBackStr != "" {
		if val, err := strconv.Atoi(hoursBackStr); err == nil && val > 0 {
			hoursBack = val
		}
	}

	spikes, err := s.repo.GetVolumeSpikes(minSpike, hoursBack)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"data":          spikes,
		"count":         len(spikes),
		"min_spike_pct": minSpike,
		"hours_back":    hoursBack,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDashboardZScoreRanking returns stocks ranked by Z-Score
func (s *Server) handleDashboardZScoreRanking(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	minZScoreStr := query.Get("min_z")
	minZScore := 3.0 // default Z > 3
	if minZScoreStr != "" {
		if val, err := strconv.ParseFloat(minZScoreStr, 64); err == nil && val > 0 {
			minZScore = val
		}
	}

	hoursBackStr := query.Get("hours")
	hoursBack := 48 // default 48 hours
	if hoursBackStr != "" {
		if val, err := strconv.Atoi(hoursBackStr); err == nil && val > 0 {
			hoursBack = val
		}
	}

	limitStr := query.Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			if val > 100 {
				val = 100
			}
			limit = val
		}
	}

	rankings, err := s.repo.GetZScoreRanking(minZScore, hoursBack, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"data":       rankings,
		"count":      len(rankings),
		"min_z":      minZScore,
		"hours_back": hoursBack,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDashboardPowerCandles returns power candles
func (s *Server) handleDashboardPowerCandles(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	minChangeStr := query.Get("min_change")
	minChange := 2.0 // default 2%
	if minChangeStr != "" {
		if val, err := strconv.ParseFloat(minChangeStr, 64); err == nil && val > 0 {
			minChange = val
		}
	}

	minVolumeStr := query.Get("min_volume")
	minVolume := 500.0 // default 500 lots
	if minVolumeStr != "" {
		if val, err := strconv.ParseFloat(minVolumeStr, 64); err == nil && val > 0 {
			minVolume = val
		}
	}

	hoursBackStr := query.Get("hours")
	hoursBack := 4 // default 4 hours
	if hoursBackStr != "" {
		if val, err := strconv.Atoi(hoursBackStr); err == nil && val > 0 {
			hoursBack = val
		}
	}

	candles, err := s.repo.GetPowerCandles(minChange, minVolume, hoursBack)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"data":           candles,
		"count":          len(candles),
		"min_change_pct": minChange,
		"min_volume":     minVolume,
		"hours_back":     hoursBack,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDashboardCandles returns candle data for a symbol
func (s *Server) handleDashboardCandles(w http.ResponseWriter, r *http.Request) {
	symbol := r.PathValue("symbol")
	if symbol == "" {
		http.Error(w, "symbol is required", http.StatusBadRequest)
		return
	}

	query := r.URL.Query()

	// Parse time range
	var startTime, endTime time.Time
	if startStr := query.Get("start"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	} else {
		// Default: last 4 hours
		startTime = time.Now().Add(-4 * time.Hour)
	}

	if endStr := query.Get("end"); endStr != "" {
		endTime, _ = time.Parse(time.RFC3339, endStr)
	} else {
		endTime = time.Now()
	}

	data, err := s.repo.GetCandlesWithWhaleAlerts(symbol, startTime, endTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleDashboardVWAP returns VWAP data for a symbol
func (s *Server) handleDashboardVWAP(w http.ResponseWriter, r *http.Request) {
	symbol := r.PathValue("symbol")
	if symbol == "" {
		http.Error(w, "symbol is required", http.StatusBadRequest)
		return
	}

	query := r.URL.Query()

	// Parse start time
	var startTime time.Time
	if startStr := query.Get("start"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	} else {
		// Default: start of trading day (09:00 WIB)
		now := time.Now()
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	}

	intervalStr := query.Get("interval")
	interval := 1 // default 1 minute
	if intervalStr != "" {
		if val, err := strconv.Atoi(intervalStr); err == nil && val > 0 {
			interval = val
		}
	}

	vwapHistory, err := s.repo.GetVWAPHistory(symbol, startTime, interval)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Also get current VWAP
	currentVWAP, err := s.repo.CalculateVWAP(symbol, startTime)
	if err != nil {
		log.Printf("Failed to get current VWAP: %v", err)
	}

	response := map[string]interface{}{
		"symbol":       symbol,
		"start_time":   startTime,
		"interval":     interval,
		"history":      vwapHistory,
		"current_vwap": currentVWAP,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDashboardTradesStream - DEPRECATED
// Use GET /api/dashboard/live-trades with polling instead
// or use the main SSE endpoint GET /api/events for whale alerts
func (s *Server) handleDashboardTradesStream(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Use GET /api/dashboard/live-trades with polling, or GET /api/events for real-time whale alerts", http.StatusGone)
}

// handleDashboardMetricsStream streams live metrics (pressure gauge) via SSE
func (s *Server) handleDashboardMetricsStream(w http.ResponseWriter, r *http.Request) {
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

	query := r.URL.Query()
	stockSymbol := query.Get("symbol")
	windowMinutes := 5

	if windowStr := query.Get("window"); windowStr != "" {
		if val, err := strconv.Atoi(windowStr); err == nil && val > 0 && val <= 60 {
			windowMinutes = val
		}
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var pressure interface{}
			var err error

			if stockSymbol != "" {
				pressure, err = s.repo.GetBuySellPressure(stockSymbol, windowMinutes)
			} else {
				pressure, err = s.repo.GetAllPressureGauges(windowMinutes, 100.0)
			}

			if err != nil {
				log.Printf("Failed to get pressure: %v", err)
				continue
			}

			data, err := json.Marshal(pressure)
			if err != nil {
				log.Printf("Failed to marshal pressure: %v", err)
				continue
			}

			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

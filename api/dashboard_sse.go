package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// handleDashboardSSE streams all dashboard data via Server-Sent Events
func (s *Server) handleDashboardSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	log.Printf("[SSE] New dashboard connection from %s", r.RemoteAddr)

	// Create tickers for different data types
	cockpitTicker := time.NewTicker(5 * time.Second)   // Live trades, pressure, whales
	analysisTicker := time.NewTicker(10 * time.Second) // Volume spikes, z-score, candles
	defer cockpitTicker.Stop()
	defer analysisTicker.Stop()

	// Send initial data immediately
	s.sendCockpitData(w, flusher)
	s.sendAnalysisData(w, flusher)

	// Stream updates
	for {
		select {
		case <-cockpitTicker.C:
			s.sendCockpitData(w, flusher)

		case <-analysisTicker.C:
			s.sendAnalysisData(w, flusher)

		case <-r.Context().Done():
			log.Printf("[SSE] Client disconnected: %s", r.RemoteAddr)
			return
		}
	}
}

func (s *Server) sendCockpitData(w http.ResponseWriter, flusher http.Flusher) {
	// Live Trades
	trades, err := s.repo.GetRecentTradesRG(20, "", "")
	if err == nil {
		data := map[string]interface{}{
			"data": trades,
		}
		s.sendSSEEvent(w, "live_trades", data)
	}

	// Pressure Gauge - Top 10 Active Symbols
	pressures, err := s.repo.GetTopSymbolsByActivity(5, 10)
	if err == nil {
		s.sendSSEEvent(w, "pressure_gauge", pressures)
	}

	// Whale Alerts - use GetHistoricalWhales
	now := time.Now()
	startTime := now.Add(-24 * time.Hour)
	whales, err := s.repo.GetHistoricalWhales("", startTime, now, "", 20, 0)
	if err == nil {
		data := map[string]interface{}{
			"data": whales,
		}
		s.sendSSEEvent(w, "whale_alerts", data)
	}

	flusher.Flush()
}

func (s *Server) sendAnalysisData(w http.ResponseWriter, flusher http.Flusher) {
	// Volume Spikes
	spikes, err := s.repo.GetVolumeSpikes(100, 4)
	if err == nil {
		data := map[string]interface{}{
			"data": spikes,
		}
		s.sendSSEEvent(w, "volume_spikes", data)
	}

	// Z-Score Ranking
	zscores, err := s.repo.GetZScoreRanking(3.0, 24, 20)
	if err == nil {
		data := map[string]interface{}{
			"data": zscores,
		}
		s.sendSSEEvent(w, "zscore_ranking", data)
	}

	// Power Candles
	candles, err := s.repo.GetPowerCandles(2.0, 500, 4)
	if err == nil {
		data := map[string]interface{}{
			"data": candles,
		}
		s.sendSSEEvent(w, "power_candles", data)
	}

	flusher.Flush()
}

func (s *Server) sendSSEEvent(w http.ResponseWriter, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("[SSE] Error marshaling %s: %v", event, err)
		return
	}

	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
}

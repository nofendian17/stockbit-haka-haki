package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"stockbit-haka-haki/database"
	"stockbit-haka-haki/llm"
	"stockbit-haka-haki/notifications"
	"stockbit-haka-haki/realtime"
)

// Server handles HTTP API requests
type Server struct {
	repo       *database.TradeRepository
	webhookMq  *notifications.WebhookManager
	broker     *realtime.Broker
	llmClient  *llm.Client
	llmEnabled bool
}

// NewServer creates a new API server instance
func NewServer(repo *database.TradeRepository, webhookMq *notifications.WebhookManager, broker *realtime.Broker, llmClient *llm.Client, llmEnabled bool) *Server {
	return &Server{
		repo:       repo,
		webhookMq:  webhookMq,
		broker:     broker,
		llmClient:  llmClient,
		llmEnabled: llmEnabled,
	}
}

// Start starts the HTTP server on the specified port
func (s *Server) Start(port int) error {
	mux := http.NewServeMux()

	// Register routes
	mux.Handle("GET /api/events", s.broker) // SSE Endpoint
	mux.HandleFunc("GET /api/whales", s.handleGetWhales)
	mux.HandleFunc("GET /api/whales/stats", s.handleGetWhaleStats)
	// Webhook Management Routes
	mux.HandleFunc("GET /api/config/webhooks", s.handleGetWebhooks)
	mux.HandleFunc("POST /api/config/webhooks", s.handleCreateWebhook)
	mux.HandleFunc("PUT /api/config/webhooks/{id}", s.handleUpdateWebhook)
	mux.HandleFunc("DELETE /api/config/webhooks/{id}", s.handleDeleteWebhook)

	// Pattern Analysis Routes (LLM)
	mux.HandleFunc("GET /api/patterns/accumulation", s.handleAccumulationPattern)
	mux.HandleFunc("GET /api/patterns/anomalies", s.handleExtremeAnomalies)
	mux.HandleFunc("GET /api/patterns/timing", s.handleTimeBasedStats)

	// Pattern Analysis Streaming Routes (LLM SSE)
	mux.HandleFunc("GET /api/patterns/accumulation/stream", s.handleAccumulationPatternStream)
	mux.HandleFunc("GET /api/patterns/anomalies/stream", s.handleExtremeAnomaliesStream)
	mux.HandleFunc("GET /api/patterns/timing/stream", s.handleTimeBasedStatsStream)
	mux.HandleFunc("GET /api/patterns/symbol/stream", s.handleSymbolAnalysisStream)

	// Trading Strategy Routes
	mux.HandleFunc("GET /api/strategies/signals", s.handleGetStrategySignals)
	mux.HandleFunc("GET /api/strategies/signals/stream", s.handleStrategySignalsStream)

	// Phase 1 Enhancement Routes
	mux.HandleFunc("GET /api/signals/history", s.handleGetSignalHistory)
	mux.HandleFunc("GET /api/signals/performance", s.handleGetSignalPerformance)
	mux.HandleFunc("GET /api/signals/{id}/outcome", s.handleGetSignalOutcome)
	mux.HandleFunc("GET /api/whales/{id}/followup", s.handleGetWhaleFollowup)
	mux.HandleFunc("GET /api/orderflow", s.handleGetOrderFlow)

	// Phase 2 Enhancement Routes
	mux.HandleFunc("GET /api/baselines", s.handleGetStatisticalBaselines)
	mux.HandleFunc("GET /api/regimes", s.handleGetMarketRegimes)
	mux.HandleFunc("GET /api/patterns", s.handleGetDetectedPatterns)
	mux.HandleFunc("GET /api/candles", s.handleGetCandles)

	// Phase 3 Enhancement Routes
	mux.HandleFunc("GET /api/analytics/correlations", s.handleGetStockCorrelations)
	mux.HandleFunc("GET /api/analytics/performance/daily", s.handleGetDailyPerformance)

	// Accumulation/Distribution Summary Route
	mux.HandleFunc("GET /api/accumulation-summary", s.handleAccumulationSummary)

	mux.HandleFunc("GET /health", s.handleHealth)

	// Serve Static Files (Public UI)
	fs := http.FileServer(http.Dir("./public"))
	mux.Handle("GET /", fs)

	// Add middleware
	handler := s.corsMiddleware(s.loggingMiddleware(mux))

	serverAddr := fmt.Sprintf("0.0.0.0:%d", port)
	log.Printf("🚀 API Server starting on %s", serverAddr)
	return http.ListenAndServe(serverAddr, handler)
}

// Middleware
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// Handlers

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGetWhales(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()
	symbol := query.Get("symbol")
	alertType := query.Get("type")
	action := query.Get("action") // NEW: Filter for BUY/SELL
	board := query.Get("board")

	limitStr := query.Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			if val > 200 {
				val = 200 // Cap at 200
			}
			limit = val
		}
	}

	offsetStr := query.Get("offset")
	offset := 0
	if offsetStr != "" {
		if val, err := strconv.Atoi(offsetStr); err == nil && val >= 0 {
			offset = val
		}
	}

	// Parse min_amount filter
	minAmount := 0.0
	if minAmountStr := query.Get("min_amount"); minAmountStr != "" {
		if val, err := strconv.ParseFloat(minAmountStr, 64); err == nil && val >= 0 {
			minAmount = val
		}
	}

	// Time range parsing (RFC3339)
	var startTime, endTime time.Time
	if startStr := query.Get("start"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	}
	if endStr := query.Get("end"); endStr != "" {
		endTime, _ = time.Parse(time.RFC3339, endStr)
	}

	whales, err := s.repo.GetHistoricalWhales(symbol, startTime, endTime, alertType, action, board, minAmount, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get total count for pagination metadata
	totalCount, err := s.repo.GetWhaleCount(symbol, startTime, endTime, alertType, action, board, minAmount)
	if err != nil {
		// If count fails, still return data but without total
		totalCount = 0
	}

	// Return response with pagination metadata
	response := map[string]interface{}{
		"data":     whales,
		"total":    totalCount,
		"limit":    limit,
		"offset":   offset,
		"has_more": int64(offset+len(whales)) < totalCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleGetWhaleStats(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()
	symbol := query.Get("symbol")

	// Time range parsing
	// Time range parsing
	var startTime, endTime time.Time

	// Default to Today 00:00:00 WIB (Asia/Jakarta) if no start time is provided
	// Use FixedZone to avoid dependency on tzdata being installed
	loc := time.FixedZone("WIB", 7*60*60)
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	if startStr := query.Get("start"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	} else {
		startTime = todayStart
	}

	if endStr := query.Get("end"); endStr != "" {
		endTime, _ = time.Parse(time.RFC3339, endStr)
	}

	stats, err := s.repo.GetWhaleStats(symbol, startTime, endTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Configuration Handlers (Webhooks Only)

func (s *Server) handleGetWebhooks(w http.ResponseWriter, r *http.Request) {
	webhooks, err := s.repo.GetWebhooks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhooks)
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var webhook database.WhaleWebhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Reset ID to let DB assign it
	webhook.ID = 0

	if err := s.repo.SaveWebhook(&webhook); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh webhook manager cache
	if s.webhookMq != nil {
		s.webhookMq.RefreshCache()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(webhook)
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var webhook database.WhaleWebhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	webhook.ID = id // Ensure ID matches path
	if err := s.repo.SaveWebhook(&webhook); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh webhook manager cache
	if s.webhookMq != nil {
		s.webhookMq.RefreshCache()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhook)
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.repo.DeleteWebhook(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh webhook manager cache
	if s.webhookMq != nil {
		s.webhookMq.RefreshCache()
	}

	w.WriteHeader(http.StatusNoContent)
}

// Pattern Analysis Handlers

func (s *Server) handleAccumulationPattern(w http.ResponseWriter, r *http.Request) {
	hoursBack := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil {
			hoursBack = parsed
		}
	}

	minAlerts := 3
	if m := r.URL.Query().Get("min_alerts"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil {
			minAlerts = parsed
		}
	}

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
		prompt := llm.FormatAccumulationPrompt(patterns)
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

func (s *Server) handleExtremeAnomalies(w http.ResponseWriter, r *http.Request) {
	minZScore := 5.0
	if z := r.URL.Query().Get("min_z"); z != "" {
		if parsed, err := strconv.ParseFloat(z, 64); err == nil {
			minZScore = parsed
		}
	}

	hoursBack := 48
	if h := r.URL.Query().Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil {
			hoursBack = parsed
		}
	}

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
		prompt := llm.FormatAnomalyPrompt(anomalies)
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

func (s *Server) handleTimeBasedStats(w http.ResponseWriter, r *http.Request) {
	daysBack := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil {
			daysBack = parsed
		}
	}

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

// Pattern Analysis Streaming Handlers (SSE)

func (s *Server) handleAccumulationPatternStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query params
	hoursBack := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil {
			hoursBack = parsed
		}
	}

	minAlerts := 3
	if m := r.URL.Query().Get("min_alerts"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil {
			minAlerts = parsed
		}
	}

	// Get patterns data
	patterns, err := s.repo.GetAccumulationPattern(hoursBack, minAlerts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(patterns) == 0 {
		http.Error(w, "No accumulation patterns found", http.StatusNotFound)
		return
	}

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

	// Generate prompt
	prompt := llm.FormatAccumulationPrompt(patterns)

	// Stream LLM response
	err = s.llmClient.AnalyzeStream(r.Context(), prompt, func(chunk string) error {
		// Write SSE format: "data: <chunk>\n\n"
		fmt.Fprintf(w, "data: %s\n\n", chunk)
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

func (s *Server) handleExtremeAnomaliesStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query params
	minZScore := 5.0
	if z := r.URL.Query().Get("min_z"); z != "" {
		if parsed, err := strconv.ParseFloat(z, 64); err == nil {
			minZScore = parsed
		}
	}

	hoursBack := 48
	if h := r.URL.Query().Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil {
			hoursBack = parsed
		}
	}

	// Get anomalies data
	anomalies, err := s.repo.GetExtremeAnomalies(minZScore, hoursBack)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(anomalies) == 0 {
		http.Error(w, "No extreme anomalies found", http.StatusNotFound)
		return
	}

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

	// Generate prompt
	prompt := llm.FormatAnomalyPrompt(anomalies)

	// Stream LLM response
	err = s.llmClient.AnalyzeStream(r.Context(), prompt, func(chunk string) error {
		// Write SSE format: "data: <chunk>\n\n"
		fmt.Fprintf(w, "data: %s\n\n", chunk)
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

func (s *Server) handleTimeBasedStatsStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query params
	daysBack := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil {
			daysBack = parsed
		}
	}

	// Get time-based stats
	stats, err := s.repo.GetTimeBasedStats(daysBack)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(stats) == 0 {
		http.Error(w, "No timing stats found", http.StatusNotFound)
		return
	}

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

	// Generate prompt
	prompt := llm.FormatTimingPrompt(stats)

	// Stream LLM response
	err = s.llmClient.AnalyzeStream(r.Context(), prompt, func(chunk string) error {
		// Write SSE format: "data: <chunk>\n\n"
		fmt.Fprintf(w, "data: %s\n\n", chunk)
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

package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"stockbit-haka-haki/database"
	"stockbit-haka-haki/llm"
	"stockbit-haka-haki/notifications"
	"stockbit-haka-haki/realtime"
)

// Server handles HTTP API requests
type Server struct {
	repo          *database.TradeRepository
	webhookMq     *notifications.WebhookManager
	broker        *realtime.Broker
	llmClient     *llm.Client
	llmEnabled    bool
	signalTracker SignalTrackerInterface // Use case for signal tracking
}

// SignalTrackerInterface defines the interface for signal tracking operations
type SignalTrackerInterface interface {
	GetOpenPositions(symbol, strategy string, limit int) ([]database.SignalOutcome, error)
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

// SetSignalTracker sets the signal tracker use case
func (s *Server) SetSignalTracker(tracker SignalTrackerInterface) {
	s.signalTracker = tracker
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
	mux.HandleFunc("GET /api/whales/followups", s.handleGetWhaleFollowups)
	mux.HandleFunc("GET /api/orderflow", s.handleGetOrderFlow)

	// Phase 2 Enhancement Routes
	mux.HandleFunc("GET /api/baselines", s.handleGetStatisticalBaselines)
	mux.HandleFunc("GET /api/regimes", s.handleGetMarketRegimes)
	mux.HandleFunc("GET /api/patterns", s.handleGetDetectedPatterns)
	mux.HandleFunc("GET /api/candles", s.handleGetCandles)

	// Phase 3 Enhancement Routes
	mux.HandleFunc("GET /api/analytics/export/ml-data", s.handleExportMLData)
	mux.HandleFunc("GET /api/analytics/correlations", s.handleGetStockCorrelations)
	mux.HandleFunc("GET /api/analytics/performance/daily", s.handleGetDailyPerformance)
	mux.HandleFunc("GET /api/positions/open", s.handleGetOpenPositions)
	mux.HandleFunc("GET /api/positions/history", s.handleGetProfitLossHistory)

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

// Handlers are distributed across multiple files:
// - handlers_market.go: Raw market data (Whales, Candles, OrderFlow)
// - handlers_strategy.go: Trading strategies and signals
// - handlers_analytics.go: AI analysis, regimes, baselines
// - handlers_config.go: System config, webhooks, health check

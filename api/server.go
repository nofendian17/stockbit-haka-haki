package api

import (
	"compress/gzip"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
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
	s.registerMarketRoutes(mux)
	s.registerWebhookRoutes(mux)
	s.registerPatternRoutes(mux)
	s.registerStrategyRoutes(mux)
	s.registerAnalyticsRoutes(mux)

	mux.HandleFunc("GET /health", s.handleHealth)

	// Serve Static Files (Public UI) with Cache Busting for index.html
	fs := http.FileServer(http.Dir("./public"))

	// Cached index.html content (load once on startup)
	indexContent, err := os.ReadFile("./public/index.html")
	if err != nil {
		log.Printf("⚠️ Warning: Could not read public/index.html: %v", err)
	}

	// Inject current timestamp as version
	version := fmt.Sprintf("%d", time.Now().Unix())
	re := regexp.MustCompile(`src="js/app\.js\?v=[^"]*"`)
	modifiedIndex := re.ReplaceAll(indexContent, []byte(fmt.Sprintf(`src="js/app.js?v=%s"`, version)))

	// Custom handler to serve index.html with dynamic version
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html")
			w.Write(modifiedIndex)
			return
		}
		fs.ServeHTTP(w, r)
	})

	// Add middleware (gzip -> cors -> logging)
	handler := s.gzipMiddleware(s.corsMiddleware(s.loggingMiddleware(mux)))

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

// gzipResponseWriter wraps http.ResponseWriter to support gzip compression
type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (g *gzipResponseWriter) Write(data []byte) (int, error) {
	return g.writer.Write(data)
}

// gzipMiddleware compresses API responses using gzip
func (s *Server) gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only compress API responses (not static files)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check if client supports gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip SSE endpoints (streaming)
		if strings.Contains(r.URL.Path, "/stream") || r.URL.Path == "/api/events" {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()

		gzw := &gzipResponseWriter{ResponseWriter: w, writer: gz}
		next.ServeHTTP(gzw, r)
	})
}

// Handlers are distributed across multiple files:
// - handlers_market.go: Raw market data (Whales, Candles, OrderFlow)
// - handlers_strategy.go: Trading strategies and signals
// - handlers_analytics.go: AI analysis, regimes, baselines
// Route registration helpers

func (s *Server) registerMarketRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/events", s.broker) // SSE Endpoint
	mux.HandleFunc("GET /api/whales", s.handleGetWhales)
	mux.HandleFunc("GET /api/whales/stats", s.handleGetWhaleStats)
	mux.HandleFunc("GET /api/whales/{id}/followup", s.handleGetWhaleFollowup)
	mux.HandleFunc("GET /api/whales/followups", s.handleGetWhaleFollowups)
	mux.HandleFunc("GET /api/orderflow", s.handleGetOrderFlow)
	mux.HandleFunc("GET /api/candles", s.handleGetCandles)
}

func (s *Server) registerWebhookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config/webhooks", s.handleGetWebhooks)
	mux.HandleFunc("POST /api/config/webhooks", s.handleCreateWebhook)
	mux.HandleFunc("PUT /api/config/webhooks/{id}", s.handleUpdateWebhook)
	mux.HandleFunc("DELETE /api/config/webhooks/{id}", s.handleDeleteWebhook)
}

func (s *Server) registerPatternRoutes(mux *http.ServeMux) {
	// Standard Endpoints
	mux.HandleFunc("GET /api/patterns/accumulation", s.handleAccumulationPattern)
	mux.HandleFunc("GET /api/patterns/anomalies", s.handleExtremeAnomalies)
	mux.HandleFunc("GET /api/patterns/timing", s.handleTimeBasedStats)
	mux.HandleFunc("GET /api/patterns", s.handleGetDetectedPatterns)
	mux.HandleFunc("GET /api/accumulation-summary", s.handleAccumulationSummary)

	// Streaming Endpoints
	mux.HandleFunc("GET /api/patterns/accumulation/stream", s.handleAccumulationPatternStream)
	mux.HandleFunc("GET /api/patterns/anomalies/stream", s.handleExtremeAnomaliesStream)
	mux.HandleFunc("GET /api/patterns/timing/stream", s.handleTimeBasedStatsStream)
	mux.HandleFunc("GET /api/patterns/symbol/stream", s.handleSymbolAnalysisStream)
	mux.HandleFunc("POST /api/patterns/custom/stream", s.handleCustomPromptStream)
}

func (s *Server) registerStrategyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/strategies/signals", s.handleGetStrategySignals)
	mux.HandleFunc("GET /api/strategies/signals/stream", s.handleStrategySignalsStream)

	// Signal History & Outcomes
	mux.HandleFunc("GET /api/signals/history", s.handleGetSignalHistory)
	mux.HandleFunc("GET /api/signals/performance", s.handleGetSignalPerformance)
	mux.HandleFunc("GET /api/signals/{id}/outcome", s.handleGetSignalOutcome)
	mux.HandleFunc("GET /api/positions/open", s.handleGetOpenPositions)
	mux.HandleFunc("GET /api/positions/history", s.handleGetProfitLossHistory)
}

func (s *Server) registerAnalyticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/baselines", s.handleGetStatisticalBaselines)
	mux.HandleFunc("GET /api/regimes", s.handleGetMarketRegimes)
	mux.HandleFunc("GET /api/analytics/correlations", s.handleGetStockCorrelations)
	mux.HandleFunc("GET /api/analytics/performance/daily", s.handleGetDailyPerformance)

	// ML Data & Stats
	mux.HandleFunc("GET /api/analytics/export/ml-data", s.handleExportMLData)
	mux.HandleFunc("GET /api/analytics/ml-data/stats", s.handleMLDataStats)

	// Effectiveness & Optimization
	mux.HandleFunc("GET /api/analytics/strategy-effectiveness", s.handleGetStrategyEffectiveness)
	mux.HandleFunc("GET /api/analytics/optimal-thresholds", s.handleGetOptimalThresholds)
	mux.HandleFunc("GET /api/analytics/time-effectiveness", s.handleGetTimeEffectiveness)
	mux.HandleFunc("GET /api/analytics/expected-values", s.handleGetExpectedValues)
}

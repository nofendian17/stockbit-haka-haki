package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"stockbit-haka-haki/api"
	"stockbit-haka-haki/auth"
	"stockbit-haka-haki/cache"
	"stockbit-haka-haki/config"
	"stockbit-haka-haki/database"
	"stockbit-haka-haki/handlers"
	"stockbit-haka-haki/llm"
	"stockbit-haka-haki/notifications"
	"stockbit-haka-haki/realtime"
	"stockbit-haka-haki/websocket"
	"sync"
)

// App represents the main application
type App struct {
	config          *config.Config
	authManager     *auth.AuthManager
	wsManager       *websocket.ConnectionManager
	handlerManager  *handlers.HandlerManager
	db              *database.Database
	redis           *cache.RedisClient
	tradeRepo       *database.TradeRepository
	webhookManager  *notifications.WebhookManager
	broker          *realtime.Broker
	signalTracker   *SignalTracker        // Phase 1: Signal outcome tracking
	whaleFollowup   *WhaleFollowupTracker // Phase 1: Whale alert followup
	baselineCalc    *BaselineCalculator   // Phase 2: Statistical baselines
	correlationAnal *CorrelationAnalyzer  // Phase 3: Stock correlations
	perfRefresher   *PerformanceRefresher // Phase 3: Performance view refresher
}

// New creates a new application instance
func New(cfg *config.Config) *App {
	// Initialize Auth Client and Manager
	authClient := auth.NewAuthClient(auth.Credentials{
		PlayerID: cfg.PlayerID,
		Email:    cfg.Username,
		Password: cfg.Password,
	})
	tokenCacheFile := "/app/cache/.token_cache.json"
	authManager := auth.NewAuthManager(authClient, tokenCacheFile)

	// Initialize WebSocket Manager
	wsManager := websocket.NewConnectionManager(cfg.TradingWSURL, authManager)

	return &App{
		config:         cfg,
		authManager:    authManager,
		wsManager:      wsManager,
		handlerManager: handlers.NewHandlerManager(),
		db:             nil, // Will be initialized in Start()
		redis:          nil, // Will be initialized in Start()
		tradeRepo:      nil,
	}
}

// Start starts the application
func (a *App) Start() error {
	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Database Connection
	fmt.Println("🗄️  Connecting to database...")

	dbPort, err := strconv.Atoi(a.config.DatabasePort)
	if err != nil {
		return fmt.Errorf("invalid database port: %w", err)
	}

	db, err := database.Connect(
		a.config.DatabaseHost,
		dbPort,
		a.config.DatabaseName,
		a.config.DatabaseUser,
		a.config.DatabasePassword,
	)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	a.db = db

	// 2. Redis Connection
	fmt.Println("🧠 Connecting to Redis...")
	redisClient := cache.NewRedisClient(
		a.config.RedisHost,
		a.config.RedisPort,
		a.config.RedisPassword,
	)

	if redisClient == nil {
		fmt.Println("⚠️  Redis connection failed. Caching disabled.")
	} else {
		a.redis = redisClient
	}

	// Initialize schema (AutoMigrate + TimescaleDB setup)
	a.tradeRepo = database.NewTradeRepository(a.db)
	if err := a.tradeRepo.InitSchema(); err != nil {
		return fmt.Errorf("schema initialization failed: %w", err)
	}

	// Initialize Webhook Manager (with Redis)
	a.webhookManager = notifications.NewWebhookManager(a.tradeRepo, a.redis)

	// Initialize Realtime Broker
	a.broker = realtime.NewBroker()
	go a.broker.Run()

	// 3. Authentication
	if err := a.authManager.EnsureAuthenticated(); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	log.Println("✅ Stockbit authentication successful")

	// 4. Connect Trading WebSocket
	if err := a.wsManager.Connect(); err != nil {
		return fmt.Errorf("trading WebSocket connection failed: %w", err)
	}
	// 5. Start ping
	a.wsManager.StartPing(25 * time.Second)
	log.Println("✅ Trading WebSocket connected")

	// 6. Setup handlers
	a.setupHandlers()

	// 7. Initialize LLM client if enabled
	var llmClient *llm.Client
	if a.config.LLM.Enabled {
		llmClient = llm.NewClient(a.config.LLM.Endpoint, a.config.LLM.APIKey, a.config.LLM.Model)
		log.Printf("✅ LLM Pattern Recognition ENABLED (Model: %s)", a.config.LLM.Model)
	} else {
		log.Println("ℹ️  LLM Pattern Recognition DISABLED")
	}

	// 8. Start Phase 1 Enhancement Trackers
	log.Println("🚀 Starting Phase 1 enhancement trackers...")

	// Signal Outcome Tracker
	// Signal Outcome Tracker
	a.signalTracker = NewSignalTracker(a.tradeRepo, a.redis, a.config)
	go a.signalTracker.Start()

	// 9. Start API Server (AFTER signal tracker is initialized)
	apiServer := api.NewServer(a.tradeRepo, a.webhookManager, a.broker, llmClient, a.config.LLM.Enabled)

	// Inject signal tracker into API server BEFORE starting the server
	apiServer.SetSignalTracker(a.signalTracker)

	// Start API Server after dependencies are initialized
	go func() {
		if err := apiServer.Start(8080); err != nil {
			log.Printf("⚠️  API Server failed: %v", err)
		}
	}()

	// Whale Followup Tracker
	a.whaleFollowup = NewWhaleFollowupTracker(a.tradeRepo)
	go a.whaleFollowup.Start()

	// 10. Start Phase 2 Enhancement Trackers
	log.Println("🚀 Starting Phase 2 enhancement calculators...")

	// Statistical Baseline Calculator
	a.baselineCalc = NewBaselineCalculator(a.tradeRepo)
	go a.baselineCalc.Start()

	// Pattern Detector removed - 100% loss rate on Range Breakout patterns

	// 11. Start Phase 3 Enhancement Trackers
	log.Println("🚀 Starting Phase 3 advanced analytics...")

	// Correlation Analyzer
	a.correlationAnal = NewCorrelationAnalyzer(a.tradeRepo)
	go a.correlationAnal.Start()

	// Performance Refresher
	a.perfRefresher = NewPerformanceRefresher(a.tradeRepo)
	go a.perfRefresher.Start()

	// Setup WaitGroup for goroutines
	var wg sync.WaitGroup

	// 12. Start background token refresh monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		// On successful refresh, update the websocket connection
		a.authManager.RunTokenMonitor(ctx, func(newToken string) {
			a.wsManager.UpdateToken(newToken)
		})
	}()

	// 13. Start WebSocket health monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.wsManager.RunHealthMonitor(ctx)
	}()

	// 14. Start message processing
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.readAndProcessMessages(ctx)
	}()

	// 15. Wait for interrupt and perform graceful shutdown
	err = a.gracefulShutdown(cancel)
	wg.Wait()
	return err
}

// gracefulShutdown handles graceful shutdown with timeout
func (a *App) gracefulShutdown(cancel context.CancelFunc) error {
	// Setup signal handling
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt signal
	<-interrupt
	fmt.Println("\n🛑 Shutdown signal received, initiating graceful shutdown...")

	// Cancel context to stop all goroutines
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shutdown tasks with timeout
	shutdownComplete := make(chan struct{})
	go func() {
		// Stop trackers
		if a.signalTracker != nil {
			fmt.Println("📊 Stopping signal tracker...")
			a.signalTracker.Stop()
		}
		if a.whaleFollowup != nil {
			fmt.Println("🐋 Stopping whale followup tracker...")
			a.whaleFollowup.Stop()
		}
		if a.baselineCalc != nil {
			fmt.Println("📊 Stopping statistical baseline calculator...")
			a.baselineCalc.Stop()
		}
		// Pattern detector removed
		if a.correlationAnal != nil {
			fmt.Println("🔗 Stopping correlation analyzer...")
			a.correlationAnal.Stop()
		}
		if a.perfRefresher != nil {
			fmt.Println("🔄 Stopping performance refresher...")
			a.perfRefresher.Stop()
		}

		// Close WebSocket connection
		fmt.Println("📡 Closing trading WebSocket connection...")
		if err := a.wsManager.Close(); err != nil {
			log.Printf("Error closing trading WebSocket: %v", err)
		} else {
			fmt.Println("✅ Trading WebSocket closed")
		}

		// Close database connection
		if a.db != nil {
			if err := a.db.Close(); err != nil {
				log.Printf("Error closing database: %v", err)
			} else {
				fmt.Println("✅ Database connection closed")
			}
		}

		// Close Redis connection
		if a.redis != nil {
			if err := a.redis.Close(); err != nil {
				log.Printf("Error closing redis: %v", err)
			} else {
				fmt.Println("✅ Redis connection closed")
			}
		}

		close(shutdownComplete)
	}()

	// Wait for shutdown to complete or timeout
	select {
	case <-shutdownComplete:
		fmt.Println("✅ Graceful shutdown completed")
		return nil
	case <-shutdownCtx.Done():
		fmt.Println("⚠️  Shutdown timeout exceeded, forcing exit")
		return fmt.Errorf("shutdown timeout")
	}
}

// readAndProcessMessages reads messages from WebSocket and processes them
func (a *App) readAndProcessMessages(ctx context.Context) {
	reconnectDelay := 5 * time.Second
	maxReconnectDelay := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
			message, err := a.wsManager.ReadMessage()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					// Check if it's an orderbook message (field 10 with text body)
					if strings.Contains(err.Error(), "orderbook message") {
						// Skip and continue - orderbook uses hybrid text format
						continue
					}

					// WebSocket connection error - attempt reconnection
					log.Printf("⚠️  WebSocket error: %v", err)
					log.Printf("🔄 Attempting to reconnect in %v...", reconnectDelay)

					// Wait before reconnecting
					select {
					case <-ctx.Done():
						return
					case <-time.After(reconnectDelay):
					}

					// Try to reconnect via manager
					if err := a.wsManager.Reconnect(); err != nil {
						log.Printf("❌ Reconnection failed: %v", err)
						// Exponential backoff
						reconnectDelay = reconnectDelay * 2
						if reconnectDelay > maxReconnectDelay {
							reconnectDelay = maxReconnectDelay
						}
						continue
					}

					// Reset delay on successful reconnection
					reconnectDelay = 5 * time.Second
					continue
				}
			}

			// Process the protobuf wrapper message
			err = a.handlerManager.HandleProtoMessage("running_trade", message)
			if err != nil {
				log.Printf("Handler error: %v", err)
				// Don't terminate on handler errors, just log and continue
				continue
			}
		}
	}
}

// setupHandlers initializes and registers all message handlers
func (a *App) setupHandlers() {
	// 4. Register Message Handlers
	// Running Trade Handler
	// Initialize Volatility Provider (ExitStrategyCalculator) for Adaptive Thresholds
	volatilityProv := NewExitStrategyCalculator(a.tradeRepo, a.config)
	runningTradeHandler := handlers.NewRunningTradeHandler(a.tradeRepo, a.webhookManager, a.redis, a.broker, volatilityProv)
	a.handlerManager.RegisterHandler("running_trade", runningTradeHandler)
}

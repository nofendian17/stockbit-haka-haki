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
	authClient      *auth.AuthClient
	tradingWS       *websocket.Client
	handlerManager  *handlers.HandlerManager
	db              *database.Database
	redis           *cache.RedisClient // Add Redis client to App struct
	tradeRepo       *database.TradeRepository
	webhookManager  *notifications.WebhookManager
	broker          *realtime.Broker
	signalTracker   *SignalTracker        // Phase 1: Signal outcome tracking
	whaleFollowup   *WhaleFollowupTracker // Phase 1: Whale alert followup
	baselineCalc    *BaselineCalculator   // Phase 2: Statistical baselines
	regimeDetector  *RegimeDetector       // Phase 2: Market regime detection
	patternDetector *PatternDetector      // Phase 2: Chart pattern detection
	correlationAnal *CorrelationAnalyzer  // Phase 3: Stock correlations
	perfRefresher   *PerformanceRefresher // Phase 3: Performance view refresher
	lastMessageTime time.Time             // Track last message for health monitoring
	lastMessageMu   sync.RWMutex
}

// New creates a new application instance
func New(cfg *config.Config) *App {
	return &App{
		config: cfg,
		authClient: auth.NewAuthClient(auth.Credentials{
			PlayerID: cfg.PlayerID,
			Email:    cfg.Username,
			Password: cfg.Password,
		}),
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
	// We don't error out if Redis fails, but it's good practice to check nil return if NewRedisClient handles error that way.
	// In the implementation I wrote, NewRedisClient returns nil on ping failure.
	// For robustness let's allow proceeding without Redis or we can act strict.
	// User said "add redis for cache", usually cache is optional but let's log warning.
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

	// 2. Authentication
	const tokenCacheFile = "/app/cache/.token_cache.json"
	if err := a.ensureAuthenticated(tokenCacheFile); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// 3. Connect Trading WebSocket
	accessToken := a.authClient.GetAccessToken()
	fmt.Println("🔌 Connecting to trading WebSocket...")
	a.tradingWS = websocket.NewClient(a.config.TradingWSURL, accessToken)

	if err := a.tradingWS.Connect(); err != nil {
		return fmt.Errorf("trading WebSocket connection failed: %w", err)
	}
	fmt.Println("✅ Trading WebSocket connected!")

	// 4. Get WebSocket key for subscription (with retry on token expiry)
	fmt.Println("🔑 Fetching WebSocket key...")
	wsKey, err := a.authClient.GetWebSocketKey()
	if err != nil {
		// If token expired, try to refresh and retry once
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "kedaluwarsa") {
			log.Println("⚠️  WebSocket key fetch failed (token expired), refreshing token...")

			// Try to refresh token
			if refreshErr := a.authClient.RefreshToken(); refreshErr != nil {
				log.Println("⚠️  Token refresh failed, re-authenticating...")
				// If refresh fails, try full re-login
				if loginErr := a.authClient.Login(); loginErr != nil {
					return fmt.Errorf("failed to re-authenticate: %w", loginErr)
				}
				// Save new token to cache
				_ = a.authClient.SaveTokenToFile(tokenCacheFile)
			}

			// Update WebSocket client with new token
			accessToken := a.authClient.GetAccessToken()
			a.tradingWS = websocket.NewClient(a.config.TradingWSURL, accessToken)
			if err := a.tradingWS.Connect(); err != nil {
				return fmt.Errorf("websocket reconnection failed: %w", err)
			}

			// Retry getting WebSocket key
			wsKey, err = a.authClient.GetWebSocketKey()
			if err != nil {
				return fmt.Errorf("failed to get websocket key after token refresh: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get websocket key: %w", err)
		}
	}
	fmt.Println("✅ WebSocket key obtained!")

	// 5. Subscribe to all stocks (wildcard)
	userID := fmt.Sprintf("%d", a.authClient.GetUserID())
	if err := a.tradingWS.SubscribeToStocks(nil, userID, wsKey); err != nil {
		log.Printf("Warning: Subscription failed: %v", err)
	}

	// 6. Setup handlers
	a.setupHandlers()

	// 7. Start ping
	a.tradingWS.StartPing(25 * time.Second)

	// 8. Initialize LLM client if enabled
	var llmClient *llm.Client
	if a.config.LLM.Enabled {
		llmClient = llm.NewClient(a.config.LLM.Endpoint, a.config.LLM.APIKey, a.config.LLM.Model)
		log.Printf("✅ LLM Pattern Recognition ENABLED (Model: %s)", a.config.LLM.Model)
	} else {
		log.Println("ℹ️  LLM Pattern Recognition DISABLED")
	}

	// 9. Start API Server
	apiServer := api.NewServer(a.tradeRepo, a.webhookManager, a.broker, llmClient, a.config.LLM.Enabled)

	// 10. Start Phase 1 Enhancement Trackers
	log.Println("🚀 Starting Phase 1 enhancement trackers...")

	// Signal Outcome Tracker
	a.signalTracker = NewSignalTracker(a.tradeRepo)
	go a.signalTracker.Start()

	// Inject signal tracker into API server
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

	// 11. Start Phase 2 Enhancement Trackers
	log.Println("🚀 Starting Phase 2 enhancement calculators...")

	// Statistical Baseline Calculator
	a.baselineCalc = NewBaselineCalculator(a.tradeRepo)
	go a.baselineCalc.Start()

	// Market Regime Detector
	a.regimeDetector = NewRegimeDetector(a.tradeRepo)
	go a.regimeDetector.Start()

	// Chart Pattern Detector
	a.patternDetector = NewPatternDetector(a.tradeRepo)
	go a.patternDetector.Start()

	// 12. Start Phase 3 Enhancement Trackers
	log.Println("🚀 Starting Phase 3 advanced analytics...")

	// Correlation Analyzer
	a.correlationAnal = NewCorrelationAnalyzer(a.tradeRepo)
	go a.correlationAnal.Start()

	// Performance Refresher
	a.perfRefresher = NewPerformanceRefresher(a.tradeRepo)
	go a.perfRefresher.Start()

	// Setup WaitGroup for goroutines
	var wg sync.WaitGroup

	// 6. Start background token refresh monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.monitorTokenExpiry(ctx, tokenCacheFile)
	}()

	// 7. Start WebSocket health monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.monitorWebSocketHealth(ctx)
	}()

	// 8. Start message processing
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.readAndProcessMessages(ctx)
	}()

	// 9. Wait for interrupt and perform graceful shutdown
	err = a.gracefulShutdown(cancel)
	wg.Wait()
	return err
}

// ensureAuthenticated handles the complex authentication and caching logic
func (a *App) ensureAuthenticated(cacheFile string) error {
	fmt.Println("🔐 Authenticating to Stockbit...")

	// Try to load and use cached token
	if err := a.authClient.LoadTokenFromFile(cacheFile); err == nil {
		if a.authClient.IsTokenValid() {
			fmt.Println("✅ Using cached token")
		} else {
			fmt.Println("⚠️  Cached token expired, refreshing...")
			if err := a.authClient.RefreshToken(); err != nil {
				fmt.Println("⚠️  Token refresh failed, logging in...")
				if err := a.authClient.Login(); err != nil {
					return err
				}
			} else {
				fmt.Println("✅ Token refreshed successfully")
			}
			_ = a.authClient.SaveTokenToFile(cacheFile)
		}
	} else {
		fmt.Println("🔑 No cached token, logging in...")
		if err := a.authClient.Login(); err != nil {
			return err
		}
		fmt.Println("✅ Login successful!")
		_ = a.authClient.SaveTokenToFile(cacheFile)
	}

	// Double check user ID
	if a.authClient.GetUserID() == 0 {
		if err := a.authClient.GetUserInfo(); err != nil {
			log.Printf("Warning: Failed to get user info: %v", err)
		} else {
			_ = a.authClient.SaveTokenToFile(cacheFile)
		}
	}

	fmt.Printf("📝 Access Token: %s...\n", a.authClient.GetAccessToken()[:m(50, len(a.authClient.GetAccessToken()))])
	fmt.Printf("⏰ Token expires at: %s\n", a.authClient.GetExpiryTime().Format("2006-01-02 15:04:05"))
	return nil
}

func m(a, b int) int {
	if a < b {
		return a
	}
	return b
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
		if a.regimeDetector != nil {
			fmt.Println("📈 Stopping market regime detector...")
			a.regimeDetector.Stop()
		}
		if a.patternDetector != nil {
			fmt.Println("🎨 Stopping chart pattern detector...")
			a.patternDetector.Stop()
		}
		if a.correlationAnal != nil {
			fmt.Println("🔗 Stopping correlation analyzer...")
			a.correlationAnal.Stop()
		}
		if a.perfRefresher != nil {
			fmt.Println("🔄 Stopping performance refresher...")
			a.perfRefresher.Stop()
		}

		// Close WebSocket connection
		if a.tradingWS != nil {
			fmt.Println("📡 Closing trading WebSocket connection...")
			if err := a.tradingWS.Close(); err != nil {
				log.Printf("Error closing trading WebSocket: %v", err)
			} else {
				fmt.Println("✅ Trading WebSocket closed")
			}
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
			message, err := a.tradingWS.ReadMessage()
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

					// Try to reconnect
					if err := a.reconnectWebSocket(); err != nil {
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
					log.Println("✅ Reconnected successfully, resuming message processing")
					continue
				}
			}

			// Update last message time for health monitoring
			a.updateLastMessageTime()

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

// reconnectWebSocket attempts to reconnect the WebSocket connection
func (a *App) reconnectWebSocket() error {
	const tokenCacheFile = "/app/cache/.token_cache.json"

	// Close existing connection
	if a.tradingWS != nil {
		_ = a.tradingWS.Close()
	}

	// Get fresh access token (might be expired)
	if !a.authClient.IsTokenValid() {
		log.Println("🔑 Token expired, refreshing...")
		if err := a.authClient.RefreshToken(); err != nil {
			log.Println("⚠️  Token refresh failed, logging in again...")
			if err := a.authClient.Login(); err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			// Save new token after successful login
			_ = a.authClient.SaveTokenToFile(tokenCacheFile)
		}
	}

	// Reconnect WebSocket
	accessToken := a.authClient.GetAccessToken()
	a.tradingWS = websocket.NewClient(a.config.TradingWSURL, accessToken)

	if err := a.tradingWS.Connect(); err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	// Get WebSocket key with retry on token expiry
	wsKey, err := a.authClient.GetWebSocketKey()
	if err != nil {
		// If token expired (401 error), try to refresh and retry once
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "kedaluwarsa") {
			log.Println("⚠️  WebSocket key fetch failed (token expired), refreshing token...")

			// Try to refresh token
			if refreshErr := a.authClient.RefreshToken(); refreshErr != nil {
				log.Println("⚠️  Token refresh failed, re-authenticating...")
				// If refresh fails, try full re-login
				if loginErr := a.authClient.Login(); loginErr != nil {
					return fmt.Errorf("failed to re-authenticate: %w", loginErr)
				}
				// Save new token to cache
				_ = a.authClient.SaveTokenToFile(tokenCacheFile)
			}

			// Update WebSocket client with new token
			accessToken = a.authClient.GetAccessToken()
			_ = a.tradingWS.Close()
			a.tradingWS = websocket.NewClient(a.config.TradingWSURL, accessToken)
			if err := a.tradingWS.Connect(); err != nil {
				return fmt.Errorf("websocket reconnection failed: %w", err)
			}

			// Retry getting WebSocket key
			wsKey, err = a.authClient.GetWebSocketKey()
			if err != nil {
				return fmt.Errorf("failed to get websocket key after token refresh: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get websocket key: %w", err)
		}
	}

	// Re-subscribe to stocks
	userID := fmt.Sprintf("%d", a.authClient.GetUserID())
	if err := a.tradingWS.SubscribeToStocks(nil, userID, wsKey); err != nil {
		return fmt.Errorf("subscription failed: %w", err)
	}

	// Restart ping
	a.tradingWS.StartPing(25 * time.Second)

	log.Println("✅ Reconnection successful with refreshed token")
	return nil
}

// setupHandlers initializes and registers all message handlers
func (a *App) setupHandlers() {
	// 4. Register Message Handlers
	// Running Trade Handler
	runningTradeHandler := handlers.NewRunningTradeHandler(a.tradeRepo, a.webhookManager, a.redis, a.broker)
	a.handlerManager.RegisterHandler("running_trade", runningTradeHandler)
}

// monitorTokenExpiry monitors token expiry and refreshes proactively
func (a *App) monitorTokenExpiry(ctx context.Context, tokenCacheFile string) {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	log.Println("🔄 Token expiry monitoring started")

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Token monitoring stopped")
			return
		case <-ticker.C:
			// Check if token will expire in the next 10 minutes
			expiryTime := a.authClient.GetExpiryTime()
			timeUntilExpiry := time.Until(expiryTime)

			if timeUntilExpiry <= 10*time.Minute {
				log.Printf("⚠️  Token will expire in %v, refreshing proactively...", timeUntilExpiry)

				if err := a.authClient.RefreshToken(); err != nil {
					log.Printf("❌ Token refresh failed: %v, attempting re-login...", err)
					if loginErr := a.authClient.Login(); loginErr != nil {
						log.Printf("❌ Re-login failed: %v", loginErr)
						continue
					}
					log.Println("✅ Re-login successful")
				} else {
					log.Println("✅ Token refreshed successfully")
				}

				// Save updated token to cache
				if err := a.authClient.SaveTokenToFile(tokenCacheFile); err != nil {
					log.Printf("⚠️  Failed to save refreshed token to cache: %v", err)
				} else {
					log.Println("💾 Token cache updated")
				}

				// Update WebSocket connection with new token
				accessToken := a.authClient.GetAccessToken()
				if a.tradingWS != nil {
					log.Println("🔄 Updating WebSocket connection with refreshed token...")
					_ = a.tradingWS.Close()
					a.tradingWS = websocket.NewClient(a.config.TradingWSURL, accessToken)

					if err := a.tradingWS.Connect(); err != nil {
						log.Printf("⚠️  Failed to reconnect WebSocket: %v", err)
						continue
					}

					// Re-subscribe
					wsKey, err := a.authClient.GetWebSocketKey()
					if err != nil {
						log.Printf("⚠️  Failed to get WebSocket key: %v", err)
						continue
					}

					userID := fmt.Sprintf("%d", a.authClient.GetUserID())
					if err := a.tradingWS.SubscribeToStocks(nil, userID, wsKey); err != nil {
						log.Printf("⚠️  Failed to re-subscribe: %v", err)
					}

					a.tradingWS.StartPing(25 * time.Second)
					log.Println("✅ WebSocket reconnected with new token")
				}
			} else if timeUntilExpiry > 0 {
				log.Printf("🔐 Token valid, expires in %v", timeUntilExpiry.Round(time.Minute))
			}
		}
	}
}

// monitorWebSocketHealth monitors WebSocket connection health
func (a *App) monitorWebSocketHealth(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second) // Check every 60 seconds
	defer ticker.Stop()

	log.Println("💓 WebSocket health monitoring started")

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 WebSocket health monitoring stopped")
			return
		case <-ticker.C:
			lastMsg := a.getLastMessageTime()
			if lastMsg.IsZero() {
				// First check, set initial time
				a.updateLastMessageTime()
				continue
			}

			timeSinceLastMessage := time.Since(lastMsg)

			// If no message received in 5 minutes, consider connection unhealthy
			if timeSinceLastMessage > 5*time.Minute {
				log.Printf("⚠️  No WebSocket message received for %v, reconnecting...", timeSinceLastMessage.Round(time.Second))

				if err := a.reconnectWebSocket(); err != nil {
					log.Printf("❌ WebSocket reconnection failed: %v", err)
				} else {
					log.Println("✅ WebSocket reconnected successfully")
					a.updateLastMessageTime()
				}
			} else {
				log.Printf("💓 WebSocket healthy, last message %v ago", timeSinceLastMessage.Round(time.Second))
			}
		}
	}
}

// updateLastMessageTime updates the timestamp of the last received message
func (a *App) updateLastMessageTime() {
	a.lastMessageMu.Lock()
	defer a.lastMessageMu.Unlock()
	a.lastMessageTime = time.Now()
}

// getLastMessageTime returns the timestamp of the last received message
func (a *App) getLastMessageTime() time.Time {
	a.lastMessageMu.RLock()
	defer a.lastMessageMu.RUnlock()
	return a.lastMessageTime
}

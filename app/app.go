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
	config         *config.Config
	authClient     *auth.AuthClient
	tradingWS      *websocket.Client
	handlerManager *handlers.HandlerManager
	db             *database.Database
	redis          *cache.RedisClient // Add Redis client to App struct
	tradeRepo      *database.TradeRepository
	webhookManager *notifications.WebhookManager
	broker         *realtime.Broker
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

	// 4. Get WebSocket key for subscription
	fmt.Println("🔑 Fetching WebSocket key...")
	wsKey, err := a.authClient.GetWebSocketKey()
	if err != nil {
		return fmt.Errorf("failed to get websocket key: %w", err)
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
	go func() {
		if err := apiServer.Start(8080); err != nil {
			log.Printf("⚠️  API Server failed: %v", err)
		}
	}()

	// 6. Start message processing
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.readAndProcessMessages(ctx)
	}()

	// 8. Wait for interrupt and perform graceful shutdown
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
		}
	}

	// Reconnect WebSocket
	accessToken := a.authClient.GetAccessToken()
	a.tradingWS = websocket.NewClient(a.config.TradingWSURL, accessToken)

	if err := a.tradingWS.Connect(); err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	// Get WebSocket key
	wsKey, err := a.authClient.GetWebSocketKey()
	if err != nil {
		return fmt.Errorf("failed to get websocket key: %w", err)
	}

	// Re-subscribe to stocks
	userID := fmt.Sprintf("%d", a.authClient.GetUserID())
	if err := a.tradingWS.SubscribeToStocks(nil, userID, wsKey); err != nil {
		return fmt.Errorf("subscription failed: %w", err)
	}

	// Restart ping
	a.tradingWS.StartPing(25 * time.Second)

	return nil
}

// setupHandlers initializes and registers all message handlers
func (a *App) setupHandlers() {
	// 4. Register Message Handlers
	// Running Trade Handler
	runningTradeHandler := handlers.NewRunningTradeHandler(a.tradeRepo, a.webhookManager, a.redis, a.broker)
	a.handlerManager.RegisterHandler("running_trade", runningTradeHandler)
}

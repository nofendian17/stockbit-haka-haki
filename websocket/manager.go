package websocket

import (
	"context"
	"fmt"
	"log"
	"stockbit-haka-haki/auth"
	pb "stockbit-haka-haki/proto"
	"strings"
	"time"
)

// ConnectionManager handles WebSocket connection lifecycle, health monitoring, and reconnection.
type ConnectionManager struct {
	client      *Client
	authManager *auth.AuthManager
	wsURL       string
	lastMsgTime time.Time
}

// NewConnectionManager creates a new ConnectionManager.
func NewConnectionManager(wsURL string, authManager *auth.AuthManager) *ConnectionManager {
	return &ConnectionManager{
		wsURL:       wsURL,
		authManager: authManager,
		lastMsgTime: time.Now(),
	}
}

// Connect establishes the initial WebSocket connection using the AuthManager.
func (cm *ConnectionManager) Connect() error {
	accessToken := cm.authManager.GetClient().GetAccessToken()
	fmt.Println("🔌 Connecting to trading WebSocket...")
	cm.client = NewClient(cm.wsURL, accessToken)

	if err := cm.client.Connect(); err != nil {
		return fmt.Errorf("trading WebSocket connection failed: %w", err)
	}
	fmt.Println("✅ Trading WebSocket connected!")

	// Authenticate WebSocket session
	return cm.AuthenticateAndSubscribe()
}

// AuthenticateAndSubscribe handles the WS-specific handshake (getting key and subscribing).
func (cm *ConnectionManager) AuthenticateAndSubscribe() error {
	// Get WebSocket key for subscription (with retry on token expiry)
	fmt.Println("🔑 Fetching WebSocket key...")
	authClient := cm.authManager.GetClient()
	wsKey, err := authClient.GetWebSocketKey()

	if err != nil {
		// If token expired, try to refresh and retry once
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "kedaluwarsa") {
			log.Println("⚠️  WebSocket key fetch failed (token expired), refreshing token...")

			// Force refresh via AuthManager's client
			if refreshErr := authClient.RefreshToken(); refreshErr != nil {
				log.Println("⚠️  Token refresh failed, re-authenticating...")
				if loginErr := authClient.Login(); loginErr != nil {
					return fmt.Errorf("failed to re-authenticate: %w", loginErr)
				}
			}

			// Update WebSocket client with new token
			accessToken := authClient.GetAccessToken()
			cm.client = NewClient(cm.wsURL, accessToken) // Re-create client with new token
			if err := cm.client.Connect(); err != nil {
				return fmt.Errorf("websocket reconnection failed: %w", err)
			}

			// Retry getting WebSocket key
			wsKey, err = authClient.GetWebSocketKey()
			if err != nil {
				return fmt.Errorf("failed to get websocket key after token refresh: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get websocket key: %w", err)
		}
	}
	fmt.Println("✅ WebSocket key obtained!")

	// Subscribe to all stocks (wildcard)
	userID := fmt.Sprintf("%d", authClient.GetUserID())
	if err := cm.client.SubscribeToStocks(nil, userID, wsKey); err != nil {
		log.Printf("Warning: Subscription failed: %v", err)
		return err
	}

	return nil
}

// StartPing starts the keep-alive pinger.
func (cm *ConnectionManager) StartPing(interval time.Duration) {
	if cm.client != nil {
		cm.client.StartPing(interval)
	}
}

// ReadMessage reads a message from the WebSocket.
func (cm *ConnectionManager) ReadMessage() (*pb.WebsocketWrapMessageChannel, error) {
	if cm.client == nil {
		return nil, fmt.Errorf("client not connected")
	}
	msg, err := cm.client.ReadMessage()
	if err == nil {
		cm.lastMsgTime = time.Now()
	}
	return msg, err
}

// Close closes the connection.
func (cm *ConnectionManager) Close() error {
	if cm.client != nil {
		return cm.client.Close()
	}
	return nil
}

// Reconnect attempts to reconnect the WebSocket.
func (cm *ConnectionManager) Reconnect() error {
	// Close existing connection
	_ = cm.Close()

	// Ensure token is valid before reconnecting
	authClient := cm.authManager.GetClient()
	if !authClient.IsTokenValid() {
		log.Println("🔑 Token expired, refreshing for reconnection...")
		if err := authClient.RefreshToken(); err != nil {
			log.Println("⚠️  Token refresh failed, logging in again...")
			if err := authClient.Login(); err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
		}
	}

	// Re-establish connection
	accessToken := authClient.GetAccessToken()
	cm.client = NewClient(cm.wsURL, accessToken)

	if err := cm.client.Connect(); err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	if err := cm.AuthenticateAndSubscribe(); err != nil {
		return err
	}

	cm.StartPing(25 * time.Second)
	log.Println("✅ Reconnection successful with refreshed token")
	return nil
}

// RunHealthMonitor starts a background loop to check connection health.
func (cm *ConnectionManager) RunHealthMonitor(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second) // Check every 60 seconds
	defer ticker.Stop()

	log.Println("💓 WebSocket health monitoring started")

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 WebSocket health monitoring stopped")
			return
		case <-ticker.C:
			timeSinceLastMessage := time.Since(cm.lastMsgTime)

			// If no message received in 5 minutes, consider connection unhealthy
			if timeSinceLastMessage > 5*time.Minute {
				log.Printf("⚠️  No WebSocket message received for %v, reconnecting...", timeSinceLastMessage.Round(time.Second))

				if err := cm.Reconnect(); err != nil {
					log.Printf("❌ WebSocket reconnection failed: %v", err)
				} else {
					log.Println("✅ WebSocket reconnected successfully")
					cm.lastMsgTime = time.Now()
				}
			} else {
				log.Printf("💓 WebSocket healthy, last message %v ago", timeSinceLastMessage.Round(time.Second))
			}
		}
	}
}

// UpdateToken updates the client connection when token is refreshed externally.
func (cm *ConnectionManager) UpdateToken(newToken string) {
	log.Println("🔄 Updating WebSocket connection with refreshed token...")
	_ = cm.Close()

	// Reconnect will pick up the new token from AuthManager (which shares the AuthClient)
	// But note: NewConnectionManager doesn't reference AuthClient directly, it references AuthManager.
	// We should rely on AuthManager having the updated token.

	if err := cm.Reconnect(); err != nil {
		log.Printf("⚠️  Failed to reconnect WebSocket after token update: %v", err)
	} else {
		log.Println("✅ WebSocket reconnected with new token")
	}
}

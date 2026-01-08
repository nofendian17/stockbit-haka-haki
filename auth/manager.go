package auth

import (
	"context"
	"fmt"
	"log"
	"time"
)

// AuthManager handles authentication lifecycle including login, token refresh, and persistence.
type AuthManager struct {
	client         *AuthClient
	tokenCacheFile string
}

// NewAuthManager creates a new AuthManager instance.
func NewAuthManager(client *AuthClient, tokenCacheFile string) *AuthManager {
	return &AuthManager{
		client:         client,
		tokenCacheFile: tokenCacheFile,
	}
}

// EnsureAuthenticated handles initial authentication: loading from cache, refreshing, or logging in.
func (am *AuthManager) EnsureAuthenticated() error {
	fmt.Println("🔐 Authenticating to Stockbit...")

	// Try to load and use cached token
	if err := am.client.LoadTokenFromFile(am.tokenCacheFile); err == nil {
		if am.client.IsTokenValid() {
			fmt.Println("✅ Using cached token")
		} else {
			fmt.Println("⚠️  Cached token expired, refreshing...")
			if err := am.client.RefreshToken(); err != nil {
				fmt.Println("⚠️  Token refresh failed, logging in...")
				if err := am.client.Login(); err != nil {
					return err
				}
			} else {
				fmt.Println("✅ Token refreshed successfully")
			}
			_ = am.client.SaveTokenToFile(am.tokenCacheFile)
		}
	} else {
		fmt.Println("🔑 No cached token, logging in...")
		if err := am.client.Login(); err != nil {
			return err
		}
		fmt.Println("✅ Login successful!")
		_ = am.client.SaveTokenToFile(am.tokenCacheFile)
	}

	// Double check user ID
	if am.client.GetUserID() == 0 {
		if err := am.client.GetUserInfo(); err != nil {
			log.Printf("Warning: Failed to get user info: %v", err)
		} else {
			_ = am.client.SaveTokenToFile(am.tokenCacheFile)
		}
	}

	token := am.client.GetAccessToken()
	fmt.Printf("📝 Access Token: %s...\n", token[:m(50, len(token))])
	fmt.Printf("⏰ Token expires at: %s\n", am.client.GetExpiryTime().Format("2006-01-02 15:04:05"))
	return nil
}

// RunTokenMonitor starts a background loop to monitor token expiry and refresh proactively.
func (am *AuthManager) RunTokenMonitor(ctx context.Context, onRefreshSuccess func(string)) {
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
			expiryTime := am.client.GetExpiryTime()
			timeUntilExpiry := time.Until(expiryTime)

			if timeUntilExpiry <= 10*time.Minute {
				log.Printf("⚠️  Token will expire in %v, refreshing proactively...", timeUntilExpiry)

				success := false
				if err := am.client.RefreshToken(); err != nil {
					log.Printf("❌ Token refresh failed: %v, attempting re-login...", err)
					if loginErr := am.client.Login(); loginErr != nil {
						log.Printf("❌ Re-login failed: %v", loginErr)
					} else {
						log.Println("✅ Re-login successful")
						success = true
					}
				} else {
					log.Println("✅ Token refreshed successfully")
					success = true
				}

				if success {
					// Save updated token to cache
					if err := am.client.SaveTokenToFile(am.tokenCacheFile); err != nil {
						log.Printf("⚠️  Failed to save refreshed token to cache: %v", err)
					} else {
						log.Println("💾 Token cache updated")
					}

					// Notify callback if provided (e.g., to reconnect WebSocket)
					if onRefreshSuccess != nil {
						token := am.client.GetAccessToken()
						onRefreshSuccess(token)
					}
				}
			} else if timeUntilExpiry > 0 {
				log.Printf("🔐 Token valid, expires in %v", timeUntilExpiry.Round(time.Minute))
			}
		}
	}
}

// GetClient returns the underlying AuthClient.
func (am *AuthManager) GetClient() *AuthClient {
	return am.client
}

func m(a, b int) int {
	if a < b {
		return a
	}
	return b
}

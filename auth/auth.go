package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Credentials menyimpan informasi kredensial pengguna
type Credentials struct {
	PlayerID string
	Email    string
	Password string
}

// TokenData stores authentication tokens
type TokenData struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	UserID       int64
}

// AuthClient handles authentication
type AuthClient struct {
	credentials Credentials
	tokenData   TokenData
	httpClient  *http.Client
	mu          sync.RWMutex
}

type LoginResponse struct {
	Data struct {
		Login struct {
			TokenData struct {
				Access struct {
					Token     string `json:"token"`
					ExpiredAt string `json:"expired_at"`
				} `json:"access"`
				Refresh struct {
					Token     string `json:"token"`
					ExpiredAt string `json:"expired_at"`
				} `json:"refresh"`
			} `json:"token_data"`
		} `json:"login"`
	} `json:"data"`
}

type RefreshResponse struct {
	Data struct {
		Refresh struct {
			Access struct {
				Token     string `json:"token"`
				ExpiredAt string `json:"expired_at"`
			} `json:"access"`
			Refresh struct {
				Token     string `json:"token"`
				ExpiredAt string `json:"expired_at"`
			} `json:"refresh"`
		} `json:"refresh"`
	} `json:"data"`
}

// NewAuthClient membuat instance AuthClient baru
func NewAuthClient(creds Credentials) *AuthClient {
	return &AuthClient{
		credentials: creds,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Increased from 10s to 30s
		},
	}
}

// SaveTokenToFile saves token data to file
func (ac *AuthClient) SaveTokenToFile(filepath string) error {
	ac.mu.RLock()
	data := struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		ExpiresAt    time.Time `json:"expires_at"`
		UserID       int64     `json:"user_id"`
	}{
		AccessToken:  ac.tokenData.AccessToken,
		RefreshToken: ac.tokenData.RefreshToken,
		ExpiresAt:    ac.tokenData.ExpiresAt,
		UserID:       ac.tokenData.UserID,
	}
	ac.mu.RUnlock()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token data: %w", err)
	}

	if err := os.WriteFile(filepath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

// LoadTokenFromFile loads token data from file
func (ac *AuthClient) LoadTokenFromFile(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("token file not found")
		}
		return fmt.Errorf("failed to read token file: %w", err)
	}

	var tokenCache struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		ExpiresAt    time.Time `json:"expires_at"`
		UserID       int64     `json:"user_id"`
	}

	if err := json.Unmarshal(data, &tokenCache); err != nil {
		return fmt.Errorf("failed to parse token file: %w", err)
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.tokenData.AccessToken = tokenCache.AccessToken
	ac.tokenData.RefreshToken = tokenCache.RefreshToken
	ac.tokenData.ExpiresAt = tokenCache.ExpiresAt
	ac.tokenData.UserID = tokenCache.UserID

	return nil
}

// Login melakukan autentikasi dan menyimpan token
func (ac *AuthClient) Login() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Prepare request body
	loginData := map[string]string{
		"player_id": ac.credentials.PlayerID,
		"user":      ac.credentials.Email,
		"password":  ac.credentials.Password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return fmt.Errorf("failed to marshal login data: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", "https://exodus.stockbit.com/login/v5/username", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers sesuai requirement Stockbit
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
	req.Header.Set("X-DeviceType", "Google Chrome")
	req.Header.Set("X-Platform", "PC")
	req.Header.Set("X-AppVersion", "3.17.2")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "ID")

	// Send request
	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send login request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response using decoder for better performance
	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("failed to parse login response: %w", err)
	}

	// Store tokens
	ac.tokenData.AccessToken = loginResp.Data.Login.TokenData.Access.Token
	ac.tokenData.RefreshToken = loginResp.Data.Login.TokenData.Refresh.Token

	// Parse expiry from API response (expired_at is ISO 8601 timestamp)
	expiresAt, err := time.Parse(time.RFC3339, loginResp.Data.Login.TokenData.Access.ExpiredAt)
	if err != nil {
		log.Printf("Warning: Failed to parse expired_at: %v", err)
		ac.tokenData.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
	} else {
		ac.tokenData.ExpiresAt = expiresAt.UTC()
	}

	return nil
}

// RefreshToken melakukan refresh access token menggunakan refresh token
func (ac *AuthClient) RefreshToken() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Create request
	req, err := http.NewRequest("POST", "https://exodus.stockbit.com/login/refresh", nil)
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+ac.tokenData.RefreshToken)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response using decoder
	var refreshResp RefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return fmt.Errorf("failed to parse refresh response: %w", err)
	}

	// Update both access and refresh tokens
	ac.tokenData.AccessToken = refreshResp.Data.Refresh.Access.Token
	ac.tokenData.RefreshToken = refreshResp.Data.Refresh.Refresh.Token

	// Parse expiry from API response
	expiresAt, err := time.Parse(time.RFC3339, refreshResp.Data.Refresh.Access.ExpiredAt)
	if err != nil {
		log.Printf("Warning: Failed to parse expired_at: %v", err)
		ac.tokenData.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
	} else {
		ac.tokenData.ExpiresAt = expiresAt.UTC()
	}

	return nil
}

// GetValidToken mengembalikan token yang valid, auto-refresh jika diperlukan
func (ac *AuthClient) GetValidToken() (string, error) {
	// Check jika token akan expired dalam 5 menit
	if time.Now().UTC().Add(5 * time.Minute).After(ac.tokenData.ExpiresAt) {
		fmt.Println("🔄 Token akan expired, melakukan refresh...")
		if err := ac.RefreshToken(); err != nil {
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
		fmt.Println("✅ Token berhasil di-refresh!")
	}

	return ac.tokenData.AccessToken, nil
}

// GetAccessToken mengembalikan access token
func (ac *AuthClient) GetAccessToken() string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.tokenData.AccessToken
}

// GetExpiryTime mengembalikan waktu kedaluwarsa token
func (ac *AuthClient) GetExpiryTime() time.Time {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.tokenData.ExpiresAt
}

// IsTokenValid mengecek apakah token masih valid
func (ac *AuthClient) IsTokenValid() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return time.Now().UTC().Before(ac.tokenData.ExpiresAt)
}

// GetUserID mengembalikan user ID
func (ac *AuthClient) GetUserID() int64 {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.tokenData.UserID
}

// GetUserInfo mengambil informasi user dari API
func (ac *AuthClient) GetUserInfo() error {
	ac.mu.RLock()
	token := ac.tokenData.AccessToken
	ac.mu.RUnlock()

	// Create request
	req, err := http.NewRequest("GET", "https://exodus.stockbit.com/usergraph/socialinfo/user/me", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("get user info failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var userInfo struct {
		Data struct {
			UserID int64 `json:"user_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return fmt.Errorf("failed to parse user info response: %w", err)
	}

	// Store user ID
	ac.mu.Lock()
	ac.tokenData.UserID = userInfo.Data.UserID
	ac.mu.Unlock()

	fmt.Printf("👤 User ID: %d\n", userInfo.Data.UserID)

	return nil
}

// WebSocketKeyResponse represents the API response structure for websocket key
type WebSocketKeyResponse struct {
	Data struct {
		Key string `json:"key"`
	} `json:"data"`
}

// GetWebSocketKey fetches the WebSocket authentication key from the API
func (ac *AuthClient) GetWebSocketKey() (string, error) {
	// Use GetValidToken to ensure token is fresh and auto-refresh if needed
	token, err := ac.GetValidToken()
	if err != nil {
		return "", fmt.Errorf("failed to get valid token: %w", err)
	}

	// Create request
	req, err := http.NewRequest("GET", "https://exodus.stockbit.com/auth/websocket/key", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		// If it's a 401, token is invalid - perform re-login and retry once
		if resp.StatusCode == http.StatusUnauthorized {
			log.Println("⚠️  Unauthorized to get websocket key, performing re-login...")

			// Perform re-login
			if loginErr := ac.Login(); loginErr != nil {
				return "", fmt.Errorf("re-login failed: %w", loginErr)
			}

			log.Println("✅ Re-login successful, retrying to get websocket key...")

			// Retry with new token
			return ac.fetchWebSocketKey()
		}

		// Non-401 error
		return "", fmt.Errorf("get websocket key failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse and return response
	return parseWebSocketKeyResponse(resp)
}

// fetchWebSocketKey is a helper that fetches websocket key with current token (no auto-relogin)
func (ac *AuthClient) fetchWebSocketKey() (string, error) {
	token := ac.GetAccessToken()

	req, err := http.NewRequest("GET", "https://exodus.stockbit.com/auth/websocket/key", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get websocket key failed with status %d: %s", resp.StatusCode, string(body))
	}

	return parseWebSocketKeyResponse(resp)
}

// parseWebSocketKeyResponse parses the websocket key from HTTP response
func parseWebSocketKeyResponse(resp *http.Response) (string, error) {
	var wsKeyResp WebSocketKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&wsKeyResp); err != nil {
		return "", fmt.Errorf("failed to parse websocket key response: %w", err)
	}
	return wsKeyResp.Data.Key, nil
}

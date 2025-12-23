package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"stockbit-haka-haki/cache"
	"stockbit-haka-haki/database"
	"stockbit-haka-haki/helpers"
)

// WebhookManager handles webhook notifications
type WebhookManager struct {
	repo   *database.TradeRepository
	redis  *cache.RedisClient
	client *http.Client
}

// WebhookPayload represents the JSON payload sent to webhooks
type WebhookPayload struct {
	AlertID         int64                  `json:"AlertID"`
	AlertType       string                 `json:"AlertType"`
	DetectedAt      time.Time              `json:"DetectedAt"`
	StockSymbol     string                 `json:"StockSymbol"`
	Action          string                 `json:"Action"`
	Price           float64                `json:"Price"`
	VolumeLots      float64                `json:"VolumeLots"`
	TotalValue      float64                `json:"TotalValue"`
	AvgPrice        float64                `json:"AvgPrice"`
	ConfidenceScore float64                `json:"ConfidenceScore"`
	MarketBoard     string                 `json:"MarketBoard"`
	Message         string                 `json:"Message"`
	Metadata        map[string]interface{} `json:"Metadata,omitempty"`
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager(repo *database.TradeRepository, redis *cache.RedisClient) *WebhookManager {
	return &WebhookManager{
		repo:  repo,
		redis: redis,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendAlert processes and sends the alert to matching webhooks
func (wm *WebhookManager) SendAlert(alert *database.WhaleAlert) {
	// 1. Get all active webhooks
	webhooks, err := wm.getActiveWebhooks()
	if err != nil {
		log.Printf("⚠️  Failed to load webhooks: %v", err)
		return
	}

	if len(webhooks) == 0 {
		return
	}

	// 2. Prepare payload
	payload := wm.CreatePayload(alert)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("⚠️  Failed to marshal webhook payload: %v", err)
		return
	}

	// 3. Process each webhook (async)
	for _, hook := range webhooks {
		if wm.shouldSend(hook, alert) {
			go wm.deliverWebhook(hook, alert.ID, payloadBytes)
		}
	}
}

func (wm *WebhookManager) getActiveWebhooks() ([]database.WhaleWebhook, error) {
	// Try cache first
	cacheKey := "active_webhooks"
	if wm.redis != nil {
		var cached []database.WhaleWebhook
		if err := wm.redis.Get(context.Background(), cacheKey, &cached); err == nil {
			return cached, nil
		}
	}

	// Fetch from DB
	webhooks, err := wm.repo.GetActiveWebhooks()
	if err != nil {
		return nil, err
	}

	// Update cache (expire 1 hour)
	if wm.redis != nil {
		_ = wm.redis.Set(context.Background(), cacheKey, webhooks, 1*time.Hour)
	}

	return webhooks, err
}

// CreatePayload generates the webhook payload from an alert
func (wm *WebhookManager) CreatePayload(alert *database.WhaleAlert) WebhookPayload {
	// Calculate derived values for message
	var zScoreVal, volPctVal float64
	if alert.ZScore != nil {
		zScoreVal = *alert.ZScore
	}
	if alert.VolumeVsAvgPct != nil {
		volPctVal = *alert.VolumeVsAvgPct
	}
	avgPriceVal := 0.0
	if alert.AvgPrice != nil {
		avgPriceVal = *alert.AvgPrice
	}

	// Format readable message
	// Example: "🐋 WHALE ALERT! BBRI BUY | Vol: 1000 (500% Avg) | Value: Rp 5.000.000.000 | Z-Score: 4.5"
	priceInfo := fmt.Sprintf("%.0f", alert.TriggerPrice)
	if avgPriceVal > 0 {
		diffPct := ((alert.TriggerPrice - avgPriceVal) / avgPriceVal) * 100
		priceInfo = fmt.Sprintf("%.0f (Avg: %.0f, %+0.1f%%)", alert.TriggerPrice, avgPriceVal, diffPct)
	}

	message := fmt.Sprintf("🐋 WHALE ALERT! %s %s | Vol: %.0f (%.0f%% Avg) | Value: %s | Price: %s | Z-Score: %.2f",
		alert.StockSymbol,
		alert.Action,
		alert.TriggerVolumeLots,
		volPctVal,
		helpers.FormatRupiah(alert.TriggerValue),
		priceInfo,
		zScoreVal,
	)

	return WebhookPayload{
		AlertID:         alert.ID,
		AlertType:       alert.AlertType,
		DetectedAt:      alert.DetectedAt,
		StockSymbol:     alert.StockSymbol,
		Action:          alert.Action,
		Price:           alert.TriggerPrice,
		VolumeLots:      alert.TriggerVolumeLots,
		TotalValue:      alert.TriggerValue,
		AvgPrice:        avgPriceVal,
		ConfidenceScore: alert.ConfidenceScore,
		MarketBoard:     alert.MarketBoard,
		Message:         message,
		Metadata: map[string]interface{}{
			"z_score":        alert.ZScore,
			"volume_vs_avg":  alert.VolumeVsAvgPct,
			"pattern_trades": alert.PatternTradeCount,
		},
	}
}

func (wm *WebhookManager) shouldSend(hook database.WhaleWebhook, alert *database.WhaleAlert) bool {
	// Check Alert Type filter
	if hook.AlertTypes != "" && hook.AlertTypes != "null" {
		// Lenient check: matches if the type is present in the string (JSON or CSV)
		if !strings.Contains(hook.AlertTypes, alert.AlertType) {
			return false
		}
	}

	// Check Stock Symbol filter
	if hook.StockSymbols != "" && hook.StockSymbols != "null" {
		if !strings.Contains(hook.StockSymbols, alert.StockSymbol) {
			return false
		}
	}

	// Check thresholds
	if hook.MinConfidence != nil && alert.ConfidenceScore < *hook.MinConfidence {
		return false
	}

	if hook.MinValue != nil && alert.TriggerValue < *hook.MinValue {
		return false
	}

	return true
}

func (wm *WebhookManager) deliverWebhook(hook database.WhaleWebhook, alertID int64, payload []byte) {
	// Basic implementation without fancy retry logic for MVP phase 1
	maxRetries := hook.RetryCount
	if maxRetries <= 0 {
		maxRetries = 1
	}

	var resp *http.Response
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, _ := http.NewRequest(hook.Method, hook.URL, bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Stockbit-Whale-Alert/1.0")

		log.Printf("🔹 Sending webhook to %s (Attempt %d/%d)", hook.URL, attempt, maxRetries)

		// Auth headers
		if hook.AuthType == "BEARER" {
			req.Header.Set("Authorization", "Bearer "+hook.AuthValue)
		} else if hook.AuthHeader != "" {
			req.Header.Set(hook.AuthHeader, hook.AuthValue)
		}

		resp, err = wm.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// Success
			wm.logDelivery(hook.ID, alertID, "SUCCESS", resp.StatusCode, "", attempt)
			if resp.Body != nil {
				resp.Body.Close()
			}
			return
		}

		// Wait before retry
		if attempt < maxRetries {
			time.Sleep(time.Duration(hook.RetryDelaySeconds) * time.Second)
		}
	}

	// Failed
	status := "FAILED"
	errMsg := ""
	statusCode := 0
	if err != nil {
		errMsg = err.Error()
	} else if resp != nil {
		statusCode = resp.StatusCode
		resp.Body.Close()
	}

	wm.logDelivery(hook.ID, alertID, status, statusCode, errMsg, maxRetries)
}

func (wm *WebhookManager) logDelivery(webhookID int, alertID int64, status string, code int, err string, attempt int) {
	logEntry := &database.WhaleWebhookLog{
		WebhookID:    webhookID,
		WhaleAlertID: &alertID,
		TriggeredAt:  time.Now(),
		Status:       status,
		RetryAttempt: attempt,
	}

	if code != 0 {
		logEntry.HTTPStatusCode = &code
	}
	if err != "" {
		logEntry.ErrorMessage = err
	}

	if dbErr := wm.repo.SaveWebhookLog(logEntry); dbErr != nil {
		log.Printf("⚠️  Failed to save webhook log: %v", dbErr)
	}
}

// RefreshCache reloads webhook configurations
func (wm *WebhookManager) RefreshCache() {
	if wm.redis != nil {
		_ = wm.redis.Delete(context.Background(), "active_webhooks")
		log.Println("🔄 Webhook cache invalidated")
	}
}

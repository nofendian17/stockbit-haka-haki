package handlers

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"stockbit-haka-haki/cache"
	"stockbit-haka-haki/database"
	"stockbit-haka-haki/helpers"
	"stockbit-haka-haki/notifications"
	pb "stockbit-haka-haki/proto"
	"stockbit-haka-haki/realtime"
)

// Detection thresholds
const (
	minSafeValue          = 100_000_000.0   // 100 Million IDR - Safety floor to avoid penny stock noise
	billionIDR            = 1_000_000_000.0 // 1 Billion IDR
	zScoreThreshold       = 3.0             // Statistical anomaly threshold
	volumeSpikeMultiplier = 5.0             // 5x average volume
	fallbackLotThreshold  = 1000            // Fallback threshold for lots
	statsLookbackMinutes  = 60              // 1 hour lookback for statistics
	statsCacheDuration    = 5 * time.Minute // Cache stats for 5 minutes
	accumulationWindow    = 5 * time.Second // Window for aggregating rapid trades
)

// Cache key prefixes
const (
	cacheKeyStatsPrefix = "stats:stock:"
)

// RunningTradeHandler mengelola pesan RunningTrade dari protobuf
type RunningTradeHandler struct {
	tradeRepo      *database.TradeRepository     // Repository untuk menyimpan data trade
	webhookManager *notifications.WebhookManager // Manager untuk notifikasi webhook
	redis          *cache.RedisClient            // Redis client for config caching
	broker         *realtime.Broker              // Realtime SSE broker

	// Accumulation buffer for detecting rapid small trades
	mu                 sync.Mutex
	accumulationBuffer map[string][]TradeInfo
}

// TradeInfo holds minimal info for accumulation detection
type TradeInfo struct {
	Timestamp time.Time
	VolumeLot float64
	Price     float64
	TotalVal  float64
	Action    string // BUY or SELL
}

// NewRunningTradeHandler membuat instance handler baru
func NewRunningTradeHandler(tradeRepo *database.TradeRepository, webhookManager *notifications.WebhookManager, redis *cache.RedisClient, broker *realtime.Broker) *RunningTradeHandler {
	return &RunningTradeHandler{
		tradeRepo:          tradeRepo,
		webhookManager:     webhookManager,
		redis:              redis,
		broker:             broker,
		accumulationBuffer: make(map[string][]TradeInfo),
	}
}

// Handle adalah method legacy - tidak digunakan dengan implementasi protobuf baru
func (h *RunningTradeHandler) Handle(data []byte) error {
	return fmt.Errorf("use HandleProto instead")
}

// HandleProto memproses pesan protobuf wrapper dari WebSocket
func (h *RunningTradeHandler) HandleProto(wrapper interface{}) error {
	msg, ok := wrapper.(*pb.WebsocketWrapMessageChannel)
	if !ok {
		return fmt.Errorf("invalid message type: expected *pb.WebsocketWrapMessageChannel")
	}

	// Proses berbagai tipe pesan dari wrapper
	switch v := msg.MessageChannel.(type) {
	case *pb.WebsocketWrapMessageChannel_RunningTrade:
		if v.RunningTrade != nil {
			h.ProcessTrade(v.RunningTrade)
		}

	case *pb.WebsocketWrapMessageChannel_RunningTradeBatch:
		if v.RunningTradeBatch != nil {
			for _, trade := range v.RunningTradeBatch.Trades {
				h.ProcessTrade(trade)
			}
		}

	case *pb.WebsocketWrapMessageChannel_Ping:
		// Ping response - silent

	case *pb.WebsocketWrapMessageChannel_OrderbookBody:
		if v.OrderbookBody != nil {
			h.ProcessOrderBookBody(v.OrderbookBody)
		}

	default:
		return fmt.Errorf("unknown message channel type")
	}

	return nil
}

// getStockStats retrieves stock statistics, checking cache first then database
func (h *RunningTradeHandler) getStockStats(stock string) *database.StockStats {
	if h.redis == nil && h.tradeRepo == nil {
		return nil
	}

	cacheKey := cacheKeyStatsPrefix + stock
	stats := &database.StockStats{}

	// Try cache first
	if h.redis != nil {
		if err := h.redis.Get(context.Background(), cacheKey, stats); err == nil {
			return stats
		}
	}

	// Cache miss - fetch from database
	if h.tradeRepo != nil {
		dbStats, err := h.tradeRepo.GetStockStats(stock, statsLookbackMinutes)
		if err != nil {
			return nil
		}

		// Update cache for next time
		if h.redis != nil {
			_ = h.redis.Set(context.Background(), cacheKey, dbStats, statsCacheDuration)
		}

		return dbStats
	}

	return nil
}

// ProcessTrade memproses satu pesan trade individual
func (h *RunningTradeHandler) ProcessTrade(t *pb.RunningTrade) {
	// Start benchmarking timer
	startTime := time.Now()

	// Tentukan action berdasarkan tipe trade
	var actionDb string

	switch t.Action {
	case pb.TradeType_TRADE_TYPE_BUY:
		actionDb = "BUY"
	case pb.TradeType_TRADE_TYPE_SELL:
		actionDb = "SELL"
	default:
		actionDb = "UNKNOWN"
	}

	// Tentukan board type (market type)
	var boardType string
	switch t.MarketBoard {
	case pb.BoardType_BOARD_TYPE_RG:
		boardType = "RG" // Regular Market
	case pb.BoardType_BOARD_TYPE_TN:
		boardType = "TN" // Cash/Tunai
	case pb.BoardType_BOARD_TYPE_NG:
		boardType = "NG" // Negotiated/Negosiasi
	default:
		boardType = "??"
	}

	// Format perubahan persentase jika tersedia
	var changePercentage *float64
	if t.Change != nil {
		changePercentage = &t.Change.Percentage
	}

	// PENTING: Volume dalam protobuf adalah SHARES (saham)
	// Konversi ke LOT: 1 lot = 100 shares
	volumeLot := t.Volume / 100

	// Hitung total nilai transaksi dalam Rupiah
	totalAmount := t.Price * t.Volume

	// Simpan ke database jika repository tersedia
	if h.tradeRepo != nil {
		// Convert trade_number to pointer for nullable field
		var tradeNumber *int64
		if t.TradeNumber != 0 {
			tradeNumber = &t.TradeNumber
		}

		trade := &database.Trade{
			Timestamp:   time.Now(), // Stored in UTC
			StockSymbol: t.Stock,
			Action:      actionDb,
			Price:       t.Price,
			Volume:      t.Volume,
			VolumeLot:   volumeLot,
			TotalAmount: totalAmount,
			MarketBoard: boardType,
			Change:      changePercentage,
			TradeNumber: tradeNumber,
		}

		if err := h.tradeRepo.SaveTrade(trade); err != nil {
			// Check if it's a duplicate key error (unique constraint violation)
			// PostgreSQL error code 23505 = unique_violation
			errMsg := err.Error()
			if containsAny(errMsg, []string{"duplicate key", "unique constraint", "23505"}) {
				// Duplicate trade detected - this is expected, log at debug level
				// Don't spam logs with duplicate warnings
				return
			}

			// Log unexpected errors
			log.Printf("⚠️  Failed to save trade to database: %v", err)
		}

		// 🐋 WHALE DETECTION - STATISTICAL MODEL

		isWhale := false
		detectionType := "UNKNOWN"

		// Calculate Statistical Metadata
		var zScore, volVsAvgPct float64

		// Get stats using helper method (handles caching internally)
		stats := h.getStockStats(t.Stock)

		if stats != nil && stats.MeanVolumeLots > 0 {
			// We have statistics, use Statistical Detection
			volVsAvgPct = (volumeLot / stats.MeanVolumeLots) * 100
			if stats.StdDevVolume > 0 {
				zScore = (volumeLot - stats.MeanVolumeLots) / stats.StdDevVolume
			}

			// Must satisfy Minimum Safety Value
			if totalAmount >= minSafeValue {
				// Primary: Z-Score threshold (Statistical Anomaly)
				if zScore >= zScoreThreshold {
					isWhale = true
					detectionType = "Z-SCORE ANOMALY"
				}

				// Secondary: Volume spike (Relative Volume Spike)
				if volumeLot >= (stats.MeanVolumeLots * volumeSpikeMultiplier) {
					isWhale = true
					if detectionType == "UNKNOWN" {
						detectionType = "RELATIVE VOL SPIKE"
					} else {
						detectionType += " & VOL SPIKE"
					}
				}
			}
		} else {
			// Fallback: No statistics available (New Listing / No History)
			// Use Hard Thresholds
			if volumeLot >= fallbackLotThreshold || totalAmount >= billionIDR {
				isWhale = true
				detectionType = "FALLBACK THRESHOLD"
			}
		}

		if isWhale {
			whaleAlert := &database.WhaleAlert{
				DetectedAt:        time.Now(),
				StockSymbol:       t.Stock,
				AlertType:         "SINGLE_TRADE",
				Action:            actionDb,
				TriggerPrice:      t.Price,
				TriggerVolumeLots: volumeLot,
				TriggerValue:      totalAmount,
				ConfidenceScore:   calculateConfidenceScore(zScore, volVsAvgPct, detectionType),
				MarketBoard:       boardType,
				ZScore:            ptr(zScore),
				VolumeVsAvgPct:    ptr(volVsAvgPct),
				AvgPrice:          ptr(stats.MeanPrice),
				// Populate pattern fields for context (Single Trade = Pattern of 1)
				PatternTradeCount:  ptrInt(1),
				TotalPatternVolume: ptr(volumeLot),
				TotalPatternValue:  ptr(totalAmount),
			}

			// Save whale alert to database
			if err := h.tradeRepo.SaveWhaleAlert(whaleAlert); err != nil {
				log.Printf("⚠️  Failed to save whale alert: %v", err)
			} else {
				// Prepare Price Info
				priceInfo := fmt.Sprintf("%.0f", t.Price)
				if stats.MeanPrice > 0 {
					diffPct := ((t.Price - stats.MeanPrice) / stats.MeanPrice) * 100
					priceInfo = fmt.Sprintf("%.0f (Avg: %.0f, %+0.1f%%)", t.Price, stats.MeanPrice, diffPct)
				}

				// Log whale detection to console
				log.Printf("🐋 WHALE ALERT! %s %s [%s] | Vol: %.0f (%.0f%% Avg) | Z-Score: %.2f | Value: %s | Price: %s",
					t.Stock, actionDb, detectionType, volumeLot, volVsAvgPct, zScore, helpers.FormatRupiah(totalAmount), priceInfo)

				// Trigger Webhook if manager is available
				if h.webhookManager != nil {
					h.webhookManager.SendAlert(whaleAlert)
				}

				// Broadcast Realtime Event
				if h.broker != nil && h.webhookManager != nil {
					// Use WebhookPayload for consistent frontend data (includes Message)
					payload := h.webhookManager.CreatePayload(whaleAlert)
					h.broker.Broadcast("whale_alert", payload)
				} else if h.broker != nil {
					// Fallback if no webhook manager
					h.broker.Broadcast("whale_alert", whaleAlert)
				}

				// Benchmark Latency
				latency := time.Since(startTime)
				log.Printf("⏱️ Detection Latency: %v", latency)
			}
		} else {
			// Not a single large trade, check for accumulation of smaller trades
			h.processAccumulation(t, actionDb, boardType, volumeLot, totalAmount, stats)
		}
	}
}

// processAccumulation checks if a series of small trades sums up to a whale alert
func (h *RunningTradeHandler) processAccumulation(t *pb.RunningTrade, actionDb, boardType string, volumeLot, totalAmount float64, stats *database.StockStats) {
	var alertToSend *database.WhaleAlert

	// CRITICAL SECTION: Lock, Calculate, Clean Buffer, Construct Alert
	func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		// 1. Add current trade to buffer
		now := time.Now()
		h.accumulationBuffer[t.Stock] = append(h.accumulationBuffer[t.Stock], TradeInfo{
			Timestamp: now,
			VolumeLot: volumeLot,
			Price:     t.Price,
			TotalVal:  totalAmount,
			Action:    actionDb,
		})

		// 2. Prune old trades
		validTrades := h.accumulationBuffer[t.Stock][:0]
		cutoff := now.Add(-accumulationWindow)

		var sumVolumeLot, sumTotalAmount float64
		var avgPriceWeighted float64

		for _, trade := range h.accumulationBuffer[t.Stock] {
			if trade.Timestamp.After(cutoff) {
				validTrades = append(validTrades, trade)
				sumVolumeLot += trade.VolumeLot
				sumTotalAmount += trade.TotalVal
				avgPriceWeighted += trade.Price * trade.VolumeLot
			}
		}
		h.accumulationBuffer[t.Stock] = validTrades

		// Minimum count to call it "accumulation" (avoid triggering on single small trade noise)
		if len(validTrades) < 3 {
			return
		}

		// Calculate weighted average price and determine majority action
		var buyVol, sellVol float64

		for _, tr := range validTrades {
			if tr.Action == "BUY" {
				buyVol += tr.VolumeLot
			} else if tr.Action == "SELL" {
				sellVol += tr.VolumeLot
			}
		}

		finalAction := actionDb // Default to current
		if buyVol > sellVol {
			finalAction = "BUY"
		} else if sellVol > buyVol {
			finalAction = "SELL"
		}

		if sumVolumeLot > 0 {
			avgPriceWeighted /= sumVolumeLot
		} else {
			avgPriceWeighted = t.Price
		}

		// 3. Check for Whale Status on the AGGREGATED stats
		isWhale := false
		detectionType := "UNKNOWN"
		var zScore, volVsAvgPct float64

		// Reuse existing detection logic but with SUMMED volume
		if stats != nil && stats.MeanVolumeLots > 0 {
			volVsAvgPct = (sumVolumeLot / stats.MeanVolumeLots) * 100
			if stats.StdDevVolume > 0 {
				zScore = (sumVolumeLot - stats.MeanVolumeLots) / stats.StdDevVolume
			}

			if sumTotalAmount >= minSafeValue {
				if zScore >= zScoreThreshold {
					isWhale = true
					detectionType = "ACCUMULATION Z-SCORE"
				}
				if sumVolumeLot >= (stats.MeanVolumeLots * volumeSpikeMultiplier) {
					isWhale = true
					if detectionType == "UNKNOWN" {
						detectionType = "ACCUMULATION VOL SPIKE"
					} else {
						detectionType += " & VOL SPIKE"
					}
				}
			}
		} else {
			// Fallback
			if sumVolumeLot >= fallbackLotThreshold || sumTotalAmount >= billionIDR {
				isWhale = true
				detectionType = "ACCUMULATION FALLBACK"
			}
		}

		// 4. Construct Alert if Accumulated
		if isWhale {
			alertToSend = &database.WhaleAlert{
				DetectedAt:         time.Now(),
				StockSymbol:        t.Stock,
				AlertType:          "RAPID_ACCUMULATION",
				Action:             finalAction,
				TriggerPrice:       avgPriceWeighted,
				TriggerVolumeLots:  sumVolumeLot,
				TriggerValue:       sumTotalAmount,
				ConfidenceScore:    calculateConfidenceScore(zScore, volVsAvgPct, detectionType),
				MarketBoard:        boardType,
				ZScore:             ptr(zScore),
				VolumeVsAvgPct:     ptr(volVsAvgPct),
				AvgPrice:           getAvgPricePtr(stats), // Safe helper: handles nil stats
				PatternTradeCount:  ptrInt(len(validTrades)),
				TotalPatternVolume: ptr(sumVolumeLot),
				TotalPatternValue:  ptr(sumTotalAmount),
			}

			// Clear buffer to avoid re-triggering immediately
			delete(h.accumulationBuffer, t.Stock)
		}
	}()

	// 5. I/O Operations (Outside Lock)
	if alertToSend != nil {
		// Save & Alert
		if h.tradeRepo != nil {
			if err := h.tradeRepo.SaveWhaleAlert(alertToSend); err != nil {
				log.Printf("⚠️  Failed to save accumulation alert: %v", err)
			}
		}

		// Log regardless of DB save
		log.Printf("🌊 ACCUMULATION WHALE! %s | Count: %d | Vol: %.0f | Val: %s",
			t.Stock, *alertToSend.PatternTradeCount, alertToSend.TriggerVolumeLots, helpers.FormatRupiah(alertToSend.TriggerValue))

		// Send notifications
		if h.webhookManager != nil {
			h.webhookManager.SendAlert(alertToSend)
		}
		if h.broker != nil && h.webhookManager != nil {
			payload := h.webhookManager.CreatePayload(alertToSend)
			h.broker.Broadcast("whale_alert", payload)
		}
	}
}

// ProcessOrderBookBody memproses update orderbook protobuf murni
func (h *RunningTradeHandler) ProcessOrderBookBody(ob *pb.OrderBookBody) {
	// Menampilkan orderbook dinonaktifkan agar console bersih
}

// GetMessageType returns the message type
func (h *RunningTradeHandler) GetMessageType() string {
	return "RunningTrade"
}

// calculateConfidenceScore computes confidence using continuous mathematical formula
// Returns a score from 40-100% with smooth progression based on Z-Score and volume
func calculateConfidenceScore(zScore, volVsAvgPct float64, detectionType string) float64 {
	// Fallback threshold (new stock, no historical data)
	if detectionType == "FALLBACK THRESHOLD" {
		return 40.0
	}

	// Continuous Z-Score component: Linear interpolation between key points
	// Formula: confidence = 70 + (zScore - 3.0) * 15
	// Z = 3.0 → 70%  (whale threshold)
	// Z = 4.0 → 85%  (very significant)
	// Z = 5.0 → 100% (extreme)
	zComponent := 70.0 + (zScore-3.0)*15.0

	// Cap at 100% for extreme Z-Scores
	if zComponent > 100.0 {
		zComponent = 100.0
	}

	// Floor at 50% for low Z-Scores (volume spike cases)
	if zComponent < 50.0 {
		zComponent = 50.0
	}

	// Volume bonus: Additional confidence for extreme volume spikes
	// Adds up to +10% for volumes >500%
	volumeBonus := 0.0
	if volVsAvgPct > 500.0 {
		// Linear bonus: 0% at 500%, +10% at 1000% and above
		volumeBonus = (volVsAvgPct - 500.0) / 50.0
		if volumeBonus > 10.0 {
			volumeBonus = 10.0
		}
	}

	// Final confidence = Z-Score component + Volume bonus
	confidence := zComponent + volumeBonus

	// Ensure final cap at 100%
	if confidence > 100.0 {
		confidence = 100.0
	}

	return confidence
}

// Helper function to create pointer
func ptr(v float64) *float64 {
	return &v
}

func ptrInt(v int) *int {
	return &v
}

// getAvgPricePtr safely retrieves average price, returns nil if stats unavailable
func getAvgPricePtr(stats *database.StockStats) *float64 {
	if stats == nil {
		return nil
	}
	return ptr(stats.MeanPrice)
}

// containsAny checks if string contains any of the substrings
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

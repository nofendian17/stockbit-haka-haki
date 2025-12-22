package handlers

import (
	"context"
	"fmt"
	"log"
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
}

// NewRunningTradeHandler membuat instance handler baru
func NewRunningTradeHandler(tradeRepo *database.TradeRepository, webhookManager *notifications.WebhookManager, redis *cache.RedisClient, broker *realtime.Broker) *RunningTradeHandler {
	return &RunningTradeHandler{
		tradeRepo:      tradeRepo,
		webhookManager: webhookManager,
		redis:          redis,
		broker:         broker,
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
		}

		if err := h.tradeRepo.SaveTrade(trade); err != nil {
			// Log error tapi jangan crash aplikasi
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
				ConfidenceScore:   100, // Simple threshold = 100% confidence
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

// Helper function to create pointer
func ptr(v float64) *float64 {
	return &v
}

func ptrInt(v int) *int {
	return &v
}

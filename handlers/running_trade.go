package handlers

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"stockbit-haka-haki/cache"
	"stockbit-haka-haki/database"
	"stockbit-haka-haki/database/types"
	"stockbit-haka-haki/helpers"
	"stockbit-haka-haki/notifications"
	pb "stockbit-haka-haki/proto"
	"stockbit-haka-haki/realtime"
)

// VolatilityProvider interface allows fetching volatility metrics (ATR%)
// Used to adjust detection thresholds dynamically
type VolatilityProvider interface {
	GetVolatilityPercent(symbol string) (float64, error)
}

// Detection thresholds
const (
	minSafeValue          = 100_000_000.0   // 100 Million IDR - Safety floor to avoid penny stock noise
	billionIDR            = 1_000_000_000.0 // 1 Billion IDR
	zScoreThreshold       = 3.0             // Statistical anomaly threshold
	volumeSpikeMultiplier = 5.0             // 5x average volume
	fallbackLotThreshold  = 2500            // Fallback threshold for lots (for stocks without historical data)
	statsLookbackMinutes  = 60              // 1 hour lookback for statistics
	statsCacheDuration    = 5 * time.Minute // Cache stats for 5 minutes
)

// Cache key prefixes
const (
	cacheKeyStatsPrefix = "stats:stock:"
)

// Config constants
const (
	tradeChanSize   = 10000
	whaleChanSize   = 1000
	batchSize       = 500
	batchTimeout    = 500 * time.Millisecond
	whaleWorkerPool = 5
)

// RunningTradeHandler mengelola pesan RunningTrade dari protobuf
type RunningTradeHandler struct {
	tradeRepo      *database.TradeRepository     // Repository untuk menyimpan data trade
	webhookManager *notifications.WebhookManager // Manager untuk notifikasi webhook
	redis          *cache.RedisClient            // Redis client for config caching
	broker         *realtime.Broker              // Realtime SSE broker
	volatilityProv VolatilityProvider            // Provider for adaptive thresholds

	// Async Processing Channels
	ingestChan chan *database.Trade
	whaleChan  chan *database.Trade
	done       chan struct{}

	// Order Flow Aggregation (Phase 1 Enhancement)
	flowAggregator *OrderFlowAggregator
}

// OrderFlowAggregator aggregates buy/sell volume per minute
type OrderFlowAggregator struct {
	repo          *database.TradeRepository
	currentBucket time.Time
	flows         map[string]*OrderFlowData // key: stock_symbol
	mu            sync.RWMutex
	inputChan     chan *orderFlowInput
}

type orderFlowInput struct {
	stock      string
	action     string
	volumeLots float64
	value      float64
}

// OrderFlowData holds aggregated order flow for a stock
type OrderFlowData struct {
	StockSymbol    string
	BuyVolumeLots  float64
	SellVolumeLots float64
	BuyTradeCount  int
	SellTradeCount int
	BuyValue       float64
	SellValue      float64
}

// NewRunningTradeHandler membuat instance handler baru
func NewRunningTradeHandler(tradeRepo *database.TradeRepository, webhookManager *notifications.WebhookManager, redis *cache.RedisClient, broker *realtime.Broker, volProv VolatilityProvider) *RunningTradeHandler {
	handler := &RunningTradeHandler{
		tradeRepo:      tradeRepo,
		webhookManager: webhookManager,
		redis:          redis,
		broker:         broker,
		volatilityProv: volProv,
		ingestChan:     make(chan *database.Trade, tradeChanSize),
		whaleChan:      make(chan *database.Trade, whaleChanSize),
		done:           make(chan struct{}),
	}

	// Initialize order flow aggregator
	if tradeRepo != nil {
		handler.flowAggregator = NewOrderFlowAggregator(tradeRepo)
		go handler.flowAggregator.Start() // Start background aggregation
	}

	// Start workers
	go handler.batchSaverWorker()
	for i := 0; i < whaleWorkerPool; i++ {
		go handler.whaleDetectionWorker()
	}

	return handler
}

// batchSaverWorker handles batch insertion of trades
func (h *RunningTradeHandler) batchSaverWorker() {
	var batch []*database.Trade
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	flush := func() {
		if len(batch) > 0 {
			if h.tradeRepo != nil {
				if err := h.tradeRepo.BatchSaveTrades(batch); err != nil {
					log.Printf("⚠️  Failed to batch save trades: %v", err)
				}
			}
			batch = nil
		}
	}

	for {
		select {
		case trade := <-h.ingestChan:
			batch = append(batch, trade)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-h.done:
			flush()
			return
		}
	}
}

// whaleDetectionWorker processes trades for whale alerts
func (h *RunningTradeHandler) whaleDetectionWorker() {
	for trade := range h.whaleChan {
		h.detectWhale(trade)
	}
}

// Close gracefully shuts down the handler
func (h *RunningTradeHandler) Close() {
	close(h.done)
	close(h.whaleChan) // ingestChan is not closed to avoid panic on send, but loop above has simple exit
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
func (h *RunningTradeHandler) getStockStats(stock string) *types.StockStats {
	if h.redis == nil && h.tradeRepo == nil {
		return nil
	}

	cacheKey := cacheKeyStatsPrefix + stock
	stats := &types.StockStats{}

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

	// 1. Send to Batch Saver (Non-blocking if buffered)
	select {
	case h.ingestChan <- trade:
	default:
		log.Printf("⚠️ Ingest channel full, dropping trade for %s", trade.StockSymbol)
	}

	// 2. Send to Whale Detector (Non-blocking)
	select {
	case h.whaleChan <- trade:
	default:
		// Drop is acceptable for whale detection under extreme load
	}

	// 3. Send to Order Flow Aggregator (Non-blocking)
	if h.flowAggregator != nil {
		h.flowAggregator.inputChan <- &orderFlowInput{
			stock:      t.Stock,
			action:     actionDb,
			volumeLots: volumeLot,
			value:      totalAmount,
		}
	}

	// 4. Broadcast to Frontend (Realtime SSE)
	if h.broker != nil {
		// Calculate duration if stats available (or just send basic info)
		// We'll send a lightweight payload for frontend
		payload := map[string]interface{}{
			"symbol":     t.Stock,
			"action":     actionDb,
			"price":      t.Price,
			"volume_lot": volumeLot,
			"value":      totalAmount,
			"board":      boardType,
			"time":       trade.Timestamp,
			"change_pct": changePercentage, // can be nil
			"trade_num":  tradeNumber,      // can be nil
		}

		h.broker.Broadcast("trade", payload)
	}
}

// detectWhale performs the whale detection logic directly (now async)
func (h *RunningTradeHandler) detectWhale(trade *database.Trade) {
	// Start benchmarking timer
	startTime := time.Now()

	isWhale := false
	detectionType := "UNKNOWN"

	// Calculate Statistical Metadata
	var zScore, volVsAvgPct float64

	// ADAPTIVE THRESHOLD VARIABLES (Function Scope)
	adaptiveThreshold := zScoreThreshold
	atrPct := 0.0

	// Get stats using helper method (handles caching internally)
	stats := h.getStockStats(trade.StockSymbol)

	if stats != nil && stats.MeanVolumeLots > 0 {
		// We have statistics, use Statistical Detection
		volVsAvgPct = (trade.VolumeLot / stats.MeanVolumeLots) * 100
		if stats.StdDevVolume > 0 {
			zScore = (trade.VolumeLot - stats.MeanVolumeLots) / stats.StdDevVolume
		}

		// Must satisfy Minimum Safety Value
		if trade.TotalAmount >= minSafeValue {
			// ADAPTIVE THRESHOLD LOGIC
			// Get volatility context if provider available
			if h.volatilityProv != nil {
				if vol, err := h.volatilityProv.GetVolatilityPercent(trade.StockSymbol); err == nil {
					atrPct = vol
					if vol > 1.5 {
						// High volatility -> Increase threshold to reduce noise
						adaptiveThreshold = 3.5
					} else if vol < 0.5 && vol > 0 {
						// Low volatility -> Decrease threshold (more sensitive)
						adaptiveThreshold = 2.5
					}
				}
			}

			// Primary: Z-Score threshold (Statistical Anomaly)
			if zScore >= adaptiveThreshold {
				isWhale = true
				detectionType = "Z-SCORE ANOMALY"
			}

			// Secondary: Volume spike (Relative Volume Spike)
			if trade.VolumeLot >= (stats.MeanVolumeLots * volumeSpikeMultiplier) {
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
		// Use Hard Thresholds with minimum value safety floor
		// Require: (High Volume AND Min Value) OR (Very High Value)
		if trade.TotalAmount >= minSafeValue {
			if trade.VolumeLot >= fallbackLotThreshold || trade.TotalAmount >= billionIDR {
				isWhale = true
				detectionType = "FALLBACK THRESHOLD"
			}
		}
	}

	if isWhale {
		whaleAlert := &database.WhaleAlert{
			DetectedAt:        time.Now(),
			StockSymbol:       trade.StockSymbol,
			AlertType:         "SINGLE_TRADE",
			Action:            trade.Action,
			TriggerPrice:      trade.Price,
			TriggerVolumeLots: trade.VolumeLot,
			TriggerValue:      trade.TotalAmount,
			ConfidenceScore:   calculateConfidenceScore(zScore, volVsAvgPct, detectionType),
			MarketBoard:       trade.MarketBoard,
			ZScore:            ptr(zScore),
			VolumeVsAvgPct:    ptr(volVsAvgPct),
			AvgPrice:          getAvgPricePtr(stats),
			// Populate pattern fields for context (Single Trade = Pattern of 1)
			PatternTradeCount:  ptrInt(1),
			TotalPatternVolume: ptr(trade.VolumeLot),
			TotalPatternValue:  ptr(trade.TotalAmount),
			// Adaptive Threshold Tracking
			AdaptiveThreshold: ptr(adaptiveThreshold),
			VolatilityPct:     ptr(atrPct),
		}

		// Save whale alert to database
		if err := h.tradeRepo.SaveWhaleAlert(whaleAlert); err != nil {
			log.Printf("⚠️  Failed to save whale alert: %v", err)
		} else {
			// Prepare Price Info
			priceInfo := fmt.Sprintf("%.0f", trade.Price)
			if stats != nil && stats.MeanPrice > 0 {
				diffPct := ((trade.Price - stats.MeanPrice) / stats.MeanPrice) * 100
				priceInfo = fmt.Sprintf("%.0f (Avg: %.0f, %+0.1f%%)", trade.Price, stats.MeanPrice, diffPct)
			}

			// Log whale detection to console
			log.Printf("🐋 WHALE ALERT! %s %s [%s] | Vol: %.0f (%.0f%% Avg) | Z-Score: %.2f | Value: %s | Price: %s",
				trade.StockSymbol, trade.Action, detectionType, trade.VolumeLot, volVsAvgPct, zScore, helpers.FormatRupiah(trade.TotalAmount), priceInfo)

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
func getAvgPricePtr(stats *types.StockStats) *float64 {
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

// ============================================================================
// Order Flow Aggregation Implementation (Phase 1 Enhancement)
// ============================================================================

// NewOrderFlowAggregator creates a new order flow aggregator
func NewOrderFlowAggregator(repo *database.TradeRepository) *OrderFlowAggregator {
	return &OrderFlowAggregator{
		repo:          repo,
		currentBucket: time.Now().Truncate(time.Minute),
		flows:         make(map[string]*OrderFlowData),
		inputChan:     make(chan *orderFlowInput, tradeChanSize),
	}
}

// Start begins the aggregation loop
func (ofa *OrderFlowAggregator) Start() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Println("📊 Order Flow Aggregator started")

	for {
		select {
		case input := <-ofa.inputChan:
			ofa.processInput(input)
		case <-ticker.C:
			ofa.flushAndReset()
		}
	}
}

// processInput adds a trade to the current minute's aggregation (called from consumer loop)
func (ofa *OrderFlowAggregator) processInput(input *orderFlowInput) {
	// No mutex needed here as we are in a single consumer loop
	// Get or create flow data for this stock
	flow, exists := ofa.flows[input.stock]
	if !exists {
		flow = &OrderFlowData{
			StockSymbol: input.stock,
		}
		ofa.flows[input.stock] = flow
	}

	// Aggregate based on action
	switch input.action {
	case "BUY":
		flow.BuyVolumeLots += input.volumeLots
		flow.BuyValue += input.value
		flow.BuyTradeCount++
	case "SELL":
		flow.SellVolumeLots += input.volumeLots
		flow.SellValue += input.value
		flow.SellTradeCount++
	}
}

// AddTrade is now deprecated/unused as we use inputChan directly,
// but kept for interface compatibility if needed, redirecting to channel
func (ofa *OrderFlowAggregator) AddTrade(stock, action string, volumeLots, value float64) {
	select {
	case ofa.inputChan <- &orderFlowInput{
		stock:      stock,
		action:     action,
		volumeLots: volumeLots,
		value:      value,
	}:
	default:
		// Drop order flow update under heavy load
	}
}

// flushAndReset persists current bucket and resets for next minute
func (ofa *OrderFlowAggregator) flushAndReset() {
	// Save current bucket and flows
	bucket := ofa.currentBucket
	flows := ofa.flows

	// Reset for next minute
	ofa.currentBucket = time.Now().Truncate(time.Minute)
	ofa.flows = make(map[string]*OrderFlowData)

	// Persist to database (async in separate goroutine to not block consumer)
	if len(flows) > 0 {
		go ofa.persistFlows(bucket, flows)
	}
}

// persistFlows saves aggregated flows to database
func (ofa *OrderFlowAggregator) persistFlows(bucket time.Time, flows map[string]*OrderFlowData) {
	if len(flows) == 0 {
		return
	}

	saved := 0
	for _, flow := range flows {
		// Calculate imbalance ratios
		totalVolume := flow.BuyVolumeLots + flow.SellVolumeLots
		totalValue := flow.BuyValue + flow.SellValue

		volumeImbalance := 0.0
		valueImbalance := 0.0

		if totalVolume > 0 {
			volumeImbalance = (flow.BuyVolumeLots - flow.SellVolumeLots) / totalVolume
		}
		if totalValue > 0 {
			valueImbalance = (flow.BuyValue - flow.SellValue) / totalValue
		}

		deltaVolume := flow.BuyVolumeLots - flow.SellVolumeLots

		// Create database record
		flowDB := &database.OrderFlowImbalance{
			Bucket:               bucket,
			StockSymbol:          flow.StockSymbol,
			BuyVolumeLots:        flow.BuyVolumeLots,
			SellVolumeLots:       flow.SellVolumeLots,
			BuyTradeCount:        flow.BuyTradeCount,
			SellTradeCount:       flow.SellTradeCount,
			BuyValue:             flow.BuyValue,
			SellValue:            flow.SellValue,
			VolumeImbalanceRatio: volumeImbalance,
			ValueImbalanceRatio:  valueImbalance,
			DeltaVolume:          deltaVolume,
		}

		if err := ofa.repo.SaveOrderFlowImbalance(flowDB); err != nil {
			log.Printf("⚠️  Failed to save order flow for %s: %v", flow.StockSymbol, err)
		} else {
			saved++
		}
	}

	if saved > 0 {
		log.Printf("✅ Order flow: saved %d symbols for bucket %s", saved, bucket.Format("15:04"))
	}
}

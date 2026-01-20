package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// shouldCallLLM determines if we should call LLM for a symbol based on pre-filtering criteria
// Returns (shouldCall bool, reason string)
func (st *SignalTracker) shouldCallLLM(ctx context.Context, symbol string) (bool, string) {
	// 1. Check LLM cooldown (prevent excessive calls for same symbol)
	if st.redis != nil {
		cooldownKey := fmt.Sprintf("llm:cooldown:%s", symbol)
		var timestamp int64
		if err := st.redis.Get(ctx, cooldownKey, &timestamp); err == nil && timestamp > 0 {
			return false, fmt.Sprintf("in cooldown (expires in %d min)", st.cfg.Trading.LLMCooldownMinutes)
		}
	}

	// 2. Get recent trade statistics for the symbol (last 60 minutes)
	recentTrades, err := st.repo.GetRecentTrades(symbol, 1000, "")
	if err != nil || len(recentTrades) == 0 {
		return false, "no recent trade data"
	}

	// Calculate volume and value from recent trades
	var totalVolumeLots float64
	var totalValue float64
	var firstPrice, lastPrice float64

	for i, trade := range recentTrades {
		if i == 0 {
			firstPrice = trade.Price
		}
		if i == len(recentTrades)-1 {
			lastPrice = trade.Price
		}
		totalVolumeLots += trade.VolumeLot
		totalValue += trade.Price * trade.VolumeLot * 100 // Convert lots to shares
	}

	// 3. Check minimum volume threshold
	if totalVolumeLots < float64(st.cfg.Trading.MinVolumeForLLM) {
		return false, fmt.Sprintf("volume %.0f lots < %d lots threshold", totalVolumeLots, st.cfg.Trading.MinVolumeForLLM)
	}

	// 4. Check minimum value threshold
	if totalValue < st.cfg.Trading.MinValueForLLM {
		return false, fmt.Sprintf("value %.0fM < %.0fM threshold", totalValue/1000000, st.cfg.Trading.MinValueForLLM/1000000)
	}

	// 5. Check minimum price change threshold
	if firstPrice > 0 {
		priceChangePct := ((lastPrice - firstPrice) / firstPrice) * 100
		if priceChangePct < st.cfg.Trading.MinPriceChangeForLLM && priceChangePct > -st.cfg.Trading.MinPriceChangeForLLM {
			return false, fmt.Sprintf("price change %.2f%% < %.1f%% threshold", priceChangePct, st.cfg.Trading.MinPriceChangeForLLM)
		}
	}

	// 6. Check market regime - AVOID VOLATILE, PRIORITIZE TRENDING
	regime, err := st.repo.GetLatestRegime(symbol)
	if err == nil && regime != nil {
		// Skip volatile stocks (high risk, unreliable signals)
		if regime.Regime == "VOLATILE" && regime.Confidence > 0.7 {
			return false, fmt.Sprintf("high volatility regime (%.1f%% confidence)", regime.Confidence*100)
		}

		// Log trending stocks (for monitoring)
		if regime.Regime == "TRENDING_UP" && regime.Confidence > 0.7 {
			log.Printf("📈 Prioritizing trending stock %s (conf: %.2f)", symbol, regime.Confidence)
		}
	}

	return true, ""
}

// generateSignals generates new trading signals from multiple sources including LLM analysis
func (st *SignalTracker) generateSignals() {
	// Get active symbols from recent trades (last 60 minutes)
	activeSymbols, err := st.repo.GetActiveSymbols(time.Now().Add(-60 * time.Minute))
	if err != nil {
		log.Printf("❌ Error fetching active symbols: %v", err)
		return
	}

	if len(activeSymbols) == 0 {
		return
	}

	log.Printf("📊 Processing %d active symbols for signal generation...", len(activeSymbols))

	generated := 0

	// Process each active symbol for LLM-based signals with optimization
	llmCallCount := 0
	llmCacheHits := 0
	llmFiltered := 0

	for _, symbol := range activeSymbols {
		ctx := context.Background()

		// PRE-FILTERING: Check if symbol meets minimum thresholds before calling LLM
		shouldCallLLM, reason := st.shouldCallLLM(ctx, symbol)
		if !shouldCallLLM {
			llmFiltered++
			if llmFiltered <= 3 { // Log first 3 filtered symbols to avoid spam
				log.Printf("⏭️ Skipping LLM for %s: %s", symbol, reason)
			}
			continue
		}

		// Generate LLM-based tape reading signal
		// Using 1-hour window for robust trend analysis (Investment Manager Persona)
		beforeCall := time.Now()
		llmSignal, err := st.tradeAgg.GenerateTradingSignal(ctx, symbol, 60*time.Minute)
		if err != nil {
			log.Printf("⚠️ Failed to generate LLM signal for %s: %v", symbol, err)
			continue
		}

		// Track if this was a cache hit (signal generated very quickly)
		callDuration := time.Since(beforeCall)
		if callDuration < 100*time.Millisecond {
			llmCacheHits++
		} else {
			llmCallCount++
		}

		// Apply regime-adaptive confidence threshold
		minConfidence := st.getAdaptiveConfidenceThreshold(symbol)
		if llmSignal != nil && llmSignal.Decision != "WAIT" && llmSignal.Confidence >= minConfidence {
			// Check if similar signal already exists in last 60 minutes
			recentSignals, err := st.repo.GetTradingSignals(symbol, "LLM_TAPE_READING", "",
				time.Now().Add(-60*time.Minute), time.Now(), 10, 0)
			if err == nil && len(recentSignals) > 0 {
				continue
			}

			// Save LLM signal
			if err := st.repo.SaveTradingSignal(llmSignal); err != nil {
				log.Printf("❌ Error saving LLM signal for %s: %v", symbol, err)
			} else {
				generated++
				log.Printf("✅ Generated LLM_TAPE_READING signal for %s (Conf: %.2f, Decision: %s)",
					symbol, llmSignal.Confidence, llmSignal.Decision)

				// Redis Broadcasting & Optimization
				if st.redis != nil {
					// 1. Set LLM Cooldown (prevent calling LLM again for this symbol too soon)
					llmCooldownKey := fmt.Sprintf("llm:cooldown:%s", symbol)
					st.redis.Set(ctx, llmCooldownKey, time.Now().Unix(), time.Duration(st.cfg.Trading.LLMCooldownMinutes)*time.Minute)

					// 2. Publish signal event
					if err := st.redis.Publish(ctx, "signals:new", llmSignal); err != nil {
						log.Printf("⚠️ Failed to publish signal to Redis: %v", err)
					}
					// 3. Set Cooldown (15 min)
					cooldownKey := fmt.Sprintf("signal:cooldown:%s:%s", symbol, "LLM_TAPE_READING")
					st.redis.Set(ctx, cooldownKey, llmSignal.ID, 15*time.Minute)
					// 4. Set Recent (5 min)
					recentKey := fmt.Sprintf("signal:recent:%s", symbol)
					st.redis.Set(ctx, recentKey, llmSignal.ID, 5*time.Minute)
				}
			}
		}
	}

	// Log optimization metrics
	if llmFiltered > 0 || llmCacheHits > 0 {
		log.Printf("📊 LLM Optimization: %d filtered, %d cache hits, %d actual calls (from %d symbols)",
			llmFiltered, llmCacheHits, llmCallCount, len(activeSymbols))
	}

	// Also generate traditional signals from whale alerts
	calculatedSignals, err := st.repo.GetStrategySignals(60, 0.3, "ALL")
	if err != nil {
		log.Printf("❌ Error calculating traditional signals: %v", err)
		return
	}

	if len(calculatedSignals) > 0 {
		// Filter duplicates and save traditional signals
		signalsToSave := st.filterDuplicateSignals(calculatedSignals)
		for _, signal := range signalsToSave {
			dbSignal := &database.TradingSignalDB{
				GeneratedAt:       signal.Timestamp,
				StockSymbol:       signal.StockSymbol,
				Strategy:          signal.Strategy,
				Decision:          signal.Decision,
				Confidence:        signal.Confidence,
				TriggerPrice:      signal.Price,
				TriggerVolumeLots: signal.Volume,
				PriceZScore:       signal.PriceZScore,
				VolumeZScore:      signal.VolumeZScore,
				PriceChangePct:    signal.Change,
				Reason:            signal.Reason,
				AnalysisData:      "{}",
			}

			if err := st.repo.SaveTradingSignal(dbSignal); err != nil {
				log.Printf("❌ Error saving traditional signal: %v", err)
			} else {
				generated++

				// Redis Broadcasting for traditional signals
				if st.redis != nil {
					ctx := context.Background()
					st.redis.Publish(ctx, "signals:new", dbSignal)
					cooldownKey := fmt.Sprintf("signal:cooldown:%s:%s", signal.StockSymbol, signal.Strategy)
					st.redis.Set(ctx, cooldownKey, dbSignal.ID, 15*time.Minute)
					recentKey := fmt.Sprintf("signal:recent:%s", signal.StockSymbol)
					st.redis.Set(ctx, recentKey, dbSignal.ID, 5*time.Minute)
				}
			}
		}
	}

	if generated > 0 {
		log.Printf("📊 Signal generation completed: %d total signals generated", generated)
	}
}

// filterDuplicateSignals removes signals that have already been saved
// Uses Redis batch check for performance (O(1) instead of O(N) database queries)
func (st *SignalTracker) filterDuplicateSignals(signals []database.TradingSignal) []database.TradingSignal {
	if st.redis == nil {
		// Fallback: use database check (slower but works without Redis)
		return st.filterDuplicateSignalsDB(signals)
	}

	ctx := context.Background()

	// Build cache keys for batch check
	cacheKeys := make([]string, len(signals))
	for i, signal := range signals {
		cacheKeys[i] = fmt.Sprintf("signal:saved:%s:%s:%d",
			signal.StockSymbol,
			signal.Strategy,
			signal.Timestamp.Unix(),
		)
	}

	// Batch check using MGet (single Redis call)
	var existingIDs []int64
	if err := st.redis.MGet(ctx, cacheKeys, &existingIDs); err != nil {
		log.Printf("⚠️ Redis MGet failed, falling back to DB check: %v", err)
		return st.filterDuplicateSignalsDB(signals)
	}

	// Filter out existing signals
	var newSignals []database.TradingSignal
	for i, signal := range signals {
		if i < len(existingIDs) && existingIDs[i] == 0 {
			// Signal not found in cache = new signal
			newSignals = append(newSignals, signal)
		}
	}

	if len(signals) > len(newSignals) {
		log.Printf("🔍 Filtered %d duplicate signals using Redis cache", len(signals)-len(newSignals))
	}

	return newSignals
}

// filterDuplicateSignalsDB is the fallback method using database queries
func (st *SignalTracker) filterDuplicateSignalsDB(signals []database.TradingSignal) []database.TradingSignal {
	var newSignals []database.TradingSignal

	for _, signal := range signals {
		// Check if signal already exists in DB to prevent duplicates
		existingSignals, err := st.repo.GetTradingSignals(
			signal.StockSymbol,
			signal.Strategy,
			signal.Decision,
			signal.Timestamp,
			signal.Timestamp,
			1,
			0, // Offset
		)

		if err == nil && len(existingSignals) == 0 {
			newSignals = append(newSignals, signal)
		}
	}

	return newSignals
}

// getAdaptiveConfidenceThreshold returns confidence threshold based on market regime
func (st *SignalTracker) getAdaptiveConfidenceThreshold(symbol string) float64 {
	regime, err := st.repo.GetLatestRegime(symbol)
	if err != nil || regime == nil {
		return st.cfg.Trading.MinLLMConfidence // 0.6 default
	}
	
	switch regime.Regime {
	case "TRENDING_UP":
		if regime.Confidence > 0.7 {
			return 0.5 // Relax for strong uptrends
		}
	case "VOLATILE":
		return 0.75 // Strict for volatile stocks
	case "TRENDING_DOWN":
		return 0.7 // Stricter for downtrends (we only trade BUY)
	}
	
	return st.cfg.Trading.MinLLMConfidence // 0.6 default
}

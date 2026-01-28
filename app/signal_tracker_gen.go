package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// generateSignals generates new trading signals from multiple sources
func (st *SignalTracker) generateSignals() {
	generated := 0
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

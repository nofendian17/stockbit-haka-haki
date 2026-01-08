package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// generateSignals fetches recent whale alerts and triggers signal generation
func (st *SignalTracker) generateSignals() {
	// Look back 60 minutes for signals, with minimum confidence 0.3
	calculatedSignals, err := st.repo.GetStrategySignals(60, 0.3, "ALL")
	if err != nil {
		log.Printf("❌ Error calculating signals: %v", err)
		return
	}

	if len(calculatedSignals) == 0 {
		return
	}

	// OPTIMIZATION: Batch duplicate check using Redis (Quick Win #2)
	signalsToSave := st.filterDuplicateSignals(calculatedSignals)

	savedCount := 0
	for _, signal := range signalsToSave {
		// Save to database for history
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
			AnalysisData:      "{}", // Initialize with empty JSON object to prevent DB error
		}

		// Generate Analysis Data for ML (Scorecard & Features)
		if st.scorecardEval != nil {
			scorecard := st.scorecardEval.EvaluateSignal(dbSignal) // Now using correct type
			if jsonBytes, err := json.Marshal(scorecard); err == nil {
				dbSignal.AnalysisData = string(jsonBytes)
				log.Printf("✅ Generated analysis_data for %s %s: %d bytes", signal.StockSymbol, signal.Strategy, len(jsonBytes))
			} else {
				log.Printf("⚠️ Failed to marshal scorecard for %s: %v", signal.StockSymbol, err)
			}
		} else {
			log.Printf("⚠️ Scorecard evaluator is nil - no analysis_data generated for %s", signal.StockSymbol)
		}

		if err := st.repo.SaveTradingSignal(dbSignal); err != nil {
			log.Printf("❌ Error saving generated signal: %v", err)
		} else {
			savedCount++

			// Redis Broadcasting & Optimization
			if st.redis != nil {
				ctx := context.Background()

				// 1. Publish signal event
				if err := st.redis.Publish(ctx, "signals:new", dbSignal); err != nil {
					log.Printf("⚠️ Failed to publish signal to Redis: %v", err)
				} else {
					log.Printf("📡 Published new signal to Redis: %s %s", signal.StockSymbol, signal.Strategy)
				}

				// 2. Set Cooldown (15 min)
				cooldownKey := fmt.Sprintf("signal:cooldown:%s:%s", signal.StockSymbol, signal.Strategy)
				if err := st.redis.Set(ctx, cooldownKey, dbSignal.ID, 15*time.Minute); err != nil {
					log.Printf("⚠️ Failed to set cooldown in Redis: %v", err)
				}

				// 3. Set Recent (5 min) - General symbol activity
				recentKey := fmt.Sprintf("signal:recent:%s", signal.StockSymbol)
				if err := st.redis.Set(ctx, recentKey, dbSignal.ID, 5*time.Minute); err != nil {
					log.Printf("⚠️ Failed to set recent activity in Redis: %v", err)
				}

				// 4. Mark signal as saved (for duplicate prevention)
				savedKey := fmt.Sprintf("signal:saved:%s:%s:%d", signal.StockSymbol, signal.Strategy, signal.Timestamp.Unix())
				if err := st.redis.Set(ctx, savedKey, dbSignal.ID, 24*time.Hour); err != nil {
					log.Printf("⚠️ Failed to mark signal as saved: %v", err)
				}
			}
		}
	}

	if savedCount > 0 {
		log.Printf("📊 Generated and saved %d new trading signals", savedCount)
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
		)

		if err == nil && len(existingSignals) == 0 {
			newSignals = append(newSignals, signal)
		}
	}

	return newSignals
}

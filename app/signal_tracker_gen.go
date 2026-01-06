package app

import (
	"context"
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

	savedCount := 0
	for _, signal := range calculatedSignals {
		// Check if signal already exists in DB to prevent duplicates
		existingSignals, err := st.repo.GetTradingSignals(signal.StockSymbol, signal.Strategy, signal.Decision, signal.Timestamp, signal.Timestamp, 1)
		if err == nil && len(existingSignals) == 0 {
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
			}

			if err := st.repo.SaveTradingSignal(dbSignal); err != nil {
				log.Printf("❌ Error saving generated signal: %v", err)
			} else {
				savedCount++

				// Redis Broadcasing & Optimization
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
				}
			}
		}
	}

	if savedCount > 0 {
		log.Printf("📊 Generated and saved %d new trading signals", savedCount)
	}
}

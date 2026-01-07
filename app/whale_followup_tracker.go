package app

import (
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// WhaleFollowupTracker monitors whale alerts and tracks price movements
type WhaleFollowupTracker struct {
	repo *database.TradeRepository
	done chan bool
}

// NewWhaleFollowupTracker creates a new whale followup tracker
func NewWhaleFollowupTracker(repo *database.TradeRepository) *WhaleFollowupTracker {
	return &WhaleFollowupTracker{
		repo: repo,
		done: make(chan bool),
	}
}

// Start begins the whale followup tracking loop
func (wt *WhaleFollowupTracker) Start() {
	log.Println("🐋 Whale Followup Tracker started")

	ticker := time.NewTicker(1 * time.Minute) // Run every minute
	defer ticker.Stop()

	// Run immediately on start
	wt.trackWhaleFollowups()

	for {
		select {
		case <-ticker.C:
			wt.trackWhaleFollowups()
		case <-wt.done:
			log.Println("🐋 Whale Followup Tracker stopped")
			return
		}
	}
}

// Stop gracefully stops the tracker
func (wt *WhaleFollowupTracker) Stop() {
	close(wt.done)
}

// trackWhaleFollowups processes whale alerts and updates followup data
func (wt *WhaleFollowupTracker) trackWhaleFollowups() {
	// Get pending followups (last 24 hours)
	followups, err := wt.repo.GetPendingFollowups(24 * time.Hour)
	if err != nil {
		log.Printf("❌ Error getting pending followups: %v", err)
		return
	}

	// Always check for new whale alerts
	wt.createNewFollowups()

	if len(followups) == 0 {
		return
	}

	updated := 0
	skipped := 0
	for _, followup := range followups {
		if err := wt.updateFollowup(&followup); err != nil {
			if err.Error() == "no update needed" {
				skipped++
			} else {
				log.Printf("❌ Error updating followup for alert %d (%s): %v", followup.WhaleAlertID, followup.StockSymbol, err)
			}
		} else {
			updated++
		}
	}

	if updated > 0 {
		log.Printf("✅ Whale followup: %d updated, %d skipped (total pending: %d)", updated, skipped, len(followups))
	}
}

// createNewFollowups creates followup records for recent whale alerts
func (wt *WhaleFollowupTracker) createNewFollowups() {
	// Get recent whale alerts (last 5 minutes)
	startTime := time.Now().Add(-5 * time.Minute)
	endTime := time.Now()

	alerts, err := wt.repo.GetHistoricalWhales("", startTime, endTime, "", "", "", 0, 50, 0)
	if err != nil {
		log.Printf("❌ Error getting recent whale alerts: %v", err)
		return
	}

	created := 0
	for _, alert := range alerts {
		// Check if followup already exists
		existing, err := wt.repo.GetWhaleFollowup(alert.ID)
		if err != nil {
			log.Printf("❌ Error checking followup for alert %d: %v", alert.ID, err)
			continue
		}

		if existing == nil {
			// Create new followup record
			followup := &database.WhaleAlertFollowup{
				WhaleAlertID: alert.ID,
				StockSymbol:  alert.StockSymbol,
				AlertTime:    alert.DetectedAt,
				AlertPrice:   alert.TriggerPrice,
				AlertAction:  alert.Action,
			}

			if err := wt.repo.SaveWhaleFollowup(followup); err != nil {
				log.Printf("❌ Error creating followup for alert %d: %v", alert.ID, err)
			} else {
				created++
			}
		}
	}

	if created > 0 {
		log.Printf("✅ Whale followup: %d created", created)
	}
}

// updateFollowup updates price data for a whale alert followup
func (wt *WhaleFollowupTracker) updateFollowup(followup *database.WhaleAlertFollowup) error {
	elapsed := time.Since(followup.AlertTime)

	// Get current price from latest trades (more reliable than candle view)
	// Try to get from 1min candle first (aggregated data)
	var currentPrice float64
	var currentVolume float64

	candle, err := wt.repo.GetLatestCandle(followup.StockSymbol)
	if err == nil && candle != nil && candle.Close > 0 {
		currentPrice = candle.Close
		currentVolume = candle.VolumeLots
	} else {
		// Fallback: Get latest trade price directly from running_trades
		trades, err := wt.repo.GetRecentTrades(followup.StockSymbol, 1, "")
		if err != nil || len(trades) == 0 {
			// No recent data available, skip this update
			return nil
		}
		currentPrice = trades[0].Price
		currentVolume = trades[0].VolumeLot
	}

	// Validate price
	if currentPrice <= 0 {
		return nil // Skip invalid price
	}

	// Calculate price change percentage
	priceChange := ((currentPrice - followup.AlertPrice) / followup.AlertPrice) * 100

	// Prepare updates map
	updates := make(map[string]interface{})

	// Update based on elapsed time
	if elapsed >= 1*time.Minute && followup.Price1MinLater == nil {
		updates["price_1min_later"] = currentPrice
		updates["change_1min_pct"] = priceChange
		updates["volume_1min_later"] = currentVolume
	}

	if elapsed >= 5*time.Minute && followup.Price5MinLater == nil {
		updates["price_5min_later"] = currentPrice
		updates["change_5min_pct"] = priceChange
		updates["volume_5min_later"] = currentVolume

		// Classify immediate impact (based on 5min change)
		impact := wt.classifyImpact(priceChange, followup.AlertAction)
		updates["immediate_impact"] = impact
	}

	if elapsed >= 15*time.Minute && followup.Price15MinLater == nil {
		updates["price_15min_later"] = currentPrice
		updates["change_15min_pct"] = priceChange
		updates["volume_15min_later"] = currentVolume
	}

	if elapsed >= 30*time.Minute && followup.Price30MinLater == nil {
		updates["price_30min_later"] = currentPrice
		updates["change_30min_pct"] = priceChange
	}

	if elapsed >= 60*time.Minute && followup.Price60MinLater == nil {
		updates["price_60min_later"] = currentPrice
		updates["change_60min_pct"] = priceChange

		// Classify sustained impact (based on 1hr change)
		impact := wt.classifyImpact(priceChange, followup.AlertAction)
		updates["sustained_impact"] = impact

		// Detect reversal
		if followup.Price5MinLater != nil {
			change5min := *followup.Change5MinPct
			if wt.detectReversal(change5min, priceChange) {
				updates["reversal_detected"] = true
				updates["reversal_time_minutes"] = int(elapsed.Minutes())
			}
		}
	}

	if elapsed >= 24*time.Hour && followup.Price1DayLater == nil {
		updates["price_1day_later"] = currentPrice
		updates["change_1day_pct"] = priceChange
	}

	// Generate analysis text if significant time has passed or significant movement
	if elapsed >= 1*time.Hour || (priceChange > 2.0 || priceChange < -2.0) {
		analysis := wt.generateAnalysis(followup.StockSymbol, followup.AlertAction, followup.AlertPrice, currentPrice, priceChange, elapsed)
		updates["analysis"] = analysis
	}

	// Apply updates if any
	if len(updates) > 0 {
		log.Printf("🔄 Updating followup for %s (Alert %d): %d fields after %.0f minutes",
			followup.StockSymbol, followup.WhaleAlertID, len(updates), elapsed.Minutes())
		return wt.repo.UpdateWhaleFollowup(followup.WhaleAlertID, updates)
	}

	return fmt.Errorf("no update needed")
}

// classifyImpact determines if price movement aligns with whale action
func (wt *WhaleFollowupTracker) classifyImpact(priceChangePct float64, action string) string {
	threshold := 0.5 // 0.5% threshold for significance

	if action == "BUY" {
		if priceChangePct > threshold {
			return "POSITIVE" // Price went up after BUY
		} else if priceChangePct < -threshold {
			return "NEGATIVE" // Price went down after BUY (unexpected)
		}
	} else if action == "SELL" {
		if priceChangePct < -threshold {
			return "POSITIVE" // Price went down after SELL
		} else if priceChangePct > threshold {
			return "NEGATIVE" // Price went up after SELL (unexpected)
		}
	}

	return "NEUTRAL"
}

// detectReversal checks if price reversed direction
func (wt *WhaleFollowupTracker) detectReversal(change5min, change60min float64) bool {
	// Reversal detected if signs are opposite and both significant
	threshold := 1.0 // 1% threshold

	if change5min > threshold && change60min < -threshold {
		return true // Was up, now down
	}
	if change5min < -threshold && change60min > threshold {
		return true // Was down, now up
	}

	return false
}

// generateAnalysis creates a descriptive analysis string
func (wt *WhaleFollowupTracker) generateAnalysis(symbol, action string, entryPrice, currentPrice, changePct float64, elapsed time.Duration) string {
	impact := "NEUTRAL"
	if action == "BUY" {
		if changePct > 0.5 {
			impact = "POSITIVE"
		} else if changePct < -0.5 {
			impact = "NEGATIVE"
		}
	} else { // SELL
		if changePct < -0.5 {
			impact = "POSITIVE" // Price fell as expected
		} else if changePct > 0.5 {
			impact = "NEGATIVE" // Price rose against sell
		}
	}

	trendText := ""
	if changePct > 0 {
		trendText = fmt.Sprintf("naik %.2f%%", changePct)
	} else {
		trendText = fmt.Sprintf("turun %.2f%%", -changePct)
	}

	timeText := ""
	if elapsed < 1*time.Hour {
		timeText = fmt.Sprintf("%.0f menit", elapsed.Minutes())
	} else {
		timeText = fmt.Sprintf("%.1f jam", elapsed.Hours())
	}

	var analysis string
	if impact == "POSITIVE" {
		analysis = fmt.Sprintf("Harga %s setelah aktivitas whale %s. Konfirmasi sinyal valid dalam %s.", trendText, action, timeText)
	} else if impact == "NEGATIVE" {
		analysis = fmt.Sprintf("Harga %s berlawanan dengan aktivitas whale %s (False Signal/Reversal) dalam %s.", trendText, action, timeText)
	} else {
		analysis = fmt.Sprintf("Harga %s cenderung stabil (%s) dalam %s paska alert.", symbol, trendText, timeText)
	}

	return analysis
}

package app

import (
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// SignalTracker monitors trading signals and tracks their outcomes
type SignalTracker struct {
	repo *database.TradeRepository
	done chan bool
}

// NewSignalTracker creates a new signal outcome tracker
func NewSignalTracker(repo *database.TradeRepository) *SignalTracker {
	return &SignalTracker{
		repo: repo,
		done: make(chan bool),
	}
}

// Start begins the signal tracking loop
func (st *SignalTracker) Start() {
	log.Println("📊 Signal Outcome Tracker started")

	ticker := time.NewTicker(5 * time.Minute) // Run every 5 minutes
	defer ticker.Stop()

	// Run immediately on start
	st.trackSignalOutcomes()

	for {
		select {
		case <-ticker.C:
			st.trackSignalOutcomes()
		case <-st.done:
			log.Println("📊 Signal Outcome Tracker stopped")
			return
		}
	}
}

// Stop gracefully stops the tracker
func (st *SignalTracker) Stop() {
	close(st.done)
}

// trackSignalOutcomes processes open signals and creates/updates outcomes
func (st *SignalTracker) trackSignalOutcomes() {
	// Get signals without outcomes (limit to 100 per run)
	signals, err := st.repo.GetOpenSignals(100)
	if err != nil {
		log.Printf("❌ Error getting open signals: %v", err)
		return
	}

	if len(signals) == 0 {
		return // No new signals to track
	}

	log.Printf("📊 Tracking %d open signals...", len(signals))
	created := 0
	updated := 0

	for _, signal := range signals {
		// Check if outcome already exists
		existing, err := st.repo.GetSignalOutcomeBySignalID(signal.ID)
		if err != nil {
			log.Printf("❌ Error checking outcome for signal %d: %v", signal.ID, err)
			continue
		}

		if existing == nil {
			// Create new outcome record
			if err := st.createSignalOutcome(&signal); err != nil {
				log.Printf("❌ Error creating outcome for signal %d: %v", signal.ID, err)
			} else {
				created++
			}
		} else {
			// Update existing outcome
			if err := st.updateSignalOutcome(&signal, existing); err != nil {
				log.Printf("❌ Error updating outcome for signal %d: %v", signal.ID, err)
			} else {
				updated++
			}
		}
	}

	if created > 0 || updated > 0 {
		log.Printf("✅ Signal tracking: %d created, %d updated", created, updated)
	}
}

// createSignalOutcome creates a new outcome record for a signal
func (st *SignalTracker) createSignalOutcome(signal *database.TradingSignalDB) error {
	// Only track BUY and SELL signals
	if signal.Decision != "BUY" && signal.Decision != "SELL" {
		return nil
	}

	outcome := &database.SignalOutcome{
		SignalID:      signal.ID,
		StockSymbol:   signal.StockSymbol,
		EntryTime:     signal.GeneratedAt,
		EntryPrice:    signal.TriggerPrice,
		EntryDecision: signal.Decision,
		OutcomeStatus: "OPEN",
	}

	return st.repo.SaveSignalOutcome(outcome)
}

// updateSignalOutcome updates an existing outcome with current price data
func (st *SignalTracker) updateSignalOutcome(signal *database.TradingSignalDB, outcome *database.SignalOutcome) error {
	// Skip if already closed
	if outcome.OutcomeStatus != "OPEN" {
		return nil
	}

	// Get current price from latest candle
	candle, err := st.repo.GetLatestCandle(signal.StockSymbol)
	if err != nil || candle == nil {
		return fmt.Errorf("failed to get latest candle: %w", err)
	}

	currentPrice := candle.Close
	entryPrice := outcome.EntryPrice

	// Calculate price change
	priceChangePct := ((currentPrice - entryPrice) / entryPrice) * 100

	// Adjust for direction (BUY vs SELL)
	profitLossPct := priceChangePct
	if outcome.EntryDecision == "SELL" {
		profitLossPct = -priceChangePct // Inverse for short positions
	}

	// Calculate holding period
	holdingMinutes := int(time.Since(outcome.EntryTime).Minutes())

	// Update MAE and MFE (simplified - track current extremes)
	mae := outcome.MaxAdverseExcursion
	mfe := outcome.MaxFavorableExcursion

	if mae == nil || profitLossPct < *mae {
		mae = &profitLossPct
	}
	if mfe == nil || profitLossPct > *mfe {
		mfe = &profitLossPct
	}

	// Determine exit conditions
	shouldExit := false
	exitReason := ""

	// Exit after 60 minutes (time-based)
	if holdingMinutes >= 60 {
		shouldExit = true
		exitReason = "TIME_BASED"
	}

	// Exit on stop loss (-3%)
	if profitLossPct <= -3.0 {
		shouldExit = true
		exitReason = "STOP_LOSS"
	}

	// Exit on take profit (+5%)
	if profitLossPct >= 5.0 {
		shouldExit = true
		exitReason = "TAKE_PROFIT"
	}

	// Update outcome
	outcome.HoldingPeriodMinutes = &holdingMinutes
	outcome.PriceChangePct = &priceChangePct
	outcome.ProfitLossPct = &profitLossPct
	outcome.MaxAdverseExcursion = mae
	outcome.MaxFavorableExcursion = mfe

	if mfe != nil && mae != nil && *mae != 0 {
		riskReward := *mfe / (-*mae)
		outcome.RiskRewardRatio = &riskReward
	}

	if shouldExit {
		now := time.Now()
		outcome.ExitTime = &now
		outcome.ExitPrice = &currentPrice
		outcome.ExitReason = &exitReason

		// Determine outcome status
		if profitLossPct > 0.5 {
			outcome.OutcomeStatus = "WIN"
		} else if profitLossPct < -0.5 {
			outcome.OutcomeStatus = "LOSS"
		} else {
			outcome.OutcomeStatus = "BREAKEVEN"
		}
	}

	return st.repo.UpdateSignalOutcome(outcome)
}

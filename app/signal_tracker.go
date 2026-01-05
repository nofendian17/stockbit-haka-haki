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

	ticker := time.NewTicker(2 * time.Minute) // Run every 2 minutes for faster updates
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
		log.Println("📊 No open signals to track")
		return
	}

	log.Printf("📊 Tracking %d open signals...", len(signals))
	created := 0
	updated := 0
	closed := 0

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
				log.Printf("✅ Created outcome for signal %d (%s %s)", signal.ID, signal.StockSymbol, signal.Decision)
			}
		} else {
			// Update existing outcome
			wasClosed := existing.OutcomeStatus != "OPEN"
			if err := st.updateSignalOutcome(&signal, existing); err != nil {
				log.Printf("❌ Error updating outcome for signal %d: %v", signal.ID, err)
			} else {
				updated++
				// Check if outcome was closed in this update
				if !wasClosed && existing.OutcomeStatus != "OPEN" {
					closed++
					log.Printf("✅ Closed outcome for signal %d (%s): %s with %.2f%%",
						signal.ID, signal.StockSymbol, existing.OutcomeStatus, *existing.ProfitLossPct)
				}
			}
		}
	}

	if created > 0 || updated > 0 {
		log.Printf("✅ Signal tracking completed: %d created, %d updated, %d closed", created, updated, closed)
	}
}

// createSignalOutcome creates a new outcome record for a signal
func (st *SignalTracker) createSignalOutcome(signal *database.TradingSignalDB) error {
	// Indonesian market: Only track BUY signals (no short selling)
	if signal.Decision != "BUY" {
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

	// Indonesian stock market: Only BUY positions (no short selling)
	if outcome.EntryDecision != "BUY" {
		log.Printf("⚠️ Skipping non-BUY signal %d: Indonesia market doesn't support short selling", signal.ID)
		return nil
	}

	// Get current price from latest candle
	candle, err := st.repo.GetLatestCandle(signal.StockSymbol)
	if err != nil || candle == nil {
		return fmt.Errorf("failed to get latest candle: %w", err)
	}

	currentPrice := candle.Close
	entryPrice := outcome.EntryPrice

	// Calculate price change (only BUY positions)
	priceChangePct := ((currentPrice - entryPrice) / entryPrice) * 100
	profitLossPct := priceChangePct

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

	// Get latest order flow to determine momentum
	orderFlow, _ := st.repo.GetLatestOrderFlow(signal.StockSymbol)

	// Determine exit conditions with dynamic take profit
	shouldExit := false
	exitReason := ""

	// Always enforce stop loss (-2%)
	if profitLossPct <= -2.0 {
		shouldExit = true
		exitReason = "STOP_LOSS"
	}

	// Dynamic Take Profit based on order flow momentum
	// Indonesian market: Only BUY positions
	if profitLossPct > 0 && orderFlow != nil {
		// Calculate sell pressure (for exit signal)
		totalVolume := orderFlow.BuyVolumeLots + orderFlow.SellVolumeLots
		var sellPressure float64
		if totalVolume > 0 {
			sellPressure = (orderFlow.SellVolumeLots / totalVolume) * 100
		}

		// Take profit jika sell pressure meningkat (momentum melemah)
		if sellPressure > 60 && profitLossPct >= 1.0 {
			shouldExit = true
			exitReason = "TAKE_PROFIT_MOMENTUM_REVERSAL"
		} else if profitLossPct >= 5.0 {
			// Maximum take profit di 5%
			shouldExit = true
			exitReason = "TAKE_PROFIT_MAX"
		}
	} else if profitLossPct >= 5.0 {
		// Fallback: Close at 5% if no order flow data
		shouldExit = true
		exitReason = "TAKE_PROFIT_MAX"
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

		// Determine outcome status - More sensitive thresholds
		if profitLossPct > 0.2 {
			outcome.OutcomeStatus = "WIN"
		} else if profitLossPct < -0.2 {
			outcome.OutcomeStatus = "LOSS"
		} else {
			outcome.OutcomeStatus = "BREAKEVEN"
		}
	}

	return st.repo.UpdateSignalOutcome(outcome)
}

package app

import (
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// TradingHours defines Indonesian stock market trading hours (WIB/UTC+7)
const (
	MarketOpenHour  = 9  // 09:00 WIB
	MarketCloseHour = 16 // 16:00 WIB (close at 16:00, but last trade acceptance ~15:50)
	MarketTimeZone  = "Asia/Jakarta"
)

// Position Management Constants
const (
	MinSignalIntervalMinutes = 15  // Minimum 15 minutes between signals for same symbol
	MaxOpenPositions         = 10  // Maximum concurrent open positions
	MaxPositionsPerSymbol    = 1   // Maximum positions per symbol (prevent averaging down)
	SignalTimeWindowMinutes  = 5   // Time window for duplicate detection
	MaxTradingHoldingMinutes = 180 // Maximum holding period: 3 hours
)

// isTradingTime checks if the given time is within Indonesian market trading hours
func isTradingTime(t time.Time) bool {
	// Convert to Jakarta timezone
	loc, err := time.LoadLocation(MarketTimeZone)
	if err != nil {
		log.Printf("⚠️ Failed to load timezone %s: %v", MarketTimeZone, err)
		// Fallback: assume UTC+7 offset
		loc = time.FixedZone("WIB", 7*60*60)
	}

	localTime := t.In(loc)
	hour := localTime.Hour()
	weekday := localTime.Weekday()

	// Market is closed on weekends
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}

	// Market hours: 09:00 - 16:00 WIB
	return hour >= MarketOpenHour && hour < MarketCloseHour
}

// getTradingSession returns the current trading session name
func getTradingSession(t time.Time) string {
	loc, err := time.LoadLocation(MarketTimeZone)
	if err != nil {
		loc = time.FixedZone("WIB", 7*60*60)
	}

	localTime := t.In(loc)
	hour := localTime.Hour()
	minute := localTime.Minute()

	// Pre-opening (08:45-09:00)
	if hour == 8 && minute >= 45 {
		return "PRE_OPENING"
	}

	// Session 1 (09:00-12:00)
	if hour >= 9 && hour < 12 {
		return "SESSION_1"
	}

	// Lunch break (12:00-13:30)
	if (hour == 12) || (hour == 13 && minute < 30) {
		return "LUNCH_BREAK"
	}

	// Session 2 (13:30-14:50)
	if (hour == 13 && minute >= 30) || (hour == 14 && minute < 50) {
		return "SESSION_2"
	}

	// Pre-closing (14:50-15:00)
	if hour == 14 && minute >= 50 {
		return "PRE_CLOSING"
	}

	// Post-market (15:00-16:00) - trades still settle but limited
	if hour >= 15 && hour < 16 {
		return "POST_MARKET"
	}

	// After hours
	return "AFTER_HOURS"
}

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

// shouldCreateOutcome checks if we should create an outcome for this signal
// Returns: (shouldCreate bool, reason string)
func (st *SignalTracker) shouldCreateOutcome(signal *database.TradingSignalDB) (bool, string) {
	// 1. Check if too many open positions globally
	openOutcomes, err := st.repo.GetSignalOutcomes("", "OPEN", time.Time{}, time.Time{}, 0)
	if err == nil && len(openOutcomes) >= MaxOpenPositions {
		return false, fmt.Sprintf("Max open positions reached (%d/%d)", len(openOutcomes), MaxOpenPositions)
	}

	// 2. Check if symbol already has open position
	symbolOutcomes, err := st.repo.GetSignalOutcomes(signal.StockSymbol, "OPEN", time.Time{}, time.Time{}, 0)
	if err == nil && len(symbolOutcomes) >= MaxPositionsPerSymbol {
		return false, fmt.Sprintf("Symbol %s already has %d open position(s)", signal.StockSymbol, len(symbolOutcomes))
	}

	// 3. Check for recent signals within time window (duplicate prevention)
	recentSignalTime := signal.GeneratedAt.Add(-time.Duration(SignalTimeWindowMinutes) * time.Minute)
	recentSignals, err := st.repo.GetTradingSignals(signal.StockSymbol, signal.Strategy, "BUY", recentSignalTime, signal.GeneratedAt, 10)
	if err == nil && len(recentSignals) > 1 {
		// More than 1 means there's a duplicate within the time window
		return false, fmt.Sprintf("Duplicate signal within %d minute window", SignalTimeWindowMinutes)
	}

	// 4. Check minimum interval since last signal for this symbol
	lastSignalTime := signal.GeneratedAt.Add(-time.Duration(MinSignalIntervalMinutes) * time.Minute)
	lastSignals, err := st.repo.GetTradingSignals(signal.StockSymbol, "", "BUY", lastSignalTime, time.Time{}, 1)
	if err == nil && len(lastSignals) > 0 {
		// Found a recent signal
		if lastSignals[0].ID != signal.ID {
			timeSince := signal.GeneratedAt.Sub(lastSignals[0].GeneratedAt).Minutes()
			if timeSince < MinSignalIntervalMinutes {
				return false, fmt.Sprintf("Signal too soon (%.1f min < %d min required)", timeSince, MinSignalIntervalMinutes)
			}
		}
	}

	return true, ""
}

// createSignalOutcome creates a new outcome record for a signal
func (st *SignalTracker) createSignalOutcome(signal *database.TradingSignalDB) error {
	// Indonesian market: Only track BUY signals (no short selling)
	if signal.Decision != "BUY" {
		return nil
	}

	// Exclude NG (Negotiated Trading) signals - these are special transactions
	// that don't reflect normal market dynamics
	if signal.WhaleAlertID != nil {
		// Get the whale alert to check market_board
		alert, err := st.repo.GetWhaleAlertByID(*signal.WhaleAlertID)
		if err == nil && alert != nil && alert.MarketBoard == "NG" {
			log.Printf("⏭️ Skipping signal %d (%s): NG (Negotiated Trading) excluded from tracking",
				signal.ID, signal.StockSymbol)
			return nil
		}
	}

	// Validate trading time
	if !isTradingTime(signal.GeneratedAt) {
		session := getTradingSession(signal.GeneratedAt)
		log.Printf("⏰ Skipping signal %d (%s): Generated outside trading hours (session: %s)",
			signal.ID, signal.StockSymbol, session)
		return nil
	}

	// Check duplicate prevention and position limits
	shouldCreate, reason := st.shouldCreateOutcome(signal)
	if !shouldCreate {
		log.Printf("⏭️ Skipping signal %d (%s): %s", signal.ID, signal.StockSymbol, reason)
		return nil
	}

	session := getTradingSession(signal.GeneratedAt)
	log.Printf("✅ Creating outcome for signal %d (%s) - Session: %s",
		signal.ID, signal.StockSymbol, session)

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

	// Check current trading session
	now := time.Now()
	currentSession := getTradingSession(now)

	// Auto-close positions at market close (16:00 WIB)
	if currentSession == "AFTER_HOURS" && outcome.ExitTime == nil {
		log.Printf("🔔 Market closed - Auto-closing position for signal %d (%s)",
			signal.ID, signal.StockSymbol)
		// Will force exit below
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

	// Maximum holding period: 3 hours (180 minutes)
	if !shouldExit && holdingMinutes >= MaxTradingHoldingMinutes {
		shouldExit = true
		exitReason = "MAX_HOLDING_TIME"
		log.Printf("⏰ Max holding time reached for signal %d (%s): %d minutes",
			signal.ID, signal.StockSymbol, holdingMinutes)
	}

	// Force exit at market close
	if !shouldExit && currentSession == "AFTER_HOURS" {
		shouldExit = true
		exitReason = "MARKET_CLOSE"
		log.Printf("⏰ Force exit due to market close for signal %d (%s)", signal.ID, signal.StockSymbol)
	}

	// Auto-exit in pre-closing session (14:50-15:00) if profitable
	if currentSession == "PRE_CLOSING" && profitLossPct > 0.5 {
		shouldExit = true
		exitReason = "PRE_CLOSE_PROFIT_TAKING"
		log.Printf("⏰ Pre-close profit taking for signal %d (%s): %.2f%%",
			signal.ID, signal.StockSymbol, profitLossPct)
	}

	// Dynamic Take Profit based on order flow momentum (only during trading hours)
	// Indonesian market: Only BUY positions
	if !shouldExit && isTradingTime(now) && profitLossPct > 0 && orderFlow != nil {
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

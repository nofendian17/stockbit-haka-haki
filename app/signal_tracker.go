package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/cache"
	"stockbit-haka-haki/config"
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
	// Optimization: Time-of-Day adjustments (Still hardcoded as business logic)
	MorningBoostHour     = 10 // Before 10:00 WIB = morning momentum
	AfternoonCautionHour = 14 // After 14:00 WIB = increased caution
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
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
	done  chan bool

	exitCalc      *ExitStrategyCalculator // ATR-based exit strategy calculator
	filterService *SignalFilterService    // Dedicated service for signal filtering logic
}

// NewSignalTracker creates a new signal outcome tracker
func NewSignalTracker(repo *database.TradeRepository, redis *cache.RedisClient, cfg *config.Config) *SignalTracker {

	// Initialize Exit Strategy Calculator
	exitCalc := NewExitStrategyCalculator(repo, cfg)
	// Initialize Signal Filter Service
	filterService := NewSignalFilterService(repo, redis, cfg)

	return &SignalTracker{
		repo:  repo,
		redis: redis,
		cfg:   cfg,
		done:  make(chan bool),

		exitCalc:      exitCalc,
		filterService: filterService,
	}
}

// Start begins the signal tracking loop
func (st *SignalTracker) Start() {
	log.Println("📊 Signal Outcome Tracker started")

	ticker := time.NewTicker(2 * time.Minute) // Run every 2 minutes for faster updates
	defer ticker.Stop()

	// Ticker for signal generation (runs more frequently)
	signalTicker := time.NewTicker(30 * time.Second)
	defer signalTicker.Stop() // FIX: Was ticker.Stop() - goroutine leak

	// Run immediately on start
	go st.generateSignals()
	st.trackSignalOutcomes()

	for {
		select {
		case <-signalTicker.C:
			st.generateSignals()
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
	created := 0
	updated := 0
	closed := 0

	// PART 1: Create outcomes for new signals (signals without outcomes)
	newSignals, err := st.repo.GetOpenSignals(100)
	if err != nil {
		log.Printf("❌ Error getting new signals: %v", err)
	} else if len(newSignals) > 0 {
		log.Printf("📊 Processing %d new signals...", len(newSignals))
		for _, signal := range newSignals {
			createdOutcome, err := st.createSignalOutcome(&signal)
			if err != nil {
				log.Printf("❌ Error creating outcome for signal %d: %v", signal.ID, err)
			} else if createdOutcome {
				created++
				log.Printf("✅ Created outcome for signal %d (%s %s)", signal.ID, signal.StockSymbol, signal.Decision)
			}
		}
	}

	// PART 2: Update existing OPEN outcomes (the critical part!)
	openOutcomes, err := st.repo.GetSignalOutcomes("", "OPEN", time.Time{}, time.Time{}, 100)
	if err != nil {
		log.Printf("❌ Error getting open outcomes: %v", err)
		return
	}

	if len(openOutcomes) == 0 {
		if created == 0 {
			log.Println("📊 No open positions to track")
		}
		return
	}

	log.Printf("📊 Updating %d open positions...", len(openOutcomes))

	// OPTIMIZATION: Bulk fetch all signals at once to eliminate N+1 queries
	signalIDs := make([]int64, len(openOutcomes))
	for i, outcome := range openOutcomes {
		signalIDs[i] = outcome.SignalID
	}

	signalsMap, err := st.repo.GetSignalsByIDs(signalIDs)
	if err != nil {
		log.Printf("❌ Error bulk fetching signals: %v", err)
		return
	}

	for _, outcome := range openOutcomes {
		// Get the signal from the bulk-fetched map
		signal := signalsMap[outcome.SignalID]
		if signal == nil {
			log.Printf("⚠️ Signal %d not found for outcome %d", outcome.SignalID, outcome.ID)
			continue
		}

		// Update the outcome
		wasClosed := outcome.OutcomeStatus != "OPEN"
		if err := st.updateSignalOutcome(signal, &outcome); err != nil {
			log.Printf("❌ Error updating outcome for signal %d: %v", signal.ID, err)
		} else {
			updated++
			// Check if outcome was closed in this update
			if !wasClosed && outcome.OutcomeStatus != "OPEN" {
				closed++
				log.Printf("✅ Closed outcome for signal %d (%s): %s with %.2f%%",
					signal.ID, signal.StockSymbol, outcome.OutcomeStatus, *outcome.ProfitLossPct)
			}
		}
	}

	if created > 0 || updated > 0 {
		log.Printf("✅ Signal tracking completed: %d created, %d updated, %d closed", created, updated, closed)
	}
}

// shouldCreateOutcome checks if we should create an outcome for this signal
// Returns: (shouldCreate bool, reason string, multiplier float64)
func (st *SignalTracker) shouldCreateOutcome(signal *database.TradingSignalDB) (bool, string, float64) {
	ctx := context.Background()

	// 1. Evaluate signal using SignalFilterService (Consolidated Logic)
	shouldTrade, reason, multiplier := st.filterService.Evaluate(signal)
	if !shouldTrade {
		return false, reason, 0.0
	}

	// 2. Redis Optimizations: Check cooldowns (fastest)
	if st.redis != nil {
		// Check cooldown key: signal:cooldown:{symbol}:{strategy}
		cooldownKey := fmt.Sprintf("signal:cooldown:%s:%s", signal.StockSymbol, signal.Strategy)
		var cooldownSignalID int64
		// Verify if key exists AND is not the current signal
		if err := st.redis.Get(ctx, cooldownKey, &cooldownSignalID); err == nil && cooldownSignalID != 0 && cooldownSignalID != signal.ID {
			return false, fmt.Sprintf("In cooldown period for %s (Signal %d)", signal.Strategy, cooldownSignalID), 0.0
		}

		// Check recent duplicate key: signal:recent:{symbol}
		recentKey := fmt.Sprintf("signal:recent:%s", signal.StockSymbol)
		var recentSignalID int64
		if err := st.redis.Get(ctx, recentKey, &recentSignalID); err == nil && recentSignalID != 0 && recentSignalID != signal.ID {
			return false, fmt.Sprintf("Recent signal %d exists for %s (too soon)", recentSignalID, signal.StockSymbol), 0.0
		}
	}

	// 3. Regime-Adaptive Position Limit
	adaptiveLimit := st.filterService.GetRegimeAdaptiveLimit(signal.StockSymbol)

	// Check if too many open positions globally (with adaptive limit)
	openOutcomes, err := st.repo.GetSignalOutcomes("", "OPEN", time.Time{}, time.Time{}, 0)
	if err == nil && len(openOutcomes) >= adaptiveLimit {
		return false, fmt.Sprintf("Max open positions reached (%d/%d adaptive)", len(openOutcomes), adaptiveLimit), 0.0
	}

	// Check if symbol already has open position
	symbolOutcomes, err := st.repo.GetSignalOutcomes(signal.StockSymbol, "OPEN", time.Time{}, time.Time{}, 0)
	if err == nil && len(symbolOutcomes) >= st.cfg.Trading.MaxPositionsPerSymbol {
		return false, fmt.Sprintf("Symbol %s already has %d open position(s)", signal.StockSymbol, len(symbolOutcomes)), 0.0
	}

	// Check for recent signals within time window (duplicate prevention)
	recentSignalTime := signal.GeneratedAt.Add(-time.Duration(st.cfg.Trading.SignalTimeWindowMinutes) * time.Minute)
	recentSignals, err := st.repo.GetTradingSignals(signal.StockSymbol, signal.Strategy, "BUY", recentSignalTime, signal.GeneratedAt, 10)
	if err == nil && len(recentSignals) > 1 {
		return false, fmt.Sprintf("Duplicate signal within %d minute window", st.cfg.Trading.SignalTimeWindowMinutes), 0.0
	}

	// Check minimum interval since last signal for this symbol
	lastSignalTime := signal.GeneratedAt.Add(-time.Duration(st.cfg.Trading.MinSignalIntervalMinutes) * time.Minute)
	lastSignals, err := st.repo.GetTradingSignals(signal.StockSymbol, "", "BUY", lastSignalTime, time.Time{}, 1)
	if err == nil && len(lastSignals) > 0 {
		if lastSignals[0].ID != signal.ID {
			timeSince := signal.GeneratedAt.Sub(lastSignals[0].GeneratedAt).Minutes()
			if timeSince < float64(st.cfg.Trading.MinSignalIntervalMinutes) {
				return false, fmt.Sprintf("Signal too soon (%.1f min < %d min required)", timeSince, st.cfg.Trading.MinSignalIntervalMinutes), 0.0
			}
		}
	}

	return true, "", multiplier
}

// createSignalOutcome creates a new outcome record for a signal
// Returns: (createdOpenPosition bool, err error)
func (st *SignalTracker) createSignalOutcome(signal *database.TradingSignalDB) (bool, error) {
	// Indonesian market: Only track BUY signals (no short selling)
	if signal.Decision != "BUY" {
		reason := "Only BUY signals are supported"
		st.createSkippedOutcome(signal, reason)
		return false, nil
	}

	// Exclude NG (Negotiated Trading) signals
	if signal.WhaleAlertID != nil {
		alert, err := st.repo.GetWhaleAlertByID(*signal.WhaleAlertID)
		if err == nil && alert != nil && alert.MarketBoard == "NG" {
			reason := "NG (Negotiated Trading) excluded"
			log.Printf("⏭️ Skipping signal %d (%s): %s", signal.ID, signal.StockSymbol, reason)
			st.createSkippedOutcome(signal, reason)
			return false, nil
		}
	}

	// Validate trading time
	if !isTradingTime(signal.GeneratedAt) {
		session := getTradingSession(signal.GeneratedAt)
		reason := fmt.Sprintf("Generated outside trading hours (session: %s)", session)
		log.Printf("⏰ Skipping signal %d (%s): %s", signal.ID, signal.StockSymbol, reason)
		st.createSkippedOutcome(signal, reason)
		return false, nil
	}

	// Check duplicate prevention and position limits (with ALL optimizations)
	shouldCreate, reason, multiplier := st.shouldCreateOutcome(signal)
	if !shouldCreate {
		log.Printf("⏭️ Skipping signal %d (%s): %s", signal.ID, signal.StockSymbol, reason)
		st.createSkippedOutcome(signal, reason)
		return false, nil
	}

	session := getTradingSession(signal.GeneratedAt)
	log.Printf("✅ Creating outcome for signal %d (%s) - Session: %s (Mult: %.2fx)",
		signal.ID, signal.StockSymbol, session, multiplier)

	// Calculate ATR-based exit levels for initial setup
	exitLevels := st.exitCalc.GetExitLevels(signal.StockSymbol, signal.TriggerPrice)

	// Create outcome
	outcome := &database.SignalOutcome{
		SignalID:          signal.ID,
		StockSymbol:       signal.StockSymbol,
		EntryTime:         signal.GeneratedAt,
		EntryPrice:        signal.TriggerPrice,
		EntryDecision:     signal.Decision,
		OutcomeStatus:     "OPEN",
		ATRAtEntry:        &exitLevels.ATR,
		TrailingStopPrice: &exitLevels.StopLossPrice,
	}

	if err := st.repo.SaveSignalOutcome(outcome); err != nil {
		return false, err
	}
	return true, nil
}

// createSkippedOutcome creates a closed/skipped outcome to prevent reprocessing
func (st *SignalTracker) createSkippedOutcome(signal *database.TradingSignalDB, reason string) {
	now := time.Now()
	outcome := &database.SignalOutcome{
		SignalID:      signal.ID,
		StockSymbol:   signal.StockSymbol,
		EntryTime:     signal.GeneratedAt,
		EntryPrice:    signal.TriggerPrice,
		EntryDecision: signal.Decision,
		OutcomeStatus: "SKIPPED",
		ExitReason:    &reason,
		ExitTime:      &now,
	}
	if err := st.repo.SaveSignalOutcome(outcome); err != nil {
		log.Printf("❌ Error saving SKIPPED outcome for signal %d: %v", signal.ID, err)
	}
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

	// Get current price from latest candle with fallback to latest trade
	var currentPrice float64
	candle, err := st.repo.GetLatestCandle(signal.StockSymbol)
	if err != nil || candle == nil {
		// Fallback: Get price from latest trade if candle is unavailable
		trades, err := st.repo.GetRecentTrades(signal.StockSymbol, 1, "")
		if err != nil || len(trades) == 0 {
			// No data available at all - log warning but don't fail completely
			log.Printf("⚠️ No price data available for %s (signal %d) - keeping OPEN status",
				signal.StockSymbol, signal.ID)
			return nil // Return without error to prevent blocking other updates
		}
		currentPrice = trades[0].Price
		log.Printf("📊 Using latest trade price for %s: %.0f (no candle data)",
			signal.StockSymbol, currentPrice)
	} else {
		currentPrice = candle.Close
	}
	entryPrice := outcome.EntryPrice

	// Calculate price change (only BUY positions)
	priceChangePct := ((currentPrice - entryPrice) / entryPrice) * 100
	profitLossPct := priceChangePct

	// Calculate holding period
	holdingMinutes := int(time.Since(outcome.EntryTime).Minutes())

	// Update MAE and MFE (track current extremes)
	mae := outcome.MaxAdverseExcursion
	mfe := outcome.MaxFavorableExcursion

	// Initialize MAE/MFE on first update if nil
	if mae == nil {
		mae = &profitLossPct
	} else if profitLossPct < *mae {
		// Update if current P&L is more adverse (more negative)
		mae = &profitLossPct
	}

	if mfe == nil {
		mfe = &profitLossPct
	} else if profitLossPct > *mfe {
		// Update if current P&L is more favorable (more positive)
		mfe = &profitLossPct
	}

	// Get latest order flow to determine momentum
	orderFlow, _ := st.repo.GetLatestOrderFlow(signal.StockSymbol)

	// Determine exit conditions with ATR-based dynamic exit strategy
	shouldExit := false
	exitReason := ""

	// Calculate ATR-based exit levels
	exitLevels := st.exitCalc.GetExitLevels(signal.StockSymbol, outcome.EntryPrice)

	// Get current trailing stop (initialize if nil)
	var currentTrailingStop float64
	if outcome.TrailingStopPrice != nil {
		currentTrailingStop = *outcome.TrailingStopPrice
	} else {
		// Initialize trailing stop at entry price minus initial stop
		currentTrailingStop = outcome.EntryPrice * (1 - exitLevels.InitialStopPct/100)
	}

	// Use ATR-based exit strategy
	shouldExit, exitReason, newTrailingStop := st.exitCalc.ShouldExitPosition(
		outcome.EntryPrice,
		currentPrice,
		exitLevels,
		currentTrailingStop,
		profitLossPct,
		holdingMinutes,
	)

	// Update trailing stop in outcome
	if newTrailingStop > currentTrailingStop {
		outcome.TrailingStopPrice = &newTrailingStop
		log.Printf("📈 Updated trailing stop for %s: %.0f → %.0f",
			signal.StockSymbol, currentTrailingStop, newTrailingStop)
	}

	// Force exit at market close
	if !shouldExit && currentSession == "AFTER_HOURS" {
		shouldExit = true
		exitReason = "MARKET_CLOSE"
		log.Printf("⏰ Force exit due to market close for signal %d (%s)", signal.ID, signal.StockSymbol)
	}

	// Auto-exit in pre-closing session (14:50-15:00) if profitable
	if !shouldExit && currentSession == "PRE_CLOSING" && profitLossPct > 1.0 {
		shouldExit = true
		exitReason = "PRE_CLOSE_PROFIT_TAKING"
		log.Printf("⏰ Pre-close profit taking for signal %d (%s): %.2f%%",
			signal.ID, signal.StockSymbol, profitLossPct)
	}

	// Order flow momentum reversal check (additional exit signal)
	if !shouldExit && isTradingTime(now) && profitLossPct > 0 && orderFlow != nil {
		totalVolume := orderFlow.BuyVolumeLots + orderFlow.SellVolumeLots
		var sellPressure float64
		if totalVolume > 0 {
			sellPressure = (orderFlow.SellVolumeLots / totalVolume) * 100
		}

		// Take profit if sell pressure high and we have gains
		if sellPressure > 65 && profitLossPct >= exitLevels.TakeProfit1Pct*0.75 {
			shouldExit = true
			exitReason = "TAKE_PROFIT_MOMENTUM_REVERSAL"
		}
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

	// 6. Check Max Holding Loss (Cut Loss if stuck in loss for too long)
	// Example: If held > 60 mins and loss > MaxHoldingLossPct (e.g., 1.5%), cut loss
	if !shouldExit && holdingMinutes > 60 && profitLossPct < -st.cfg.Trading.MaxHoldingLossPct {
		shouldExit = true
		exitReason = "TIME_BASED_CUT_LOSS"
		log.Printf("✂️ Time-based cut loss for %s: held %d mins, P/L %.2f%%", signal.StockSymbol, holdingMinutes, profitLossPct)
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

// GetOpenPositions returns currently open trading positions with optional filters
func (st *SignalTracker) GetOpenPositions(symbol, strategy string, limit int) ([]database.SignalOutcome, error) {
	// Get open signal outcomes
	outcomes, err := st.repo.GetSignalOutcomes(symbol, "OPEN", time.Time{}, time.Time{}, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get open positions: %w", err)
	}

	// Filter by strategy if provided
	if strategy != "" && strategy != "ALL" {
		var filtered []database.SignalOutcome
		for _, outcome := range outcomes {
			// Get the signal to check strategy
			signal, err := st.repo.GetSignalByID(outcome.SignalID)
			if err == nil && signal != nil && signal.Strategy == strategy {
				filtered = append(filtered, outcome)
			}
		}
		return filtered, nil
	}

	return outcomes, nil
}

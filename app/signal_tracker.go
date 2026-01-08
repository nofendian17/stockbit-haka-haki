package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/cache"
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
	MinSignalIntervalMinutes = 15 // Minimum 15 minutes between signals for same symbol
	MaxOpenPositions         = 10 // Maximum concurrent open positions (adaptive based on regime)
	MaxPositionsPerSymbol    = 1  // Maximum positions per symbol (prevent averaging down)
	SignalTimeWindowMinutes  = 5  // Time window for duplicate detection

	// Optimization: Order Flow Imbalance thresholds
	OrderFlowBuyThreshold  = 0.5  // 50% buy imbalance required for BUY signals (Tightened from 30%)
	AggressiveBuyThreshold = 60.0 // 60% aggressive buyers required

	// Optimization: Baseline quality requirements
	MinBaselineSampleSize       = 50 // Minimum trades for reliable baseline (Tightened from 30)
	MinBaselineSampleSizeStrict = 80 // Minimum for full confidence

	// Optimization: Time-of-day adjustments
	MorningBoostHour     = 10 // Before 10:00 WIB = morning momentum
	AfternoonCautionHour = 14 // After 14:00 WIB = increased caution

	// Optimization: Strategy performance tracking
	MinStrategySignals   = 20 // Minimum signals before performance evaluation
	LowWinRateThreshold  = 30 // Below 30% = disable strategy temporarily
	HighWinRateThreshold = 65 // Above 65% = boost confidence
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
	repo          *database.TradeRepository
	redis         *cache.RedisClient
	done          chan bool
	mtfAnalyzer   *MTFAnalyzer            // Multi-timeframe analyzer
	scorecardEval *ScorecardEvaluator     // Scorecard evaluator for signal quality
	exitCalc      *ExitStrategyCalculator // ATR-based exit strategy calculator
}

// NewSignalTracker creates a new signal outcome tracker
func NewSignalTracker(repo *database.TradeRepository, redis *cache.RedisClient) *SignalTracker {
	// Initialize MTF Analyzer
	mtf := NewMTFAnalyzer(repo)

	// Initialize Scorecard Evaluator with MTF
	scoreEval := NewScorecardEvaluator(repo, mtf)

	// Initialize Exit Strategy Calculator
	exitCalc := NewExitStrategyCalculator(repo)

	return &SignalTracker{
		repo:          repo,
		redis:         redis,
		done:          make(chan bool),
		mtfAnalyzer:   mtf,
		scorecardEval: scoreEval,
		exitCalc:      exitCalc,
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
// Returns: (shouldCreate bool, reason string)
func (st *SignalTracker) shouldCreateOutcome(signal *database.TradingSignalDB) (bool, string) {
	ctx := context.Background()

	// NEW: Scorecard Evaluation (comprehensive quality check)
	if st.scorecardEval != nil {
		var scorecard *SignalScorecard

		// 1. Try to load from AnalysisData (generation time features)
		if signal.AnalysisData != "" {
			var loadedScorecard SignalScorecard
			if err := json.Unmarshal([]byte(signal.AnalysisData), &loadedScorecard); err == nil {
				scorecard = &loadedScorecard
			}
		}

		// 2. Fallback: Re-evaluate if missing
		if scorecard == nil {
			scorecard = st.scorecardEval.EvaluateSignal(signal)
		}

		if !scorecard.IsPassing() {
			return false, fmt.Sprintf("Scorecard too low (%d/%d required)", scorecard.Total(), MinScoreForSignal)
		}
		log.Printf("✅ Scorecard PASSED for %s: %s", signal.StockSymbol, scorecard.String())
	}

	// OPTIMIZATION #10: Strategy Performance Check (disable underperforming strategies)
	strategyMult, shouldSkip, reason := st.getStrategyPerformanceMultiplier(signal.Strategy)
	if shouldSkip {
		return false, reason
	}

	// OPTIMIZATION #8: Baseline Quality Check
	baselinePassed, _, baselineReason := st.checkBaselineQuality(signal.StockSymbol)
	if !baselinePassed {
		return false, baselineReason
	}

	// OPTIMIZATION NEW #1: Dynamic Confidence Threshold (Historical Data Driven)
	optimalThreshold, thresholdReason := st.getDynamicConfidenceThreshold(signal.Strategy)
	if signal.Confidence < optimalThreshold {
		return false, fmt.Sprintf("Below optimal confidence threshold (%.2f < %.2f): %s",
			signal.Confidence, optimalThreshold, thresholdReason)
	}

	// OPTIMIZATION NEW #2: Regime-Specific Effectiveness Check
	regimePassed, regimeReason := st.checkRegimeEffectiveness(signal.Strategy, signal.StockSymbol)
	if !regimePassed {
		return false, regimeReason
	}

	// OPTIMIZATION NEW #3: Expected Value Filter (Reject Negative EV Strategies)
	evPassed, ev, evReason := st.checkSignalExpectedValue(signal.Strategy)
	if !evPassed {
		return false, evReason
	}
	if ev > 0 {
		log.Printf("📈 Signal EV check passed for %s: +%.4f", signal.Strategy, ev)
	}

	// OPTIMIZATION #2: Order Flow Imbalance Check
	flowPassed, _, flowReason := st.checkOrderFlowImbalance(signal.StockSymbol, signal.Decision)
	if !flowPassed {
		return false, flowReason
	}

	// 0. Redis Optimizations: Check cooldowns first (fastest)
	if st.redis != nil {
		// Check cooldown key: signal:cooldown:{symbol}:{strategy}
		cooldownKey := fmt.Sprintf("signal:cooldown:%s:%s", signal.StockSymbol, signal.Strategy)
		var cooldownSignalID int64
		// Verify if key exists AND is not the current signal
		if err := st.redis.Get(ctx, cooldownKey, &cooldownSignalID); err == nil && cooldownSignalID != 0 && cooldownSignalID != signal.ID {
			return false, fmt.Sprintf("In cooldown period for %s (Signal %d)", signal.Strategy, cooldownSignalID)
		}

		// Check recent duplicate key: signal:recent:{symbol}
		recentKey := fmt.Sprintf("signal:recent:%s", signal.StockSymbol)
		var recentSignalID int64
		if err := st.redis.Get(ctx, recentKey, &recentSignalID); err == nil && recentSignalID != 0 && recentSignalID != signal.ID {
			return false, fmt.Sprintf("Recent signal %d exists for %s (too soon)", recentSignalID, signal.StockSymbol)
		}
	}

	// OPTIMIZATION #5: Regime-Adaptive Position Limit
	adaptiveLimit := st.getRegimeAdaptiveLimit(signal.StockSymbol)

	// 1. Check if too many open positions globally (with adaptive limit)
	openOutcomes, err := st.repo.GetSignalOutcomes("", "OPEN", time.Time{}, time.Time{}, 0)
	if err == nil && len(openOutcomes) >= adaptiveLimit {
		return false, fmt.Sprintf("Max open positions reached (%d/%d adaptive)", len(openOutcomes), adaptiveLimit)
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
		return false, fmt.Sprintf("Duplicate signal within %d minute window", SignalTimeWindowMinutes)
	}

	// 4. Check minimum interval since last signal for this symbol
	lastSignalTime := signal.GeneratedAt.Add(-time.Duration(MinSignalIntervalMinutes) * time.Minute)
	lastSignals, err := st.repo.GetTradingSignals(signal.StockSymbol, "", "BUY", lastSignalTime, time.Time{}, 1)
	if err == nil && len(lastSignals) > 0 {
		if lastSignals[0].ID != signal.ID {
			timeSince := signal.GeneratedAt.Sub(lastSignals[0].GeneratedAt).Minutes()
			if timeSince < MinSignalIntervalMinutes {
				return false, fmt.Sprintf("Signal too soon (%.1f min < %d min required)", timeSince, MinSignalIntervalMinutes)
			}
		}
	}

	// Log optimization results
	if strategyMult != 1.0 {
		log.Printf("📊 Strategy multiplier for %s: %.2fx (%s)", signal.Strategy, strategyMult, reason)
	}

	return true, ""
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

	// OPTIMIZATION #6: Time-of-Day Filtering
	timeMultiplier, timeReason := st.getTimeOfDayMultiplier(signal.GeneratedAt)
	if timeMultiplier == 0.0 {
		log.Printf("⏰ Skipping signal %d (%s): %s", signal.ID, signal.StockSymbol, timeReason)
		st.createSkippedOutcome(signal, timeReason)
		return false, nil
	}

	// Check duplicate prevention and position limits (with ALL optimizations)
	shouldCreate, reason := st.shouldCreateOutcome(signal)
	if !shouldCreate {
		log.Printf("⏭️ Skipping signal %d (%s): %s", signal.ID, signal.StockSymbol, reason)
		st.createSkippedOutcome(signal, reason)
		return false, nil
	}

	session := getTradingSession(signal.GeneratedAt)
	log.Printf("✅ Creating outcome for signal %d (%s) - Session: %s (Time Mult: %.2fx)",
		signal.ID, signal.StockSymbol, session, timeMultiplier)

	if timeReason != "" {
		log.Printf("   └─ %s", timeReason)
	}

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

// OPTIMIZATION HELPER FUNCTIONS

// checkOrderFlowImbalance validates order flow for BUY signals
// Returns: (passed bool, confidenceMultiplier float64, reason string)
func (st *SignalTracker) checkOrderFlowImbalance(symbol string, decision string) (bool, float64, string) {
	if decision != "BUY" {
		return true, 1.0, "" // Only check for BUY signals
	}

	orderFlow, err := st.repo.GetLatestOrderFlow(symbol)
	if err != nil || orderFlow == nil {
		// No order flow data - allow but don't boost
		return true, 1.0, ""
	}

	totalVolume := orderFlow.BuyVolumeLots + orderFlow.SellVolumeLots
	if totalVolume == 0 {
		return true, 1.0, ""
	}

	// Calculate buy pressure
	buyPressure := (orderFlow.BuyVolumeLots / totalVolume)

	// Check aggressive buyers if available
	if orderFlow.AggressiveBuyPct != nil && *orderFlow.AggressiveBuyPct < AggressiveBuyThreshold {
		// Only penalize if buy pressure is also weak
		if buyPressure < 0.6 {
			return false, 0.7, fmt.Sprintf("Low aggressive buy pressure (%.1f%% < %.0f%%)", *orderFlow.AggressiveBuyPct, AggressiveBuyThreshold)
		}
	}

	// Check buy imbalance
	if buyPressure < OrderFlowBuyThreshold {
		return false, 0.7, fmt.Sprintf("Insufficient buy pressure (%.1f%% < %.0f%%)", buyPressure*100, OrderFlowBuyThreshold*100)
	}

	// Strong buy pressure - boost confidence
	if buyPressure > 0.6 {
		return true, 1.3, "Strong buy pressure detected"
	}

	return true, 1.0, ""
}

// checkBaselineQuality validates statistical baseline quality
// Returns: (passed bool, confidenceMultiplier float64, reason string)
func (st *SignalTracker) checkBaselineQuality(symbol string) (bool, float64, string) {
	// OPTIMIZATION: Try Redis cache first (5-minute TTL)
	if st.redis != nil {
		ctx := context.Background()
		cacheKey := fmt.Sprintf("baseline:%s", symbol)

		var baseline database.StatisticalBaseline
		if err := st.redis.Get(ctx, cacheKey, &baseline); err == nil {
			// Cache hit - use cached baseline
			return st.evaluateBaselineQuality(&baseline)
		}
	}

	// Cache miss or no Redis - fetch from database
	baseline, err := st.repo.GetLatestBaseline(symbol)
	if err != nil || baseline == nil {
		return false, 0.0, "No statistical baseline available"
	}

	// Cache the result for 5 minutes
	if st.redis != nil {
		ctx := context.Background()
		cacheKey := fmt.Sprintf("baseline:%s", symbol)
		if err := st.redis.Set(ctx, cacheKey, baseline, 5*time.Minute); err != nil {
			log.Printf("⚠️ Failed to cache baseline for %s: %v", symbol, err)
		}
	}

	return st.evaluateBaselineQuality(baseline)
}

// evaluateBaselineQuality performs the actual baseline quality evaluation
func (st *SignalTracker) evaluateBaselineQuality(baseline *database.StatisticalBaseline) (bool, float64, string) {
	// Strict quality check
	if baseline.SampleSize < MinBaselineSampleSize {
		return false, 0.0, fmt.Sprintf("Insufficient baseline data (%d < %d trades)", baseline.SampleSize, MinBaselineSampleSize)
	}

	// Penalize weak baselines
	if baseline.SampleSize < MinBaselineSampleSizeStrict {
		return true, 0.6, fmt.Sprintf("Limited baseline data (%d trades)", baseline.SampleSize)
	}

	// Good baseline quality
	return true, 1.0, ""
}

// getTimeOfDayMultiplier returns confidence multiplier based on trading session
func (st *SignalTracker) getTimeOfDayMultiplier(t time.Time) (float64, string) {
	loc, _ := time.LoadLocation(MarketTimeZone)
	if loc == nil {
		loc = time.FixedZone("WIB", 7*60*60)
	}

	localTime := t.In(loc)
	hour := localTime.Hour()
	session := getTradingSession(t)

	// Skip low-liquidity sessions
	if session == "PRE_OPENING" || session == "LUNCH_BREAK" || session == "POST_MARKET" {
		return 0.0, fmt.Sprintf("Low liquidity session: %s", session)
	}

	// Morning momentum boost (before 10:00)
	if session == "SESSION_1" && hour < MorningBoostHour {
		return 1.2, "Morning momentum period"
	}

	// Afternoon caution (after 14:00)
	if session == "SESSION_2" && hour >= AfternoonCautionHour {
		return 0.8, "Afternoon caution period"
	}

	return 1.0, ""
}

// getStrategyPerformanceMultiplier calculates confidence adjustment based on strategy win rate
// Returns: (multiplier float64, shouldSkip bool, reason string)
func (st *SignalTracker) getStrategyPerformanceMultiplier(strategy string) (float64, bool, string) {
	// OPTIMIZATION: Try Redis cache first (5-minute TTL) - Quick Win #3
	if st.redis != nil {
		ctx := context.Background()
		cacheKey := fmt.Sprintf("strategy:perf:%s", strategy)

		type CachedPerf struct {
			Multiplier float64
			ShouldSkip bool
			Reason     string
		}

		var cached CachedPerf
		if err := st.redis.Get(ctx, cacheKey, &cached); err == nil {
			// Cache hit
			return cached.Multiplier, cached.ShouldSkip, cached.Reason
		}
	}

	// Cache miss or no Redis - calculate from database
	multiplier, shouldSkip, reason := st.calculateStrategyPerformance(strategy)

	// Cache the result for 5 minutes
	if st.redis != nil {
		ctx := context.Background()
		cacheKey := fmt.Sprintf("strategy:perf:%s", strategy)

		cached := struct {
			Multiplier float64
			ShouldSkip bool
			Reason     string
		}{
			Multiplier: multiplier,
			ShouldSkip: shouldSkip,
			Reason:     reason,
		}

		if err := st.redis.Set(ctx, cacheKey, cached, 5*time.Minute); err != nil {
			log.Printf("⚠️ Failed to cache strategy performance for %s: %v", strategy, err)
		}
	}

	return multiplier, shouldSkip, reason
}

// calculateStrategyPerformance performs the actual strategy performance calculation
func (st *SignalTracker) calculateStrategyPerformance(strategy string) (float64, bool, string) {
	// Get recent outcomes for this strategy (last 7 days)
	since := time.Now().Add(-7 * 24 * time.Hour)
	outcomes, err := st.repo.GetSignalOutcomes("", "", since, time.Time{}, 0)
	if err != nil {
		return 1.0, false, ""
	}

	// Filter by strategy and count wins/losses
	var totalSignals, wins, losses int
	for _, outcome := range outcomes {
		signal, err := st.repo.GetSignalByID(outcome.SignalID)
		if err != nil || signal == nil || signal.Strategy != strategy {
			continue
		}

		if outcome.OutcomeStatus == "WIN" || outcome.OutcomeStatus == "LOSS" || outcome.OutcomeStatus == "BREAKEVEN" {
			totalSignals++
			if outcome.OutcomeStatus == "WIN" {
				wins++
			} else if outcome.OutcomeStatus == "LOSS" {
				losses++
			}
		}
	}

	// Need minimum signals for evaluation
	if totalSignals < MinStrategySignals {
		return 1.0, false, ""
	}

	winRate := float64(wins) / float64(totalSignals) * 100

	// Disable underperforming strategies
	if winRate < float64(LowWinRateThreshold) {
		return 0.0, true, fmt.Sprintf("Strategy %s underperforming (WR: %.1f%% < %d%%)", strategy, winRate, LowWinRateThreshold)
	}

	// Boost high-performing strategies
	if winRate > float64(HighWinRateThreshold) {
		return 1.2, false, fmt.Sprintf("Strategy %s performing well (WR: %.1f%%)", strategy, winRate)
	}

	// Moderate performance
	if winRate < 45 {
		return 0.9, false, fmt.Sprintf("Strategy %s moderate performance (WR: %.1f%%)", strategy, winRate)
	}

	return 1.0, false, ""
}

// getRegimeAdaptiveLimit returns max positions based on market regime
func (st *SignalTracker) getRegimeAdaptiveLimit(symbol string) int {
	regime, err := st.repo.GetLatestRegime(symbol)
	if err != nil || regime == nil {
		return MaxOpenPositions // Default
	}

	// Aggressive in trending markets
	if regime.Regime == "TRENDING_UP" && regime.Confidence > 0.7 {
		return 15
	}

	// Conservative in volatile markets
	if regime.Regime == "VOLATILE" {
		if regime.ATR != nil && regime.Volatility != nil && *regime.Volatility > 3.0 {
			return 5
		}
	}

	// Moderate in ranging markets
	if regime.Regime == "RANGING" {
		return 8
	}

	return MaxOpenPositions
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

// ============================================================================
// SIGNAL OPTIMIZATION FUNCTIONS (Historical Data Driven)
// ============================================================================

// getDynamicConfidenceThreshold returns the optimal confidence threshold for a strategy
// based on historical data. This replaces static thresholds with data-driven values.
func (st *SignalTracker) getDynamicConfidenceThreshold(strategy string) (float64, string) {
	// OPTIMIZATION: Try Redis cache first (10-minute TTL)
	if st.redis != nil {
		ctx := context.Background()
		cacheKey := fmt.Sprintf("opt:threshold:%s", strategy)

		type CachedThreshold struct {
			Threshold float64
			Reason    string
		}

		var cached CachedThreshold
		if err := st.redis.Get(ctx, cacheKey, &cached); err == nil {
			return cached.Threshold, cached.Reason
		}
	}

	// Fetch from database (last 30 days)
	thresholds, err := st.repo.GetOptimalConfidenceThresholds(30)
	if err != nil || len(thresholds) == 0 {
		return 0.6, "Using default threshold (no historical data)"
	}

	// Find threshold for this strategy
	var optThreshold float64 = 0.6
	var reason string = "Using default threshold"
	for _, t := range thresholds {
		if t.Strategy == strategy {
			optThreshold = t.RecommendedMinConf
			reason = fmt.Sprintf("Optimal threshold %.0f%% based on %d signals (win rate %.1f%%)",
				t.OptimalConfidence*100, t.SampleSize, t.WinRateAtThreshold)
			break
		}
	}

	// Cache the result for 10 minutes
	if st.redis != nil {
		ctx := context.Background()
		cacheKey := fmt.Sprintf("opt:threshold:%s", strategy)
		cached := struct {
			Threshold float64
			Reason    string
		}{Threshold: optThreshold, Reason: reason}

		if err := st.redis.Set(ctx, cacheKey, cached, 10*time.Minute); err != nil {
			log.Printf("⚠️ Failed to cache optimal threshold for %s: %v", strategy, err)
		}
	}

	return optThreshold, reason
}

// checkRegimeEffectiveness validates that a strategy performs well in the current regime
// Returns: (passed bool, reason string)
func (st *SignalTracker) checkRegimeEffectiveness(strategy string, symbol string) (bool, string) {
	// Get current regime
	regime, err := st.repo.GetLatestRegime(symbol)
	if err != nil || regime == nil {
		return true, "" // No regime data - allow
	}

	// OPTIMIZATION: Try Redis cache first (10-minute TTL)
	cacheKey := fmt.Sprintf("opt:regime_eff:%s:%s", strategy, regime.Regime)
	if st.redis != nil {
		ctx := context.Background()
		var cachedWinRate float64
		if err := st.redis.Get(ctx, cacheKey, &cachedWinRate); err == nil {
			if cachedWinRate < 40.0 {
				return false, fmt.Sprintf("%s underperforms in %s regime (%.1f%% win rate)",
					strategy, regime.Regime, cachedWinRate)
			}
			return true, ""
		}
	}

	// Fetch effectiveness data
	effectiveness, err := st.repo.GetStrategyEffectivenessByRegime(30)
	if err != nil || len(effectiveness) == 0 {
		return true, "" // No historical data - allow
	}

	// Find win rate for this strategy + regime combination
	var winRate float64 = 50.0 // Default to neutral
	for _, eff := range effectiveness {
		if eff.Strategy == strategy && eff.MarketRegime == regime.Regime {
			winRate = eff.WinRate
			break
		}
	}

	// Cache the result
	if st.redis != nil {
		ctx := context.Background()
		if err := st.redis.Set(ctx, cacheKey, winRate, 10*time.Minute); err != nil {
			log.Printf("⚠️ Failed to cache regime effectiveness: %v", err)
		}
	}

	// Reject if win rate is below 40% in this regime
	if winRate < 40.0 {
		return false, fmt.Sprintf("%s underperforms in %s regime (%.1f%% win rate)",
			strategy, regime.Regime, winRate)
	}

	return true, ""
}

// checkSignalExpectedValue validates that a strategy has positive expected value
// EV = (Win Rate × Avg Win) - ((1 - Win Rate) × |Avg Loss|)
// Returns: (passed bool, ev float64, reason string)
func (st *SignalTracker) checkSignalExpectedValue(strategy string) (bool, float64, string) {
	// OPTIMIZATION: Try Redis cache first (10-minute TTL)
	cacheKey := fmt.Sprintf("opt:ev:%s", strategy)
	if st.redis != nil {
		ctx := context.Background()
		var cachedEV float64
		if err := st.redis.Get(ctx, cacheKey, &cachedEV); err == nil {
			if cachedEV < 0 {
				return false, cachedEV, fmt.Sprintf("Negative EV: %.4f (strategy not profitable)", cachedEV)
			}
			return true, cachedEV, ""
		}
	}

	// Fetch expected value data
	evData, err := st.repo.GetSignalExpectedValues(30)
	if err != nil || len(evData) == 0 {
		return true, 0, "" // No data - allow
	}

	// Find EV for this strategy
	var ev float64 = 0.0
	var found bool
	for _, e := range evData {
		if e.Strategy == strategy {
			ev = e.ExpectedValue
			found = true
			break
		}
	}

	// Cache the result
	if st.redis != nil && found {
		ctx := context.Background()
		if err := st.redis.Set(ctx, cacheKey, ev, 10*time.Minute); err != nil {
			log.Printf("⚠️ Failed to cache EV: %v", err)
		}
	}

	// Reject negative EV strategies
	if found && ev < 0 {
		return false, ev, fmt.Sprintf("Negative EV: %.4f (strategy not profitable)", ev)
	}

	return true, ev, ""
}

package app

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"stockbit-haka-haki/cache"
	"stockbit-haka-haki/config"
	"stockbit-haka-haki/database"
	models "stockbit-haka-haki/database/models_pkg"
)

// SignalFilter is an interface for individual signal filtering logic
type SignalFilter interface {
	Name() string
	Evaluate(ctx context.Context, signal *database.TradingSignalDB) (shouldPass bool, reason string, multiplier float64)
}

// SignalFilterService handles the complex decision logic using a pipeline of filters
type SignalFilterService struct {
	repo    *database.TradeRepository
	redis   *cache.RedisClient
	cfg     *config.Config
	filters []SignalFilter
}

// NewSignalFilterService creates a new signal filter service
func NewSignalFilterService(repo *database.TradeRepository, redis *cache.RedisClient, cfg *config.Config) *SignalFilterService {
	service := &SignalFilterService{
		repo:  repo,
		redis: redis,
		cfg:   cfg,
	}

	// Register filters in order
	service.filters = []SignalFilter{
		&StrategyPerformanceFilter{repo: repo, redis: redis, cfg: cfg},
		&DynamicConfidenceFilter{repo: repo, redis: redis, cfg: cfg},
		&OrderFlowFilter{repo: repo, redis: redis, cfg: cfg},
		&TimeOfDayFilter{},
	}

	return service
}

// Evaluate determines if a signal should be traded by running it through the filter pipeline
// Also determines if signal is suitable for swing trading
func (s *SignalFilterService) Evaluate(signal *database.TradingSignalDB) (bool, string, float64) {
	ctx := context.Background()
	overallMultiplier := 1.0

	for _, filter := range s.filters {
		passed, reason, multiplier := filter.Evaluate(ctx, signal)

		if !passed {
			return false, reason, 0.0
		}

		// Apply multiplier if passed
		if multiplier != 0.0 && multiplier != 1.0 {
			overallMultiplier *= multiplier
			log.Printf("   └─ %s modifier: %.2fx (%s)", filter.Name(), multiplier, reason)
		} else if reason != "" {
			// Log important info even if multiplier is neutral
			log.Printf("   └─ %s info: %s", filter.Name(), reason)
		}
	}

	// Final validation on zero multiplier
	if overallMultiplier == 0.0 {
		return false, "Calculated probability is zero", 0.0
	}

	return true, "", overallMultiplier
}

// GetRegimeAdaptiveLimit returns max positions based on market regime
// Kept as a separate public method for external usage
func (s *SignalFilterService) GetRegimeAdaptiveLimit(symbol string) int {
	return s.cfg.Trading.MaxOpenPositions
}

// ============================================================================
// INDIVIDUAL FILTERS
// ============================================================================

// 1. Strategy Performance & Baseline Quality Filter (combined)
type StrategyPerformanceFilter struct {
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
}

func (f *StrategyPerformanceFilter) Name() string { return "Strategy & Baseline Performance" }

func (f *StrategyPerformanceFilter) Evaluate(ctx context.Context, signal *database.TradingSignalDB) (bool, string, float64) {
	strategy := signal.Strategy

	if f.redis != nil {
		cacheKey := fmt.Sprintf("strategy:perf:%s", strategy)
		type CachedPerf struct {
			Multiplier float64
			ShouldSkip bool
			Reason     string
		}
		var cached CachedPerf
		if err := f.redis.Get(ctx, cacheKey, &cached); err == nil {
			if cached.ShouldSkip {
				return false, cached.Reason, 0.0
			}
			return true, cached.Reason, cached.Multiplier
		}
	}

	multiplier, shouldSkip, reason := f.calculate(strategy, signal.StockSymbol)

	if f.redis != nil {
		cacheKey := fmt.Sprintf("strategy:perf:%s", strategy)
		cached := struct {
			Multiplier float64
			ShouldSkip bool
			Reason     string
		}{Multiplier: multiplier, ShouldSkip: shouldSkip, Reason: reason}
		_ = f.redis.Set(ctx, cacheKey, cached, 5*time.Minute)
	}

	if shouldSkip {
		return false, reason, 0.0
	}
	return true, reason, multiplier
}

func (f *StrategyPerformanceFilter) calculate(strategy string, symbol string) (float64, bool, string) {
	// Get baseline data first
	baseline, err := f.repo.GetLatestBaseline(symbol)
	baselineMultiplier := 1.0
	var baselineReason string

	if err != nil || baseline == nil {
		// No baseline data is a critical issue - reject signal
		return 0.0, true, "No statistical baseline available"
	}

	// STRICT: Reject if baseline sample size is insufficient
	if baseline.SampleSize < f.cfg.Trading.MinBaselineSampleSize {
		return 0.0, true, fmt.Sprintf("Insufficient baseline data (%d < %d trades)", baseline.SampleSize, f.cfg.Trading.MinBaselineSampleSize)
	}

	// Reduce multiplier for limited baseline
	if baseline.SampleSize < f.cfg.Trading.MinBaselineSampleSizeStrict {
		baselineMultiplier = 0.7 // Reduced from 0.6 for better quality signals
		baselineReason = fmt.Sprintf("Limited baseline data (%d trades)", baseline.SampleSize)
	}

	// NEW: Check baseline recency (must be calculated within last 2 hours)
	if time.Since(baseline.CalculatedAt) > 2*time.Hour {
		baselineMultiplier *= 0.9
		baselineReason += "; Stale baseline (>2h old)"
	}

	// Get strategy performance data
	outcomes, err := f.repo.GetSignalOutcomes(symbol, "", time.Now().Add(-24*time.Hour), time.Time{}, 0, 0)
	if err != nil {
		return baselineMultiplier, false, baselineReason
	}

	var totalSignals, wins int
	for _, outcome := range outcomes {
		signal, err := f.repo.GetSignalByID(outcome.SignalID)
		if err == nil && signal != nil && signal.Strategy == strategy {
			if outcome.OutcomeStatus == "WIN" || outcome.OutcomeStatus == "LOSS" || outcome.OutcomeStatus == "BREAKEVEN" {
				totalSignals++
				if outcome.OutcomeStatus == "WIN" {
					wins++
				}
			}
		}
	}

	if totalSignals < f.cfg.Trading.MinStrategySignals {
		return baselineMultiplier, false, baselineReason
	}

	winRate := float64(wins) / float64(totalSignals) * 100
	var strategyReason string
	strategyMultiplier := 1.0

	// STRICT: Reject if strategy is underperforming
	if winRate < f.cfg.Trading.LowWinRateThreshold {
		return 0.0, true, fmt.Sprintf("Strategy %s underperforming (WR: %.1f%% < %.0f%%)", strategy, winRate, f.cfg.Trading.LowWinRateThreshold)
	}

	// NEW: Check for consecutive losses (circuit breaker logic)
	recentOutcomes, _ := f.repo.GetSignalOutcomes("", "", time.Now().Add(-24*time.Hour), time.Time{}, 20, 0)
	consecutiveLosses := 0
	for _, outcome := range recentOutcomes {
		signal, err := f.repo.GetSignalByID(outcome.SignalID)
		if err == nil && signal != nil && signal.Strategy == strategy {
			if outcome.OutcomeStatus == "LOSS" {
				consecutiveLosses++
			} else if outcome.OutcomeStatus == "WIN" {
				break // Reset counter on win
			}
		}
	}
	if consecutiveLosses >= f.cfg.Trading.MaxConsecutiveLosses {
		return 0.0, true, fmt.Sprintf("Strategy %s hit circuit breaker (%d consecutive losses)", strategy, consecutiveLosses)
	}

	// Multiplier based on performance
	if winRate > f.cfg.Trading.HighWinRateThreshold {
		strategyMultiplier = 1.25
		strategyReason = fmt.Sprintf("Strategy %s excellent (WR: %.1f%%)", strategy, winRate)
	} else if winRate > 55 {
		strategyMultiplier = 1.1
		strategyReason = fmt.Sprintf("Strategy %s good (WR: %.1f%%)", strategy, winRate)
	} else if winRate >= f.cfg.Trading.LowWinRateThreshold {
		strategyMultiplier = 1.0
		strategyReason = fmt.Sprintf("Strategy %s acceptable (WR: %.1f%%)", strategy, winRate)
	}

	// Combine multipliers and reasons
	finalMultiplier := baselineMultiplier * strategyMultiplier
	var finalReason string
	if baselineReason != "" && strategyReason != "" {
		finalReason = baselineReason + "; " + strategyReason
	} else if baselineReason != "" {
		finalReason = baselineReason
	} else {
		finalReason = strategyReason
	}

	return finalMultiplier, false, finalReason
}

// 2. Dynamic Confidence Filter
type DynamicConfidenceFilter struct {
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
}

func (f *DynamicConfidenceFilter) Name() string { return "Dynamic Confidence" }

func (f *DynamicConfidenceFilter) Evaluate(ctx context.Context, signal *database.TradingSignalDB) (bool, string, float64) {
	// Calculate Volume Z-Score Multiplier (High Volume = Higher Confidence)
	isHighVolume := signal.VolumeZScore > 3.0     // Increased from 2.5
	isVeryHighVolume := signal.VolumeZScore > 4.0 // NEW

	// Trend Alignment Check (Price vs VWAP)
	isTrendAligned := false
	baseline, _ := f.repo.GetLatestBaseline(signal.StockSymbol)
	if baseline != nil && baseline.MeanVolumeLots > 0 {
		vwap := baseline.MeanValue / baseline.MeanVolumeLots
		if signal.TriggerPrice > vwap {
			isTrendAligned = true
		}
	}

	// STRICT: For BUY signals, must be above VWAP (trend aligned)
	if signal.Decision == "BUY" && !isTrendAligned {
		return false, "BUY signal rejected: Price below VWAP (counter-trend)", 0.0
	}

	optimalThreshold, thresholdReason := f.getOptimalThreshold(ctx, signal.Strategy)

	// ENHANCED: Adaptive thresholds - only relax for very strong signals
	confidenceMultiplier := 1.0
	if isVeryHighVolume && isTrendAligned {
		// Exceptional signal: very high volume + trend aligned
		optimalThreshold *= 0.85
		confidenceMultiplier = 1.3
		thresholdReason += " (Strong signal: High volume + Trend aligned)"
	} else if isHighVolume && isTrendAligned {
		// Good signal: high volume + trend aligned
		optimalThreshold *= 0.92
		confidenceMultiplier = 1.15
		thresholdReason += " (Good signal: Above average volume)"
	}
	// Removed: No relaxation for trend-only signals (too risky)

	if signal.Confidence < optimalThreshold {
		return false, fmt.Sprintf("Below optimal confidence threshold (%.2f < %.2f): %s",
			signal.Confidence, optimalThreshold, thresholdReason), 0.0
	}

	return true, "", confidenceMultiplier
}

func (f *DynamicConfidenceFilter) getOptimalThreshold(ctx context.Context, strategy string) (float64, string) {
	if f.redis != nil {
		cacheKey := fmt.Sprintf("opt:threshold:%s", strategy)
		type CachedThreshold struct {
			Threshold float64
			Reason    string
		}
		var cached CachedThreshold
		if err := f.redis.Get(ctx, cacheKey, &cached); err == nil {
			return cached.Threshold, cached.Reason
		}
	}

	thresholds, err := f.repo.GetOptimalConfidenceThresholds(30)
	if err != nil || len(thresholds) == 0 {
		return 0.5, "Using default threshold (no historical data)"
	}

	var optThreshold float64 = 0.5
	var reason string = "Using default threshold"
	for _, t := range thresholds {
		if t.Strategy == strategy {
			optThreshold = t.RecommendedMinConf
			reason = fmt.Sprintf("Optimal threshold %.0f%% based on %d signals (win rate %.1f%%)",
				t.OptimalConfidence*100, t.SampleSize, t.WinRateAtThreshold)
			break
		}
	}

	if f.redis != nil {
		cacheKey := fmt.Sprintf("opt:threshold:%s", strategy)
		cached := struct {
			Threshold float64
			Reason    string
		}{Threshold: optThreshold, Reason: reason}
		_ = f.redis.Set(ctx, cacheKey, cached, 10*time.Minute)
	}

	return optThreshold, reason
}

// 3. Order Flow Filter
type OrderFlowFilter struct {
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
}

func (f *OrderFlowFilter) Name() string { return "Order Flow" }

func (f *OrderFlowFilter) Evaluate(ctx context.Context, signal *database.TradingSignalDB) (bool, string, float64) {
	if signal.Decision != "BUY" {
		return true, "", 1.0
	}

	orderFlow, err := f.repo.GetLatestOrderFlow(signal.StockSymbol)
	if err != nil || orderFlow == nil {
		if f.cfg.Trading.RequireOrderFlow {
			return false, "Order flow data missing (Fail-Safe triggered)", 0.0
		}
		return true, "", 1.0
	}

	totalVolume := orderFlow.BuyVolumeLots + orderFlow.SellVolumeLots
	if totalVolume == 0 {
		return true, "", 1.0
	}

	if f.cfg.Trading.RequireOrderFlow && totalVolume < 100 {
		return false, "Insufficient order flow volume (Fail-Safe)", 0.0
	}

	buyPressure := (orderFlow.BuyVolumeLots / totalVolume)

	if orderFlow.AggressiveBuyPct != nil && *orderFlow.AggressiveBuyPct < f.cfg.Trading.AggressiveBuyThreshold {
		if buyPressure < 0.6 {
			return false, fmt.Sprintf("Low aggressive buy pressure (%.1f%% < %.0f%%)", *orderFlow.AggressiveBuyPct, f.cfg.Trading.AggressiveBuyThreshold), 0.7
		}
	}

	// Dynamic Threshold Logic
	requiredThreshold := f.cfg.Trading.OrderFlowBuyThreshold

	// Check Trend Alignment (Reuse cached baseline if available)
	isTrendAligned := false
	if f.redis != nil {
		cacheKey := fmt.Sprintf("baseline:%s", signal.StockSymbol)
		var baseline database.StatisticalBaseline
		if err := f.redis.Get(ctx, cacheKey, &baseline); err == nil {
			if baseline.MeanVolumeLots > 0 {
				vwap := baseline.MeanValue / baseline.MeanVolumeLots
				if signal.TriggerPrice > vwap {
					isTrendAligned = true
				}
			}
		}
	}

	// EXPERT MODE: Remove relaxation. Demand Order Flow to be good regardless of trend.
	// Relaxation logic removed.
	if !isTrendAligned {
		// If NOT trend aligned, we actually want HIGHER buy pressure
		requiredThreshold *= 1.1
	}

	// Whale Alignment Check (validate against institutional activity)
	recentWhales, err := f.repo.GetHistoricalWhales(
		signal.StockSymbol,
		time.Now().Add(-15*time.Minute),
		time.Now(),
		"", "", "", 0, 10, 0,
	)

	if err == nil && len(recentWhales) > 0 {
		whaleBuyCount := 0
		whaleSellCount := 0
		var totalWhaleValue float64

		for _, whale := range recentWhales {
			switch whale.Action {
			case "BUY":
				whaleBuyCount++
				totalWhaleValue += whale.TriggerValue
			case "SELL":
				whaleSellCount++
			}
		}

		// BOOST: Whale alignment (whales buying, we're buying)
		if signal.Decision == "BUY" && whaleBuyCount > whaleSellCount {
			// Strong whale buying activity
			if whaleBuyCount >= 3 || totalWhaleValue > 500_000_000 { // 500M IDR
				return true, fmt.Sprintf("Strong whale alignment: %d BUY whales (%.0fM IDR)", whaleBuyCount, totalWhaleValue/1_000_000), 1.5
			}
			// Moderate whale buying
			return true, fmt.Sprintf("Whale alignment: %d BUY whales", whaleBuyCount), 1.3
		}

		// REJECT: Whale divergence (whales selling, we're buying)
		if signal.Decision == "BUY" && whaleSellCount > whaleBuyCount && whaleSellCount >= 2 {
			return false, fmt.Sprintf("Whale divergence: %d SELL whales detected", whaleSellCount), 0.0
		}
	}

	if buyPressure < requiredThreshold {
		if orderFlow.AggressiveBuyPct != nil && *orderFlow.AggressiveBuyPct > f.cfg.Trading.AggressiveBuyThreshold {
			if buyPressure > 0.50 { // Increased from 0.45
				return true, fmt.Sprintf("Aggressive buying detected (%.1f%%)", *orderFlow.AggressiveBuyPct), 1.15
			}
		}
		return false, fmt.Sprintf("Insufficient buy pressure (%.1f%% < %.0f%%)", buyPressure*100, requiredThreshold*100), 0.0
	}

	// Enhanced multipliers based on buy pressure strength
	if buyPressure > 0.70 {
		return true, "Very strong buy pressure", 1.4
	} else if buyPressure > 0.60 {
		return true, "Strong buy pressure detected", 1.25
	} else if buyPressure > 0.55 {
		return true, "Moderate buy pressure", 1.1
	}

	return true, "", 1.0
}

// 7. Time of Day Filter
type TimeOfDayFilter struct{}

func (f *TimeOfDayFilter) Name() string { return "Time of Day" }

func (f *TimeOfDayFilter) Evaluate(ctx context.Context, signal *database.TradingSignalDB) (bool, string, float64) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*60*60)
	}

	localTime := signal.GeneratedAt.In(loc)
	hour := localTime.Hour()
	minute := localTime.Minute()

	// Helper for session determination
	var session string
	if hour == 8 && minute >= 45 {
		session = "PRE_OPENING"
	} else if hour >= 9 && hour < 12 {
		session = "SESSION_1"
	} else if (hour == 12) || (hour == 13 && minute < 30) {
		session = "LUNCH_BREAK"
	} else if (hour == 13 && minute >= 30) || (hour == 14 && minute < 50) {
		session = "SESSION_2"
	} else if hour == 14 && minute >= 50 {
		session = "PRE_CLOSING"
	} else if hour >= 15 && hour < 16 {
		session = "POST_MARKET"
	} else {
		session = "AFTER_HOURS"
	}

	if session == "PRE_OPENING" || session == "LUNCH_BREAK" || session == "POST_MARKET" {
		return false, fmt.Sprintf("Low liquidity session: %s", session), 0.0
	}

	// STRICT: Avoid first 15 minutes of session 1 (09:00-09:15) - high volatility
	if session == "SESSION_1" && hour == 9 && minute < 15 {
		return false, "First 15 minutes - high volatility", 0.0
	}

	// STRICT: Avoid last 30 minutes before lunch (11:30-12:00)
	if session == "SESSION_1" && hour == 11 && minute >= 30 {
		return false, "Approaching lunch break - low liquidity", 0.0
	}

	// STRICT: Avoid first 15 minutes after lunch (13:30-13:45)
	if session == "SESSION_2" && hour == 13 && minute < 45 {
		return false, "Post-lunch volatility", 0.0
	}

	// Best trading windows
	if session == "SESSION_1" && hour >= 10 && hour < 11 {
		// 10:00-11:00: Best morning window
		return true, "Optimal morning window", 1.25
	}

	if session == "SESSION_1" && hour >= 9 && minute >= 15 {
		// 09:15-12:00 (excluding 11:30-12:00)
		return true, "Good morning session", 1.1
	}

	if session == "SESSION_2" && hour >= 13 && minute >= 45 && hour < 14 {
		// 13:45-14:00
		return true, "Post-lunch recovery", 1.0
	}

	if session == "SESSION_2" && hour >= 14 && hour < 14 {
		// 14:00-14:50 (avoid pre-closing)
		return true, "Afternoon session", 0.9
	}

	// Pre-closing (14:50-15:00) - only allow if very strong signal
	if session == "PRE_CLOSING" {
		return true, "Pre-closing", 0.7
	}

	return true, "", 1.0
}

// SwingTradingEvaluator evaluates if a signal is suitable for swing trading
// This is not a filter but an evaluator that adds metadata to the signal
type SwingTradingEvaluator struct {
	repo *database.TradeRepository
	cfg  *config.Config
}

func NewSwingTradingEvaluator(repo *database.TradeRepository, cfg *config.Config) *SwingTradingEvaluator {
	return &SwingTradingEvaluator{repo: repo, cfg: cfg}
}

// EvaluateSwingPotential checks if signal meets swing trading criteria
// Returns: (isSwing bool, swingScore float64, reason string)
func (ste *SwingTradingEvaluator) EvaluateSwingPotential(signal *database.TradingSignalDB) (bool, float64, string) {
	if !ste.cfg.Trading.EnableSwingTrading {
		return false, 0, "Swing trading disabled"
	}

	// 1. Check confidence threshold for swing (higher than day trading)
	if signal.Confidence < ste.cfg.Trading.SwingMinConfidence {
		return false, 0, fmt.Sprintf("Confidence %.2f below swing threshold %.2f",
			signal.Confidence, ste.cfg.Trading.SwingMinConfidence)
	}

	// 2. Check if we have enough daily baseline data
	baseline, err := ste.repo.GetLatestBaseline(signal.StockSymbol)
	if err != nil || baseline == nil {
		return false, 0, "No baseline data available"
	}

	// Check sample size converted to days (assuming ~20 samples per day for active stocks)
	minSamples := ste.cfg.Trading.SwingMinBaselineDays * 20
	if baseline.SampleSize < minSamples {
		return false, 0, fmt.Sprintf("Insufficient history: %d samples (need %d)",
			baseline.SampleSize, minSamples)
	}

	// 3. Calculate trend strength
	trendScore := ste.calculateTrendStrength(signal, baseline)
	if ste.cfg.Trading.SwingRequireTrend && trendScore < 0.6 {
		return false, trendScore, fmt.Sprintf("Trend strength %.2f below threshold 0.6", trendScore)
	}

	// 4. Calculate volume confirmation
	volumeScore := ste.calculateVolumeConfirmation(signal, baseline)

	// 5. Calculate overall swing score
	swingScore := (signal.Confidence*0.4 + trendScore*0.4 + volumeScore*0.2)

	// Require minimum swing score
	if swingScore < 0.65 {
		return false, swingScore, fmt.Sprintf("Swing score %.2f below threshold 0.65", swingScore)
	}

	return true, swingScore, fmt.Sprintf("Strong swing candidate: score=%.2f (trend=%.2f, vol=%.2f)",
		swingScore, trendScore, volumeScore)
}

// calculateTrendStrength determines trend strength for swing trading
func (ste *SwingTradingEvaluator) calculateTrendStrength(signal *database.TradingSignalDB, baseline *models.StatisticalBaseline) float64 {
	// Price above VWAP is good
	priceVsVWAP := 0.0
	if baseline.MeanVolumeLots > 0 {
		vwap := baseline.MeanValue / baseline.MeanVolumeLots
		if signal.TriggerPrice > vwap {
			priceVsVWAP = (signal.TriggerPrice - vwap) / vwap * 100
		}
	}

	// Normalize to 0-1 score
	trendScore := math.Min(priceVsVWAP/5.0, 1.0) // 5% above VWAP = full score

	// Price Z-score contribution
	priceZContribution := math.Min(math.Abs(signal.PriceZScore)/3.0, 1.0) * 0.5
	if signal.PriceZScore < 0 {
		priceZContribution *= 0.5 // Penalty for negative Z-score
	}

	return math.Min((trendScore*0.6 + priceZContribution*0.4), 1.0)
}

// calculateVolumeConfirmation checks volume pattern for swing
func (ste *SwingTradingEvaluator) calculateVolumeConfirmation(signal *database.TradingSignalDB, baseline *models.StatisticalBaseline) float64 {
	// High volume Z-score is good for swing confirmation
	volScore := math.Min(signal.VolumeZScore/4.0, 1.0)

	// Compare to baseline
	baselineVolRatio := 0.0
	if baseline.MeanVolumeLots > 0 {
		baselineVolRatio = signal.TriggerVolumeLots / baseline.MeanVolumeLots
	}

	// Normalize: 3x average volume = full score
	baselineScore := math.Min(baselineVolRatio/3.0, 1.0)

	return volScore*0.7 + baselineScore*0.3
}

// IsSwingSignal determines if a signal should be treated as swing trade
// This can be called separately after the main filter pipeline
func (s *SignalFilterService) IsSwingSignal(signal *database.TradingSignalDB) (bool, float64, string) {
	evaluator := NewSwingTradingEvaluator(s.repo, s.cfg)
	return evaluator.EvaluateSwingPotential(signal)
}

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
		&BaselineQualityFilter{repo: repo, redis: redis, cfg: cfg},
		&DynamicConfidenceFilter{repo: repo, redis: redis, cfg: cfg},
		&RegimeEffectivenessFilter{repo: repo, redis: redis, cfg: cfg},
		&ExpectedValueFilter{repo: repo, redis: redis, cfg: cfg},
		&OrderFlowFilter{repo: repo, redis: redis, cfg: cfg},
		&TimeOfDayFilter{},
	}

	return service
}

// Evaluate determines if a signal should be traded by running it through the filter pipeline
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
	regime, err := s.repo.GetLatestRegime(symbol)
	if err != nil || regime == nil {
		return s.cfg.Trading.MaxOpenPositions
	}

	if regime.Regime == "TRENDING_UP" && regime.Confidence > 0.7 {
		return 15
	}

	if regime.Regime == "VOLATILE" {
		if regime.ATR != nil && regime.Volatility != nil && *regime.Volatility > 3.0 {
			return 5
		}
	}

	if regime.Regime == "RANGING" {
		return 8
	}

	return s.cfg.Trading.MaxOpenPositions
}

// ============================================================================
// INDIVIDUAL FILTERS
// ============================================================================

// 1. Strategy Performance Filter
type StrategyPerformanceFilter struct {
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
}

func (f *StrategyPerformanceFilter) Name() string { return "Strategy Performance" }

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
	outcomes, err := f.repo.GetSignalOutcomes(symbol, "", time.Now().Add(-24*time.Hour), time.Time{}, 0, 0)
	if err != nil {
		return 1.0, false, ""
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
		return 1.0, false, ""
	}

	winRate := float64(wins) / float64(totalSignals) * 100

	if winRate < f.cfg.Trading.LowWinRateThreshold {
		return 0.0, true, fmt.Sprintf("Strategy %s underperforming (WR: %.1f%% < %.0f%%)", strategy, winRate, f.cfg.Trading.LowWinRateThreshold)
	}
	if winRate > f.cfg.Trading.HighWinRateThreshold {
		return 1.2, false, fmt.Sprintf("Strategy %s performing well (WR: %.1f%%)", strategy, winRate)
	}
	if winRate < 45 {
		return 0.9, false, fmt.Sprintf("Strategy %s moderate performance (WR: %.1f%%)", strategy, winRate)
	}

	return 1.0, false, ""
}

// 2. Baseline Quality Filter
type BaselineQualityFilter struct {
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
}

func (f *BaselineQualityFilter) Name() string { return "Baseline Quality" }

func (f *BaselineQualityFilter) Evaluate(ctx context.Context, signal *database.TradingSignalDB) (bool, string, float64) {
	if f.redis != nil {
		cacheKey := fmt.Sprintf("baseline:%s", signal.StockSymbol)
		var baseline database.StatisticalBaseline
		if err := f.redis.Get(ctx, cacheKey, &baseline); err == nil {
			return f.evaluate(&baseline)
		}
	}

	baseline, err := f.repo.GetLatestBaseline(signal.StockSymbol)
	if err != nil || baseline == nil {
		return false, "No statistical baseline available", 0.0
	}

	if f.redis != nil {
		cacheKey := fmt.Sprintf("baseline:%s", signal.StockSymbol)
		_ = f.redis.Set(ctx, cacheKey, baseline, 5*time.Minute)
	}

	return f.evaluate(baseline)
}

func (f *BaselineQualityFilter) evaluate(baseline *database.StatisticalBaseline) (bool, string, float64) {
	if baseline.SampleSize < f.cfg.Trading.MinBaselineSampleSize {
		return false, fmt.Sprintf("Insufficient baseline data (%d < %d trades)", baseline.SampleSize, f.cfg.Trading.MinBaselineSampleSize), 0.0
	}
	if baseline.SampleSize < f.cfg.Trading.MinBaselineSampleSizeStrict {
		return true, fmt.Sprintf("Limited baseline data (%d trades)", baseline.SampleSize), 0.6
	}
	return true, "", 1.0
}

// 3. Dynamic Confidence Filter
type DynamicConfidenceFilter struct {
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
}

func (f *DynamicConfidenceFilter) Name() string { return "Dynamic Confidence" }

func (f *DynamicConfidenceFilter) Evaluate(ctx context.Context, signal *database.TradingSignalDB) (bool, string, float64) {
	// Calculate Volume Z-Score Multiplier (High Volume = Higher Confidence)
	isHighVolume := signal.VolumeZScore > 2.5

	// Trend Alignment Check (Price vs VWAP)
	isTrendAligned := false
	baseline, _ := f.repo.GetLatestBaseline(signal.StockSymbol)
	if baseline != nil && baseline.MeanVolumeLots > 0 {
		vwap := baseline.MeanValue / baseline.MeanVolumeLots
		if signal.TriggerPrice > vwap {
			isTrendAligned = true
		}
	}

	optimalThreshold, thresholdReason := f.getOptimalThreshold(ctx, signal.Strategy)

	// Adaptive: Relax confidence if High Volume or Trend Aligned
	if isHighVolume {
		optimalThreshold *= 0.90 // 10% relaxation (was 5%)
		thresholdReason += " (Relaxed due to High Volume)"
	} else if isTrendAligned {
		optimalThreshold *= 0.95 // 5% relaxation (was 2%)
		thresholdReason += " (Relaxed due to Trend Alignment)"
	}

	if signal.Confidence < optimalThreshold {
		return false, fmt.Sprintf("Below optimal confidence threshold (%.2f < %.2f): %s",
			signal.Confidence, optimalThreshold, thresholdReason), 0.0
	}

	return true, "", 1.0
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

// 4. Regime Effectiveness Filter
type RegimeEffectivenessFilter struct {
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
}

func (f *RegimeEffectivenessFilter) Name() string { return "Regime Effectiveness" }

func (f *RegimeEffectivenessFilter) Evaluate(ctx context.Context, signal *database.TradingSignalDB) (bool, string, float64) {
	regime, err := f.repo.GetLatestRegime(signal.StockSymbol)
	if err != nil || regime == nil {
		return true, "", 1.0
	}

	cacheKey := fmt.Sprintf("opt:regime_eff:%s:%s", signal.Strategy, regime.Regime)
	if f.redis != nil {
		var cachedWinRate float64
		if err := f.redis.Get(ctx, cacheKey, &cachedWinRate); err == nil {
			if cachedWinRate < 40.0 {
				return false, fmt.Sprintf("%s underperforms in %s regime (%.1f%% win rate)", signal.Strategy, regime.Regime, cachedWinRate), 0.0
			}
			return true, "", 1.0
		}
	}

	effectiveness, err := f.repo.GetStrategyEffectivenessByRegime(30)
	if err != nil || len(effectiveness) == 0 {
		return true, "", 1.0
	}

	var winRate float64 = 50.0
	for _, eff := range effectiveness {
		if eff.Strategy == signal.Strategy && eff.MarketRegime == regime.Regime {
			winRate = eff.WinRate
			break
		}
	}

	if f.redis != nil {
		_ = f.redis.Set(ctx, cacheKey, winRate, 10*time.Minute)
	}

	if winRate < 40.0 {
		return false, fmt.Sprintf("%s underperforms in %s regime (%.1f%% win rate)", signal.Strategy, regime.Regime, winRate), 0.0
	}

	return true, "", 1.0
}

// 5. Expected Value Filter
type ExpectedValueFilter struct {
	repo  *database.TradeRepository
	redis *cache.RedisClient
	cfg   *config.Config
}

func (f *ExpectedValueFilter) Name() string { return "Expected Value" }

func (f *ExpectedValueFilter) Evaluate(ctx context.Context, signal *database.TradingSignalDB) (bool, string, float64) {
	cacheKey := fmt.Sprintf("opt:ev:%s", signal.Strategy)
	if f.redis != nil {
		var cachedEV float64
		if err := f.redis.Get(ctx, cacheKey, &cachedEV); err == nil {
			if cachedEV < 0 {
				return false, fmt.Sprintf("Negative EV: %.4f", cachedEV), 0.0
			}
			return true, fmt.Sprintf("EV: %.4f", cachedEV), 1.0
		}
	}

	evData, err := f.repo.GetSignalExpectedValues(30)
	if err != nil || len(evData) == 0 {
		return true, "", 1.0
	}

	var ev float64 = 0.0
	var found bool
	for _, e := range evData {
		if e.Strategy == signal.Strategy {
			ev = e.ExpectedValue
			found = true
			break
		}
	}

	if f.redis != nil && found {
		_ = f.redis.Set(ctx, cacheKey, ev, 10*time.Minute)
	}

	if found && ev < 0 {
		return false, fmt.Sprintf("Negative EV: %.4f", ev), 0.0
	}

	if found && ev > 0 {
		return true, fmt.Sprintf("Positive EV: %.4f", ev), 1.0
	}

	return true, "", 1.0
}

// 6. Order Flow Filter
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
	// Recalculate context flags (could be passed down in context, but recalculating is cheap enough here)
	isHighVolume := signal.VolumeZScore > 2.5

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

	if isHighVolume || isTrendAligned {
		requiredThreshold *= 0.9
	}

	if buyPressure < requiredThreshold {
		if orderFlow.AggressiveBuyPct != nil && *orderFlow.AggressiveBuyPct > f.cfg.Trading.AggressiveBuyThreshold {
			if buyPressure > 0.45 { // Was 0.4 - tightened to avoid falling knives
				return true, fmt.Sprintf("Aggressive Haka detected (%.1f%%)", *orderFlow.AggressiveBuyPct), 1.2
			}
		}
		return false, fmt.Sprintf("Insufficient buy pressure (%.1f%% < %.0f%%)", buyPressure*100, requiredThreshold*100), 0.7
	}

	if buyPressure > 0.6 {
		return true, "Strong buy pressure detected", 1.3
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

	if session == "SESSION_1" && hour < 10 {
		return true, "Morning momentum period", 1.2
	}

	if session == "SESSION_2" && hour >= 14 {
		return true, "Afternoon caution period", 0.8
	}

	return true, "", 1.0
}

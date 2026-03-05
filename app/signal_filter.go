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
			Reason     string
		}
		var cached CachedPerf
		if err := f.redis.Get(ctx, cacheKey, &cached); err == nil {
			return true, cached.Reason, cached.Multiplier
		}
	}

	multiplier, reason := f.calculate(strategy, signal.StockSymbol)

	if f.redis != nil {
		cacheKey := fmt.Sprintf("strategy:perf:%s", strategy)
		cached := struct {
			Multiplier float64
			Reason     string
		}{Multiplier: multiplier, Reason: reason}
		_ = f.redis.Set(ctx, cacheKey, cached, 5*time.Minute)
	}

	return true, reason, multiplier
}

func (f *StrategyPerformanceFilter) calculate(strategy string, symbol string) (float64, string) {
	// Get baseline data first
	baseline, err := f.repo.GetLatestBaseline(symbol)
	baselineMultiplier := 1.0
	var baselineReason string

	if err != nil || baseline == nil {
		return 1.0, "No statistical baseline available"
	}

	// Make sure the required baseline isn't completely zero, but respect config.
	// Hardcap fallback to at least 2 trades if config somehow returns incredibly high by mistake.
	requiredBaseline := f.cfg.Trading.MinBaselineSampleSize
	if requiredBaseline > 50 {
		// Safety net for mock trading: if ENV is heavily cached to old defaults, override it temporarily
		requiredBaseline = 2
	}

	if baseline.SampleSize < requiredBaseline {
		baselineReason = fmt.Sprintf("Insufficient baseline data (%d < %d trades)", baseline.SampleSize, requiredBaseline)
	}

	// Reduce multiplier for limited baseline
	requiredStrict := f.cfg.Trading.MinBaselineSampleSizeStrict
	if requiredStrict > 100 {
		requiredStrict = 10
	}
	if baseline.SampleSize < requiredStrict {
		baselineMultiplier = 0.7 // Reduced from 0.6 for better quality signals
		if baselineReason == "" {
			baselineReason = fmt.Sprintf("Limited baseline data (%d trades)", baseline.SampleSize)
		}
	}

	// Check baseline recency (must be calculated within last 2 hours)
	if time.Since(baseline.CalculatedAt) > 2*time.Hour {
		baselineMultiplier *= 0.9
		if baselineReason != "" {
			baselineReason += "; Stale baseline (>2h old)"
		} else {
			baselineReason = "Stale baseline (>2h old)"
		}
	}

	// Get strategy performance data
	outcomes, err := f.repo.GetSignalOutcomes(symbol, "", time.Now().Add(-24*time.Hour), time.Time{}, 0, 0)
	if err != nil {
		return baselineMultiplier, baselineReason
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
		return baselineMultiplier, baselineReason
	}

	winRate := float64(wins) / float64(totalSignals) * 100
	var strategyReason string
	strategyMultiplier := 1.0

	if winRate < f.cfg.Trading.LowWinRateThreshold {
		strategyReason = fmt.Sprintf("Strategy %s underperforming (WR: %.1f%% < %.0f%%)", strategy, winRate, f.cfg.Trading.LowWinRateThreshold)
	}

	// Check for consecutive losses (circuit breaker logic)
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
		if strategyReason != "" {
			strategyReason += fmt.Sprintf("; Strategy %s hit circuit breaker (%d consecutive losses)", strategy, consecutiveLosses)
		} else {
			strategyReason = fmt.Sprintf("Strategy %s hit circuit breaker (%d consecutive losses)", strategy, consecutiveLosses)
		}
	}

	// Multiplier based on performance
	if winRate > f.cfg.Trading.HighWinRateThreshold {
		strategyMultiplier = 1.25
		if strategyReason != "" {
			strategyReason += fmt.Sprintf("; Strategy %s excellent (WR: %.1f%%)", strategy, winRate)
		} else {
			strategyReason = fmt.Sprintf("Strategy %s excellent (WR: %.1f%%)", strategy, winRate)
		}
	} else if winRate > 55 {
		strategyMultiplier = 1.1
		if strategyReason != "" {
			strategyReason += fmt.Sprintf("; Strategy %s good (WR: %.1f%%)", strategy, winRate)
		} else {
			strategyReason = fmt.Sprintf("Strategy %s good (WR: %.1f%%)", strategy, winRate)
		}
	} else if winRate >= f.cfg.Trading.LowWinRateThreshold {
		strategyMultiplier = 1.0
		if strategyReason != "" {
			strategyReason += fmt.Sprintf("; Strategy %s acceptable (WR: %.1f%%)", strategy, winRate)
		} else {
			strategyReason = fmt.Sprintf("Strategy %s acceptable (WR: %.1f%%)", strategy, winRate)
		}
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

	return finalMultiplier, finalReason
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

	if signal.Confidence < optimalThreshold {
		thresholdReason = fmt.Sprintf("Below optimal confidence threshold (%.2f < %.2f): %s", signal.Confidence, optimalThreshold, thresholdReason)
	}

	return true, thresholdReason, confidenceMultiplier
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

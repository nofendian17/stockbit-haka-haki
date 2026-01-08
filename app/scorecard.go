package app

import (
	"fmt"
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// Scorecard thresholds
const (
	MinScoreForSignal = 30 // Minimum score out of 100 to generate signal (optimized for momentum capture)
)

// SignalScorecard represents a weighted scoring system for signal quality evaluation.
// Maximum score is 100 points across 4 categories (25 points each).
type SignalScorecard struct {
	// Volume Analysis (max 25 pts)
	VolumeZScore       int // 0-10: Z >= 5 = 10, Z >= 4 = 8, Z >= 3 = 6, Z < 3 = 0
	OrderFlowImbalance int // 0-10: > 60% = 10, > 55% = 7, > 50% = 5, < 50% = 0
	VolumeVsAvg        int // 0-5:  > 300% = 5, > 200% = 3, > 100% = 1, < 100% = 0

	// Trend Analysis (max 25 pts)
	VWAPPosition    int // 0-10: > 1% above = 10, above = 7, at = 3, below = 0
	MTFConfluence   int // 0-10: 3 TF agree = 10, 2 TF = 6, 1 TF = 3, conflict = 0
	RegimeAlignment int // 0-5:  TRENDING_UP = 5, NEUTRAL = 3, RANGING = 2, DOWN/VOLATILE = 0

	// Quality Factors (max 25 pts)
	BaselineSampleSize int // 0-10: > 100 = 10, > 80 = 7, > 50 = 5, < 50 = 0
	StrategyWinRate    int // 0-10: > 65% = 10, > 55% = 7, > 45% = 5, < 30% = -5
	TimeOfDay          int // 0-5:  Morning (9-10) = 5, Midday = 3, Late (14+) = 1

	// Confirmation Signals (max 25 pts)
	PatternDetected    int // 0-10: Confirmed breakout = 10, Pattern = 5, No = 0
	WhaleImpactHistory int // 0-10: Positive history = 10, Mixed = 5, Negative = 0
	SectorStrength     int // 0-5:  Sector strong = 5, neutral = 2, weak = 0

	// Breakdown for logging
	Breakdown map[string]int
}

// NewScorecard creates a new empty scorecard
func NewScorecard() *SignalScorecard {
	return &SignalScorecard{
		Breakdown: make(map[string]int),
	}
}

// Total calculates the total score (max 100)
func (sc *SignalScorecard) Total() int {
	total := sc.VolumeZScore + sc.OrderFlowImbalance + sc.VolumeVsAvg +
		sc.VWAPPosition + sc.MTFConfluence + sc.RegimeAlignment +
		sc.BaselineSampleSize + sc.StrategyWinRate + sc.TimeOfDay +
		sc.PatternDetected + sc.WhaleImpactHistory + sc.SectorStrength

	// Cap at 100 and floor at 0
	if total > 100 {
		return 100
	}
	if total < 0 {
		return 0
	}
	return total
}

// VolumeAnalysisScore returns subtotal for volume analysis (max 25)
func (sc *SignalScorecard) VolumeAnalysisScore() int {
	return sc.VolumeZScore + sc.OrderFlowImbalance + sc.VolumeVsAvg
}

// TrendAnalysisScore returns subtotal for trend analysis (max 25)
func (sc *SignalScorecard) TrendAnalysisScore() int {
	return sc.VWAPPosition + sc.MTFConfluence + sc.RegimeAlignment
}

// QualityFactorsScore returns subtotal for quality factors (max 25)
func (sc *SignalScorecard) QualityFactorsScore() int {
	return sc.BaselineSampleSize + sc.StrategyWinRate + sc.TimeOfDay
}

// ConfirmationScore returns subtotal for confirmation signals (max 25)
func (sc *SignalScorecard) ConfirmationScore() int {
	return sc.PatternDetected + sc.WhaleImpactHistory + sc.SectorStrength
}

// IsPassing returns true if score meets minimum threshold
func (sc *SignalScorecard) IsPassing() bool {
	return sc.Total() >= MinScoreForSignal
}

// String returns a formatted breakdown of the scorecard
func (sc *SignalScorecard) String() string {
	return fmt.Sprintf(
		"Score: %d/100 [Volume:%d/25, Trend:%d/25, Quality:%d/25, Confirm:%d/25]",
		sc.Total(),
		sc.VolumeAnalysisScore(),
		sc.TrendAnalysisScore(),
		sc.QualityFactorsScore(),
		sc.ConfirmationScore(),
	)
}

// ScorecardEvaluator evaluates signals and produces scorecards
type ScorecardEvaluator struct {
	repo        *database.TradeRepository
	mtfAnalyzer *MTFAnalyzer
}

// NewScorecardEvaluator creates a new evaluator
func NewScorecardEvaluator(repo *database.TradeRepository, mtf *MTFAnalyzer) *ScorecardEvaluator {
	return &ScorecardEvaluator{
		repo:        repo,
		mtfAnalyzer: mtf,
	}
}

// EvaluateSignal evaluates a signal and returns a scorecard
func (se *ScorecardEvaluator) EvaluateSignal(signal *database.TradingSignalDB) *SignalScorecard {
	sc := NewScorecard()

	// 1. Volume Analysis
	sc.VolumeZScore = se.scoreVolumeZScore(signal.VolumeZScore)
	sc.Breakdown["VolumeZScore"] = sc.VolumeZScore

	orderFlow, _ := se.repo.GetLatestOrderFlow(signal.StockSymbol)
	sc.OrderFlowImbalance = se.scoreOrderFlowImbalance(orderFlow)
	sc.Breakdown["OrderFlowImbalance"] = sc.OrderFlowImbalance

	baseline, _ := se.repo.GetLatestBaseline(signal.StockSymbol)
	if baseline != nil && baseline.MeanVolumeLots > 0 {
		volumeVsAvg := (signal.TriggerVolumeLots / baseline.MeanVolumeLots) * 100
		sc.VolumeVsAvg = se.scoreVolumeVsAvg(volumeVsAvg)
	}
	sc.Breakdown["VolumeVsAvg"] = sc.VolumeVsAvg

	// 2. Trend Analysis
	if baseline != nil && baseline.MeanVolumeLots > 0 {
		vwap := baseline.MeanValue / baseline.MeanVolumeLots
		sc.VWAPPosition = se.scoreVWAPPosition(signal.TriggerPrice, vwap)
	}
	sc.Breakdown["VWAPPosition"] = sc.VWAPPosition

	if se.mtfAnalyzer != nil {
		mtfResult := se.mtfAnalyzer.Analyze(signal.StockSymbol)
		sc.MTFConfluence = se.scoreMTFConfluence(mtfResult)
	}
	sc.Breakdown["MTFConfluence"] = sc.MTFConfluence

	regime, _ := se.repo.GetLatestRegime(signal.StockSymbol)
	sc.RegimeAlignment = se.scoreRegimeAlignment(regime, signal.Decision)
	sc.Breakdown["RegimeAlignment"] = sc.RegimeAlignment

	// 3. Quality Factors
	if baseline != nil {
		sc.BaselineSampleSize = se.scoreBaselineSampleSize(baseline.SampleSize)
	}
	sc.Breakdown["BaselineSampleSize"] = sc.BaselineSampleSize

	strategyPerf, _ := se.repo.GetSignalPerformanceStats(signal.Strategy, "")
	if strategyPerf != nil && strategyPerf.TotalSignals > 20 {
		sc.StrategyWinRate = se.scoreStrategyWinRate(strategyPerf.WinRate)
	} else {
		sc.StrategyWinRate = 5 // Neutral if not enough data
	}
	sc.Breakdown["StrategyWinRate"] = sc.StrategyWinRate

	sc.TimeOfDay = se.scoreTimeOfDay(signal.GeneratedAt)
	sc.Breakdown["TimeOfDay"] = sc.TimeOfDay

	// 4. Confirmation Signals
	patterns, _ := se.repo.GetRecentPatterns(signal.StockSymbol, time.Now().Add(-2*time.Hour))
	sc.PatternDetected = se.scorePatternDetected(patterns, signal.Decision)
	sc.Breakdown["PatternDetected"] = sc.PatternDetected

	whaleStats, _ := se.repo.GetWhaleStats(signal.StockSymbol, time.Now().Add(-24*time.Hour), time.Now())
	sc.WhaleImpactHistory = se.scoreWhaleImpactHistory(whaleStats, signal.Decision)
	sc.Breakdown["WhaleImpactHistory"] = sc.WhaleImpactHistory

	// SectorStrength - Placeholder (requires sector mapping)
	sc.SectorStrength = 2 // Neutral default
	sc.Breakdown["SectorStrength"] = sc.SectorStrength

	log.Printf("📊 Scorecard for %s: %s", signal.StockSymbol, sc.String())
	return sc
}

// Scoring helper functions

func (se *ScorecardEvaluator) scoreVolumeZScore(z float64) int {
	switch {
	case z >= 4.0:
		return 10 // Very high
	case z >= 3.0:
		return 9 // High
	case z >= 2.5:
		return 8 // Moderate-high
	case z >= 2.0:
		return 7 // Moderate
	case z >= 1.5:
		return 5 // Slight elevation
	case z >= 1.0:
		return 3 // Early momentum
	case z >= 0.5:
		return 1 // Minimal signal
	default:
		return 0
	}
}

func (se *ScorecardEvaluator) scoreOrderFlowImbalance(flow *database.OrderFlowImbalance) int {
	if flow == nil {
		return 6 // Slightly favorable if no data
	}

	totalVolume := flow.BuyVolumeLots + flow.SellVolumeLots
	if totalVolume == 0 {
		return 6
	}

	buyPct := (flow.BuyVolumeLots / totalVolume) * 100

	switch {
	case buyPct > 65:
		return 10 // Very strong
	case buyPct > 58:
		return 9 // Strong
	case buyPct > 54:
		return 8 // Good
	case buyPct > 52:
		return 7 // Moderate buy bias
	case buyPct > 50:
		return 6 // Slight buy bias
	case buyPct > 48:
		return 5 // Balanced
	case buyPct > 45:
		return 3 // Slight sell bias
	default:
		return 1 // Sell pressure
	}
}

func (se *ScorecardEvaluator) scoreVolumeVsAvg(pct float64) int {
	switch {
	case pct > 250:
		return 5 // Extreme
	case pct > 150:
		return 4 // High
	case pct > 100:
		return 3 // Above average
	case pct > 75:
		return 2 // Moderate
	case pct > 50:
		return 1 // Slight increase
	default:
		return 0
	}
}

func (se *ScorecardEvaluator) scoreVWAPPosition(price, vwap float64) int {
	if vwap == 0 {
		return 6 // Slightly favorable neutral
	}

	deviation := ((price - vwap) / vwap) * 100

	switch {
	case deviation > 1.5:
		return 10 // Well above VWAP
	case deviation > 0.5:
		return 9 // Above VWAP
	case deviation > 0.0:
		return 8 // Slightly above VWAP
	case deviation > -0.3:
		return 6 // At VWAP
	case deviation > -0.7:
		return 4 // Slightly below
	default:
		return 2 // Below VWAP
	}
}

func (se *ScorecardEvaluator) scoreMTFConfluence(result *MTFResult) int {
	if result == nil {
		return 5 // Neutral
	}

	switch {
	case result.ConfluenceScore >= 0.9:
		return 10
	case result.ConfluenceScore >= 0.7:
		return 8
	case result.ConfluenceScore >= 0.5:
		return 6
	case result.ConfluenceScore >= 0.3:
		return 3
	default:
		return 0
	}
}

func (se *ScorecardEvaluator) scoreRegimeAlignment(regime *database.MarketRegime, decision string) int {
	if regime == nil {
		return 4 // Favorable neutral
	}

	// For BUY signals
	if decision == "BUY" {
		switch regime.Regime {
		case "TRENDING_UP":
			return 5
		case "RANGING":
			return 3 // Range breakout opportunity
		case "VOLATILE":
			return 2
		case "TRENDING_DOWN":
			return 0
		}
	}

	return 3 // Default neutral
}

func (se *ScorecardEvaluator) scoreBaselineSampleSize(size int) int {
	switch {
	case size >= 100:
		return 10
	case size >= 70:
		return 9
	case size >= 50:
		return 8
	case size >= 35:
		return 7
	case size >= 25:
		return 6
	case size >= 15:
		return 4
	case size >= 10:
		return 3 // Minimal acceptable
	case size >= 5:
		return 1 // Very limited data
	default:
		return 0
	}
}

func (se *ScorecardEvaluator) scoreStrategyWinRate(winRate float64) int {
	switch {
	case winRate >= 60:
		return 10
	case winRate >= 52:
		return 9
	case winRate >= 45:
		return 8
	case winRate >= 40:
		return 7
	case winRate >= 35:
		return 6
	case winRate >= 30:
		return 4
	case winRate >= 25:
		return 2
	default:
		return -2 // Minimal penalty
	}
}

func (se *ScorecardEvaluator) scoreTimeOfDay(t time.Time) int {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*60*60)
	}

	hour := t.In(loc).Hour()

	switch {
	case hour >= 9 && hour < 10:
		return 5 // Morning momentum
	case hour >= 10 && hour < 12:
		return 5 // Active morning
	case hour >= 13 && hour < 14:
		return 4 // Early afternoon
	case hour >= 14 && hour < 15:
		return 3 // Late afternoon
	case hour >= 15:
		return 2 // Near close
	default:
		return 0 // Outside trading hours
	}
}

func (se *ScorecardEvaluator) scorePatternDetected(patterns []database.DetectedPattern, decision string) int {
	if len(patterns) == 0 {
		return 5 // Neutral - no pattern required
	}

	for _, p := range patterns {
		if p.PatternType == "RANGE_BREAKOUT" && p.PatternDirection != nil {
			if *p.PatternDirection == decision {
				return 10 // Confirmed breakout in same direction
			}
			return 4 // Pattern exists but different direction
		}
	}

	return 6 // Pattern exists
}

func (se *ScorecardEvaluator) scoreWhaleImpactHistory(stats *database.WhaleStats, decision string) int {
	if stats == nil || stats.TotalWhaleTrades == 0 {
		return 6 // Favorable neutral - no whale interference
	}

	// Calculate buy/sell ratio for historical whale activity
	if decision == "BUY" {
		if stats.BuyVolumeLots > stats.SellVolumeLots*1.5 {
			return 10 // Strong buy bias in whale activity
		} else if stats.BuyVolumeLots > stats.SellVolumeLots*1.2 {
			return 9 // Good buy bias
		} else if stats.BuyVolumeLots > stats.SellVolumeLots {
			return 8 // Moderate buy bias
		} else if stats.BuyVolumeLots > stats.SellVolumeLots*0.7 {
			return 5 // Balanced
		} else if stats.BuyVolumeLots > stats.SellVolumeLots*0.5 {
			return 3 // Mixed
		}
		return 1 // Strong sell bias
	}

	return 6 // Default
}

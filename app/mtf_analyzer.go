package app

import (
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// MTF Analysis constants
const (
	MTFTrendBullish = "BULLISH"
	MTFTrendBearish = "BEARISH"
	MTFTrendNeutral = "NEUTRAL"

	// Lookback periods for trend calculation (in candles)
	TrendLookbackCandles = 5
)

// TimeframeAnalysis represents analysis for a single timeframe
type TimeframeAnalysis struct {
	Timeframe   string  // "1m", "5m", "15m", "1h"
	Trend       string  // BULLISH, BEARISH, NEUTRAL
	VWAPDelta   float64 // Percentage from VWAP
	VolumeRank  string  // HIGH, NORMAL, LOW
	ClosePrice  float64
	OpenPrice   float64
	AvgVolume   float64
	LastUpdated time.Time
}

// MTFResult contains the aggregated multi-timeframe analysis
type MTFResult struct {
	Symbol          string
	Analyses        []TimeframeAnalysis
	ConfluenceScore float64 // 0.0 - 1.0
	DominantTrend   string  // Most agreed trend
	AgreementCount  int     // Number of timeframes agreeing on dominant trend
	CalculatedAt    time.Time
}

// MTFAnalyzer performs multi-timeframe analysis
type MTFAnalyzer struct {
	repo *database.TradeRepository
}

// NewMTFAnalyzer creates a new multi-timeframe analyzer
func NewMTFAnalyzer(repo *database.TradeRepository) *MTFAnalyzer {
	return &MTFAnalyzer{
		repo: repo,
	}
}

// Analyze performs multi-timeframe analysis for a symbol
func (ma *MTFAnalyzer) Analyze(symbol string) *MTFResult {
	result := &MTFResult{
		Symbol:       symbol,
		Analyses:     make([]TimeframeAnalysis, 0, 4),
		CalculatedAt: time.Now(),
	}

	// Analyze each timeframe
	timeframes := []string{"1m", "5m", "15m", "1h"}
	for _, tf := range timeframes {
		analysis := ma.analyzeTimeframe(symbol, tf)
		if analysis != nil {
			result.Analyses = append(result.Analyses, *analysis)
		}
	}

	// Calculate confluence
	result.ConfluenceScore, result.DominantTrend, result.AgreementCount = ma.calculateConfluence(result.Analyses)

	log.Printf("📊 MTF Analysis %s: Confluence=%.0f%%, Trend=%s (%d/%d agree)",
		symbol, result.ConfluenceScore*100, result.DominantTrend, result.AgreementCount, len(result.Analyses))

	return result
}

// analyzeTimeframe analyzes a single timeframe
func (ma *MTFAnalyzer) analyzeTimeframe(symbol, timeframe string) *TimeframeAnalysis {
	// Get recent candles for this timeframe
	candles, err := ma.repo.GetCandlesByTimeframe(timeframe, symbol, TrendLookbackCandles+1)
	if err != nil || len(candles) < 3 {
		// Not enough data
		return nil
	}

	analysis := &TimeframeAnalysis{
		Timeframe:   timeframe,
		LastUpdated: time.Now(),
	}

	// Calculate trend based on recent price action
	firstCandle := candles[len(candles)-1] // Oldest
	lastCandle := candles[0]               // Most recent

	// Extract prices safely
	openPrice := getFloat(firstCandle, "open")
	closePrice := getFloat(lastCandle, "close")

	analysis.OpenPrice = openPrice
	analysis.ClosePrice = closePrice

	// Trend calculation: Compare first open to last close
	if openPrice > 0 {
		priceChange := ((closePrice - openPrice) / openPrice) * 100
		if priceChange > 0.3 {
			analysis.Trend = MTFTrendBullish
		} else if priceChange < -0.3 {
			analysis.Trend = MTFTrendBearish
		} else {
			analysis.Trend = MTFTrendNeutral
		}
	} else {
		analysis.Trend = MTFTrendNeutral
	}

	// Calculate volume rank
	var totalVolume float64
	for _, c := range candles {
		totalVolume += getFloat(c, "volume_lots")
	}
	avgVolume := totalVolume / float64(len(candles))
	lastVolume := getFloat(lastCandle, "volume_lots")

	analysis.AvgVolume = avgVolume
	if avgVolume > 0 {
		volRatio := lastVolume / avgVolume
		if volRatio > 1.5 {
			analysis.VolumeRank = "HIGH"
		} else if volRatio < 0.5 {
			analysis.VolumeRank = "LOW"
		} else {
			analysis.VolumeRank = "NORMAL"
		}
	} else {
		analysis.VolumeRank = "NORMAL"
	}

	return analysis
}

// calculateConfluence determines how many timeframes agree on trend
func (ma *MTFAnalyzer) calculateConfluence(analyses []TimeframeAnalysis) (float64, string, int) {
	if len(analyses) == 0 {
		return 0.5, MTFTrendNeutral, 0 // Neutral if no data
	}

	// Count trends
	trendCount := map[string]int{
		MTFTrendBullish: 0,
		MTFTrendBearish: 0,
		MTFTrendNeutral: 0,
	}

	// Weight by timeframe importance (higher TF = more weight)
	weights := map[string]float64{
		"1m":  1.0,
		"5m":  1.5,
		"15m": 2.0,
		"1h":  2.5,
	}

	for _, a := range analyses {
		trendCount[a.Trend]++
	}

	// Determine dominant trend
	var dominantTrend string
	maxCount := 0
	for trend, count := range trendCount {
		if count > maxCount && trend != MTFTrendNeutral {
			maxCount = count
			dominantTrend = trend
		}
	}

	if dominantTrend == "" {
		dominantTrend = MTFTrendNeutral
		maxCount = trendCount[MTFTrendNeutral]
	}

	// Calculate weighted confluence score
	totalWeight := 0.0
	agreeWeight := 0.0

	for _, a := range analyses {
		weight := weights[a.Timeframe]
		totalWeight += weight

		switch a.Trend {
		case dominantTrend:
			agreeWeight += weight
		case MTFTrendNeutral:
			agreeWeight += weight * 0.5 // Neutral counts as half
		}
	}

	confluence := 0.5
	if totalWeight > 0 {
		confluence = agreeWeight / totalWeight
	}

	return confluence, dominantTrend, maxCount
}

// IsBullishConfluence returns true if majority of timeframes are bullish
func (mr *MTFResult) IsBullishConfluence() bool {
	return mr.DominantTrend == MTFTrendBullish && mr.ConfluenceScore >= 0.6
}

// IsBearishConfluence returns true if majority of timeframes are bearish
func (mr *MTFResult) IsBearishConfluence() bool {
	return mr.DominantTrend == MTFTrendBearish && mr.ConfluenceScore >= 0.6
}

// HasStrongConfluence returns true if confluence is very high
func (mr *MTFResult) HasStrongConfluence() bool {
	return mr.ConfluenceScore >= 0.8
}

// Helper function to safely extract float from map
func getFloat(m map[string]interface{}, key string) float64 {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case float32:
			return float64(val)
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0
}

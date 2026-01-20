package app

import (
	"log"
	"math"
	"time"

	"stockbit-haka-haki/database"
)

// RegimeDetector classifies market conditions for stocks
type RegimeDetector struct {
	repo *database.TradeRepository
	done chan bool
}

// NewRegimeDetector creates a new regime detector
func NewRegimeDetector(repo *database.TradeRepository) *RegimeDetector {
	return &RegimeDetector{
		repo: repo,
		done: make(chan bool),
	}
}

// Start begins the detection loop
func (rd *RegimeDetector) Start() {
	log.Println("📈 Market Regime Detector started")

	// Run every 15 minutes
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	// Initial run
	rd.detectRegimes()

	for {
		select {
		case <-ticker.C:
			rd.detectRegimes()
		case <-rd.done:
			log.Println("📈 Market Regime Detector stopped")
			return
		}
	}
}

// Stop stops the detection loop
func (rd *RegimeDetector) Stop() {
	rd.done <- true
}

// detectRegimes classifies regimes for active stocks
func (rd *RegimeDetector) detectRegimes() {
	log.Println("📈 Detecting market regimes...")

	// 1. Get active symbols from last 1 hour
	since := time.Now().Add(-1 * time.Hour)
	symbols, err := rd.repo.GetActiveSymbols(since)
	if err != nil {
		log.Printf("⚠️  Failed to get active symbols for regime detection: %v", err)
		return
	}

	detected := 0
	for _, symbol := range symbols {
		// 2. Fetch last 50 candles (5min timeframe for consistency)
		candles, err := rd.repo.GetCandlesByTimeframe("5min", symbol, 50)
		if err != nil {
			log.Printf("⚠️  Failed to get candles for %s: %v", symbol, err)
			continue
		}

		if len(candles) < 30 {
			continue
		}

		// 3. Calculate indicators and regime
		regime := rd.classifyRegime(symbol, candles)

		// 4. Save to database
		if err := rd.repo.SaveMarketRegime(regime); err != nil {
			log.Printf("⚠️  Failed to save regime for %s: %v", symbol, err)
		} else {
			detected++
		}
	}

	log.Printf("✅ Regime detection complete: %d symbols updated", detected)
}

// classifyRegime determines the market condition based on technical indicators
// FIXED: Separated trend and volatility classification, adjusted thresholds for Indonesian market
func (rd *RegimeDetector) classifyRegime(symbol string, candles []map[string]interface{}) *database.MarketRegime {
	n := len(candles)
	prices := make([]float64, n)
	for i, c := range candles {
		prices[i] = getFloat(c, "close")
	}

	// 1. Calculate all indicators
	sma20 := calculateSMA(prices, 20)
	ema20 := calculateEMA(prices, 20)
	prevEma20 := calculateEMA(prices[:n-1], 20)
	stdDev20 := calculateStdDev(prices, 20)
	atr := calculateATR(candles, 14)

	// 2. Normalize metrics
	emaSlope := (ema20 - prevEma20) / prevEma20
	bollingerWidth := (stdDev20 * 4) / sma20
	atrPercent := 0.0
	if prices[n-1] > 0 {
		atrPercent = (atr / prices[n-1]) * 100
	}

	// 3. Classify TREND (primary classification)
	var trendRegime string
	var trendConfidence float64

	// Adjusted thresholds for 5-minute candles on Indonesian market
	// 0.005 = 0.5% per 5-min candle (realistic for trending stocks)
	if emaSlope > 0.005 {
		trendRegime = "TRENDING_UP"
		// Normalize confidence: 0.6 base + slope strength (capped at 1.0)
		trendConfidence = math.Min(0.6+(emaSlope*50), 1.0)
	} else if emaSlope < -0.005 {
		trendRegime = "TRENDING_DOWN"
		trendConfidence = math.Min(0.6+(math.Abs(emaSlope)*50), 1.0)
	} else {
		trendRegime = "RANGING"
		trendConfidence = 0.5
	}

	// 4. Classify VOLATILITY (secondary - affects confidence)
	var volatilityLevel string

	// Adjusted threshold for Indonesian stocks (typical ATR: 0.5-2%)
	if atrPercent > 2.0 {
		volatilityLevel = "HIGH"
		// Reduce confidence for volatile trends (less reliable)
		if trendRegime != "RANGING" {
			trendConfidence *= 0.8
		}
	} else if atrPercent > 1.0 {
		volatilityLevel = "MEDIUM"
	} else {
		volatilityLevel = "LOW"
		// Increase confidence for low volatility trends (more reliable)
		if trendRegime != "RANGING" {
			trendConfidence *= 1.1
			trendConfidence = math.Min(trendConfidence, 1.0)
		}
	}

	// 5. Combine into final regime
	// Only mark as VOLATILE if ranging with high volatility
	finalRegime := trendRegime
	if volatilityLevel == "HIGH" && trendRegime == "RANGING" {
		finalRegime = "VOLATILE"
		trendConfidence = 0.7
	}

	// Log regime detection for debugging
	log.Printf("📊 Regime for %s: %s (conf: %.2f, EMA slope: %.4f, ATR: %.2f%%)",
		symbol, finalRegime, trendConfidence, emaSlope, atrPercent)

	return &database.MarketRegime{
		StockSymbol:     symbol,
		DetectedAt:      time.Now(),
		Regime:          finalRegime,
		Confidence:      trendConfidence,
		LookbackPeriods: 20,
		BollingerWidth:  &bollingerWidth,
		Volatility:      &stdDev20,
		ATR:             &atr,
	}
}

// Simple Moving Average
func calculateSMA(data []float64, period int) float64 {
	if len(data) < period {
		return 0
	}
	sum := 0.0
	for i := len(data) - period; i < len(data); i++ {
		sum += data[i]
	}
	return sum / float64(period)
}

// Exponential Moving Average
func calculateEMA(data []float64, period int) float64 {
	if len(data) < period {
		return calculateSMA(data, len(data))
	}
	k := 2.0 / (float64(period) + 1.0)
	ema := calculateSMA(data[:period], period)
	for i := period; i < len(data); i++ {
		ema = (data[i] * k) + (ema * (1 - k))
	}
	return ema
}

// Standard Deviation
func calculateStdDev(data []float64, period int) float64 {
	if len(data) < period {
		return 0
	}
	mean := calculateSMA(data, period)
	variance := 0.0
	for i := len(data) - period; i < len(data); i++ {
		variance += math.Pow(data[i]-mean, 2)
	}
	return math.Sqrt(variance / float64(period))
}

// calculateATR calculates Average True Range using Wilder's smoothing method
func calculateATR(candles []map[string]interface{}, period int) float64 {
	if len(candles) < period+1 {
		return 0
	}

	// Calculate True Range for each candle
	var trueRanges []float64
	for i := 1; i < len(candles); i++ {
		high := getFloat(candles[i], "high")
		low := getFloat(candles[i], "low")
		prevClose := getFloat(candles[i-1], "close")

		// True Range = max(H-L, |H-PrevC|, |L-PrevC|)
		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		tr := math.Max(tr1, math.Max(tr2, tr3))
		trueRanges = append(trueRanges, tr)
	}

	if len(trueRanges) < period {
		return 0
	}

	// Calculate initial ATR = SMA of first period true ranges
	atr := 0.0
	for i := 0; i < period; i++ {
		atr += trueRanges[i]
	}
	atr /= float64(period)

	// Apply Wilder's smoothing for remaining data points
	// ATR = (PrevATR × (n-1) + CurrentTR) / n
	for i := period; i < len(trueRanges); i++ {
		atr = (atr*float64(period-1) + trueRanges[i]) / float64(period)
	}

	return atr
}

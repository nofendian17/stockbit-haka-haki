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
		// 2. Fetch last 50 candles (1min)
		candles, err := rd.repo.GetCandles(symbol, time.Now().Add(-2*time.Hour), time.Now(), 50)
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
func (rd *RegimeDetector) classifyRegime(symbol string, candles []database.Candle) *database.MarketRegime {
	// For Phase 2, we'll use a simplified version:
	// - Trend: Based on 20-period EMA slope
	// - Volatility: Based on Bollinger Band width
	// - Strength: Based on relative price move

	n := len(candles)
	prices := make([]float64, n)
	for i, c := range candles {
		prices[i] = c.Close
	}

	// Calculate 20-period SMA as a simple baseline
	sma20 := calculateSMA(prices, 20)

	// Calculate Volatility (Std Dev over 20 periods)
	stdDev20 := calculateStdDev(prices, 20)
	bollingerWidth := (stdDev20 * 4) / sma20 // Typical BB width: (Upper - Lower) / Middle

	// Calculate EMA 20 for trend direction
	ema20 := calculateEMA(prices, 20)
	prevEma20 := calculateEMA(prices[:n-1], 20)
	emaSlope := (ema20 - prevEma20) / prevEma20

	regime := "RANGING"
	confidence := 0.5

	// Classification Logic
	if bollingerWidth > 0.05 { // Arbitrary threshold for high volatility
		regime = "VOLATILE"
		confidence = 0.7
	} else if emaSlope > 0.001 { // Upward trend
		regime = "TRENDING_UP"
		confidence = 0.6 + (emaSlope * 100)
	} else if emaSlope < -0.001 { // Downward trend
		regime = "TRENDING_DOWN"
		confidence = 0.6 + (math.Abs(emaSlope) * 100)
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return &database.MarketRegime{
		StockSymbol:     symbol,
		DetectedAt:      time.Now(),
		Regime:          regime,
		Confidence:      confidence,
		LookbackPeriods: 20,
		BollingerWidth:  &bollingerWidth,
		Volatility:      &stdDev20,
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

package app

import (
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// PatternDetector identifies chart patterns in stock price action
type PatternDetector struct {
	repo *database.TradeRepository
	done chan bool
}

// NewPatternDetector creates a new pattern detector
func NewPatternDetector(repo *database.TradeRepository) *PatternDetector {
	return &PatternDetector{
		repo: repo,
		done: make(chan bool),
	}
}

// Start begins the detection loop
func (pd *PatternDetector) Start() {
	log.Println("🎨 Chart Pattern Detector started")

	// Run every 30 minutes
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	// Initial run
	pd.detectPatterns()

	for {
		select {
		case <-ticker.C:
			pd.detectPatterns()
		case <-pd.done:
			log.Println("🎨 Chart Pattern Detector stopped")
			return
		}
	}
}

// Stop stops the detection loop
func (pd *PatternDetector) Stop() {
	pd.done <- true
}

// detectPatterns identifies patterns for active stocks
func (pd *PatternDetector) detectPatterns() {
	log.Println("🎨 Detecting chart patterns...")

	// 1. Get active symbols from last 2 hours
	since := time.Now().Add(-2 * time.Hour)
	symbols, err := pd.repo.GetActiveSymbols(since)
	if err != nil {
		log.Printf("⚠️  Failed to get active symbols for pattern detection: %v", err)
		return
	}

	detected := 0
	for _, symbol := range symbols {
		// 2. Fetch last 100 candles (5min)
		// We'll use the generic GetCandlesByTimeframe method
		candlesData, err := pd.repo.GetCandlesByTimeframe("5min", symbol, 100)
		if err != nil {
			log.Printf("⚠️  Failed to get 5min candles for %s: %v", symbol, err)
			continue
		}

		if len(candlesData) < 40 {
			continue
		}

		// 3. Simple Pattern Recognition (e.g. Breakouts, Mean Reversion)
		// For Phase 2, we implement a framework that identifies basic structures
		patterns := pd.analyzeStructures(symbol, candlesData)

		// 4. Save detected patterns
		for _, p := range patterns {
			if err := pd.repo.SaveDetectedPattern(&p); err != nil {
				log.Printf("⚠️  Failed to save pattern for %s: %v", symbol, err)
			} else {
				detected++
			}
		}
	}

	log.Printf("✅ Pattern detection complete: %d patterns found", detected)
}

// analyzeStructures performs structural analysis on candles to find patterns
func (pd *PatternDetector) analyzeStructures(symbol string, candlesData []map[string]interface{}) []database.DetectedPattern {
	var patterns []database.DetectedPattern

	// Convert map to prices for easier handling
	n := len(candlesData)
	closes := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)

	for i, c := range candlesData {
		// Time in results might be index indexed or bucket indexed
		// Assuming typical map result from repo.GetCandlesByTimeframe
		if v, ok := c["close"].(float64); ok {
			closes[i] = v
		}
		if v, ok := c["high"].(float64); ok {
			highs[i] = v
		}
		if v, ok := c["low"].(float64); ok {
			lows[i] = v
		}
	}

	// 1. Detect Range Breakout
	if breakout := pd.detectRangeBreakout(symbol, closes, highs, lows); breakout != nil {
		patterns = append(patterns, *breakout)
	}

	// 2. Detect Double Top/Bottom (High level approximation)
	if double := pd.detectDoubleExtreme(symbol, highs, lows); double != nil {
		patterns = append(patterns, *double)
	}

	return patterns
}

func (pd *PatternDetector) detectRangeBreakout(symbol string, closes, highs, lows []float64) *database.DetectedPattern {
	n := len(closes)
	if n < 40 {
		return nil
	}

	// Look at last 30 candles for range, and last 5 for breakout
	rangeHigh := 0.0
	rangeLow := 100000000.0

	for i := n - 35; i < n-5; i++ {
		if highs[i] > rangeHigh {
			rangeHigh = highs[i]
		}
		if lows[i] < rangeLow {
			rangeLow = lows[i]
		}
	}

	currentPrice := closes[n-1]

	if currentPrice > rangeHigh*1.01 {
		direction := "BULLISH"
		return &database.DetectedPattern{
			StockSymbol:      symbol,
			DetectedAt:       time.Now(),
			PatternType:      "RANGE_BREAKOUT",
			PatternDirection: &direction,
			Confidence:       0.75,
			BreakoutLevel:    &rangeHigh,
		}
	} else if currentPrice < rangeLow*0.99 {
		direction := "BEARISH"
		return &database.DetectedPattern{
			StockSymbol:      symbol,
			DetectedAt:       time.Now(),
			PatternType:      "RANGE_BREAKOUT",
			PatternDirection: &direction,
			Confidence:       0.75,
			BreakoutLevel:    &rangeLow,
		}
	}

	return nil
}

func (pd *PatternDetector) detectDoubleExtreme(symbol string, highs, lows []float64) *database.DetectedPattern {
	// Simplified approximation for Phase 2
	return nil
}

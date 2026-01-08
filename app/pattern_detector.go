package app

import (
	"fmt"
	"log"
	"math"
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
	signalsGenerated := 0

	for _, symbol := range symbols {
		// 2. Fetch last 100 candles (5min)
		candlesData, err := pd.repo.GetCandlesByTimeframe("5min", symbol, 100)
		if err != nil {
			log.Printf("⚠️  Failed to get 5min candles for %s: %v", symbol, err)
			continue
		}

		if len(candlesData) < 40 {
			continue
		}

		// 3. Get Statistical Baseline for Data-Driven Thresholds
		baseline, err := pd.repo.GetLatestBaseline(symbol)
		if err != nil {
			// Fail silently, just continue without baseline enhancements if not available
			// But for strict data-driven approach, we might want to skip.
			// Let's pass nil and handle inside.
		}

		// 4. Pattern Recognition
		patterns := pd.analyzeStructures(symbol, candlesData, baseline)

		// 5. Save patterns and GENERATE SIGNALS
		for _, p := range patterns {
			// Save Pattern
			if err := pd.repo.SaveDetectedPattern(&p); err != nil {
				log.Printf("⚠️  Failed to save pattern for %s: %v", symbol, err)
				continue
			}
			detected++

			// Generate Trading Signal if confidence is high
			if p.Confidence > 0.7 && p.PatternDirection != nil {
				// Create Signal
				signal := &database.TradingSignalDB{
					GeneratedAt:  time.Now(),
					StockSymbol:  symbol,
					Strategy:     fmt.Sprintf("PATTERN_%s", p.PatternType),
					Decision:     *p.PatternDirection, // BUY or SELL (derived from BULLISH/BEARISH)
					Confidence:   p.Confidence,
					TriggerPrice: *p.BreakoutLevel,
					Reason:       fmt.Sprintf("Detected %s pattern with %.0f%% confidence", p.PatternType, p.Confidence*100),
					AnalysisData: "{}", // Initialize with empty JSON object to prevent DB error
				}

				if p.PatternDirection != nil {
					switch *p.PatternDirection {
					case "BULLISH":
						signal.Decision = "BUY"
					case "BEARISH":
						signal.Decision = "SELL"
						// Skip SELL generation for Indonesia market if strictly following rules,
						// but PatternDetector can detect them. SignalTracker will filter if needed.
					}
				}

				// Only save BUY signals for now to be safe
				if signal.Decision == "BUY" {
					if err := pd.repo.SaveTradingSignal(signal); err != nil {
						log.Printf("⚠️  Failed to save pattern signal for %s: %v", symbol, err)
					} else {
						signalsGenerated++
						log.Printf("🚀 Generated signal from pattern: %s %s (%s)", symbol, signal.Decision, signal.Strategy)
					}
				}
			}
		}
	}

	log.Printf("✅ Pattern detection complete: %d patterns found, %d signals generated", detected, signalsGenerated)
}

// analyzeStructures performs structural analysis on candles to find patterns
func (pd *PatternDetector) analyzeStructures(symbol string, candlesData []map[string]interface{}, baseline *database.StatisticalBaseline) []database.DetectedPattern {
	var patterns []database.DetectedPattern

	n := len(candlesData)
	closes := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	volumes := make([]float64, n)

	for i, c := range candlesData {
		if v, ok := c["close"].(float64); ok {
			closes[i] = v
		}
		if v, ok := c["high"].(float64); ok {
			highs[i] = v
		}
		if v, ok := c["low"].(float64); ok {
			lows[i] = v
		}
		// Try to get volume_lots or volume
		if v, ok := c["volume_lots"].(float64); ok {
			volumes[i] = v
		} else if v, ok := c["volume"].(float64); ok {
			volumes[i] = v
		}
	}

	// 1. Detect Range Breakout (Enhanced)
	if breakout := pd.detectRangeBreakout(symbol, closes, highs, lows, volumes, baseline); breakout != nil {
		patterns = append(patterns, *breakout)
	}

	// 2. Detect Double Top/Bottom
	if double := pd.detectDoubleExtreme(symbol, highs, lows, baseline); double != nil {
		patterns = append(patterns, *double)
	}

	return patterns
}

func (pd *PatternDetector) detectRangeBreakout(symbol string, closes, highs, lows, volumes []float64, baseline *database.StatisticalBaseline) *database.DetectedPattern {
	n := len(closes)
	if n < 40 {
		return nil
	}

	// Dynamic Thresholds
	thresholdMult := 0.02 // Default 2%
	if baseline != nil && baseline.MeanPrice > 0 {
		// Use partial StdDev as threshold (e.g., 1.5 sigma)
		thresholdMult = (baseline.StdDevPrice * 2) / baseline.MeanPrice
		if thresholdMult < 0.01 {
			thresholdMult = 0.01 // Min 1%
		}
	}

	// Period for range: last 30 candles, excluding last 3 (breakout confirmation)
	rangeHigh := 0.0
	rangeLow := 100000000.0
	rangeVol := 0.0

	pendingOutcome := "PENDING"

	for i := n - 35; i < n-3; i++ {
		if highs[i] > rangeHigh {
			rangeHigh = highs[i]
		}
		if lows[i] < rangeLow {
			rangeLow = lows[i]
		}
		rangeVol += volumes[i]
	}
	avgRangeVol := rangeVol / 32.0

	currentPrice := closes[n-1]
	currentVol := volumes[n-1]

	// Breakout check
	breakoutUp := currentPrice > rangeHigh*(1+thresholdMult/2) // Relaxed threshold for detection
	breakoutDown := currentPrice < rangeLow*(1-thresholdMult/2)

	// Volume confirmation (if available)
	volConfirmed := true
	if avgRangeVol > 0 {
		volConfirmed = currentVol > avgRangeVol*1.5
	}

	if breakoutUp && volConfirmed {
		direction := "BULLISH"
		conf := 0.7
		if baseline != nil {
			// Boost confidence if volume > baseline mean + stddev
			if currentVol > (baseline.MeanVolumeLots + baseline.StdDevVolume) {
				conf = 0.85
			}
		}

		return &database.DetectedPattern{
			StockSymbol:      symbol,
			DetectedAt:       time.Now(),
			PatternType:      "RANGE_BREAKOUT",
			PatternDirection: &direction,
			Confidence:       conf,
			BreakoutLevel:    &rangeHigh,
			Outcome:          &pendingOutcome,
		}
	} else if breakoutDown && volConfirmed {
		direction := "BEARISH"
		return &database.DetectedPattern{
			StockSymbol:      symbol,
			DetectedAt:       time.Now(),
			PatternType:      "RANGE_BREAKOUT",
			PatternDirection: &direction,
			Confidence:       0.7,
			BreakoutLevel:    &rangeLow,
			Outcome:          &pendingOutcome,
		}
	}

	return nil
}

func (pd *PatternDetector) detectDoubleExtreme(symbol string, highs, lows []float64, baseline *database.StatisticalBaseline) *database.DetectedPattern {
	n := len(highs)
	if n < 50 {
		return nil
	}

	// Look for Double Bottom (W shape) - Bullish Reversal
	// Logic: Low1 (at i), High (at j), Low2 (at k)
	// Low1 approx equal Low2
	// Current price breaking above High (Neckline)

	// Simplified scanner: Check last 50 candles for 2 distinct lows
	// Use 5% tolerance
	tolerance := 0.05
	pendingOutcome := "PENDING"

	// Find global low in last 50
	minIdx := -1
	minVal := 100000000.0
	for i := n - 50; i < n; i++ {
		if lows[i] < minVal {
			minVal = lows[i]
			minIdx = i
		}
	}

	if minIdx == -1 {
		return nil
	}

	// Find second low (at least 10 bars apart)
	minIdx2 := -1
	minVal2 := 100000000.0

	startSearch := minIdx + 10
	if startSearch >= n {
		startSearch = n - 5 // Fallback? No, just can't find second low to right.
		// Try search to left
		endSearch := minIdx - 10
		if endSearch > n-50 {
			for i := n - 50; i < endSearch; i++ {
				if lows[i] < minVal2 { // Searching for local min
					// Check if it's a local trough
					if i > 0 && i < n-1 && lows[i] < lows[i-1] && lows[i] < lows[i+1] {
						// Check proximity to main low
						if math.Abs(lows[i]-minVal)/minVal < tolerance {
							minVal2 = lows[i]
							minIdx2 = i
						}
					}
				}
			}
		}
	} else {
		// Search to right
		for i := startSearch; i < n; i++ {
			if lows[i] < minVal2 { // Searching for local min
				if i < n-1 && lows[i] < lows[i-1] && lows[i] < lows[i+1] {
					if math.Abs(lows[i]-minVal)/minVal < tolerance {
						minVal2 = lows[i]
						minIdx2 = i
					}
				}
			}
		}
	}

	if minIdx2 != -1 {
		// Found two lows. Find the peak in between (Neckline)
		start := minIdx
		end := minIdx2
		if start > end {
			start, end = end, start
		}

		maxNeck := 0.0
		for k := start; k <= end; k++ {
			if highs[k] > maxNeck {
				maxNeck = highs[k]
			}
		}

		// Check if we are breaking the neckline
		currentPrice := highs[n-1] // Use high to check breakout test
		if currentPrice > maxNeck {
			direction := "BULLISH"
			return &database.DetectedPattern{
				StockSymbol:      symbol,
				DetectedAt:       time.Now(),
				PatternType:      "DOUBLE_BOTTOM",
				PatternDirection: &direction,
				Confidence:       0.8, // Strong pattern
				BreakoutLevel:    &maxNeck,
				Outcome:          &pendingOutcome,
			}
		}
	}

	return nil
}

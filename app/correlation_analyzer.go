package app

import (
	"log"
	"math"
	"time"

	"stockbit-haka-haki/database"
)

// CorrelationAnalyzer computes price correlations between different stocks
type CorrelationAnalyzer struct {
	repo *database.TradeRepository
	done chan bool
}

// NewCorrelationAnalyzer creates a new correlation analyzer
func NewCorrelationAnalyzer(repo *database.TradeRepository) *CorrelationAnalyzer {
	return &CorrelationAnalyzer{
		repo: repo,
		done: make(chan bool),
	}
}

// Start begins the analysis loop
func (ca *CorrelationAnalyzer) Start() {
	log.Println("🔗 Correlation Analyzer started")

	// Run every 1 hour (increased from 12h for better real-time correlation)
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Initial run
	ca.runAnalysis()

	for {
		select {
		case <-ticker.C:
			ca.runAnalysis()
		case <-ca.done:
			log.Println("🔗 Correlation Analyzer stopped")
			return
		}
	}
}

// Stop stops the analysis loop
func (ca *CorrelationAnalyzer) Stop() {
	ca.done <- true
}

// runAnalysis computes correlations between active stocks
func (ca *CorrelationAnalyzer) runAnalysis() {
	log.Println("🔗 Running stock correlation analysis...")

	// 1. Get active symbols from last 24 hours
	since := time.Now().Add(-24 * time.Hour)
	symbols, err := ca.repo.GetActiveSymbols(since)
	if err != nil {
		log.Printf("⚠️  Failed to get active symbols for correlation: %v", err)
		return
	}

	if len(symbols) < 2 {
		log.Printf("ℹ️  Not enough symbols for correlation analysis (found %d, need at least 2)", len(symbols))
		return
	}

	log.Printf("📊 Found %d active symbols for correlation analysis", len(symbols))

	// Limit to top 100 symbols to avoid N^2 explosion (increased from 50)
	if len(symbols) > 100 {
		symbols = symbols[:100]
	}

	// 2. Fetch data (1-hour candles for last 30 days)
	stockData := make(map[string][]float64)
	skippedSymbols := 0

	for _, symbol := range symbols {
		candles, err := ca.repo.GetCandlesByTimeframe("1hour", symbol, 100) // approx 30 trading days
		if err != nil {
			log.Printf("⚠️  Failed to get candles for %s: %v", symbol, err)
			skippedSymbols++
			continue
		}

		if len(candles) < 20 {
			skippedSymbols++
			continue
		}

		returns := make([]float64, len(candles)-1)
		for i := 1; i < len(candles); i++ {
			closeVal, ok := candles[i-1]["close"]
			if !ok {
				continue
			}
			prev, ok := closeVal.(float64)
			if !ok {
				continue
			}

			closeVal, ok = candles[i]["close"]
			if !ok {
				continue
			}
			curr, ok := closeVal.(float64)
			if !ok {
				continue
			}

			if prev > 0 {
				returns[i-1] = (curr - prev) / prev
			}
		}

		if len(returns) > 0 {
			stockData[symbol] = returns
		}
	}

	log.Printf("📈 Processed %d symbols with sufficient data (%d skipped)", len(stockData), skippedSymbols)

	if len(stockData) < 2 {
		log.Printf("ℹ️  Not enough symbols with valid data for correlation (found %d)", len(stockData))
		return
	}

	// 3. Compute Pearson correlation for pairs
	processedSymbols := make([]string, 0, len(stockData))
	for s := range stockData {
		processedSymbols = append(processedSymbols, s)
	}

	count := 0
	for i := 0; i < len(processedSymbols); i++ {
		for j := i + 1; j < len(processedSymbols); j++ {
			s1 := processedSymbols[i]
			s2 := processedSymbols[j]

			correlation := ca.computePearsonCorrelation(stockData[s1], stockData[s2])
			if math.IsNaN(correlation) {
				continue
			}

			// 4. Save to database
			entry := &database.StockCorrelation{
				StockA:                 s1,
				StockB:                 s2,
				CalculatedAt:           time.Now(),
				CorrelationCoefficient: correlation,
				LookbackDays:           30,
				Period:                 "1hour",
			}

			if err := ca.repo.SaveStockCorrelation(entry); err != nil {
				log.Printf("⚠️  Failed to save correlation for %s-%s: %v", s1, s2, err)
			} else {
				count++
			}
		}
	}

	if count > 0 {
		log.Printf("✅ Correlation analysis complete: %d pairs processed successfully", count)
	} else {
		log.Println("⚠️  No correlations saved - check if data is sufficient")
	}
}

// computePearsonCorrelation calculates the Pearson correlation coefficient between two datasets
func (ca *CorrelationAnalyzer) computePearsonCorrelation(x, y []float64) float64 {
	n := len(x)
	if len(y) < n {
		n = len(y)
	}
	if n < 10 {
		return math.NaN()
	}

	sumX, sumY, sumXY, sumX2, sumY2 := 0.0, 0.0, 0.0, 0.0, 0.0
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
		sumY2 += y[i] * y[i]
	}

	numerator := float64(n)*sumXY - sumX*sumY
	denominator := math.Sqrt((float64(n)*sumX2 - sumX*sumX) * (float64(n)*sumY2 - sumY*sumY))

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

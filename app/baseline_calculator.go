package app

import (
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// BaselineCalculator periodically calculates statistical baselines for stocks
type BaselineCalculator struct {
	repo *database.TradeRepository
	done chan bool
}

// NewBaselineCalculator creates a new baseline calculator
func NewBaselineCalculator(repo *database.TradeRepository) *BaselineCalculator {
	return &BaselineCalculator{
		repo: repo,
		done: make(chan bool),
	}
}

// Start begins the calculation loop
func (bc *BaselineCalculator) Start() {
	log.Println("📊 Statistical Baseline Calculator started")

	// Run every 1 hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Initial run
	bc.calculateBaselines()

	for {
		select {
		case <-ticker.C:
			bc.calculateBaselines()
		case <-bc.done:
			log.Println("📊 Statistical Baseline Calculator stopped")
			return
		}
	}
}

// Stop stops the calculation loop
func (bc *BaselineCalculator) Stop() {
	bc.done <- true
}

// calculateBaselines computes statistics for all active stocks using database aggregation
func (bc *BaselineCalculator) calculateBaselines() {
	log.Println("📊 Calculating statistical baselines (DB-optimized)...")

	// Try multiple lookback periods to handle fresh deployments
	lookbackPeriods := []struct {
		duration  time.Duration
		hours     int
		minTrades int
	}{
		{24 * time.Hour, 24, 10}, // Primary: 24 hours with 10 trades minimum
		{2 * time.Hour, 2, 5},    // Fallback 1: 2 hours with 5 trades
		{30 * time.Minute, 0, 3}, // Fallback 2: 30 minutes with 3 trades (hours=0 handles as <1h in logic if needed, but DB query uses hours param)
	}

	calculated := 0
	// Track verified symbols to avoid overwriting good data with fallback data
	processedSymbols := make(map[string]bool)

	for _, period := range lookbackPeriods {
		log.Printf("📊 Aggregating baselines for loopback %v...", period.duration)

		// Calculate baselines directly in database
		baselines, err := bc.repo.CalculateBaselinesDB(period.hours, period.minTrades)
		if err != nil {
			log.Printf("⚠️  Failed to calculate baselines for %v lookback: %v", period.duration, err)
			continue
		}

		batchCount := 0
		for _, baseline := range baselines {
			// Skip if better quality data already processed
			if processedSymbols[baseline.StockSymbol] {
				continue
			}

			// Validate result integrity (sanity check)
			if baseline.MeanPrice <= 0 || baseline.SampleSize < period.minTrades {
				continue
			}

			// Save to database (individual saves for now, could be batched further if needed)
			if err := bc.repo.SaveStatisticalBaseline(&baseline); err != nil {
				log.Printf("⚠️  Failed to save baseline for %s: %v", baseline.StockSymbol, err)
			} else {
				calculated++
				batchCount++
				processedSymbols[baseline.StockSymbol] = true
			}
		}

		log.Printf("✅ Saved %d baselines for lookback %v", batchCount, period.duration)
	}

	log.Printf("✅ Baseline calculation complete: %d symbols updated", calculated)
}

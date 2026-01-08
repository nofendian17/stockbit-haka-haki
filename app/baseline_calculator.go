package app

import (
	"log"
	"time"

	"stockbit-haka-haki/database"
	models "stockbit-haka-haki/database/models_pkg"
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
		{24 * time.Hour, 24, 2},  // Primary: 24 hours with 2 trades minimum (lowered from 3)
		{2 * time.Hour, 2, 2},    // Fallback 1: 2 hours with 2 trades
		{30 * time.Minute, 0, 1}, // Fallback 2: 30 minutes with 1 trade minimum
		{15 * time.Minute, 0, 1}, // Fallback 3: 15 minutes with 1 trade minimum (new)
	}

	calculated := 0
	// Track verified symbols to avoid overwriting good data with fallback data
	processedSymbols := make(map[string]bool)

	// OPTIMIZATION: Collect all baselines for batch save
	batchToSave := make([]models.StatisticalBaseline, 0, 100)

	for _, period := range lookbackPeriods {
		log.Printf("📊 Aggregating baselines for lookback %v...", period.duration)

		// Calculate baselines directly in database
		baselines, err := bc.repo.CalculateBaselinesDB(period.hours, period.minTrades)
		if err != nil {
			log.Printf("⚠️  Failed to calculate baselines for %v lookback: %v", period.duration, err)
			continue
		}

		for _, baseline := range baselines {
			// Skip if better quality data already processed
			if processedSymbols[baseline.StockSymbol] {
				continue
			}

			// Validate result integrity (sanity check)
			if baseline.MeanPrice <= 0 || baseline.SampleSize < period.minTrades {
				continue
			}

			// Add to batch for saving
			batchToSave = append(batchToSave, baseline)
			calculated++
			processedSymbols[baseline.StockSymbol] = true
		}

		log.Printf("✅ Collected %d baselines for lookback %v", len(baselines), period.duration)
	}

	// OPTIMIZATION: Single batch save instead of individual saves
	if len(batchToSave) > 0 {
		if err := bc.repo.BatchSaveStatisticalBaselines(batchToSave); err != nil {
			log.Printf("⚠️  Failed to batch save baselines: %v", err)
		} else {
			log.Printf("✅ Batch saved %d baselines", len(batchToSave))
		}
	}

	log.Printf("✅ Baseline calculation complete: %d symbols updated", calculated)
}

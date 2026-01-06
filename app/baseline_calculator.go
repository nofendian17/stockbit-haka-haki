package app

import (
	"log"
	"math"
	"sort"
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

// calculateBaselines computes statistics for all active stocks
func (bc *BaselineCalculator) calculateBaselines() {
	log.Println("📊 Calculating statistical baselines...")

	// Try multiple lookback periods to handle fresh deployments
	lookbackPeriods := []struct {
		duration  time.Duration
		hours     int
		minTrades int
	}{
		{24 * time.Hour, 24, 10}, // Primary: 24 hours with 10 trades minimum
		{2 * time.Hour, 2, 5},    // Fallback 1: 2 hours with 5 trades
		{30 * time.Minute, 0, 3}, // Fallback 2: 30 minutes with 3 trades
	}

	calculated := 0
	skipped := 0
	processedSymbols := make(map[string]bool)

	for _, period := range lookbackPeriods {
		since := time.Now().Add(-period.duration)

		symbols, err := bc.repo.GetActiveSymbols(since)
		if err != nil {
			log.Printf("⚠️  Failed to get active symbols for %v lookback: %v", period.duration, err)
			continue
		}

		log.Printf("📊 Found %d active symbols in the last %v", len(symbols), period.duration)

		for _, symbol := range symbols {
			// Skip if already processed with longer lookback
			if processedSymbols[symbol] {
				continue
			}

			// Fetch trades for this lookback period
			trades, err := bc.repo.GetTradesByTimeRange(symbol, since, time.Now())
			if err != nil {
				log.Printf("⚠️  Failed to get trades for %s: %v", symbol, err)
				continue
			}

			if len(trades) < period.minTrades {
				// Skip symbols with too few trades for reliable statistics
				continue
			}

			// Calculate statistics
			baseline := bc.computeStats(symbol, trades, period.hours)

			// Save to database
			if err := bc.repo.SaveStatisticalBaseline(baseline); err != nil {
				log.Printf("⚠️  Failed to save baseline for %s: %v", symbol, err)
			} else {
				calculated++
				processedSymbols[symbol] = true
				log.Printf("✅ Saved baseline for %s: %d trades (lookback: %v), mean price %.2f, std dev %.2f",
					symbol, baseline.SampleSize, period.duration, baseline.MeanPrice, baseline.StdDevPrice)
			}
		}

		// If we got enough data with this lookback, no need to try shorter periods
		if calculated >= 5 {
			break
		}
	}

	log.Printf("✅ Baseline calculation complete: %d symbols updated, %d checked", calculated, len(processedSymbols)+skipped)
}

// computeStats calculates statistical metrics from a slice of trades
func (bc *BaselineCalculator) computeStats(symbol string, trades []database.Trade, lookbackHours int) *database.StatisticalBaseline {
	var prices []float64
	var volumes []float64
	var values []float64

	var sumPrice, sumVolume, sumValue float64

	for _, t := range trades {
		prices = append(prices, t.Price)
		volumes = append(volumes, t.VolumeLot)
		values = append(values, t.TotalAmount)

		sumPrice += t.Price
		sumVolume += t.VolumeLot
		sumValue += t.TotalAmount
	}

	n := float64(len(trades))
	meanPrice := sumPrice / n
	meanVolume := sumVolume / n
	meanValue := sumValue / n

	// Sort for percentiles and median
	sort.Float64s(prices)
	sort.Float64s(volumes)

	// Calculate standard deviation
	var varPrice, varVolume, varValue float64
	for i := 0; i < len(trades); i++ {
		varPrice += math.Pow(prices[i]-meanPrice, 2)
		varVolume += math.Pow(volumes[i]-meanVolume, 2)
		varValue += math.Pow(values[i]-meanValue, 2)
	}
	stdDevPrice := math.Sqrt(varPrice / n)
	stdDevVolume := math.Sqrt(varVolume / n)
	stdDevValue := math.Sqrt(varValue / n)

	return &database.StatisticalBaseline{
		StockSymbol:      symbol,
		CalculatedAt:     time.Now(),
		LookbackHours:    lookbackHours,
		SampleSize:       len(trades),
		MeanPrice:        meanPrice,
		StdDevPrice:      stdDevPrice,
		MedianPrice:      getMedian(prices),
		PriceP25:         getPercentile(prices, 25),
		PriceP75:         getPercentile(prices, 75),
		MeanVolumeLots:   meanVolume,
		StdDevVolume:     stdDevVolume,
		MedianVolumeLots: getMedian(volumes),
		VolumeP25:        getPercentile(volumes, 25),
		VolumeP75:        getPercentile(volumes, 75),
		MeanValue:        meanValue,
		StdDevValue:      stdDevValue,
	}
}

// getMedian returns the median of a sorted slice
func getMedian(data []float64) float64 {
	length := len(data)
	if length == 0 {
		return 0
	}
	if length%2 == 0 {
		return (data[length/2-1] + data[length/2]) / 2
	}
	return data[length/2]
}

// getPercentile returns the p-th percentile of a sorted slice
func getPercentile(data []float64, p float64) float64 {
	length := len(data)
	if length == 0 {
		return 0
	}
	index := (p / 100) * float64(length-1)
	i := int(math.Floor(index))
	fraction := index - float64(i)
	if i+1 < length {
		return data[i] + fraction*(data[i+1]-data[i])
	}
	return data[i]
}

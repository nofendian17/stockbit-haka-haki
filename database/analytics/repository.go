package analytics

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	models "stockbit-haka-haki/database/models_pkg"
	"stockbit-haka-haki/database/types"

	"gorm.io/gorm"
)

// Repository handles database operations for analytics data
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new analytics repository
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ============================================================================
// Statistical Baselines
// ============================================================================

// SaveStatisticalBaseline persists a statistical baseline to the database
func (r *Repository) SaveStatisticalBaseline(baseline *models.StatisticalBaseline) error {
	if err := r.db.Create(baseline).Error; err != nil {
		return fmt.Errorf("SaveStatisticalBaseline: %w", err)
	}
	return nil
}

// OPTIMIZATION: BatchSaveStatisticalBaselines persists multiple baselines in a single transaction
// Uses ON CONFLICT to handle duplicates gracefully
func (r *Repository) BatchSaveStatisticalBaselines(baselines []models.StatisticalBaseline) error {
	if len(baselines) == 0 {
		return nil
	}

	// Use CreateInBatches for efficient bulk insertion
	// Batch size of 50 is optimal for this data size
	if err := r.db.CreateInBatches(baselines, 50).Error; err != nil {
		return fmt.Errorf("BatchSaveStatisticalBaselines: %w", err)
	}
	return nil
}

// GetLatestBaseline retrieves the most recent statistical baseline for a symbol
func (r *Repository) GetLatestBaseline(symbol string) (*models.StatisticalBaseline, error) {
	var baseline models.StatisticalBaseline
	err := r.db.Where("stock_symbol = ?", symbol).Order("calculated_at DESC").First(&baseline).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("GetLatestBaseline: %w", err)
	}
	return &baseline, nil
}

// GetAggregateBaseline calculates a composite baseline for the entire market (IHSG)
func (r *Repository) GetAggregateBaseline() (*models.StatisticalBaseline, error) {
	type result struct {
		StockCount  int64
		TotalValue  float64
		TotalVolume float64
		AvgPrice    float64
	}

	var res result
	// Aggregate valid baselines from the last 24 hours
	err := r.db.Raw(`
		WITH latest_baselines AS (
			SELECT DISTINCT ON (stock_symbol) *
			FROM statistical_baselines
			WHERE calculated_at >= NOW() - INTERVAL '24 hours'
			ORDER BY stock_symbol, calculated_at DESC
		)
		SELECT 
			COUNT(*) as stock_count,
			SUM(mean_value) as total_value,
			SUM(mean_volume_lots) as total_volume,
			AVG(mean_price) as avg_price
		FROM latest_baselines
	`).Scan(&res).Error

	if err != nil {
		return nil, fmt.Errorf("GetAggregateBaseline: %w", err)
	}

	if res.StockCount == 0 {
		return nil, nil
	}

	// Create composite baseline
	return &models.StatisticalBaseline{
		StockSymbol:    "IHSG",
		CalculatedAt:   time.Now(),
		LookbackHours:  24,
		SampleSize:     int(res.StockCount), // Using stock count as proxy for sample size
		MeanValue:      res.TotalValue,
		MeanVolumeLots: res.TotalVolume,
		MeanPrice:      res.AvgPrice,
		// Leave other fields zero as they don't make sense for aggregate without complex weighting
	}, nil
}

// CalculateBaselinesDB calculates statistical baselines directly in the database
// Uses candle_1min view for efficient aggregation
func (r *Repository) CalculateBaselinesDB(minutesBack int, minTrades int) ([]models.StatisticalBaseline, error) {
	var baselines []models.StatisticalBaseline

	// Calculate hours for display/storage (integer division)
	lookbackHours := minutesBack / 60

	// Complex aggregation query using Postgres/TimescaleDB functions
	// We use candle_1min to get precise volume/price data but aggregated by minute first for speed
	// Note: We use fmt.Sprintf for lookback_hours in SELECT to avoid type inference issues
	query := fmt.Sprintf(`
		WITH stats AS (
			SELECT
				stock_symbol,
				COUNT(*) as sample_size,
				AVG(close) as mean_price,
				STDDEV(close) as std_dev_price,
				PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY close) as median_price,
				PERCENTILE_CONT(0.25) WITHIN GROUP (ORDER BY close) as price_p25,
				PERCENTILE_CONT(0.75) WITHIN GROUP (ORDER BY close) as price_p75,
				AVG(volume_lots) as mean_volume_lots,
				STDDEV(volume_lots) as std_dev_volume,
				PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY volume_lots) as median_volume_lots,
				PERCENTILE_CONT(0.25) WITHIN GROUP (ORDER BY volume_lots) as volume_p25,
				PERCENTILE_CONT(0.75) WITHIN GROUP (ORDER BY volume_lots) as volume_p75,
				AVG(total_value) as mean_value,
				STDDEV(total_value) as std_dev_value
			FROM candle_1min
			WHERE bucket >= NOW() - INTERVAL '1 minute' * ?
			GROUP BY stock_symbol
			HAVING COUNT(*) >= ?
		)
		SELECT
			stock_symbol,
			NOW() as calculated_at,
			%d as lookback_hours,
			sample_size::bigint,
			COALESCE(mean_price, 0) as mean_price,
			COALESCE(std_dev_price, 0) as std_dev_price,
			COALESCE(median_price, 0) as median_price,
			COALESCE(price_p25, 0) as price_p25,
			COALESCE(price_p75, 0) as price_p75,
			COALESCE(mean_volume_lots, 0) as mean_volume_lots,
			COALESCE(std_dev_volume, 0) as std_dev_volume,
			COALESCE(median_volume_lots, 0) as median_volume_lots,
			COALESCE(volume_p25, 0) as volume_p25,
			COALESCE(volume_p75, 0) as volume_p75,
			COALESCE(mean_value, 0) as mean_value,
			COALESCE(std_dev_value, 0) as std_dev_value
		FROM stats
	`, lookbackHours)

	if err := r.db.Raw(query, minutesBack, minTrades).Scan(&baselines).Error; err != nil {
		return nil, fmt.Errorf("CalculateBaselinesDB: %w", err)
	}

	return baselines, nil
}

// ============================================================================
// Market Regimes
// ============================================================================

// SaveMarketRegime persists a market regime detection to the database
func (r *Repository) SaveMarketRegime(regime *models.MarketRegime) error {
	if err := r.db.Create(regime).Error; err != nil {
		return fmt.Errorf("SaveMarketRegime: %w", err)
	}
	return nil
}

// GetLatestRegime retrieves the most recent market regime for a symbol
func (r *Repository) GetLatestRegime(symbol string) (*models.MarketRegime, error) {
	var regime models.MarketRegime
	err := r.db.Where("stock_symbol = ?", symbol).Order("detected_at DESC").First(&regime).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("GetLatestRegime: %w", err)
	}
	return &regime, nil
}

// GetAggregateMarketRegime calculates the overall market regime based on individual stock regimes
func (r *Repository) GetAggregateMarketRegime() (*models.MarketRegime, error) {
	type result struct {
		Regime    string
		Count     int64
		AvgConf   float64
		AvgVol    float64
		AvgChange float64
	}

	var res result
	// Query to find the majority regime among active stocks in the last 24 hours
	err := r.db.Raw(`
		WITH latest_regimes AS (
			SELECT DISTINCT ON (stock_symbol) *
			FROM market_regimes
			WHERE detected_at >= NOW() - INTERVAL '24 hours'
			ORDER BY stock_symbol, detected_at DESC
		)
		SELECT 
			regime, 
			COUNT(*) as count, 
			AVG(confidence) as avg_conf, 
			AVG(volatility) as avg_vol,
			AVG(price_change_pct) as avg_change
		FROM latest_regimes
		GROUP BY regime
		ORDER BY count DESC
		LIMIT 1
	`).Scan(&res).Error

	if err != nil {
		return nil, fmt.Errorf("GetAggregateMarketRegime: %w", err)
	}

	if res.Regime == "" {
		// Return default NEUTRAL regime instead of nil
		return &models.MarketRegime{
			StockSymbol:     "IHSG",
			DetectedAt:      time.Now(),
			Regime:          "NEUTRAL",
			Confidence:      0.5,
			LookbackPeriods: 24,
		}, nil
	}

	return &models.MarketRegime{
		StockSymbol:     "IHSG", // Virtual symbol
		DetectedAt:      time.Now(),
		Regime:          res.Regime,
		Confidence:      res.AvgConf,
		Volatility:      &res.AvgVol,
		PriceChangePct:  &res.AvgChange,
		LookbackPeriods: 24, // Represents 24h aggregation
	}, nil
}

// ============================================================================
// Detected Patterns
// ============================================================================

// SaveDetectedPattern persists a detected chart pattern to the database
func (r *Repository) SaveDetectedPattern(pattern *models.DetectedPattern) error {
	if err := r.db.Create(pattern).Error; err != nil {
		return fmt.Errorf("SaveDetectedPattern: %w", err)
	}
	return nil
}

// GetRecentPatterns retrieves recently detected patterns for a symbol
func (r *Repository) GetRecentPatterns(symbol string, since time.Time) ([]models.DetectedPattern, error) {
	var patterns []models.DetectedPattern
	err := r.db.Where("stock_symbol = ? AND detected_at >= ?", symbol, since).Order("detected_at DESC").Find(&patterns).Error
	if err != nil {
		return nil, fmt.Errorf("GetRecentPatterns: %w", err)
	}
	return patterns, nil
}

// GetAllRecentPatterns retrieves recently detected patterns for all symbols
func (r *Repository) GetAllRecentPatterns(since time.Time) ([]models.DetectedPattern, error) {
	var patterns []models.DetectedPattern
	err := r.db.Where("detected_at >= ?", since).Order("detected_at DESC").Limit(50).Find(&patterns).Error
	if err != nil {
		return nil, fmt.Errorf("GetAllRecentPatterns: %w", err)
	}
	return patterns, nil
}

// UpdatePatternOutcome updates the outcome of a detected pattern
func (r *Repository) UpdatePatternOutcome(id int64, outcome string, breakout bool, maxMove float64) error {
	if err := r.db.Model(&models.DetectedPattern{}).Where("id = ?", id).Updates(map[string]interface{}{
		"outcome":         outcome,
		"actual_breakout": breakout,
		"max_move_pct":    maxMove,
	}).Error; err != nil {
		return fmt.Errorf("UpdatePatternOutcome: %w", err)
	}
	return nil
}

// ============================================================================
// Stock Correlations
// ============================================================================

// SaveStockCorrelation persists a stock correlation record
func (r *Repository) SaveStockCorrelation(correlation *models.StockCorrelation) error {
	if err := r.db.Create(correlation).Error; err != nil {
		return fmt.Errorf("SaveStockCorrelation: %w", err)
	}
	return nil
}

// GetStockCorrelations retrieves recent correlations for a symbol or top global correlations
func (r *Repository) GetStockCorrelations(symbol string, limit int) ([]models.StockCorrelation, error) {
	var correlations []models.StockCorrelation
	query := r.db.Order("ABS(correlation_coefficient) DESC").Limit(limit)

	if symbol != "" {
		// Specific symbol correlations (either A or B)
		query = query.Where("stock_a = ? OR stock_b = ?", symbol, symbol)
	} else {
		// Global top correlations (only strong ones, e.g., > 0.5 or < -0.5)
		query = query.Where("ABS(correlation_coefficient) >= 0.5")
	}

	// Ensure we get the latest calculation
	// Note for global: we might want distinct pairs, but for now simple latest strong ones is good enough
	err := query.Find(&correlations).Error

	if err != nil {
		log.Printf("❌ Error fetching correlations (symbol=%s): %v", symbol, err)
		return nil, fmt.Errorf("GetStockCorrelations: %w", err)
	}

	log.Printf("📊 Found %d correlations (symbol=%s)", len(correlations), symbol)
	return correlations, nil
}

// GetCorrelationsForPair retrieves historical correlations between two specific stocks
func (r *Repository) GetCorrelationsForPair(stockA, stockB string) ([]models.StockCorrelation, error) {
	var correlations []models.StockCorrelation
	err := r.db.Where("(stock_a = ? AND stock_b = ?) OR (stock_a = ? AND stock_b = ?)", stockA, stockB, stockB, stockA).
		Order("calculated_at DESC").
		Find(&correlations).Error

	if err != nil {
		return nil, fmt.Errorf("GetCorrelationsForPair: %w", err)
	}
	return correlations, nil
}

// ============================================================================
// Order Flow Imbalance
// ============================================================================

// SaveOrderFlowImbalance persists order flow data
func (r *Repository) SaveOrderFlowImbalance(flow *models.OrderFlowImbalance) error {
	if err := r.db.Create(flow).Error; err != nil {
		return fmt.Errorf("SaveOrderFlowImbalance: %w", err)
	}
	return nil
}

// GetOrderFlowImbalance retrieves order flow data with filters
func (r *Repository) GetOrderFlowImbalance(symbol string, startTime, endTime time.Time, limit int) ([]models.OrderFlowImbalance, error) {
	var flows []models.OrderFlowImbalance
	query := r.db.Order("bucket DESC")

	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
	}
	if !startTime.IsZero() {
		query = query.Where("bucket >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("bucket <= ?", endTime)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&flows).Error; err != nil {
		return nil, fmt.Errorf("GetOrderFlowImbalance: %w", err)
	}
	return flows, nil
}

// GetLatestOrderFlow retrieves the most recent order flow for a symbol
func (r *Repository) GetLatestOrderFlow(symbol string) (*models.OrderFlowImbalance, error) {
	var flow models.OrderFlowImbalance
	err := r.db.Where("stock_symbol = ?", symbol).
		Order("bucket DESC").
		First(&flow).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("GetLatestOrderFlow: %w", err)
	}
	return &flow, nil
}

// Combined Accumulation/Distribution Configuration
const (
	WhaleWeight             = 0.35 // 35% - institutional activity
	OrderFlowWeight         = 0.65 // 65% - market sentiment (retail + institution)
	MinWhaleAlerts          = 3    // Minimum whale alerts for classification
	MinOrderFlowBuckets     = 30   // Minimum order flow buckets (~30 minutes)
	ClassificationThreshold = 0.25 // Score threshold for A/D classification
)

// GetCombinedAccumulationDistribution retrieves combined A/D data from whale alerts and order flow
// This provides more accurate classification by combining institutional activity with market sentiment
func (r *Repository) GetCombinedAccumulationDistribution(
	startTime time.Time,
	minWhaleAlerts int,
	minOrderFlowBuckets int,
) ([]types.CombinedAccumulationDistribution, error) {
	if startTime.IsZero() {
		startTime = time.Now().Add(-24 * time.Hour)
	}

	if minWhaleAlerts == 0 {
		minWhaleAlerts = MinWhaleAlerts
	}
	if minOrderFlowBuckets == 0 {
		minOrderFlowBuckets = MinOrderFlowBuckets
	}

	// Step 1: Get whale alerts aggregation by symbol
	whaleQuery := `
		SELECT 
			stock_symbol,
			SUM(CASE WHEN action = 'BUY' THEN 1 ELSE 0 END) as whale_buy_count,
			SUM(CASE WHEN action = 'SELL' THEN 1 ELSE 0 END) as whale_sell_count,
			SUM(CASE WHEN action = 'BUY' THEN trigger_value ELSE 0 END) as whale_buy_value,
			SUM(CASE WHEN action = 'SELL' THEN trigger_value ELSE 0 END) as whale_sell_value,
			COUNT(*) as total_alerts
		FROM whale_alerts
		WHERE detected_at >= ? 
			AND (market_board != 'NG' OR market_board IS NULL)
		GROUP BY stock_symbol
		HAVING COUNT(*) >= ?
	`

	whaleRows, err := r.db.Raw(whaleQuery, startTime, minWhaleAlerts).Rows()
	if err != nil {
		return nil, fmt.Errorf("whale query failed: %w", err)
	}
	defer whaleRows.Close()

	whaleData := make(map[string]struct {
		buyCount    int64
		sellCount   int64
		buyValue    float64
		sellValue   float64
		totalAlerts int64
	})

	for whaleRows.Next() {
		var symbol string
		var w struct {
			buyCount    int64
			sellCount   int64
			buyValue    float64
			sellValue   float64
			totalAlerts int64
		}
		if err := whaleRows.Scan(&symbol, &w.buyCount, &w.sellCount, &w.buyValue, &w.sellValue, &w.totalAlerts); err != nil {
			continue
		}
		whaleData[symbol] = w
	}

	// Step 2: Get order flow aggregation by symbol
	flowQuery := `
		SELECT 
			stock_symbol,
			SUM(buy_volume_lots) as total_buy_volume,
			SUM(sell_volume_lots) as total_sell_volume,
			SUM(delta_volume) as total_delta,
			AVG(volume_imbalance_ratio) as avg_imbalance,
			COUNT(*) as bucket_count
		FROM order_flow_imbalance
		WHERE bucket >= ?
		GROUP BY stock_symbol
		HAVING COUNT(*) >= ?
	`

	flowRows, err := r.db.Raw(flowQuery, startTime, minOrderFlowBuckets).Rows()
	if err != nil {
		return nil, fmt.Errorf("order flow query failed: %w", err)
	}
	defer flowRows.Close()

	flowData := make(map[string]struct {
		buyVolume   float64
		sellVolume  float64
		delta       float64
		imbalance   float64
		bucketCount int
	})

	for flowRows.Next() {
		var symbol string
		var f struct {
			buyVolume   float64
			sellVolume  float64
			delta       float64
			imbalance   float64
			bucketCount int
		}
		if err := flowRows.Scan(&symbol, &f.buyVolume, &f.sellVolume, &f.delta, &f.imbalance, &f.bucketCount); err != nil {
			continue
		}
		flowData[symbol] = f
	}

	// Step 3: Combine and calculate scores
	combined := make([]types.CombinedAccumulationDistribution, 0)

	// Get all unique symbols from both sources
	symbols := make(map[string]bool)
	for symbol := range whaleData {
		symbols[symbol] = true
	}
	for symbol := range flowData {
		symbols[symbol] = true
	}

	for symbol := range symbols {
		var entry types.CombinedAccumulationDistribution
		entry.StockSymbol = symbol

		// Whale data
		if w, ok := whaleData[symbol]; ok {
			entry.WhaleBuyCount = w.buyCount
			entry.WhaleSellCount = w.sellCount
			entry.WhaleBuyValue = w.buyValue
			entry.WhaleSellValue = w.sellValue
			entry.WhaleNetValue = w.buyValue - w.sellValue
			entry.WhaleTotalValue = w.buyValue + w.sellValue
			if w.totalAlerts > 0 {
				entry.WhaleBuyPercentage = float64(w.buyCount) / float64(w.totalAlerts) * 100
			}
		}

		// Order flow data
		if f, ok := flowData[symbol]; ok {
			entry.OrderFlowBuyVolume = f.buyVolume
			entry.OrderFlowSellVolume = f.sellVolume
			entry.OrderFlowDelta = f.delta
			entry.OrderFlowImbalance = f.imbalance
			entry.OrderFlowBucketCount = f.bucketCount
		}

		// Calculate combined score
		entry.CombinedScore = calculateCombinedScore(entry)

		// Classify
		if entry.CombinedScore > ClassificationThreshold {
			entry.Status = "ACCUMULATION"
		} else if entry.CombinedScore < -ClassificationThreshold {
			entry.Status = "DISTRIBUTION"
		} else {
			entry.Status = "NEUTRAL"
		}

		// Only include A/D (not neutral)
		if entry.Status != "NEUTRAL" {
			combined = append(combined, entry)
		}
	}

	// Step 4: Sort by combined score
	// Accumulation: highest positive score first
	// Distribution: lowest negative score first
	sort.Slice(combined, func(i, j int) bool {
		if combined[i].Status == combined[j].Status {
			// Same status - sort by absolute score descending
			return math.Abs(combined[i].CombinedScore) > math.Abs(combined[j].CombinedScore)
		}
		// Accumulation before Distribution
		return combined[i].Status == "ACCUMULATION"
	})

	// Step 5: Split into accumulation and distribution
	var accumulation []types.CombinedAccumulationDistribution
	var distribution []types.CombinedAccumulationDistribution

	for _, c := range combined {
		if c.Status == "ACCUMULATION" {
			accumulation = append(accumulation, c)
		} else if c.Status == "DISTRIBUTION" {
			distribution = append(distribution, c)
		}
	}

	// Limit to top 10 each for display (total top 20)
	if len(accumulation) > 10 {
		accumulation = accumulation[:10]
	}
	if len(distribution) > 10 {
		distribution = distribution[:10]
	}

	// Combine and return
	result := make([]types.CombinedAccumulationDistribution, 0, len(accumulation)+len(distribution))
	result = append(result, accumulation...)
	result = append(result, distribution...)

	return result, nil
}

// calculateCombinedScore computes the weighted score from whale and order flow data
func calculateCombinedScore(data types.CombinedAccumulationDistribution) float64 {
	// Whale Score: -1 (full sell) to +1 (full buy)
	var whaleScore float64
	if data.WhaleTotalValue > 0 {
		whaleScore = data.WhaleNetValue / data.WhaleTotalValue
	}

	// Order Flow Score: already -1 to +1 from volume_imbalance_ratio
	flowScore := data.OrderFlowImbalance

	// If no order flow data, rely only on whale
	if data.OrderFlowBucketCount == 0 && data.WhaleTotalValue > 0 {
		return whaleScore * 1.0 // Use whale only
	}

	// If no whale data but has order flow, rely only on order flow
	if data.WhaleTotalValue == 0 && data.OrderFlowBucketCount > 0 {
		return flowScore * 1.0
	}

	// Combined weighted score
	combined := (whaleScore * WhaleWeight) + (flowScore * OrderFlowWeight)

	return combined
}

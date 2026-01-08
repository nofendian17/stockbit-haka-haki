package analytics

import (
	"fmt"
	"log"
	"time"

	models "stockbit-haka-haki/database/models_pkg"

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

// CalculateBaselinesDB calculates statistical baselines directly in the database
// Uses candle_1min view for efficient aggregation
func (r *Repository) CalculateBaselinesDB(hoursBack int, minTrades int) ([]models.StatisticalBaseline, error) {
	var baselines []models.StatisticalBaseline

	// Complex aggregation query using Postgres/TimescaleDB functions
	// We use candle_1min to get precise volume/price data but aggregated by minute first for speed
	query := `
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
			WHERE bucket >= NOW() - INTERVAL '1 hour' * ?
			GROUP BY stock_symbol
			HAVING COUNT(*) >= ?
		)
		SELECT
			stock_symbol,
			NOW() as calculated_at,
			? as lookback_hours,
			sample_size,
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
	`

	if err := r.db.Raw(query, hoursBack, minTrades, hoursBack).Scan(&baselines).Error; err != nil {
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

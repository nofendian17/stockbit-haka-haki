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

// GetStockCorrelations retrieves recent correlations for a symbol
func (r *Repository) GetStockCorrelations(symbol string, limit int) ([]models.StockCorrelation, error) {
	var correlations []models.StockCorrelation
	err := r.db.Where("stock_a = ? OR stock_b = ?", symbol, symbol).
		Order("calculated_at DESC").
		Limit(limit).
		Find(&correlations).Error

	if err != nil {
		log.Printf("❌ Error fetching correlations for %s: %v", symbol, err)
		return nil, fmt.Errorf("GetStockCorrelations: %w", err)
	}

	log.Printf("📊 Found %d correlations for symbol %s", len(correlations), symbol)
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

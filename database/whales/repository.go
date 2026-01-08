package whales

import (
	"fmt"
	"time"

	models "stockbit-haka-haki/database/models_pkg"
	"stockbit-haka-haki/database/types"

	"gorm.io/gorm"
)

// Repository handles database operations for whale alerts
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new whales repository
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// SaveWhaleAlert saves a whale alert
func (r *Repository) SaveWhaleAlert(alert *models.WhaleAlert) error {
	if err := r.db.Create(alert).Error; err != nil {
		return fmt.Errorf("SaveWhaleAlert: %w", err)
	}
	return nil
}

// GetHistoricalWhales retrieves whale alerts with filters
func (r *Repository) GetHistoricalWhales(stockSymbol string, startTime, endTime time.Time, alertType string, action string, board string, minAmount float64, limit, offset int) ([]models.WhaleAlert, error) {
	var whales []models.WhaleAlert
	query := r.db.Order("detected_at DESC")

	if stockSymbol != "" {
		query = query.Where("stock_symbol = ?", stockSymbol)
	}

	if !startTime.IsZero() {
		query = query.Where("detected_at >= ?", startTime)
	}

	if !endTime.IsZero() {
		query = query.Where("detected_at <= ?", endTime)
	}

	if alertType != "" {
		query = query.Where("alert_type = ?", alertType)
	}

	if action != "" {
		query = query.Where("action = ?", action)
	}

	if board != "" {
		query = query.Where("market_board = ?", board)
	}

	if minAmount > 0 {
		query = query.Where("trigger_value >= ?", minAmount)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&whales).Error; err != nil {
		return nil, fmt.Errorf("GetHistoricalWhales: %w", err)
	}
	return whales, nil
}

// GetWhaleAlertByID retrieves a specific whale alert by ID
func (r *Repository) GetWhaleAlertByID(id int64) (*models.WhaleAlert, error) {
	var alert models.WhaleAlert
	err := r.db.First(&alert, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("GetWhaleAlertByID: %w", err)
	}
	return &alert, nil
}

// GetWhaleCount returns total count of whales matching filters
func (r *Repository) GetWhaleCount(stockSymbol string, startTime, endTime time.Time, alertType string, action string, board string, minAmount float64) (int64, error) {
	var count int64
	query := r.db.Model(&models.WhaleAlert{})

	if stockSymbol != "" {
		query = query.Where("stock_symbol = ?", stockSymbol)
	}

	if !startTime.IsZero() {
		query = query.Where("detected_at >= ?", startTime)
	}

	if !endTime.IsZero() {
		query = query.Where("detected_at <= ?", endTime)
	}

	if alertType != "" {
		query = query.Where("alert_type = ?", alertType)
	}

	if action != "" {
		query = query.Where("action = ?", action)
	}

	if board != "" {
		query = query.Where("market_board = ?", board)
	}

	if minAmount > 0 {
		query = query.Where("trigger_value >= ?", minAmount)
	}

	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("GetWhaleCount: %w", err)
	}
	return count, nil
}

// GetWhaleStats calculates aggregated statistics for whale alerts
func (r *Repository) GetWhaleStats(stockSymbol string, startTime, endTime time.Time) (*models.WhaleStats, error) {
	var stats models.WhaleStats

	// Base selection columns for aggregation
	aggSelect := "count(*) as total_whale_trades, sum(trigger_value) as total_whale_value, " +
		"sum(case when action = 'BUY' then trigger_volume_lots else 0 end) as buy_volume_lots, " +
		"sum(case when action = 'SELL' then trigger_volume_lots else 0 end) as sell_volume_lots, " +
		"max(trigger_value) as largest_trade_value"

	var query *gorm.DB
	if stockSymbol != "" {
		// Specific stock: Select symbol and group by it
		query = r.db.Model(&models.WhaleAlert{}).Select("stock_symbol, "+aggSelect).Where("stock_symbol = ?", stockSymbol).Group("stock_symbol")
	} else {
		// Global stats: Select static 'ALL' as symbol, no grouping (aggregates entire filtered set)
		query = r.db.Model(&models.WhaleAlert{}).Select("'ALL' as stock_symbol, " + aggSelect)
	}

	if !startTime.IsZero() {
		query = query.Where("detected_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("detected_at <= ?", endTime)
	}

	if err := query.Scan(&stats).Error; err != nil {
		return nil, fmt.Errorf("GetWhaleStats: %w", err)
	}
	return &stats, nil
}

// GetAccumulationPattern detects BUY/SELL sequences (accumulation/distribution)
// Identifies repeated whale activity grouped by stock and action
func (r *Repository) GetAccumulationPattern(hoursBack int, minAlerts int) ([]types.AccumulationPattern, error) {
	var patterns []types.AccumulationPattern

	query := `
		SELECT 
			stock_symbol,
			action,
			COUNT(*) as alert_count,
			SUM(trigger_value) as total_value,
			SUM(trigger_volume_lots) as total_volume_lots,
			MIN(detected_at) as first_alert_time,
			MAX(detected_at) as last_alert_time,
			AVG(COALESCE(z_score, 0)) as avg_z_score
		FROM whale_alerts
		WHERE detected_at >= NOW() - INTERVAL '1 hour' * ?
		GROUP BY stock_symbol, action
		HAVING COUNT(*) >= ?
		ORDER BY total_value DESC
	`

	if err := r.db.Raw(query, hoursBack, minAlerts).Scan(&patterns).Error; err != nil {
		return nil, fmt.Errorf("GetAccumulationPattern: %w", err)
	}
	return patterns, nil
}

// GetAccumulationDistributionSummary returns top 20 accumulation and top 20 distribution separately
// Data is calculated from startTime
func (r *Repository) GetAccumulationDistributionSummary(startTime time.Time) (accumulation []types.AccumulationDistributionSummary, distribution []types.AccumulationDistributionSummary, err error) {
	// Default to 24 hours if zero
	if startTime.IsZero() {
		startTime = time.Now().Add(-24 * time.Hour)
	}

	baseQuery := `
		WITH symbol_stats AS (
			SELECT 
				stock_symbol,
				SUM(CASE WHEN action = 'BUY' THEN 1 ELSE 0 END) as buy_count,
				SUM(CASE WHEN action = 'SELL' THEN 1 ELSE 0 END) as sell_count,
				SUM(CASE WHEN action = 'BUY' THEN trigger_value ELSE 0 END) as buy_value,
				SUM(CASE WHEN action = 'SELL' THEN trigger_value ELSE 0 END) as sell_value,
				COUNT(*) as total_count,
				SUM(trigger_value) as total_value
			FROM whale_alerts
			WHERE detected_at >= ?
			GROUP BY stock_symbol
		)
		SELECT 
			stock_symbol,
			buy_count,
			sell_count,
			buy_value,
			sell_value,
			total_count,
			total_value,
			CASE 
				WHEN total_count > 0 THEN ROUND((buy_count::DECIMAL / total_count * 100), 2)
				ELSE 0 
			END as buy_percentage,
			CASE 
				WHEN total_count > 0 THEN ROUND((sell_count::DECIMAL / total_count * 100), 2)
				ELSE 0 
			END as sell_percentage,
			CASE 
				WHEN buy_count > sell_count AND (buy_count::DECIMAL / NULLIF(total_count, 0)) > 0.55 THEN 'ACCUMULATION'
				WHEN sell_count > buy_count AND (sell_count::DECIMAL / NULLIF(total_count, 0)) > 0.55 THEN 'DISTRIBUTION'
				ELSE 'NEUTRAL'
			END as status,
			buy_value - sell_value as net_value
		FROM symbol_stats
	`

	// Get top 20 ACCUMULATION (sorted by net_value DESC - highest positive)
	accumulationQuery := baseQuery + `
		WHERE (buy_count::DECIMAL / NULLIF(total_count, 0)) > 0.55 
		  AND buy_count > sell_count
		ORDER BY net_value DESC
		LIMIT 20
	`
	if err := r.db.Raw(accumulationQuery, startTime).Scan(&accumulation).Error; err != nil {
		return nil, nil, fmt.Errorf("GetAccumulationDistributionSummary: %w", err)
	}

	// Get top 20 DISTRIBUTION (sorted by net_value ASC - highest negative)
	distributionQuery := baseQuery + `
		WHERE (sell_count::DECIMAL / NULLIF(total_count, 0)) > 0.55 
		  AND sell_count > buy_count
		ORDER BY net_value ASC
		LIMIT 20
	`
	if err := r.db.Raw(distributionQuery, startTime).Scan(&distribution).Error; err != nil {
		return nil, nil, fmt.Errorf("GetAccumulationDistributionSummary: %w", err)
	}

	return accumulation, distribution, nil
}

// GetExtremeAnomalies returns alerts with Z-Score > minZScore
func (r *Repository) GetExtremeAnomalies(minZScore float64, hoursBack int) ([]models.WhaleAlert, error) {
	var anomalies []models.WhaleAlert

	err := r.db.Where("z_score >= ? AND detected_at >= NOW() - INTERVAL '1 hour' * ?", minZScore, hoursBack).
		Order("z_score DESC").
		Limit(50).
		Find(&anomalies).Error

	if err != nil {
		return nil, fmt.Errorf("GetExtremeAnomalies: %w", err)
	}
	return anomalies, nil
}

// GetTimeBasedStats returns whale activity distribution by hour
func (r *Repository) GetTimeBasedStats(daysBack int) ([]types.TimeBasedStat, error) {
	var stats []types.TimeBasedStat

	query := `
		SELECT 
			EXTRACT(HOUR FROM (detected_at AT TIME ZONE 'Asia/Jakarta'))::TEXT as time_bucket,
			COUNT(*) as alert_count,
			SUM(trigger_value) as total_value,
			AVG(COALESCE(z_score, 0)) as avg_z_score,
			SUM(CASE WHEN action = 'BUY' THEN 1 ELSE 0 END) as buy_count,
			SUM(CASE WHEN action = 'SELL' THEN 1 ELSE 0 END) as sell_count
		FROM whale_alerts
		WHERE detected_at >= NOW() - INTERVAL '1 day' * ?
		GROUP BY EXTRACT(HOUR FROM (detected_at AT TIME ZONE 'Asia/Jakarta'))
		ORDER BY time_bucket
	`

	if err := r.db.Raw(query, daysBack).Scan(&stats).Error; err != nil {
		return nil, fmt.Errorf("GetTimeBasedStats: %w", err)
	}
	return stats, nil
}

// GetRecentAlertsBySymbol returns recent alerts for a specific stock (for LLM context)
func (r *Repository) GetRecentAlertsBySymbol(symbol string, limit int) ([]models.WhaleAlert, error) {
	var alerts []models.WhaleAlert

	err := r.db.Where("stock_symbol = ?", symbol).
		Order("detected_at DESC").
		Limit(limit).
		Find(&alerts).Error

	if err != nil {
		return nil, fmt.Errorf("GetRecentAlertsBySymbol: %w", err)
	}
	return alerts, nil
}

// SaveWhaleFollowup creates a new whale alert followup record
func (r *Repository) SaveWhaleFollowup(followup *models.WhaleAlertFollowup) error {
	if err := r.db.Create(followup).Error; err != nil {
		return fmt.Errorf("SaveWhaleFollowup: %w", err)
	}
	return nil
}

// UpdateWhaleFollowup updates specific fields of a whale followup
func (r *Repository) UpdateWhaleFollowup(alertID int64, updates map[string]interface{}) error {
	if err := r.db.Model(&models.WhaleAlertFollowup{}).
		Where("whale_alert_id = ?", alertID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("UpdateWhaleFollowup: %w", err)
	}
	return nil
}

// GetWhaleFollowup retrieves followup data for a specific whale alert
func (r *Repository) GetWhaleFollowup(alertID int64) (*models.WhaleAlertFollowup, error) {
	var followup models.WhaleAlertFollowup
	err := r.db.Where("whale_alert_id = ?", alertID).First(&followup).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("GetWhaleFollowup: %w", err)
	}
	return &followup, nil
}

// GetWhaleFollowupsByAlertIDs retrieves followups for a list of alert IDs (batch fetch)
func (r *Repository) GetWhaleFollowupsByAlertIDs(alertIDs []int64) ([]models.WhaleAlertFollowup, error) {
	if len(alertIDs) == 0 {
		return nil, nil
	}

	var followups []models.WhaleAlertFollowup
	if err := r.db.Where("whale_alert_id IN ?", alertIDs).Find(&followups).Error; err != nil {
		return nil, fmt.Errorf("GetWhaleFollowupsByAlertIDs: %w", err)
	}
	return followups, nil
}

// GetPendingFollowups retrieves whale alerts that need followup updates
func (r *Repository) GetPendingFollowups(maxAge time.Duration) ([]models.WhaleAlertFollowup, error) {
	var followups []models.WhaleAlertFollowup
	cutoffTime := time.Now().Add(-maxAge)

	// Get followups where latest price update is still pending
	err := r.db.Where("alert_time >= ?", cutoffTime).
		Where("price_1day_later IS NULL"). // Still tracking
		Order("alert_time ASC").
		Find(&followups).Error

	if err != nil {
		return nil, fmt.Errorf("GetPendingFollowups: %w", err)
	}
	return followups, nil
}

// GetWhaleFollowups retrieves list of whale followups with filters
func (r *Repository) GetWhaleFollowups(symbol, status string, limit int) ([]models.WhaleAlertFollowup, error) {
	var followups []models.WhaleAlertFollowup

	query := r.db.Order("alert_time DESC")

	// Filter by symbol if provided
	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
	}

	// Filter by status if provided
	if status == "active" {
		// Active followups: being tracked (not completed 1-day followup)
		query = query.Where("price_1day_later IS NULL")
	} else if status == "completed" {
		// Completed followups: 1-day followup done
		query = query.Where("price_1day_later IS NOT NULL")
	}
	// "all" or empty status returns all followups

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&followups).Error; err != nil {
		return nil, fmt.Errorf("GetWhaleFollowups: %w", err)
	}
	return followups, nil
}

// GetActiveWebhooks retrieves all active webhooks
func (r *Repository) GetActiveWebhooks() ([]models.WhaleWebhook, error) {
	var webhooks []models.WhaleWebhook
	err := r.db.Where("is_active = ?", true).Find(&webhooks).Error
	if err != nil {
		return nil, fmt.Errorf("GetActiveWebhooks: %w", err)
	}
	return webhooks, nil
}

// SaveWebhookLog saves a new webhook delivery log
func (r *Repository) SaveWebhookLog(log *models.WhaleWebhookLog) error {
	if err := r.db.Create(log).Error; err != nil {
		return fmt.Errorf("SaveWebhookLog: %w", err)
	}
	return nil
}

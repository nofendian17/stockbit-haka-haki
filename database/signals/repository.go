package signals

import (
	"fmt"
	"log"
	"sort"
	"time"

	"stockbit-haka-haki/database/analytics"
	models "stockbit-haka-haki/database/models_pkg"
	"stockbit-haka-haki/database/types"

	"gorm.io/gorm"
)

// Repository handles database operations for trading signals
type Repository struct {
	db        *gorm.DB
	analytics *analytics.Repository
}

// SetAnalyticsRepository sets the analytics repository for strategy evaluation
func (r *Repository) SetAnalyticsRepository(analyticsRepo *analytics.Repository) {
	r.analytics = analyticsRepo
}

// NewRepository creates a new signals repository
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// SaveTradingSignal persists a trading signal to the database
func (r *Repository) SaveTradingSignal(signal *models.TradingSignalDB) error {
	if err := r.db.Create(signal).Error; err != nil {
		return fmt.Errorf("SaveTradingSignal: %w", err)
	}
	return nil
}

// GetTradingSignals retrieves trading signals with filters
func (r *Repository) GetTradingSignals(symbol string, strategy string, decision string, startTime, endTime time.Time, limit int) ([]models.TradingSignalDB, error) {
	var signals []models.TradingSignalDB
	query := r.db.Order("generated_at DESC")

	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
	}
	if strategy != "" {
		query = query.Where("strategy = ?", strategy)
	}
	if decision != "" {
		query = query.Where("decision = ?", decision)
	}
	if !startTime.IsZero() {
		query = query.Where("generated_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("generated_at <= ?", endTime)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&signals).Error; err != nil {
		return nil, fmt.Errorf("GetTradingSignals: %w", err)
	}
	return signals, nil
}

// GetSignalByID retrieves a specific signal by ID
func (r *Repository) GetSignalByID(id int64) (*models.TradingSignalDB, error) {
	var signal models.TradingSignalDB
	err := r.db.First(&signal, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("GetSignalByID: %w", err)
	}
	return &signal, nil
}

// SaveSignalOutcome creates a new signal outcome record
func (r *Repository) SaveSignalOutcome(outcome *models.SignalOutcome) error {
	if err := r.db.Create(outcome).Error; err != nil {
		return fmt.Errorf("SaveSignalOutcome: %w", err)
	}
	return nil
}

// UpdateSignalOutcome updates an existing signal outcome
func (r *Repository) UpdateSignalOutcome(outcome *models.SignalOutcome) error {
	if err := r.db.Save(outcome).Error; err != nil {
		return fmt.Errorf("UpdateSignalOutcome: %w", err)
	}
	return nil
}

// GetSignalOutcomes retrieves signal outcomes with filters
func (r *Repository) GetSignalOutcomes(symbol string, status string, startTime, endTime time.Time, limit int) ([]models.SignalOutcome, error) {
	var outcomes []models.SignalOutcome
	query := r.db.Order("entry_time DESC")

	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
	}
	if status != "" {
		query = query.Where("outcome_status = ?", status)
	}
	if !startTime.IsZero() {
		query = query.Where("entry_time >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("entry_time <= ?", endTime)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&outcomes).Error; err != nil {
		return nil, fmt.Errorf("GetSignalOutcomes: %w", err)
	}
	return outcomes, nil
}

// GetSignalOutcomeBySignalID retrieves outcome for a specific signal
func (r *Repository) GetSignalOutcomeBySignalID(signalID int64) (*models.SignalOutcome, error) {
	var outcome models.SignalOutcome
	err := r.db.Where("signal_id = ?", signalID).First(&outcome).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("GetSignalOutcomeBySignalID: %w", err)
	}
	return &outcome, nil
}

// GetOpenSignals retrieves signals that don't have outcomes yet
func (r *Repository) GetOpenSignals(limit int) ([]models.TradingSignalDB, error) {
	var signals []models.TradingSignalDB

	// Subquery to find signal IDs that already have outcomes
	subQuery := r.db.Model(&models.SignalOutcome{}).Select("signal_id")

	// Get signals NOT IN the subquery
	query := r.db.Where("id NOT IN (?)", subQuery).
		Where("decision IN ('BUY', 'SELL')"). // Only actionable signals
		Order("generated_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&signals).Error; err != nil {
		return nil, fmt.Errorf("GetOpenSignals: %w", err)
	}
	return signals, nil
}

// GetSignalPerformanceStats calculates performance statistics
func (r *Repository) GetSignalPerformanceStats(strategy string, symbol string) (*types.PerformanceStats, error) {
	var stats types.PerformanceStats

	query := `
		SELECT
			ts.strategy,
			ts.stock_symbol,
			COUNT(*) AS total_signals,
			SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END) AS wins,
			SUM(CASE WHEN so.outcome_status = 'LOSS' THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN so.outcome_status = 'OPEN' THEN 1 ELSE 0 END) AS open_positions,
			ROUND(
				(SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END)::DECIMAL /
					NULLIF(SUM(CASE WHEN so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN') THEN 1 ELSE 0 END), 0)) * 100,
				2
			) AS win_rate,
			COALESCE(AVG(so.profit_loss_pct), 0) AS avg_profit_pct,
			COALESCE(SUM(so.profit_loss_pct), 0) AS total_profit_pct,
			COALESCE(MAX(so.profit_loss_pct), 0) AS max_win_pct,
			COALESCE(MIN(so.profit_loss_pct), 0) AS max_loss_pct,
			COALESCE(AVG(so.risk_reward_ratio), 0) AS avg_risk_reward,
			(COALESCE(AVG(so.profit_loss_pct), 0) *
			 (SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END)::DECIMAL / NULLIF(SUM(CASE WHEN so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN') THEN 1 ELSE 0 END), 0))
			) AS expectancy
		FROM trading_signals ts
		JOIN signal_outcomes so ON ts.id = so.signal_id AND date_trunc('day', ts.generated_at) = date_trunc('day', so.entry_time)
		WHERE so.outcome_status IN ('WIN', 'LOSS', 'BREAKEVEN', 'OPEN')
	`

	var args []interface{}
	if strategy != "" && strategy != "ALL" {
		query += " AND ts.strategy = ?"
		args = append(args, strategy)
	}
	if symbol != "" {
		query += " AND ts.stock_symbol = ?"
		args = append(args, symbol)
	}

	query += " GROUP BY ts.strategy, ts.stock_symbol"

	if err := r.db.Raw(query, args...).Scan(&stats).Error; err != nil {
		return nil, fmt.Errorf("GetSignalPerformanceStats: %w", err)
	}

	return &stats, nil
}

// GetDailyStrategyPerformance retrieves daily aggregated performance data
func (r *Repository) GetDailyStrategyPerformance(strategy, symbol string, limit int) ([]map[string]interface{}, error) {
	// Refresh materialized view to ensure latest data
	if err := r.db.Exec(`REFRESH MATERIALIZED VIEW strategy_performance_daily`).Error; err != nil {
		// Log but don't fail - use existing data
		fmt.Printf("⚠️ Failed to refresh performance view: %v\n", err)
	}

	var results []map[string]interface{}
	query := r.db.Table("strategy_performance_daily").Order("day DESC")

	if strategy != "" && strategy != "ALL" {
		query = query.Where("strategy = ?", strategy)
	}
	if symbol != "" {
		query = query.Where("stock_symbol = ?", symbol)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&results).Error; err != nil {
		return nil, fmt.Errorf("GetDailyStrategyPerformance: %w", err)
	}
	return results, nil
}

// EvaluateVolumeBreakoutStrategy implements Volume Breakout Validation strategy
// Logic: Price increase (>2%) + explosive volume (z-score > 3) = BUY signal
func (r *Repository) EvaluateVolumeBreakoutStrategy(alert *models.WhaleAlert, zscores *types.ZScoreData) *models.TradingSignal {
	signal := &models.TradingSignal{
		StockSymbol:  alert.StockSymbol,
		Timestamp:    alert.DetectedAt,
		Strategy:     "VOLUME_BREAKOUT",
		PriceZScore:  zscores.PriceZScore,
		VolumeZScore: zscores.VolumeZScore,
		Price:        alert.TriggerPrice,
		Volume:       alert.TriggerVolumeLots,
		Change:       zscores.PriceChange,
	}

	// Check conditions: change > 2% AND volume_z_score > 3
	if zscores.PriceChange > 2.0 && zscores.VolumeZScore > 3.0 {
		signal.Decision = "BUY"
		signal.Confidence = calculateConfidence(zscores.VolumeZScore, 3.0, 6.0)
		signal.Reason = fmt.Sprintf("Kenaikan harga %.2f%% didukung volume meledak (Z=%.2f). Entry valid ✓",
			zscores.PriceChange, zscores.VolumeZScore)
	} else if zscores.PriceChange > 2.0 && zscores.VolumeZScore <= 3.0 {
		signal.Decision = "WAIT"
		signal.Confidence = 0.3
		signal.Reason = fmt.Sprintf("Harga naik %.2f%% tapi volume biasa saja (Z=%.2f). Tunggu konfirmasi.",
			zscores.PriceChange, zscores.VolumeZScore)
	} else {
		signal.Decision = "NO_TRADE"
		signal.Confidence = 0.1
		signal.Reason = "Tidak ada breakout signifikan"
	}

	return signal
}

// EvaluateMeanReversionStrategy implements Mean Reversion (Contrarian) strategy
// Logic: Extreme price (z-score > 4) + declining volume = SELL signal (overbought)
func (r *Repository) EvaluateMeanReversionStrategy(alert *models.WhaleAlert, zscores *types.ZScoreData, prevVolumeZScore float64) *models.TradingSignal {
	signal := &models.TradingSignal{
		StockSymbol:  alert.StockSymbol,
		Timestamp:    alert.DetectedAt,
		Strategy:     "MEAN_REVERSION",
		PriceZScore:  zscores.PriceZScore,
		VolumeZScore: zscores.VolumeZScore,
		Price:        alert.TriggerPrice,
		Volume:       alert.TriggerVolumeLots,
		Change:       zscores.PriceChange,
	}

	// Detect volume divergence
	volumeDeclining := zscores.VolumeZScore < prevVolumeZScore

	// Check conditions: price_z_score > 4 AND volume declining
	if zscores.PriceZScore > 4.0 && volumeDeclining {
		signal.Decision = "SELL"
		signal.Confidence = calculateConfidence(zscores.PriceZScore, 4.0, 7.0)
		signal.Reason = fmt.Sprintf("Harga overextended (Z=%.2f), volume menurun. Mean reversion imminent ⚠️",
			zscores.PriceZScore)
	} else if zscores.PriceZScore > 4.0 {
		signal.Decision = "WAIT"
		signal.Confidence = 0.5
		signal.Reason = fmt.Sprintf("Harga overbought (Z=%.2f) tapi volume masih kuat. Monitor divergence.",
			zscores.PriceZScore)
	} else if zscores.PriceZScore < -4.0 {
		signal.Decision = "BUY"
		signal.Confidence = calculateConfidence(-zscores.PriceZScore, 4.0, 7.0)
		signal.Reason = fmt.Sprintf("Harga oversold (Z=%.2f). Potential bounce 📈",
			zscores.PriceZScore)
	} else {
		signal.Decision = "NO_TRADE"
		signal.Confidence = 0.1
		signal.Reason = "Harga dalam range normal"
	}

	return signal
}

// EvaluateFakeoutFilterStrategy implements Fakeout Filter (Defense) strategy
// Logic: Price breakout + low volume (z-score < 1) = NO_TRADE (likely bull trap)
func (r *Repository) EvaluateFakeoutFilterStrategy(alert *models.WhaleAlert, zscores *types.ZScoreData) *models.TradingSignal {
	signal := &models.TradingSignal{
		StockSymbol:  alert.StockSymbol,
		Timestamp:    alert.DetectedAt,
		Strategy:     "FAKEOUT_FILTER",
		PriceZScore:  zscores.PriceZScore,
		VolumeZScore: zscores.VolumeZScore,
		Price:        alert.TriggerPrice,
		Volume:       alert.TriggerVolumeLots,
		Change:       zscores.PriceChange,
	}

	// Detect potential breakout (price moving significantly)
	isBreakout := zscores.PriceChange > 3.0 || zscores.PriceZScore > 2.0

	// Check volume strength
	if isBreakout && zscores.VolumeZScore < 1.0 {
		signal.Decision = "NO_TRADE"
		signal.Confidence = 0.8 // High confidence to AVOID
		signal.Reason = fmt.Sprintf("⚠️ FAKEOUT DETECTED! Breakout tanpa volume (Z=%.2f). Bull trap!",
			zscores.VolumeZScore)
	} else if isBreakout && zscores.VolumeZScore >= 2.0 {
		signal.Decision = "BUY"
		signal.Confidence = calculateConfidence(zscores.VolumeZScore, 2.0, 5.0)
		signal.Reason = fmt.Sprintf("✓ Breakout valid dengan volume kuat (Z=%.2f). Safe entry.",
			zscores.VolumeZScore)
	} else if isBreakout {
		signal.Decision = "WAIT"
		signal.Confidence = 0.4
		signal.Reason = fmt.Sprintf("Breakout dengan volume moderate (Z=%.2f). Tunggu konfirmasi.",
			zscores.VolumeZScore)
	} else {
		signal.Decision = "NO_TRADE"
		signal.Confidence = 0.1
		signal.Reason = "Tidak ada breakout terdeteksi"
	}

	return signal
}

// GetStrategySignals evaluates recent whale alerts and generates trading signals
func (r *Repository) GetStrategySignals(lookbackMinutes int, minConfidence float64, strategyFilter string, alerts []models.WhaleAlert, baseline *models.StatisticalBaseline, regime *models.MarketRegime, patterns []models.DetectedPattern) ([]models.TradingSignal, error) {
	var signals []models.TradingSignal

	// Track previous volume z-scores for divergence detection
	prevVolumeZScores := make(map[string]float64)

	// Create regime map for quick lookup
	regimeMap := make(map[string]*models.MarketRegime)
	if regime != nil {
		regimeMap[regime.StockSymbol] = regime
	}

	for _, alert := range alerts {
		// Use the provided baseline
		if baseline == nil {
			log.Printf("⚠️ No baseline provided for %s", alert.StockSymbol)
			continue
		}

		// Calculate z-scores using persistent baseline if available
		volumeLots := alert.TriggerVolumeLots
		var zscores *types.ZScoreData

		if baseline.SampleSize > 30 {
			// Calculate Z-Score using persistent baseline (more accurate)
			priceZ := (alert.TriggerPrice - baseline.MeanPrice) / baseline.StdDevPrice
			volZ := (volumeLots - baseline.MeanVolumeLots) / baseline.StdDevVolume
			zscores = &types.ZScoreData{
				PriceZScore:  priceZ,
				VolumeZScore: volZ,
				SampleCount:  int64(baseline.SampleSize),
			}
		} else {
			// Skip if insufficient data
			continue
		}

		// Get regime for this stock symbol
		regimeForSymbol := regimeMap[alert.StockSymbol]

		// Evaluate each strategy
		strategies := []string{"VOLUME_BREAKOUT", "MEAN_REVERSION", "FAKEOUT_FILTER"}
		if strategyFilter != "" && strategyFilter != "ALL" {
			strategies = []string{strategyFilter}
		}

		for _, strategy := range strategies {
			var signal *models.TradingSignal

			switch strategy {
			case "VOLUME_BREAKOUT":
				signal = r.EvaluateVolumeBreakoutStrategy(&alert, zscores)
				// Filter breakouts in RANGING markets
				if signal != nil && regimeForSymbol != nil && regimeForSymbol.Regime == "RANGING" && regimeForSymbol.Confidence > 0.6 {
					signal.Confidence *= 0.5 // Deprioritize
					signal.Reason += " (Filtered: Ranging Market)"
				}
			case "MEAN_REVERSION":
				prevZScore := prevVolumeZScores[alert.StockSymbol]
				signal = r.EvaluateMeanReversionStrategy(&alert, zscores, prevZScore)
				// Boost Mean Reversion in RANGING/VOLATILE markets
				if signal != nil && regimeForSymbol != nil && (regimeForSymbol.Regime == "RANGING" || regimeForSymbol.Regime == "VOLATILE") {
					signal.Confidence *= 1.2
					signal.Reason += " (Regime Boost)"
				}
			case "FAKEOUT_FILTER":
				signal = r.EvaluateFakeoutFilterStrategy(&alert, zscores)
			}

			// Pattern Confirmation
			if signal != nil && len(patterns) > 0 {
				for _, p := range patterns {
					if p.StockSymbol == alert.StockSymbol && p.PatternType == "RANGE_BREAKOUT" && p.PatternDirection != nil {
						if *p.PatternDirection == signal.Decision {
							signal.Confidence *= 1.3 // Strong confirmation
							signal.Reason += fmt.Sprintf(" (Confirmed by %s)", p.PatternType)
							break
						}
					}
				}
			}

			// Only include signals meeting confidence threshold
			if signal != nil && signal.Confidence >= minConfidence && signal.Decision != "NO_TRADE" {
				signals = append(signals, *signal)
			}
		}

		// Update previous volume z-score
		prevVolumeZScores[alert.StockSymbol] = zscores.VolumeZScore
	}

	// Sort signals by timestamp DESC (newest first), then by strategy name for consistency
	sort.Slice(signals, func(i, j int) bool {
		// First, sort by timestamp (newest first)
		if !signals[i].Timestamp.Equal(signals[j].Timestamp) {
			return signals[i].Timestamp.After(signals[j].Timestamp)
		}
		// If timestamps are equal, sort by strategy name alphabetically
		return signals[i].Strategy < signals[j].Strategy
	})

	return signals, nil
}

// getWhaleAlertsForStrategy fetches whale alerts for strategy evaluation
func (r *Repository) getWhaleAlertsForStrategy(startTime time.Time) ([]models.WhaleAlert, error) {
	var alerts []models.WhaleAlert

	query := r.db.Where("detected_at >= ?", startTime).
		Order("detected_at DESC")

	if err := query.Find(&alerts).Error; err != nil {
		return nil, fmt.Errorf("getWhaleAlertsForStrategy: %w", err)
	}

	return alerts, nil
}

// getMarketRegimesForStrategy fetches market regimes for strategy context
func (r *Repository) getMarketRegimesForStrategy() (map[string]*models.MarketRegime, error) {
	var regimes []models.MarketRegime
	regimeMap := make(map[string]*models.MarketRegime)

	// Get the most recent regime for each stock symbol
	subQuery := r.db.Model(&models.MarketRegime{}).
		Select("stock_symbol, MAX(detected_at) as max_detected_at").
		Group("stock_symbol")

	query := r.db.Joins("JOIN (?) AS latest ON market_regimes.stock_symbol = latest.stock_symbol AND market_regimes.detected_at = latest.max_detected_at", subQuery).
		Order("market_regimes.detected_at DESC")

	if err := query.Find(&regimes).Error; err != nil {
		return nil, fmt.Errorf("getMarketRegimesForStrategy: %w", err)
	}

	for _, regime := range regimes {
		regimeCopy := regime
		regimeMap[regime.StockSymbol] = &regimeCopy
	}

	return regimeMap, nil
}

// getDetectedPatternsForStrategy fetches detected patterns for strategy confirmation
func (r *Repository) getDetectedPatternsForStrategy(startTime time.Time) ([]models.DetectedPattern, error) {
	var patterns []models.DetectedPattern

	query := r.db.Where("detected_at >= ?", startTime).
		Where("pattern_type = ?", "RANGE_BREAKOUT"). // Only get range breakout patterns for now
		Order("detected_at DESC")

	if err := query.Find(&patterns).Error; err != nil {
		return nil, fmt.Errorf("getDetectedPatternsForStrategy: %w", err)
	}

	return patterns, nil
}

// calculateConfidence converts z-score range to confidence percentage
func calculateConfidence(value, minThreshold, maxThreshold float64) float64 {
	if value < minThreshold {
		return 0.0
	}
	if value >= maxThreshold {
		return 1.0
	}

	// Prevent division by zero
	denominator := maxThreshold - minThreshold
	if denominator <= 0 {
		return 0.5 // Return neutral confidence if thresholds are invalid
	}

	// Linear interpolation between min and max
	confidence := (value - minThreshold) / denominator

	// Clamp to [0, 1] range to prevent overflow
	if confidence < 0 {
		return 0.0
	}
	if confidence > 1 {
		return 1.0
	}

	return confidence
}

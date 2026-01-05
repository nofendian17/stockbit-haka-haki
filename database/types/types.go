package types

import "time"

// StockStats holds aggregated statistical data for a stock
type StockStats struct {
	MeanVolumeLots float64 `json:"mean_volume_lots"`
	StdDevVolume   float64 `json:"std_dev_volume"`
	MeanValue      float64 `json:"mean_value"`
	StdDevValue    float64 `json:"std_dev_value"`
	MeanPrice      float64 `json:"mean_price"`
	SampleCount    int64   `json:"sample_count"`
}

// ZScoreData holds z-score calculations for price and volume
type ZScoreData struct {
	PriceZScore  float64 `json:"price_z_score"`
	VolumeZScore float64 `json:"volume_z_score"`
	MeanPrice    float64 `json:"mean_price"`
	StdDevPrice  float64 `json:"std_dev_price"`
	MeanVolume   float64 `json:"mean_volume"`
	StdDevVolume float64 `json:"std_dev_volume"`
	SampleCount  int64   `json:"sample_count"`
	PriceChange  float64 `json:"price_change"`
	VolumeChange float64 `json:"volume_change"`
}

// AccumulationPattern represents detected accumulation/distribution pattern
type AccumulationPattern struct {
	StockSymbol     string    `json:"stock_symbol"`
	Action          string    `json:"action"`
	AlertCount      int64     `json:"alert_count"`
	TotalValue      float64   `json:"total_value"`
	TotalVolumeLots float64   `json:"total_volume_lots"`
	FirstAlertTime  time.Time `json:"first_alert_time"`
	LastAlertTime   time.Time `json:"last_alert_time"`
	AvgZScore       float64   `json:"avg_z_score"`
}

// AccumulationDistributionSummary represents accumulation vs distribution summary per symbol
type AccumulationDistributionSummary struct {
	StockSymbol    string  `json:"stock_symbol"`
	BuyCount       int64   `json:"buy_count"`
	SellCount      int64   `json:"sell_count"`
	BuyValue       float64 `json:"buy_value"`
	SellValue      float64 `json:"sell_value"`
	TotalCount     int64   `json:"total_count"`
	TotalValue     float64 `json:"total_value"`
	BuyPercentage  float64 `json:"buy_percentage"`
	SellPercentage float64 `json:"sell_percentage"`
	Status         string  `json:"status"`
	NetValue       float64 `json:"net_value"`
}

// TimeBasedStat represents whale activity statistics by time bucket
type TimeBasedStat struct {
	TimeBucket string  `json:"time_bucket"`
	AlertCount int64   `json:"alert_count"`
	TotalValue float64 `json:"total_value"`
	AvgZScore  float64 `json:"avg_z_score"`
	BuyCount   int64   `json:"buy_count"`
	SellCount  int64   `json:"sell_count"`
}

// PerformanceStats holds aggregated performance metrics
type PerformanceStats struct {
	Strategy       string  `json:"strategy"`
	StockSymbol    string  `json:"stock_symbol"`
	TotalSignals   int64   `json:"total_signals"`
	Wins           int64   `json:"wins"`
	Losses         int64   `json:"losses"`
	OpenPositions  int64   `json:"open_positions"`
	WinRate        float64 `json:"win_rate"`
	AvgProfitPct   float64 `json:"avg_profit_pct"`
	TotalProfitPct float64 `json:"total_profit_pct"`
	MaxWinPct      float64 `json:"max_win_pct"`
	MaxLossPct     float64 `json:"max_loss_pct"`
	AvgRiskReward  float64 `json:"avg_risk_reward"`
	Expectancy     float64 `json:"expectancy"`
}

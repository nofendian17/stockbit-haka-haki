package signals

import (
	models "stockbit-haka-haki/database/models_pkg"
	"stockbit-haka-haki/database/types"
	"testing"
	"time"
)

func TestEvaluateVolumeBreakoutStrategy(t *testing.T) {
	repo := &Repository{} // logic doesn't use db connection

	now := time.Now()
	alert := &models.WhaleAlert{
		StockSymbol:       "BBCA",
		DetectedAt:        now,
		TriggerPrice:      10000,
		TriggerVolumeLots: 5000,
	}

	tests := []struct {
		name           string
		zscores        *types.ZScoreData
		vwap           float64
		orderFlow      *models.OrderFlowImbalance
		expectedDec    string
		expectedReason string
	}{
		{
			name: "Strong Buy - High Price Z & High Vol Z",
			zscores: &types.ZScoreData{
				PriceZScore:  2.5, // > 2.0 (Threshold will be > 2.0)
				VolumeZScore: 3.5, // > 2.5
				PriceChange:  1.0, // Low raw change, but high Z-score
			},
			vwap: 9900, // Price > VWAP
			orderFlow: &models.OrderFlowImbalance{
				AggressiveBuyPct: floatPtr(60.0),
			},
			expectedDec: "BUY",
		},
		{
			name: "No Trade - Low Price Z",
			zscores: &types.ZScoreData{
				PriceZScore:  0.5,
				VolumeZScore: 3.5,
				PriceChange:  3.0, // High raw change, but low Z-score (Fakeout check might catch this, but here purely testing breakout logic)
			},
			vwap:        9900,
			expectedDec: "NO_TRADE", // Assuming we switch to Z-score based logic where Z > 2.0 is required
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signal := repo.EvaluateVolumeBreakoutStrategy(alert, tt.zscores, nil, tt.vwap, tt.orderFlow)
			if signal.Decision != tt.expectedDec {
				t.Errorf("expected decision %s, got %s", tt.expectedDec, signal.Decision)
			}
		})
	}
}

func TestEvaluateMeanReversionStrategy(t *testing.T) {
	repo := &Repository{}

	now := time.Now()
	alert := &models.WhaleAlert{
		StockSymbol:       "BBCA",
		DetectedAt:        now,
		TriggerPrice:      10000,
		TriggerVolumeLots: 5000,
	}

	tests := []struct {
		name        string
		zscores     *types.ZScoreData
		prevVolZ    float64
		vwap        float64
		expectedDec string
	}{
		{
			name: "Sell - High Price Z & Declining Volume",
			zscores: &types.ZScoreData{
				PriceZScore:  3.8, // > 3.0
				VolumeZScore: 2.0,
			},
			prevVolZ:    3.0, // Declining
			expectedDec: "SELL",
		},
		{
			name: "Wait - High Price Z but Volume Increasing",
			zscores: &types.ZScoreData{
				PriceZScore:  3.8,
				VolumeZScore: 3.5,
			},
			prevVolZ:    2.0,
			expectedDec: "WAIT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signal := repo.EvaluateMeanReversionStrategy(alert, tt.zscores, tt.prevVolZ, nil, tt.vwap, nil)
			if signal.Decision != tt.expectedDec {
				t.Errorf("expected decision %s, got %s", tt.expectedDec, signal.Decision)
			}
		})
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

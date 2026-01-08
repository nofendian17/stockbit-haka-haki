package app

import (
	"fmt"
	"log"
	"math"
	"time"

	"stockbit-haka-haki/config"
	"stockbit-haka-haki/database"
)

// ATR Calculation Constants
const (
	ATRPeriod = 14 // Standard ATR period (14 candles)

	// Exit Level Multipliers (based on ATR)
	// Fallback values when ATR cannot be calculated
	FallbackStopLossPct    = 2.0 // -2% default stop loss
	FallbackTakeProfit1Pct = 4.0 // +4% default TP1
	FallbackTakeProfit2Pct = 8.0 // +8% default TP2
)

// ExitLevels contains calculated exit levels for a position
type ExitLevels struct {
	ATR              float64 // ATR value at calculation time
	ATRPercent       float64 // ATR as percentage of price
	InitialStopPct   float64 // Stop loss percentage (negative)
	TrailingStopPct  float64 // Trailing stop offset percentage
	TakeProfit1Pct   float64 // First take profit percentage
	TakeProfit2Pct   float64 // Final take profit percentage
	StopLossPrice    float64 // Absolute stop loss price
	TakeProfit1Price float64 // Absolute TP1 price
	TakeProfit2Price float64 // Absolute TP2 price
	CalculatedAt     time.Time
}

// ExitStrategyCalculator calculates dynamic exit levels based on ATR
type ExitStrategyCalculator struct {
	repo *database.TradeRepository
	cfg  *config.Config
}

// NewExitStrategyCalculator creates a new exit strategy calculator
func NewExitStrategyCalculator(repo *database.TradeRepository, cfg *config.Config) *ExitStrategyCalculator {
	return &ExitStrategyCalculator{
		repo: repo,
		cfg:  cfg,
	}
}

// CalculateATR calculates the Average True Range for a symbol
// Uses 5-minute candles for better intraday precision
func (esc *ExitStrategyCalculator) CalculateATR(symbol string) (float64, error) {
	// Get recent candles (need ATRPeriod + 1 for TR calculation)
	candles, err := esc.repo.GetCandlesByTimeframe("5min", symbol, ATRPeriod+5)
	if err != nil {
		return 0, err
	}

	if len(candles) < ATRPeriod+1 {
		// Not enough data, return 0 (will trigger fallback)
		return 0, nil
	}

	// Calculate True Range for each candle
	var trueRanges []float64
	for i := 1; i < len(candles); i++ {
		high := getFloat(candles[i], "high")
		low := getFloat(candles[i], "low")
		prevClose := getFloat(candles[i-1], "close")

		// True Range = max(H-L, |H-PrevC|, |L-PrevC|)
		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		tr := math.Max(tr1, math.Max(tr2, tr3))
		trueRanges = append(trueRanges, tr)
	}

	if len(trueRanges) < ATRPeriod {
		return 0, nil
	}

	// Calculate ATR using Wilder's smoothing (exponential)
	// Initial ATR = SMA of first ATRPeriod true ranges
	atr := 0.0
	for i := 0; i < ATRPeriod; i++ {
		atr += trueRanges[i]
	}
	atr /= float64(ATRPeriod)

	// Apply smoothing for remaining data points
	// ATR = (PrevATR × (n-1) + CurrentTR) / n
	for i := ATRPeriod; i < len(trueRanges); i++ {
		atr = (atr*float64(ATRPeriod-1) + trueRanges[i]) / float64(ATRPeriod)
	}

	return atr, nil
}

// GetVolatilityPercent returns the current ATR as a percentage of price
// Implements VolatilityProvider interface
func (esc *ExitStrategyCalculator) GetVolatilityPercent(symbol string) (float64, error) {
	// Optimization: This should ideally be cached
	atr, err := esc.CalculateATR(symbol)
	if err != nil {
		return 0, err
	}

	// Get latest price to normalize ATR
	candles, err := esc.repo.GetCandlesByTimeframe("5min", symbol, 1)
	if err != nil || len(candles) == 0 {
		return 0, fmt.Errorf("no price data for %s", symbol)
	}

	closePrice := getFloat(candles[0], "close")
	if closePrice == 0 {
		return 0, fmt.Errorf("zero price for %s", symbol)
	}

	return (atr / closePrice) * 100, nil
}

// GetExitLevels calculates exit levels for a given entry price and symbol
func (esc *ExitStrategyCalculator) GetExitLevels(symbol string, entryPrice float64) *ExitLevels {
	levels := &ExitLevels{
		CalculatedAt: time.Now(),
	}

	// Calculate ATR
	atr, err := esc.CalculateATR(symbol)
	if err != nil || atr <= 0 {
		log.Printf("⚠️ ATR calculation failed for %s, using fallback levels", symbol)
		// Use fallback fixed percentages
		levels.ATR = 0
		levels.ATRPercent = 0
		levels.InitialStopPct = FallbackStopLossPct
		levels.TrailingStopPct = FallbackStopLossPct * 0.7
		levels.TakeProfit1Pct = FallbackTakeProfit1Pct
		levels.TakeProfit2Pct = FallbackTakeProfit2Pct
	} else {
		// Calculate ATR as percentage of price
		atrPct := (atr / entryPrice) * 100
		levels.ATR = atr
		levels.ATRPercent = atrPct

		// Apply multipliers from config
		levels.InitialStopPct = atrPct * esc.cfg.Trading.StopLossATRMultiplier
		levels.TrailingStopPct = atrPct * esc.cfg.Trading.TrailingStopATRMultiplier
		levels.TakeProfit1Pct = atrPct * esc.cfg.Trading.TakeProfit1ATRMultiplier
		levels.TakeProfit2Pct = atrPct * esc.cfg.Trading.TakeProfit2ATRMultiplier

		// Apply reasonable boundaries
		levels.InitialStopPct = clamp(levels.InitialStopPct, 0.5, 5.0)   // 0.5% - 5% max
		levels.TrailingStopPct = clamp(levels.TrailingStopPct, 0.5, 4.0) // 0.5% - 4% max
		levels.TakeProfit1Pct = clamp(levels.TakeProfit1Pct, 1.5, 12.0)  // 1.5% - 12% max
		levels.TakeProfit2Pct = clamp(levels.TakeProfit2Pct, 3.0, 20.0)  // 3% - 20% max
	}

	// Calculate absolute price levels
	levels.StopLossPrice = entryPrice * (1 - levels.InitialStopPct/100)
	levels.TakeProfit1Price = entryPrice * (1 + levels.TakeProfit1Pct/100)
	levels.TakeProfit2Price = entryPrice * (1 + levels.TakeProfit2Pct/100)

	log.Printf("📊 Exit levels for %s @ %.0f: SL=%.1f%% (%.0f), TP1=%.1f%% (%.0f), TP2=%.1f%% (%.0f), ATR=%.2f",
		symbol, entryPrice,
		levels.InitialStopPct, levels.StopLossPrice,
		levels.TakeProfit1Pct, levels.TakeProfit1Price,
		levels.TakeProfit2Pct, levels.TakeProfit2Price,
		levels.ATR)

	return levels
}

// CalculateTrailingStop calculates the new trailing stop level
// Returns the new stop level (as a price)
func (esc *ExitStrategyCalculator) CalculateTrailingStop(
	entryPrice float64,
	currentPrice float64,
	currentStopPrice float64,
	trailingStopPct float64,
) float64 {
	// Calculate new trailing stop based on current price
	newStopPrice := currentPrice * (1 - trailingStopPct/100)

	// Only move stop up, never down (for long positions)
	if newStopPrice > currentStopPrice {
		return newStopPrice
	}

	return currentStopPrice
}

// ShouldExitPosition determines if position should be exited and why
func (esc *ExitStrategyCalculator) ShouldExitPosition(
	entryPrice float64,
	currentPrice float64,
	levels *ExitLevels,
	currentTrailingStop float64,
	profitLossPct float64,
	holdingMinutes int,
) (shouldExit bool, reason string, newTrailingStop float64) {
	// Update trailing stop first
	if profitLossPct > 0 {
		newTrailingStop = esc.CalculateTrailingStop(
			entryPrice,
			currentPrice,
			currentTrailingStop,
			levels.TrailingStopPct,
		)
	} else {
		newTrailingStop = currentTrailingStop
	}

	// 1. Check initial stop loss (hard stop)
	if profitLossPct <= -levels.InitialStopPct {
		return true, "ATR_STOP_LOSS", newTrailingStop
	}

	// 2. Check trailing stop hit
	if newTrailingStop > 0 && currentPrice <= newTrailingStop {
		return true, "TRAILING_STOP_HIT", newTrailingStop
	}

	// 3. Check Take Profit 2 (full exit)
	if profitLossPct >= levels.TakeProfit2Pct {
		return true, "TAKE_PROFIT_FULL", newTrailingStop
	}

	// 4. Check Take Profit 1 with time consideration
	// If we hit TP1 and have been holding for > 60 mins, consider exit
	if profitLossPct >= levels.TakeProfit1Pct && holdingMinutes > 60 {
		return true, "TAKE_PROFIT_TIME_BASED", newTrailingStop
	}

	// 5. Maximum holding period (4 hours) with any profit
	if holdingMinutes >= 240 && profitLossPct > 0.2 {
		return true, "MAX_HOLDING_PROFIT", newTrailingStop
	}

	return false, "", newTrailingStop
}

// Helper function to clamp value between min and max
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// Helper function to safely extract float from map
func getFloat(m map[string]interface{}, key string) float64 {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case float32:
			return float64(val)
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0
}

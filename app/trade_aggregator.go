package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"stockbit-haka-haki/database"
	"stockbit-haka-haki/llm"
)

// TradeAggregator handles the aggregation of running trade data into statistical snapshots
type TradeAggregator struct {
	repo *database.TradeRepository
	llm  *llm.Client
}

// NewTradeAggregator creates a new trade aggregator
func NewTradeAggregator(repo *database.TradeRepository, llmClient *llm.Client) *TradeAggregator {
	return &TradeAggregator{
		repo: repo,
		llm:  llmClient,
	}
}

// AggregatedTradeData represents the statistical snapshot of trade data
type AggregatedTradeData struct {
	StockSymbol     string
	TimeWindow      time.Duration
	NetBuySell      float64 // Positive = net buy, Negative = net sell
	TradeFrequency  float64 // Trades per second
	BigOrderCount   int     // Orders > 100M IDR
	PriceVelocity   float64 // Price change percentage per minute
	TotalVolumeLots float64
	TotalValue      float64
	BuyVolumeLots   float64
	SellVolumeLots  float64
	AvgPrice        float64
	CurrentPrice    float64
	Timestamp       time.Time
}

// AggregateRunningTrades aggregates raw trade data into statistical metrics
func (ta *TradeAggregator) AggregateRunningTrades(ctx context.Context, stockSymbol string, window time.Duration) (*AggregatedTradeData, error) {
	endTime := time.Now()
	startTime := endTime.Add(-window)

	// Get trades in the specified window using existing repository method
	trades, err := ta.repo.GetTradesByTimeRange(stockSymbol, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}

	if len(trades) == 0 {
		return nil, fmt.Errorf("no trades found for %s in last %v", stockSymbol, window)
	}

	// Initialize aggregation variables
	var totalVolumeLots, totalValue, buyVolumeLots, sellVolumeLots, totalPrice float64
	var bigOrderCount int
	tradeCount := len(trades)

	// Find price range for velocity calculation
	minPrice := trades[0].Price
	maxPrice := trades[0].Price
	currentPrice := trades[len(trades)-1].Price

	// Aggregate trade data
	for _, trade := range trades {
		totalVolumeLots += trade.VolumeLot
		totalValue += trade.TotalAmount
		totalPrice += trade.Price

		if trade.Price < minPrice {
			minPrice = trade.Price
		}
		if trade.Price > maxPrice {
			maxPrice = trade.Price
		}

		switch trade.Action {
		case "BUY":
			buyVolumeLots += trade.VolumeLot
		case "SELL":
			sellVolumeLots += trade.VolumeLot
		}

		// Count big orders (> 100M IDR)
		if trade.TotalAmount > 100_000_000 {
			bigOrderCount++
		}
	}

	// Calculate metrics
	netBuySell := buyVolumeLots - sellVolumeLots
	avgPrice := totalPrice / float64(tradeCount)

	// Calculate trade frequency (trades per second)
	windowSeconds := window.Seconds()
	tradeFrequency := float64(tradeCount) / windowSeconds

	// Calculate price velocity (max price change percentage per minute)
	priceVelocity := 0.0
	priceRange := maxPrice - minPrice
	if minPrice > 0 {
		priceChangePct := (priceRange / minPrice) * 100
		// Normalize to per minute basis
		windowMinutes := window.Minutes()
		if windowMinutes > 0 {
			priceVelocity = priceChangePct / windowMinutes
		}
	}

	aggregatedData := &AggregatedTradeData{
		StockSymbol:     stockSymbol,
		TimeWindow:      window,
		NetBuySell:      netBuySell,
		TradeFrequency:  tradeFrequency,
		BigOrderCount:   bigOrderCount,
		PriceVelocity:   priceVelocity,
		TotalVolumeLots: totalVolumeLots,
		TotalValue:      totalValue,
		BuyVolumeLots:   buyVolumeLots,
		SellVolumeLots:  sellVolumeLots,
		AvgPrice:        avgPrice,
		CurrentPrice:    currentPrice,
		Timestamp:       endTime,
	}

	return aggregatedData, nil
}

// GenerateLLMAnalysis generates LLM analysis based on aggregated trade data
func (ta *TradeAggregator) GenerateLLMAnalysis(ctx context.Context, data *AggregatedTradeData) (string, error) {
	// Create prompt for tape reading and market psychology analysis
	prompt := fmt.Sprintf(`
Analyze the following aggregated trade data for stock %s:

TIME WINDOW: Last %.0f minutes
CURRENT PRICE: %.2f
AVERAGE PRICE: %.2f
NET BUY/SELL: %.2f lots (positive = net buy, negative = net sell)
TRADE FREQUENCY: %.2f trades per second
BIG ORDER COUNT: %d orders > 100M IDR
PRICE VELOCITY: %.2f%% price change per minute
TOTAL VOLUME: %.2f lots
BUY VOLUME: %.2f lots
SELL VOLUME: %.2f lots

Based on this data, provide analysis on:
1. Tape reading and market psychology (whale analysis)
2. Potential hidden distribution or accumulation
3. Anomaly detection and urgency assessment
4. Trading signal recommendation with confidence level

Respond with concise, professional analysis suitable for institutional traders.
`, data.StockSymbol, data.TimeWindow.Minutes(), data.CurrentPrice, data.AvgPrice,
		data.NetBuySell, data.TradeFrequency, data.BigOrderCount, data.PriceVelocity,
		data.TotalVolumeLots, data.BuyVolumeLots, data.SellVolumeLots)

	analysis, err := ta.llm.Analyze(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate LLM analysis: %w", err)
	}

	return analysis, nil
}

// GenerateTradingSignal creates a structured trading signal from LLM analysis
func (ta *TradeAggregator) GenerateTradingSignal(ctx context.Context, stockSymbol string, window time.Duration) (*database.TradingSignalDB, error) {
	// Aggregate trade data
	aggregatedData, err := ta.AggregateRunningTrades(ctx, stockSymbol, window)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate trades: %w", err)
	}

	// Generate LLM analysis
	analysis, err := ta.GenerateLLMAnalysis(ctx, aggregatedData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate LLM analysis: %w", err)
	}

	// Determine decision based on analysis keywords
	decision := "WAIT"
	confidence := 0.5
	var reason string

	// Simple keyword-based decision logic (can be enhanced with more sophisticated parsing)
	lowerAnalysis := strings.ToLower(analysis)
	if contains(lowerAnalysis, []string{"bearish", "sell", "distribusi", "distribution", "fakeout", "reversal"}) {
		decision = "SELL"
		confidence = 0.7
		reason = "LLM detected bearish signals or distribution"
	} else if contains(lowerAnalysis, []string{"bullish", "buy", "akumulasi", "accumulation", "breakout", "momentum"}) {
		decision = "BUY"
		confidence = 0.7
		reason = "LLM detected bullish signals or accumulation"
	} else {
		decision = "WAIT"
		confidence = 0.3
		reason = "LLM analysis inconclusive"
	}

	// Calculate Z-scores for price and volume (simplified)
	priceZScore := (aggregatedData.CurrentPrice - aggregatedData.AvgPrice) / (aggregatedData.AvgPrice * 0.01) // Rough estimate
	volumeZScore := (aggregatedData.TotalVolumeLots - 1000) / 500                                             // Simplified baseline

	signal := &database.TradingSignalDB{
		GeneratedAt:       time.Now(),
		StockSymbol:       stockSymbol,
		Strategy:          "LLM_TAPE_READING",
		Decision:          decision,
		Confidence:        confidence,
		TriggerPrice:      aggregatedData.CurrentPrice,
		TriggerVolumeLots: aggregatedData.TotalVolumeLots,
		PriceZScore:       priceZScore,
		VolumeZScore:      volumeZScore,
		PriceChangePct:    ((aggregatedData.CurrentPrice - aggregatedData.AvgPrice) / aggregatedData.AvgPrice) * 100,
		Reason:            reason,
		AnalysisData:      analysis,
	}

	return signal, nil
}

// Helper function to check if string contains any of the substrings
func contains(s string, substrs []string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

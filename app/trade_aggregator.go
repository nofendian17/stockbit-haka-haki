package app

import (
	"context"
	"encoding/json"
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
You are a Senior Investment Manager at a top-tier quantitative hedge fund.
Your task is to analyze the following aggregated trade data for stock %s and make a high-conviction trading decision.

MARKET METRICS:
- Time Window: %.0f minutes
- Current Price: %.2f
- VWAP: %.2f
- Net Order Flow: %.2f lots (Positive=Accumulation, Negative=Distribution)
- Trade Frequency: %.2f trades/sec
- Whale Activity: %d large block orders (>100M IDR)
- Price Velocity: %.2f%% per minute
- Total Volume: %.2f lots
- Buying Pressure: %.2f lots
- Selling Pressure: %.2f lots

ANALYSIS REQUIRED:
1. MARKET STRUCTURE: Is the stock in accumulation, distribution, or equilibrium?
2. WHALE FOOTPRINT: Are institutional players active?
3. RISKS: Volatility and sustained potential.
4. ACTIONABLE VERDICT: What is the immediate move?

You must conclude with this EXACT format:
ANALYSIS SUMMARY: [One sentence summary]
FINAL_DECISION: [BUY | SELL | WAIT]
CONFIDENCE_SCORE: [0.0 - 1.0]

Respond with professional, data-driven insights.
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

	// Determine decision based on strict parsing of LLM output
	decision := "WAIT"
	confidence := 0.5
	reason := "LLM analysis inconclusive"

	lines := strings.Split(analysis, "\n")
	foundDecision := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "FINAL_DECISION:") {
			d := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(line, "FINAL_DECISION:")))
			if d == "BUY" {
				decision = "BUY"
				foundDecision = true
			} else if d == "SELL" {
				decision = "SELL"
				foundDecision = true
			} else if d == "WAIT" {
				decision = "WAIT"
				foundDecision = true
			}
		}
		if strings.HasPrefix(line, "CONFIDENCE_SCORE:") {
			fmt.Sscanf(strings.TrimPrefix(line, "CONFIDENCE_SCORE:"), "%f", &confidence)
		}
		if strings.HasPrefix(line, "ANALYSIS SUMMARY:") {
			summary := strings.TrimPrefix(line, "ANALYSIS SUMMARY:")
			reason = strings.TrimSpace(summary)
		}
	}

	// Fallback logic if parsing failed (Robustness check)
	if !foundDecision {
		lowerAnalysis := strings.ToLower(analysis)
		// Count directional keywords instead of just simple existence
		buyScore := 0
		sellScore := 0
		buyScore += strings.Count(lowerAnalysis, "accumulation") * 2
		buyScore += strings.Count(lowerAnalysis, "bullish")
		buyScore += strings.Count(lowerAnalysis, "buy")

		sellScore += strings.Count(lowerAnalysis, "distribution") * 2
		sellScore += strings.Count(lowerAnalysis, "bearish")
		sellScore += strings.Count(lowerAnalysis, "sell")

		if buyScore > sellScore && buyScore >= 2 {
			decision = "BUY"
			reason = "LLM analysis indicates buying pressure (keyword fallback)"
		} else if sellScore > buyScore && sellScore >= 2 {
			decision = "SELL"
			reason = "LLM analysis indicates selling pressure (keyword fallback)"
		}
	}

	// Normalize checks
	if reason == "" {
		reason = "Automated signal based on flow analysis"
	}
	if len(reason) > 255 {
		reason = reason[:252] + "..."
	}

	// Calculate Z-scores for price and volume (simplified)
	priceZScore := 0.0
	if aggregatedData.AvgPrice != 0 {
		priceZScore = (aggregatedData.CurrentPrice - aggregatedData.AvgPrice) / (aggregatedData.AvgPrice * 0.01)
	}
	volumeZScore := (aggregatedData.TotalVolumeLots - 1000) / 500

	// Wrap analysis in JSON to satisfy database requirement
	analysisJSON, _ := json.Marshal(map[string]string{
		"analysis": analysis,
	})

	priceChangePct := 0.0
	if aggregatedData.AvgPrice != 0 {
		priceChangePct = ((aggregatedData.CurrentPrice - aggregatedData.AvgPrice) / aggregatedData.AvgPrice) * 100
	}

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
		PriceChangePct:    priceChangePct,
		Reason:            reason,
		AnalysisData:      string(analysisJSON),
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

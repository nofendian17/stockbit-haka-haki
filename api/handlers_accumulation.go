package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// handleAccumulationSummary returns separate top 20 accumulation and distribution lists
// Smart timeframe selection:
// - Before 08:30 WIB: show last 24 hours (previous day's data)
// - 08:30 WIB onwards: show data since market open (09:00 WIB)
func (s *Server) handleAccumulationSummary(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()

	var startTime time.Time
	var hoursBack float64
	var timeframeDescription string

	// Use market hours constants (mirrored from app/signal_tracker.go)
	loc, err := time.LoadLocation(marketTimeZone)
	if err != nil {
		loc = time.FixedZone("WIB", 7*60*60)
	}
	now := time.Now().In(loc)

	// Market open time today (09:00 WIB)
	marketOpen := time.Date(now.Year(), now.Month(), now.Day(), marketOpenHour, 0, 0, 0, loc)

	// Pre-market time threshold (08:30 WIB)
	preMarketThreshold := time.Date(now.Year(), now.Month(), now.Day(), 8, 30, 0, 0, loc)

	if h := query.Get("hours"); h != "" {
		// Manual override via query parameter
		if parsed, err := strconv.Atoi(h); err == nil {
			hoursBack = float64(parsed)
			startTime = now.Add(-time.Duration(parsed) * time.Hour)
			timeframeDescription = fmt.Sprintf("Last %d hours (manual)", parsed)
		} else {
			// Fallback if parsing fails - use smart logic
			startTime, hoursBack, timeframeDescription = getSmartTimeframe(now, marketOpen, preMarketThreshold)
		}
	} else {
		// Smart timeframe selection based on current time
		startTime, hoursBack, timeframeDescription = getSmartTimeframe(now, marketOpen, preMarketThreshold)
	}

	// Log for debugging time range issues
	log.Printf("[handleAccumulationSummary] now=%s, startTime=%s, hoursBack=%.2f, description=%s",
		now.Format("2006-01-02 15:04:05"), startTime.Format("2006-01-02 15:04:05"),
		hoursBack, timeframeDescription)

	// Get accumulation/distribution summary (now returns 2 separate lists)
	accumulation, distribution, err := s.repo.GetAccumulationDistributionSummary(startTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accumulation":       accumulation,
		"distribution":       distribution,
		"accumulation_count": len(accumulation),
		"distribution_count": len(distribution),
		"hours_back":         hoursBack,
		"timeframe":          timeframeDescription,
		"current_time":       now.Format("2006-01-02 15:04:05"),
		"market_status":      getMarketStatus(now),
	})
}

// getSmartTimeframe determines the appropriate timeframe based on current time
// - Before 08:30 WIB: show last 24 hours (previous day's data)
// - 08:30 WIB onwards: show data since market open (09:00 WIB)
func getSmartTimeframe(now, marketOpen, preMarketThreshold time.Time) (startTime time.Time, hoursBack float64, description string) {
	if now.Before(preMarketThreshold) {
		// Before 08:30 WIB: Show last 24 hours (previous day's data)
		startTime = now.Add(-24 * time.Hour)
		hoursBack = 24.0
		description = "Last 24 hours (pre-market view)"
	} else if now.Before(marketOpen) {
		// Between 08:30 and 09:00 WIB: Show since 08:30 (pre-market activity)
		startTime = preMarketThreshold
		hoursBack = now.Sub(preMarketThreshold).Hours()
		description = fmt.Sprintf("Pre-market activity (since 08:30 WIB, %.1f hours)", hoursBack)
	} else {
		// Market is open (09:00 onwards): Show since market open
		startTime = marketOpen
		hoursBack = now.Sub(marketOpen).Hours()
		description = fmt.Sprintf("Today's session (since 09:00 WIB, %.1f hours)", hoursBack)
	}
	return startTime, hoursBack, description
}

// getMarketStatus returns a string describing current market status
func getMarketStatus(now time.Time) string {
	hour := now.Hour()
	minute := now.Minute()

	if hour == 8 && minute >= 30 {
		return "PRE_MARKET"
	} else if hour >= 9 && hour < 12 {
		return "SESSION_1"
	} else if (hour == 12) || (hour == 13 && minute < 30) {
		return "LUNCH_BREAK"
	} else if (hour == 13 && minute >= 30) || (hour == 14 && minute < 50) {
		return "SESSION_2"
	} else if hour == 14 && minute >= 50 {
		return "PRE_CLOSING"
	} else if hour >= 15 && hour < 16 {
		return "POST_MARKET"
	} else {
		return "CLOSED"
	}
}

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

// handleAccumulationSummary returns separate top 20 accumulation and distribution lists
// Uses market open time (09:00 WIB) as default for more accurate trading hours analysis
func (s *Server) handleAccumulationSummary(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	query := r.URL.Query()

	var startTime time.Time
	var hoursBack float64

	// Use market hours constants (mirrored from app/signal_tracker.go)
	loc, err := time.LoadLocation(marketTimeZone)
	if err != nil {
		loc = time.FixedZone("WIB", 7*60*60)
	}
	now := time.Now().In(loc)
	marketOpen := time.Date(now.Year(), now.Month(), now.Day(), marketOpenHour, 0, 0, 0, loc) // 09:00 WIB

	if h := query.Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil {
			hoursBack = float64(parsed)
			startTime = time.Now().Add(-time.Duration(parsed) * time.Hour)
		} else {
			// Fallback if parsing fails - use market open time
			startTime = marketOpen
			hoursBack = time.Since(startTime).Hours()
		}
	} else {
		// Default: use market open time (09:00 WIB) instead of midnight
		startTime = marketOpen
		hoursBack = time.Since(startTime).Hours()
	}

	// Log for debugging time range issues
	log.Printf("[handleAccumulationSummary] now=%s, startTime=%s, hoursBack=%.2f, marketOpenHour=%d",
		now.Format("2006-01-02 15:04:05"), startTime.Format("2006-01-02 15:04:05"),
		hoursBack, marketOpenHour)

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
	})
}

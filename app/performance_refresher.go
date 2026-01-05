package app

import (
	"log"
	"time"

	"stockbit-haka-haki/database"
)

// PerformanceRefresher periodically refreshes the performance materialized view
type PerformanceRefresher struct {
	repo *database.TradeRepository
	done chan bool
}

// NewPerformanceRefresher creates a new performance refresher
func NewPerformanceRefresher(repo *database.TradeRepository) *PerformanceRefresher {
	return &PerformanceRefresher{
		repo: repo,
		done: make(chan bool),
	}
}

// Start begins the refresh loop
func (pr *PerformanceRefresher) Start() {
	log.Println("🔄 Performance Refresher started")

	// Run every 5 minutes to keep performance data fresh
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Initial run
	pr.refreshView()

	for {
		select {
		case <-ticker.C:
			pr.refreshView()
		case <-pr.done:
			log.Println("🔄 Performance Refresher stopped")
			return
		}
	}
}

// Stop stops the refresh loop
func (pr *PerformanceRefresher) Stop() {
	pr.done <- true
}

// refreshView refreshes the materialized view
func (pr *PerformanceRefresher) refreshView() {
	log.Println("🔄 Refreshing strategy_performance_daily materialized view...")

	// Use CONCURRENTLY to avoid blocking reads
	_, err := pr.repo.GetDailyStrategyPerformance("", "", 1)
	if err != nil {
		log.Printf("⚠️ Failed to refresh performance view: %v", err)
		return
	}

	log.Println("✅ Performance view refreshed successfully")
}

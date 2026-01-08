package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// handleMLDataStats returns statistics about ML training data availability
func (s *Server) handleMLDataStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.repo.GetMLTrainingDataStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleExportMLData returns a CSV of training data
func (s *Server) handleExportMLData(w http.ResponseWriter, r *http.Request) {
	data, err := s.repo.GetMLTrainingData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment;filename=training_data_%d.csv", time.Now().Unix()))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header
	writer.Write([]string{"generated_at", "symbol", "strategy", "confidence", "outcome", "profit_pct", "feature_vector"})

	// Rows
	for _, row := range data {
		writer.Write([]string{
			row.GeneratedAt.Format(time.RFC3339),
			row.StockSymbol,
			row.Strategy,
			fmt.Sprintf("%.2f", row.Confidence),
			row.OutcomeResult,
			fmt.Sprintf("%.2f", row.ProfitLossPct),
			row.AnalysisData,
		})
	}
}

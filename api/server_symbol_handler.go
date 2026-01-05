package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"stockbit-haka-haki/database"
	"stockbit-haka-haki/llm"
)

func (s *Server) handleSymbolAnalysisStream(w http.ResponseWriter, r *http.Request) {
	// Check if LLM is enabled
	if !s.llmEnabled || s.llmClient == nil {
		http.Error(w, "LLM is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Get symbol from query param
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "symbol parameter is required", http.StatusBadRequest)
		return
	}

	// Get limit (default 20)
	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	// Get recent alerts for symbol
	alerts, err := s.repo.GetRecentAlertsBySymbol(symbol, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(alerts) == 0 {
		http.Error(w, "No whale alerts found for this symbol", http.StatusNotFound)
		return
	}

	// Fetch enriched metadata for context
	regime, _ := s.repo.GetLatestRegime(symbol)
	baseline, _ := s.repo.GetLatestBaseline(symbol)
	orderFlow, _ := s.repo.GetLatestOrderFlow(symbol)

	// Fetch followups for the specific alerts retrieved
	var followups []database.WhaleAlertFollowup
	for _, a := range alerts {
		if f, err := s.repo.GetWhaleFollowup(a.ID); err == nil && f != nil {
			followups = append(followups, *f)
		}
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Generate prompt with enriched data
	prompt := llm.FormatSymbolAnalysisPrompt(symbol, alerts, regime, baseline, orderFlow, followups)

	// Stream LLM response
	err = s.llmClient.AnalyzeStream(r.Context(), prompt, func(chunk string) error {
		// Properly format multi-line chunks for SSE
		lines := strings.Split(chunk, "\n")
		for i, line := range lines {
			if i < len(lines)-1 {
				fmt.Fprintf(w, "data: %s\n", line)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
		}
		flusher.Flush()
		return nil
	})

	if err != nil {
		log.Printf("LLM streaming failed: %v", err)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send completion event
	fmt.Fprintf(w, "event: done\ndata: Stream completed\n\n")
	flusher.Flush()
}

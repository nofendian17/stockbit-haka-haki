package api

import (
	"log"
	"net/http"
	"strconv"
)

// Market hours constants
const (
	marketOpenHour  = 9  // 09:00 WIB - market open
	marketCloseHour = 16 // 16:00 WIB - market close
	marketTimeZone  = "Asia/Jakarta"
	millionDivisor  = 1_000_000
)

// setupSSE configures the response writer for Server-Sent Events streaming
// Returns the Flusher if supported, or an error if not
func setupSSE(w http.ResponseWriter) (http.Flusher, bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	return flusher, true
}

// getIntParam retrieves an integer query parameter with default value and optional range validation
func getIntParam(r *http.Request, key string, defaultVal int, minVal, maxVal *int) int {
	valStr := r.URL.Query().Get(key)
	if valStr == "" {
		return defaultVal
	}

	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}

	if minVal != nil && val < *minVal {
		return defaultVal
	}
	if maxVal != nil && val > *maxVal {
		return defaultVal
	}

	return val
}

// getFloatParam retrieves a float query parameter with default value
func getFloatParam(r *http.Request, key string, defaultVal float64) float64 {
	valStr := r.URL.Query().Get(key)
	if valStr == "" {
		return defaultVal
	}

	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return defaultVal
	}

	return val
}

// respondWithError logs the error and sends a JSON error response
// Use this to avoid exposing internal errors while still logging them
func respondWithError(w http.ResponseWriter, code int, message string, err error) {
	if err != nil {
		log.Printf("API Error [%d]: %s - %v", code, message, err)
	} else {
		log.Printf("API Error [%d]: %s", code, message)
	}
	http.Error(w, message, code)
}

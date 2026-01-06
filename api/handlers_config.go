package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"stockbit-haka-haki/database"
)

// handleHealth returns the health status of the API
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Configuration Handlers (Webhooks Only)

func (s *Server) handleGetWebhooks(w http.ResponseWriter, r *http.Request) {
	webhooks, err := s.repo.GetWebhooks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhooks)
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var webhook database.WhaleWebhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Reset ID to let DB assign it
	webhook.ID = 0

	if err := s.repo.SaveWebhook(&webhook); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh webhook manager cache
	if s.webhookMq != nil {
		s.webhookMq.RefreshCache()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(webhook)
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var webhook database.WhaleWebhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	webhook.ID = id // Ensure ID matches path
	if err := s.repo.SaveWebhook(&webhook); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh webhook manager cache
	if s.webhookMq != nil {
		s.webhookMq.RefreshCache()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhook)
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.repo.DeleteWebhook(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh webhook manager cache
	if s.webhookMq != nil {
		s.webhookMq.RefreshCache()
	}

	w.WriteHeader(http.StatusNoContent)
}

package realtime

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// Broker handles Server-Sent Events (SSE) clients and broadcasting
type Broker struct {
	clients    map[chan []byte]bool
	register   chan chan []byte
	unregister chan chan []byte
	broadcast  chan []byte
	mu         sync.RWMutex
}

// NewBroker creates a new SSE broker
func NewBroker() *Broker {
	return &Broker{
		clients:    make(map[chan []byte]bool),
		register:   make(chan chan []byte),
		unregister: make(chan chan []byte),
		broadcast:  make(chan []byte, 1000), // Buffer broadcast (Limit increased to 1000)
	}
}

// Run starts the broker loop
func (b *Broker) Run() {
	for {
		select {
		case client := <-b.register:
			b.mu.Lock()
			b.clients[client] = true
			b.mu.Unlock()
			log.Printf("SSE Client connected. Total: %d", len(b.clients))

		case client := <-b.unregister:
			b.mu.Lock()
			if _, ok := b.clients[client]; ok {
				delete(b.clients, client)
				close(client)
				log.Printf("SSE Client disconnected. Total: %d", len(b.clients))
			}
			b.mu.Unlock()

		case msg := <-b.broadcast:
			b.mu.RLock()
			for client := range b.clients {
				select {
				case client <- msg:
				default:
					// Skip if client buffer is full to prevent blocking
				}
			}
			b.mu.RUnlock()
		}
	}
}

// ServeHTTP handles the SSE endpoint
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	clientChan := make(chan []byte, 10)
	b.register <- clientChan

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			b.unregister <- clientChan
			return
		case msg := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			w.(http.Flusher).Flush()
		}
	}
}

// Broadcast sends a message to all connected clients
func (b *Broker) Broadcast(event string, payload interface{}) {
	data := map[string]interface{}{
		"event":   event,
		"payload": payload,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshalling broadcast message: %v", err)
		return
	}

	select {
	case b.broadcast <- jsonBytes:
	default:
		// Drop if broadcast buffer full
	}
}

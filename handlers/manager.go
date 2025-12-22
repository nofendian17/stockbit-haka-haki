package handlers

import (
	"fmt"
	"sync"
)

// HandlerManager mengelola multiple message handlers
type HandlerManager struct {
	handlers map[string]MessageHandler
	mu       sync.RWMutex
}

// NewHandlerManager membuat instance HandlerManager baru
func NewHandlerManager() *HandlerManager {
	return &HandlerManager{
		handlers: make(map[string]MessageHandler),
	}
}

// RegisterHandler mendaftarkan handler dengan nama tertentu
func (hm *HandlerManager) RegisterHandler(name string, handler MessageHandler) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.handlers[name] = handler
	fmt.Printf("📦 Registered handler: %s (type: %s)\n", name, handler.GetMessageType())
}

// UnregisterHandler menghapus handler dengan nama tertentu
func (hm *HandlerManager) UnregisterHandler(name string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	delete(hm.handlers, name)
}

// GetHandler mendapatkan handler berdasarkan nama
func (hm *HandlerManager) GetHandler(name string) (MessageHandler, bool) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	handler, exists := hm.handlers[name]
	return handler, exists
}

// HandleMessage memproses message menggunakan handler yang sesuai
func (hm *HandlerManager) HandleMessage(handlerName string, data []byte) error {
	handler, exists := hm.GetHandler(handlerName)
	if !exists {
		return fmt.Errorf("handler '%s' not found", handlerName)
	}

	return handler.Handle(data)
}

// HandleProtoMessage processes protobuf wrapper messages and routes to appropriate handler
func (hm *HandlerManager) HandleProtoMessage(handlerName string, wrapper interface{}) error {
	handler, exists := hm.GetHandler(handlerName)
	if !exists {
		return fmt.Errorf("handler '%s' not found", handlerName)
	}

	// Check if handler supports protobuf wrapper messages
	if protoHandler, ok := handler.(ProtoMessageHandler); ok {
		return protoHandler.HandleProto(wrapper)
	}

	return fmt.Errorf("handler '%s' does not support protobuf messages", handlerName)
}

// ListHandlers mengembalikan daftar nama handler yang terdaftar
func (hm *HandlerManager) ListHandlers() []string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	names := make([]string, 0, len(hm.handlers))
	for name := range hm.handlers {
		names = append(names, name)
	}
	return names
}

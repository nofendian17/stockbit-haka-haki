package cache

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"time"

	"stockbit-haka-haki/database"
)

// LLMCache provides caching functionality for LLM analysis results
type LLMCache struct {
	redis *RedisClient
}

// NewLLMCache creates a new LLM cache instance
func NewLLMCache(redis *RedisClient) *LLMCache {
	return &LLMCache{
		redis: redis,
	}
}

// GetAnalysis retrieves cached LLM analysis for a symbol
// Returns the cached signal and true if found, nil and false otherwise
func (c *LLMCache) GetAnalysis(ctx context.Context, symbol string, dataHash string) (*database.TradingSignalDB, bool) {
	if c.redis == nil {
		return nil, false
	}

	cacheKey := fmt.Sprintf("llm:analysis:%s:%s", symbol, dataHash)
	var signal database.TradingSignalDB

	if err := c.redis.Get(ctx, cacheKey, &signal); err != nil {
		return nil, false
	}

	return &signal, true
}

// SetAnalysis caches LLM analysis result for a symbol
func (c *LLMCache) SetAnalysis(ctx context.Context, symbol string, dataHash string, signal *database.TradingSignalDB, ttl time.Duration) error {
	if c.redis == nil {
		return fmt.Errorf("redis client not available")
	}

	cacheKey := fmt.Sprintf("llm:analysis:%s:%s", symbol, dataHash)
	return c.redis.Set(ctx, cacheKey, signal, ttl)
}

// SetCooldown sets a cooldown period for a symbol to prevent excessive LLM calls
func (c *LLMCache) SetCooldown(ctx context.Context, symbol string, ttl time.Duration) error {
	if c.redis == nil {
		return fmt.Errorf("redis client not available")
	}

	cooldownKey := fmt.Sprintf("llm:cooldown:%s", symbol)
	return c.redis.Set(ctx, cooldownKey, time.Now().Unix(), ttl)
}

// IsInCooldown checks if a symbol is in cooldown period
func (c *LLMCache) IsInCooldown(ctx context.Context, symbol string) bool {
	if c.redis == nil {
		return false
	}

	cooldownKey := fmt.Sprintf("llm:cooldown:%s", symbol)
	var timestamp int64

	if err := c.redis.Get(ctx, cooldownKey, &timestamp); err != nil {
		return false
	}

	return timestamp > 0
}

// GenerateDataHash creates a hash from trade data to detect if market conditions changed
func GenerateDataHash(data interface{}) string {
	jsonData, _ := json.Marshal(data)
	hash := md5.Sum(jsonData)
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes for shorter hash
}

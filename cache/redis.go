package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps redis.Client
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client
func NewRedisClient(host, port, password string) *RedisClient {
	addr := fmt.Sprintf("%s:%s", host, port)
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0, // use default DB
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("⚠️  Failed to connect to Redis at %s: %v", addr, err)
		return nil
	}

	log.Printf("✅ Connected to Redis at %s", addr)
	return &RedisClient{client: client}
}

// Set stores a value in Redis with expiration
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if r.client == nil {
		return fmt.Errorf("redis client not initialized")
	}

	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, jsonBytes, expiration).Err()
}

// Get retrieves a value from Redis
func (r *RedisClient) Get(ctx context.Context, key string, dest interface{}) error {
	if r.client == nil {
		return fmt.Errorf("redis client not initialized")
	}

	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(val), dest)
}

// Delete removes a key from Redis
func (r *RedisClient) Delete(ctx context.Context, key string) error {
	if r.client == nil {
		return fmt.Errorf("redis client not initialized")
	}
	return r.client.Del(ctx, key).Err()
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Publish sends a message to a channel
func (r *RedisClient) Publish(ctx context.Context, channel string, message interface{}) error {
	if r.client == nil {
		return fmt.Errorf("redis client not initialized")
	}

	jsonBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return r.client.Publish(ctx, channel, jsonBytes).Err()
}

// Subscribe subscribes to a channel
func (r *RedisClient) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	if r.client == nil {
		return nil
	}
	return r.client.Subscribe(ctx, channel)
}

// Exists checks if a key exists in Redis
func (r *RedisClient) Exists(ctx context.Context, key string) bool {
	if r.client == nil {
		return false
	}

	result, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false
	}

	return result > 0
}

// MGet retrieves multiple values from Redis
// Returns a slice where nil/zero values indicate key doesn't exist
func (r *RedisClient) MGet(ctx context.Context, keys []string, dest interface{}) error {
	if r.client == nil {
		return fmt.Errorf("redis client not initialized")
	}

	if len(keys) == 0 {
		return nil
	}

	// Convert []string to []interface{} for MGet
	keysInterface := make([]string, len(keys))
	copy(keysInterface, keys)

	results, err := r.client.MGet(ctx, keysInterface...).Result()
	if err != nil {
		return err
	}

	// Parse results based on destination type
	switch v := dest.(type) {
	case *[]int64:
		*v = make([]int64, len(results))
		for i, result := range results {
			if result == nil {
				(*v)[i] = 0 // Key doesn't exist
			} else if str, ok := result.(string); ok {
				var id int64
				if err := json.Unmarshal([]byte(str), &id); err == nil {
					(*v)[i] = id
				}
			}
		}
	case *[]string:
		*v = make([]string, len(results))
		for i, result := range results {
			if result != nil {
				if str, ok := result.(string); ok {
					(*v)[i] = str
				}
			}
		}
	default:
		return fmt.Errorf("unsupported destination type for MGet")
	}

	return nil
}

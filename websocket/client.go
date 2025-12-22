package websocket

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	pb "stockbit-haka-haki/proto"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Constants for subscription
const (
	allStocksWildcard = "*" // Subscribe to all stocks
)

// Client represents a WebSocket client
type Client struct {
	url        string
	conn       *websocket.Conn
	header     http.Header
	writeMu    sync.Mutex
	pingCancel context.CancelFunc // Cancel function for ping goroutine
}

// NewClient creates a new WebSocket client
func NewClient(url string, authToken string) *Client {
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+authToken)
	header.Set("User-Agent", "Mozilla/5.0")

	return &Client{
		url:    url,
		header: header,
	}
}

// Connect establishes WebSocket connection
func (c *Client) Connect() error {
	conn, _, err := websocket.DefaultDialer.Dial(c.url, c.header)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.url, err)
	}

	c.conn = conn
	log.Printf("✅ Connected to %s", c.url)
	return nil
}

// SubscribeToStocks sends subscription messages for all stocks (wildcard subscription)
func (c *Client) SubscribeToStocks(stocks []string, userID string, wsKey string) error {
	// Use wildcard to subscribe to ALL stocks
	allStocks := []string{allStocksWildcard}

	subReq := &pb.WebsocketRequest{
		UserId: userID,
		Channel: &pb.WebsocketChannel{
			RunningTradeBatch: allStocks, // Subscribe to ALL stocks
			Watchlist:         allStocks,
		},
		Key: wsKey,
	}

	data, err := proto.Marshal(subReq)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription: %w", err)
	}

	if err := c.WriteBinaryMessage(data); err != nil {
		return fmt.Errorf("failed to send subscription: %w", err)
	}

	log.Printf("📡 Subscribed to ALL stocks (wildcard subscription)")
	return nil
}

// StartPing starts periodic ping to keep connection alive
// Returns a context cancel function that can be used to stop the ping loop
func (c *Client) StartPing(interval time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	c.pingCancel = cancel

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Context canceled, exit goroutine
				return
			case <-ticker.C:
				pingMsg := &pb.WebsocketRequest{
					Ping: &pb.PingRequest{
						Timestamp: timestamppb.Now(),
					},
				}

				data, err := proto.Marshal(pingMsg)
				if err != nil {
					log.Println("Failed to marshal ping:", err)
					continue
				}

				if err := c.WriteBinaryMessage(data); err != nil {
					log.Println("Failed to send ping:", err)
					return
				}
			}
		}
	}()
}

// WriteBinaryMessage sends a binary message to the WebSocket connection thread-safely
func (c *Client) WriteBinaryMessage(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("connection is nil")
	}
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// ReadMessage reads and decodes a protobuf message from WebSocket
func (c *Client) ReadMessage() (*pb.WebsocketWrapMessageChannel, error) {
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	// Check first byte to identify message type
	if len(data) > 0 {
		firstByte := data[0]
		fieldNum := firstByte >> 3

		// Field 10 = Orderbook (has text body inside protobuf wrapper) - skip silently
		if fieldNum == 10 {
			return nil, fmt.Errorf("orderbook message with text body")
		}

		// Field 6 = OrderBookBody (pure protobuf) - accept
		// Field 1, 8, 9 = RunningTrade, RunningTradeBatch, LivePrice - accept
		// All other pure protobuf messages are accepted
	}

	// Decode the wrapper message
	wrapper := &pb.WebsocketWrapMessageChannel{}
	if err := proto.Unmarshal(data, wrapper); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return wrapper, nil
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	// Cancel ping goroutine if it's running
	if c.pingCancel != nil {
		c.pingCancel()
	}

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

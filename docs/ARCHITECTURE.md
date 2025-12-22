# 🏗️ Dokumentasi Arsitektur

## Gambaran Umum Sistem

Stockbit Analysis adalah sistem real-time untuk deteksi aktivitas "whale" (investor besar) di pasar saham Indonesia menggunakan data dari Stockbit. Sistem ini menggunakan analisis statistik murni dan AI untuk mengidentifikasi pola trading signifikan.

## Arsitektur High-Level

```
┌──────────────────────────────────────────────────────────────────┐
│                        Client Layer                               │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐     │
│  │  Web Browser   │  │  Webhook       │  │  API Clients   │     │
│  │  (Dashboard)   │  │  Endpoints     │  │  (External)    │     │
│  └────────┬───────┘  └────────┬───────┘  └────────┬───────┘     │
└───────────┼──────────────────┼──────────────────┼───────────────┘
            │                  │                  │
            │ HTTP/SSE         │ HTTP POST        │ REST API
            │                  │                  │
┌───────────▼──────────────────▼──────────────────▼───────────────┐
│                      Application Layer                           │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              Go Application (app.go)                    │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐             │    │
│  │  │   API    │  │ WebSocket│  │ Handlers │             │    │
│  │  │  Server  │  │  Client  │  │ Manager  │             │    │
│  │  └────┬─────┘  └────┬─────┘  └────┬─────┘             │    │
│  └───────┼─────────────┼─────────────┼────────────────────┘    │
│          │             │             │                          │
│  ┌───────▼─────┐ ┌─────▼──────┐ ┌───▼────────┐                │
│  │   Realtime  │ │    LLM     │ │ Webhook    │                │
│  │   Broker    │ │   Client   │ │  Manager   │                │
│  │   (SSE)     │ │  (OpenAI)  │ │ (Notifier) │                │
│  └─────────────┘ └────────────┘ └────────────┘                │
└──────────────────────────┬───────────────────────────────────────┘
                           │
┌──────────────────────────▼───────────────────────────────────────┐
│                     Data/Storage Layer                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ TimescaleDB  │  │    Redis     │  │   Stockbit   │          │
│  │ (PostgreSQL) │  │   (Cache)    │  │   WebSocket  │          │
│  │              │  │              │  │   (External) │          │
│  │ - trades     │  │ - stats      │  └──────────────┘          │
│  │ - candles    │  │ - session    │                            │
│  │ - alerts     │  │              │                            │
│  └──────────────┘  └──────────────┘                            │
└──────────────────────────────────────────────────────────────────┘
```

## Detail Komponen

### 1. Application Core (`app/`)

**File:** `app.go`

**Tanggung Jawab:**

- Application lifecycle management
- WebSocket connection initialization
- Handler registration and routing
- Graceful shutdown coordination
- Token management and authentication

**Fungsi Utama:**

- `New()`: Membuat instance aplikasi baru
- `Start()`: Bootstrap aplikasi dan start semua services
- `ensureAuthenticated()`: Manajemen autentikasi dan token caching
- `readAndProcessMessages()`: Main loop untuk membaca dan process WebSocket messages
- `reconnectWebSocket()`: Reconnection logic dengan exponential backoff
- `gracefulShutdown()`: Cleanup resources saat shutdown

**Pola Desain:**

- **Singleton**: Single app instance dengan shared dependencies
- **Dependency Injection**: Semua dependencies diinject melalui constructor
- **Observer Pattern**: Handler manager untuk message routing

### 2. WebSocket Client (`websocket/`)

**File:** `client.go`

**Tanggung Jawab:**

- Koneksi ke Stockbit WebSocket (wss://wss-trading.stockbit.com/ws)
- Mengirim subscription messages menggunakan Protocol Buffers
- Menerima dan decode binary protobuf messages
- Connection health monitoring

**Protocol:**

```
Message format: [4-byte length][protobuf payload]
- Length: Big-endian uint32
- Payload: Protobuf DataFeed message
```

**Subscription Flow:**

1. Connect ke WebSocket
2. Kirim subscription request untuk running trades
3. Terima DataFeed wrapper messages
4. Extract RunningTrade/OrderBookBody dari wrapper

**Key Features:**

- Binary protobuf encoding/decoding
- Automatic ping/pong untuk keep-alive
- Thread-safe read/write operations

### 3. Message Handlers (`handlers/`)

**Architecture:** Handler pattern dengan interface-based routing

**Files:**

- `base.go`: Base handler interface
- `manager.go`: Handler registration dan message routing
- `running_trade.go`: Trade processing dan whale detection

**Handler Interface:**

```go
type Handler interface {
    HandleProto(wrapper interface{}) error
    GetMessageType() string
}
```

**Running Trade Handler Flow:**

```
1. Terima RunningTrade protobuf message
2. Convert ke domain model (Trade)
3. Save ke database (TimescaleDB)
4. Hitung statistik (mean, stddev dari last 60 min)
5. Check whale criteria:
   - Z-Score >= 3.0 ATAU
   - Volume >= 5x average ATAU
   - (Fallback) Volume >= 1000 lots ATAU value >= 1B
6. Jika whale detected:
   - Save WhaleAlert ke database
   - Broadcast ke realtime broker (SSE)
   - Trigger webhook notifications
7. Broadcast trade ke realtime broker
```

### 4. Database Layer (`database/`)

**Files:**

- `connection.go`: Database initialization
- `models.go`: GORM models
- `repository.go`: Data access operations

**Models:**

```go
type Trade struct {
    // Time-series data untuk running trades
    ExecutedAt time.Time (PK, hypertable dimension)
    StockSymbol string
    Price, Volume, Value float64
    Action string (BUY/SELL)
    MarketBoard string (RG/TN/NG)
}

type Candle struct {
    // Aggregated OHLCV data
    Bucket time.Time (1-minute intervals)
    StockSymbol string
    Open, High, Low, Close, Volume, Value float64
}

type WhaleAlert struct {
    // Detected whale activities
    ID uint (auto-increment)
    DetectedAt time.Time
    StockSymbol, Action string
    TriggerPrice, TriggerVolumeLots, TriggerValue float64
    ZScore, VolumeVsAvgPct float64
}

type WhaleWebhook struct {
    // Webhook configuration
    ID uint
    Name, URL, Method string
    IsActive bool
}
```

**TimescaleDB Features:**

- **Hypertables**: Auto-partitioning by time
- **Continuous Aggregates**: Real-time OHLCV calculation
- **Retention Policies**: Auto-cleanup old data
- **Compression**: Space optimization for historical data

**Key Repository Methods:**

- `SaveTrade()`: Insert trade dengan conflict handling
- `GetStockStats()`: Calculate mean/stddev untuk whale detection
- `SaveWhaleAlert()`: Persist whale alert
- `GetAccumulationPattern()`: Detect accumulation/distribution patterns
- `GetExtremeAnomalies()`: Query anomalies dengan Z-Score >= 4.0

### 5. API Server (`api/`)

**Files:**

- `server.go`: Main HTTP server, routing, handlers
- `server_symbol_handler.go`: Symbol-specific analysis

**Endpoints:**

**REST API:**

- `GET /health`: Health check
- `GET /api/whales`: List whale alerts (dengan filters)
- `GET /api/whales/stats`: Aggregated statistics
- `GET /api/config/webhooks`: CRUD operations untuk webhooks

**SSE Streaming API:**

- `GET /api/llm/accumulation/stream`: Stream accumulation analysis
- `GET /api/llm/anomalies/stream`: Stream anomaly analysis
- `GET /api/llm/time-stats/stream`: Stream time-based analysis
- `GET /api/llm/symbol/stream`: Stream symbol-specific analysis

**SSE Implementation:**

```go
// Set SSE headers
w.Header().Set("Content-Type", "text/event-stream")
w.Header().Set("Cache-Control", "no-cache")
w.Header().Set("Connection", "keep-alive")

// Stream data menggunakan callback
llmClient.AnalyzeStream(ctx, prompt, func(chunk string) error {
    fmt.Fprintf(w, "data: %s\n\n", chunk)
    w.(http.Flusher).Flush()
    return nil
})
```

### 6. Real-time Broker (`realtime/`)

**File:** `broker.go`

**Tanggung Jawab:**

- Manage SSE client connections
- Broadcast messages ke semua clients
- Auto-cleanup disconnected clients

**Architecture:**

```go
type Broker struct {
    clients    map[chan string]bool
    newClients chan chan string
    broadcast  chan string
    closing    chan chan string
}
```

**Flow:**

1. Client subscribe via HTTP endpoint
2. Broker creates dedicated channel untuk client
3. Broadcast messages dikirim ke semua active channels
4. Client disconnect → channel closed dan removed

### 7. LLM Integration (`llm/`)

**Files:**

- `client.go`: OpenAI-compatible HTTP client
- `patterns.go`: Pattern detection prompts

**Features:**

- Streaming response support (SSE)
- Context-aware prompts dengan Indonesia language
- Multiple analysis types (accumulation, anomalies, timing)

**LLM Client Flow:**

```
1. Prepare chat messages (system + user prompt)
2. Send POST to /chat/completions dengan stream=true
3. Parse Server-Sent Events response
4. Extract delta content dari chunks
5. Invoke callback untuk setiap chunk
6. Handle [DONE] signal untuk stream end
```

**Prompt Engineering:**

- System prompt: "Analis keuangan ahli di pasar Indonesia"
- Context injection: Historical whale data dari database
- Structured output: Insights yang actionable dan ringkas

### 8. Notification System (`notifications/`)

**File:** `webhook_manager.go`

**Tanggung Jawab:**

- Manage webhook configurations
- Deliver whale alerts ke external endpoints
- Log delivery results
- Retry mechanism untuk failed deliveries

**Delivery Flow:**

```go
1. Whale detected → Generate webhook payload
2. Query active webhooks dari database
3. For each webhook:
   - Prepare HTTP request dengan payload
   - Send POST request
   - Log result (success/failure)
   - Handle errors dan retries
```

**Payload Format:**

```json
{
  "trace_id": "whale-alert-{timestamp}",
  "type": "SINGLE_TRADE",
  "data": {
    "StockSymbol": "BBCA",
    "Action": "BUY",
    "TriggerPrice": 9850,
    "TriggerVolumeLots": 50000,
    "Message": "🐋 WHALE ALERT! ...",
    "ZScore": 4.52,
    "VolumeVsAvgPct": 850.5
  }
}
```

### 9. Caching Layer (`cache/`)

**File:** `redis.go`

**Tanggung Jawab:**

- Cache stock statistics (mean, stddev)
- Session management
- Rate limiting (future)

**Cache Keys:**

```
stats:{symbol}:60m → StockStats (TTL: 5 minutes)
session:{user_id} → Session data
```

**Benefits:**

- Reduce database load untuk repeated queries
- Fast lookup untuk statistical calculations
- Shared state across multiple app instances

### 10. Web Interface (`public/`)

**Files:**

- `index.html`: Structure dan layout
- `script.js`: Client-side logic dan SSE handling
- `style.css`: Modern, responsive styling

**Features:**

- Real-time whale alerts table
- AI pattern analysis panel (4 modes)
- Advanced filtering controls
- Live statistics ticker
- SSE client untuk streaming updates

**JavaScript Architecture:**

```javascript
// Main components
- fetchWhales(): Fetch dan display whale alerts
- setupFilters(): Filter controls
- setupLLMAnalysis(): AI panel logic
- connectSSE(): SSE connection management

// SSE implementation
const eventSource = new EventSource('/api/llm/.../stream');
eventSource.onmessage = (event) => {
    // Append streaming chunks ke output
    outputElement.innerHTML += event.data;
};
```

## Alur Data

### Alur Pemrosesan Trade

```
1. Stockbit WebSocket → Binary protobuf message
2. WebSocket Client → Decode protobuf
3. Handler Manager → Route ke RunningTradeHandler
4. RunningTradeHandler → Process trade:
   a. Save ke TimescaleDB
   b. Get statistics dari Redis/DB
   c. Calculate Z-Score
   d. Check whale criteria
5. If whale:
   a. Save WhaleAlert ke DB
   b. Broadcast to realtime broker (SSE clients)
   c. Trigger webhook notifications
6. Broadcast trade ke realtime broker
```

### Alur Request API

```
1. Client → HTTP request ke API endpoint
2. Middleware → CORS, Logging
3. Handler → Parse query params, validate
4. Repository → Query database
5. LLM (optional) → Analyze data
6. Response → JSON or SSE stream
```

### Alur Streaming SSE

```
1. Client → GET /api/llm/.../stream
2. Server → Set SSE headers
3. Repository → Query historical data
4. LLM Client → Send streaming request
5. For each chunk:
   a. Receive dari LLM
   b. Write to HTTP response
   c. Flush immediately
6. Client → Receive incremental updates
```

## Algoritma Deteksi Whale

### Pendekatan Statistik

**Data Input:**

- Last 60 minutes of trades untuk symbol
- Current trade (price, volume, value)

**Perhitungan:**

1. **Statistical Baseline:**

```
μ_vol = MEAN(volume_lots)
σ_vol = STDDEV(volume_lots)
μ_price = MEAN(price)
```

2. **Z-Score:**

```
Z = (current_volume - μ_vol) / σ_vol
```

3. **Relative Volume:**

```
Vol% = (current_volume / μ_vol) × 100
```

**Logika Keputusan:**

```
IF total_value < 100M:
    RETURN False  // Safety floor

IF statistics_available:
    IF Z >= 3.0 OR Vol% >= 500:
        RETURN True  // Whale detected
    ELSE:
        RETURN False
ELSE:  // Fallback untuk new/illiquid stocks
    IF volume >= 1000 OR total_value >= 1B:
        RETURN True
    ELSE:
        RETURN False
```

**Skor Kepercayaan:**
Currently fixed at 100% for all detected whales. Future: Graduated scoring based on Z-Score tiers.

## Pertimbangan Scaling

### Scaling Horizontal

**App Instances:**

- Stateless design memungkinkan multiple instances
- Use load balancer (nginx, HAProxy)
- Shared state via Redis

**Database:**

- TimescaleDB clustering untuk distributed queries
- Read replicas untuk API queries
- Master untuk writes

**Redis:**

- Redis Cluster untuk high availability
- Sentinel untuk automatic failover

### Scaling Vertikal

**Database:**

- Increase `shared_buffers`, `work_mem`
- Adjust chunk intervals (default: 7 days)
- Enable compression policies

**Application:**

- Increase worker goroutines
- Tune WebSocket buffer sizes
- Connection pooling

### Optimisasi Performa

**Database Queries:**

- Index pada `stock_symbol`, `detected_at`, `action`
- Materialized views untuk aggregations
- Query result caching

**API:**

- Response compression (gzip)
- ETags untuk client-side caching
- Rate limiting per client

**WebSocket:**

- Binary protobuf (lebih efficient dari JSON)
- Connection keep-alive tuning
- Message batching untuk high-frequency updates

## Security

### Authentication

- Stockbit credentials via environment variables
- Token caching dengan file permissions
- No hardcoded secrets

### API Security

- CORS configuration
- Input validation dan sanitization
- SQL injection prevention (GORM ORM)

### Database

- Strong passwords (production)
- Network isolation (Docker network)
- Backup dan retention policies

### Webhooks

- URL validation before saving
- HTTPS enforcement (recommended)
- Delivery logging untuk audit

## Monitoring & Debugging

### Logging

- Structured logging dengan timestamps
- Log levels: INFO, WARN, ERROR
- Request/response logging di middleware

### Metrics (Future)

- Prometheus integration
- Grafana dashboards
- Alert rules untuk anomalies

### Tracing (Future)

- OpenTelemetry integration
- Distributed tracing untuk request flows
- Performance profiling

## Dependencies

### Library Utama

- `gorilla/websocket`: WebSocket client
- `gorm.io/gorm`: ORM untuk database operations
- `google.golang.org/protobuf`: Protobuf encoding/decoding
- `go-redis/redis`: Redis client

### Infrastructure

- TimescaleDB: Time-series database
- Redis: Caching layer
- Docker & Docker Compose: Containerization

### Layanan Eksternal

- Stockbit WebSocket API
- LLM API (OpenAI compatible)

## Pengembangan Masa Depan

### Jangka Pendek

- [ ] Graduated confidence scoring
- [ ] More granular alerts (MEDIUM_TRADE tier)
- [ ] Advanced filters di web UI
- [ ] Export data (CSV, Excel)

### Jangka Menengah

- [ ] Machine learning untuk pattern prediction
- [ ] Portfolio tracking
- [ ] Alert rules customization
- [ ] Mobile app (React Native)

### Jangka Panjang

- [ ] Automated trading execution
- [ ] Multi-exchange support
- [ ] Social features (sharing insights)
- [ ] Premium subscription tiers

---

**Terakhir Diperbarui:** 2025-12-22

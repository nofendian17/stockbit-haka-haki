# 📡 Dokumentasi API

## Base URL

```
http://localhost:8080
```

## Autentikasi

Saat ini, API tidak memerlukan autentikasi untuk operasi baca. Endpoint manajemen webhook terbuka namun sebaiknya dilindungi di environment production.

## Format Response

Semua response API mengikuti kode status HTTP standar:

- `200 OK`: Request berhasil
- `400 Bad Request`: Parameter tidak valid
- `404 Not Found`: Resource tidak ditemukan
- `500 Internal Server Error`: Server error

Response JSON memiliki struktur konsisten:

```json
{
  "data": [...],      // Untuk request yang berhasil
  "error": "message"  // Untuk response error
}
```

---

## Endpoints

### 1. Health Check

Cek apakah API server berjalan.

**Endpoint:**

```
GET /health
```

**Response:**

```json
{
  "status": "ok"
}
```

**Contoh cURL:**

```bash
curl http://localhost:8080/health
```

---

### 2. Get Whale Alerts

Mendapatkan whale alerts dengan filter opsional.

**Endpoint:**

```
GET /api/whales
```

**Query Parameters:**

| Parameter | Tipe    | Required | Default     | Deskripsi                                   |
| --------- | ------- | -------- | ----------- | ------------------------------------------- |
| `symbol`  | string  | Tidak    | Semua       | Filter berdasarkan simbol saham (mis: BBCA) |
| `limit`   | integer | Tidak    | 50          | Maksimal jumlah record (maks: 1000)         |
| `start`   | string  | Tidak    | 24 jam lalu | Waktu mulai (format RFC3339)                |
| `end`     | string  | Tidak    | Sekarang    | Waktu akhir (format RFC3339)                |
| `type`    | string  | Tidak    | Semua       | Tipe alert (SINGLE_TRADE, ACCUMULATION)     |
| `action`  | string  | Tidak    | Semua       | BUY atau SELL                               |

**Response:**

```json
[
  {
    "ID": 12345,
    "DetectedAt": "2024-12-22T14:29:28+07:00",
    "StockSymbol": "BBCA",
    "AlertType": "SINGLE_TRADE",
    "Action": "BUY",
    "TriggerPrice": 9850,
    "TriggerVolumeLots": 50000,
    "TriggerValue": 492500000000,
    "ZScore": 4.52,
    "VolumeVsAvgPct": 850.5,
    "AvgPrice": 9800,
    "ConfidenceScore": 100,
    "MarketBoard": "RG"
  }
]
```

**cURL Examples:**

```bash
# Get latest 10 whale alerts
curl "http://localhost:8080/api/whales?limit=10"

# Get BBCA whale alerts from specific date range
curl "http://localhost:8080/api/whales?symbol=BBCA&start=2024-12-20T00:00:00Z&end=2024-12-22T23:59:59Z"

# Get only BUY alerts with high volume
curl "http://localhost:8080/api/whales?action=BUY&limit=20"
```

---

### 3. Get Whale Statistics

Get aggregated statistics for whale activities.

**Endpoint:**

```
GET /api/whales/stats
```

**Query Parameters:**

| Parameter | Type   | Required | Default | Description            |
| --------- | ------ | -------- | ------- | ---------------------- |
| `symbol`  | string | No       | All     | Filter by stock symbol |
| `start`   | string | Yes      | -       | Start time (RFC3339)   |
| `end`     | string | Yes      | -       | End time (RFC3339)     |

**Response:**

```json
{
  "stock_symbol": "BBCA",
  "total_whale_trades": 142,
  "total_whale_value": 15432000000000,
  "buy_volume_lots": 1234567,
  "sell_volume_lots": 987654,
  "largest_trade_value": 500000000000,
  "avg_zscore": 3.85,
  "start_time": "2024-12-20T00:00:00Z",
  "end_time": "2024-12-22T23:59:59Z"
}
```

**cURL Examples:**

```bash
# Get stats for last 24 hours (all stocks)
curl "http://localhost:8080/api/whales/stats?start=2024-12-21T00:00:00Z&end=2024-12-22T00:00:00Z"

# Get stats for specific symbol
curl "http://localhost:8080/api/whales/stats?symbol=TLKM&start=2024-12-01T00:00:00Z&end=2024-12-31T23:59:59Z"
```

---

### 4. Webhook Management

#### 4.1 List All Webhooks

**Endpoint:**

```
GET /api/config/webhooks
```

**Response:**

```json
[
  {
    "ID": 1,
    "Name": "Discord Alert Channel",
    "URL": "https://discord.com/api/webhooks/123456789/abcdefg",
    "Method": "POST",
    "IsActive": true,
    "TotalSent": 234,
    "CreatedAt": "2024-12-01T10:00:00Z"
  }
]
```

**cURL Example:**

```bash
curl http://localhost:8080/api/config/webhooks
```

#### 4.2 Create Webhook

**Endpoint:**

```
POST /api/config/webhooks
```

**Request Body:**

```json
{
  "name": "My Webhook",
  "url": "https://webhook.site/unique-url",
  "method": "POST",
  "is_active": true
}
```

**Response:**

```json
{
  "ID": 2,
  "Name": "My Webhook",
  "URL": "https://webhook.site/unique-url",
  "Method": "POST",
  "IsActive": true,
  "TotalSent": 0,
  "CreatedAt": "2024-12-22T15:30:00Z"
}
```

**cURL Example:**

```bash
curl -X POST http://localhost:8080/api/config/webhooks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Discord Whale Alerts",
    "url": "https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_TOKEN",
    "method": "POST",
    "is_active": true
  }'
```

#### 4.3 Update Webhook

**Endpoint:**

```
PUT /api/config/webhooks/{id}
```

**Request Body:**

```json
{
  "name": "Updated Name",
  "url": "https://new-url.com/webhook",
  "method": "POST",
  "is_active": false
}
```

**cURL Example:**

```bash
curl -X PUT http://localhost:8080/api/config/webhooks/1 \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Disabled Webhook",
    "is_active": false
  }'
```

#### 4.4 Delete Webhook

**Endpoint:**

```
DELETE /api/config/webhooks/{id}
```

**Response:**

```json
{
  "message": "Webhook deleted successfully"
}
```

**cURL Example:**

```bash
curl -X DELETE http://localhost:8080/api/config/webhooks/1
```

---

### 5. Webhook Payload Format

When a whale is detected, all active webhooks receive this JSON payload:

**POST to webhook URL:**

```json
{
  "trace_id": "whale-alert-1703251828",
  "type": "SINGLE_TRADE",
  "timestamp": "2024-12-22T14:29:28+07:00",
  "data": {
    "StockSymbol": "BBCA",
    "Action": "BUY",
    "TriggerPrice": 9850,
    "TriggerVolumeLots": 50000,
    "TriggerValue": 492500000000,
    "AvgPrice": 9800,
    "Message": "🐋 WHALE ALERT! BBCA BUY | Vol: 50,000 lots (850% Avg) | Value: Rp 492.50 B | Price: 9,850 (Avg: 9,800) | Z-Score: 4.52 | Board: RG",
    "ZScore": 4.52,
    "VolumeVsAvgPct": 850.5,
    "ConfidenceScore": 100,
    "MarketBoard": "RG",
    "DetectedAt": "2024-12-22T14:29:28+07:00"
  }
}
```

**Field Descriptions:**

- `trace_id`: Unique identifier untuk tracking
- `type`: Alert type (currently always SINGLE_TRADE)
- `timestamp`: Waktu webhook dikirim
- `data.StockSymbol`: Kode saham (4 huruf)
- `data.Action`: BUY atau SELL
- `data.TriggerPrice`: Harga transaksi whale
- `data.TriggerVolumeLots`: Volume dalam lots (1 lot = 100 shares)
- `data.TriggerValue`: Total nilai transaksi (IDR)
- `data.AvgPrice`: Harga rata-rata 60 menit terakhir
- `data.ZScore`: Standard deviations dari mean volume
- `data.VolumeVsAvgPct`: Persentase volume vs rata-rata
- `data.ConfidenceScore`: Confidence level (0-100)
- `data.MarketBoard`: RG (Regular), TN (Tunai), NG (Negosiasi)
- `data.Message`: Human-readable message (siap kirim ke Discord/Slack)

---

### 6. Real-time Events Stream (SSE)

Subscribe ke whale alerts real-time menggunakan Server-Sent Events.

**Endpoint:**

```
GET /api/events
```

**Response:** Server-Sent Events stream dengan whale alerts real-time

**JavaScript Example:**

```javascript
const eventSource = new EventSource("/api/events");

eventSource.onmessage = function (event) {
  const whaleAlert = JSON.parse(event.data);
  console.log("New whale alert:", whaleAlert);

  // Update UI with new alert
  addWhaleAlertToTable(whaleAlert);
};

eventSource.onerror = function (error) {
  console.error("SSE error:", error);
  // Reconnect logic
};
```

**Event Data Format:**

```json
{
  "ID": 12345,
  "DetectedAt": "2024-12-23T14:29:28+07:00",
  "StockSymbol": "BBCA",
  "AlertType": "SINGLE_TRADE",
  "Action": "BUY",
  "TriggerPrice": 9850,
  "TriggerVolumeLots": 50000,
  "TriggerValue": 492500000000,
  "ZScore": 4.52,
  "VolumeVsAvgPct": 850.5,
  "MarketBoard": "RG"
}
```

---

### 7. Trading Strategy Signals

Mendapatkan sinyal strategi trading berdasarkan pattern detection dan analisis volume.

#### 7.1 Get Strategy Signals (REST)

**Endpoint:**

```
GET /api/strategies/signals
```

**Query Parameters:**

| Parameter        | Tipe    | Default | Deskripsi                                                                     |
| ---------------- | ------- | ------- | ----------------------------------------------------------------------------- |
| `lookback`       | integer | 60      | Lookback period dalam menit                                                   |
| `min_confidence` | float   | 0.3     | Minimum confidence score (0.0 - 1.0)                                          |
| `strategy`       | string  | Semua   | Filter by strategy: VOLUME_BREAKOUT, MEAN_REVERSION, FAKEOUT_FILTER, atau ALL |

**Response:**

```json
{
  "signals": [
    {
      "Timestamp": "2024-12-23T14:30:00+07:00",
      "StockSymbol": "BBCA",
      "Strategy": "VOLUME_BREAKOUT",
      "Decision": "BUY",
      "Confidence": 0.85,
      "Price": 9850,
      "Volume": 125000,
      "Reasoning": "Volume spike 850% above average with strong buying pressure"
    }
  ],
  "count": 1
}
```

**cURL Example:**

```bash
# Get all strategy signals from last hour
curl "http://localhost:8080/api/strategies/signals?lookback=60"

# Get only high-confidence VOLUME_BREAKOUT signals
curl "http://localhost:8080/api/strategies/signals?strategy=VOLUME_BREAKOUT&min_confidence=0.7"
```

#### 7.2 Stream Strategy Signals (SSE)

Stream sinyal strategi secara real-time.

**Endpoint:**

```
GET /api/strategies/signals/stream
```

**Query Parameters:**

| Parameter  | Tipe   | Deskripsi                          |
| ---------- | ------ | ---------------------------------- |
| `strategy` | string | Filter by strategy type (opsional) |

**Response:** Server-Sent Events stream

**JavaScript Example:**

```javascript
const eventSource = new EventSource(
  "/api/strategies/signals/stream?strategy=VOLUME_BREAKOUT"
);

// Connection established
eventSource.addEventListener("connected", (event) => {
  console.log("Connected to strategy signals stream");
});

// New strategy signal
eventSource.addEventListener("signal", (event) => {
  const signal = JSON.parse(event.data);
  console.log("New strategy signal:", signal);
  displaySignal(signal);
});

eventSource.onerror = (error) => {
  console.error("Stream error:", error);
  eventSource.close();
};
```

**Event Types:**

- `connected`: Koneksi berhasil established
- `signal`: Sinyal strategi baru terdeteksi

**Signal Event Data:**

```json
{
  "Timestamp": "2024-12-23T14:30:00+07:00",
  "StockSymbol": "BBCA",
  "Strategy": "VOLUME_BREAKOUT",
  "Decision": "BUY",
  "Confidence": 0.85,
  "Price": 9850,
  "Volume": 125000,
  "Reasoning": "Strong volume breakout with institutional buying pressure"
}
```

---

### 8. Accumulation/Distribution Summary

Mendapatkan ringkasan terpisah untuk top stocks dengan akumulasi (buying) dan distribusi (selling) terbanyak.

**Endpoint:**

```
GET /api/accumulation-summary
```

**Query Parameters:**

| Parameter | Tipe    | Default | Deskripsi          |
| --------- | ------- | ------- | ------------------ |
| `hours`   | integer | 24      | Hours to look back |

**Response:**

```json
{
  "accumulation": [
    {
      "StockSymbol": "BBCA",
      "TotalAlerts": 25,
      "TotalValue": 1250000000000,
      "AvgZScore": 4.2,
      "FirstAlert": "2024-12-22T09:00:00+07:00",
      "LastAlert": "2024-12-23T15:30:00+07:00"
    }
  ],
  "distribution": [
    {
      "StockSymbol": "TLKM",
      "TotalAlerts": 18,
      "TotalValue": 850000000000,
      "AvgZScore": 3.8,
      "FirstAlert": "2024-12-22T10:00:00+07:00",
      "LastAlert": "2024-12-23T14:00:00+07:00"
    }
  ],
  "accumulation_count": 20,
  "distribution_count": 20,
  "hours_back": 24
}
```

**Field Descriptions:**

- `accumulation`: Top 20 saham dengan aktivitas whale BUY terbanyak
- `distribution`: Top 20 saham dengan aktivitas whale SELL terbanyak
- `TotalAlerts`: Jumlah whale alerts untuk saham tersebut
- `TotalValue`: Total nilai transaksi whale (IDR)
- `AvgZScore`: Rata-rata Z-Score dari semua alerts
- `FirstAlert` / `LastAlert`: Timestamp whale alert pertama dan terakhir

**cURL Example:**

```bash
# Get accumulation/distribution summary for last 24 hours
curl "http://localhost:8080/api/accumulation-summary?hours=24"

# Get summary for last 7 days
curl "http://localhost:8080/api/accumulation-summary?hours=168"
```

**Use Cases:**

- Identifikasi saham yang sedang di-**accumulate** (dikumpulkan) oleh whale
- Deteksi saham yang sedang di-**distribute** (dijual) oleh institutional investors
- Analisis sentiment dan trend pasar berdasarkan aktivitas whale
- Watchlist stocks dengan high whale activity

---

## Analisis Pola dengan LLM

> **Note:** Requires `LLM_ENABLED=true` in `.env`

### 6. Accumulation Pattern Analysis

Detect continuous BUY/SELL patterns indicating accumulation or distribution.

**Endpoint (Non-streaming):**

```
GET /api/patterns/accumulation
```

**Query Parameters:**

| Parameter    | Type    | Default | Description                   |
| ------------ | ------- | ------- | ----------------------------- |
| `hours`      | integer | 24      | Hours to look back            |
| `min_alerts` | integer | 3       | Minimum whale alerts required |

**Response:**

```json
{
  "analysis": "Berdasarkan data 24 jam terakhir, terdeteksi pola akumulasi signifikan pada saham BBCA dengan 15 whale BUY alerts dan total value Rp 2.5 T. Pattern menunjukkan strong buying pressure dari institutional investors..."
}
```

**Endpoint (Streaming SSE):**

```
GET /api/patterns/accumulation/stream
```

**Response:** Server-Sent Events stream

**JavaScript Example:**

```javascript
const eventSource = new EventSource(
  "/api/patterns/accumulation/stream?hours=24&min_alerts=3"
);

eventSource.onmessage = function (event) {
  console.log("Chunk:", event.data);
  // Append to display
  document.getElementById("output").innerHTML += event.data;
};

eventSource.onerror = function (error) {
  console.error("SSE error:", error);
  eventSource.close();
};

// Stop streaming
function stopAnalysis() {
  eventSource.close();
}
```

**cURL Example (non-streaming):**

```bash
curl "http://localhost:8080/api/patterns/accumulation?hours=48&min_alerts=5"
```

---

### 7. Extreme Anomalies Analysis

Identify extreme trading anomalies with very high Z-Scores.

**Endpoint (Non-streaming):**

```
GET /api/patterns/anomalies
```

**Query Parameters:**

| Parameter    | Type    | Default | Description               |
| ------------ | ------- | ------- | ------------------------- |
| `hours`      | integer | 24      | Hours to look back        |
| `min_zscore` | float   | 5.0     | Minimum Z-Score threshold |

**Response:**

```json
{
  "analysis": "Terdeteksi 3 anomali ekstrem dalam 24 jam terakhir:\n1. BUKA SELL - Z-Score 5.2 (Value: Rp 850M)\n2. TLKM BUY - Z-Score 4.8 (Value: Rp 1.2B)\n3. GOTO SELL - Z-Score 4.5 (Value: Rp 680M)\n\nAnomali ini mengindikasikan..."
}
```

**Endpoint (Streaming SSE):**

```
GET /api/patterns/anomalies/stream
```

**cURL Example:**

```bash
curl "http://localhost:8080/api/patterns/anomalies?hours=12&min_zscore=5.0"
```

---

### 8. Time-based Statistics Analysis

Analyze whale activity patterns by time of day.

**Endpoint (Non-streaming):**

```
GET /api/patterns/timing
```

**Query Parameters:**

| Parameter | Type    | Default | Description       |
| --------- | ------- | ------- | ----------------- |
| `days`    | integer | 7       | Days to look back |

**Response:**

```json
{
  "analysis": "Analisis pola waktu 7 hari terakhir:\n- Jam 09:00-10:00 WIB: Aktivitas whale tertinggi (32% dari total)\n- Jam 14:00-15:00 WIB: High volatility period (28%)\n- Jam 11:00-12:00 WIB: Lowest activity (8%)\n\nRekomendasi timing..."
}
```

**Endpoint (Streaming SSE):**

```
GET /api/patterns/timing/stream
```

**cURL Example:**

```bash
curl "http://localhost:8080/api/patterns/timing?days=14"
```

---

### 9. Symbol-specific Analysis

Deep analysis for a specific stock symbol.

**Endpoint (Streaming SSE only):**

```
GET /api/patterns/symbol/stream
```

**Query Parameters:**

| Parameter | Type    | Required | Default | Description               |
| --------- | ------- | -------- | ------- | ------------------------- |
| `symbol`  | string  | Yes      | -       | Stock symbol (e.g., BBCA) |
| `hours`   | integer | No       | 24      | Hours to look back        |

**Response:** Server-Sent Events stream with comprehensive analysis

**JavaScript Example:**

```javascript
const symbol = document.getElementById("symbolInput").value.toUpperCase();
const eventSource = new EventSource(
  `/api/patterns/symbol/stream?symbol=${symbol}&hours=48`
);

eventSource.onmessage = function (event) {
  document.getElementById("analysis").innerHTML += event.data;
};
```

**cURL Example:**

```bash
# Note: cURL will receive streaming response
curl "http://localhost:8080/api/patterns/symbol/stream?symbol=BBCA&hours=24"
```

---

## Error Responses

### 400 Bad Request

```json
{
  "error": "invalid parameter: limit must be between 1 and 1000"
}
```

### 404 Not Found

```json
{
  "error": "webhook not found"
}
```

### 500 Internal Server Error

```json
{
  "error": "database connection failed"
}
```

### 503 Service Unavailable (LLM)

```json
{
  "error": "LLM service is not enabled"
}
```

---

## Rate Limiting

Currently, there are no rate limits implemented. For production use:

- Recommended: 100 requests per minute per IP
- Webhook deliveries: Max 3 retries with exponential backoff
- SSE connections: Max 10 concurrent connections per client

---

## CORS Configuration

The API allows cross-origin requests from any origin:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type
```

For production, configure specific allowed origins.

---

## Pagination

For endpoints returning large datasets:

- Use `limit` parameter to control page size
- Maximum limit: 1000 records
- Use `start` and `end` times for time-based pagination

**Example:**

```bash
# Page 1 (first 100 records)
curl "http://localhost:8080/api/whales?limit=100"

# Page 2 (next 100, using last timestamp from page 1)
curl "http://localhost:8080/api/whales?limit=100&start=2024-12-22T10:00:00Z"
```

---

## Webhook Testing

### Test with webhook.site

1. Go to https://webhook.site
2. Copy your unique URL
3. Create webhook:

```bash
curl -X POST http://localhost:8080/api/config/webhooks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Webhook",
    "url": "https://webhook.site/YOUR-UNIQUE-ID",
    "method": "POST",
    "is_active": true
  }'
```

4. Wait for whale detection or trigger manual test
5. Check webhook.site for received payload

### Discord Webhook Example

```bash
curl -X POST http://localhost:8080/api/config/webhooks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Discord Alerts",
    "url": "https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_TOKEN",
    "method": "POST",
    "is_active": true
  }'
```

**Note:** Discord webhooks expect specific format. You may need to transform the payload.

---

## Best Practices

### API Usage

1. **Use appropriate time ranges**: Avoid querying years of data in single request
2. **Cache responses**: Implement client-side caching for frequently accessed data
3. **Handle errors gracefully**: Implement retry logic with exponential backoff
4. **Use SSE for real-time**: Prefer SSE endpoints over polling for live updates

### Webhook Configuration

1. **Use HTTPS**: Always use HTTPS URLs for webhooks
2. **Validate URLs**: Test webhook URLs before deploying to production
3. **Monitor deliveries**: Regularly check webhook logs for failures
4. **Implement idempotency**: Handle duplicate webhook deliveries using `trace_id`

### Performance

1. **Limit concurrent SSE connections**: Close inactive connections
2. **Use appropriate `limit` values**: Don't fetch more data than needed
3. **Filter early**: Use query parameters to reduce payload size
4. **Monitor API response times**: Alert on degraded performance

---

## Client Libraries

### JavaScript/TypeScript

```typescript
class StockbitAPI {
  baseURL = "http://localhost:8080";

  async getWhales(filters?: WhaleFilters) {
    const params = new URLSearchParams(filters);
    const response = await fetch(`${this.baseURL}/api/whales?${params}`);
    return response.json();
  }

  async getStats(start: string, end: string, symbol?: string) {
    const params = new URLSearchParams({ start, end });
    if (symbol) params.append("symbol", symbol);
    const response = await fetch(`${this.baseURL}/api/whales/stats?${params}`);
    return response.json();
  }

  streamAnalysis(type: string, params: any, callback: (chunk: string) => void) {
    const queryString = new URLSearchParams(params).toString();
    const eventSource = new EventSource(
      `${this.baseURL}/api/patterns/${type}/stream?${queryString}`
    );

    eventSource.onmessage = (event) => callback(event.data);
    return eventSource;
  }
}
```

### Python

```python
import requests
from typing import Optional, Dict, Any

class StockbitAPI:
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url

    def get_whales(self, **filters) -> list:
        response = requests.get(f"{self.base_url}/api/whales", params=filters)
        response.raise_for_status()
        return response.json()

    def get_stats(self, start: str, end: str, symbol: Optional[str] = None) -> dict:
        params = {"start": start, "end": end}
        if symbol:
            params["symbol"] = symbol
        response = requests.get(f"{self.base_url}/api/whales/stats", params=params)
        response.raise_for_status()
        return response.json()

    def create_webhook(self, name: str, url: str, method: str = "POST", is_active: bool = True) -> dict:
        data = {
            "name": name,
            "url": url,
            "method": method,
            "is_active": is_active
        }
        response = requests.post(f"{self.base_url}/api/config/webhooks", json=data)
        response.raise_for_status()
        return response.json()
```

---

## Changelog

### v2.0.0 (2025-12-23)

- Menambahkan endpoint baru: Trading Strategy Signals, Accumulation Summary, Real-time Events Stream
- Memperbaiki path API dari `/api/llm/*` ke `/api/patterns/*`
- Update default `min_zscore` menjadi 5.0
- Menambahkan dokumentasi lengkap untuk 3 endpoint baru

### v1.0.0 (2024-12-22)

- Initial API release
- Whale alerts endpoints
- Webhook management
- LLM pattern analysis (streaming and non-streaming)
- SSE support for real-time updates

---

**Untuk issues atau pertanyaan, silakan rujuk ke [README.md](../README.md) atau [ARCHITECTURE.md](ARCHITECTURE.md)**

# API Reference

The Stockbit Analysis system exposes a REST API on port `8080` (default) for retrieving market data, whale alerts, trading signals, and AI-powered analysis. It also supports Server-Sent Events (SSE) for real-time data streaming.

**Base URL:** `http://localhost:8080`

## Table of Contents

1. [Health Check](#health-check)
2. [Whale Alerts](#whale-alerts)
3. [Trading Strategies & Signals](#trading-strategies--signals)
4. [Analytics & Performance](#analytics--performance)
5. [Webhook Management](#webhook-management)
6. [Real-time Events (SSE)](#real-time-events-sse)

---

## Health Check

### Get System Health
`GET /health`

Checks if the API server is running and responsive.

**Response:**
```json
{
  "status": "ok"
}
```

---

## Whale Alerts

### Get Historical Whale Alerts
`GET /api/whales`

Retrieve a list of detected whale alerts with optional filtering.

**Parameters:**
- `symbol` (optional): Filter by stock symbol (e.g., `BBCA`).
- `type` (optional): Filter by alert type (`SINGLE_TRADE`, `ACCUMULATION`, `DISTRIBUTION`).
- `action` (optional): Filter by action (`BUY`, `SELL`).
- `board` (optional): Filter by market board (`RG`, `TN`, `NG`).
- `min_value` (optional): Filter by minimum transaction value.
- `start` (optional): Start time (RFC3339 format, e.g., `2024-01-01T00:00:00Z`).
- `end` (optional): End time (RFC3339 format).
- `limit` (optional): Max results (default 50, max 200).
- `offset` (optional): Pagination offset.

**Response:**
```json
{
  "data": [
    {
      "id": 123,
      "detected_at": "2024-01-15T10:30:00Z",
      "stock_symbol": "BBCA",
      "alert_type": "SINGLE_TRADE",
      "action": "BUY",
      "trigger_price": 9500,
      "trigger_volume_lots": 5000,
      "trigger_value": 4750000000,
      "z_score": 4.5,
      "confidence_score": 0.95
    }
  ],
  "total": 150,
  "limit": 50,
  "offset": 0,
  "has_more": true
}
```

### Get Whale Statistics
`GET /api/whales/stats`

Get aggregated statistics of whale activity for a symbol or overall.

**Parameters:**
- `symbol` (optional): Filter by stock symbol.
- `start` (optional): Start time (RFC3339).
- `end` (optional): End time (RFC3339).

**Response:**
```json
{
  "stock_symbol": "BBCA",
  "total_whale_trades": 45,
  "total_whale_value": 150000000000,
  "buy_volume_lots": 80000,
  "sell_volume_lots": 50000,
  "largest_trade_value": 10000000000
}
```

### Get Whale Follow-ups
`GET /api/whales/followups` or `GET /api/whales/{id}/followup`

Retrieve performance data showing price movement after a whale alert was detected.

**Parameters (for list):**
- `symbol` (optional): Stock symbol.
- `limit` (optional): Max results.

**Response:**
```json
[
  {
    "whale_alert_id": 123,
    "stock_symbol": "BBCA",
    "alert_time": "2024-01-15T10:30:00Z",
    "price_1min_later": 9525,
    "change_1min_pct": 0.26,
    "immediate_impact": "POSITIVE"
  }
]
```

---

## Trading Strategies & Signals

### Get Strategy Signals
`GET /api/strategies/signals`

Retrieve generated trading signals (e.g., Breakout, Mean Reversion).

**Parameters:**
- `strategy` (optional): Strategy name (`VOLUME_BREAKOUT`, `MEAN_REVERSION`, `FAKEOUT_FILTER`).
- `lookback` (optional): Lookback minutes.
- `min_confidence` (optional): Minimum confidence score (0.0 - 1.0).

**Response:**
```json
[
  {
    "stock_symbol": "NATO",
    "strategy": "VOLUME_BREAKOUT",
    "decision": "BUY",
    "confidence": 0.85,
    "price": 450,
    "reason": "Volume spike 500% with price breakout"
  }
]
```

### Get Signal History
`GET /api/signals/history`

Retrieve persisted history of generated signals.

**Parameters:**
- `symbol` (optional): Stock symbol.
- `strategy` (optional): Strategy name.
- `limit` (optional): Max records.

### Get Signal Outcomes
`GET /api/signals/{id}/outcome`

Check the performance outcome of a specific signal (Profit/Loss).

**Response:**
```json
{
  "signal_id": 55,
  "entry_price": 450,
  "exit_price": 475,
  "profit_loss_pct": 5.55,
  "outcome_status": "WIN"
}
```

---

## Market Analysis & Intelligence

### Accumulation/Distribution Summary
`GET /api/accumulation-summary`

Top 20 stocks with highest accumulation (buying) and distribution (selling) pressure.

---

## Analytics & Performance

### Stock Correlations
`GET /api/analytics/correlations`

Get correlation coefficients between stock pairs.

### Daily Performance
`GET /api/analytics/performance/daily`

Get daily strategy performance metrics.

### Open Positions
`GET /api/positions/open`

Get currently active trading positions based on signals.

---

## Webhook Management

Manage webhooks for receiving external notifications (Discord, Slack, Custom).

- `GET /api/config/webhooks`: List all webhooks.
- `POST /api/config/webhooks`: Create a new webhook.
- `PUT /api/config/webhooks/{id}`: Update a webhook.
- `DELETE /api/config/webhooks/{id}`: Delete a webhook.

**Payload Example:**
```json
{
  "name": "Discord Alert",
  "url": "https://discord.com/api/webhooks/...",
  "method": "POST",
  "is_active": true
}
```

---

## Real-time Events (SSE)

### Subscribe to Global Events
`GET /api/events`

Stream all whale alerts and system events in real-time.

### Subscribe to Signal Stream
`GET /api/strategies/signals/stream`

Stream generated trading strategy signals in real-time.

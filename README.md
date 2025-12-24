# 📈 Stockbit Analysis - Whale Detection & Pattern Recognition System

Aplikasi Go berkinerja tinggi yang dikontainerisasi untuk menganalisis data saham real-time dari Stockbit, mendeteksi pergerakan "whale" (investor besar), dan melakukan analisis pola menggunakan AI.

## ✨ Fitur Utama

### 🎯 Core Features

- **Real-time Data Streaming**: Koneksi WebSocket ke Stockbit menggunakan Protocol Buffers untuk feed trading real-time
- **Whale Detection**: Deteksi anomali statistik murni (Z-Score & Relative Volume) untuk mengidentifikasi aktivitas institusional signifikan tanpa konfigurasi manual
- **TimescaleDB Integration**: Penyimpanan time-series yang efisien untuk jutaan record trading dan candle OHLCV
- **Redis Caching**: Cache performa tinggi untuk statistik trading dan manajemen session
- **Token Persistence**: Cache token autentikasi untuk meminimalkan upaya login dan menghindari rate limit

### 🤖 AI-Powered Analysis

- **LLM Pattern Recognition**: Integrasi dengan LLM (OpenAI API compatible) untuk analisis pola trading cerdas
- **Streaming Analysis**: Analisis real-time menggunakan Server-Sent Events (SSE) untuk hasil yang responsif
- **Multiple Analysis Types**:
  - 📈 **Accumulation Pattern**: Deteksi pola akumulasi/distribusi berkelanjutan
  - ⚡ **Extreme Anomalies**: Identifikasi anomali trading ekstrem (Z-Score > 4.0)
  - ⏰ **Time-based Statistics**: Analisis aktivitas whale berdasarkan waktu dan jam trading
  - 🎯 **Symbol-specific Analysis**: Analisis mendalam per simbol saham

### 🌐 Web Interface

- **Modern Dashboard**: Interface web real-time dengan desain modern dan responsif
- **Live Whale Feed**: Streaming whale alerts real-time di browser
- **AI Analysis Panel**: Panel analisis AI dengan 4 mode berbeda
- **Advanced Filtering**: Filter berdasarkan simbol, aksi (BUY/SELL), nilai, dan board type
- **Real-time Statistics**: Dashboard statistik trading real-time

### 🔔 Notification System

- **Webhook Management**: Kelola multiple webhook endpoints untuk notifikasi
- **Discord/Slack Integration**: Kirim alert ke Discord, Slack, atau platform lainnya
- **Webhook Logs**: Tracking lengkap untuk setiap webhook delivery

### 🏗️ Architecture

- **Fully Dockerized**: Setup kontainer lengkap untuk deployment yang mudah dan konsisten
- **Microservices Ready**: Arsitektur modular yang siap untuk scaling
- **Graceful Shutdown**: Penanganan shutdown yang elegan dengan cleanup resource yang proper
- **Health Checks**: Endpoint health check untuk monitoring
- **Extensible API**: HTTP REST API dan SSE untuk berbagai use case

## 🔌 API Reference

The application exposes a REST API on port `8080` and a web interface at `http://localhost:8080`.

### 🌐 Web Interface

**URL:** `http://localhost:8080`

Modern, responsive web dashboard with:

- Real-time whale alert feed
- AI-powered pattern analysis panel (4 modes)
- Advanced filtering controls
- Live statistics ticker
- Interactive trade data table

### 📡 REST API Endpoints

#### 1. Health Check

**Endpoint:** `GET /health`

Check if the API server is running.

#### 2. Whale Alerts

Retrieve historical whale alerts.

**Endpoint:** `GET /api/whales`

**Query Parameters:**
| Parameter | Type | Description | Example |
| :--- | :--- | :--- | :--- |
| `symbol` | string | Filter by stock symbol | `BBCA` |
| `limit` | int | Max number of records (default 50) | `10` |
| `start` | string | Start time (RFC3339) | `2024-05-20T09:00:00Z` |
| `end` | string | End time (RFC3339) | `2024-05-20T16:00:00Z` |
| `type` | string | Filter by alert type | `SINGLE_TRADE` |

**Response Example:**

```json
[
  {
    "ID": 6208,
    "DetectedAt": "2024-12-22T14:29:28+07:00",
    "StockSymbol": "BUKA",
    "AlertType": "SINGLE_TRADE",
    "Action": "SELL",
    "TriggerPrice": 155,
    "TriggerVolumeLots": 35536,
    "TriggerValue": 550808000,
    "ZScore": 2.24,
    "VolumeVsAvgPct": 582.35,
    "AvgPrice": 157.25,
    "ConfidenceScore": 100,
    "MarketBoard": "RG"
  }
]
```

#### 3. Alert Statistics

Get aggregated statistics of whale activities within a time range.

**Endpoint:** `GET /api/whales/stats`

**Query Parameters:**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `symbol` | string | Filter by stock symbol (optional) |
| `start` | string | Start time (RFC3339) |
| `end` | string | End time (RFC3339) |

**Response Example:**

```json
{
  "stock_symbol": "ALL",
  "total_whale_trades": 6208,
  "total_whale_value": 1673531477400,
  "buy_volume_lots": 10000967,
  "sell_volume_lots": 11879432,
  "largest_trade_value": 102272625000
}
```

#### 4. Webhook Management

Manage webhooks for real-time notifications.

**List Webhooks:** `GET /api/config/webhooks`

**Response Example:**

```json
[
  {
    "ID": 1,
    "Name": "Discord Channel",
    "URL": "https://discord.com/api/webhooks/...",
    "Method": "POST",
    "IsActive": true,
    "TotalSent": 42
  }
]
```

**Create Webhook:** `POST /api/config/webhooks`

**Request Body:**

```json
{
  "name": "My Webhook",
  "url": "https://webhook.site/...",
  "method": "POST",
  "is_active": true
}
```

**Update Webhook:** `PUT /api/config/webhooks/{id}`

**Delete Webhook:** `DELETE /api/config/webhooks/{id}`

#### 5. Webhook Payload

When a whale is detected, your webhook URL will receive a `POST` request with this JSON body:

```json
{
  "trace_id": "whale-alert-1234567890",
  "type": "SINGLE_TRADE",
  "data": {
    "StockSymbol": "BBCA",
    "Action": "BUY",
    "TriggerPrice": 9850,
    "TriggerVolumeLots": 50000,
    "TriggerValue": 49250000000,
    "AvgPrice": 9800,
    "Message": "🐋 WHALE ALERT! BBCA BUY | Vol: 50000 (850% Avg) | Value: Rp 49.25M | Price: 9850 ...",
    "ZScore": 4.52,
    "VolumeVsAvgPct": 850.5,
    "DetectedAt": "2024-05-20T10:00:00Z"
  }
}
```

#### 5. SSE Real-time Events

Subscribe ke whale alerts real-time.

**Endpoint:** `GET /api/events`

**Response:** Server-Sent Events stream dengan whale alerts real-time

#### 6. Trading Strategy Signals

Mendapatkan sinyal strategi trading (VOLUME_BREAKOUT, MEAN_REVERSION, FAKEOUT_FILTER).

**REST Endpoint:** `GET /api/strategies/signals`

**SSE Streaming:** `GET /api/strategies/signals/stream`

**Query Parameters:**
| Parameter | Type | Description | Default |  
| :--- | :--- | :--- | :--- |
| `lookback` | int | Lookback period (menit) | 60 |
| `min_confidence` | float | Minimum confidence (0.0-1.0) | 0.3 |
| `strategy` | string | Filter by strategy type | ALL |

#### 7. Accumulation/Distribution Summary

Top 20 saham dengan akumulasi (BUY) dan distribusi (SELL) terbanyak.

**Endpoint:** `GET /api/accumulation-summary`

**Query Parameters:**
| Parameter | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| `hours` | int | Hours lookback | 24 |

**Response:** Dua list terpisah (accumulation & distribution) dengan statistik whale activity

### 🤖 LLM Pattern Analysis Endpoints

> **Note:** LLM endpoints require `LLM_ENABLED=true` in `.env` configuration

#### 6. Accumulation Pattern (Non-streaming)

Detect continuous buy/sell accumulation or distribution patterns.

**Endpoint:** `GET /api/patterns/accumulation`

**Query Parameters:**
| Parameter | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| `hours` | int | Hours to look back | 24 |
| `min_alerts` | int | Minimum alerts required | 3 |

#### 7. Accumulation Pattern Stream (SSE)

Real-time streaming analysis of accumulation patterns.

**Endpoint:** `GET /api/patterns/accumulation/stream`

**Query Parameters:** Same as non-streaming version

**Response:** Server-Sent Events stream with incremental analysis results

**Example Client (JavaScript):**

```javascript
const eventSource = new EventSource(
  "/api/patterns/accumulation/stream?hours=24"
);
eventSource.onmessage = (event) => {
  console.log("Chunk:", event.data);
};
```

#### 8. Extreme Anomalies (Non-streaming)

Identify extreme trading anomalies (Z-Score >= 4.0).

**Endpoint:** `GET /api/patterns/anomalies`

**Query Parameters:**
| Parameter | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| `hours` | int | Hours to look back | 24 |
| `min_zscore` | float | Minimum Z-Score threshold | 5.0 |

#### 9. Extreme Anomalies Stream (SSE)

Real-time streaming analysis of extreme anomalies.

**Endpoint:** `GET /api/patterns/anomalies/stream`

**Query Parameters:** Same as non-streaming version

#### 10. Time-based Statistics (Non-streaming)

Analyze whale activity patterns by time of day.

**Endpoint:** `GET /api/patterns/timing`

**Query Parameters:**
| Parameter | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| `days` | int | Days to look back | 7 |

#### 11. Time-based Statistics Stream (SSE)

Real-time streaming time-based analysis.

**Endpoint:** `GET /api/patterns/timing/stream`

**Query Parameters:** Same as non-streaming version

#### 12. Symbol-specific Analysis Stream (SSE)

Deep analysis for a specific stock symbol.

**Endpoint:** `GET /api/patterns/symbol/stream`

**Query Parameters:**
| Parameter | Type | Description | Required |
| :--- | :--- | :--- | :--- |
| `symbol` | string | Stock symbol (e.g., BBCA) | Yes |
| `hours` | int | Hours to look back | 24 |

## 🧠 Detection Logic (Pure Statistical)

The system uses a **configuration-free** statistical model to detect anomalies:

1.  **Statistical Baseline**: Calculates Mean & StdDev of Volume/Value over the last 60 minutes.
2.  **Safety Floor**: Ignores trades with Total Value < **100 Million IDR** (to filter noise).
3.  **Trigger Conditions**:
    - **Primary**: `Z-Score >= 3.0` (Statistically significant anomaly).
    - **Secondary**: `Volume >= 5x Average` (500% relative volume spike).
4.  **Fallback**: If no history exists (e.g., new listing), uses hard thresholds: (≥2500 Lots AND ≥100M IDR) OR ≥1 Billion IDR.

### 📐 Mathematical Formulation

#### 1. Statistical Baseline Calculation

For each stock symbol, the system computes rolling statistics over a **60-minute lookback window**:

```
μ_vol = Mean(Volume_lots)        // Average volume in lots
σ_vol = StdDev(Volume_lots)      // Standard deviation of volume
μ_price = Mean(Price)            // Average price
```

These statistics are cached in Redis for 5 minutes to optimize performance.

#### 2. Z-Score (Anomaly Detection)

The Z-Score measures how many standard deviations a trade's volume is away from the mean:

```
Z = (V_current - μ_vol) / σ_vol

Where:
  V_current = Current trade volume (in lots)
  μ_vol     = Mean volume over last 60 minutes
  σ_vol     = Standard deviation of volume
```

**Interpretation:**

- `Z >= 3.0` → **Exceptionally large** (99.7th percentile) - **WHALE ALERT**
- `Z >= 2.0` → Large trade (95th percentile)
- `Z < 2.0` → Normal activity

**Example:**

```
If μ_vol = 100 lots, σ_vol = 50 lots
Trade of 250 lots → Z = (250 - 100) / 50 = 3.0 ✅ ALERT!
```

#### 3. Volume vs Average Percentage

Relative volume spike indicator:

```
Vol% = (V_current / μ_vol) × 100

Alert if Vol% >= 500% (5x average)
```

**Example:**

```
If μ_vol = 100 lots
Trade of 600 lots → Vol% = (600 / 100) × 100 = 600% ✅ ALERT!
```

#### 4. Price Context (Delta from Average)

```
ΔPrice% = ((P_current - μ_price) / μ_price) × 100

Where:
  P_current = Current trade price
  μ_price   = Average price over 60 minutes
```

This is displayed but not used for triggering alerts (informational only).

#### 5. Combined Alert Logic

```python
def is_whale_alert(trade):
    # Step 1: Safety floor
    total_value = trade.price * trade.volume
    if total_value < 100_000_000:  # 100M IDR
        return False

    # Step 2: Get statistics (or use fallback)
    stats = get_stats(trade.symbol, lookback=60_min)

    if stats exists:
        # Primary: Z-Score check
        z_score = (trade.volume_lots - stats.mean_vol) / stats.stddev_vol
        if z_score >= 3.0:
            return True  # Anomaly detected

        # Secondary: Relative volume spike
        vol_pct = (trade.volume_lots / stats.mean_vol) * 100
        if vol_pct >= 500:
            return True  # 5x volume spike

        return False
    else:
        # Fallback for new/illiquid stocks (with safety floor)
        if total_value >= 100_000_000:  # 100M IDR minimum
            if trade.volume_lots >= 2500 or total_value >= 1_000_000_000:
                return True
        return False
```

### 📊 Example Detection Scenarios

| Scenario               | Volume (Lots) | Avg Volume | Z-Score | Vol% | Value (IDR) | Alert? | Reason            |
| ---------------------- | ------------- | ---------- | ------- | ---- | ----------- | ------ | ----------------- |
| **Normal Trade**       | 100           | 100        | 0.0     | 100% | 50M         | ❌     | Below Z-threshold |
| **Whale (Z-Score)**    | 500           | 100        | 3.2     | 500% | 250M        | ✅     | Z >= 3.0          |
| **Whale (Vol Spike)**  | 600           | 100        | 4.0     | 600% | 300M        | ✅     | Vol >= 5x         |
| **Small but Frequent** | 50            | 100        | -0.5    | 50%  | 25M         | ❌     | Below floor       |
| **New Stock Whale**    | 3000          | N/A        | N/A     | N/A  | 1.5B        | ✅     | Fallback ≥ 1B     |

### 🎯 Confidence Score

The system uses a **continuous confidence scoring** model with smooth mathematical progression:

#### 📐 Formula

**Z-Score Component:**

```
Confidence = 70 + (Z-Score - 3.0) × 15

Z = 3.0 → 70%  | Z = 4.0 → 85% | Z = 5.0+ → 100%
```

**Volume Bonus (up to +10%):**

```
If Volume% > 500%: Bonus = (Volume% - 500) / 50
```

#### 📊 Example Calculations

| Z-Score | Volume% | Base | Bonus | **Final** | Severity       |
| ------- | ------- | ---- | ----- | --------- | -------------- |
| 3.0     | 510%    | 70%  | +0.2% | **70%**   | 🟢 Threshold   |
| 3.5     | 600%    | 77%  | +2%   | **79%**   | 🟡 Significant |
| 4.0     | 750%    | 85%  | +5%   | **90%**   | 🟠 Very High   |
| 4.5     | 900%    | 92%  | +8%   | **100%**  | 🔴 Extreme     |
| 5.0+    | 1200%   | 100% | +10%  | **100%**  | 🔴 Extreme     |
| 2.5     | 600%    | 50%  | +2%   | **52%**   | 🔵 Vol Spike   |
| N/A     | N/A     | -    | -     | **40%**   | ⚪ Fallback    |

**Keuntungan:**

- ✅ Smooth progression (Z=3.1 ≠ Z=3.9)
- ✅ Precise signal strength
- ✅ Volume spike recognition
- ✅ Transparent formula

**Usage:**

- **≥85%**: Extreme whales, priority action
- **70-85%**: Strong signals for entry/exit
- **50-70%**: Moderate, needs confirmation
- **<50%**: Weak, watch only

## 🚀 Quick Start

### Prerequisites

- [Docker](https://www.docker.com/) dan [Docker Compose](https://docs.docker.com/compose/) terinstal.
- Akun [Stockbit](https://stockbit.com/) yang valid.
- (Opsional) LLM API Key untuk fitur analisis pattern AI.

### Installation

1.  **Clone the repository** (jika belum)

2.  **Setup Configuration**:
    Salin file environment example dan edit dengan kredensial Anda:

    ```bash
    make setup-env
    # Atau manual: cp .env.example .env
    ```

    Buka `.env` dan isi detail Anda:

    ```ini
    # Stockbit Credentials
    STOCKBIT_PLAYER_ID=your_id      # Required
    STOCKBIT_USERNAME=your_email    # Required
    STOCKBIT_PASSWORD=your_password # Required

    # WebSocket URL
    TRADING_WS_URL=wss://wss-trading.stockbit.com/ws

    # Database Configuration
    DB_HOST=timescaledb
    DB_PORT=5432
    DB_NAME=stockbit_trades
    DB_USER=stockbit
    DB_PASSWORD=stockbit123

    # Redis Configuration
    REDIS_HOST=redis
    REDIS_PORT=6379
    REDIS_PASSWORD=

    # LLM Pattern Recognition (Optional)
    LLM_ENABLED=true                              # Set false untuk disable AI
    LLM_ENDPOINT=https://api.openai.com/v1        # OpenAI atau compatible endpoint
    LLM_API_KEY=sk-your-api-key-here              # API key Anda
    LLM_MODEL=gpt-4o                               # Model yang digunakan
    ```

3.  **Run the Application**:
    Build dan start services (App + Database + Redis) di background:

    ```bash
    make build  # Build Docker image
    make up     # Start all services
    ```

4.  **View Logs**:
    Monitor application logs untuk memastikan semuanya berjalan:

    ```bash
    make logs
    ```

5.  **Access Web Interface**:
    Buka browser dan akses:
    ```
    http://localhost:8080
    ```

## 🛠️ Usage Commands

Using the included `Makefile`, you can easily manage the application:

| Command        | Description                                    |
| -------------- | ---------------------------------------------- |
| `make up`      | Start all services in the background           |
| `make down`    | Stop all services                              |
| `make logs`    | Follow real-time logs                          |
| `make restart` | Restart the application                        |
| `make build`   | Rebuild the Docker image                       |
| `make clean`   | **WARNING**: Stop services and delete ALL data |
| `make test`    | Run Go unit tests                              |

## 📂 Project Structure

```
stockbit-analysis/
├── app/                    # Main application logic and lifecycle
│   └── app.go             # Application bootstrap, WebSocket connection, handlers
├── api/                    # HTTP API server
│   ├── server.go          # REST API endpoints, SSE streaming, routing
│   └── server_symbol_handler.go  # Symbol-specific analysis handler
├── auth/                   # Authentication client
│   └── auth.go            # Stockbit login and token management
├── cache/                  # Redis caching layer
│   └── redis.go           # Redis client and operations
├── config/                 # Configuration management
│   └── config.go          # Environment variable loading
├── database/               # Database layer (TimescaleDB)
│   ├── connection.go      # Database connection setup
│   ├── models.go          # GORM models (Trade, Candle, WhaleAlert, etc.)
│   └── repository.go      # Database operations and queries
├── docs/                   # 📚 Dokumentasi lengkap
│   ├── ARCHITECTURE.md    # Arsitektur sistem dan komponen
│   ├── API.md             # Referensi API lengkap
│   ├── DEPLOYMENT.md      # Panduan deployment production
│   └── DOCUMENTATION.md   # Index navigasi dokumentasi
├── handlers/               # WebSocket message handlers
│   ├── base.go            # Base handler interface
│   ├── manager.go         # Handler registration and routing
│   └── running_trade.go   # Running trade processing and whale detection
├── helpers/                # Utility functions
│   └── currency.go        # Currency formatting helpers
├── llm/                    # LLM integration for pattern analysis
│   ├── client.go          # OpenAI-compatible LLM client (streaming support)
│   └── patterns.go        # Pattern detection prompts and logic
├── notifications/          # Notification system
│   └── webhook_manager.go # Webhook management and delivery
├── proto/                  # Protocol Buffers definitions
│   ├── datafeed.proto     # Protobuf schema for Stockbit WebSocket
│   └── datafeed.pb.go     # Generated Go code from proto
├── public/                 # Web interface static files
│   ├── index.html         # Main HTML page with AI panel
│   ├── script.js          # JavaScript for real-time updates and SSE
│   └── style.css          # Modern, responsive CSS styling
├── realtime/               # Real-time data broadcasting
│   └── broker.go          # SSE broker for real-time client updates
├── websocket/              # Stockbit WebSocket client
│   └── client.go          # WebSocket connection and protobuf handling
├── docker-compose.yml      # Container orchestration (App, DB, Redis)
├── Dockerfile             # App container definition
├── Makefile               # Development commands
├── .env.example           # Environment variables template
├── go.mod                 # Go module dependencies
└── README.md              # Dokumentasi utama (file ini)
```

## ⚙️ Configuration (.env)

| Variable             | Description                          | Default                             | Required        |
| -------------------- | ------------------------------------ | ----------------------------------- | --------------- |
| `STOCKBIT_USERNAME`  | Your Stockbit email/username         | -                                   | ✅              |
| `STOCKBIT_PASSWORD`  | Your Stockbit password               | -                                   | ✅              |
| `STOCKBIT_PLAYER_ID` | Your Stockbit player ID              | -                                   | ✅              |
| `TRADING_WS_URL`     | Stockbit Trading WebSocket URL       | `wss://wss-trading.stockbit.com/ws` | ✅              |
| `DB_HOST`            | Database hostname                    | `timescaledb`                       | ✅              |
| `DB_PORT`            | Database port                        | `5432`                              | ✅              |
| `DB_NAME`            | Database name                        | `stockbit_trades`                   | ✅              |
| `DB_USER`            | Database username                    | `stockbit`                          | ✅              |
| `DB_PASSWORD`        | Database password                    | `stockbit123`                       | ✅              |
| `REDIS_HOST`         | Redis hostname                       | `redis`                             | ✅              |
| `REDIS_PORT`         | Redis port                           | `6379`                              | ✅              |
| `REDIS_PASSWORD`     | Redis password (if any)              | -                                   | ❌              |
| `LLM_ENABLED`        | Enable LLM pattern analysis          | `false`                             | ❌              |
| `LLM_ENDPOINT`       | LLM API endpoint (OpenAI compatible) | `https://api.openai.com/v1`         | ⚠️ (if enabled) |
| `LLM_API_KEY`        | LLM API key                          | -                                   | ⚠️ (if enabled) |
| `LLM_MODEL`          | LLM model to use                     | `gpt-4o`                            | ⚠️ (if enabled) |

### LLM Configuration Notes

- **LLM_ENABLED**: Set to `true` untuk mengaktifkan fitur AI pattern analysis
- **LLM_ENDPOINT**: Mendukung OpenAI API dan endpoint compatible lainnya (mis: Azure OpenAI, local LLM server)
- **LLM_MODEL**: Model yang mendukung streaming response (GPT-4, GPT-3.5-turbo, atau model compatible)
- **Tanpa LLM**: Aplikasi tetap berfungsi penuh tanpa LLM, hanya fitur AI analysis yang tidak tersedia

## 🔍 Database

Produk menggunakan **TimescaleDB** (berbasis PostgreSQL) untuk performa tinggi pada time-series data.

### Tables

- **`trades`**: Running trades dengan hypertable, retention 3 bulan
- **`candles`**: OHLCV data yang di-aggregate otomatis ke bucket 1-menit menggunakan continuous aggregates
- **`whale_alerts`**: Whale detection results, retention 1 tahun
- **`whale_webhooks`**: Webhook configuration
- **`whale_webhook_logs`**: Webhook delivery logs untuk tracking

### Hypertable Features

- **Auto-partitioning**: Data otomatis di-partition berdasarkan waktu
- **Compression**: Historical data dikompres otomatis untuk efisiensi storage
- **Continuous Aggregates**: Real-time materialized views untuk OHLCV candles
- **Retention Policies**: Auto-cleanup data lama sesuai konfigurasi

## 🌐 Web Interface Usage

Setelah aplikasi berjalan, akses web dashboard di `http://localhost:8080`.

### Main Features

#### 📊 Statistics Ticker (Header)

- **Total Alerts**: Jumlah total whale alerts
- **Today's Volume**: Volume trading hari ini
- **Largest Value**: Nilai transaksi whale terbesar
- **Live Indicator**: Status koneksi real-time (hijau = connected)

#### 🔍 Filter Controls

1. **Search Symbol**: Cari berdasarkan kode saham (mis: BBCA, TLKM)
2. **Action Filter**: Filter BUY (Accumulation) atau SELL (Distribution)
3. **Minimum Value**: Filter berdasarkan nilai transaksi minimum
4. **Board Type**: Filter berdasarkan papan (RG/Regular, TN/Tunai, NG/Negosiasi)
5. **Refresh Button**: Manual refresh data

#### 🤖 AI Pattern Analysis Panel

Panel AI terdiri dari 4 mode analisis berbeda:

**1. 📈 Akumulasi (Accumulation)**

- Deteksi pola akumulasi/distribusi berkelanjutan
- Menampilkan saham dengan aktivitas whale berulang
- Analisis: Apakah whale sedang mengumpulkan atau mendistribusikan posisi

**2. ⚡ Anomali (Anomalies)**

- Deteksi anomali ekstrem (Z-Score >= 4.0)
- Transaksi dengan deviasi statistik sangat tinggi
- Potensi breaking news atau event signifikan

**3. ⏰ Timing (Time-based)**

- Analisis pola waktu aktivitas whale
- Jam-jam trading paling aktif
- Strategi timing entry/exit berdasarkan pola historis

**4. 🎯 Symbol (Symbol-specific)**

- Analisis mendalam untuk saham tertentu
- Masukkan kode saham (4 huruf, mis: BBCA)
- Riwayat whale activity dan prediksi

**Cara Menggunakan:**

1. Pilih salah satu tab mode analisis
2. (Untuk mode Symbol) Masukkan kode saham di input field
3. Klik tombol "▶ Start Analysis"
4. LLM akan mulai streaming hasil analisis real-time
5. Klik "⏹ Stop" untuk menghentikan analisis

#### 📋 Whale Alerts Table

Tabel menampilkan whale alerts real-time dengan kolom:

- **Time**: Waktu deteksi (WIB/local timezone)
- **Symbol**: Kode saham
- **Action**: BUY (hijau) atau SELL (merah)
- **Price**: Harga transaksi
- **Value**: Nilai total (IDR)
- **Volume**: Volume dalam lots
- **Details**: Informasi tambahan (Z-Score, % vs Average)

Data diupdate otomatis setiap deteksi whale baru.

## 🤝 Troubleshooting

### Common Issues

**❌ "Login Failed" loop**

- Verifikasi username dan password di `.env`
- Hapus cache token: `rm .token_cache.json` lalu `make restart`
- Pastikan credentials Stockbit valid

**❌ Database connection refused**

- Pastikan service `timescaledb` healthy: `docker-compose ps`
- Database butuh beberapa detik untuk initialize saat first run
- Cek logs: `docker-compose logs timescaledb`

**❌ Redis connection failed**

- Pastikan service `redis` running: `docker-compose ps`
- Cek port conflict: Pastikan port 6380 tidak digunakan aplikasi lain
- Restart Redis: `docker-compose restart redis`

**❌ WebSocket connection drops**

- Check network stability
- Lihat logs untuk error reconnection: `make logs`
- Aplikasi akan auto-reconnect dengan exponential backoff

**❌ LLM Analysis not working**

- Pastikan `LLM_ENABLED=true` di `.env`
- Verifikasi `LLM_API_KEY` valid
- Test endpoint: `curl -H "Authorization: Bearer $LLM_API_KEY" $LLM_ENDPOINT/models`
- Cek quota/limit API key Anda
- Lihat logs untuk error detail: `make logs`

**❌ No whale alerts detected**

- Normal jika market sedang sepi atau di luar jam trading
- Cek apakah WebSocket connected di logs
- Verifikasi data masuk: Query database `SELECT COUNT(*) FROM trades;`
- Threshold detection mungkin terlalu tinggi untuk kondisi market saat ini

**❌ Web interface not loading**

- Pastikan aplikasi running: `docker-compose ps`
- Cek port 8080 tidak digunakan: `lsof -i :8080` (Linux/Mac)
- Akses via IP jika localhost tidak work: `http://127.0.0.1:8080`
- Clear browser cache dan reload

### Performance Tuning

**Untuk high-frequency trading data:**

1. Increase PostgreSQL `shared_buffers` di docker-compose.yml
2. Adjust TimescaleDB chunk interval untuk data partitioning
3. Enable Redis persistence untuk production

**Untuk banyak concurrent users:**

1. Scale Redis dengan Redis Cluster
2. Add load balancer di depan multiple app instances
3. Use CDN untuk static files (`public/`)

## 🔧 Development

### Building from Source

```bash
# Clone repository
git clone <repository-url>
cd stockbit-analysis

# Install dependencies
go mod download

# Generate protobuf (if modified)
protoc --go_out=. --go_opt=paths=source_relative proto/datafeed.proto

# Run tests
go test ./...

# Build binary
go build -o stockbit-analysis .

# Run locally (requires PostgreSQL & Redis)
./stockbit-analysis
```

### Environment Variables for Local Development

```bash
# Database
DB_HOST=localhost
DB_PORT=5432
# ... other variables from .env.example
```

### Docker Development

```bash
# Build image
docker build -t stockbit-analysis:dev .

# Run with docker-compose
docker-compose up --build
```

## 📊 Monitoring & Observability

### Metrics & Logs

**Application Logs:**

```bash
# Follow all logs
make logs

# Specific service
docker-compose logs -f app
docker-compose logs -f timescaledb
docker-compose logs -f redis
```

**Database Queries:**

```sql
-- Connect to TimescaleDB
docker exec -it stockbit-timescaledb psql -U stockbit -d stockbit_trades

-- Check total trades
SELECT COUNT(*) FROM trades;

-- Check whale alerts today
SELECT COUNT(*) FROM whale_alerts WHERE detected_at >= CURRENT_DATE;

-- Top active stocks
SELECT stock_symbol, COUNT(*) as alert_count
FROM whale_alerts
WHERE detected_at >= NOW() - INTERVAL '24 hours'
GROUP BY stock_symbol
ORDER BY alert_count DESC
LIMIT 10;
```

**Redis Monitoring:**

```bash
# Connect to Redis
docker exec -it stockbit-redis redis-cli

# Check cached keys
KEYS *

# Monitor real-time commands
MONITOR
```

## 🛡️ Security Considerations

- **Credentials**: Jangan commit file `.env` ke repository
- **API Keys**: Gunakan environment variables, jangan hardcode
- **Webhooks**: Validate webhook URLs sebelum save
- **Database**: Use strong passwords untuk production
- **Network**: Expose hanya port yang diperlukan (8080 untuk web, block DB ports dari internet)

## 📚 Dokumentasi Lengkap

Dokumentasi detail tersedia di folder **`docs/`**:

- **[ARCHITECTURE.md](docs/ARCHITECTURE.md)** - Arsitektur sistem, komponen, dan alur data
- **[API.md](docs/API.md)** - Referensi lengkap semua endpoint API (REST & SSE)
- **[DEPLOYMENT.md](docs/DEPLOYMENT.md)** - Panduan deployment ke production
- **[DOCUMENTATION.md](docs/DOCUMENTATION.md)** - Index navigasi semua dokumentasi

### 🎯 Mulai dari mana?

- **Pengguna baru**: Baca README ini terlebih dahulu
- **Developer**: Lihat [ARCHITECTURE.md](docs/ARCHITECTURE.md) untuk memahami sistem
- **API consumers**: Buka [API.md](docs/API.md) untuk referensi endpoint
- **DevOps**: Ikuti [DEPLOYMENT.md](docs/DEPLOYMENT.md) untuk deploy

## 📝 License & Disclaimer

**Catatan Penting**: Tool ini untuk tujuan **edukasi dan riset**.

- Gunakan secara bertanggung jawab
- Tidak ada jaminan akurasi deteksi whale
- Keputusan trading adalah tanggung jawab Anda sendiri
- Pastikan compliance dengan regulasi OJK dan bursa efek

---

**Stockbit Analysis System** © 2025 - Built with ❤️ using Go, TimescaleDB, and AI

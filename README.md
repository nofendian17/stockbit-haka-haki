# 📈 Stockbit Analysis - Whale Detection & AI Insights

A high-performance, containerized Go application for real-time stock market analysis, whale detection, and AI-powered pattern recognition using Stockbit data.

## ✨ Key Features

- **🐋 Whale Detection**: Real-time statistical anomaly detection (Z-Score > 3.0) to identify institutional activity with follow-up tracking.
- **🧠 AI Insights**: Integrated LLM agent (OpenAI-compatible) with intelligent pre-filtering and regime-adaptive confidence thresholds.
- **📊 Signal History**: Persistent quality tracking with regime-aware performance metrics.
- **⚡ High Performance**:
    - **TimescaleDB**: Efficient storage of millions of trade records with optimized candle aggregation.
    - **Redis**: Low-latency caching for baselines, regime data, and LLM results.
    - **Go + SSE**: Concurrent processing and real-time streaming to frontend.
- **🔔 Notifications**: Webhook integration for Discord/Slack alerts.

## 🎯 Enhanced Signal Generation

### Multi-Layer Filtering Pipeline

```
Signal → RegimeFilter → StrategyPerformance → DynamicConfidence → OrderFlow → TimeOfDay → Position
           ↓                                                          ↓
      1.3x (trending)                                         1.5x (whale aligned)
      0.8x (ranging)                                          1.3x (strong buy)
      0.0x (volatile)                                         0.0x (whale divergence)
```

### Regime-Based Optimization

| Regime | Characteristics | LLM Threshold | Position Multiplier |
|--------|----------------|---------------|---------------------|
| **TRENDING_UP** | EMA slope > 0.5%, ATR < 2% | 0.5 (relaxed) | 1.3x (boost) |
| **RANGING** | EMA slope < 0.5%, ATR < 2% | 0.6 (default) | 0.8x (reduce) |
| **VOLATILE** | ATR > 2% | 0.75 (strict) | REJECT |
| **TRENDING_DOWN** | EMA slope < -0.5% | 0.7 (strict) | 0.7x (reduce) |

### Whale Alignment Validation

| Whale Activity | Our Signal | Action | Multiplier |
|----------------|-----------|--------|------------|
| 3+ BUY whales (>500M) | BUY | BOOST | 1.5x |
| BUY > SELL | BUY | BOOST | 1.3x |
| SELL > BUY (2+) | BUY | REJECT | 0.0x |

## 🧠 Logic at a Glance

| Feature | Threshold / Rule | Action |
| :--- | :--- | :--- |
| **Whale Detection** | Z-Score ≥ 3.0 **AND** Vol Spike ≥ 500% | 🚨 **ALERT** |
| **Regime Detection** | ATR-based with 5-min candles | 📊 **CLASSIFY** |
| **LLM Pre-filter** | Volume > 1000 lots, Value > 100M, Regime ≠ VOLATILE | 🤖 **ANALYZE** |
| **Volume Breakout** | Price > 2% **AND** Vol Z > 3.0 **AND** Trending | 🟢 **BUY** |
| **Whale Alignment** | 3+ BUY whales in 15min | 🐋 **BOOST 1.5x** |
| **Whale Divergence** | 2+ SELL whales vs BUY signal | ⛔ **REJECT** |
| **Stop Loss** | ATR-based (2× ATR) | 🔴 **CLOSE** |
| **Take Profit** | ATR-based (4× ATR TP1, 8× ATR TP2) | 💰 **CLOSE** |

## 🚀 Quick Start

1. **Setup Environment**:
   ```bash
   cp .env.example .env
   # Edit .env with your Stockbit credentials and LLM API key
   ```

2. **Configure Trading Parameters** (Optional):
   ```bash
   # Regime-adaptive thresholds
   TRADING_MIN_LLM_CONFIDENCE_TRENDING=0.5
   TRADING_MIN_LLM_CONFIDENCE_VOLATILE=0.75
   
   # LLM optimization
   TRADING_MIN_VOLUME_FOR_LLM=1000
   TRADING_MIN_VALUE_FOR_LLM=100000000
   TRADING_LLM_COOLDOWN_MINUTES=3
   ```

3. **Run with Docker**:
   ```bash
   make up
   ```

4. **Access Dashboard**:
   Open [http://localhost:8080](http://localhost:8080)

## 📊 Performance Metrics

### Expected Improvements (vs baseline)

| Metric | Improvement |
|--------|-------------|
| **LLM API Costs** | -25-30% |
| **Signal Win Rate** | +8-12% (50% → 58-62%) |
| **Profit Factor** | +33-75% (1.2 → 1.6-2.1) |
| **Max Drawdown** | -3% (-8% → -5%) |
| **Sharpe Ratio** | +38-75% (0.8 → 1.1-1.4) |

## 📚 Documentation

For detailed technical information, please refer to the `docs/` directory:

- **[API Reference](docs/API.md)**: Endpoints, Parameters, and Response formats.
- **[Architecture](docs/ARCHITECTURE.md)**: System design and component diagrams.
- **[Configuration](docs/CONFIGURATION.md)**: Environment variables and tuning guide.
- **[Deployment](docs/DEPLOYMENT.md)**: Configuration and production setup.

## 🛠️ Project Structure

```
.
├── api/            # REST API & SSE Handlers
├── app/            # Core Application Logic
│   ├── regime_detector.go      # Market regime classification (ATR-based)
│   ├── signal_tracker.go       # Signal outcome tracking
│   ├── signal_tracker_gen.go   # LLM-based signal generation
│   ├── signal_filter.go        # Multi-layer signal filtering
│   ├── exit_strategy.go        # ATR-based exit levels
│   └── whale_followup_tracker.go
├── cache/          # Redis Caching Layer
├── config/         # Configuration Management
├── database/       # TimescaleDB Models & Repositories
├── docs/           # Documentation
├── llm/            # AI Agent Integration
├── public/         # Frontend Web UI
├── realtime/       # Real-time Broadcast System
└── ...
```

## 🔧 Advanced Features

### Regime Detection
- **ATR Calculation**: 14-period Wilder's smoothing on 5-minute candles
- **Trend Classification**: EMA slope-based with 0.5% threshold
- **Volatility Measurement**: ATR percentage (>2% = high volatility)
- **Confidence Scoring**: Dynamic adjustment based on volatility

### LLM Optimization
- **Pre-filtering**: Skip volatile stocks, prioritize trending stocks
- **Dynamic Thresholds**: 0.5 (trending) to 0.75 (volatile)
- **Caching**: 5-minute TTL for analysis results
- **Cooldown**: 3-minute per-symbol to prevent excessive calls

### Signal Filtering
- **5-Layer Pipeline**: Regime → Strategy → Confidence → OrderFlow → TimeOfDay
- **Multiplier System**: Combined multipliers up to 2.8x for perfect signals
- **Whale Validation**: 15-minute window for institutional activity check
- **Auto-rejection**: Volatile regime or whale divergence

## 🎓 Monitoring

### Key Queries

Check regime distribution:
```sql
SELECT regime, COUNT(*), AVG(confidence)
FROM market_regimes
WHERE detected_at > NOW() - INTERVAL '1 hour'
GROUP BY regime;
```

Signal quality by regime:
```sql
SELECT mr.regime, 
       COUNT(so.id) as signals,
       ROUND(100.0 * SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END) / COUNT(so.id), 1) as win_rate
FROM signal_outcomes so
JOIN trading_signals ts ON so.signal_id = ts.id
LEFT JOIN market_regimes mr ON mr.stock_symbol = ts.stock_symbol
WHERE so.created_at > NOW() - INTERVAL '24 hours'
GROUP BY mr.regime;
```

## 🆕 Recent Updates

### Enhanced Signal Quality (v2.0)

We've significantly improved signal quality through stricter filtering and better risk management:

#### Stricter Entry Criteria
| Parameter | Before | After | Impact |
|-----------|--------|-------|--------|
| **Require Order Flow** | `false` | `true` | Must have order flow confirmation |
| **Buy Pressure Threshold** | 50% | 55% | Stronger buying confirmation |
| **Aggressive Buy Threshold** | 55% | 60% | Higher smart money requirement |
| **Min Baseline Samples** | 30 | 50 | More historical data required |
| **Low Win Rate Filter** | 40% | 45% | Faster rejection of underperforming strategies |
| **Confidence Threshold** | 0.50 | 0.55 | Higher signal quality |

#### Improved Risk Management
- **Daily Loss Limit**: Max 5% daily loss before trading stops
- **Circuit Breaker**: Stops after 3 consecutive losses
- **Breakeven Protection**: Triggers at 1% profit, moves stop to +0.15%
- **Fee-Aware Outcomes**: Accounts for 0.25% round-trip fees

#### Time-Based Filters
- **Skip First 15 Minutes**: Avoid 09:00-09:15 volatility
- **Pre-Lunch Caution**: No signals 11:30-12:00
- **Post-Lunch Wait**: Skip 13:30-13:45
- **Best Window**: Priority for 10:00-11:00 signals

Read more: [SIGNAL_IMPROVEMENTS.md](SIGNAL_IMPROVEMENTS.md)

---

### Swing Trading Support (NEW)

Hold positions overnight for larger profit potential!

#### Day Trading vs Swing Trading

| Feature | Day Trading | Swing Trading |
|---------|-------------|---------------|
| **Holding Period** | Max 4 hours | Max 30 days |
| **Auto-Close** | 16:00 WIB | ❌ No (hold overnight) |
| **Stop Loss** | 1.5× ATR | 4.5× Daily ATR |
| **Take Profit** | 3×/6× ATR | 9×/18× Daily ATR |
| **Min Confidence** | 0.55 | 0.75 |
| **Min History** | 50 samples | 400 samples (20 days) |
| **Trend Required** | Above VWAP | Strong trend (score > 0.6) |

#### Swing Trade Criteria
A signal qualifies as swing trade if:
1. ✅ Confidence ≥ 0.75
2. ✅ 20+ days of historical data
3. ✅ Trend score ≥ 0.6
4. ✅ Swing Score ≥ 0.65
   ```
   Swing Score = (Confidence × 0.4) + (Trend × 0.4) + (Volume × 0.2)
   ```

#### Configuration
```bash
# Enable Swing Trading
SWING_TRADING_ENABLED=true
SWING_MIN_CONFIDENCE=0.75
SWING_MAX_HOLDING_DAYS=30
SWING_ATR_MULTIPLIER=3.0
SWING_MIN_BASELINE_DAYS=20
SWING_POSITION_SIZE_PCT=5.0
SWING_REQUIRE_TREND=true
```

Read more: [SWING_TRADING.md](SWING_TRADING.md)

---

## 📊 API Reference

### Signal Statistics Endpoint
Debug signal flow and filtering:
```bash
GET /api/signals/stats?lookback=60
```

Response:
```json
{
  "total_signals": 50,
  "by_decision": {"BUY": 5, "WAIT": 20, "NO_TRADE": 25},
  "by_outcome_status": {"OPEN": 2, "SKIPPED": 45, "PENDING": 3},
  "truly_pending": 3
}
```

### Position Endpoints
- `GET /api/positions/open` - View open positions
- `GET /api/positions/history` - View closed positions with P&L
- `GET /api/signals/history` - Full signal history

### Analytics Endpoints
- `GET /api/analytics/strategy-effectiveness` - Performance by strategy
- `GET /api/analytics/optimal-thresholds` - Best confidence levels
- `GET /api/analytics/time-effectiveness` - Best trading hours
- `GET /api/analytics/expected-values` - EV calculations

---

## License

This project is for educational purposes only. Not for financial advice.

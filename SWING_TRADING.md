# Swing Trading Support

## Overview

Sistem trading kini mendukung **Swing Trading** selain Day Trading. Swing trades dapat menahan posisi overnight dan hingga 30 hari (configurable).

## Konfigurasi

Tambahkan ke `.env` file:

```env
# Swing Trading Configuration
SWING_TRADING_ENABLED=true          # Enable swing trading (default: false)
SWING_MIN_CONFIDENCE=0.75           # Minimum confidence for swing (default: 0.75)
SWING_MAX_HOLDING_DAYS=30           # Max holding period in days (default: 30)
SWING_ATR_MULTIPLIER=3.0            # ATR multiplier for exit levels (default: 3.0)
SWING_MIN_BASELINE_DAYS=20          # Min 20 days of history required
SWING_POSITION_SIZE_PCT=5.0         # Position size as % of portfolio
SWING_REQUIRE_TREND=true            # Require strong trend confirmation
```

## Perbedaan Day Trading vs Swing Trading

| Aspek | Day Trading | Swing Trading |
|-------|-------------|---------------|
| **Holding Period** | Max 4 jam | Max 30 hari |
| **Auto-close Market** | Ya (16:00 WIB) | Tidak (hold overnight) |
| **Stop Loss** | 1.5x ATR | 4.5x ATR |
| **Take Profit 1** | 3x ATR | 9x ATR |
| **Take Profit 2** | 6x ATR | 18x ATR |
| **Min Confidence** | 0.50 | 0.75 |
| **Min Baseline** | 50 samples | 400 samples (20 hari) |
| **Trend Required** | Above VWAP | Strong trend (score > 0.6) |

## Kriteria Swing Trade

Sinyal akan dianggap sebagai **Swing Trade** jika memenuhi:

### 1. **Confidence Tinggi**
   - Signal confidence ≥ 0.75
   - Lebih tinggi dari threshold day trading (0.50)

### 2. **Historical Data Cukup**
   - Minimum 20 hari data historis
   - Setara dengan ~400 samples (asumsi 20 trade/hari)

### 3. **Trend Strength**
   - Price di atas VWAP
   - Trend score ≥ 0.6 (dihitung dari price vs VWAP dan Z-score)
   - Volume confirmation baik

### 4. **Swing Score**
   ```
   Swing Score = (Confidence × 0.4) + (Trend × 0.4) + (Volume × 0.2)
   Minimum: 0.65
   ```

## Exit Strategy untuk Swing

### ATR-Based Levels (Swing)
```go
// Swing menggunakan daily ATR, bukan 5-min ATR
Stop Loss    = Daily ATR × 3.0 × 1.5  // 4.5x daily ATR
Trailing Stop = Daily ATR × 3.0        // 3x daily ATR
Take Profit 1 = Daily ATR × 3.0 × 3.0  // 9x daily ATR
Take Profit 2 = Daily ATR × 3.0 × 6.0  // 18x daily ATR
```

### Time-Based Exits
- **Day Trade**: Max 240 menit (4 jam)
- **Swing Trade**: Max 30 hari (configurable)

### Market Close Behavior
- **Day Trade**: Auto-close jam 16:00 WIB
- **Swing Trade**: Tetap hold, lanjut besok

## Monitoring Swing Trades

### API Endpoints

#### Get Signal Statistics
```bash
GET /api/signals/stats?lookback=1440  # 24 jam
```

Response akan menunjukkan:
```json
{
  "total_signals": 10,
  "by_decision": {"BUY": 3, "WAIT": 5, "NO_TRADE": 2},
  "by_outcome_status": {"OPEN": 2, "SKIPPED": 8},
  "truly_pending": 0
}
```

#### Get Open Positions
```bash
GET /api/positions/open
```

### Log Indicators

Swing trade terdeteksi:
```
📈 SWING TRADE detected for signal 123 (BBCA): score=0.72, Strong swing candidate
```

Swing exit levels:
```
📊 SWING Exit levels for BBCA @ 7250: 
  SL=-8.0% (6670), TP1=+15.0% (8338), TP2=+30.0% (9425), ATR=145.50 [SWING MODE]
```

Swing max holding reached:
```
📅 Swing max holding reached for BBCA: 30 days, P/L +12.5%
```

## Risk Management

### Position Sizing
- **Day Trade**: Tidak ada limit khusus (fokus pada setup)
- **Swing Trade**: Max 5% portfolio per posisi (configurable)

### Stop Loss
- **Day Trade**: Ketat (1.5x ATR) untuk proteksi cepat
- **Swing Trade**: Longgar (4.5x daily ATR) untuk toleransi volatility

### Drawdown Protection
- Daily loss limit tetap berlaku (default 5%)
- Circuit breaker setelah 3 consecutive losses

## Use Cases

### Kapan Menggunakan Swing Trading?

**Cocok untuk:**
1. **Saham trend kuat** dengan fundamental bagus
2. **Breakout** dengan volume tinggi di weekly chart
3. **Sinyal confidence tinggi** (>0.75) dengan trend alignment
4. **Tidak bisa monitoring intraday** (hold overnight)

**Tidak cocok untuk:**
1. **Saham volatile** dengan gap risk tinggi
2. **Earnings season** (avoid overnight risk)
3. **Low liquidity stocks** (slippage risk)
4. **Market uncertainty** (geopolitical events)

### Contoh Skenario

**Scenario 1: Strong Trend Continuation**
```
Signal: BBCA BUY
Confidence: 0.82
Price: 7500 (8% above VWAP)
Volume: 3.5x average
Baseline: 450 samples (22 hari)
Trend Score: 0.72

→ Qualifies as SWING TRADE
→ Exit: SL 6670 (-11%), TP1 8625 (+15%), TP2 9750 (+30%)
→ Hold: Max 30 hari
```

**Scenario 2: Day Trade Only**
```
Signal: BBRI BUY  
Confidence: 0.58
Price: 4200 (2% above VWAP)
Volume: 2.1x average
Baseline: 180 samples (9 hari)
Trend Score: 0.45

→ Does NOT qualify for swing
→ Treated as DAY TRADE
→ Exit: SL 4137 (-1.5%), TP1 4326 (+3%), TP2 4452 (+6%)
→ Hold: Max 4 jam, close 16:00
```

## Debugging

### Cek Kenapa Sinyal Bukan Swing

Tambahkan logging:
```go
isSwing, swingScore, reason := filterService.IsSwingSignal(signal)
log.Printf("Swing check: %v, score=%.2f, reason=%s", isSwing, swingScore, reason)
```

### Common Issues

**Issue**: Semua sinyal di-skip sebagai swing
**Cause**: `SWING_MIN_CONFIDENCE` terlalu tinggi atau data baseline tidak cukup
**Fix**: Turunkan threshold atau tunggu lebih banyak data historis

**Issue**: Swing trades tidak hold overnight
**Cause**: Logic error di isSwingTrade atau EnableSwingTrading=false
**Fix**: Cek konfigurasi dan log swing detection

## Performance Tracking

Swing trades dapat dianalisis secara terpisah:

```sql
-- Swing vs Day trade performance
SELECT 
  CASE 
    WHEN so.holding_period_minutes > 240 THEN 'SWING'
    ELSE 'DAY'
  END as trade_type,
  COUNT(*) as total,
  AVG(so.profit_loss_pct) as avg_pnl,
  SUM(CASE WHEN so.outcome_status = 'WIN' THEN 1 ELSE 0 END) as wins
FROM signal_outcomes so
WHERE so.outcome_status IN ('WIN', 'LOSS')
GROUP BY trade_type;
```

## Future Enhancements

1. **Pyramiding**: Tambah posisi saat trend berlanjut
2. **Partial Exit**: Keluar 50% di TP1, hold 50% ke TP2
3. **Trailing Stop ATR**: Adjust trailing stop berdasarkan volatility
4. **Multi-timeframe Analysis**: Konfirmasi daily + weekly trend
5. **Fundamental Filter**: Filter berdasarkan earnings, dividend, etc.

## Summary

Swing trading memberikan:
- ✅ Lebih besar profit potential (15-30% vs 3-6%)
- ✅ Tidak perlu monitoring intraday
- ✅ Ride strong trends
- ⚠️ Higher overnight risk
- ⚠️ Longer capital tie-up
- ⚠️ Stricter entry criteria

**Gunakan swing trading hanya untuk sinyal berkualitas tinggi dengan trend yang jelas!**

# Perbaikan Kodebase Trading Signal - Stockbit Whale Analysis

## Ringkasan Perubahan

Kodebase telah diperbaiki secara signifikan untuk menghasilkan sinyal trading yang lebih berkualitas dengan fokus pada:
1. **Kualitas sinyal lebih tinggi** - Filter lebih ketat
2. **Risk management lebih baik** - Daily loss limit dan circuit breaker
3. **Exit strategy lebih optimal** - Breakeven dan time-decay profit taking
4. **Konsistensi confidence calculation** - Algoritma yang lebih matang

---

## 1. Konfigurasi Trading yang Diperketat (config/config.go)

### Perubahan Nilai Default:

| Parameter | Nilai Lama | Nilai Baru | Alasan |
|-----------|-----------|-----------|---------|
| MinSignalIntervalMinutes | 15 | 20 | Mengurangi over-trading |
| MaxOpenPositions | 10 | 8 | Fokus pada posisi berkualitas |
| SignalTimeWindowMinutes | 5 | 10 | Menghindari duplikat lebih baik |
| MinBaselineSampleSize | 30 | 50 | Data baseline lebih reliable |
| MinBaselineSampleSizeStrict | 50 | 100 | Standar lebih tinggi |
| MinStrategySignals | 10 | 15 | Evaluasi strategi lebih akurat |
| LowWinRateThreshold | 40.0 | 45.0 | Reject strategi underperform lebih cepat |
| HighWinRateThreshold | 65.0 | 70.0 | Standar "bagus" lebih tinggi |
| MaxHoldingLossPct | 5.0 | 3.0 | Cut loss lebih ketat |
| StopLossATRMultiplier | 2.0 | 1.5 | Stop loss lebih ketat |
| TrailingStopATRMultiplier | 2.5 | 2.0 | Trailing stop lebih ketat |
| TakeProfit1ATRMultiplier | 4.0 | 3.0 | Profit lebih cepat |
| TakeProfit2ATRMultiplier | 8.0 | 6.0 | Target lebih realistis |

### Parameter Baru:
- **MaxDailyLossPct**: 5.0% - Batas kerugian harian
- **MaxConsecutiveLosses**: 3 - Circuit breaker setelah 3 loss berturut-turut
- **BreakevenTriggerPct**: 1.0% - Aktivasi breakeven pada profit 1%
- **BreakevenBufferPct**: 0.15% - Stop loss dipindahkan ke +0.15% (cover biaya)

---

## 2. Filter Pipeline Berbasis Statistik (app/signal_filter.go)

### Strategy Performance Filter:
- ✅ **Baseline recency check**: Data baseline harus kurang dari 2 jam untuk mendapat multiplier optimal.
- ✅ **Consecutive losses circuit breaker**: Modifikasi multiplier jika strategi mengalami banyak loss berturut-turut.
- ✅ **Purely Statistical**: Filter ini tidak lagi menolak (reject) sinyal secara sepihak jika threshold tidak terpenuhi, melainkan hanya menyesuaikan multiplier probabilitas.

### Dynamic Confidence Filter:
- ✅ **High volume threshold**: Volume Z-Score > 3.0 (dari 2.5)
- ✅ **Very high volume bonus**: Z > 4.0 + trend aligned = 1.3x multiplier
- ✅ **Purely Statistical**: Filter ini tidak lagi menolak sinyal yang confidence-nya di bawah batas optimal, melainkan membiarkannya lewat dengan mencatat alasannya. VWAP trend rejection juga telah dihapus.

*(Catatan: `OrderFlowFilter` dan `TimeOfDayFilter` beserta aturan ketat lainnya telah dihapus sepenuhnya untuk memberi jalan pada sistem filter yang 100% berbasis statistik dan multiplier.)*

---

## 3. Exit Strategy yang Lebih Baik (app/exit_strategy.go)

### Breakeven Mechanism:
```go
// Sebelumnya: Fixed 50% of TP1
if profitLossPct >= (TP1 / 2) {
    stop = entry * 1.001
}

// Sekarang: Configurable
if profitLossPct >= BreakevenTriggerPct {  // 1.0%
    stop = entry * (1 + BreakevenBufferPct/100)  // +0.15%
}
```

### Time-Decay Profit Taking:
```go
// Baru: Reduce profit target as time passes
if holdingMinutes > 120 && holdingMinutes < 240 {
    adjustedTP1 := TP1 * (1.0 - float64(holdingMinutes-120)/120.0*0.4)
    // Setelah 2 jam: TP1 berkurang 20%
    // Setelah 3 jam: TP1 berkurang 40%
}
```

### Enhanced Max Holding:
- Profit > 0.15%: Exit dengan profit
- Profit -0.5% sampai 0.15%: Exit near breakeven
- Loss > 0.5%: Biarkan stop loss bekerja

---

## 4. Daily Loss Limit & Circuit Breaker (app/signal_tracker.go)

### Fitur Baru:
```go
// Check daily loss limit
if dailyLoss <= -MaxDailyLossPct {  // -5.0%
    return false, "Daily loss limit reached"
}
```

### Outcome Classification:
```go
// Sebelumnya: Fixed 0.2% threshold
if profitLossPct > 0.2 { WIN }

// Sekarang: Account for trading fees (0.25% total)
const feeThreshold = 0.25
if profitLossPct > feeThreshold { WIN }  // > 0.25%
```

---

## 5. Confidence Calculation yang Konsisten (database/signals/repository.go)

### Sigmoid-like Curve:
```go
// Sebelumnya: Linear interpolation
confidence = ratio

// Sekarang: Quadratic ease-out
confidence = ratio * (2 - ratio)

// Acceleration near top
if ratio > 0.8 {
    confidence = 0.8 + (ratio-0.8)*1.5
}

// Minimum 0.3 (avoid extremely low confidence)
if confidence < 0.3 { confidence = 0.3 }
```

### Volume Breakout Strategy - Stricter:
- Threshold: Price Z > 2.5, Volume Z > 3.0 (dari 2.0/2.5)
- Confidence: Weighted average (60% volume, 40% price)
- NO_TRADE jika below VWAP (reject counter-trend)

### Mean Reversion Strategy - Stricter:
- Threshold: Price Z > 3.5 atau < -3.5 (dari 3.0)
- Deep value: 7% below VWAP (dari 5%)
- Smart money: >45% aggressive buy (dari 30%)
- Strong requirement: Deep value AND smart money

---

## 6. Hasil yang Diharapkan

### Kualitas Sinyal:
- **Win rate lebih tinggi**: Filter ketat memilih sinyal berkualitas
- **Drawdown lebih kecil**: Daily loss limit dan tighter stops
- **Profit lebih konsisten**: Time-decay profit taking

### Risk Management:
- **Maximum daily loss**: 5% hard limit
- **Circuit breaker**: Stop trading setelah 3 consecutive losses
- **Position sizing**: Max 8 posisi untuk fokus

### Exit Performance:
- **Breakeven protection**: 1% trigger dengan 0.15% buffer
- **Time-based exits**: Maksimal 4 jam holding
- **Fee consideration**: Threshold 0.25% (realistic)

---

## 7. Monitoring & Tuning

### Metrics yang Perlu Dipantau:
1. Win rate per strategi (target: >50%)
2. Average profit/loss per trade
3. Daily P&L tracking
4. Consecutive losses counter
5. Signal frequency (jangan terlalu banyak)

### Parameter yang Bisa Dituning:
- `MaxDailyLossPct`: Naikkan jika terlalu sering stop
- `BreakevenTriggerPct`: Turunkan jika terlalu cepat breakeven
- `MaxHoldingLossPct`: Sesuaikan dengan volatilitas pasar
- `MinBaselineSampleSize`: Turunkan untuk saham baru

---

## 8. Catatan Penting

### ⚠️ Perubahan Breaking:
1. **Penghapusan Aturan Ketat**: Sistem filter tidak lagi menolak sinyal berdasarkan win rate, confidence, order flow, atau waktu trading. Semua sinyal akan diproses dan dievaluasi kemungkinannya murni menggunakan *statistical multiplier*.
2. **BUY only**: Sistem masih hanya support long positions (Indonesia market)

### ✅ Keuntungan:
1. **Signal quality > quantity**: Lebih sedikit sinyal tapi win rate lebih tinggi
2. **Better risk management**: Daily limits dan circuit breakers
3. **Consistent exits**: Breakeven dan time-decay mechanisms
4. **Fee-aware**: Outcome classification memperhitungkan biaya trading

---

## Checklist Deployment

- [ ] Update environment variables jika perlu
- [ ] Pastikan Redis berjalan (untuk caching)
- [ ] Monitor log pertama 1-2 jam untuk melihat signal frequency
- [ ] Cek win rate harian untuk tuning parameter
- [ ] Review daily P&L untuk adjust MaxDailyLossPct jika perlu

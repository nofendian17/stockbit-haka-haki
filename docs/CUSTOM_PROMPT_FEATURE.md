# Custom Prompt Feature - AI Pattern Analysis

## Overview
Fitur Custom Prompt memungkinkan user untuk memberikan pertanyaan atau prompt sendiri ke AI berdasarkan data real-time di database. Fitur ini memberikan fleksibilitas penuh untuk analisis custom sesuai kebutuhan trading.

## Lokasi
Tab "✍️ Custom" di section **🤖 AI Pattern Analysis** pada dashboard.

## Cara Penggunaan

### 1. Akses Tab Custom
Klik tab "✍️ Custom" di AI Pattern Analysis section.

### 2. Masukkan Prompt
Ketik pertanyaan atau instruksi analisis Anda di textarea. Contoh:
- "Saham mana yang paling banyak akumulasi hari ini?"
- "Analisis pola BBCA dalam 24 jam terakhir"
- "Identifikasi saham dengan anomali Z-Score tertinggi dan berikan rekomendasi"
- "Bandingkan aktivitas whale di TLKM vs BBRI"

### 3. Konfigurasi Context (Opsional)

#### Symbols
Masukkan kode saham yang ingin dianalisis (pisahkan dengan koma):
```
BBCA,TLKM,BMRI
```
Kosongkan untuk analisis semua saham aktif.

#### Include Data
Pilih jenis data yang akan dimasukkan sebagai context:
- **Alerts + Regimes**: Whale alerts + kondisi pasar (default)
- **+ Patterns**: Tambah pola akumulasi/distribusi
- **+ Signals**: Tambah semua (alerts, regimes, patterns, signals)
- **Alerts Only**: Hanya whale alerts
- **Regimes Only**: Hanya market regime
- **Patterns Only**: Hanya pola akumulasi
- **Signals Only**: Hanya trading signals

#### Hours Back
Rentang waktu data yang akan dianalisis (1-168 jam). Default: 24 jam.

### 4. Start Analysis
Klik tombol "▶ Start Analysis" untuk memulai streaming AI response.

## Backend Implementation

### Endpoint
```
POST /api/patterns/custom/stream
```

### Request Body
```json
{
  "prompt": "Analisis pola BBCA dalam 24 jam terakhir",
  "symbols": ["BBCA", "TLKM"],  // opsional
  "hours_back": 24,
  "include_data": "alerts,regimes,patterns"
}
```

### Response
Server-Sent Events (SSE) stream dengan format:
```
data: [chunk of AI response]
data: [chunk of AI response]
...
event: done
data: Stream completed
```

### Context Building
Backend secara otomatis membangun context dari database berdasarkan `include_data`:

#### 1. Alerts (Whale Alerts)
```
=== WHALE ALERTS (Transaksi Besar) ===
- BBCA (BUY): Rp 1500.0M, Z-Score: 5.23, 15 menit lalu
- TLKM (SELL): Rp 800.5M, Z-Score: 4.12, 30 menit lalu
...
```

#### 2. Regimes (Market Regime)
```
=== MARKET REGIME (Kondisi Pasar) ===
- BBCA: TRENDING_UP (Confidence: 85%, Volatility: 2.34%)
- TLKM: RANGING (Confidence: 78%, Volatility: 1.89%)
...
```

#### 3. Patterns (Akumulasi/Distribusi)
```
=== POLA AKUMULASI/DISTRIBUSI ===
- BBCA (BUY): 12 alerts, Total: Rp 5400.0M, Avg Price: 9500, Z-Score: 4.56
- ASII (SELL): 8 alerts, Total: Rp 3200.0M, Avg Price: 5200, Z-Score: 3.89
...
```

#### 4. Signals (Trading Signals)
```
=== TRADING SIGNALS (AI) ===
- BBCA (VOLUME_BREAKOUT): BUY, Price: 9450, Confidence: 85%, Result: OPEN
- TLKM (MEAN_REVERSION): SELL, Price: 3200, Confidence: 72%, Result: WIN
...
```

## Use Cases

### 1. Analisis Saham Spesifik
**Prompt**: "Analisis lengkap BBCA: pola akumulasi, kondisi market, dan rekomendasi entry/exit"
**Config**: 
- Symbols: BBCA
- Include Data: + Signals
- Hours Back: 48

### 2. Screening Opportunities
**Prompt**: "Temukan 5 saham dengan potensi breakout tertinggi berdasarkan volume dan akumulasi"
**Config**:
- Symbols: (kosong - all stocks)
- Include Data: + Patterns
- Hours Back: 24

### 3. Risk Analysis
**Prompt**: "Identifikasi saham dengan anomali ekstrem yang berisiko tinggi"
**Config**:
- Symbols: (kosong)
- Include Data: Alerts + Regimes
- Hours Back: 12

### 4. Comparative Analysis
**Prompt**: "Bandingkan aktivitas institusi di sektor perbankan: BBCA, BBRI, BMRI"
**Config**:
- Symbols: BBCA,BBRI,BMRI
- Include Data: + Patterns
- Hours Back: 72

### 5. Strategy Validation
**Prompt**: "Evaluasi performa strategy VOLUME_BREAKOUT dalam 7 hari terakhir"
**Config**:
- Symbols: (kosong)
- Include Data: Signals Only
- Hours Back: 168

## Technical Details

### Frontend Components
- `/public/index.html`: UI untuk custom prompt tab
- `/public/js/app.js`: Logic untuk handle custom prompt
- `/public/js/sse-handler.js`: SSE streaming dengan fetch API

### Backend Components
- `/api/handlers_analytics.go`: `handleCustomPromptStream()` handler
- `/api/server.go`: Route registration
- `/llm/client.go`: LLM integration

### Data Flow
1. User input prompt + config di frontend
2. Frontend POST request ke `/api/patterns/custom/stream`
3. Backend build context dari database:
   - Query whale alerts (if selected)
   - Query market regimes (if selected)
   - Query accumulation patterns (if selected)
   - Query trading signals (if selected)
4. Backend format prompt: context + user question
5. Backend stream LLM response via SSE
6. Frontend render response dengan markdown

## Best Practices

### Writing Effective Prompts
1. **Spesifik**: Jelas tentang apa yang ingin dianalisis
2. **Actionable**: Minta rekomendasi atau insight yang bisa digunakan
3. **Kontekstual**: Manfaatkan data yang ada dengan baik
4. **Terstruktur**: Gunakan bullet points atau numbering untuk pertanyaan kompleks

### Optimizing Context
1. **Gunakan Symbols** untuk analisis focused
2. **Pilih Include Data** sesuai kebutuhan (jangan over-fetch)
3. **Adjust Hours Back** untuk trade-off antara konteks vs recency

### Example Prompts

#### Good Prompts ✅
```
"Berdasarkan whale alerts dan market regime, identifikasi 3 saham dengan setup akumulasi terbaik untuk entry besok"

"Analisis correlation antara volume spike dan profit di BBCA dalam 48 jam terakhir"

"Review trading signals yang hasil WIN vs LOSS, apa pattern yang membedakan?"
```

#### Bad Prompts ❌
```
"Apa itu saham?"  // Terlalu general, tidak memanfaatkan data

"BBCA bagus gak?"  // Tidak spesifik, tidak actionable

"Predict BBCA besok"  // AI tidak bisa predict future, hanya analisis data
```

## Limitations

1. **Data Availability**: Context terbatas pada data yang ada di database
2. **LLM Context Window**: Terlalu banyak data bisa melebihi context limit
3. **No Future Prediction**: AI hanya bisa analisis data historis/real-time
4. **Rate Limiting**: Tergantung pada LLM provider settings

## Security & Privacy

- Prompt tidak disimpan di database
- Data context hanya diambil dari database internal (tidak ada external data)
- LLM API key disimpan di server config (tidak exposed ke frontend)
- User hanya bisa akses data yang sudah ada di database

## Future Enhancements

- [ ] Prompt history/favorites
- [ ] Pre-built prompt templates
- [ ] Export analysis results
- [ ] Multi-turn conversation (chat mode)
- [ ] Advanced filtering (date range, board type, etc.)

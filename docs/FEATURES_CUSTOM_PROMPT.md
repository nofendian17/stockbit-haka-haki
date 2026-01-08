# 🤖 Custom Prompt Feature - Quick Guide

## Apa itu Custom Prompt?

Fitur baru di **AI Pattern Analysis** yang memungkinkan Anda bertanya APAPUN ke AI berdasarkan data real-time di database. Tidak perlu terbatas pada analisis preset - tanyakan sesuai kebutuhan trading Anda!

## 🎯 Kenapa Ini Powerful?

- ✅ **Fleksibel**: Tanya apa saja, tidak terbatas analisis preset
- ✅ **Data-Driven**: AI menjawab berdasarkan data REAL di database
- ✅ **Contextual**: Pilih data mana yang ingin dianalisis (alerts, regimes, patterns, signals)
- ✅ **Real-time**: Streaming response, bukan tunggu lama
- ✅ **Multi-stock**: Analisis 1 saham atau compare multiple stocks

## 🚀 Cara Pakai (3 Langkah)

### 1. Buka Tab Custom
Dashboard → AI Pattern Analysis → Tab "✍️ Custom"

### 2. Ketik Prompt
Contoh prompt bagus:
```
"Saham mana yang paling banyak akumulasi hari ini?"
"Analisis pola BBCA dalam 24 jam terakhir dan berikan rekomendasi"
"Compare aktivitas whale di BBCA vs TLKM vs BMRI"
"Temukan 5 saham dengan setup breakout terbaik"
```

### 3. Klik Start Analysis
AI akan streaming analisis real-time!

## 📊 Configuration Options

### Symbols (Opsional)
```
BBCA,TLKM,BMRI
```
Kosongkan untuk all stocks.

### Include Data
- **Alerts + Regimes** ← Default, cukup untuk most cases
- **+ Patterns** ← Add akumulasi/distribusi patterns
- **+ Signals** ← Add trading signals (comprehensive)

### Hours Back
24 jam (default) - 168 jam (1 minggu)

## 💡 Contoh Use Cases

### 1. Screening Opportunities
```
Prompt: "Temukan 5 saham dengan volume spike + akumulasi konsisten hari ini"
Config: All stocks, Include: alerts + patterns, 24 hours
```

### 2. Deep Dive Analysis
```
Prompt: "Analisis lengkap BBCA: whale activity, market regime, dan setup entry"
Symbols: BBCA
Config: Include: all data, 48 hours
```

### 3. Comparative Analysis
```
Prompt: "Bandingkan strength akumulasi di sektor banking: BBCA, BBRI, BMRI"
Symbols: BBCA,BBRI,BMRI
Config: Include: alerts + patterns, 72 hours
```

### 4. Risk Check
```
Prompt: "Saham mana yang ada anomali ekstrem dan berisiko reversal?"
Config: All stocks, Include: alerts + regimes, 12 hours
```

### 5. Strategy Review
```
Prompt: "Evaluasi performa VOLUME_BREAKOUT strategy minggu ini"
Config: All stocks, Include: signals only, 168 hours
```

## 🎓 Tips Menulis Prompt Efektif

### ✅ DO:
- Spesifik: "Analisis BBCA..." bukan "Gimana BBCA?"
- Actionable: "...dan berikan rekomendasi entry/exit"
- Kontekstual: Manfaatkan data yang ada
- Terstruktur: Gunakan numbering untuk multiple questions

### ❌ DON'T:
- Terlalu general: "Apa itu saham?"
- Prediksi masa depan: "BBCA naik berapa besok?"
- Tanpa context: "BBCA bagus gak?"

## 🔧 Technical Info

- **Endpoint**: `POST /api/patterns/custom/stream`
- **Method**: Server-Sent Events (SSE) streaming
- **LLM**: Requires `LLM_ENABLED=true` in config
- **Context Limit**: ~20 alerts, 15 signals, 10 patterns per request

## 📝 Example API Call

```bash
curl -X POST http://localhost:8080/api/patterns/custom/stream \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Analisis pola akumulasi BBCA hari ini",
    "symbols": ["BBCA"],
    "hours_back": 24,
    "include_data": "alerts,regimes,patterns"
  }'
```

## 🔗 Related Docs

- [Full Documentation](./CUSTOM_PROMPT_FEATURE.md)
- [API Reference](./API.md#custom-prompt-analysis-new)
- [Architecture](./ARCHITECTURE.md)

## 🆕 What's Next?

Future enhancements:
- Prompt templates (pre-built common queries)
- Prompt history
- Multi-turn conversation (chat mode)
- Export analysis results

---

**Pro Tip**: Start simple dengan default config, lalu customize sesuai kebutuhan. Most use cases cukup dengan "Alerts + Regimes" saja! 🎯

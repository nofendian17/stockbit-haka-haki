# 📈 Stockbit Analysis - Whale Detection & AI Insights

A high-performance, containerized Go application for real-time stock market analysis, whale detection, and AI-powered pattern recognition using Stockbit data.

## ✨ Key Features

- **🐋 Whale Detection**: Real-time statistical anomaly detection (Z-Score > 3.0) to identify institutional activity.
- **🧠 AI Insights**: Integrated LLM agent (OpenAI-compatible) to explain *why* a move is happening.
- **📊 Market Intelligence**:
    - **Order Flow**: Real-time HAKA vs HAKI (Buy vs Sell Aggression) analysis.
    - **Market Regimes**: Auto-classification of market state (Trending, Ranging).
    - **Signal History**: Persistent quality tracking of all generated trading signals.
- **⚡ High Performance**:
    - **TimescaleDB**: Efficient storage of millions of trade records.
    - **Redis**: Low-latency caching for real-time statistical baselines.
    - **Go + SSE**: Concurrent processing and real-time streaming to frontend.
- **🔔 Notifications**: Webhook integration for Discord/Slack alerts.

## 🧠 Logic at a Glance

| Feature | Threshold / Rule | Action |
| :--- | :--- | :--- |
| **Whale Detection** | Z-Score $\ge 3.0$ **AND** Vol Spike $\ge 500\%$ | 🚨 **ALERT** |
| **Volume Breakout** | Price $> 2\%$ **AND** Vol Z $> 3.0$ | 🟢 **BUY** |
| **Mean Reversion** | Price Z $< -4.0$ (Oversold) | 🟢 **BUY** |
| **Fakeout Filter** | Price Breakout **BUT** Weak Vol ($Z < 1$) | ⛔ **NO TRADE** |
| **Stop Loss** | Loss $\ge 2.0\%$ | 🔴 **CLOSE** |

## 🚀 Quick Start

1. **Setup Environment**:
   ```bash
   cp .env.example .env
   # Edit .env with your Stockbit credentials
   ```

2. **Run with Docker**:
   ```bash
   make up
   ```

3. **Access Dashboard**:
   Open [http://localhost:8080](http://localhost:8080)

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
├── database/       # TimescaleDB Models & Repositories
├── docs/           # Documentation
├── llm/            # AI Agent Integration
├── public/         # Frontend Web UI
├── realtime/       # Real-time Broadcast System
└── ...
```

## License

This project is for educational purposes only. Not for financial advice.

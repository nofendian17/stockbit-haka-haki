# ⚙️ Configuration Guide

This application is configured via environment variables, typically stored in a `.env` file. A sample configuration is provided in `.env.example`.

## Core Configuration

| Variable | Description | Required | Default |
| :--- | :--- | :--- | :--- |
| `STOCKBIT_PLAYER_ID` | Your Stockbit Player ID (from browser cookie) | Yes | - |
| `STOCKBIT_USERNAME` | Stockbit Email/Username | Yes | - |
| `STOCKBIT_PASSWORD` | Stockbit Password | Yes | - |
| `TRADING_WS_URL` | Stockbit Trading WebSocket URL | No | `wss://wss-trading.stockbit.com/ws` |

## Infrastructure

| Variable | Description | Default |
| :--- | :--- | :--- |
| `DB_HOST` | Database Host | `localhost` |
| `DB_PORT` | Database Port | `5432` |
| `REDIS_HOST` | Redis Host | `localhost` |
| `REDIS_PORT` | Redis Port | `6379` |

## 🤖 AI & LLM

| Variable | Description | Default |
| :--- | :--- | :--- |
| `LLM_ENABLED` | Enable LLM features | `false` |
| `LLM_ENDPOINT` | LLM API Endpoint | `https://ai.onehub.biz.id/v1` |
| `LLM_API_KEY` | LLM API Key | - |
| `LLM_MODEL` | Model Name | `qwen3-max` |

## 📈 Trading Logic Configuration (New)

The trading strategy parameters are now fully configurable without code changes.

### Position Management

| Variable | Description | Default |
| :--- | :--- | :--- |
| `TRADING_MIN_SIGNAL_INTERVAL` | Minimum minutes between signals for the same symbol | `15` |
| `TRADING_MAX_OPEN_POSITIONS` | Maximum global open positions allowed | `10` |
| `TRADING_MAX_POSITIONS_PER_SYMBOL` | Maximum open positions per symbol (no averaging down) | `1` |
| `TRADING_SIGNAL_TIME_WINDOW` | Time window (minutes) to check for duplicate signals | `5` |

### Entry Thresholds (Filters)

| Variable | Description | Default | Note |
| :--- | :--- | :--- | :--- |
| `TRADING_ORDER_FLOW_THRESHOLD` | Minimum Buy Pressure (0.0-1.0) required | `0.5` (50%) | Primary filter |
| `TRADING_AGGRESSIVE_BUY_THRESHOLD` | Minimum Aggressive Buy % at Offer | `60.0` | Used for override |
| `TRADING_REQUIRE_ORDER_FLOW` | Strict Mode for Bandarmology | `false` | If `true`, rejects signals when data missing |

### Exit Strategy (ATR Based)

| Variable | Description | Default | Multiplier of ATR |
| :--- | :--- | :--- | :--- |
| `TRADING_SL_ATR_MULT` | Initial Stop Loss distance | `1.5` | Stop Price = Entry - (ATR * 1.5) |
| `TRADING_TS_ATR_MULT` | Trailing Stop distance | `1.5` | |
| `TRADING_TP1_ATR_MULT` | Take Profit 1 distance | `3.0` | |
| `TRADING_TP2_ATR_MULT` | Take Profit 2 distance | `5.0` | |

### Risk Management

| Variable | Description | Default |
| :--- | :--- | :--- |
| `TRADING_MAX_HOLDING_LOSS_PCT` | Time-Based Cut Loss Percentage | `1.5` | Cuts loss if held > 60m and -1.5% |

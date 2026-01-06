# Deployment Guide

This guide covers how to deploy the Stockbit Analysis system in a Dockerized environment.

## Prerequisites

- **Docker** & **Docker Compose**
- **Make** (optional, for easy commands)
- **Stockbit Account** (ID, Username, Password)
- **OpenAI API Key** (Optional, for AI features)

## Configuration

1. **Copy the example environment file:**
   ```bash
   cp .env.example .env
   ```

2. **Edit `.env` with your credentials:**

   ```ini
   # Stockbit Credentials
   STOCKBIT_PLAYER_ID=your_id
   STOCKBIT_USERNAME=your_username
   STOCKBIT_PASSWORD=your_password
   
   # Database (TimescaleDB)
   DB_HOST=timescaledb
   DB_PORT=5432
   DB_NAME=stockbit_trades
   DB_USER=stockbit
   DB_PASSWORD=stockbit123
   
   # Redis
   REDIS_HOST=redis
   REDIS_PORT=6379
   
   # LLM / AI Configuration (Optional)
   LLM_ENABLED=true
   LLM_API_KEY=sk-proj-....
   LLM_MODEL=qwen3-max                       # Default: qwen3-max
   LLM_ENDPOINT=https://ai.onehub.biz.id/v1  # Default: https://ai.onehub.biz.id/v1
   ```

## Running with Docker Compose

The easiest way to run the entire stack (App, Database, Redis) is using `make`:

```bash
# Start everything in background
make up

# View logs
make logs
```

Or using plain Docker Compose:

```bash
docker-compose up -d
docker-compose logs -f
```

## Service Management

- **Restart**: `make restart`
- **Stop**: `make down`
- **Rebuild**: `make build`
- **Clean Reset** (Warning: Deletes Data): `make clean`

## Database Migrations

The application automatically handles database schema migrations on startup using GORM AutoMigrate.
However, TimescaleDB specifics (Hypertables, Continuous Aggregates) are initialized via SQL scripts located in `database/init.sql` (if applicable) or through code logic in `database/connection.go`.

## Production Considerations

1.  **Persistence**: Ensure Docker volumes (`postgres_data`, `redis_data`) are mapped to persistent storage.
2.  **Security**:
    - Change default `DB_PASSWORD`.
    - Put the service behind a Reverse Proxy (Nginx/Caddy) with SSL.
    - Do not expose port `8080` directly to the public internet without auth.
3.  **Resources**:
    - TimescaleDB can be memory intensive. Ensure the container has at least 2GB RAM for decent performance.
    - Adjust `shared_buffers` in PostgreSQL config if needed.

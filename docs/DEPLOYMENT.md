# 🚀 Panduan Deployment

Panduan lengkap untuk deploy Stockbit Analysis System ke berbagai environment.

## Daftar Isi

- [Environment Development](#environment-development)
- [Deployment Production](#deployment-production)
  - [Docker Compose (Rekomendasi)](#docker-compose-rekomendasi)
  - [Kubernetes](#kubernetes)
  - [Deployment Manual](#deployment-manual)
- [Konfigurasi](#konfigurasi)
- [Monitoring](#monitoring)
- [Backup & Recovery](#backup--recovery)
- [Troubleshooting](#troubleshooting)

---

## Environment Development

### Prerequisites

- Docker & Docker Compose
- Go 1.21+ (untuk development langsung)
- Git

### Quick Start

```bash
# Clone repository
git clone <repository-url>
cd stockbit-analysis

# Setup environment
make setup-env
# Edit .env dengan credentials Anda

# Start development environment
make build
make up

# View logs
make logs

# Access application
# API: http://localhost:8080
# Web UI: http://localhost:8080
```

### Development Tools

```bash
# Format code
go fmt ./...

# Run linters
go vet ./...

# Build binary
go build -o stockbit-analysis .

# Run tests
make test
```

---

## Production Deployment

### Docker Compose (Recommended)

#### Step 1: Prepare Server

**Minimum Requirements:**

- CPU: 2 cores
- RAM: 4GB
- Storage: 50GB SSD
- OS: Ubuntu 20.04+ / Debian 11+

**Install Docker:**

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Install Docker Compose
sudo apt install docker-compose -y

# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

#### Step 2: Deploy Application

```bash
# Clone repository
git clone <repository-url>
cd stockbit-analysis

# Create production .env
cp .env.example .env

# Edit .env dengan production values
nano .env
```

**Production .env Configuration:**

```ini
# Stockbit Credentials
STOCKBIT_PLAYER_ID=your_production_player_id
STOCKBIT_USERNAME=your_production_email
STOCKBIT_PASSWORD=your_secure_password

# Database (Change default passwords!)
DB_HOST=timescaledb
DB_PORT=5432
DB_NAME=stockbit_trades
DB_USER=stockbit
DB_PASSWORD=CHANGE_THIS_STRONG_PASSWORD

# Redis
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=CHANGE_THIS_REDIS_PASSWORD

# LLM (Optional)
LLM_ENABLED=true
LLM_ENDPOINT=https://api.openai.com/v1
LLM_API_KEY=sk-your-production-api-key
LLM_MODEL=gpt-4o
```

**Start Services:**

```bash
# Build and start
docker-compose up -d --build

# Check status
docker-compose ps

# View logs
docker-compose logs -f app
```

#### Step 3: Configure Reverse Proxy (Nginx)

**Install Nginx:**

```bash
sudo apt install nginx -y
```

**Create Nginx configuration:**

```bash
sudo nano /etc/nginx/sites-available/stockbit-analysis
```

**Nginx config:**

```nginx
server {
    listen 80;
    server_name your-domain.com;  # Replace with your domain

    # Redirect HTTP to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    # SSL Certificates (use Let's Encrypt)
    ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Gzip compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE support
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 86400s;
    }

    # Static files caching
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg)$ {
        proxy_pass http://localhost:8080;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
}
```

**Enable site:**

```bash
sudo ln -s /etc/nginx/sites-available/stockbit-analysis /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

**Install SSL Certificate (Let's Encrypt):**

```bash
sudo apt install certbot python3-certbot-nginx -y
sudo certbot --nginx -d your-domain.com
```

#### Step 4: Configure Firewall

```bash
# Allow SSH, HTTP, HTTPS
sudo ufw allow ssh
sudo ufw allow 'Nginx Full'
sudo ufw enable
sudo ufw status
```

#### Step 5: Setup Automated Backups

**Create backup script:**

```bash
sudo nano /usr/local/bin/backup-stockbit.sh
```

**Script content:**

```bash
#!/bin/bash
BACKUP_DIR="/var/backups/stockbit"
DATE=$(date +%Y%m%d_%H%M%S)

# Create backup directory
mkdir -p $BACKUP_DIR

# Backup database
docker exec stockbit-timescaledb pg_dump -U stockbit stockbit_trades | gzip > $BACKUP_DIR/stockbit_${DATE}.sql.gz

# Backup .env
cp /path/to/stockbit-analysis/.env $BACKUP_DIR/.env_${DATE}

# Keep only last 7 days
find $BACKUP_DIR -name "*.sql.gz" -mtime +7 -delete
find $BACKUP_DIR -name ".env_*" -mtime +7 -delete

echo "Backup completed: $DATE"
```

**Make executable:**

```bash
sudo chmod +x /usr/local/bin/backup-stockbit.sh
```

**Schedule daily backup:**

```bash
sudo crontab -e
```

**Add line:**

```
0 2 * * * /usr/local/bin/backup-stockbit.sh >> /var/log/stockbit-backup.log 2>&1
```

#### Step 6: Setup Monitoring

**Install Prometheus & Grafana (Optional):**

```bash
# Add to docker-compose.yml
# See monitoring section below
```

---

### Kubernetes

**Coming soon...**

For Kubernetes deployment, you'll need:

- Kubernetes cluster (GKE, EKS, AKS, or self-hosted)
- Helm charts for TimescaleDB and Redis
- Application deployment YAML
- Ingress controller (nginx-ingress)
- Cert-manager for SSL

---

### Manual Deployment

#### Prerequisites

- PostgreSQL 15+ with TimescaleDB extension
- Redis 7+
- Go 1.21+

#### Step 1: Install Dependencies

**PostgreSQL with TimescaleDB:**

```bash
sudo apt install postgresql-15 postgresql-15-timescaledb
sudo timescaledb-tune
sudo systemctl restart postgresql
```

**Redis:**

```bash
sudo apt install redis-server
sudo systemctl enable redis-server
sudo systemctl start redis-server
```

#### Step 2: Setup Database

```bash
# Create database
sudo -u postgres psql
CREATE DATABASE stockbit_trades;
CREATE USER stockbit WITH PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE stockbit_trades TO stockbit;
\c stockbit_trades
CREATE EXTENSION IF NOT EXISTS timescaledb;
\q
```

#### Step 3: Build Application

```bash
# Clone and build
git clone <repository-url>
cd stockbit-analysis
go mod download
go build -o stockbit-analysis .
```

#### Step 4: Configure Environment

```bash
# Create .env for production
cat > .env << EOF
STOCKBIT_USERNAME=your_email
STOCKBIT_PASSWORD=your_password
DB_HOST=localhost
DB_PORT=5432
DB_NAME=stockbit_trades
DB_USER=stockbit
DB_PASSWORD=secure_password
REDIS_HOST=localhost
REDIS_PORT=6379
# ... other variables
EOF
```

#### Step 5: Setup Systemd Service

```bash
sudo nano /etc/systemd/system/stockbit-analysis.service
```

**Service file:**

```ini
[Unit]
Description=Stockbit Analysis Service
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=stockbit
WorkingDirectory=/opt/stockbit-analysis
EnvironmentFile=/opt/stockbit-analysis/.env
ExecStart=/opt/stockbit-analysis/stockbit-analysis
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

**Enable and start:**

```bash
sudo systemctl daemon-reload
sudo systemctl enable stockbit-analysis
sudo systemctl start stockbit-analysis
sudo systemctl status stockbit-analysis
```

---

## Configuration

### Environment Variables

**Required:**

- `STOCKBIT_USERNAME`
- `STOCKBIT_PASSWORD`
- `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`
- `REDIS_HOST`, `REDIS_PORT`

**Optional:**

- `LLM_ENABLED` (default: false)
- `LLM_ENDPOINT`, `LLM_API_KEY`, `LLM_MODEL`

### Database Tuning

**PostgreSQL configuration** (`postgresql.conf`):

```
# Memory
shared_buffers = 2GB
effective_cache_size = 6GB
work_mem = 64MB
maintenance_work_mem = 512MB

# TimescaleDB
timescaledb.max_background_workers = 8
max_worker_processes = 16

# Checkpointing
checkpoint_completion_target = 0.9
wal_buffers = 16MB
```

**TimescaleDB specific:**

```sql
-- Adjust chunk interval (default: 7 days)
SELECT set_chunk_time_interval('trades', INTERVAL '3 days');

-- Enable compression (after 7 days)
ALTER TABLE trades SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_symbol'
);

SELECT add_compression_policy('trades', INTERVAL '7 days');
```

### Redis Configuration

**redis.conf:**

```
# Memory limit
maxmemory 1gb
maxmemory-policy allkeys-lru

# Persistence (optional)
save 900 1
save 300 10
save 60 10000

# Security
requirepass your_redis_password
```

---

## Monitoring

### Docker Compose Monitoring Stack

**Add to docker-compose.yml:**

```yaml
  prometheus:
    image: prom/prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus
    networks:
      - stockbit-network

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana_data:/var/lib/grafana
    networks:
      - stockbit-network

volumes:
  prometheus_data:
  grafana_data:
```

### Application Metrics

**Key metrics to monitor:**

- WebSocket connection status
- Database query performance
- Whale detection rate
- Webhook delivery success rate
- LLM API latency
- Memory and CPU usage

### Health Checks

**Endpoint:**

```bash
curl http://localhost:8080/health
```

**Expected response:**

```json
{ "status": "ok" }
```

**Docker health check:**

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
  interval: 30s
  timeout: 10s
  retries: 3
```

---

## Backup & Recovery

### Database Backup

**Manual backup:**

```bash
docker exec stockbit-timescaledb pg_dump -U stockbit stockbit_trades > backup.sql
```

**Restore:**

```bash
docker exec -i stockbit-timescaledb psql -U stockbit stockbit_trades < backup.sql
```

### Configuration Backup

```bash
# Backup .env and docker-compose.yml
tar -czf stockbit-config-backup.tar.gz .env docker-compose.yml
```

### Volume Backup

```bash
# Backup Docker volumes
docker run --rm -v stockbit-analysis_timescaledb_data:/data -v $(pwd):/backup ubuntu tar -czf /backup/timescaledb-data.tar.gz /data
```

### Automated Backup to S3

```bash
# Install AWS CLI
pip install awscli

# Backup script with S3 upload
#!/bin/bash
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="stockbit_${DATE}.sql.gz"

docker exec stockbit-timescaledb pg_dump -U stockbit stockbit_trades | gzip > $BACKUP_FILE
aws s3 cp $BACKUP_FILE s3://your-bucket/backups/
rm $BACKUP_FILE
```

---

## Scaling

### Horizontal Scaling

**Application:**

1. Deploy multiple app instances
2. Use load balancer (Nginx, HAProxy)
3. Shared Redis for session/cache

**Database:**

1. TimescaleDB clustering
2. Read replicas for queries
3. Master for writes

**Example nginx load balancer:**

```nginx
upstream stockbit_app {
    server 10.0.1.10:8080;
    server 10.0.1.11:8080;
    server 10.0.1.12:8080;
}

server {
    listen 80;
    location / {
        proxy_pass http://stockbit_app;
    }
}
```

### Vertical Scaling

- Increase container resources dalam docker-compose.yml
- Adjust database memory settings
- Increase Redis maxmemory

---

## Security Checklist

- [ ] Change default database passwords
- [ ] Set Redis password
- [ ] Use HTTPS (SSL certificates)
- [ ] Configure firewall (UFW/iptables)
- [ ] Secure .env file permissions (600)
- [ ] Enable Docker security scanning
- [ ] Regular security updates
- [ ] Implement rate limiting
- [ ] Configure CORS properly
- [ ] Backup encryption
- [ ] Secret management (Vault, AWS Secrets Manager)
- [ ] Network isolation
- [ ] Regular penetration testing

---

## Troubleshooting

### Application won't start

```bash
# Check logs
docker-compose logs app

# Check database connectivity
docker exec stockbit-app nc -zv timescaledb 5432

# Check Redis
docker exec stockbit-app nc -zv redis 6379
```

### High memory usage

```bash
# Check container stats
docker stats

# Adjust PostgreSQL shared_buffers
# Adjust Redis maxmemory
```

### WebSocket disconnections

```bash
# Check network stability
ping stockbit.com

# Review reconnection logs
docker-compose logs app | grep reconnect

# Adjust timeout settings
```

### Database performance issues

```sql
-- Check slow queries
SELECT query, mean_exec_time
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;

-- Vacuum and analyze
VACUUM ANALYZE;
```

---

## Maintenance

### Regular Tasks

**Daily:**

- Check application logs
- Monitor disk space
- Verify backups

**Weekly:**

- Review whale detection accuracy
- Check webhook delivery rates
- Database vacuum

**Monthly:**

- Update dependencies
- Security patches
- Performance review
- Backup restoration test

### Update Procedure

```bash
# Backup first!
./backup-stockbit.sh

# Pull latest changes
git pull origin main

# Rebuild and restart
docker-compose down
docker-compose up -d --build

# Verify
docker-compose ps
docker-compose logs -f app
```

---

## Support & Resources

- **Documentation**: [README.md](README.md), [ARCHITECTURE.md](ARCHITECTURE.md), [API.md](API.md)
- **Issues**: GitHub Issues
- **Monitoring**: Grafana dashboard at http://your-domain:3000

---

**Last Updated:** 2025-12-22

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o stockbit-analysis .

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS connections
RUN apk --no-cache add ca-certificates tzdata

# Set timezone to Asia/Jakarta
ENV TZ=Asia/Jakarta

# Create app directory
WORKDIR /app

# Create cache directory for token persistence
RUN mkdir -p /app/cache

# Copy binary from builder
COPY --from=builder /build/stockbit-analysis .
COPY --from=builder /build/public ./public

# Run as non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser && \
    chown -R appuser:appuser /app

USER appuser

# Run the application
CMD ["./stockbit-analysis"]

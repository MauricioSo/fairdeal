# Multi-stage Dockerfile for Fairdeal Protocol
# Can be deployed to Render (Docker Service), Railway, Fly.io, or GCP Cloud Run

FROM golang:1.26-alpine AS builder

WORKDIR /app

# Dependencies
COPY go.mod go.sum ./
RUN go mod download

# Source code & embedded assets
COPY . .
RUN CGO_ENABLED=0 go build -tags netgo -ldflags="-s -w" -o fairdeal .

# Minimal scratch/alpine container
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/fairdeal /app/fairdeal

# Data directory for persistent SQLite database
VOLUME ["/var/data"]

ENV PORT=8080 \
    DB_PATH=/var/data/fairdeal.db \
    PAYPAL_MODE=mock \
    DEMO_MODE=true \
    AI_MODE=llm \
    LLM_BASE_URL=https://api.deepseek.com \
    LLM_MODEL=deepseek-flash \
    KERNEL_API_URL=https://api.onkernel.com

EXPOSE 8080

CMD ["/app/fairdeal"]

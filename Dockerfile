# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o bill-bot ./cmd/bot

# Run stage
FROM alpine:latest
WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata && \
    cp /usr/share/zoneinfo/Asia/Taipei /etc/localtime && \
    echo "Asia/Taipei" > /etc/timezone

COPY --from=builder /app/bill-bot .

# config.yaml override (optional, mounted via volume)
VOLUME ["/app/data"]

ENV BOT_TOKEN=""
ENV DB_PATH="/app/data/bill-bot.db"

CMD ["./bill-bot"]

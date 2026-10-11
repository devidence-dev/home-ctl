# Build stage
FROM golang:1.27.2 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/homectl ./cmd/homectl

# Runtime stage
FROM alpine:3.24.2

RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=builder /app/homectl /app/homectl
COPY locales/ /app/locales/

# SQLite database lives under /app/data (default app.database_path), so it must be writable by app
RUN mkdir -p /app/data && chown -R app:app /app/data

USER app

ENTRYPOINT ["/app/homectl"]

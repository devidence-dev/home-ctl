# Build stage
FROM golang:1.27.1 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/homectl ./cmd/homectl

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/homectl /app/homectl
COPY locales/ /app/locales/

ENTRYPOINT ["/app/homectl"]

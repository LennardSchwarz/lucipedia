FROM golang:1.24-alpine AS builder
# build-base is necessary for CGO, which is needed by SQLite GORM driver
RUN apk add --no-cache --update build-base
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /app/bin/lucipedia ./cmd/server

FROM alpine AS server

WORKDIR /app

RUN apt-get update \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home /app appuser \
    && mkdir -p /app/data \
    && chown -R appuser:appuser /app

COPY --from=builder /app/bin/lucipedia /usr/local/bin/lucipedia

USER appuser

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/lucipedia"]
CMD []

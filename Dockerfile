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

COPY --from=builder /app/bin/lucipedia /usr/local/bin/lucipedia

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/lucipedia"]
CMD []

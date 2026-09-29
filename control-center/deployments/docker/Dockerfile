# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o anixops cmd/anixops/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o anixops-tui cmd/anixops-tui/main.go

# Runtime stage
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/anixops /app/anixops-tui /usr/local/bin/
COPY configs/config.yaml /etc/anixops/config.yaml

RUN mkdir -p /data

EXPOSE 8080 50052

ENTRYPOINT ["anixops"]
CMD ["server", "-c", "/etc/anixops/config.yaml"]
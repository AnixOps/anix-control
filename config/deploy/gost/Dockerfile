FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

RUN git clone --depth 1 -b main https://github.com/go-gost/gost.git . && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o gost ./cmd/gost

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/gost /app/gost
COPY gost.yml /app/gost.yml

EXPOSE 18080 9000

ENTRYPOINT ["./gost", "-C", "gost.yml"]
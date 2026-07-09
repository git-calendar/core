# Builder
FROM golang:alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ./ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /cors-proxy

# Final image
FROM alpine:latest
RUN apk --no-cache add ca-certificates curl

WORKDIR /app
COPY --from=builder /cors-proxy .

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=1s --retries=3 \
  CMD curl -f http://localhost:8080/ || exit 1

CMD ["./cors-proxy"]

# Builder
FROM golang:alpine as builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd/cors-proxy ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /cors-proxy

# Final image
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /cors-proxy .

EXPOSE 8080
CMD ["./cors-proxy"]

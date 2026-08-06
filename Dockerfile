# 构建阶段
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /esa-sync .

# 运行阶段
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /esa-sync /app/esa-sync
ENTRYPOINT ["/app/esa-sync"]
CMD ["-config", "/app/config.json"]

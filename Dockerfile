# ============================================================================
# KeyOps 集成 Dockerfile
# 将前端代码嵌入到 Go 二进制文件中，一个容器运行所有服务
# ============================================================================

# ---------- Stage 1: Build backend (Go) with embedded frontend ----------
FROM golang:1.23-alpine AS backend-builder
WORKDIR /build/backend

# 使用阿里云镜像源加速下载
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update

# 启用自动工具链下载
ENV GOTOOLCHAIN=auto
ENV GO111MODULE=on
# ENV GOPROXY=https://goproxy.cn

# Copy go modules first for better cache
COPY go.mod go.sum ./
RUN go mod download

# Copy backend sources
COPY . ./

# Copy frontend build output to embed directory
# Go embed 需要文件在编译时存在于文件系统中
# 从本地已编译好的 dist 目录拷贝
COPY ui/web/dist ./pkg/static/dist

# Build api-server binary (static) with embedded frontend
# 使用 embed_frontend build tag 启用前端嵌入
# ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -tags embed_frontend -o /out/keyops-api ./cmd/api-server

# ---------- Stage 2: Runtime (minimal alpine) ----------
FROM alpine:latest

WORKDIR /app

# 使用阿里云镜像源加速下载
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && \
    apk add --no-cache \
        ca-certificates \
        bash \
        tzdata \
        wget && \
    update-ca-certificates

# Copy backend binary (with embedded frontend)
COPY --from=backend-builder /out/keyops-api /usr/local/bin/keyops-api

# Create directories
RUN mkdir -p /app/config && \
    mkdir -p /app/logs

# Copy backend config (can be overridden by volume)
COPY config /app/config

# Expose ports: 8080 (HTTP/API + Frontend), 2222 (SSH gateway)
EXPOSE 8080 2222

ENV KEYOPS_CONFIG=/app/config/config.yaml \
    KEYOPS_ADDR_HTTP=:8080 \
    KEYOPS_ADDR_SSH=:2222


# Start backend (serves both API and frontend)
CMD ["/usr/local/bin/keyops-api"]

# 编译阶段：Go 版本与 go.mod 一致，缓存依赖下载。
FROM golang:1.26.2-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/campus-books . \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/seeduser ./cmd/seeduser

# 运行阶段：只包含程序、HTTPS证书和中国时区数据。
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /out/campus-books /app/campus-books
COPY --from=builder /out/seeduser /app/seeduser
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/app/campus-books"]

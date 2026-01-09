# 第一阶段：编译阶段
FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /app

# 设置代理加速下载（改用全球通用代理，适应 GitHub 美国服务器）
ENV GOPROXY=https://proxy.golang.org,direct

# 复制依赖文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源码并编译
COPY . .
# -ldflags="-s -w" 压缩体积，GOOS=linux 确保在服务器运行
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o mgaide-api .

# 第二阶段：运行阶段 (使用极小的 alpine 镜像)
FROM alpine:latest

# 安装基础证书（如果需要调用 https 接口）
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# 从编译阶段复制二进制文件
COPY --from=builder /app/mgaide-api .

# 暴露端口 (与代码中一致)
EXPOSE 3000

# 运行应用
CMD ["./mgaide-api"]

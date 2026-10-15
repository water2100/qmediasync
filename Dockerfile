# 构建阶段
FROM qicfan/qms-build-base:latest AS builder
# 设置时区
ENV TZ=Asia/Shanghai
# 设置国内镜像源
ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off \
    CGO_ENABLED=0

# 设置工作目录
WORKDIR /app
# 复制源代码
COPY . ./
# 设置构建参数
# 下载依赖
RUN go mod tidy
ARG TARGETARCH
ARG VERSION=v0.0.0
ARG BUILD_DATE=0000-00-00T00:00:00
ARG FANART_API_KEY
ARG DEFAULT_TMDB_ACCESS_TOKEN
ARG DEFAULT_TMDB_API_KEY
ARG DEFAULT_SC_API_KEY
ARG ENCRYPTION_KEY
# 构建可执行文件
RUN GOOS=linux GOARCH=${TARGETARCH} go build -ldflags "-s -w -X main.Version=${VERSION} -X 'main.PublishDate=${BUILD_DATE}' -X main.FANART_API_KEY=${FANART_API_KEY} -X main.DEFAULT_TMDB_ACCESS_TOKEN=${DEFAULT_TMDB_ACCESS_TOKEN} -X main.DEFAULT_TMDB_API_KEY=${DEFAULT_TMDB_API_KEY} -X main.DEFAULT_SC_API_KEY=${DEFAULT_SC_API_KEY} -X main.ENCRYPTION_KEY=${ENCRYPTION_KEY}" -o QMediaSync .

# 运行阶段
FROM qicfan/qms-build-base:latest
# 设置时区
ENV TZ=Asia/Shanghai

# 115 Cookie 下载/上传需要 p115client 的加解密与签名实现。
RUN apk add --no-cache python3 py3-pip && \
    python3 -m venv /opt/qms-python && \
    /opt/qms-python/bin/pip install --no-cache-dir 'p115client==0.0.9.6.5.1'
ENV PATH=/app:$PATH
ENV DB_HOST=localhost
ENV DB_PORT=5432
ENV DB_USER=qms
ENV DB_PASSWORD=qms123456
ENV DB_NAME=qms
ENV DB_SSLMODE=disable

# 添加用户和组（推荐方式）
RUN addgroup -g 12331 qms && \
    adduser -D -u 12331 -G qms qms
# 验证用户创建
RUN id qms
# 配置免密码 sudo
RUN echo 'qms ALL=(ALL) NOPASSWD: ALL' >> /etc/sudoers
# 创建共享内存目录
RUN mkdir -p /dev/shm && chmod 1777 /dev/shm
# 设置工作目录
WORKDIR /app
# # 从构建阶段复制文件
COPY --from=builder /app/QMediaSync .
COPY --from=builder /app/web_statics ./web_statics/
COPY --from=builder /app/scripts ./scripts/
RUN /opt/qms-python/bin/python -c "from p115client import P115Client, check_response" && \
    /opt/qms-python/bin/python -m py_compile /app/scripts/115_cookie_bridge.py
# COPY --from=builder /app/icon.ico .
# COPY QMediaSync .
# COPY web_statics ./web_statics/
# COPY scripts ./scripts/
# COPY icon.ico .
RUN chmod +x /app/scripts/docker-entrypoint.sh
RUN chmod +x /app/scripts/watch_update.sh
RUN chmod +x /app/QMediaSync

VOLUME ["/app/config", "/media"]
# web ui端口
EXPOSE 12333
# 代理emby端口http
EXPOSE 8095
# 代理emby端口https
EXPOSE 8094
# 启动命令
CMD ["/app/scripts/docker-entrypoint.sh"]

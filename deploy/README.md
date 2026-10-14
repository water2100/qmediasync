# VPS 部署

此目录用于在 VPS 上运行当前 Fork 的代码。QMediaSync 仅监听 `127.0.0.1:12333`，应由现有反向代理转发；PostgreSQL 没有宿主机端口映射。

## GitHub Actions 镜像部署

推送到 `main` 后，GitHub Actions 会构建 `linux/amd64` 镜像并发布到
`ghcr.io/water2100/qmediasync:latest`。首次发布后，请在 GitHub 的 Packages
页面确认该 Container package 对 VPS 可拉取。

在 VPS 的本项目目录执行：

```bash
git pull --ff-only
docker compose -f deploy/compose.ghcr.yaml pull qmediasync
docker compose -f deploy/compose.ghcr.yaml up -d
```

## 本地源码构建（备用）

1. 将 `.env.example` 复制为 `.env`，并为 `POSTGRES_PASSWORD` 设置强随机密码。
2. 在此目录运行 `docker compose up -d --build`。
3. 通过反向代理打开 QMediaSync 配置向导，选择 `PostgreSQL`、`外置数据库`，主机填写 `postgres`，端口 `5432`，数据库、用户与密码使用 `.env` 的对应值，并设置非默认管理员密码。

运行数据保存在 `data/`，其中 `data/config` 与 `data/postgres` 都应纳入备份；`data/media` 保存 STRM 与元数据。

## 更新与回滚

更新前先备份 `data/config` 和 `data/postgres`。使用 GitHub Actions 镜像部署时，执行上述 `pull` 与 `up -d` 命令即可；若需回滚，指定旧镜像标签后重新启动。

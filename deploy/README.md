# VPS 部署

此目录用于在 VPS 上运行当前 Fork 的代码。QMediaSync 仅监听 `127.0.0.1:12333`，应由现有反向代理转发；PostgreSQL 没有宿主机端口映射。

## 首次启动

1. 将 `.env.example` 复制为 `.env`，并为 `POSTGRES_PASSWORD` 设置强随机密码。
2. 在此目录运行 `docker compose up -d --build`。
3. 通过反向代理打开 QMediaSync 配置向导，选择 `PostgreSQL`、`外置数据库`，主机填写 `postgres`，端口 `5432`，数据库、用户与密码使用 `.env` 的对应值，并设置非默认管理员密码。

运行数据保存在 `data/`，其中 `data/config` 与 `data/postgres` 都应纳入备份；`data/media` 保存 STRM 与元数据。

## 更新与回滚

更新前先备份 `data/config` 和 `data/postgres`。更新代码后执行 `git pull --ff-only && docker compose up -d --build`。若需要回滚，切换到先前提交后重新构建即可。

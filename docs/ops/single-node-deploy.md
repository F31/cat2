# 单机首上线部署（4C8G）

> 目标：一台 4C8G 云主机承载 **API + worker/scheduler + PostgreSQL + Redis + 备份**，Nginx 负责 TLS 与后台静态页；面向初期小流量运营。
> 文件：`deploy/compose/docker-compose.single.yml`、`.env.single.example`、`deploy/nginx/catcube.conf`。

## 1. 机器与系统

- Ubuntu 22.04/24.04，4 vCPU / 8G / 80G+ SSD；公网带宽按初期流量 5Mbps 起。
- 安全组/防火墙：仅开放 `22`、`80`、`443`；**不开放** 5432/6379/8080。
- 安装 Docker Engine + Compose v2、Nginx、Certbot；建议加 2G swap 防瞬时内存峰值。
- 域名解析到该机器；完成 ICP 备案（国内）与小程序服务器域名白名单。

## 2. 获取代码与配置

```bash
git clone <repo> && cd cat2
cp deploy/compose/.env.single.example deploy/compose/.env.single
openssl rand -hex 32   # 生成两个密钥，分别填 IDENTITY_ENC_KEY / BLIND_KEY
vi deploy/compose/.env.single
chmod 600 deploy/compose/.env.single
```

必填：`POSTGRES_PASSWORD`、`REDIS_PASSWORD`、`IDENTITY_ENC_KEY`、`BLIND_KEY`、`WECHAT_APPID/SECRET`。
密码只用 `A-Z a-z 0-9`（避免连接串转义问题）。生产必须保持 `CATCUBE_SMS_DEV_CODE` 为空（compose 已固定为 `""`）。

## 3. 构建并启动

```bash
# 后端镜像（Dockerfile 使用 vendor，无需模块代理）
docker compose --env-file deploy/compose/.env.single \
  -f deploy/compose/docker-compose.single.yml build api worker

# 启动 DB/Redis/API/worker/备份
docker compose --env-file deploy/compose/.env.single \
  -f deploy/compose/docker-compose.single.yml up -d

# 查看健康
docker compose --env-file deploy/compose/.env.single \
  -f deploy/compose/docker-compose.single.yml ps
curl -s --noproxy '*' http://127.0.0.1:8080/readyz
```

首次启动 API 会自动执行迁移；也可显式执行：

```bash
docker compose --env-file deploy/compose/.env.single -f deploy/compose/docker-compose.single.yml \
  exec api catcube migrate
```

## 4. 后台前端与 Nginx

> 无域名/证书的初始部署：使用 `deploy/nginx/catcube-http.conf`（HTTP + Basic Auth，仅内测），
> 生成 `/etc/nginx/.htpasswd` 后启用；域名就绪后切换 `catcube.conf` + Certbot。
> 有 Basic Auth 时后台令牌走 `X-Admin-Token`（前端已适配），避免与 `Authorization` 冲突。

```bash
# 在 CI 或本机构建（Node 20 + pnpm）
pnpm install --frozen-lockfile
pnpm --filter admin-web build
sudo mkdir -p /var/www/catcube-admin
sudo cp -r apps/admin-web/dist/* /var/www/catcube-admin/

# Nginx
sudo cp deploy/nginx/catcube.conf /etc/nginx/sites-available/catcube.conf
# 修改 server_name 与证书路径后启用
sudo ln -s /etc/nginx/sites-available/catcube.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx

# 首次证书
sudo certbot --nginx -d admin.example.com
```

访问 `https://admin.example.com`，用手机号登录后，在服务器上授予超管：

```bash
docker compose --env-file deploy/compose/.env.single -f deploy/compose/docker-compose.single.yml \
  exec api catcube grant-admin 13800000000
```

> 注意：当前 admin-web 登录走开发态微信适配器（手机号当 code）。**若配置了真实 WECHAT_APPID/SECRET，后台登录会失败**；生产前需补后台账号体系（密码/SSO/短信），这是上公网前的阻塞项。

## 5. 备份与恢复

- `db` 开启 WAL 归档（`/wal-archive`），`backup` 边车每日全备到卷 `pgbackups`，保留 14 天。
- 备份脚本启动时会把 WAL/备份卷交给 `postgres` 用户（否则首次部署会因权限导致归档失败）。
- 卷位置：`docker volume inspect compose_pgbackups` / `compose_walarchive`。
- 恢复流程见 `deploy/compose/restore.md`；建议先在测试机演练，再每季度复跑 `pitr-drill.sh`。
- 建议把 `pgbackups`/`walarchive` 同步到对象存储（异地备份）。

## 6. 容量与调优

- 默认限额：PG 2G、API 1.5G、worker 1G、Redis 256M、backup 128M，合计约 5G，给系统/页缓存留 3G。
- 单 API 已验证并发 100（feed P95 60ms、search P99 19ms、detail P99 66ms，本机 S1 形态）；初期建议按 100~200 在线/日活观察 CPU、内存、P95。
- 若 CPU 持续 >70% 或 P95 超门槛：先升配到 8C16G，再考虑 API 拆出第二实例（改用 `docker-compose.s1.yml`）。
- 定期清理 Docker 日志（compose 已限制 20m×5）与旧备份。

## 7. 上线前阻塞（务必确认）

| 项 | 现状 | 影响 |
| --- | --- | --- |
| 后台登录 | 开发态手机号登录 | 配真实微信后无法登录；公网前必须补账号体系 |
| 微信支付 | 仅 DevClient | 真实支付不可用 |
| 实名核验 | 仅 DevClient | 真实实名不可用 |
| 短信 | 仅开发固定码 | 后台二次验证/通知短信不可用 |
| 内容安全 | 微信适配器未装配、图片异步 TODO | 生产内容合规不达标 |
| 对象存储 | 预签名/签名 TODO | 文件直传与 CDN 不可用 |
| 小程序审核/发版 | 外部 | 用户无法正式使用 |

**结论**：该单机编排适合“内测/试运行 + 生产基础设施预演”；在补齐上述适配器与后台登录前，不要直接对外收取真实资金。

## 8. 升级与回滚

```bash
# 拉代码/构建新镜像
docker compose --env-file deploy/compose/.env.single -f deploy/compose/docker-compose.single.yml build api worker
docker compose --env-file deploy/compose/.env.single -f deploy/compose/docker-compose.single.yml up -d --no-deps api worker

# 回滚：切回上一镜像 tag（建议每次发布打 tag），或 git checkout 上一版本后重建
```

迁移遵循 expand → migrate → contract；回滚前确认新迁移向后兼容。

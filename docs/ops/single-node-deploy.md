# 单机首上线部署（4C8G）

> 目标：一台 4C8G 云主机承载 **API + worker/scheduler + PostgreSQL + Redis + 备份**，Nginx 负责 TLS 与后台静态页；面向初期小流量运营。
> 文件：`deploy/compose/docker-compose.single.yml`、`.env.single.example`、`deploy/nginx/catcube.conf`。

## 1. 机器与系统

- Ubuntu 22.04/24.04，4 vCPU / 8G / 80G+ SSD；公网带宽按初期流量 5Mbps 起。
- 安全组/防火墙：仅开放 `22`、`80`、`443`；**不开放** 5432/6379/8080。
- 安装 Docker Engine + Compose v2、Nginx、acme.sh、socat；建议加 2G swap 防瞬时内存峰值。
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
真实收款还需：`WECHAT_PAY_MCHID`、`WECHAT_PAY_SERIAL_NO`、`WECHAT_PAY_API_V3_KEY`、`WECHAT_PAY_NOTIFY_URL`，以及挂载到 `deploy/compose/secrets/` 的商户私钥和微信支付平台证书。
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
# 首次导入字典/敏感词种子（幂等；区域/行业数据在 contracts/seeds/dict/*.json）
docker compose --env-file deploy/compose/.env.single -f deploy/compose/docker-compose.single.yml \
  exec api catcube seed
# 无应用容器时，也可直接用 SQL 初始化区域/行业（幂等）
docker exec -i compose-db-1 psql -U catcube -d catcube \
  < deploy/sql/init_dict_region_industry.sql
```

## 3.1 小程序接入信息（后台热配置）

登录后台 →「系统 → 小程序设置」可修改：

- `miniapp.appid`：小程序 AppID
- `miniapp.secret`：AppSecret（脱敏回显，仅超级管理员可写）
- `miniapp.env_version`：小程序码环境（`release`/`trial`/`develop`）

保存后立即生效：微信登录、手机号获取、小程序码都按新配置重建客户端；未配置时回退 env `CATCUBE_WECHAT_APPID/SECRET`，仍为空则使用开发态实现。

### 3.2 微信支付 V3（真实收款）

1. 将商户 API 私钥、微信支付平台证书放入 `deploy/compose/secrets/`（已 gitignore）：
   ```bash
   mkdir -p deploy/compose/secrets
   cp apiclient_key.pem deploy/compose/secrets/wechatpay_private_key.pem
   cp wechatpay_platform_cert.pem deploy/compose/secrets/wechatpay_platform_cert.pem
   chmod 600 deploy/compose/secrets/*.pem
   ```
2. 在 `deploy/compose/.env.single` 填写 `WECHAT_PAY_MCHID`、`WECHAT_PAY_SERIAL_NO`、`WECHAT_PAY_API_V3_KEY`、`WECHAT_PAY_NOTIFY_URL`，并保持两个 `_FILE` 路径指向 `/run/secrets/...`。
3. 回调地址必须为公网 HTTPS：`https://<域名>/api/mp/v1/pay/notify`，并在微信支付商户平台配置。
4. `CATCUBE_ENV=production` 时：未配置商户号会启用“支付未开通”占位（接口明确失败，不会回退可伪造回调）；配置了商户号但证书/密钥缺失会拒绝启动。开发/测试环境未配置时才使用 DevClient。

后台登录使用手机号 + 密码（`ADMIN_PHONE` / `ADMIN_PASSWORD_HASH`），不再依赖开发态微信登录。

## 4. 后台前端与 Nginx

> 无域名/证书的初始部署：使用 `deploy/nginx/catcube-http.conf`（HTTP，后台走应用层登录页）。
> 域名就绪后执行 `deploy/scripts/setup-tls.sh` 切换 `catcube.conf`。后台令牌走 `X-Admin-Token`，与标准 Bearer 兼容。
> 若 80 端口被腾讯云备案拦截，脚本使用 TLS-ALPN-01（443）签发，不依赖 HTTP-01；域名仍需完成 ICP 备案后才能对国内用户正常提供 HTTP 访问。

```bash
# 在 CI 或本机构建（Node 20 + pnpm）
pnpm install --frozen-lockfile
pnpm --filter admin-web build
sudo mkdir -p /var/www/catcube-admin
sudo cp -r apps/admin-web/dist/* /var/www/catcube-admin/

# 首次证书 + Nginx HTTPS（TLS-ALPN-01，80 被备案拦截时也可用）
sudo ACME_EMAIL=ops@example.com bash deploy/scripts/setup-tls.sh
```

访问 `https://cat2.mindbase.fun`，用手机号登录后，在服务器上授予超管：

```bash
docker compose --env-file deploy/compose/.env.single -f deploy/compose/docker-compose.single.yml \
  exec api catcube grant-admin 13800000000
```

> 后台登录走手机号 + 密码（`ADMIN_PHONE` / `ADMIN_PASSWORD_HASH`）；找回密码依赖短信通道，通道未接入前由运维通过 CLI/DB 重置。

> 域名已接入 EdgeOne/CDN 时，TLS-ALPN 或 HTTP-01 验证可能被 CDN 拦截。需先暂停该子域名的 CDN/代理，或改用 DNS-01。

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
| 后台登录 | 手机号 + 密码登录已实现 | 忘记密码需短信通道；未接入前走运维重置 |
| 微信支付 | V3 适配器已实现；生产未配置时明确禁用（不回退 DevClient） | 需配置商户号/证书/APIv3Key/回调 URL 并真实小额验证 |
| 实名核验 | 仅 DevClient | 真实实名不可用 |
| 短信 | 仅开发固定码 | 后台二次验证/通知短信不可用 |
| 内容安全 | 文本机检 + 图片异步回调已实现；生产强制 `MODERATION=wechat` | 配置内容安全权限和回调 token；媒体 URL 必须公网可访问 |
| 对象存储 | 阿里云 OSS V1 签名已实现；未配置时落 `/data/oss` 持久卷 | 需配置 bucket/AK/SK 并真实上传验证；非阿里云需适配 |
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

## 9. 实名认证（人工审核模式）

- 生产默认 `CATCUBE_REALNAME_MANUAL_REVIEW=true`：小程序上传身份证正反面（2 张）+ 姓名/身份证号，本地做格式、出生日期、GB 11643 校验位校验，进入平台待审。
- 运营/审核员在「内容管理 → 认证审核」查看材料并审批；通过后回写 `realname_status=1` 并授予实名 Badge，驳回回写 `2`。
- 开发/自动化测试可设 `CATCUBE_REALNAME_MANUAL_REVIEW=false` 继续使用 DevClient 直通；生产禁止关闭。
- 未配置 OSS 时材料落 `med_blob`（≤5MB/张）或本地卷 `/data/oss`；生产仍建议接 OSS/CDN。

## 10. 组织管理与资质材料

- 组织管理在「运营管理 → 组织管理」，六类主体：合作圈、商协会/校友会、大学/科研、服务机构、企业、商务人士。
- 组织认证审批需要 `verify:cert:review`；组织维护/成员辅助加入需要 `governance:org:manage`（默认授予运营角色，超管隐含全部）。
- 未配置对象存储时，上传落 `/data/oss` 持久卷并可由后台读取；导出下载走受令牌保护的 `/api/admin/v1/exports/{id}/raw`。生产仍建议接入 OSS。
- `catcube seed` 会导入六类主体配置；历史 `gov_service` 仅停用不删除。

#!/usr/bin/env bash
# 为 cat2.mindbase.fun 申请 Let's Encrypt 证书并启用 HTTPS。
# 80 端口受腾讯云备案拦截时 HTTP-01 不可用，因此使用 TLS-ALPN-01（443）。
# 用法：ACME_EMAIL=you@example.com sudo -E bash deploy/scripts/setup-tls.sh
set -euo pipefail

DOMAIN="${DOMAIN:-cat2.mindbase.fun}"
EMAIL="${ACME_EMAIL:-${CERTBOT_EMAIL:-}}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NGINX_CONF="/etc/nginx/sites-available/catcube.conf"
CERT_DIR="/etc/letsencrypt/live/$DOMAIN"
CRON_FILE="/etc/cron.d/acme.sh"

if [[ $EUID -ne 0 ]]; then
  echo "请用 root 或 sudo 执行" >&2
  exit 1
fi
if [[ -z "$EMAIL" ]]; then
  echo "请设置 ACME_EMAIL，例如：ACME_EMAIL=ops@example.com sudo -E bash $0" >&2
  exit 1
fi
command -v nginx >/dev/null || { echo "未安装 nginx" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "需要 systemd" >&2; exit 1; }

# 1. 安装 acme.sh 与 TLS-ALPN 所需的 socat
if ! command -v acme.sh >/dev/null || ! command -v socat >/dev/null; then
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y acme.sh socat
fi
command -v acme.sh >/dev/null || { echo "acme.sh 安装失败" >&2; exit 1; }
command -v socat >/dev/null || { echo "socat 安装失败" >&2; exit 1; }

install -d -m 755 /var/www/certbot
install -d -m 755 "$CERT_DIR"
ln -sf "$NGINX_CONF" /etc/nginx/sites-enabled/catcube.conf
rm -f /etc/nginx/sites-enabled/default

# 2. 无有效证书时先启用 HTTP 配置，再签发（RSA 2048 兼容旧安卓/小程序）
if ! openssl x509 -in "$CERT_DIR/fullchain.pem" -noout -checkend 86400 >/dev/null 2>&1; then
  install -m 644 "$REPO_ROOT/deploy/nginx/catcube-http.conf" "$NGINX_CONF"
  nginx -t
  systemctl reload-or-restart nginx

  acme.sh --issue --alpn --server letsencrypt --keylength 2048 -d "$DOMAIN" \
    --accountemail "$EMAIL" \
    --pre-hook "systemctl stop nginx" \
    --post-hook "systemctl start nginx" \
    --key-file "$CERT_DIR/privkey.pem" \
    --cert-file "$CERT_DIR/cert.pem" \
    --ca-file "$CERT_DIR/chain.pem" \
    --fullchain-file "$CERT_DIR/fullchain.pem" \
    --reloadcmd "systemctl start nginx"
else
  echo "证书已存在，跳过签发"
  acme.sh --cron --home /root/.acme.sh || true
fi

# 3. 自动续期（TLS-ALPN 续期前停止 Nginx，完成后启动）
cat > "$CRON_FILE" <<'EOF'
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
17 3 * * * root flock -n /run/acme.sh.lock /usr/bin/acme.sh --cron --home /root/.acme.sh >> /var/log/acme.sh.log 2>&1
EOF
chmod 644 "$CRON_FILE"

cat > /etc/logrotate.d/acme.sh <<'EOF'
/var/log/acme.sh.log {
  weekly
  rotate 8
  missingok
  notifempty
  compress
}
EOF
chmod 644 /etc/logrotate.d/acme.sh

if systemctl list-unit-files | grep -q '^certbot.timer'; then
  systemctl disable --now certbot.timer || true
fi

# 4. 启用 HTTPS 配置
install -m 644 "$REPO_ROOT/deploy/nginx/catcube.conf" "$NGINX_CONF"
nginx -t
systemctl reload-or-restart nginx
systemctl is-active --quiet nginx

# 5. 验证
acme.sh --info -d "$DOMAIN" | sed -n '1,20p' || true
openssl s_client -connect "127.0.0.1:443" -servername "$DOMAIN" </dev/null 2>/dev/null \
  | grep -E 'subject=|issuer=|Verify return code' || true
curl -fsS --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/readyz"

#!/bin/bash
# Debian 一键安装/升级。优先使用预构建包；systemd 自动重启并随开机启动。
set -euo pipefail
umask 077
ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
INSTALL_DIR="${INSTALL_DIR:-/opt/mingmiao-exchange}"
RELEASE_ARCHIVE="${1:-}"
# PID 文件可能被面板或失败的 nginx 启动清空，直接核对 /proc 中的实际 master。
# 返回 3 表示没有 nginx master；其他失败必须停止，不能再启动第二个 nginx。
find_existing_nginx_master() {
  local nginx_binary nginx_build default_conf='' net_namespace process_dir master_command executable config pid
  local -a matching_pids=() other_pids=()
  nginx_binary="$(readlink -f -- "$(command -v nginx)")" || return 1
  nginx_build="$(nginx -V 2>&1)" || return 1
  if [[ "$nginx_build" =~ --conf-path=([^[:space:]]+) ]]; then default_conf="${BASH_REMATCH[1]}"; fi
  net_namespace="$(readlink /proc/self/ns/net)" || return 1
  for process_dir in /proc/[0-9]*; do
    [[ "$(readlink "$process_dir/ns/net" 2>/dev/null)" == "$net_namespace" ]] || continue
    IFS= read -r -d '' master_command 2>/dev/null < "$process_dir/cmdline" || continue
    [[ "$master_command" == 'nginx: master process '* ]] || continue
    pid="${process_dir##*/}"
    executable="$(readlink "$process_dir/exe" 2>/dev/null)" || continue
    executable="${executable% (deleted)}"
    config="$default_conf"
    if [[ "$master_command" =~ [[:space:]]-c[[:space:]]+([^[:space:]]+) ]]; then
      config="${BASH_REMATCH[1]}"
    elif [[ "$master_command" =~ [[:space:]]-c([^[:space:]]+) ]]; then
      config="${BASH_REMATCH[1]}"
    fi
    if [[ "$executable" == "$nginx_binary" && "$config" == /* &&
          "$(readlink -f -- "$config" 2>/dev/null)" == "$(readlink -f /etc/nginx/nginx.conf)" ]]; then
      matching_pids+=("$pid")
    else
      other_pids+=("$pid")
    fi
  done
  if [[ ${#matching_pids[@]} -eq 0 && ${#other_pids[@]} -eq 0 ]]; then return 3; fi
  if [[ ${#matching_pids[@]} -ne 1 ]]; then
    echo "无法唯一确认 /etc/nginx/nginx.conf 对应的 master，匹配 PID：${matching_pids[*]:-无}；其他 PID：${other_pids[*]:-无}。请检查现有 nginx 管理方式。" >&2
    return 1
  fi
  pid="${matching_pids[0]}"
  # 信号发送前再次核对进程身份，不依赖可能为空或过期的 PID 文件。
  IFS= read -r -d '' master_command 2>/dev/null < "/proc/$pid/cmdline" || return 1
  executable="$(readlink "/proc/$pid/exe" 2>/dev/null)" || return 1
  [[ "$master_command" == 'nginx: master process '* && "${executable% (deleted)}" == "$nginx_binary" ]] || return 1
  printf '%s\n' "$pid"
}
reload_existing_nginx() {
  local pid
  pid="$(find_existing_nginx_master)" || return $?
  kill -HUP "$pid" || return 1
  echo "已向现有 nginx master（PID $pid）发送重载信号，保留其原有管理方式。"
}
# nginx 的连接槽与系统文件描述符是两套限制；只调 worker_connections 仍可能在 1024 处失败。
# 使用现有硬上限，不改变连接槽、CPU / 内存配置，也不重启或接管 nginx master。
configure_nginx_nofile() {
  local master='' status
  if master="$(find_existing_nginx_master)"; then :; else
    status=$?
    [[ $status -eq 3 ]] || return "$status"
  fi
  NGINX_NOFILE_BACKUP="$(python3 - "$master" <<'PY'
import os, pathlib, re, resource, shlex, shutil, signal, subprocess, sys, tempfile

descriptor = None
try:
    descriptor = os.pidfd_open(os.getpid())
    signal.pidfd_send_signal(descriptor, 0)
except (AttributeError, OSError):
    raise SystemExit('nginx 安全重载核验需要 Python 3.9+ 和支持 pidfd 的 Linux 5.3+；未修改配置。')
finally:
    if descriptor is not None: os.close(descriptor)
conf = pathlib.Path('/etc/nginx/nginx.conf').resolve()
original = conf.read_text()
dump = subprocess.check_output(['nginx', '-T'], stderr=subprocess.DEVNULL, text=True)
lexer = shlex.shlex(dump, posix=True, punctuation_chars='{};')
lexer.whitespace_split = True
tokens = list(lexer)
def numbers(name):
    return [int(tokens[i+1]) for i, token in enumerate(tokens[:-1]) if token == name and
            (i == 0 or tokens[i-1].endswith((';', '{', '}'))) and tokens[i+1].isdigit()]
connections = max(numbers('worker_connections') or [512])
configured = numbers('worker_rlimit_nofile')
hard = resource.getrlimit(resource.RLIMIT_NOFILE)[1]
if sys.argv[1]:
    limits = pathlib.Path('/proc/'+sys.argv[1]+'/limits').read_text()
    hard = int(re.search(r'^Max open files\s+\S+\s+(\d+)', limits, re.M)[1])
kernel_max = int(pathlib.Path('/proc/sys/fs/nr_open').read_text())
available = kernel_max if hard == resource.RLIM_INFINITY else min(hard, kernel_max)
target = max(available, *(int(v) for v in configured)) if configured else available
if target > kernel_max or target < connections + 64:
    raise SystemExit('nginx 文件描述符硬上限不足以覆盖连接槽及日志等文件；请先检查服务 LimitNOFILE 和内核 fs.nr_open。')
if configured and configured[0] >= target:
    print('nginx worker 文件描述符配置已覆盖连接槽，无需修改。', file=sys.stderr)
    sys.exit(0)
pattern = r'(?m)(^|[;{}])(\s*)worker_rlimit_nofile\s+\d+\s*;'
updated, count = re.subn(pattern, lambda m: m[1]+m[2]+'worker_rlimit_nofile '+str(target)+';', original)
if configured and count != 1:
    raise SystemExit('worker_rlimit_nofile 定义在外部 include；请在原文件调整，未修改任何配置。')
if not count:
    updated = '# 让 worker 使用现有文件描述符硬上限，避免 EMFILE 导致 TLS 握手失败。\nworker_rlimit_nofile '+str(target)+';\n'+original
metadata = conf.stat()
fd, candidate_name = tempfile.mkstemp(prefix='.mmex-nofile-', dir=conf.parent)
candidate = pathlib.Path(candidate_name)
try:
    with os.fdopen(fd, 'w') as output:
        output.write(updated)
    os.chmod(candidate, metadata.st_mode & 0o7777)
    os.chown(candidate, metadata.st_uid, metadata.st_gid)
    subprocess.run(['nginx', '-t', '-c', str(candidate)], check=True)
    if conf.read_text() != original:
        raise SystemExit('nginx.conf 在检查期间发生变化，未覆盖配置。')
    backup_dir = pathlib.Path('/var/backups/mingmiao-nginx')
    backup_dir.mkdir(mode=0o700, exist_ok=True)
    os.chmod(backup_dir, 0o700)
    backup_fd, backup_name = tempfile.mkstemp(prefix='nginx.conf.', dir=backup_dir)
    os.close(backup_fd)
    shutil.copy2(conf, backup_name)
    os.replace(candidate, conf)
    print(backup_name)
    print(f'nginx worker_rlimit_nofile={target}，worker_connections={connections}；备份：{backup_name}', file=sys.stderr)
finally:
    candidate.unlink(missing_ok=True)
PY
)" || return 1
}
snapshot_nginx_workers() {
  local master status
  if master="$(find_existing_nginx_master)"; then
    NGINX_WORKERS_BEFORE="$(cat "/proc/$master/task/$master/children")" || return 1
  else
    status=$?
    [[ $status -eq 3 ]] || return "$status"
    NGINX_WORKERS_BEFORE=''
  fi
}
verify_nginx_reload() {
  local master
  master="$(find_existing_nginx_master)" || return $?
  python3 - "$master" "${NGINX_WORKERS_BEFORE:-}" <<'PY'
import os, pathlib, re, shlex, signal, subprocess, sys, time

master = int(sys.argv[1])
previous = set(sys.argv[2].split())
dump = subprocess.check_output(['nginx', '-T'], stderr=subprocess.DEVNULL, text=True)
lexer = shlex.shlex(dump, posix=True, punctuation_chars='{};')
lexer.whitespace_split = True
tokens = list(lexer)
def numbers(name):
    return [int(tokens[i+1]) for i, token in enumerate(tokens[:-1]) if token == name and
            (i == 0 or tokens[i-1].endswith((';', '{', '}'))) and tokens[i+1].isdigit()]
connections = max(numbers('worker_connections') or [512])
configured = numbers('worker_rlimit_nofile')
required = max(connections + 64, *(int(v) for v in configured)) if configured else connections + 64
for attempt in range(50):
    workers = []
    for proc in pathlib.Path('/proc').iterdir():
        if not proc.name.isdigit() or proc.name in previous: continue
        try:
            command = (proc/'cmdline').read_bytes().split(b'\0')[0]
            parent = int((proc/'stat').read_text().split(') ', 1)[1].split()[1])
            if command != b'nginx: worker process' or parent != master: continue
            limit = re.search(r'^Max open files\s+(\d+)\s+(\d+)', (proc/'limits').read_text(), re.M)
            workers.append((proc.name, int(limit[1]), int(limit[2])))
        except (FileNotFoundError, ProcessLookupError):
            continue
    if workers and all(soft >= required for _, soft, _ in workers):
        # FD 耗尽可能损坏控制通道，使旧代收不到 master 的退出通知。
        # 新代就绪后，仅向同一 master 下仍接收请求的旧 worker 补发优雅退出信号。
        # pidfd 确保核对身份后 PID 被重用也不会误伤其他进程。
        for pid in previous:
            descriptor = None
            try:
                descriptor = os.pidfd_open(int(pid))
                proc = pathlib.Path('/proc/'+pid)
                if (proc/'cmdline').read_bytes().split(b'\0')[0] != b'nginx: worker process': continue
                if int((proc/'stat').read_text().split(') ', 1)[1].split()[1]) != master: continue
                # 部分容器禁止 root 读取其他 UID 的 /proc/<pid>/exe；父进程及标题已限定为该 master 的 worker。
                if (proc/'comm').read_text().strip() != 'nginx': continue
                signal.pidfd_send_signal(descriptor, signal.SIGQUIT)
            except (FileNotFoundError, ProcessLookupError):
                continue
            finally:
                if descriptor is not None: os.close(descriptor)
        print(f'已验证 nginx 活跃 worker 的文件描述符上限（要求至少 {required}）：{workers}')
        sys.exit(0)
    time.sleep(0.2)
raise SystemExit(f'nginx 活跃 worker 上限未生效：{workers}；检查 setrlimit 错误、master 权限和实际配置。')
PY
}
if [[ $EUID -ne 0 ]]; then echo '请使用 sudo bash deploy.sh [release.tar.gz]'; exit 1; fi
if [[ "$RELEASE_ARCHIVE" == --nginx-only ]]; then
  # 已安装服务器可单独修复 nginx；无需读取应用凭据、构建 release 或停止交易所。
  nginx -t
  find_existing_nginx_master >/dev/null || { echo '单独修复需要已运行且可确认的 nginx master。' >&2; exit 1; }
  configure_nginx_nofile
  snapshot_nginx_workers
  if ! reload_existing_nginx || ! verify_nginx_reload; then
    if [[ -n "$NGINX_NOFILE_BACKUP" ]]; then
      cp -p -- "$NGINX_NOFILE_BACKUP" /etc/nginx/nginx.conf
      nginx -t && reload_existing_nginx
      echo "已恢复修改前的 nginx 配置；备份：$NGINX_NOFILE_BACKUP" >&2
    fi
    exit 1
  fi
  echo 'nginx 文件描述符修复完成；请继续验收源站双栈 TLS、API、静态资源和公网链路。'
  exit 0
fi
if [[ ! -f "$ENV_FILE" ]]; then cp "$ROOT_DIR/.env.example" "$ENV_FILE"; chmod 600 "$ENV_FILE"; echo "已创建 $ENV_FILE，请填写密钥、密码及外部 TLS 证书路径后重新执行。"; exit 1; fi
# 只解析 KEY=VALUE，不执行环境文件中的 shell 命令。
while IFS= read -r env_line || [[ -n "$env_line" ]]; do
  env_line="${env_line%$'\r'}"
  [[ "$env_line" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || continue
  env_key="${BASH_REMATCH[1]}"; env_value="${BASH_REMATCH[2]}"
  if [[ "$env_value" == \"*\" || "$env_value" == \'*\' ]]; then env_value="${env_value:1:${#env_value}-2}"; fi
  export "$env_key=$env_value"
done < "$ENV_FILE"
for env_key in EXCHANGE_DOMAIN RECAPTCHA_SECRET_KEY ADMIN_PASSWORD SESSION_SECRET TLS_CERT_FILE TLS_KEY_FILE; do
  env_value="${!env_key:-}"
  if [[ -z "$env_value" || "$env_value" == replace-* ]]; then echo "请在 $ENV_FILE 配置 $env_key"; exit 1; fi
done
[[ ${MOCK_MODE:-false} != true ]] || { echo '生产部署禁止 MOCK_MODE=true'; exit 1; }
[[ "$EXCHANGE_DOMAIN" =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$ ]] || { echo 'EXCHANGE_DOMAIN 须为域名，不含协议或路径'; exit 1; }
[[ ${#ADMIN_PASSWORD} -ge 12 && ${#SESSION_SECRET} -ge 32 ]] || { echo '管理员密码至少 12 字符，会话密钥至少 32 字符'; exit 1; }
[[ "$INSTALL_DIR" == /opt/* && "$INSTALL_DIR" != /opt/ && "$INSTALL_DIR" != *'..'* && "$INSTALL_DIR" != *$'\n'* && "$INSTALL_DIR" != *' '* ]] || { echo '安装目录须为 /opt 下无空格的绝对路径'; exit 1; }
for tls_file in "$TLS_CERT_FILE" "$TLS_KEY_FILE"; do
  [[ "$tls_file" == /* && -f "$tls_file" && -r "$tls_file" && "$tls_file" != *[[:cntrl:]]* && "$tls_file" != *';'* && "$tls_file" != *'"'* && "$tls_file" != *'\'* ]] || { echo '必须提供可读的外部 TLS 证书及私钥绝对路径'; exit 1; }
done
# 旧版环境文件中的固定资源限制不再生效，构建和服务使用系统可用 CPU / 内存。
unset GOMAXPROCS GOMEMLIMIT
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y --no-install-recommends ca-certificates nginx curl openssl python3
# 先检查现有站点；面板重复 include 等错误必须在交易所停服、替换文件前处理。
nginx -t || { echo '现有 nginx 配置无效，尚未停服或替换交易所文件；请先修复上方报错后重试。'; exit 1; }
# 在停服或替换文件前验证证书有效期、域名和密钥配对，兼容 RSA / ECDSA。
openssl x509 -in "$TLS_CERT_FILE" -noout -checkend 0 >/dev/null || { echo 'TLS 证书无效或已过期'; exit 1; }
openssl x509 -in "$TLS_CERT_FILE" -noout -checkhost "$EXCHANGE_DOMAIN" >/dev/null || { echo 'TLS 证书不包含交易所域名'; exit 1; }
openssl pkey -in "$TLS_KEY_FILE" -passin pass: -noout >/dev/null 2>&1 || { echo 'TLS 私钥无效或需要密码'; exit 1; }
CERT_PUBLIC_HASH="$(openssl x509 -in "$TLS_CERT_FILE" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum)"
KEY_PUBLIC_HASH="$(openssl pkey -in "$TLS_KEY_FILE" -passin pass: -pubout -outform DER | sha256sum)"
[[ "$CERT_PUBLIC_HASH" == "$KEY_PUBLIC_HASH" ]] || { echo 'TLS 证书与私钥不匹配'; exit 1; }
configure_nginx_nofile
BUILD_DIR="$ROOT_DIR"
if [[ -n "$RELEASE_ARCHIVE" ]]; then
  BUILD_DIR="$(mktemp -d /tmp/mmex-release.XXXXXX)"
  # 拒绝绝对路径、父目录穿越和链接，解包不接受环境文件或数据库。
  python3 - "$RELEASE_ARCHIVE" "$BUILD_DIR" <<'PY'
import pathlib, sys, tarfile
with tarfile.open(sys.argv[1], 'r:gz') as archive:
    for member in archive.getmembers():
        path = pathlib.PurePosixPath(member.name)
        if path.is_absolute() or '..' in path.parts or member.issym() or member.islnk() or not (member.isdir() or member.isfile()):
            raise SystemExit('发布包路径无效')
        if not (member.name.startswith(('bin/', 'frontend/dist/')) or member.name in ('bin', 'frontend', 'frontend/dist', 'SHA256SUMS', './')):
            raise SystemExit('发布包包含非构建文件')
    archive.extractall(sys.argv[2])
PY
  (cd "$BUILD_DIR" && sha256sum -c SHA256SUMS)
fi
case "$(uname -m)" in x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) echo '暂支持 Debian amd64 / arm64'; exit 1;; esac
BINARY="$BUILD_DIR/bin/exchange-linux-$ARCH"
if [[ ! -f "$BINARY" || ! -f "$BUILD_DIR/frontend/dist/index.html" ]]; then
  echo '未找到预构建产物，将使用可用 CPU 在本机编译；小内存服务器推荐先运行 scripts/release.sh 生成发布包。'
  if ! command -v go >/dev/null; then
    apt-get install -y --no-install-recommends golang-go
  fi
  if ! go version | awk '{split($3,a,"."); sub("go","",a[1]); exit !(a[1]>1 || a[2]>=24)}'; then
    GO_VERSION="$(curl -fsSL 'https://go.dev/dl/?mode=json' | python3 -c 'import json,sys; print(json.load(sys.stdin)[0]["version"])')"
    GO_FILENAME="$GO_VERSION.linux-$ARCH.tar.gz"
    GO_SHA="$(curl -fsSL 'https://go.dev/dl/?mode=json' | python3 -c 'import json,sys; n=sys.argv[1]; print(next(f["sha256"] for r in json.load(sys.stdin) for f in r["files"] if f["filename"]==n))' "$GO_FILENAME")"
    GO_TMP="$(mktemp -d /tmp/mmex-go.XXXXXX)"
    curl -fsSL "https://go.dev/dl/$GO_FILENAME" -o "$GO_TMP/go.tgz"
    echo "$GO_SHA  $GO_TMP/go.tgz" | sha256sum -c -
    tar -xzf "$GO_TMP/go.tgz" -C "$GO_TMP"
    export PATH="$GO_TMP/go/bin:$PATH"
  fi
  apt-get install -y --no-install-recommends nodejs npm
  [[ $(node -p 'Number(process.versions.node.split(".")[0])') -ge 18 ]] || { echo '本机 Node 需 >=18，建议使用预构建发布包'; exit 1; }
  (cd "$ROOT_DIR/frontend" && npm ci --no-audit --no-fund && npm run build)
  mkdir -p "$ROOT_DIR/bin"
  (cd "$ROOT_DIR" && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$BINARY" ./cmd/exchange)
fi
id mmex >/dev/null 2>&1 || useradd --system --home "$INSTALL_DIR" --shell /usr/sbin/nologin mmex
install -d -m 755 "$INSTALL_DIR" "$INSTALL_DIR/frontend" "$INSTALL_DIR/frontend/dist"
install -d -o mmex -g mmex -m 700 "$INSTALL_DIR/data"
if [[ -f "$INSTALL_DIR/exchange" ]]; then
  systemctl daemon-reload
  systemctl stop mingmiao-exchange || true
  # 停服后数据库 WAL 一起备份，不覆盖已有数据。
  BACKUP_DIR="$INSTALL_DIR/data/backup-$(date -u +%Y%m%dT%H%M%SZ)"
  install -d -o mmex -g mmex -m 700 "$BACKUP_DIR"
  for db_file in "$INSTALL_DIR/data/exchange.db"*; do [[ -f "$db_file" ]] && cp -p "$db_file" "$BACKUP_DIR/"; done
fi
install -m 755 "$BINARY" "$INSTALL_DIR/exchange"
cp -R "$BUILD_DIR/frontend/dist/." "$INSTALL_DIR/frontend/dist/"
chmod -R a+rX "$INSTALL_DIR/frontend/dist"
# systemd EnvironmentFile 使用字面值，不执行 .env；保持服务器路径固定。
python3 - "$ENV_FILE" "$INSTALL_DIR/exchange.env" "$INSTALL_DIR" <<'PY'
import pathlib, sys
source, target, root = sys.argv[1:]
values = {}
for line in pathlib.Path(source).read_text().splitlines():
    if '=' not in line or line.lstrip().startswith('#'): continue
    k, v = line.split('=', 1)
    if not k.replace('_', '').isalnum() or not k.isupper(): continue
    if len(v) >= 2 and v[0] == v[-1] and v[0] in "\"'": v = v[1:-1]
    values[k] = v
for legacy in ('GOMAXPROCS', 'GOMEMLIMIT'): values.pop(legacy, None)
values.update(LISTEN_ADDR='127.0.0.1:8080', DATABASE_PATH=root+'/data/exchange.db', FRONTEND_DIR=root+'/frontend/dist', MOCK_MODE='false')
def quote(v): return '"'+v.replace('\\','\\\\').replace('"','\\"')+'"'
pathlib.Path(target).write_text(''.join(k+'='+quote(v)+'\n' for k,v in values.items()))
PY
chown root:mmex "$INSTALL_DIR/exchange.env"; chmod 640 "$INSTALL_DIR/exchange.env"
cat > /etc/systemd/system/mingmiao-exchange.service <<UNIT
[Unit]
Description=茗喵证券交易所 Go API
After=network.target
StartLimitIntervalSec=60
StartLimitBurst=10
[Service]
Type=simple
User=mmex
Group=mmex
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=$INSTALL_DIR/exchange.env
ExecStart=$INSTALL_DIR/exchange
Restart=always
RestartSec=3
TimeoutStopSec=15
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$INSTALL_DIR/data
LimitNOFILE=4096
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
# 仅信任 Cloudflare 公布的代理网段，恢复日志中的真实访问 IP。
CF_IP_FILE="$(mktemp /tmp/mmex-cloudflare.XXXXXX)"
curl -fsSL https://www.cloudflare.com/ips-v4 -o "$CF_IP_FILE"
printf '\n' >> "$CF_IP_FILE"
curl -fsSL https://www.cloudflare.com/ips-v6 >> "$CF_IP_FILE"
python3 - "$CF_IP_FILE" /etc/nginx/mingmiao-realip.conf <<'PY'
import ipaddress, pathlib, sys
networks = [ipaddress.ip_network(line.strip()) for line in pathlib.Path(sys.argv[1]).read_text().splitlines() if line.strip()]
if len(networks) < 10: raise SystemExit('Cloudflare 代理网段列表无效')
pathlib.Path(sys.argv[2]).write_text(''.join('set_real_ip_from '+str(n)+';\n' for n in networks)+'real_ip_header CF-Connecting-IP;\n')
PY
# 外部证书直接交给 nginx。IPv4 / IPv6 均监听，避免 IPv6 回源落到其他站点。
# 同步和交易请求直接转发，事件流关闭代理缓冲。
cat > /etc/nginx/sites-available/mingmiao-exchange <<NGINX
upstream mmex_api_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name $EXCHANGE_DOMAIN;
    ssl_certificate "$TLS_CERT_FILE";
    ssl_certificate_key "$TLS_KEY_FILE";
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:mmex_tls:1m;
    ssl_session_timeout 10m;
    include /etc/nginx/mingmiao-realip.conf;
    client_max_body_size 0;
    client_header_timeout 5s;
    client_body_timeout 12s;
    send_timeout 20s;
    keepalive_timeout 20s;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_connect_timeout 3s;
    proxy_read_timeout 20s;
    gzip on;
    gzip_min_length 1024;
    gzip_types application/json application/javascript text/css;
    location = /api/events {
        proxy_buffering off;
        proxy_read_timeout 60s;
        proxy_pass http://mmex_api_backend;
    }
    location /api/ {
        proxy_pass http://mmex_api_backend;
    }
    location / { proxy_pass http://mmex_api_backend; }
}
server {
    listen 80;
    listen [::]:80;
    server_name $EXCHANGE_DOMAIN;
    return 301 https://\$host\$request_uri;
}
NGINX
ln -sf /etc/nginx/sites-available/mingmiao-exchange /etc/nginx/sites-enabled/mingmiao-exchange
nginx -t
systemctl daemon-reload
systemctl enable mingmiao-exchange
systemctl restart mingmiao-exchange
# 已有 nginx 可能由面板启动或 PID 文件损坏，不启动第二个 master。
snapshot_nginx_workers
if systemctl is-active --quiet nginx && systemctl reload nginx; then
  systemctl enable nginx
elif reload_existing_nginx; then
  echo '请按原有 nginx 管理方式维护启动和保活；本脚本不会让 systemd 接管现有 master。'
else
  nginx_reload_status=$?
  if [[ $nginx_reload_status -eq 3 ]]; then
    systemctl enable --now nginx
  else
    echo '现有 nginx 重载失败，已停止部署；不会启动第二个 master。' >&2
    exit 1
  fi
fi
verify_nginx_reload
for attempt in {1..15}; do if curl -fsS http://127.0.0.1:8080/healthz >/dev/null; then break; fi; sleep 1; done
curl -fsS http://127.0.0.1:8080/healthz >/dev/null
# 验证运行中的后端确为本次发布的二进制，而不只是磁盘文件已替换。
EXCHANGE_PID="$(systemctl show -p MainPID --value mingmiao-exchange)"
[[ "$EXCHANGE_PID" =~ ^[1-9][0-9]*$ ]] && cmp -s "$BINARY" "/proc/$EXCHANGE_PID/exe" || { echo '运行中的后端与本次发布包不一致，请检查 mingmiao-exchange 服务。' >&2; exit 1; }
curl -fsS --max-time 5 http://127.0.0.1:8080/api/meta >/dev/null
# HUP 为异步重载；等待 IPv4 / IPv6 源站都返回本次安装的前端及有效 API。
# -k 仅用于本机源站探测，兼容非公共信任的 Origin CA 证书。
EXPECTED_FRONTEND_HASH="$(sha256sum "$BUILD_DIR/frontend/dist/index.html" | cut -d ' ' -f 1)"
verify_nginx_frontend() {
  local address frontend_hash
  for address in 127.0.0.1 '[::1]'; do
    frontend_hash="$(curl --noproxy '*' -kfSs --max-time 5 --resolve "$EXCHANGE_DOMAIN:443:$address" "https://$EXCHANGE_DOMAIN/console" | sha256sum | cut -d ' ' -f 1)" || return 1
    [[ "$frontend_hash" == "$EXPECTED_FRONTEND_HASH" ]] || return 1
    curl --noproxy '*' -kfSs --max-time 5 --resolve "$EXCHANGE_DOMAIN:443:$address" "https://$EXCHANGE_DOMAIN/api/meta" >/dev/null || return 1
  done
}
for attempt in {1..15}; do if verify_nginx_frontend 2>/dev/null; then break; fi; sleep 1; done
verify_nginx_frontend || { echo '源站页面/API 验证失败，请检查 nginx 重载、域名站点及 IPv4 / IPv6 路由。' >&2; exit 1; }
echo '已验证运行中的后端、IPv4 / IPv6 源站 API 和本次发布的控制台页面。'
echo "部署完成：https://$EXCHANGE_DOMAIN，控制台 https://$EXCHANGE_DOMAIN/console。Cloudflare 代理请选择 Full (strict)。"
echo '保活：systemctl status mingmiao-exchange；日志：journalctl -u mingmiao-exchange；更新：重新运行本脚本。'
echo '证书由外部维护；更新证书后执行 nginx -t && systemctl reload nginx。本脚本不会签发或续期。'

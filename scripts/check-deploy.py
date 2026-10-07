"""在临时 nginx 容器中验证部署前置校验和实际 nginx 配置，不修改宿主服务。"""

import atexit
import os
from pathlib import Path
import subprocess
import tempfile
import uuid


def main():
    root = Path(__file__).resolve().parent.parent
    script = (root / 'deploy.sh').read_text(encoding='utf-8')
    image = os.environ.get('NGINX_TEST_IMAGE', 'nginx:stable')
    subprocess.run(['docker', 'pull', image], check=True)
    # 部署前置校验及运行进程核验使用 Python；所有运行用例仍断网隔离。
    runtime_image = 'mmex-nginx-check:' + uuid.uuid4().hex
    subprocess.run(['docker', 'build', '--quiet', '-t', runtime_image, '-'],
                   input=f'FROM {image}\nRUN apt-get update -qq && apt-get install -y --no-install-recommends python3 && rm -rf /var/lib/apt/lists/*\n',
                   text=True, check=True)
    atexit.register(subprocess.run, ['docker', 'image', 'rm', runtime_image],
                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    image = runtime_image
    with tempfile.TemporaryDirectory(prefix='mmex-deploy-check-') as directory:
        fixture = Path(directory)

        def run(command, *, success=True, nofile=None):
            limits = ['--ulimit', 'nofile=' + nofile] if nofile else []
            container_name = 'mmex-deploy-check-' + uuid.uuid4().hex
            try:
                result = subprocess.run(
                    ['docker', 'run', '--rm', '--name', container_name, '--network=none', *limits,
                     '-v', f'{fixture}:/fixture', image, 'bash', '-c', command],
                    capture_output=True, text=True, encoding='utf-8', timeout=180,
                )
            except subprocess.TimeoutExpired:
                subprocess.run(['docker', 'rm', '-f', container_name], timeout=15,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                raise
            if (result.returncode == 0) != success:
                raise RuntimeError(result.stdout + result.stderr)
            return result.stdout + result.stderr

        run('openssl req -x509 -newkey rsa:2048 -nodes -days 1 '
            '-subj /CN=stock.nanoda.work -addext subjectAltName=DNS:stock.nanoda.work '
            '-keyout /fixture/key.pem -out /fixture/cert.pem 2>/dev/null && '
            'openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 '
            '-out /fixture/other-key.pem 2>/dev/null')
        stage = script[:script.index('BUILD_DIR=')]
        stage = stage.replace('umask 077', 'umask 077\napt-get() { :; }', 1)
        (fixture / 'validate.sh').write_text(stage+'\necho CERTIFICATE_VALIDATED\n', encoding='utf-8', newline='\n')
        values = dict(EXCHANGE_DOMAIN='stock.nanoda.work', RECAPTCHA_SECRET_KEY='test-private-key',
                      ADMIN_PASSWORD='deployment-test-password', SESSION_SECRET='s'*32,
                      TLS_CERT_FILE='/fixture/cert.pem', TLS_KEY_FILE='/fixture/key.pem')
        cases = [
            ('valid', {}, True, 'CERTIFICATE_VALIDATED'),
            ('missing-cert', {'TLS_CERT_FILE': ''}, False, 'TLS_CERT_FILE'),
            ('missing-file', {'TLS_KEY_FILE': '/fixture/missing.pem'}, False, '可读的外部 TLS'),
            ('wrong-domain', {'EXCHANGE_DOMAIN': 'wrong.example.com'}, False, '不包含交易所域名'),
            ('wrong-key', {'TLS_KEY_FILE': '/fixture/other-key.pem'}, False, '不匹配'),
        ]
        for name, override, success, expected in cases:
            (fixture / (name+'.env')).write_text(''.join(f'{k}={v}\n' for k,v in (values|override).items()), encoding='utf-8', newline='\n')
            result = run(f'ENV_FILE=/fixture/{name}.env bash /fixture/validate.sh', success=success)
            if expected not in result:
                raise AssertionError(f'{name}: {result}')
            print(f'{name}: PASS', flush=True)
        # 面板重复加载站点时，在证书校验和后续部署之前拒绝无效配置。
        (fixture / 'duplicate.conf').write_text(
            'limit_req_zone $binary_remote_addr zone=mmex_duplicate_check:1m rate=1r/s;\n' * 2,
            encoding='utf-8', newline='\n')
        result = run('cp /fixture/duplicate.conf /etc/nginx/conf.d/mmex-duplicate-check.conf && '
                     'ENV_FILE=/fixture/valid.env bash /fixture/validate.sh', success=False)
        if 'is already bound' not in result or '尚未停服或替换交易所文件' not in result or 'CERTIFICATE_VALIDATED' in result:
            raise AssertionError(result)
        print('nginx duplicate zone rejected before deployment: PASS', flush=True)
        start = script.index('cat > /etc/nginx/sites-available/mingmiao-exchange <<NGINX')
        end = script.index('\nNGINX', start)+len('\nNGINX')
        render = script[start:end].replace('/etc/nginx/sites-available/mingmiao-exchange', '/fixture/site.conf')
        render = render.replace('/etc/nginx/mingmiao-realip.conf', '/fixture/realip.conf')
        (fixture / 'render.sh').write_text(
            'set -eu\nEXCHANGE_DOMAIN=stock.nanoda.work\nTLS_CERT_FILE=/fixture/cert.pem\n'
            'TLS_KEY_FILE=/fixture/key.pem\n'+render+'\n', encoding='utf-8', newline='\n')
        (fixture / 'realip.conf').write_text('set_real_ip_from 127.0.0.1;\nreal_ip_header CF-Connecting-IP;\n', encoding='utf-8', newline='\n')
        (fixture / 'nginx.conf').write_text('events {}\nhttp { include /fixture/site.conf; }\n', encoding='utf-8', newline='\n')
        result = run('bash /fixture/render.sh && nginx -t -c /fixture/nginx.conf')
        if 'test is successful' not in result:
            raise AssertionError(result)
        print('nginx external TLS / keepalive / request protection: PASS', flush=True)

        # 使用真实 nginx 验证重载；systemctl 只模拟进程管理方式。
        helper_start = script.index('find_existing_nginx_master() {')
        helper_end = script.index('\nif [[ $EUID', helper_start)
        activation_start = script.index('# 已有 nginx 可能由面板启动')
        activation_end = script.index('\nfor attempt in', activation_start)
        verify_start = script.index('verify_nginx_frontend() {')
        verify_end = script.index('\nfor attempt in', verify_start)
        (fixture / 'control.sh').write_text('''set -euo pipefail
systemctl() {
    printf '%s\\n' "$*" >> /fixture/control-calls.log
    case "$1" in
        is-active) [[ "$MOCK_MANAGED" == true ]];;
        reload) nginx -s reload;;
        enable) if [[ "$2" == --now ]]; then nginx -c /etc/nginx/nginx.conf; fi;;
        *) return 1;;
    esac
}
''' + script[helper_start:helper_end] + '\n' + script[activation_start:activation_end] + '''
EXCHANGE_DOMAIN=stock.nanoda.work
EXPECTED_FRONTEND_HASH="$(sha256sum /fixture/frontend/index.html | cut -d ' ' -f 1)"
''' + script[verify_start:verify_end] + '''
for attempt in {1..15}; do
    if verify_nginx_frontend 2>/dev/null; then
        curl --noproxy '*' -kfsSI --resolve 'stock.nanoda.work:443:[::1]' https://stock.nanoda.work/console | grep -q 'X-Deploy-Probe: updated'
        exit 0
    fi
    sleep 1
done
echo 'frontend verification failed' >&2
exit 1
''', encoding='utf-8', newline='\n')
        (fixture / 'frontend').mkdir()
        (fixture / 'frontend/index.html').write_text('<html>updated release frontend</html>\n', encoding='utf-8', newline='\n')
        (fixture / 'nginx-live.conf').write_text('''events {}
http {
    server {
        listen 80;
        listen [::]:80;
        listen 443 ssl;
        listen [::]:443 ssl;
        server_name other.example.com;
        ssl_certificate /fixture/cert.pem;
        ssl_certificate_key /fixture/key.pem;
        location / { return 404; }
    }
    server {
        listen 127.0.0.1:8080;
        location = /console { alias /fixture/frontend/index.html; }
        location = /api/meta { default_type application/json; return 200 '{"ok":true}'; }
    }
    include /fixture/site.conf;
}
''', encoding='utf-8', newline='\n')
        fixed_site = (fixture / 'site.conf').read_text(encoding='utf-8').replace(
            'server_name stock.nanoda.work;',
            'server_name stock.nanoda.work;\n    add_header X-Deploy-Probe updated always;')
        (fixture / 'new-site.conf').write_text(fixed_site, encoding='utf-8', newline='\n')
        (fixture / 'old-site.conf').write_text(
            fixed_site.replace('    listen [::]:443 ssl;\n', '').replace('    listen [::]:80;\n', '').replace('    add_header X-Deploy-Probe updated always;\n', ''),
            encoding='utf-8', newline='\n')
        for name, managed, pid_mode in [
            ('managed-valid-pid', True, 'valid'),
            ('unmanaged-valid-pid', False, 'valid'),
            ('unmanaged-empty-pid', False, 'empty'),
            ('unmanaged-stale-pid', False, 'stale'),
            ('managed-empty-pid', True, 'empty'),
            ('fresh-start', False, 'absent'),
            ('other-config-refused', False, 'other-config'),
        ]:
            command = ('cp /fixture/nginx-live.conf /etc/nginx/nginx.conf && '
                       'chmod 755 /fixture /fixture/frontend && chmod 644 /fixture/frontend/index.html && '
                       ': > /fixture/control-calls.log && cp /fixture/old-site.conf /fixture/site.conf && ')
            if pid_mode == 'other-config':
                command += 'nginx -c /fixture/nginx-live.conf && '
            elif pid_mode != 'absent':
                command += 'nginx -c /etc/nginx/nginx.conf && '
            if pid_mode == 'empty':
                command += ': > /run/nginx.pid && '
            elif pid_mode == 'stale':
                command += "printf '99999999\\n' > /run/nginx.pid && "
            command += 'cp /fixture/new-site.conf /fixture/site.conf && '
            command += f'MOCK_MANAGED={str(managed).lower()} bash /fixture/control.sh'
            result = run(command, success=pid_mode != 'other-config')
            calls = (fixture / 'control-calls.log').read_text(encoding='utf-8').splitlines()
            if ('enable --now nginx' in calls) != (pid_mode == 'absent'):
                raise AssertionError(f'{name}: unexpected nginx start: {calls}')
            if pid_mode == 'other-config' and '无法唯一确认' not in result:
                raise AssertionError(result)
            print(f'nginx lifecycle {name}: PASS', flush=True)

        # 实际重现：worker_connections 高于软上限，新连接在 TLS 握手前因 EMFILE 被阻塞。
        # 修复后使用超过 1024 个双栈 TLS 长连接，且 master 不重启、业务连接槽不变。
        (fixture / 'deploy.sh').write_text(script, encoding='utf-8', newline='\n')
        (fixture / 'nofile-nginx.conf').write_text('''worker_processes 1;
error_log /fixture/nofile-error.log;
events { worker_connections 4096; }
http {
    server {
        listen 443 ssl;
        listen [::]:443 ssl;
        server_name stock.nanoda.work;
        ssl_certificate /fixture/cert.pem;
        ssl_certificate_key /fixture/key.pem;
        ssl_protocols TLSv1.2 TLSv1.3;
        client_header_timeout 5m;
        location / { return 200 'ok'; }
    }
}
''', encoding='utf-8', newline='\n')
        (fixture / 'check-nofile.py').write_text('''import concurrent.futures, pathlib, re, socket, ssl, subprocess, time

def start():
    subprocess.run(['bash', '-c', 'ulimit -Sn 1024; exec nginx'], check=True)
    time.sleep(0.3)
    return int(pathlib.Path('/run/nginx.pid').read_text())

def worker(master):
    workers = []
    for proc in pathlib.Path('/proc').iterdir():
        if not proc.name.isdigit(): continue
        try:
            if (proc/'cmdline').read_bytes().split(b'\\0')[0] != b'nginx: worker process': continue
            if int((proc/'stat').read_text().split(') ', 1)[1].split()[1]) != master: continue
            soft = int(re.search(r'^Max open files\\s+(\\d+)', (proc/'limits').read_text(), re.M)[1])
            workers.append((proc, soft))
        except FileNotFoundError: continue
    if workers: return max(workers, key=lambda item: item[1])
    raise AssertionError('active worker missing')

def repair():
    subprocess.run(['bash', '/fixture/deploy.sh', '--nginx-only'], check=True)

ctx = ssl.create_default_context(cafile='/fixture/cert.pem')
master = start()
old_worker, old_soft = worker(master)
assert old_soft == 1024
sockets = []
try:
    for _ in range(1200):
        sockets.append(socket.create_connection(('127.0.0.1', 443), timeout=2))
    time.sleep(0.5)
    assert 'Too many open files' in pathlib.Path('/fixture/nofile-error.log').read_text()
    try:
        with socket.create_connection(('127.0.0.1', 443), timeout=1) as raw:
            with ctx.wrap_socket(raw, server_hostname='stock.nanoda.work'):
                raise AssertionError('expected handshake failure before repair')
    except (TimeoutError, ConnectionError, ssl.SSLError):
        pass
    repair()
    assert int(pathlib.Path('/run/nginx.pid').read_text()) == master
finally:
    for sock in sockets: sock.close()
    sockets.clear()

# 关闭旧连接并等待旧代退出，容量断言只统计新代，不借用旧代释放后的槽位。
for _ in range(100):
    if not old_worker.exists() or not (old_worker/'cmdline').read_bytes(): break
    time.sleep(0.1)
else:
    raise AssertionError(('old worker did not finish graceful shutdown',
                          (old_worker/'cmdline').read_bytes(),
                          len(list((old_worker/'fd').iterdir())),
                          pathlib.Path('/fixture/nofile-error.log').read_text()[-3000:]))

def connect(i):
    raw = socket.create_connection(('127.0.0.1' if i % 2 == 0 else '::1', 443), timeout=3)
    return ctx.wrap_socket(raw, server_hostname='stock.nanoda.work')

try:
    with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
        sockets.extend(pool.map(connect, range(1200)))
    proc, soft = worker(master)
    assert soft == 16384, soft
    fd_count = len(list((proc/'fd').iterdir()))
    assert fd_count > 1024, (proc.name, fd_count)
    for address in ('127.0.0.1', '::1'):
        with socket.create_connection((address, 443), timeout=3) as raw:
            with ctx.wrap_socket(raw, server_hostname='stock.nanoda.work') as tls:
                tls.sendall(b'GET /healthz HTTP/1.1\\r\\nHost: stock.nanoda.work\\r\\nConnection: close\\r\\n\\r\\n')
                assert b'200 OK' in tls.recv(4096)
    assert not re.search(proc.name + r'#\\d+:.*Too many open files', pathlib.Path('/fixture/nofile-error.log').read_text())
finally:
    for sock in sockets: sock.close()

conf = pathlib.Path('/etc/nginx/nginx.conf')
contents = conf.read_text()
assert 'worker_connections 4096' in contents
backups = list(pathlib.Path('/var/backups/mingmiao-nginx').iterdir())
assert len(backups) == 1
repair()
assert conf.read_text() == contents
assert list(pathlib.Path('/var/backups/mingmiao-nginx').iterdir()) == backups
subprocess.run(['nginx', '-s', 'quit'], check=True)
for _ in range(50):
    if not pathlib.Path('/run/nginx.pid').exists(): break
    time.sleep(0.1)
else: raise AssertionError('master did not stop')
master = start()
assert worker(master)[1] == 16384
print('EMFILE reproduced; 1200 dual-stack TLS connections, graceful reload, idempotence and restart: PASS')
''', encoding='utf-8', newline='\n')
        result = run('cp /fixture/nofile-nginx.conf /etc/nginx/nginx.conf && python3 /fixture/check-nofile.py',
                     nofile='16384:16384')
        if 'restart: PASS' not in result:
            raise AssertionError(result)
        print('nginx EMFILE / dual-stack TLS / persistent limits: PASS', flush=True)
        result = run('cp /fixture/nofile-nginx.conf /etc/nginx/nginx.conf && nginx && '
                     'bash /fixture/deploy.sh --nginx-only; status=$?; '
                     'cmp /fixture/nofile-nginx.conf /etc/nginx/nginx.conf || exit 99; exit "$status"',
                     success=False, nofile='1024:1024')
        if '硬上限不足' not in result:
            raise AssertionError(result)
        print('nginx insufficient hard limit rejected without modifying configuration: PASS', flush=True)
        (fixture / 'external-nofile.conf').write_text('worker_rlimit_nofile 1024;\n', encoding='utf-8', newline='\n')
        (fixture / 'include-nginx.conf').write_text(
            'include /fixture/external-nofile.conf;\n' + (fixture / 'nofile-nginx.conf').read_text(),
            encoding='utf-8', newline='\n')
        result = run('cp /fixture/include-nginx.conf /etc/nginx/nginx.conf && nginx && '
                     'bash /fixture/deploy.sh --nginx-only; status=$?; '
                     'cmp /fixture/include-nginx.conf /etc/nginx/nginx.conf || exit 99; exit "$status"',
                     success=False, nofile='16384:16384')
        if '定义在外部 include' not in result:
            raise AssertionError(result)
        print('nginx external limit definition rejected without duplicate directives: PASS', flush=True)
        verify_start = script.index('verify_nginx_reload() {')
        verify_end = script.index('\nif [[ $EUID', verify_start)
        (fixture / 'rollback-deploy.sh').write_text(
            script[:verify_start] + 'verify_nginx_reload() { return 1; }\n' + script[verify_end:],
            encoding='utf-8', newline='\n')
        result = run('cp /fixture/nofile-nginx.conf /etc/nginx/nginx.conf && nginx && '
                     'bash /fixture/rollback-deploy.sh --nginx-only; status=$?; '
                     'cmp /fixture/nofile-nginx.conf /etc/nginx/nginx.conf || exit 99; exit "$status"',
                     success=False, nofile='16384:16384')
        if '已恢复修改前的 nginx 配置' not in result:
            raise AssertionError(result)
        print('nginx runtime verification failure restores previous configuration: PASS', flush=True)


if __name__ == '__main__':
    main()

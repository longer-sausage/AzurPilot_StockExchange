"""在临时 nginx 容器中验证部署前置校验和实际 nginx 配置，不修改宿主服务。"""

import os
from pathlib import Path
import subprocess
import tempfile


def main():
    root = Path(__file__).resolve().parent.parent
    script = (root / 'deploy.sh').read_text(encoding='utf-8')
    image = os.environ.get('NGINX_TEST_IMAGE', 'nginx:stable')
    subprocess.run(['docker', 'pull', image], check=True)
    with tempfile.TemporaryDirectory(prefix='mmex-deploy-check-') as directory:
        fixture = Path(directory)

        def run(command, *, success=True):
            result = subprocess.run(
                ['docker', 'run', '--rm', '--network=none', '-v', f'{fixture}:/fixture', image,
                 'bash', '-c', command], capture_output=True, text=True, encoding='utf-8',
            )
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
        helper_start = script.index('reload_existing_nginx() {')
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


if __name__ == '__main__':
    main()

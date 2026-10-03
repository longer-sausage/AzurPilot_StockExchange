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
        values = dict(EXCHANGE_DOMAIN='stock.nanoda.work', TURNSTILE_SECRET_KEY='test-private-key',
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


if __name__ == '__main__':
    main()

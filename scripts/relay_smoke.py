"""隔离 Python 原生代理 → Go API 验收，不使用真实账号或截图。"""
import json
import os
import socket
import subprocess
import sys
import tempfile
import time
from datetime import datetime
from pathlib import Path
from types import SimpleNamespace
from urllib.request import urlopen
from unittest.mock import Mock

root = Path(__file__).resolve().parents[1]
pilot = Path(os.environ.get('AZURPILOT_PATH', str(root.parent / 'AzurPilot')))
sys.path.insert(0, str(pilot))
from module.api.stock_exchange_service import StockExchangeService

binary = Path(os.environ.get('EXCHANGE_BIN', str(root / 'bin' / ('exchange.exe' if os.name == 'nt' else 'exchange'))))
if not binary.is_file():
    raise SystemExit('请先构建 bin/exchange(.exe)，或指定 EXCHANGE_BIN')
with socket.socket() as probe:
    probe.bind(('127.0.0.1', 0))
    port = probe.getsockname()[1]
origin = f'http://127.0.0.1:{port}'
os.environ['STOCK_EXCHANGE_URL'] = origin
with tempfile.TemporaryDirectory() as directory:
    workspace = Path(directory)
    (workspace / 'config').mkdir()
    row = {'Dashboard': {'ActionPoint': {'Total': 1200, 'Record': datetime.now().isoformat()}}}
    config = workspace / 'config' / 'test.json'
    config.write_text(json.dumps(row))
    configs = SimpleNamespace(root=workspace, path=lambda name: workspace / 'config' / (name + '.json'), read=lambda _: (json.loads(config.read_text()), 'revision'))
    env = {**os.environ, 'MOCK_MODE': 'true', 'LISTEN_ADDR': f'127.0.0.1:{port}', 'DATABASE_PATH': str(workspace / 'exchange.db'), 'FRONTEND_DIR': str(root / 'frontend' / 'dist')}
    process = subprocess.Popen([str(binary)], cwd=root, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    service = reloaded = None
    try:
        for _ in range(100):
            try:
                with urlopen(origin + '/api/meta', timeout=1) as response:
                    assert json.load(response)['mock']
                break
            except OSError:
                time.sleep(.1)
        else:
            raise AssertionError('隔离 Mock Go 服务未就绪')
        service = StockExchangeService(configs)
        service.start = Mock()
        body = {'username': '隔离实例验收', 'password': 'smoke-test-password', 'acceptedNotice': '2026-10-03', 'turnstileToken': 'XXXX.DUMMY.TOKEN.XXXX'}
        reply = service.request('test', '/register', 'POST', body)
        assert reply['status'] == 201, reply
        assert reply['data']['token'] == 'instance-session' and 'uploadToken' not in reply['data']
        assert service.status('test')['bound'] and service.status('test')['authenticated']
        account = service.request('test', '/account')['data']
        assert account['player']['quote']['price'] == 120000
        other = service.request('other', '/login', 'POST', {k: v for k, v in body.items() if k != 'acceptedNotice'})
        assert other['status'] == 403 and other['data']['error']['code'] == 'INSTANCE_MISMATCH', other
        time.sleep(16)
        row['Dashboard']['ActionPoint'].update(Total=1300, Record=datetime.now().isoformat())
        config.write_text(json.dumps(row))
        service.sync_once('test', force=True)
        assert service.request('test', '/account')['data']['player']['quote']['price'] == 130000
        service.request('test', '/logout', 'POST', {})
        assert service.status('test')['bound'] and not service.status('test')['authenticated']
        reloaded = StockExchangeService(configs)
        reloaded.start = Mock()
        assert reloaded.status('test')['bound'] and reloaded.status('test')['instanceId'] == service.status('test')['instanceId']
        assert not reloaded.status('test')['authenticated']
        logged = reloaded.request('test', '/login', 'POST', {k: v for k, v in body.items() if k != 'acceptedNotice'})
        assert logged['status'] == 200, logged
        print('Python 原生代理 → Go：开户、实例签名、跨实例拒绝、1200 → 1300 同步、退出与重启绑定恢复全部通过')
    finally:
        if service:
            service.close()
        if reloaded:
            reloaded.close()
        process.terminate()
        process.wait(timeout=15)

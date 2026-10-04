"""隔离 Python 原生代理 → Go API 验收，不使用真实账号或截图。"""
import json
import os
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timedelta
from contextlib import ExitStack, closing
from pathlib import Path
from types import SimpleNamespace
from urllib.request import urlopen
from unittest.mock import Mock, patch

root = Path(__file__).resolve().parents[1]
pilot = Path(os.environ.get('AZURPILOT_PATH', str(root.parent / 'AzurPilot')))
sys.path.insert(0, str(pilot))
from module.api.stock_exchange_service import StockExchangeService
from module.api.stock_exchange_history import SHANGHAI, history_point
from module.scheduler.store import ProgramStore
from module.runtime.account_local import LocalProtector

binary = Path(os.environ.get('EXCHANGE_BIN', str(root / 'bin' / ('exchange.exe' if os.name == 'nt' else 'exchange'))))
if not binary.is_file():
    raise SystemExit('请先构建 bin/exchange(.exe)，或指定 EXCHANGE_BIN')
with socket.socket() as probe:
    probe.bind(('127.0.0.1', 0))
    port = probe.getsockname()[1]
origin = f'http://127.0.0.1:{port}'
os.environ['STOCK_EXCHANGE_URL'] = origin
with tempfile.TemporaryDirectory() as directory, ExitStack() as protection:
    workspace = Path(directory) / 'project'
    (workspace / 'config').mkdir(parents=True)
    protection.enter_context(patch.object(LocalProtector, 'key_directory', return_value=Path(directory) / 'keys'))
    row = {'Alas': {}, 'Dashboard': {'ActionPoint': {'Total': 1200, 'Record': datetime.now().isoformat()}}}
    config = workspace / 'config' / 'test.json'
    config.write_text(json.dumps(row))
    (workspace / 'config' / 'other.json').write_text(json.dumps(row))
    configs = SimpleNamespace(root=workspace, path=lambda name: workspace / 'config' / (name + '.json'),
                              read=lambda name: (json.loads((workspace / 'config' / (name + '.json')).read_text()), 'revision'))
    now = datetime.now(SHANGHAI)
    start = now.replace(day=1, hour=0, minute=0, second=0, microsecond=123000)
    historical = [{'ts': (start + (now - start) * (i / 3002)).isoformat(), 'ap_total': 1100 + i % 50}
                  for i in range(3001)]
    with closing(sqlite3.connect(workspace / 'config' / 'cl1_data.db')) as stats, stats:
        stats.execute('CREATE TABLE cl1_data(instance TEXT,month TEXT,data_json TEXT,encrypted_blob BLOB,PRIMARY KEY(instance,month))')
        stats.execute('INSERT INTO cl1_data VALUES(?,?,?,NULL)', ('test', now.strftime('%Y-%m'), json.dumps({'ap_snapshots': historical})))
        previous = (start - timedelta(days=1)).strftime('%Y-%m')
        stats.execute('INSERT INTO cl1_data VALUES(?,?,NULL,?)', ('test', previous, b'unreadable-old-ciphertext'))
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
        body = {'username': '隔离实例验收', 'password': 'smoke-test-password', 'acceptedNotice': '2026-10-03', 'recaptchaToken': 'recaptcha-test-token'}
        reply = service.request('test', '/register', 'POST', body)
        assert reply['status'] == 201, reply
        assert reply['data']['token'] == 'instance-session' and 'uploadToken' not in reply['data']
        assert service.status('test')['bound'] and service.status('test')['authenticated']
        assert '旧统计' not in service.status('test')['message']
        account = service.request('test', '/account')['data']
        assert account['player']['quote']['price'] == 120000
        expected = {history_point(point['ap_total'], point['ts'])[0]: point['ap_total'] for point in historical}
        expected[history_point(1200, row['Dashboard']['ActionPoint']['Record'])[0]] = 1200
        for _ in range(5):
            service.sync_once('test', force=True)
        stock_id = account['player']['id']
        detail_path = f"/stocks/{stock_id}?period=day&month={now:%Y-%m}"
        detail = service.request('test', detail_path)['data']
        assert detail['coverage']['count'] == len(expected), detail['coverage']
        assert detail['coverage']['firstObservedAt'] == min(expected), detail['coverage']
        assert detail['coverage']['lastObservedAt'] == max(expected), detail['coverage']
        assert detail['coverage']['reconciledAt'] > 0, detail['coverage']
        assert sum(bar['samples'] for bar in detail['bars']) == len(expected)
        first_day = service.request('test', f"/stocks/{stock_id}?period=time&month={now:%Y-%m}&day={now:%Y-%m}-01")['data']
        assert first_day['bars'] and first_day['bars'][0]['open'] == historical[0]['ap_total'] * 100
        previous_detail = service.request('test', f'/stocks/{stock_id}?period=day&month={previous}')['data']
        assert previous_detail['coverage']['count'] == 0, previous_detail['coverage']
        with closing(sqlite3.connect(workspace / 'config' / 'cl1_data.db')) as stats:
            assert stats.execute('SELECT encrypted_blob FROM cl1_data WHERE instance=? AND month=?', ('test', previous)).fetchone()[0] == b'unreadable-old-ciphertext'
        other = service.request('other', '/login', 'POST', {k: v for k, v in body.items() if k != 'acceptedNotice'})
        assert other['status'] == 403 and other['data']['error']['code'] == 'INSTANCE_MISMATCH', other
        time.sleep(16)
        row['Dashboard']['ActionPoint'].update(Total=1300, Record=datetime.now().isoformat())
        row['_stockInstance'] = json.loads(config.read_text())['_stockInstance']
        config.write_text(json.dumps(row))
        ProgramStore(workspace / 'config').observe('test', 'ActionPoint', {'Total': 1300}, row['Dashboard']['ActionPoint']['Record'], 'isolated-smoke')
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
        print('Python 原生代理 → Go：跳过且保留无法读取的旧月份、当月 3001 条注册前历史补传、日 K / 月初分时、摘要校对、实例隔离、实时同步与重启全部通过')
    finally:
        if service:
            service.close()
        if reloaded:
            reloaded.close()
        process.terminate()
        process.wait(timeout=15)

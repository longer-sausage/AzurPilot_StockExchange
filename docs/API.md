# HTTP API 契约

生产 origin 为环境变量 `EXCHANGE_DOMAIN` 指定的 HTTPS 域名，默认 `https://stock.nanoda.work`。原生玩家页面通过 AzurPilot `/api/v1/ws` 的 `stock.request` 转发；开发管理控制台通过同源 Vite `/api` 代理。JSON 字段采用 camelCase；**金额为整数分，费率为 ppm（1%=10000）**，时间为 Unix 秒，股票 / 玩家 / 委托 ID 为整数。GET 返回资源，错误格式为 `{"error":{"code":"...","message":"..."}}`，前端应显示 message。

| 方法 | 路径 | 身份 | 功能 |
| --- | --- | --- | --- |
| GET | `/healthz` | 无 | 无数据库查询的健康检查 |
| GET | `/api/meta` | 无 | 名称、域名、noticeVersion、允许来源、mock 状态；不返回 site key |
| POST | `/api/register` | 无 + CF + 实例身份 | 注册、永久绑定实例、自动上市、初始资金，返回玩家会话和仅上传凭据 |
| POST | `/api/login` | 无 + CF + 实例身份 | 玩家用户名密码登录，返回 24 小时会话 |
| GET | `/api/market` | 无 | 股票、市场状态、规则、实时净值排行榜、收入；支持 ETag / 304 |
| GET | `/api/history/{stock}` | 无 | 此证券最近 240 条有效历史报价，按时间升序 |
| GET | `/api/stocks/{stock}` | 无 | 专业行情详情、K 线与真实成交量、最近 100 笔成交及 50 笔公开挂单；ETag / 304 |
| POST | `/api/quote-history` | 仅上传 + 实例签名 | 持久整月历史补传、同一毫秒修正、月摘要确认；不验证数据真实性 |
| GET | `/api/quote-history/manifest?month=YYYY-MM` | 仅上传 + 实例绑定 | 本人股票月原始记录的 count、SHA-256、首末时间及校对时间 |
| GET | `/api/seasons` | 无 | 最近 12 个已归档赛季及最终排名 |
| GET | `/api/account` | 玩家 | 本人身份识别码、现金、持仓、委托、交收、冻结、净值、保证金、融资本金/利息、借券费用 |
| POST | `/api/orders` | 玩家 | 提交带 clientId 的幂等委托 |
| DELETE | `/api/orders/{order}` | 玩家 | 撤销本人待触发委托 |
| POST | `/api/upload-token` | 玩家 | 轮换仅上传凭据，旧凭据立即撤销 |
| POST | `/api/quotes` | 仅上传 | 上传绑定实例的自报总行动力；不进行真实性验证；不接受玩家会话替代 |
| POST | `/api/console/login` | 无 + CF | 环境变量管理员密码验证，返回 2 小时管理会话 |
| GET | `/api/console/settings` | 管理员 | 当前规则、预设、初始资金、赛季设置、暂停状态 |
| PUT | `/api/console/settings` | 管理员 | 完整配置原子保存，所有数值与时段由服务器验证 |
| GET | `/api/console/players` | 管理员 | 私有用户列表，支持用户名 / 证券代码 / 身份码 / 实例 ID 搜索、状态筛选、分页 |
| GET | `/api/console/players/{player}` | 管理员 | 完整账户详情，包括身份码、永久绑定、资金、持仓、委托和交收 |
| PUT | `/api/console/players/{player}` | 管理员 | 修改用户名、重设密码、调整现金或停用 / 恢复账户，成功返回完整 Account |

使用 `Authorization: Bearer <token>`；玩家交易接口还须携带 `X-MMEX-Instance: <binding.key>`，必须与会话和永久绑定一致。玩家和管理员会话用不同 role 的 HMAC 签名验证；仅上传凭据随机生成，服务器只保存其 SHA-256。API 不设置跨站 cookie，不将会话放进 URL。

## 用户管理与私有身份码

每个账户的 `player.identityCode` 为独立的 128 位随机身份识别码，格式为 `MMEX-XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX`，数据库施加唯一约束。已有账户在启动升级时自动补发。身份码永久保留，改名、重设密码、停用恢复、赛季重置和服务重启均不会改变；管理 API 不允许修改身份码或换绑实例。身份码仅出现在成功的本人注册 / 登录、本人 `/account` 与管理员用户接口，公开行情、排行榜、历史和成交记录不返回此字段；私有接口使用 `Cache-Control: no-store`。该码用于人工核实账户，不替代密码和实例签名认证。

用户列表参数为 `q`（最多 128 字符）、`status=all|active|disabled`、`page`（默认 1，最大 1000000）、`pageSize`（默认 20，最大 100）；响应为 `{players,total,page,pageSize,active,disabled}`，其中计数 `active/disabled` 为全站账户数。越过末页时返回最后一个有效页。停用用户的资金和净值仍完整显示。

修改请求只提交变更字段，例如：

```json
{"username":"新用户名","cashAdjustment":123456,"expectedCash":2000000000}
```

`username` 沿用 2–20 位归一化唯一性校验。可选 `password` 为新密码，需 10–72 字节，成功后立即撤销旧交易会话，哈希与凭据版本不会回传。`cashAdjustment` 为整数分的增减量，必须同时提供读取详情时的 `expectedCash`；余额已变化时返回 HTTP 409 / `CASH_CHANGED`。现金调整先结清累计利息 / 借券费，再按增减量入账，保留赛季本金并影响当前净值；调整后现金须在 0 至 1 万亿模拟币范围内，扣减不得占用冻结、待交收资金或导致保证金不足。非法输入或落盘失败时，整笔修改回滚。

`disabled=true` 强平本人持仓、撤销其全部挂单及其他用户针对其股票的挂单，并撤销旧会话；恢复账户后须重新登录。密码重设及停用产生的会话撤销状态会持久化，服务重启或恢复账户不会使旧会话再次有效。

## 注册与登录

注册请求：

```json
{
  "username": "指挥官猫",
  "password": "至少十位的安全密码",
  "turnstileToken": "由 register action 的 Turnstile widget 生成",
  "acceptedNotice": "2026-10-03",
  "report": {
    "instanceId": "当前实例持久 UUID",
    "publicKey": "Ed25519 32 字节公钥的 Base64",
    "actionPoints": 8000,
    "observedAt": 1790953200,
    "issuedAt": 1790953200,
    "signature": "实例签名的 Base64"
  }
}
```

示例时间须替换为当前游戏记录时间。用户名归一后 2–20 字符，密码 10–72 字节。noticeVersion 从 `/api/meta` 获取。公开 site key `0x4AAAAAAFMCQstp3hgd939a` 硬编码在玩家和管理前端，服务端只读取私密 `TURNSTILE_SECRET_KEY`，不提供 site key 配置或元数据字段。登录提交 `username`、`password`、`turnstileToken` 和同一实例签名的 `report`，widget action 为 `login`；管理员 action 为 `console-login`。注册使用 `register`。生产每次都调用 Cloudflare Siteverify，检查 success、hostname、action，失败不创建账户/会话。验证 token 一次使用，错误后重新挑战。

注册响应 `{"token":"...","uploadToken":"...","player":{...}}`，登录响应不返回原上传凭据；遗失时由已登录玩家轮换。所有私有哈希都从公开 JSON 中排除。AzurPilot 后端保存真实会话和上传凭据，浏览器只收到 `token: "instance-session"` 标记。登录旧的未绑定账户时只允许首次绑定；之后不能换绑，退出登录不解除绑定。

Mock 也使用 Cloudflare 官方测试组件与 Siteverify：前端测试 site key 为 `1x00000000000000000000AA`，Go 仅在回环监听的 Mock 模式选择官方测试 secret。必须提交 `XXXX.DUMMY.TOKEN.XXXX` 并获得服务端验证成功，不在本地直接放行；测试响应没有实际页面 hostname/action，Mock 不匹配这两个字段，生产继续严格匹配并拒绝使用官方测试 secret。单元测试以本机 HTTP 夹具替代外部网络，演示与浏览器联调使用真实 Cloudflare 测试接口。

## 委托与报价

```json
{
  "clientId": "客户端为本笔委托生成并在重试中保持的UUID",
  "stockId": 1,
  "side": "buy",
  "kind": "limit",
  "tif": "GTC",
  "quantity": 100,
  "limit": 700000,
  "leverage": 2
}
```

clientId 需 8–80 位 ASCII 字母/数字/下划线/短横线。`side` 为 buy/sell/short/cover，`kind` 为 market/limit/stop，`tif` 为 DAY/GTC；market 的 limit 为 0。委托数量按当前整手单位。成功返回完整 Order，状态 pending 或 filled。委托触发后，状态也可为 rejected、expired、cancelled。API 受理与后续状态通过 `/api/account` 更新。

限价买入/回补 `quote <= limit`，限价卖出/卖空 `quote >= limit`；止损条件相反。触发以**实际报价**全量成交。挂单冻结，市价操作同一原子事务核算资金与费用；重复 clientId 返回已有回执，不二次执行。重试必须保留同一 clientId，不能生成新委托。异常大额开仓若超出按最高报价计算的整数账本安全范围，返回 `NUMERICAL_LIMIT`；平仓仍然允许。

`leverage` 为整数 1–10，仅 buy/short 可选择大于 1；上限 `max(1,floor(1000000/imrPPM))`，开仓受理与实际触发都会核验。省略时旧现金买单按 1 倍、旧空单按原 IMR 行为处理。平仓 sell/cover 不新增借款；显式非法倍数返回 `INVALID_LEVERAGE`。

2 倍买入以 50% 成交额自付、其余融资，交易费全额自付。`player.positions[].loan` 为未偿本金；卖出按股数比例偿还，只有净卖出款进入 T+N 交收。`account.financingDebt`、`financingAccrued`、`longMargin` 分别为融资余额、未落账利息和融资多头初始保证金；净值已扣本金及利息。`fees.financing` 单列融资利息，计入玩家及交易所收入。`rules.financingAnnualPPM` 为可配置的融资年利率，老配置缺失时迁移为 80000（8%），显式 0 保留。融资不允许充当收入重复提升排行榜。

同一证券混合不同杠杆的持仓以 `marginUnits` 保存精确的股数 × 保证金率，平仓按股数同比例减少。MMR 按融资多头现值与空仓现值之和核算，净值不足时强平并保留负现金债务。更新融资利率前先结算旧区间；按秒、整数分和余数结转计息，复用现有每 15 秒结算循环。

报价：`{"report":{...}}`，report 包含当前实例公钥、UUID、自报行动力、记录时间和请求签名，Authorization 为仅上传凭据。新记录 AP 可以涨或跌，记录时间只能前进。同时间同数值幂等，同时间不同数值失败。没有新游戏记录时不要人为刷新记录时间以绕过报价过期。

## 安全与负载

详情参数 `period=time|day|m5|m10|m20|m30|m60`、`month=YYYY-MM`、分时的 `day=YYYY-MM-DD`。月份可选本月及此前 12 个月；空日期默认该月最近有行情日。`bars` 返回开高低收（整数分）、真实成交量、成交额（分）、买卖方向量、实际采集样本数；`bars[].time`、逐笔成交 `trades[].time` 和原始历史时间为 **Unix 毫秒**。`summary` 为所选最近行情日统计，`coverage` 为该月精确来源记录数量及校对时间。日 K 携带此前三个月用于均线预热，分钟 K 携带此前两天；`displayFrom` 标出所选月份显示起点。没有行情也没有成交的时段不填造 K 线；仅有成交的周期使用实际成交价并标记 samples=0。成交只统计一次，含强平，幂等委托重试不重复计数。账户现金、费用和交易结算继续使用整数账本。

历史上传为 `{"report":{instanceId,publicKey,month,issuedAt,points:[{time,actionPoints}],count,digest,signature}}`。最多 1024 点，严格毫秒递增，支持本月和过去 12 个月；允许同时间的来源修正。普通批次 count=0、digest=""；校对批次 points=[]、count=完整月份记录数、digest=SHA-256。摘要按所有来源点时间升序，对每行 `<time>:<actionPoints>\n` 的 UTF-8 字节计算。服务端只核验签名、凭据、格式和两端一致性；无法证明行动力真实。摘要不匹配返回 `HISTORY_MISMATCH`，同批内容整体回滚。迟到补传只修正历史，不重放旧交易；新鲜的最新记录可更新实时报价和触发当前风控。历史补传每 IP 最多 30 次/分钟，摘要查询 12 次/分钟，仍受总体限流。

历史签名文本（末尾换行）：

```text
mmex-history-v1
<instanceId>
<publicKey>
<month>
<issuedAt>
<count>
<digest>
<point.time>:<point.actionPoints>
...其余点按时间升序
```

AzurPilot 在现有资源入库处保留总行动力历史，独立 WAL 补传日志持久化待传/确认状态，每批一签、最短 3 秒，指数退避最大 60 秒并持续重试。月之间轮转，避免新记录阻断旧月补传；每五分钟重扫来源并按月 count / SHA-256 校对，发现缺行或修正即重新排队。完整性边界是**实际采集并保存的记录**；升级前已被覆盖的记录或游戏离线、未采集期间的变化不可还原。中央历史与 `cache/stock-exchange/` 均应备份。

Go 保留毫秒原始历史，按变化分钟增量重建 OHLC；新表不采用旧 quotes 的 2000 行截断。图表请求按证券版本缓存，最多 24 份 / 8 MiB；技术指标在浏览器计算，详情隐藏页面停止轮询。旧 quotes 与已成交回执首次升级迁移，旧报价标记为暂估来源并在精确观测到达时替换。

64 KiB 请求体、8 KiB 请求头；严格字段、整数与数量校验；认证最多每 IP 每分钟 8 次，普通 API 每 IP 每分钟 240 次。CF 网络并发按 `max(4, 2×GOMAXPROCS)`、bcrypt 计算并发按 `GOMAXPROCS` 分别准入；繁忙时立即拒绝，不排队等待。客户端应处理 429 / 503 并稍后重试；上传按新记录去重。浏览器来源在环境变量 allowlist 中逐个匹配，回环任意端口可使用 `:*`，不使用任意公网通配符。static `/assets` 为哈希文件名长缓存，API 私有数据 no-store。

## 实例签名及原生 AzurPilot 代理

服务器只验证实例身份与请求完整性，**不判断行动力真实性、不上传截图、不运行 OCR**。每个实例 UUID 与公钥派生唯一 binding.key，数据库唯一约束保证一实例一账户、一账户一实例。新报告必须与此身份一致。

签名使用 Ed25519。依次以 UTF-8 编码以下字面文本，末尾保留换行，再签名并使用标准 Base64 编码：

```text
mmex-instance-v1
<instanceId>
<publicKey>
<actionPoints>
<observedAt>
<issuedAt>
```

`issuedAt` 是本次请求签发时间，最多超前 60 秒、落后 120 秒；这只是身份认证请求时效，不代表游戏数据被验证。上传另有基本整数范围、24 小时接收窗口和记录顺序检查；报价超过管理员预设有效期则暂停新成交。首次注册没有近期记录时可由 AzurPilot 以 0 上市；登录不要求行动力记录有效，也不自动刷新旧报价的有效期。

AzurPilot 复用已认证的 `/api/v1/ws`，只有两个方法：

| 方法 | 参数 | 返回 |
| --- | --- | --- |
| `stock.status` | `{instance}` | 当前实例 UUID、绑定状态、绑定用户名、登录状态、行动力快照；不回传凭据 |
| `stock.request` | `{instance,path,method,body,etag}` | `{status,data,etag,serverTime}`；method 为 GET / POST / DELETE |

实例代理只允许市场、教程数据、注册、登录、本人账户、委托、撤单、同步及退出；不能代理管理接口。浏览器不能提供 report、实例公钥、行情或上传/会话凭据；由 AzurPilot 后端按所选实例补齐、签名。注册、登录和退出后刷新绑定状态，切换实例重新挂载终端，丢弃前一个实例的页面数据。

真实交易会话只保存在后端内存；实例 UUID / 私钥与上传令牌保存在被忽略的 `cache/stock-exchange/` 下，需备份。后台每 2 秒只查文件修改状态，未变化时不重读/签名/访问远端；最多每 15 秒同步一条新行动力记录。退出登录、关闭页面和后端重启不会解除永久绑定或停止后台上传；后端重启后交易需再次通过验证码登录。

旧的 iframe / postMessage 桥、`stock.bind` 和 `stock.unbind` 已移除。此接口为本项目新功能的开发期契约更新，需同时更新 AzurPilot 和交易所后端。

## 初始资金与收益率

`settings.initialCash` 是整数分，默认 `2000000000`（20,000,000 模拟币），范围 `100` 至 `100000000000000`。修改控制台配置只影响此后新开户和下一赛季重置，已有账户当前现金及起始本金保留。`meta.initialCash` 和 `market.initialCash` 返回当前开户配置；`player.initialCash` 返回本人本赛季实际起始本金，排行榜收益率以此为分母。旧数据库缺字段时迁移为默认 2 千万，零或越界的新配置会拒绝。

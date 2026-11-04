# 茗喵证券交易所

独立的模拟证券游戏，站点为 **https://stock.azurpilot.com**。主页面公开浏览行情；交易与管理员登录均支持“记住我”。本项目不涉及真实资金、充值或提现。

## 账户与三个组件

- **交易所**：本仓库 Go 后端、Next.js / React 交易站和 `/console` 管理控制台。前端使用 Cloudflare Kumo、Tailwind CSS 4、Phosphor Icons 和 ECharts，静态导出后由 Cloudflare Workers Static Assets 提供；同源 API 使用无脚本路由直接回源至既有 Go 服务，不执行 Worker。也保留直接由 Go 提供静态文件的部署方式。交易账本使用外部 PostgreSQL。
- **AzurPilot**：相邻仓库仅提供实例股票注册、绑定状态与行动力增量上报，不再嵌入交易终端、不代理玩家登录或交易。
- **AstrBot**：`astrbot_plugin_mingmiao_stock` 插件仅支持官方 QQ 机器人。只修改插件目录，不需要修改 AstrBot 核心、全局配置或操作其 Docker。

交易账户与股票账户完全独立：

1. 加入 QQ 群 [1126124220](https://qm.qq.com/q/93NWF3S362)，向官方机器人发送 `注册 <用户名>`。机器人直接创建交易账户并在当前群内返回随机初始密码；官方唯一身份为 `appid:openid`，不收集真实 QQ 号。
2. 使用该用户名和初始密码登录独立交易站，首次登录强制修改为自己的强密码。网页仅提供登录、改密和交易，已完全删除网页注册及注册密钥流程。
3. 在 AzurPilot 当前实例的“茗喵证券交易所”填写股票名称，注册独立股票。复制绑定命令，在群内向机器人发送 `绑定股票 <绑定码>`。
4. 交易账户必须完成改密和一对一股票绑定才能查看账户、维护自选或交易。未绑定账户仅可完成登录、改密、绑定、重置与退出；网页显示必需的绑定引导。
5. 机器人注册、重置密码、持仓、绑定、查询和交易全部支持群聊，密码等回复也在原群内公开显示。确认码仍仅允许发起人在原会话使用，不允许他人代为确认。

密码重置需要二次确认，会撤销旧会话，生成随机强密码，并要求下次登录改密。管理员删除股票会按最后报价结清该股票的持仓、撤销相关挂单、解除发行交易账户的绑定；AzurPilot 收到删除响应后持久停止该身份上报，账户必须绑定其他股票才能继续使用。历史和成交记录保留。

交易站已恢复月赛进度及当前/历史排行榜、证券筛选与排序、账户持久自选、独立证券详情、分时/日 K/分钟 K、技术指标、成交量及逐笔成交、公开待成交委托、持仓和损益、账户费用及交收明细、六步教程与 37 项名词词典、亮暗主题。管理端提供账户搜索分页、完整详情、改名、并发保护的余额增量调整、二次确认停用及密码重置、股票停牌/删除、预设另存/更新/删除。

市场休市且当前赛季未来 24 小时内将开市时，交易站仅展示时、分、秒及九位纳秒倒计时与北京时间开市时间，保留登录、首次改密、股票绑定、退出、主题、刷新及管理入口；行情、持仓、委托、排名、指南导航和内容暂时隐藏，旧链接与刷新也遵守此显示规则。倒计时按当前赛季窗口及生效市场的时区、交易时段、周末和节假日计算；管理员暂停、赛季已结算或 24 小时内无可交易时段时不进入倒计时。到点自动刷新，后端确认开放后恢复原视图，管理控制台始终可用。倒计时统一显示为单行 `HH:MM:SS:NNNNNNNNN`，不显示单位标签。`performance.now()` 驱动实际剩余时间，末五位使用按帧变化的模拟尾数，呈现九位连续滚动效果；模拟值不代表真实纳秒精度，不参与开市或交易准入判断。通过 `requestAnimationFrame` 只刷新倒计时组件，到零或离开页面即停止，每秒重新对齐本机时钟。此为前端展示规则，不改变后端交易准入或 v2 API 的毫秒时间单位。

本次改动范围、旧数据限制、兼容性及配套发布顺序见 [注册、绑定与功能恢复交付说明](docs/FEATURE_RESTORATION.md)。旧版差异审计保留为改动前基线，不作为当前缺失列表。

## 游戏规则

`config/defaults.json` 是新数据库的默认规则，只在首次启动空的 v2 数据库时载入。重启不会覆盖管理员已保存配置，不改写其他已有规则预设或旧 SQLite 数据库。

`config/presets/` 独立保留茗喵、A 股、港股、美股参考预设。现实参考预设的原有佣金、印花税、征费、整手、交易时段、T+N 交收、保证金及融资/借券利率不变；仅更新过时的无限额度描述。管理员可选择并编辑当前生效副本，不会改写这些参考文件。有限发行量、普通和融券小时额度是所有模式的公共账本约束；新增阶梯税与流动性溢价只在茗喵默认模式启用。

| 项目 | 默认值 |
| --- | --- |
| 初始模拟币 | 20,000,000 |
| 单股票总发行 | 10,000 股 |
| 单股票普通小时交易额度 | 1,000 股，买卖共用 |
| 单股票融券总量 | 10,000 股，与普通库存独立 |
| 单股票融券小时交易额度 | 1,000 股，卖空与回补共用 |
| 最大流动性溢价 | 5%，随小时利用率增加 |
| 总资产税率 | `[0, 3千万]` 0%；`(3千万, 8千万]` 8%；`(8千万, 1亿]` 16%；大于 1亿 32% |

税率按每次成交前总资产所属档位，对该次整个成交额征税，按整数分向上取整。另有佣金和借券费用，当前配置以“交易指南 → 当前交易规则”为准。管理员可以调整默认规则及每支现有股票的发行量、交易额度、借券量和借券额度。

额度内立即成交；超额、库存不足或不满足限价的余量挂单。每个自然小时整点恢复额度，挂单按限价从高到低、同价先到先得处理。买入/回补不超过限价，卖出/卖空不低于限价。未成交部分可撤销，冻结资金和股票不能重复使用。

只有绑定交易账户的股票可以被卖空，不得交易自己绑定的股票。做空需要现金保证金，所得冻结；回补亏损按实际收取金额直接补偿股票绑定账户，盈利或未平仓不向发行者收费。保证金不足会风控平仓，发行者不承担交易者债务。

保留融资买入、借款利息、保证金强平、T+N 股票/现金交收、月赛结算与低行动力退市。月赛按上海时间，默认每月 5 日开赛、倒数第 5 日结束时截止；新月重置赛季资金和持仓，保留账户、绑定和行情历史。退市默认每月 5 日起且开赛后，实时行动力低于 500 时本月退市，历史补传不会重放退市。当前开放状态与实际赛季窗口以市场接口和页面展示为准。

## API 与带宽

完整契约见 [docs/API-v2.md](docs/API-v2.md)。API 前缀 `/api/v2`，金额与价格为整数分、费率 ppm、行情时间毫秒。没有 Google reCAPTCHA 或旧 v1 API。

公开行情仅返回股票摘要，支持 ETag/304；SSE 仅通知版本，浏览器合并变化后重取。图表按所选范围返回指定周期的 OHLC 聚合，不传全量行动力。原始历史独表保存，不随账本重写。AzurPilot 仅发送未确认增量，历史缺口/同毫秒修正通过摘要核验补传；空档不造点，历史补传不重放交易。实例 Ed25519 签名用于确认来源，不证明游戏数据真实，私钥不进入浏览器。

11 月行动力实机采样、自动上传与月初单根 K 线的修复及发布验收见 [行动力采样与图表修复记录](docs/ACTION-POINT-2026-11-01.md)。

## 构建与验证

Go 版本以 `go.mod`、Node 依赖以 `frontend/package.json` 为准。

前端要求 Node.js 22 及以上。Next.js 仅导出一个业务首页，前端是 React 单页应用（SPA）。市场、详情、持仓、委托、月赛、指南和管理界面通过 React 状态切换，地址栏始终保持 `/`；`history.state` 保存当前视图，支持刷新和浏览器前进、后退。构建产物复制至既有 `frontend/dist/`，无需 Node.js 生产服务；Cloudflare 使用一个整站 Worker Route，无需逐页配置路由。设计约束与所用 Skill 见 [frontend/DESIGN.md](frontend/DESIGN.md)。

```powershell
go test ./...
go vet ./...
go test -race ./...
npm ci --prefix frontend
npm run build --prefix frontend
```

### 本地假数据预览（无需 Docker 或数据库）

```powershell
npm run build --prefix frontend
npm run preview:demo --prefix frontend
```

打开 `http://127.0.0.1:5178`，点击顶部“管理”进入控制台。本地玩家为 `预览玩家`，密码 `Demo-only-Password!87`；管理员演示密码为 `Demo-only-Admin!87`。用户名 `首次登录演示` 可体验强制改密流程。这些值仅属于本地假数据夹具，不是线上凭据。夹具仅监听回环地址，不连接线上 API 或 PostgreSQL，重启即恢复初始状态，也不会进入 `frontend/dist/`。

浏览器 UI 验证使用相同的生产静态产物和本地夹具：

```powershell
npm exec --prefix frontend -- playwright install chromium
npm run test:ui --prefix frontend
```

该组用例覆盖布局、控件、登录、交易请求及管理交互，不能代替真实 PostgreSQL 账本验证。连接本地 Go 开发服务时使用 `npm run dev --prefix frontend`，Next.js 开发代理默认转发至 `127.0.0.1:8888`，可用 `EXCHANGE_API_PORT` 调整；生产 API 通过 Cloudflare 无脚本路由直接回源，静态导出本身不运行 Next.js 服务。

真实 PostgreSQL 测试使用专用隔离数据库，绝不能填生产库。浏览器测试要求本机回环 PostgreSQL，测试会创建模拟账户与股票：

```powershell
$env:EXCHANGE_E2E_DATABASE_URL='postgres://postgres:isolated-test-only@127.0.0.1:15439/mmex_test?sslmode=disable'
npm run test:e2e --prefix frontend
```

先执行前端构建。E2E 从当前 Go 源码启动独立后端，默认 `127.0.0.1:18889`，并在 `127.0.0.1:5178` 提供当前 `frontend/dist/` 和同源 API 代理。不使用旧 `bin/` 程序，不连接生产服务。假数据夹具只在显式运行 `preview:demo` 或 `test:ui` 时启用，正式前端没有 Mock 模式。

## Cloudflare 静态前端与 API 直接回源

生产发布使用 **GitHub `wess09/AzurPilot_StockExchange` 的 `main` 分支 → Cloudflare Workers Builds → `azurpilot-stockexchange`**。CF 已绑定这个仓库和分支并开启自动构建；本地只进行修改、构建和验证，生产静态文件以仓库触发的构建为准，不通过本地 Wrangler 或 API 直传替代这条链路。构建设置如下，根目录是仓库内相对路径 `frontend`，不填 `/frontend`。

| 设置 | 值 |
| --- | --- |
| 根目录 | `frontend` |
| 构建命令 | `npm run build` |
| 部署命令 | `npx wrangler deploy` |
| Node.js | 22 或更新版本 |

配置文件为 `frontend/wrangler.jsonc`，静态目录为 `frontend/dist/`。Wrangler 由 Workers Builds 使用平台配置的部署凭据执行，上传整个静态目录，并配置 `stock.azurpilot.com/*` Route；配置不含 `main`、运行时绑定或 API 转发代码。无需本地 Docker、Go 镜像或数据库。已有部署命令附带 `--assets ./dist --compatibility-date 2026-11-03 --name azurpilot-stockexchange` 也可读取同一仓库配置。

正式域名的 DNS 继续指向现有 Go 源站、保持 Cloudflare 代理开启。在 Zone 的 Worker Routes 中保留以下规则；更具体的无脚本规则优先于整站静态路由：

| Route | Worker |
| --- | --- |
| `stock.azurpilot.com/api*` | 无（`script: null`） |
| `stock.azurpilot.com/healthz*` | 无（`script: null`） |
| `stock.azurpilot.com/*` | `azurpilot-stockexchange`，仅 Static Assets |

前两条规则在 Cloudflare Zone 层管理，末尾通配符同时覆盖查询参数。Wrangler 只发布最后一条属于本应用的规则，不会删除无脚本规则。无脚本 Route 的匹配行为见 [Cloudflare 路由文档](https://developers.cloudflare.com/workers/configuration/routing/routes/#matching-behavior)。不能把本站改为同域 Custom Domain，否则无法沿用这组源站绕过规则。

页面与 `/_next/*` 资源直接由 Static Assets 提供，旧 `/console` 导航使用 SPA fallback。API 与 SSE 直接走 Cloudflare 常规代理到原有 nginx / Go，不调用 Worker，不改写 Origin、Cookie、请求体或认证。身份、来源校验、缓存和 SSE 仍由原服务负责；不需要 CORS 或跨域会话。`workers.dev` 仅预览静态文件，不提供本站 API，完整功能使用正式域名。

按 [Cloudflare 静态资源计费说明](https://developers.cloudflare.com/workers/static-assets/billing-and-limitations/)，不执行 Worker 脚本的静态请求免费且不限量；API 的无脚本 Route 也不会消耗 Worker 调用额度。本配置不启用 Workers Caching。构建、存储及账户其他功能仍按平台自身规则处理。

构建后检查与只读线上验收：

```powershell
npm run check:worker --prefix frontend
npm run test:edge --prefix frontend
```

`check:worker` 只做 Wrangler dry-run，不发布。`test:edge` 仅执行公开/未认证读取，检查正式域名的静态资源、SPA 导航 fallback、API、ETag/304、Origin 与 SSE，不在生产创建账户或交易。仅本地假数据 Playwright 用例覆盖登录与交互。

本次配置相对于改动前仓库的 v2 后端不改变接口路径、方法、价格/时间单位与业务语义；新前端可配合原 v2 后端，原交易站、管理站与 AzurPilot 同步端也可继续使用原接口。Go 代码未变，不涉及新后端与旧客户端的契约变化。数据库、旧数据、实例签名、密码和会话无迁移；正式域名不变、Cookie 不重写，已有会话不因静态站切换被主动撤销。先配置无脚本 API 路由，再发布纯静态站；后续部署保留这两条绕过规则。回滚只需恢复完整旧静态版本，并保留 API 直接回源规则，不恢复数据库。

## Docker Compose 后端部署

仅提供 **一个纯 Go 后端服务**，不启动/管理 PostgreSQL、nginx、证书或前端构建容器。后端同时读取挂载的 `frontend/dist/` 静态文件。旧一键部署和 release 脚本已移除。

1. 在现有 PostgreSQL 中新建专用数据库与最小权限用户，例如 `mingmiao_exchange`。先备份现有数据库；不要清空现有库、复用其他应用的表或更改其用户密码。
2. 找到 PostgreSQL 容器现有 Docker 网络和容器 DNS 名称。复制 `.env.example` 为 `.env`，设置 `POSTGRES_NETWORK`、专用 `DATABASE_URL`，随机生成长 `ADMIN_PASSWORD`、`BOT_API_TOKEN`、`SESSION_SECRET`。服务器 `.env` 权限设为 600，密钥不进入 Git 或聊天记录。
3. 在服务器安装目录准备本仓源码、整个 `config/` 与已构建的整个 `frontend/dist/`。数据库不在镜像中；`docker-compose.yml` 不含任何数据库服务或数据卷清理动作。
4. 在该目录运行：

```bash
docker compose config --quiet
docker compose up -d --build exchange
docker compose ps
curl -fsS http://127.0.0.1:8888/healthz
```

Compose 仅映射 `127.0.0.1:8888`。沿用现有 `stock.azurpilot.com` nginx 反代和证书；SSE 路径需要禁用代理缓冲，应用也发送 `X-Accel-Buffering: no`。检查 `/`、`/console`、静态资源、API 和公网 HTTPS，不能仅以健康检查作为部署成功依据。

若 nginx 全局启用了 `proxy_cache`，本站的代理 location 需设置 `proxy_cache off;`，保证认证接口不被代理缓存，并让 `If-None-Match` 正常到达应用。域名使用 IPv6 时同时核对本站 IPv6 监听与证书，避免请求进入其他虚拟主机。初版部署见 [v2 交付记录](docs/V2_DELIVERY.md)；注册与功能恢复版本的交易所后端及配套交易站、管理站已部署，验收及回滚说明见 [2026-11-01 部署记录](docs/DEPLOYMENT-2026-11-01.md)。

将同一 `BOT_API_TOKEN` 写入插件目录内忽略的 `.data/connection.json`（格式见插件 README），无需改 AstrBot 全局配置。插件加载/热重载和官方机器人真机联调需在 AstrBot 中执行；本项目不会自动重启 AstrBot 容器。

### 只更新站点前端

在本机 `npm run build --prefix frontend` 后，把**整个** `frontend/dist/` 同步到服务器安装目录对应位置。保持该挂载路径不变，无需重建 Go、重启服务；替换目录内容要保留完整资产集合并最后更新 `index.html`，避免旧页面引用已删除资源。

Next.js 产物包含 `/_next/`，必须和唯一业务首页一起发布。所有视图地址均为 `/`，Worker 只需提供首页和静态资源，并沿用同源 `/api/v2` 转发；站内导航不会重新加载 HTML。已有 `/?stock=…` 链接仍可打开证券详情，旧哈希链接及既有 Go fallback 下的 `/console` 入口也会在客户端读取后归一到 `/`。视图保存在浏览器历史状态，复制根地址只会分享首页；外部分享证券时使用 `/?stock=…`。

本次迁移相对于改动前仓库的 v2 后端保留原有前端接口的路径、方法、单位和请求语义。市场列表可点击整行进入详情，最新价下的波动小图读取已有 `/public/stocks/{id}/chart` 端点，显示最近报价日的真实分钟收盘价。新前端可配合旧 v2 后端，旧前端也可继续配合原后端；未核实线上版本。数据库、密码、已有会话、实例签名与同步协议均无迁移，无需配套更新 AzurPilot 或 QQ 插件。回滚前端时恢复完整旧静态目录，不恢复数据库。当前本地完成状态及验证边界见 [前端设计与迁移说明](frontend/DESIGN.md)。

### 更新后端与恢复

后端运行代码更改需要在安装目录重新构建镜像并 `docker compose up -d --build exchange`。Compose 不更新外部 PostgreSQL 容器。保留旧镜像和发布目录，切换前对专用数据库执行 `pg_dump`；有数据库格式变化时，回滚镜像必须配套恢复对应数据库备份。

此次 v2 与改动前仓库的交易端、管理端、AzurPilot 代理和认证均不兼容，需要成套更新。旧 SQLite、账户和会话不自动迁入新 PostgreSQL；旧数据库保留原样，旧实例需要新客户端登记股票。生产服务器未发现旧交易所服务时可以直接部署独立 v2，不影响其他 PostgreSQL 数据库。不要以删除旧数据代替迁移。

## 密钥与运行配置

后端必填 `DATABASE_URL`、`ADMIN_PASSWORD`（至少16字符）、`BOT_API_TOKEN` 与 `SESSION_SECRET`（至少48字符）。默认域名 `stock.azurpilot.com`，默认本机监听 `127.0.0.1:8888`，Compose 内部监听 `0.0.0.0:8888`。`COOKIE_SECURE` 生产保持 true，仅回环 HTTP 开发设为 false。Cookie 为 HttpOnly，玩家与管理员会话隔离；“记住我”30天，否则浏览器会话有效。

`SESSION_SECRET` 同时保护会话与幂等密码重置回执，轮换会使已有会话失效；不要随每次重启重新生成。真实 `.env`、数据库、绑定码、密钥、构建产物及测试报告均不提交。

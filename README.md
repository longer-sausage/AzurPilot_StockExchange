# 茗喵证券交易所 · MMEX

以 AzurPilot 玩家总行动力为股价的模拟证券游戏。交易主界面位于相邻 `../AzurPilot/frontend/src/stock/`，本仓库 `frontend/` 只提供 React + TypeScript `/console` 管理控制台，Go 单进程提供 `/api` 和静态页面，SQLite 持久化账户、委托凭据、历史报价及月赛排名。默认 API / 页面域名为 `stock.nanoda.work`，通过环境变量修改。

## 本机体验

```powershell
npm ci --prefix frontend
npm run dev:mock --prefix frontend
```

需要 Go >= 1.24，或者已有 `bin/exchange.exe`（Linux 为 `bin/exchange`）。开发启动器优先使用这个二进制；否则运行 `go run ./cmd/exchange`，也可以通过 `GO_BIN` 指定 Go 的绝对路径。管理员控制台为 http://127.0.0.1:5178/console。交易主界面需从 AzurPilot 实例进入；交易所 origin 首页只显示入口说明。

在另一个终端进入相邻 AzurPilot 仓库，运行：

```powershell
$env:STOCK_EXCHANGE_URL='http://127.0.0.1:8080'
npm run dev:mock --prefix frontend
```

打开 AzurPilot 的 http://127.0.0.1:5173，进入实例运行总览，点击资源卡片调节键左侧的交易所入口。

- 玩家：`海风指挥官` / `mock-player-password`；也可直接注册新用户名。
- 管理员：`mock-admin-password`。
- 注册、登录和管理员登录都使用 Cloudflare 官方测试验证码，前端使用公开测试 site key `1x00000000000000000000AA`，Go 用官方测试 secret 调用 Siteverify，须联网；不再使用本地复选框或直接放行。测试密钥自动通过测试挑战，生产只接受真实密钥及真实验证响应。
- Mock 使用真实 Go 交易引擎和独立 `data/mock.db`；只替换验证码、种子行情和演示赛期。模拟数据库不进入版本控制。

普通开发模式先启动 Go，再运行 `npm run dev --prefix frontend`。Vite 将 `/api` 代理到本机 8080；生产管理控制台与 API 同源；玩家终端经 AzurPilot 的认证 WebSocket 转发，不在浏览器中保存交易会话、上传令牌或实例私钥。

## 游戏与交易

- 证券详情提供分时、日 K、M5/M10/M20/M30/M60、真实成交量和公开逐笔成交；原生终端使用现有 ECharts，支持 MA、EXPMA、BOLL、ENE、BBI、MACD、多空趋势、全屏及缩放平移。K 线按实际采集记录聚合；每月原始历史不截断为 2000 条。5 秒条件缓存刷新，隐藏页面停止轮询，指标计算留在浏览器。
- 新开户自动补传实例已保存的完整月份总行动力，包括注册前记录；旧实例及已开户账户首次升级会只读导入旧 CL1 统计库的 `ap_total` 月快照，与中央认证历史合并。同毫秒冲突优先中央记录，迁移状态与队列一起认证，复制或重建实例不继承旧统计。中央资源入库仅追加总行动力历史保存，游戏采集脚本不改动；持久补传日志按月轮转、每批至多 1024 点、最短 3 秒发送，断网与重启持续补传，失败最大 60 秒退避。每五分钟重读源数据并进行 count / SHA-256 校对，同毫秒修正会重算受影响 K 线。完整性仅涵盖实际采集、已保存的记录，升级前覆盖掉且未在旧统计库保留或离线未采集的数据无法还原；服务器不验证游戏真实性。

- 每位注册玩家自动上市一支股票，证券代码 `MM000001` 等，初始现金默认 **20,000,000 模拟币**，可在控制台调整（1 至 1 万亿，最多两位小数）。新开户采用发布时的金额，已有账户当前余额及本赛季本金保留；收益率按个人实际起始本金计算。用户名进行 Unicode 全角归一及大小写折叠后唯一，不回收已停用用户名。
- **1 点总行动力 = 1 模拟币/股**。行情直接同步实例现有资源记录，服务器不上传/识别截图，不验证行动力真实性。无最近记录时以 0 上市，后续收到新记录后更新；0 价股票暂停成交。旧实时 `/quotes` 接口只接受最近 24 小时内、按时间递增的整数记录，相同时间不同数值会拒绝，最短间隔 15 秒。新增历史接口允许当前及此前 12 个月的毫秒原始记录、同一时间修正和批量补传，也能及时更新新鲜的最新报价；客户端最短 3 秒一批。报价超时后暂停新成交，仍按最后有效报价估值、计借券费和执行风控。
- 买入、卖出、卖空、回补四个方向；市价、限价、止损触发市价；DAY / GTC；撤单释放冻结资金与股份。止损跳空时以触发后的实际报价成交，不保证触发价。不能交易自己的股票，反向持仓需先平仓。
- 所有成交由交易所提供无限**模拟流动性**，全量成交，不设置可借证券库存或融券定位要求。为保护数值和数据库，单笔数量最多 100 万股，单笔名义金额上限 1 万亿模拟币；可以在资金及保证金允许范围内多次提交，不设置融券库存总量限制。异常大额开仓还需符合 64 位账本安全范围，按最高可能报价核验，达到边界仍可平仓。没有真实五档盘口、部分成交、价格冲击或真实资产。
- 买入与卖空均可选择杠杆倍数；上限为 `max(1, floor(1 / IMR))`，最多 10 倍，默认 50% IMR 支持最高 **2 倍**。1 倍买入使用全额现金，2 倍买入只支付一半本金和全额交易费，其余记为融资借款。空头冻结所选杠杆对应的自有保证金及全部卖空所得现值，所得不能再次开仓。
- 融资利息按 `未偿本金 × 融资年利率 × 持有秒数 / (365 × 86400)` 累计，默认 8%，可在管理配置修改。卖出按股份比例偿还借款，仅净款进入现金交收；现金与融资买入可在同一证券中混合，保证金按股数精确加权，不重复冻结首付款。挂单只冻结自有资金，实际成交时才放贷；规则变更后不符合当前杠杆上限的开仓挂单会拒绝。
- 净值为现金 + 多头市值 − 融资本金 − 空仓负债 − 待落账融资利息与借券费。维持保证金为 `(仍有融资借款的多头现值 + 空仓现值) × MMR`；不足时撤销全部挂单、卖出多头偿还融资并回补空头，跳空债务保留至本季结束。融资多头跌价后的追加初始保证金差额会冻结可用现金。
- 借券费按 `空仓现值 × 年费率 × 持有秒数 / (365 × 86400)` 累计；变价前先结算旧价区间，小数分结转。金额以整数分保存；没有浮点余额或收费。
- T+N 可卖股份与 T+N 可用现金分别配置，跳过配置的周末与休市日期。关闭页面不会清除委托，停止服务后重启恢复持仓、挂单与累计费用。
- 每月 **5 日 00:00（上海时间）开赛，倒数第 5 日 23:59:59 结束**。例如 2026 年 10 月为 10 月 5 日至 10 月 27 日；闰年 2 月截止 25 日。后台最多 15 秒内完成到期结算，业务请求也检查赛期。按最后报价强制平仓并收取正常交易费，保存最终排名；次月 1 日按届时配置重置所有玩家本金（默认 2,000 万），5 日开放交易。长期离线后重启也会补结算。
- 控制台可编辑/新建/删除市场预设、暂停交易、调整初始资金、佣金、最低佣金、印花税、取整、征费、借券费、融资年利率、IMR、MMR、整手、交易时段、时区、休市日和赛期；新赛期日期从下月生效。已受理委托保留原手续费及交收快照，执行时仍接受当前保证金风控。
- 交易所收入按佣金、模拟印花税、征费、借券费、融资利息拆分，展示本季与累计值。现实的税费通常代收转付；本游戏统一列入模拟交易所统计，不能兑换、充值或提现。
- 新手学院提供六步教程、26 个词条和当前规则，明确说明融资杠杆、做空、保证金、交收和强平风险。玩家终端只保留市场、持仓、委托、排行榜与教程；没有手动行动力同步页面或控制台链接，后台自动上传仍持续运行。管理员直接访问 `/console`。
- 控制台的「用户管理」支持用户名、证券代码、身份码及实例 ID 查询、正常 / 停用筛选和分页；用户详情展示注册时间、永久绑定、行情、资金、交收、持仓及委托。可修改用户名、重设密码、增减现金与停用 / 恢复账户；现金扣减检查冻结、交收和保证金，保存失败整体回滚，重设密码或停用会撤销旧会话。
- 每位用户拥有独立、唯一且永久的随机身份识别码，旧账户升级时自动补发。仅本人和管理员能查看：本人点击 AzurPilot 交易终端右上角用户名，管理员打开用户详情。公开行情、排行榜与成交记录不含此码；改名、赛季重置和重启后保留，用于核实账户身份。

预设以 A 股、港股、美股规则作教学参考；佣金和借券费为可调游戏示例。港股手数统一简化，美股低价股最低保证金、币种/汇率、特定证券税收豁免及真实假期自动同步没有逐项复制。管理员应自行维护假期。

## 与 AzurPilot 连接

交易主界面和教程位于 `../AzurPilot/frontend/src/stock/`，由 `src/pages/StockExchange.tsx` 挂载为原生 React 页面。入口在运行总览资源卡片调节按钮**左边**，无需 iframe 或消息桥。本仓库 Go 提供 `/api`，React 提供 `/console`。

AzurPilot 后端环境：

```sh
STOCK_EXCHANGE_URL=https://stock.nanoda.work
# 本机开发用 http://127.0.0.1:8080，直接连接 Go API
```

注册或首次登录旧账户时，账户与当前实例的持久 UUID / Ed25519 身份**永久一对一绑定**。另一个实例不能登录该账户，同一实例不能注册第二个账户；退出登录只清除交易会话，不解除绑定，也不停止后台同步。复制实例配置不会复制身份。更换模拟器连接设置不改变实例身份。

实例身份文件和仅上传凭据分别保存在 AzurPilot 的 `cache/stock-exchange/identities/<UUID>.json` 和 `bindings.json`，复用游戏账号保险库的 AES-256-GCM 与本机密钥保护，不再明文保存私钥或上传令牌。Windows 使用用户 DPAPI，Linux 使用项目外 `0700/0600` 密钥目录；实例登记、文件摘要和历史检查点也保存在该目录。备份必须同时保留实例配置、中央 SQLite、`cache/stock-exchange/` 和项目外密钥/登记，并保持原项目路径、主机与用户保护环境。密钥或身份丢失不能靠新建同名实例恢复，不会自动降级或重建。交易会话只保存在 AzurPilot 后端内存，24 小时过期，后端重启后重新登录并完成验证码；上传凭据仍恢复后台同步。

AzurPilot 的总行动力历史追加保存 HMAC-SHA-256 哈希链，正常修正保留原事件；读取时验证整条链、当前值索引及项目外检查点。补传样本、游标和回执也全部认证。修改、删行、截断、单独回滚历史或替换凭据会停止同步，修改仪表盘 JSON 不会产生可上传的新采集记录。旧版首次迁移保留原身份与现存数据并建立基线，无法证明迁移前未被改动；本地认证也不证明游戏数据真实。

玩家绑定稳定 UUID，实例名称只用于显示及查找。API 或外部删除配置都会撤销本地会话、监视与凭据；同名重建、复制或导入配置不会继承原玩家。外部重命名保留 UUID 时沿用原绑定并迁移中央资源数据库，数据库冲突则停止并保留原件。删除后单独恢复旧配置不能复活原 UUID，交易所服务器上的玩家与永久绑定仍保留。

后台每 2 秒只比较配置文件 / 运行观察 SQLite 的修改状态，未变化时不重读配置、不签名、不访问交易所；仅转发新行动力记录，最多每 15 秒上传一次。失败在 15 秒后重试同条记录，最多 3 次，新记录重新开始。没有新增游戏截图、OCR 或游戏操作，也没有修改游戏任务脚本。实例签名只确认身份和请求完整性，不证明游戏数据真实。

**Cloudflare 域名配置必须包含 AzurPilot 地址栏 hostname**，以及管理控制台的 `stock.nanoda.work`。例如本地 AzurPilot 的 hostname 是 `localhost` 或 `127.0.0.1`，远程 WebUI 为 `pilot.example.com`；这些值同时加入 Turnstile 控制台允许域名与交易所 `.env` 的 `TURNSTILE_HOSTNAMES`。只给 stock 域名授权会使原生页面的注册和登录验证码被拒绝。公开 site key `0x4AAAAAAFMCQstp3hgd939a` 固定在 AzurPilot 玩家前端及本仓库管理前端，后端不读取、不下发 site key；更换时须修改两个前端并重新构建。私密 secret 只留在 Go 环境变量 `TURNSTILE_SECRET_KEY` 中。注册及登录必须通过服务端 Siteverify 校验，缺失、过期或来源/用途不匹配的验证码均拒绝。

`ALLOWED_ORIGINS` 控制浏览器直连 Go API 的来源；原生页面通过 AzurPilot 服务端代理，HTTP 请求无需浏览器跨域或嵌入权限。

## Debian 一键部署

推荐先在开发机 / CI 构建，服务器无需 Node、Go 或 C 编译器：

Windows 安装 Go >= 1.24、Node.js >= 18 和 PowerShell 7 后，在仓库根目录执行（使用系统自带的 `tar`，无需 Git Bash / WSL）：

```powershell
pwsh -NoProfile -File .\scripts\release.ps1
```

脚本先构建前端并运行本机 Go 测试，再交叉编译 Linux amd64 / arm64，生成 `releases/mingmiao-exchange.tar.gz`。运行结束后恢复原有 Go 环境变量；任一步失败会停止发布。Linux / Git Bash 可使用下面的 Bash 脚本构建，两个脚本生成相同结构的部署包：

```sh
bash scripts/release.sh
# 将 deploy.sh、.env.example 和 releases/mingmiao-exchange.tar.gz 上传到服务器同一目录
cp .env.example .env
chmod 600 .env
# 编辑 .env，填写真实密钥、密码、外部 TLS 证书/无密码私钥绝对路径及允许来源；先将域名 DNS 指向此服务器
sudo bash deploy.sh mingmiao-exchange.tar.gz
```

`deploy.sh` 支持源代码目录直接运行：`sudo bash deploy.sh`，没有构建产物时安装构建工具并按系统可用资源编译。极低内存服务器使用预构建包可避免构建开销。脚本仅支持 Debian amd64 / arm64，需要 root、systemd、可访问软件源，80 / 443 端口可用；已有服务器服务配置不会被删除。

填写以下环境变量（**不要将私密值发到聊天或提交 Git**）：

| 变量 | 用途 |
| --- | --- |
| `EXCHANGE_DOMAIN` | 默认 `stock.nanoda.work` |
| `TURNSTILE_SECRET_KEY` | Cloudflare 的服务器 secret，前端永不收到 |
| `TURNSTILE_HOSTNAMES` | Siteverify 可接受的 hostname，逗号分隔 |
| `ADMIN_PASSWORD` | 至少 12 字符 |
| `SESSION_SECRET` | 至少 32 字符的随机密钥，更换会使所有会话失效 |
| `ALLOWED_ORIGINS` | 浏览器直连 Go API 的 CORS 来源 |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | 必填，外部证书链及无密码私钥的可读绝对路径 |

脚本强制使用外部证书，不安装 certbot、不签发证书、不设置续期任务。未配置证书、文件不可读、已过期、域名不符或公私钥不匹配会拒绝部署。证书校验在停服及替换文件之前完成；HTTP 自动跳转 HTTPS。证书由外部流程维护，更新后执行 `nginx -t && systemctl reload nginx`。使用 Cloudflare 代理时设置 **Full (strict)**，Turnstile 中允许管理控制台及 AzurPilot 的实际 hostname。脚本从 Cloudflare 官方拉取受信代理网段，nginx 只信任这些来源的 `CF-Connecting-IP`，Go 只信任本机反代的真实 IP 头。

程序安装至 `/opt/mingmiao-exchange`，数据库独立保留于 `data/exchange.db`，每次升级停服后连同 WAL 备份。systemd 设置开机启动、自动重启和权限隔离；不再设置固定 CPU、Go 内存、进程内存或任务数上限，旧环境文件里的 `GOMAXPROCS=1` / `GOMEMLIMIT=64MiB` 也不再被部署脚本写入服务环境。Go 使用系统默认调度，认证按启动时可用 CPU 配置并发。数据库使用 WAL / NORMAL、单连接、2 MiB 页缓存；关闭系统时先完成在途请求。nginx 复用上游连接及 TLS 会话，异常请求在进入 Go 之前过滤；按 IP 防护，不设置整个服务的固定吞吐上限。

```sh
systemctl status mingmiao-exchange
journalctl -u mingmiao-exchange
sudo systemctl restart mingmiao-exchange
# 更新：上传新包后重新运行 deploy.sh；环境变量更改后重新部署或重启服务
```

环境文件按字面值解析，不执行 shell 代码。程序日志不输出密码、上传凭据、验证码 token 或数据库账户内容。生产脚本拒绝 `MOCK_MODE=true`；程序的 mock 监听也只能是回环地址。

## 前端部署与 `/console` 404 排查

本仓库的 `frontend/` 是管理控制台，访问地址为 `https://stock.nanoda.work/console`（使用不带末尾 `/` 的地址）。玩家交易页面位于相邻 AzurPilot 仓库，修改玩家页面后须另行构建、部署 AzurPilot 的前端。

生产部署链路为：浏览器 → nginx → `127.0.0.1:8080` Go 服务 → `FRONTEND_DIR` 下的静态文件。Go 二进制**不内嵌前端**，只上传后端程序不够；必须同时部署 `index.html`、`assets/` 及其他构建文件。`/console` 由 Go 返回 `index.html`，React 再显示管理控制台，无需创建 `console/` 目录。前端以域名根路径构建，资源路径为 `/assets/...`，API 路径为 `/api/...`，不要把整个 `dist` 放进 `dist/console/`，也不要将 Vite 的 `base` 改成 `/console/`。

### 推荐：部署完整发布包

在 Windows 开发机的仓库根目录执行：

```powershell
pwsh -NoProfile -File .\scripts\release.ps1
```

将生成的 `releases/mingmiao-exchange.tar.gz` 和仓库里的 `deploy.sh` 上传到服务器同一目录，首次部署还需 `.env.example` 并按前文填写 `.env`。在服务器上传目录执行：

```sh
# Windows 上传的旧脚本如存在 CRLF，先转换为 LF
sed -i 's/\r$//' deploy.sh
sudo bash deploy.sh mingmiao-exchange.tar.gz
```

脚本会校验发布包，将前端安装到 `/opt/mingmiao-exchange/frontend/dist`，在 `/opt/mingmiao-exchange/exchange.env` 中设置 `FRONTEND_DIR`，并配置 nginx 反向代理，HTTP / HTTPS 同时监听 IPv4 和 IPv6。如果使用了自定义 `INSTALL_DIR`，下面所有 `/opt/mingmiao-exchange` 路径须相应替换。

更新时上传新的发布包，再执行同一部署命令。脚本会停服备份数据库及 WAL、覆盖后端和前端、重启交易所，再重载 nginx；数据库和账户保留。完成前会比对运行中的后端与发布包二进制，并核对 IPv4 / IPv6 源站 `/console` 返回页面的 SHA-256 和 `/api/meta`。只有检查通过才显示“部署完成”。如果在 nginx 阶段失败，后端 / 前端可能已经更新，旧 nginx 配置仍可能继续提供页面，应修复报错并重新执行部署。

### 只更新管理控制台

在开发机仓库根目录执行（PowerShell / Linux 均可）：

```sh
npm ci --prefix frontend --no-audit --no-fund
npm run build --prefix frontend
```

把**整个** `frontend/dist` 目录上传到服务器仓库的 `frontend/dist`，确认文件已上传完整后，在服务器仓库根目录执行：

```sh
test -f frontend/dist/index.html
sudo install -d -m 755 /opt/mingmiao-exchange/frontend/dist
sudo cp -R frontend/dist/. /opt/mingmiao-exchange/frontend/dist/
sudo chmod -R a+rX /opt/mingmiao-exchange/frontend/dist
```

默认安装中仓库目录与 `/opt/mingmiao-exchange` 是两个目录，只在仓库中运行 `npm run build` 不会自动更新线上文件。Go 在请求时读取静态文件，同一路径下更新前端通常无需重启；如果修改了已安装服务的 `FRONTEND_DIR`，须执行 `sudo systemctl restart mingmiao-exchange`。生产环境不需要启动 Vite 开发服务器。

### 逐层定位 404

在 Debian 服务器执行，先确认前端文件和服务账号权限，再直接请求 Go，绕过 nginx 和 CDN：

```sh
ls -l /opt/mingmiao-exchange/frontend/dist/index.html
ls -l /opt/mingmiao-exchange/frontend/dist/assets/
sudo -u mmex test -r /opt/mingmiao-exchange/frontend/dist/index.html
sudo grep '^FRONTEND_DIR=' /opt/mingmiao-exchange/exchange.env
sudo systemctl status mingmiao-exchange --no-pager
curl -i http://127.0.0.1:8080/healthz
curl -i http://127.0.0.1:8080/api/meta
curl -I http://127.0.0.1:8080/console
```

预期 `/healthz`、`/api/meta` 和 `/console` 都返回 200，前两者为 JSON，最后一个为 HTML。健康检查不检查前端文件，只有 `/healthz` 成功不能证明控制台已正确部署。

| 检查结果 | 排查方向 |
| --- | --- |
| 本机 8080 连接失败 | 检查 systemd 状态及 `journalctl -u mingmiao-exchange -n 100 --no-pager` |
| 本机 `/api/meta` 为 200，但 `/console` 为 404 | 检查 `FRONTEND_DIR`、`index.html` 是否部署及 `mmex` 是否有目录访问 / 文件读取权限 |
| 本机接口和页面均为 200，但公网为 nginx 的 HTML 404 | 优先检查域名对应的 nginx `server` / `location`，以及 CDN 的源站地址、端口和 Host 配置 |
| 页面为 200，但 JS / CSS 为 404 | 检查是否上传完整 `assets/`、是否错误添加 `/console/` 前缀，以及资源请求是否也代理到 Go |

默认部署应将页面、静态资源和 API 都代理给 Go。检查**实际接收该域名 HTTPS 请求**的 nginx `server`（通常为 `/etc/nginx/sites-available/mingmiao-exchange`），确保包含以下通用路由；保留部署脚本生成的 TLS、真实 IP 和 API 限流配置：

```nginx
# 放在 server_name stock.nanoda.work 对应的 HTTPS server 内
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

检查是否存在重复的 `server_name stock.nanoda.work`，或其他更优先的 `location /console`、`location /assets/`、`location /api/` 将请求转到错误目录 / 服务。修改后校验配置并重新加载：

```sh
sudo nginx -T
sudo nginx -t && sudo systemctl reload nginx
# 在源站本机测试 HTTPS 虚拟主机，绕过 DNS / CDN；非公共信任证书需用 --cacert 指定相应 CA
curl --resolve stock.nanoda.work:443:127.0.0.1 -I https://stock.nanoda.work/console
curl --resolve stock.nanoda.work:443:127.0.0.1 -i https://stock.nanoda.work/api/meta
# 再检查公网链路
curl -I https://stock.nanoda.work/console
curl -i https://stock.nanoda.work/api/meta
```

如果源站 nginx 已返回 200、公网仍返回 404，检查当前 CDN / DNS 是否指向这台服务器，以及回源 Host 是否为 `stock.nanoda.work`；修复后再清除可能缓存的旧 404。控制台加载后若登录验证码失败，按前文配置 Turnstile 的允许域名、`TURNSTILE_HOSTNAMES` 和服务器 secret。

如果部署时 nginx 启动失败并报告 `Address already in use`，但 `ss -ltnp` 显示端口由已有 nginx 占用，可能存在未被 `nginx.service` 管理的 master，或 `/run/nginx.pid` 为空 / 过期。新版脚本会在 systemd 重载失败或服务未激活时扫描 `/proc`，核对 nginx 可执行文件、网络命名空间和 `/etc/nginx/nginx.conf`，对唯一匹配的 master 发送 HUP；不会写入 PID 文件或让 systemd 接管现有进程。只有没有 nginx master 时才尝试 systemd 启动，识别不明确时直接报错。原有启动 / 保活机制仍需维护；若要迁移到 systemd，先用 `ps` 和 `/proc/<master-pid>/cgroup` 确认现有启动来源，再安排迁移，避免影响同机其他站点。

如果 `nginx -t` 报 `limit_req_zone ... is already bound`，检查同名限流区是否被重复定义，以及站点是否被 `/etc/nginx/conf.d` 和 GMSSH 等面板的 `include` 同时加载。同一套站点配置只保留一个加载入口；限流区定义只保留一份，对应站点可以继续引用它。应先确认重复来源再调整，避免误删其他站点的配置。新版脚本在停服 / 替换交易所文件前检查现有 nginx 配置，已有配置无效时会提前退出。旧版部署失败后若交易所已被停止，可先执行 `sudo systemctl daemon-reload && sudo systemctl start mingmiao-exchange` 恢复后端，再修复 nginx 配置并重新部署。

还需分别验证 IPv4 / IPv6：上面的 `127.0.0.1` 只测试 IPv4。如果交易所只配置了 `listen 443 ssl;`，而其他站点配置了 `listen [::]:443 ssl;`，CDN 通过 IPv6 回源时可能命中其他站点，出现 IPv4 控制台为 200、公网为 404 的情况。可在服务器运行：

```sh
curl -kI --resolve 'stock.nanoda.work:443:[::1]' https://stock.nanoda.work/console
```

此处 `-k` 仅用于本机源站诊断。如果 IPv6 测试返回其他站点的 404，在交易所 HTTPS `server` 中保留 `listen 443 ssl;` 并添加 `listen [::]:443 ssl;`，HTTP `server` 中保留 `listen 80;` 并添加 `listen [::]:80;`，再运行 `sudo nginx -t && sudo systemctl reload nginx`。若服务器不使用 IPv6，则改为让 CDN 使用正确的 IPv4 源站。两种协议均返回 200 后，仍需确认 CDN 回源 Host / HTTPS SNI 与域名一致。

## 性能和数据边界

单进程内存账本 + SQLite 原子事务，提交失败恢复内存。事务仅复制、备份和落盘受影响账户；每个证券索引持仓/挂单订阅者，报价更新不扫描所有玩家进行成交。行情和排行榜共享编码缓存 + ETag，空闲市场最多每分钟重新编码，有空仓或融资持仓时每 15 秒刷新计费估值；交易或报价变动使缓存立即失效。浏览器每 15 秒检查，隐藏时停查；HTTP Date 用于校准界面时间。融资沿用现有账本、订阅索引与计息循环，不新增轮询、数据库查询或服务。所有资金变更由服务端核验，客户端费用估算仅供提交前参考。 定时任务仅遍历存在保证金持仓、待交收款项或挂单的账户；月赛结算仍统一处理所有玩家。时区规则缓存复用，IP 桶每分钟至多清理一次，避免异常访问反复扫描全部限流记录。

认证每 IP 每分钟最多 8 次，普通 API 每 IP 每分钟最多 240 次；CF 网络验证并发为 `max(4, 2×GOMAXPROCS)`，bcrypt 计算并发为 `GOMAXPROCS`，繁忙时立即返回 503，不堆积认证队列。使用 bcrypt 密码哈希、Siteverify 成功/hostname/action 三重核验；验证码过期/错误时必须重新完成挑战。每账户最多 20 个挂单；界面保留最近 100 个已结束委托；持久化幂等回执确保重复请求不会再次扣款。每支股票保存最近 2,000 条报价，接口返回最近 240 条；月赛归档保留在数据库，接口展示最近 12 个月。 nginx 设置 64 KiB 请求体、请求超时、每 IP 64 个处理中的连接、普通请求 10 次/秒（允许 40 次突发）及认证 8 次/分钟（允许 3 次突发），超限立即返回 429。这些防护限制单个访问来源，不限制所有玩家合计吞吐。

按诚实玩家假设使用实例自报行动力，不进行真实性验证。签名用于永久实例绑定和请求完整性；基本范围、格式、顺序检查只防止传输/账本错误。游戏禁止自我交易、同记录改价和倒序数据。未接入游戏账号身份证明、真实交易所行情、真实资金或交易所节假日服务。

## 验证

```sh
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -bench BenchmarkMarketCache -benchmem ./internal/exchange
bash -n deploy.sh scripts/release.sh
npm run build --prefix frontend
npx --prefix frontend playwright install chromium
npm run test:e2e --prefix frontend
```

引擎测试覆盖用户名归一/唯一性、初始资金、并发幂等、重启恢复、冻结/撤单、变价触发、借券费区间、T+N、周末/假期/DST、债务强平、月末/闰月、归档与重置、过期数据、上传凭据轮换、真实 Siteverify 的 hostname/action/失败结果、管理员权限、304、磁盘失败回滚、私有身份码迁移和休眠挂单不写盘。本仓库浏览器测试覆盖管理控制台预设发布及用户查询、资料编辑、资金校验、密码重设和停用恢复；AzurPilot 的独立交易所浏览器测试覆盖原生入口、验证码开户与登录、买入、撤单、教程、私有身份码、手机布局和跨实例拒绝。

AzurPilot 的独立同步测试为 `uv run python -m unittest tests.test_stock_exchange`，入口浏览器测试为 `npm run test:e2e:stock --prefix frontend`（在 AzurPilot 仓库执行）。Windows 原有 API 套件的一项符号链接测试需要系统创建符号链接权限；其余项目可正常验证。

## 参考资料

- [Cloudflare Turnstile 服务端验证](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/)：一次性 token、服务端校验和响应字段。
- [Cloudflare 真实访问 IP](https://developers.cloudflare.com/support/troubleshooting/restoring-visitor-ips/restoring-original-visitor-ips/)：只信任受信代理的 IP 头。
- [上交所交易规则（2026 年修订）](https://www.sse.com.cn/lawandrules/sselawsrules2025/stocks/exchange/c/c_20260424_10816482.shtml)：股份回转交易和交易时段参考。
- [港交所交易税费](https://www.hkex.com.hk/Services/Rules-and-Forms-and-Fees/Fees/Securities-(Hong-Kong)/Trading/Transaction?sc_lang=en)：印花税及交易征费参考。
- [FINRA 保证金参考](https://www.finra.org/rules-guidance/notices/21-12)：初始和维持保证金区分。
- [SEC T+1 交收](https://www.sec.gov/newsroom/press-releases/2024-62)：美股现金交收参考。

部署配置验证：`python scripts/check-deploy.py`（需已运行的 Docker）。使用临时 nginx 容器测试缺证书、错域名、错私钥及有效证书，并对脚本生成的实际 nginx 配置执行 `nginx -t`，不改宿主服务。

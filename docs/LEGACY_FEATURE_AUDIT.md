# 旧版茗交所功能与当前 v2 的差异清单

核对日期：2026-11-01（上海时间）。这份文档记录功能恢复前的基线。后续实现与验证见 [功能恢复交付说明](FEATURE_RESTORATION.md)，下表保留原始审计结论以便追溯。

## 核对范围与结论口径

- **当前交易所**：本仓库当前工作区，包括尚未提交的 v2 文件；不能用 `HEAD` 代替当前实现。本仓库 `HEAD` 是旧版 `a1081f5bb3fec1760155603946d147c7b9e2a828`。
- **旧交易所**：本地全部可达的 12 个提交，范围 `391e9de` 至 `a1081f5`，不是只比较重构前最后一个版本。已检查历史路由、模型、运行代码、前端、文档、测试名称和发布工具。
- **AzurPilot**：当前版本 `b003617858bce3cf9109e3c975a896870c218425`；重构前基线为其父提交 `dcf0489107ee0a4b4e3bdbb35c2b1ff6ad6fd79d`。同时追溯股票相关路径的全部 16 个变更提交，覆盖 `ea3795c85`、`b50f56cf8`、`2c93a4519`、`ebeacb5dd`、`d46ccdefa`、`22f51d0ca`、`2a285300d`、`aedc49bd9`、`02c7770a7`、`826c765a0`、`35386ced7`、`eb2a01624`、`63d443456`、`b30f6939d`、`c50cd8bac`、`b00361785`。重构前 `frontend/src/stock/` 共 26 个文件，均纳入核对。
- **官方 QQ 插件**：检查本地 `astrbot_plugin_mingmiao_stock` 的 README、指令和接口实现；该仓库没有可用的历史提交，不能据此虚构“旧版机器人曾经支持”的功能。
- 两个有历史的仓库均不是浅克隆。完整性边界是上述本地可达源码；未取回的远端分支、被删除且不可达的提交、其他旧仓库及服务器私有改动不在证据范围内。未访问生产服务，文中的“当前”不等同于重新核实了线上版本。

**月赛没有整体消失。** 当前仍有开赛/截止窗口、结算、排名归档写入、次月重置和退市；缺的是实时排行榜、归档读取接口及多项展示。`frontend/src/Rules.tsx` 仍展示当前赛季窗口，不能写成“完全没有赛季页面信息”。当前还存在融资、借息、保证金强平、T+N 和现实市场预设，不能因为原文件删除就判为功能全部删除。

下文每个编号是一项可以单独识别的能力、信息或行为。技术指标的参数、过滤选项等在对应项中明确列出；不把文件改名、依赖替换、颜色数值或每个测试断言分别计为功能。相同能力在多个页面的入口合并说明，前端缺少展示与后端数据/规则缺失分别判断。

| 标记 | 含义 |
| --- | --- |
| 缺失 | 当前没有完整的等价能力或数据链路 |
| 页面缺失 | 核心能力或数据仍在，用户/管理员入口或展示缺少 |
| 部分保留 | 仍有一部分能力，选项、精度、信息或保护减少 |
| 行为变化 | 实际语义已改变；记录事实，不直接认定为缺陷，也不默认授权恢复 |
| 按要求替换/移除 | 现行 AGENTS.md 或 v2 契约明确采用新方案，不能当作待恢复需求 |

每行末列的 `O`、`N` 是文末的历史与当前源码索引，均给出具体文件及关键符号。涉及行为差异的是源码契约审查结论，没有冒充运行测试结果。

共逐项记录 **209 项差异**。这个数量包含页面缺口、后端/数据缺失、规则行为变化和按要求淘汰的旧方案，**不等于 209 个缺陷或 209 项待恢复需求**。

| 类别 | 项数 |
| --- | --- |
| A 赛季、排行榜、收入 | 15 |
| B 证券列表、自选、筛选排序 | 21 |
| C 行情详情、图表、成交数据 | 36 |
| D 委托方式与交互 | 23 |
| E 持仓、损益、账户明细 | 20 |
| F 账本与风控语义 | 14 |
| G 管理控制台 | 25 |
| H 教学、身份、主题、弹窗 | 17 |
| I 历史同步、缓存、接口 | 13 |
| J 本地恢复入口 | 3 |
| K 架构替换与历史兼容 | 12 |
| L 开发、发布、运维工具 | 10 |

## A. 赛季、排行榜和交易所收入

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| A01 | 侧栏常驻月赛卡片，显示赛季编号及状态 | 页面缺失；规则页仍有赛季编号、起止时间和结算状态 | O1 / N1、N5 |
| A02 | 赛季进度条 | 页面缺失 | O1 / N1 |
| A03 | 开赛等待、距离截止天数、页脚精确截止时间 | 部分保留；有市场关闭原因和规则页窗口，没有原常驻倒计时与页脚截止提示 | O1 / N1、N5 |
| A04 | 当前赛季按最新净资产计算的实时排行榜接口 | 缺失；`Market()` 不返回 rankings，没有公开实时排名方法 | O10 `ranks/MarketJSON` / N2、N6 |
| A05 | 读取已结算赛季及最终排名 | 页面与 API 缺失；`advanceSeason` 仍写入并加载 `SeasonArchive`，但没有读取路由 | O10 `Seasons` / N2、N3、N6 |
| A06 | 当前/历史赛季选择器 | 页面缺失 | O1 / N1 |
| A07 | 前三名领奖台卡片 | 页面缺失 | O1 `RankCard` / N1 |
| A08 | 全部玩家的名次、名称、总资产、收益率表格 | 页面缺失 | O1 / N1 |
| A09 | 本人排名高亮、“我”标记 | 页面缺失 | O1 / N1 |
| A10 | 全局账户概览中的本人名次与参赛人数 | 缺失；股票数、已绑定股票数不能代替交易账户参赛人数 | O1、O10 / N1、N2 |
| A11 | 个人本赛季收益率，以个人实际起始本金为分母 | 部分保留；归档仍使用 `InitialCash` 算收益率，个人账户响应与页面未提供实时收益率/本金展示 | O1、O11 / N2、N3 |
| A12 | 排名排除停用账户和截止后才加入的账户，截止后新注册不改旧赛季结果 | 行为变化；当前归档循环直接加入全部账户，没有旧 `!Disabled && JoinedAt < EndsAt` 条件 | O10 `ranks`、旧 `LateJoinDoesNotChangeClosedSeasonRanking` / N3 `advanceSeason` |
| A13 | 修改月赛日期只影响后续月份，已建立的本月窗口保持 | 行为变化；当前 `SetSettings` 会直接重算同月份 `StartsAt/EndsAt` | O8、O10 / N2 `SetSettings` |
| A14 | 交易所本赛季收入总额及佣金、印花税、征费、借券费、融资利息分项；玩家与控制台展示 | 缺失；当前不维护旧 `Season.Revenue`，页面没有对应统计 | O1、O8、O10、O11 / N1、N2、N3、N4 |
| A15 | 交易所跨赛季累计收入 | 缺失；旧 `LifetimeRevenue` 数据、接口和管理统计均无等价物 | O8、O10、O11 / N2、N4 |

## B. 证券列表、自选和过滤排序

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| B01 | 列表及当前股票详情的星标添加/移除自选，操作幂等 | 缺失；无 watchlist 字段、接口或星标 | O1、O2、O10 `SetWatchlist` / N1、N2、N6 |
| B02 | 自选保存在账户，重启、换设备、改名、次月重置后仍保留，且不公开 | 缺失；不能用浏览器本地搜索条件代替账户持久化自选 | O2、O10 / N2 |
| B03 | “自选股”列表及数量 | 页面缺失，且依赖 B01/B02 | O2 / N1 |
| B04 | “持仓证券”列表范围 | 页面缺失；当前只有名称/编号搜索 | O2 / N1 |
| B05 | 默认隐藏本月退市股票，切换未退市/全部/已退市 | 页面缺失；当前市场列表直接包含全部股票，仅详情标记退市 | O2 / N1 |
| B06 | 可交易、过期、零报价、停牌等报价状态过滤 | 页面缺失；后台零价/过期校验仍在 | O2 / N1、N3 |
| B07 | 上涨/下跌/平盘/无开盘价方向过滤 | 页面缺失 | O2 / N1 |
| B08 | 我的股票/其他玩家/全部归属过滤 | 页面缺失 | O2 / N1 |
| B09 | 最低与最高价格组合过滤 | 页面缺失 | O2 / N1 |
| B10 | 最低与最高涨跌幅组合过滤 | 页面缺失 | O2 / N1 |
| B11 | 最近 1、5、30 分钟或 24 小时报价过滤 | 页面缺失 | O2 / N1 |
| B12 | 展开过滤器、显示已调整条件数、一键重置条件及排序 | 页面缺失 | O2 / N1 |
| B13 | 证券代码正序/倒序 | 页面缺失；当前固定按后端 ID 顺序展示 | O2 / N1、N2 |
| B14 | 中文名称正序/倒序 | 页面缺失 | O2 / N1 |
| B15 | 价格高到低/低到高，点击表头切换 | 页面缺失 | O2 / N1 |
| B16 | 涨跌幅双向排序，未知开盘价后置 | 页面缺失 | O2 / N1 |
| B17 | 报价更新时间新到旧/旧到新 | 页面缺失 | O2 / N1 |
| B18 | 顶部多股票行情条，点击直接切换证券 | 页面缺失 | O1 / N1 |
| B19 | 列表中的今日开盘至最新价方向小图 | 页面缺失；旧 `Sparkline` 是方向示意，不能误写为完整历史迷你走势图 | O2、O3 / N1 |
| B20 | “我的股票/我”标记；默认优先选择其他未退市、未停牌股票 | 页面缺失；当前默认选择第一支股票 | O1、O2 / N1 |
| B21 | 列表与报价标题直接显示过期、停牌、有效报价状态 | 部分保留；有本月退市标记和最新上报时间，没有旧 stale/disabled 状态链路 | O1、O2、O5、O10 / N1、N2 |

## C. 行情详情、图表和成交数据

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| C01 | 上海时间今日第一条已保存报价作为开盘价，列表与详情涨跌幅统一基于该开盘价；补传/修正/跨日会重算 | 缺失且口径改变；当前使用 `previous` 前次报价，`publicStock` 没有 open | O3、O5、O10 `openingPrices` / N1、N2、N6 |
| C02 | 所选交易日最高价摘要 | 页面与摘要 API 缺失；原始 OHLC 的 h 仍在 | O5 / N1、N6 |
| C03 | 所选交易日最低价摘要 | 页面与摘要 API 缺失；原始 OHLC 的 l 仍在 | O5 / N1、N6 |
| C04 | 昨收/前一个有行情日收盘价及参考线 | 缺失；前次上报不等同于昨收 | O3、O5 / N1、N6 |
| C05 | 所选日与每根 K 线的真实成交股数 | 缺失；当前 `Candle.n` 是行动力样本数，不是成交量；小时额度用量也不是历史成交量 | O3、O5、O10 / N1、N2、N6 |
| C06 | 所选日与每根 K 线的真实成交额 | 缺失 | O3、O5 / N2、N6 |
| C07 | 买入/回补与卖出/卖空的成交方向统计及比例条 | 缺失 | O5 / N1、N6 |
| C08 | 独立证券详情视图、更大图表、返回市场和证券资料区 | 页面缺失；当前保留市场中的简化股票面板 | O1、O5 / N1 |
| C09 | 指定一天的 1 分钟分时图 | 页面缺失；当前“1日”为最近 24 小时的 5 分钟聚合，不是旧功能等价物 | O3 / N1 |
| C10 | 真正的日 K 蜡烛图（开、高、低、收） | 页面缺失；当前画收盘折线和高低竖线，后端仍提供 OHLC | O3 / N1、N6 |
| C11 | 独立选择 M5、M10、M20、M30、M60 K 线周期 | 页面缺失；新 API 可传分钟 interval，但当前 UI 仅按 1日/1周/1月/1年自动选择 5/60/1440 分钟 | O3 / N1、N6 |
| C12 | 月份选择，查阅此前月份 | 页面缺失；后端 from/to 仍可查历史范围 | O3 / N1、N6 |
| C13 | 月内分时日期选择，默认定位该月最近有行情的一天 | 页面缺失 | O3、O5 / N1 |
| C14 | MA5、MA10、MA20 | 缺失 | O3 / N1 |
| C15 | EXPMA12、EXPMA50 | 缺失 | O3 / N1 |
| C16 | BOLL(20,2) 上中下轨 | 缺失 | O3 / N1 |
| C17 | ENE(10,11,9) 上中下轨 | 缺失 | O3 / N1 |
| C18 | BBI（MA3/6/12/24 的均值） | 缺失 | O3 / N1 |
| C19 | MACD(12,26,9)、DIF、DEA 与柱状图 | 缺失 | O3 / N1 |
| C20 | 基于 BBI 的多头/空头/震荡/样本不足趋势说明 | 缺失 | O3 / N1 |
| C21 | 分时累计成交加权均价线，无成交时不画假均价 | 缺失 | O3 / N1、N6 |
| C22 | 成交量副图与“成交量＋MACD”副图切换 | 缺失 | O3 / N1 |
| C23 | 十字光标、跨副图联动、逐点 OHLC/量额/样本/指标提示 | 页面缺失；当前 SVG 无逐点交互 | O3 / N1 |
| C24 | 鼠标滚轮、拖拽、双指缩放平移和可视范围滑块 | 页面缺失 | O3 / N1 |
| C25 | 缩放、左右平移、重置按钮，以及方向键/加减键操作 | 页面缺失 | O3 / N1 |
| C26 | 图表全屏、退出按钮及 Escape 退出 | 页面缺失 | O3 / N1 |
| C27 | 实时刷新保留当前图表视野，切换证券/周期/日期才重置 | 页面缺失；当前没有可调整的图表视野 | O3 / N1 |
| C28 | 多指标同时叠加、图例末值、指标说明开关、样本不足不画未形成均线 | 缺失 | O3 / N1 |
| C29 | 月原始记录数、最近校对时间的完整性提示 | 页面缺失；月 count/digest 仍可通过实例接口取得，checkedAt 已无等价字段 | O3、O5、O12 / N1、N6 |
| C30 | 日 K 提前取三个月、分钟 K 提前取两天用于指标预热，`displayFrom` 区分显示范围 | 缺失 | O3、O5 / N1、N6 |
| C31 | 日 K 按上海午夜分桶 | 行为变化；当前 Store.Chart 使用 Unix 毫秒整除，1440 分钟桶按 UTC 午夜，即上海 08:00 对齐 | O5 `periodBucket` / N6 `Chart` |
| C32 | 指定分钟周期保持该周期，不在长区间请求中隐式改粗 | 行为变化；当前超过约 2000 桶时自动扩大 interval。原始历史仍完整，不能把它写成原始历史被截断 | O5 / N6 `Engine.Chart` |
| C33 | 所选月份完整公开逐笔成交，含玩家、方向、时间、价格、数量、成交方式与强平标记 | 缺失；当前只返回本人订单汇总 | O5、O11 / N2、N3、N6 |
| C34 | 指定证券的全部公开待触发委托列表，含玩家、方向、限价、类型和有效期 | 缺失；当前只有本人委托，未提供公开挂单接口；旧功能也不是真实五档盘口 | O5 / N2、N6 |
| C35 | 独立持久化实际成交事件，并在有成交、无新观测的周期生成 samples=0 的实际成交 K 线 | 缺失；当前订单只累计 Filled/Notional/AveragePrice/UpdatedAt，无法还原每次部分成交的完整时间与价格序列；图表 SQL 只读行动力历史 | O5、O11、O12 / N2、N3、N6 |
| C36 | 图表独立加载状态，刷新失败时在上次成功图形上提示重连/错误 | 部分保留；当前有错误提示，但无独立 loading 状态，失败时用 Alert 替换图形；切换查询等待时也没有旧加载覆盖层 | O3 `FinancialChart/useStockDetail` / N1 `StockPanel` |

## D. 委托方式与交易交互

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| D01 | 市价委托 | 缺失；当前必须提供正数 price，只有限价委托 | O4、O11 / N1、N2、N3 |
| D02 | 止损触发市价单，触发后按实际报价成交并提示跳空风险 | 缺失；模型没有 kind/stop 条件 | O4、O11 / N2、N3 |
| D03 | DAY 及市场时区次日零点自动失效、释放冻结、显示 expired | 缺失；没有 TIF/ExpiresAt 和对应到期状态 | O4、O11 / N2、N3 |
| D04 | 明确选择 GTC，本赛季成交/撤销/截止前有效 | 部分保留；当前挂单实际上可持续到赛季关闭，但无 DAY/GTC 选择及有效期说明字段 | O4、O11 / N1、N2、N3 |
| D05 | 报价一达到条件立即触发原有挂单 | 按要求替换；当前余量遵守后续整点撮合与优先级规则，不能擅自恢复旧触发时机 | O11、O12 / N3、API-v2 |
| D06 | 卖空也能选择 1 至 IMR 上限、最多 10 倍杠杆 | 页面缺失；当前网页仅买入可选杠杆，卖空固定传 1；后端仍支持合法卖空杠杆 | O4 / N1 `Trade`、N3 |
| D07 | 买入按 IMR 动态提供全部合法整数杠杆，最多 10 倍 | 部分保留；当前网页只有现金与 2 倍，后端仍支持更多合法倍数 | O4 / N1、N3 |
| D08 | 按整手 1、5、10 倍快捷填写数量 | 页面缺失 | O4 / N1 |
| D09 | 输入框最小值和 step 跟随当前 lotSize | 页面缺失；当前 UI min=1/step=1，后端整手校验仍在 | O4 / N1、N3 |
| D10 | 交易表单内实时显示参考金额、预计费用及自有资金占用 | 部分保留；当前需先发起服务器预览才显示明细，没有旧表单即时计算区 | O4 / N1 |
| D11 | 交易表单常驻本人可用资金 | 页面缺失；当前可用资金在持仓页 | O4 / N1 |
| D12 | 表单随方向显示可卖股份或可回补空仓 | 页面缺失；后端持仓/数量限制仍在 | O4 / N1、N2、N3 |
| D13 | 卖出预先展示按比例归还融资本金 | 缺失于预览展示；当前 preview 的卖出 estimatedTotal 只减税费，未减该次应还贷款，实际成交仍按比例还贷 | O4 / N1、N3 `Preview/fill` |
| D14 | 选择杠杆后直接显示 IMR 上限、MMR 强平门槛及风险警告；卖空显示借券年费率 | 部分保留；规则页有参数，交易表单没有原有上下文提示 | O4 / N1、N5 |
| D15 | 自己股票、过期报价、停牌、未开市在表单直接禁用并给具体原因 | 部分保留；当前禁用覆盖退市/未绑定卖空，其他原因主要等待服务器预览报错；后端校验仍在 | O4 / N1、N3 |
| D16 | 确认弹窗明确显示选择的杠杆、自有保证金与融资/还贷信息 | 部分保留；有 financing，未展示所选杠杆和卖空 collateral 等原有明细 | O4 / N1 |
| D17 | 独立委托导航及待触发数量徽标 | 页面缺失；目前合并为“持仓与委托” | O1、O4 / N1 |
| D18 | 市场首页底部“当前委托/全部成交与委托”快捷页签 | 页面缺失 | O1 / N1 |
| D19 | 订单列表展示委托号、开仓杠杆、委托类型及有效期 | 部分保留；订单 ID、leverage 在响应仍有，但表格不显示；kind/TIF 已无数据 | O4 / N1、N2 `publicOrder` |
| D20 | 订单列表展示实际成交价格和成交时间 | 部分保留；当前展示限价，未展示响应中的 averagePrice/updatedAt；updatedAt 也不等于完整逐笔成交时间 | O4 / N1、N2 |
| D21 | 订单列表展示该笔实际交易费用 | 页面缺失；当前响应仍有 tax/commission/stamp/levy，列表未展示 | O4 / N1、N2 |
| D22 | 行情连接中断时保留上次数据，并阻止页面继续提交交易 | 部分保留；当前保留数据与重连提示，但 Trade 仍接收 account，没有旧 online=false 时停止提交的客户端保护；服务器的报价/交易校验仍在 | O1 的 `account={online?account:undefined}`、O4 / N1 |
| D23 | 挂单触发后因资金等条件不足而无法成交时，记 rejected、给拒绝原因并释放冻结 | 行为变化；当前不足以成交时可能继续 pending，部分错误改为 cancelled；没有旧 processOrders 的统一拒绝终止语义 | O11 `processOrders` / N3 `fill/auction` |

## E. 持仓、损益和账户明细

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| E01 | 总浮动盈亏汇总 | 页面缺失 | O2、O4 / N1 |
| E02 | 多头浮动盈亏汇总 | 页面缺失 | O2、O4 / N1 |
| E03 | 空头浮动盈亏汇总 | 页面缺失，且依赖 E20 的成本口径 | O2、O4 / N1、N2 |
| E04 | 每笔持仓平均成本 | 页面缺失；多头总成本仍返回 | O4 / N1、N2 |
| E05 | 持仓表内当前价格 | 页面缺失；可从行情取得 | O4 / N1、N2 |
| E06 | 持仓表内多头市值/空头负债 | 页面缺失 | O4 / N1、N2 |
| E07 | 每支证券浮动盈亏及正负色彩 | 页面缺失 | O4 / N1 |
| E08 | 点击持仓证券直接进入该证券详情和交易 | 页面缺失；当前持仓名称为普通单元格 | O4 / N1 |
| E09 | 账户汇总融资借款余额 | 页面缺失；各持仓 loan 仍返回并展示 | O4、O10 / N1、N2 |
| E10 | 账户汇总做空负债 | 页面与专门汇总字段缺失；可由数量与现价计算 | O4、O10 / N1、N2 |
| E11 | 账户初始保证金总额 | 缺失于旧统计口径；当前仅有每笔空仓 collateral | O4、O10、O11 / N1、N2、N3 |
| E12 | 账户维持保证金总额 | 缺失于接口和页面 | O4、O10 / N1、N2、N3 |
| E13 | 融资多头初始保证金及追加冻结差额明细 | 缺失；相关旧计算也未保留，见 F03 | O4、O10、O11 / N1、N2、N3 |
| E14 | 待交收现金总额展示 | 页面缺失；当前仍返回 settlements 并限制可用现金 | O1、O4 / N1、N2、N3 |
| E15 | 未落账融资利息单列 | 缺失于账户响应及展示；当前按操作/边界实际计费，不提供旧 as-of-now 估算 | O4、O10 / N1、N2、N3 |
| E16 | 未落账借券费单列 | 缺失于账户响应及展示 | O4、O10 / N1、N2、N3 |
| E17 | 本赛季已实现盈亏累计 | 缺失；`Account` 无旧 `Realized` 字段，平仓不累计同等指标 | O4、O10、O11 / N2、N3 |
| E18 | 本人本赛季已付佣金、印花税、征费、融资利息、借券费分项 | 缺失；订单部分税费可查，不等于包含持有费用的完整账户累计 | O4、O10、O11 / N2、N3 |
| E19 | 总冻结分解为挂单、空头负债/保证金、融资追加冻结，且在行情页常驻资金概览 | 部分保留；当前“冻结资金”使用 reserved，只是挂单冻结，空头 escrow 独立于 cash；不能与旧 Frozen 指标直接对比 | O1、O4、O10 / N1、N2、N3 |
| E20 | 空头按扣开仓费用后的账面成本计算持仓盈亏，并支持部分回补按比例释放成本 | 行为变化；当前 shortProceeds 保留毛卖空所得，short 不记录旧净成本，不能直接代入旧浮盈亏公式 | O2、O4、O11 / N2、N3 `fill` |

## F. 账本、风控和交易规则的细节变化

这些项目影响规则语义。列入审计不代表应直接恢复，尤其不能用旧机制覆盖有限库存、小时额度及发行者补偿的新规则。

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| F01 | 以整个账户净值对比全部融资多头和空仓总维持保证金 | 行为变化；当前 `riskLong/liquidateStock` 按单支持仓权益判断，未采用旧账户合并判断 | O10 `account`、O11 `risk` / N3 |
| F02 | 账户风险不足时撤销全账户挂单，先卖全部多头、再回补全部空头 | 行为变化；当前融资风险关闭相关股票及其挂单；空头风险虽取消全账户挂单，但只回补该股票 | O11 `forcedClose` / N3 `closePosition/riskLong/liquidateStock` |
| F03 | 用精确 `marginUnits` 处理混合现金/融资持仓，按现价计算多头初始保证金并冻结追加差额 | 缺失；当前保留按比例还贷，没有旧 marginUnits/capitalMargin/追加冻结计算 | O10、O11 / N2、N3 `availableCash` |
| F04 | 空仓按当前市值与持仓保证金率动态计算冻结/可用资金 | 行为变化；当前以开仓 proceeds/collateral 独立 escrow 和单股风险检查代替旧动态资金计算 | O10、O11 / N2、N3 |
| F05 | 读取账户和排名时，从净值、可用资金中扣除截至当时尚未落账的融资/借券费用 | 行为变化；当前 `equity/availableCash` 不即时估算自上次计提以来的费用 | O10 `account` / N2、N3 |
| F06 | 每秒推进持仓计息、风险和订单到期，页面时钟也得到更新 | 行为变化；主循环仍每秒调用 Tick，但当前 nextTick/auction 通常到小时或开闭市边界才处理持仓，不能说仍等价于旧每秒计提风控 | O10 `Tick`、旧 realtime / N2 `Tick`、N3、N6 |
| F07 | 跳空强平的未偿损失保留为交易者负现金债务，直到赛季重置 | 行为变化；当前 forced sell/cover 对实际支出做资金上限裁剪，旧负现金债务语义未保留；发行者仅收实际可收部分仍按新规执行 | O11 `fill/forcedClose` / N3 `fill` |
| F08 | 借券费用足额记入现金，即使因此形成债务 | 行为变化；当前 accrue 把 charge 限制到可用现金加 collateral，超出部分没有旧独立欠费账 | O10 `accrue` / N3 `accrue` |
| F09 | 融资利息与贷款本金分开，以现金费用入账，未偿本金不因利息不足自动增加 | 行为变化；当前 `accrueFinancing` 将未付部分加入 Loan，后续计息基数因此不同 | O10、O11 / N3 |
| F10 | 已有反向开仓挂单时禁止再挂相反方向的开仓单 | 缺失于对应准入检查；当前只检查已有持仓，pendingQuantity 仅用于卖出/回补数量 | O11 `validatePosition` / N3 `validateOrder` |
| F11 | 挂单真正成交时再次检查反向持仓冲突 | 缺失于旧同等保护；当前 fill 未调用完整 validateOrder 或重新检查 buy/short 反向持仓 | O11 `fill/validatePosition` / N3 `fill` |
| F12 | 把全账户现有持仓及待开仓数量合并，按最高可能报价检查累计整数安全空间 | 部分保留；当前有单笔数量、价款、库存和资金限制，但未找到旧逐项 consume 的账户累计最坏报价准入；这是静态保护差异，不声称已触发实际溢出 | O11 `validatePosition` / N3 `validateOrder` |
| F13 | 注册时无近期记录、以 0 报价开户，也立即按当月退市规则判断；已有低价候选不要求报价仍新鲜 | 行为变化；当前 InstanceRegister 不执行旧注册退市判断，delistStock 排除 ObservedAt=0，开赛检查排除过期报价 | O10 `Register`、旧 delisting / N3、N6 |
| F14 | 历史批次中夹有最新、新鲜低价记录时，仍对该最新记录执行当前退市判断 | 部分保留/行为变化；当前 AzurPilot 只要同批含超过 24 小时的点就标记整个批次 backfill，Go 因此跳过 delistStock；保证金风控仍执行，后续实时报告或其他触发可能补做退市 | O12 `IngestHistory`、O13 / N3、N6 `InstanceReport`、N7 `synchronize` |

## G. 管理控制台

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| G01 | 按用户名、证券代码、身份码、实例 UUID 搜索用户 | 缺失；当前 accounts 接口和页面全量列出，没有这些查询参数 | O9 / N2、N4、N6 |
| G02 | 正常/停用/全部账户筛选 | 页面/API 缺失；disabled 字段仍存在 | O9 / N4、N6 |
| G03 | 用户列表分页、总数、页码及记录变化后的有效页回退 | 缺失 | O9 / N4、N6 |
| G04 | 正常与停用账户分别计数 | 页面与独立汇总响应缺失 | O9 / N4、N6 |
| G05 | 用户列表显示注册时间、持仓数、待触发委托数与报价状态 | 部分保留；createdAt 仍返回，当前表格未展示，持仓/挂单数和旧 stale 汇总不返回 | O9 / N2、N4 |
| G06 | 指定用户完整详情，基本资料/资金交收/持仓/委托四类视图 | 缺失；当前“管理”只有现金绝对值和停用开关，没有详情读取接口 | O9 / N2、N4、N6 |
| G07 | 详情展示个人起始本金、行情记录时间和服务器最近上传时间 | 部分保留；有 stock.observedAt 和账户 createdAt，未展示旧完整资料，UploadedAt 字段消失 | O9、O10 / N2、N4 |
| G08 | 私有查看绑定 UUID、公钥、绑定指纹 | 页面缺失；AdminStocks 仍返回 instanceId/publicKey，但没有旧账户详情关联展示及 binding.key 展示 | O9 / N2、N4 |
| G09 | 查看其他用户的持仓、融资余额、可卖数量及保证金资料 | 缺失；当前管理员账户列表不返回 positions，也没有指定用户 Portfolio 路由 | O9 / N2、N4、N6 |
| G10 | 查看其他用户的全部委托与成交明细、冻结和拒绝原因 | 缺失 | O9 / N2、N4、N6 |
| G11 | 查看其他用户资金交收金额与可用日期 | 页面缺失；admin accountPublic 仍含 settlements，UI 不展示 | O9 / N2、N4 |
| G12 | 管理员修改用户名，并保持原账户、身份码及绑定 | 缺失；UpdateAccount 只接受 cash/disabled | O9 / N2、N4、N6 |
| G13 | 管理员在控制台重置指定用户密码 | 缺失于管理员能力；用户经机器人二次确认重置仍在，不能写成“系统不能重置密码” | O9 / N2、N4、N6、N8 |
| G14 | 用正负增量调整现金，保留个人赛季本金 | 行为变化；当前设置绝对 cash，没有 cashAdjustment | O9 / N2、N4 |
| G15 | 现金调整携带 expectedCash，余额变动时拒绝旧页面覆盖 | 缺失；当前绝对值覆盖无此并发保护 | O9 `UpdatePlayer` / N2 `UpdateAccount` |
| G16 | 扣减现金前核验挂单冻结、待交收、未落账费用及账户 MMR | 部分保留；当前主要检查 cash >= reserved，未做旧完整资金/保证金约束 | O9 / N2 `UpdateAccount`、N3 |
| G17 | 停用时明确二次确认，并强平本人持仓、撤销本人挂单与相关冻结 | 部分保留；当前撤销会话，但没有旧停用确认、立即清仓及撤单流程 | O9 `disablePlayer` / N2 `UpdateAccount`、N4 |
| G18 | 管理员可使指定股票停牌/恢复；旧版通过停用发行账户联动 | 缺失于单股票控制；v2 账户与股票独立，停用账户不会设置 Stock.Disabled，也没有单股暂停字段。恢复旧联动须先确认是否符合新独立账户设计 | O9、O10、O11 / N2、N3、N4 |
| G19 | 停牌时自动撤销其他用户针对该股票的挂单 | 缺失；当前停用绑定交易账户不执行旧 watcher 撤单 | O9 / N2、N3 |
| G20 | 管理端订阅行情变更，更新市场状态及用户列表/详情 | 页面缺失；当前 load 在进入、保存或手动刷新时调用，未订阅事件 | O8、O9 / N4 |
| G21 | 输入新 ID/名称，将当前草稿另存为新预设 | 页面缺失；后端 marketPresets 数组仍可整体提交 | O8 / N4、N6 |
| G22 | 将编辑的生效副本更新回已保存预设 | 页面缺失；当前只修改 activeMarket，不更新对应 marketPresets 项 | O8 / N4 |
| G23 | 删除预设并至少保留一个 | 页面缺失；后端仍校验预设数组非空 | O8 / N4、N6 |
| G24 | 编辑预设说明和参考来源 URL | 页面缺失；后端 Description/Source 字段和校验仍有 | O8 / N4、N2、N6 |
| G25 | 用百分比与模拟币编辑费率/印花税取整，自动转换 ppm/分 | 部分保留；现金字段仍用模拟币，多个费率改为直接填写 ppm，取整单位改为分，原人类单位输入体验不再提供 | O8 / N4 |

## H. 教学、身份信息、主题与弹窗体验

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| H01 | 新手六步教程，选择步骤、下一步、重新学习 | 页面缺失；当前详细交易规则不等于步骤式教程 | O6 / N5 |
| H02 | 交易名词词典及每个术语的独立解释 | 页面缺失；旧实际源码有 **37 个词条**，不是旧 README 所写的 28 个；完整词条见附录 | O6 / N5 |
| H03 | 按关键词搜索术语、逐项展开解释、无结果反馈 | 页面缺失；当前规则页不可按术语检索 | O6 / N5 |
| H04 | 独立、随机、永久的私有 MMEX 身份识别码；本人卡片、复制及管理员核实 | 缺失；数字账户 ID、股票绑定码、官方 appid:openid 用途不同，不能认定为原身份码仍在 | O7、O9 / N2、N4、N5 |
| H05 | 用户名先 NFKC 全半角归一，再 Unicode case folding 后唯一/登录 | 行为变化；当前 normalized 只有 TrimSpace＋ToLower；旧全角和部分 Unicode 等价名保护未保留 | O10 `normalizeName`、O9 / N2、N6 `auth/instance` |
| H06 | 开户前完整告知公开、持续同步行动力，勾选同意并由后端校验 noticeVersion | 缺失；当前页面有说明，但无同意勾选、版本字段及对应服务器校验 | O14 / N5、N6、N7 |
| H07 | 茗交所独立暗色/亮色切换，选择跨刷新持久化，图表与弹窗同步 | 缺失；当前独立交易站固定亮色样式；AzurPilot 注册页继承宿主主题不等于原交易终端主题功能 | O15 / N1、N5、N7 |
| H08 | 弹窗通过 createPortal 挂到 body，脱离祖先布局/变换层 | 部分保留；当前使用 fixed 遮罩，但普通 Modal 不使用 portal | O15 / N5 `Modal` |
| H09 | 弹窗跟随 visualViewport 的高度、宽度及偏移，适配手机软键盘与页面放大 | 缺失于专门处理；当前只使用 dvh/媒体查询 | O15 / N5 |
| H10 | 弹窗头尾固定、正文独立滚动，始终能看到关闭和确认按钮 | 部分保留；当前整个 .modal overflow:auto，头部和提交按钮随内容滚动 | O15 / N5 |
| H11 | 弹窗打开时给背景元素设置 inert，关闭恢复 | 缺失；当前有锁滚动与基础 Tab 循环，未设置背景 inert | O15 / N5 |
| H12 | 点击空白遮罩关闭允许关闭的弹窗，忙碌时不关闭 | 页面缺失；当前支持关闭按钮/Escape，遮罩无点击关闭处理 | O15 / N5 |
| H13 | 初始焦点进入第一个可见控件，过滤隐藏控件，并在焦点跑出弹窗后强制回收 | 部分保留；当前先聚焦面板，有基础首尾循环及关闭恢复焦点，没有旧完整可见控件/越界处理 | O15 / N5 |
| H14 | 状态提示不拦截底下点击、不能误选提示文字 | 缺失于旧 CSS 保护；当前 .toast 未设置 pointer-events:none/user-select:none | O15 / N5 |
| H15 | 交易完成提示区分立即成交/挂单，并直接给实际成交数量与成交价 | 部分保留；当前成功后统一提示“委托已提交，请在持仓与委托中查看成交状态” | O1、O4 / N1 |
| H16 | 玩家能阅读管理员配置的当前预设自定义说明 | 页面缺失；当前规则页显示预设名称和固定说明，未展示 activeMarket.description；字段及 API 仍在 | O6 `rules.description` / N5 |
| H17 | 玩家能查看当前预设的具体休市日期清单或明确“未配置”状态 | 页面缺失；当前仅说明遵守配置休市日，没有列出 holidays；休市日仍由后端执行 | O6 `rules.holidays` / N5 |

## I. 历史同步、缓存与接口细节

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| I01 | 上传签名的整月 count/digest 确认，由服务端核对并持久化最近校对时间；不匹配事务失败 | 行为变化；当前实例获取服务端 manifest，由 AzurPilot 本地比对；无原确认请求、checkedAt 记录和同等服务端确认事务 | O12、O13 / N6、N7 |
| I02 | 月历史摘要包含首条、末条观测时间 | 缺失于接口；当前 Month 只有 month/count/digest | O12 / N2、N6 |
| I03 | 约每五分钟完成历史摘要轮转核对，变化后尽快校对 | 行为变化；当前源重扫仍约五分钟，但服务端摘要核对等待通常为 1800 秒；不能把两个周期混为一谈 | O13 / N7 `ActionHistory.synchronize` |
| I04 | 新记录发现约 250ms 检查节奏，未变化不请求远端 | 行为变化；当前后台 wait(2)，发现延迟变为约两秒；未变化少做工作原则仍在 | O13 / N7 `_run` |
| I05 | 实时报价独立通道优先上传最新新鲜点，历史补传独立重试；月份轮转避免旧月被新数据阻塞 | 按新增量协议合并；旧独立实时/按月轮转机制不在，当前一次发送全部未确认点，旧故障隔离与时延语义不能原样认定保留 | O13 `_sync_instance/_sync_history` / N7 |
| I06 | 只重建变化分钟的持久分钟 OHLC，再从分钟表聚合图表 | 缺失于优化实现；当前 Chart 对所请求区间原始历史分组。历史独立存储和图表缓存仍有 | O5、O12 `writeMarketData` / N6 `PostgresStore.Chart` |
| I07 | 行情详情缓存按最近使用淘汰，最多 24 份/8 MiB | 行为变化；当前 Go 缓存为每股票一份，没有旧全局数量/字节淘汰。此处是缓存策略差异，不是要求恢复进程 CPU/内存配额 | O5 / N6 `cachedChart` |
| I08 | 同一股票的不同周期/月/日可分别复用详情缓存 | 部分保留；当前每股票只保留一个图表查询键，切换范围会替换 | O5 / N6 |
| I09 | If-None-Match 支持弱 ETag、逗号列表与通配符 | 部分保留；当前 market/rules/chart/account 主要直接比较字符串，普通强 ETag 的 304 仍支持 | O10 `matchesETag` / N6 |
| I10 | 受信开发来源支持 host:* 的端口模式 | 行为变化；当前 allowlist 精确字符串匹配，通配端口不再解释；仍有来源限制，不是开放 CORS | O10 `allowed` / N6 `ServeHTTP` |
| I11 | 页面重新可见时主动刷新市场/账户与图表 | 页面缺失于显式补刷新；当前独立站监听 online 和 SSE，未监听 visibilitychange | O1、O3 / N1 |
| I12 | 每秒 SSE 同时给 serverTime，客户端用于费用、报价时效和赛期更新 | 按版本通知方案替换；当前只发送 version 和 25 秒注释保活，不保留旧时钟事件。版本推送本身仍在，不能误列“没有实时通知” | O10 旧 realtime / N6 `events`、N1 |
| I13 | 认证资源繁忙的 503 附带 Retry-After: 1 | 部分保留；当前仍做 CPU 相关密码资源准入并返回 503，但未设置旧重试头 | O10 `passwordSlot` / N6 `passwordAdmission` |

## J. AzurPilot 本地数据诊断与重建入口

| 编号 | 以前有的功能或特性 | 当前缺口与保留情况 | 依据 |
| --- | --- | --- | --- |
| J01 | 区分网络失败与本地身份/历史/来源不一致，显示“玩家数据需要处理”及专门建议 | 页面缺失；当前注册页主要显示一般 ErrorBox | O13 旧页面、旧 api.tsx `isPlayerDataError` / N7 |
| J02 | 先请求重建计划，展示当前实例/全部实例范围、受影响实例、数据后果，再勾选确认或取消 | 页面缺失；当前 `stock.rebuild` 路由与后端计划仍存在 | O13 / N7 `router.py`、`StockExchangeRecovery` |
| J03 | 从页面执行“完全重建”，后端先备份本地旧数据、再清理范围并重新进入页面；不删除远端账户和脚本资料 | 页面缺失；后端 recovery/rebuild 保留并改用 stocks-v2.json，当前 StockExchange.tsx 没有调用入口 | O13 / N7 |

## K. 明确被新架构替换的旧能力与历史兼容

这些也是“以前存在、现在不再提供”的项目，但**列举不等于建议恢复**。当前要求禁止恢复旧 v1 API、SQLite 生产存储、验证码和 AzurPilot 交易代理等方案。

| 编号 | 以前有的功能或特性 | 当前情况与边界 | 依据 |
| --- | --- | --- | --- |
| K01 | 无 QQ 机器人身份即可在 AzurPilot 开交易账户，同时自动发行本人股票 | 按要求替换；当前交易账户必须官方 QQ 注册密钥，实例股票独立注册，再永久绑定 | O10、O14 / N5、N6、N7、N8 |
| K02 | 玩家只能从永久绑定实例签名登录，交易会话留在 AzurPilot 后端内存 | 按要求替换；当前独立站账号密码＋HttpOnly Cookie，官方身份与股票分别绑定 | O10、O13 / N5、N6、N7 |
| K03 | AzurPilot 原生交易终端及 stock.request 白名单代理，SSE 转本机 WebSocket，不需浏览器直连交易所 | 按要求移除；当前只提供注册、状态、同步与独立站跳转；不能要求恢复交易代理来补 UI | O1、O13 / N7 |
| K04 | 玩家轮换仅上传 token，立即撤销旧上传 token | 被实例私钥直接签名请求替换；当前没有 upload-token，也不再需要独立上传 Bearer。实例私钥保护仍在 | O10 `RotateUpload`、O13 / N6、N7 |
| K05 | 旧 /api 路径与旧实例/历史签名文本、旧请求响应字段 | 明确不兼容；当前为 /api/v2 与 mmex-v2 签名。不能把路径消失重复算作其所有业务能力都消失 | O10、O12、O13 / N6、API-v2 |
| K06 | SQLite 生产账本、原数据文件直接打开及旧字段自动补默认值/身份码/报价和成交迁移 | 按要求替换为独立 PostgreSQL；没有旧 SQLite 到 v2 的账户、资金、委托、赛季、自选、身份码自动迁移，不能宣称旧数据已连续继承 | O7、O10、O11、O12 / N2、N6、README |
| K07 | 旧交易会话、上传凭据和既有实例绑定在旧版升级中继续工作 | v2 不继承；旧会话不能登录新站，旧实例需按新协议登记股票，旧身份信息不能凭同名重建继承 | O10、O13 / N6、N7、README |
| K08 | Google reCAPTCHA v2 注册/玩家登录/管理登录及挑战生命周期管理 | 按要求完全移除；不得为恢复“旧功能”重新引入 | O14、O17 / N5、N6、AGENTS |
| K09 | 更早版本的 Cloudflare Turnstile，校验 hostname/action，及其测试模式 | 历史确认存在于 391e9de，46de22b 已换成 reCAPTCHA；当前没有，属于已淘汰认证方案 | O17 / N5、N6 |
| K10 | 无限模拟普通库存/借券、满足条件全量成交、没有部分成交或额度等待 | 按要求替换为独立有限库存与小时额度；当前部分成交及整点优先级是新能力，不能复原无限规则 | O4、O11 / N3、API-v2 |
| K11 | 所有股票在符合旧基础校验时均可被卖空，成交价直接等于行动力报价、不受利用率溢价影响 | 按要求替换；当前卖空须股票已绑定，茗喵模式有溢价和新增阶梯税，现实参考预设仍保留各自原参数 | O10、O11 / N3、N6、API-v2 |
| K12 | 公开读取独立原始报价历史的旧 GET /api/history/{stock} | 被有界 OHLC 图表接口替换；当前原始历史仍完整保存。恢复能力应优先摘要/OHLC，不能误以为必须向客户端重新传全量历史 | O10 `History` / N6、AGENTS |

## L. 开发、发布和运维工具

此组记录以前仓库自带的能力。当前外部 nginx、证书、数据库和 AstrBot 不属于本项目可以自动接管的范围；“没有自带脚本”不代表线上实际配置也不存在。

| 编号 | 以前有的功能或特性 | 当前情况与边界 | 依据 |
| --- | --- | --- | --- |
| L01 | npm run dev:mock 一键启动带种子玩家、报价、演示赛期的真实 Go 本地演示 | 移除；当前 dev-test 要求回环 PostgreSQL，不提供原隔离 SQLite 演示及自动种子场景 | O16 `dev-mock/SeedMock` / N9 |
| L02 | 开发启动器优先使用已有 bin/exchange，支持 GO_BIN 回退至源码启动 | 移除；当前隔离 E2E 固定从 Go 源码启动，这是现行验证要求 | O16 / N9 |
| L03 | Windows PowerShell 与 Bash 两套 release 脚本，构建前端、执行检查、交叉编译 Linux amd64/arm64、打包部署归档 | 移除；当前有 Dockerfile/Compose，但无原预构建发布包工具 | O16 / N9 |
| L04 | Debian 一键安装，支持部署包或源码并按需安装构建工具 | 按要求移除；当前使用已准备好的外部环境与纯后端 Compose | O16 / N9、AGENTS |
| L05 | systemd 服务安装、开机启动及服务权限隔离 | 部署方式替换；Compose 仍有 restart:unless-stopped、非 root 用户，不能说自动重启或非 root 运行消失 | O16 / N9 |
| L06 | 脚本在切换前检查外部证书缺失/过期/域名/密钥匹配 | 不再内置；README 要求沿用外部证书，不能推断服务器没有相应检查 | O16 / N9、README |
| L07 | 自动生成 nginx HTTPS/HTTP 跳转、站点代理、SSE 关闭缓冲、连接复用和 Cloudflare 受信真实 IP 配置 | 不再由仓库安装脚本执行；当前 SSE 响应头和部署说明仍有，外部 nginx 需自行维护 | O16 / N6、N9 |
| L08 | --nginx-only 诊断修复，检查现有 master、持久文件描述符上限、平滑重载、验证失败恢复旧配置 | 随旧部署脚本移除；不得恢复为擅自管理现有 nginx 的路径 | O16 / N9、AGENTS |
| L09 | 升级停服时自动备份 SQLite 主文件与 WAL | 存储与流程替换；当前要求发布前备份专用 PostgreSQL，Compose 不自动运行 pg_dump，不能称有旧自动备份功能 | O16 / N9、README |
| L10 | 独立 relay_smoke.py 跨服务冒烟与 check-deploy.py 的真实 nginx/TLS/1200 双栈连接回归 | 原工具/专用 nginx 回归移除；前者已有 AzurPilot 新 HTTP＋PostgreSQL 集成测试，不能把“文件被删”写成所有跨服务测试都没有 | O16 / N9、N7 `test_stock_exchange_integration.py` |

## 已保留、不能误列为完全缺失的能力

以下正面核对用于排除误报；“源码/现有测试包含”不表示本次重新执行过这些测试。

| 旧能力 | 当前证据与实际保留范围 |
| --- | --- |
| 月赛开赛、截止、强制平仓、撤单、归档写入、次月重置、长期离线后推进 | `market_runtime.go: advanceSeason`，`market_runtime_test.go: TestSeasonCloseArchiveRestartAndNextMonthReset`；查询/展示缺口另见 A |
| 上海赛季时区、月末/闰月窗口 | `market_rules.go: SeasonFor`，`market_rules_test.go: TestMarketSeasonShanghaiBoundaryAndMarginRounding` |
| 低行动力本月退市、只结算目标股票、下月恢复、阈值 0 关闭 | `delistStock/advanceSeason`，当前 runtime/regression 测试；边界变化见 F13/F14 |
| 四方向、限价不越界、撤销释放、委托幂等、普通反向持仓准入 | `orders.go` 与 `regression_test.go`；市价/止损/TIF/反向挂单差异见 D/F |
| 全部本人订单持久回执 | `Portfolio` 从 accountOrders 返回全部订单，未做旧“最近若干条”截断；原 GET /orders 被合并到 /account，不算回执全部丢失 |
| 融资买入、按比例还本金、杠杆上限检查、融资/借券利息及分以下余数 | `orders.go/market_runtime.go`；费用归类、计提节奏和欠费语义见 E/F |
| 做空所得/保证金不能重复用于开仓、保证金强平 | 当前通过独立 escrow 保留基础约束；风险计算范围和资金表示改变，见 F |
| T+N 股票可卖、现金可用、周末/休市日/市场时区与 DST | `sellable/availableCash/AvailableAt` 与 `market_rules_test.go`；页面可见性见 E/G |
| 茗喵、A 股、港股、美股预设及原佣金/印花税/征费/整手/时段/保证金/融资借息参数 | `config/presets/`，`TestMarketPresetsRetainLegacyParametersAndJSON`；预设管理 UI 缺口见 G21–G25 |
| 初始模拟币可调，保存不改已有当前本金/余额，次月按配置重置；暂停交易 | `Settings/SetSettings/advanceSeason/marketOpen` |
| 自我交易、零价/过期报价、资金/可卖/借券准入、整数输入校验 | `validateOrder/fill`；累计安全与成交时反向校验差异见 F |
| 完整原始行动力历史、同毫秒修正、旧历史不重放旧交易、空档不造点 | `mmex_v2_history/InstanceReport/Store.Chart`；原始记录没有因 2000 桶上限而截断 |
| 实例 UUID/Ed25519 永久身份、私钥留后端、签名仅证明来源 | `instance.go`、AzurPilot `stock_exchange_identity.py`；认证架构变化见 K |
| 退出/关页不解除永久绑定、不停止上报；实例改名/删除/同名重建隔离 | AzurPilot 当前 service/history/recovery 及其测试；旧交易会话不迁移，见 K |
| 本地历史 HMAC/持久检查点、密钥加密保护、旧 CL1 当月导入和失败重试、损坏不阻断脚本核心业务 | AzurPilot 当前 `ActionHistory/GameDataProtector/StockExchangeRecovery`；UI 重建入口缺口见 J |
| 受影响实体事务、提交失败不改内存/不通知、持久化恢复、单进程账本 | `Engine.transaction/PostgresStore.Commit`、postgres/regression 测试；数据库实现已经更换 |
| ETag/304、压缩、SSE 版本通知、断线重连、市场刷新合并通知 | 当前 `http.go/api.ts/main.tsx`；弱 ETag、可见性补刷等差异见 I |
| 认证资源准入、超时、请求头保护、严格 JSON 字段、受信来源和角色隔离 | `cmd/exchange/main.go/http.go`；没有重新引入固定请求次数/挂单数量/CPU 配额 |
| 账号停用或密码变更使旧会话失效 | `PasswordVersion/CheckSession/UpdateAccount/ChangePassword/ResetPassword`；停用清仓能力见 G17 |
| 响应式布局、基础弹窗角色、锁滚动、Tab 首尾循环、Escape 关闭及恢复焦点 | 当前 styles/ui；更完整的旧弹窗保护差异见 H |
| 返回 AzurPilot 总览与股票页失败隔离 | 当前 `StockExchange.tsx/StockExchangeBoundary.tsx`；原生交易终端返回入口随架构更换，不算宿主总览入口整体消失 |

机器人目前增加了帮助、注册密钥、二次确认重置、持仓、股票查询、绑定、四方向预览确认及按钮/文本降级。没有旧插件历史证据，不把这些新能力混入“旧版功能缺失”。

## 逐路由核对，防止只看界面漏掉接口能力

这里“旧路径”指 `a1081f5` 的实际路由，旧版前缀是 `/api`，并非字符串 `/api/v1`。

| 旧路径 | 当前等价位置或结论 |
| --- | --- |
| GET /healthz | 保留 |
| GET /api/meta | 元数据拆到规则/市场响应和页面常量；mock/notice/旧 origins 信息不再同等提供，涉及 H06、K、L |
| POST /api/register | 新 /api/v2/auth/register；开户及实例发行拆开，见 K01 |
| POST /api/login | 新 /api/v2/auth/login；认证语义改变，见 K02 |
| GET /api/market | 新 /api/v2/public/market；排名、收入、开盘/状态等缺口见 A/B/C |
| GET /api/events | 新 /api/v2/public/events；时钟与通知协议见 I12 |
| GET /api/history/{stock} | 无原始历史公开等价路由，见 K12 |
| GET /api/stocks/{stock} | 简化为新 chart；详情、成交及挂单缺口见 C |
| POST /api/quote-history | 新 instance/report；协议与确认语义见 I01、K05 |
| GET /api/quote-history/manifest | 新 instance/manifest；字段与校对节奏见 I01–I03 |
| GET /api/seasons | 无读取等价路由，见 A05 |
| POST /api/quotes | 合并到 instance/report，见 I05、K04/K05 |
| GET /api/account | 新 /api/v2/account；损益/保证金/费用信息缺口见 E/F |
| POST /api/watchlist | 无等价路由，见 B01/B02 |
| POST /api/upload-token | 无等价路由，实例签名替代，见 K04 |
| POST /api/orders | 拆为 preview/confirm；类型/有效期差异见 D |
| GET /api/orders | 已并入 /account.orders，完整本人回执保留 |
| DELETE /api/orders/{order} | 新 /api/v2/orders/{id}，撤销能力保留 |
| POST /api/console/login | 新 /api/v2/admin/login；Cookie 与认证变化见 K |
| GET /api/console/settings | 新 /api/v2/admin/settings |
| PUT /api/console/settings | 新 /api/v2/admin/settings；预设 UI 与赛期生效差异见 A13、G |
| GET /api/console/players | 新 admin/accounts＋admin/stocks；查询分页能力见 G |
| GET /api/console/players/{player} | 无管理员详情等价路由，见 G06–G11 |
| PUT /api/console/players/{player} | 新 admin/accounts/{id}；编辑与保护能力缩减见 G12–G19 |

## 源码证据索引

历史文件已被删除时，不创建旧代码副本、不回退工作区；用下列固定版本读取，例如：

```powershell
git -C E:\github\AzurPilot_StockExchange show a1081f5:internal/exchange/types.go
git -C E:\github\AzurPilot show dcf048910:frontend/src/stock/App.tsx
```

| 索引 | 固定版本及路径/关键符号 |
| --- | --- |
| O1 | AzurPilot `dcf048910`：`frontend/src/stock/App.tsx`，navigation、account-strip、season-card、leaderboard、RankCard、market-ticker |
| O2 | 同版本：`frontend/src/stock/Securities.tsx`、`market.ts`，defaultFilters、filterStocks、sortStocks、positionPnl |
| O3 | 同版本：`frontend/src/stock/Charts.tsx`、`indicators.ts`、`useStockDetail.ts`，FinancialChart、periods、indicatorLines、macd、trend |
| O4 | 同版本：`frontend/src/stock/Trading.tsx`，TradeTicket、OrdersTable、Portfolio |
| O5 | 同版本 `frontend/src/stock/StockDetail.tsx`；交易所 `a1081f5:internal/exchange/market_data.go`，StockDetail、Candle、Trade、PublicPending、StockSummary、periodBucket、candles、StockDetailJSON |
| O6 | AzurPilot `dcf048910:frontend/src/stock/Learn.tsx`，glossary、steps、Learn |
| O7 | 同版本 `frontend/src/stock/IdentityCard.tsx`；交易所 `a1081f5:internal/exchange/identity_codes.go`，generateIdentityCode、migrateIdentityCodes |
| O8 | 交易所 `a1081f5:frontend/src/Console.tsx`，Console、preset-tools、console-stats |
| O9 | 同版本：`frontend/src/Users.tsx`、`internal/exchange/users.go`、`users_http.go`，UserManagement、UserDetail、AdminPlayers、UpdatePlayer、disablePlayer |
| O10 | 同版本：`internal/exchange/types.go`、`engine.go`、`cycle.go`、`rules.go`、`store.go`、`realtime.go`、`http.go`；覆盖账户/赛季/收入模型、归档、行情、自选与 HTTP |
| O11 | 同版本：`internal/exchange/orders.go`、`funding.go`、`leverage.go`，Submit、validatePosition、triggered、fill、risk、forcedClose、capitalMargin、returnPPM |
| O12 | 同版本：`internal/exchange/market_history.go`，IngestHistory、historyManifest、writeMarketData、migrateMarketData |
| O13 | AzurPilot `dcf048910`：`module/api/stock_exchange_service.py`、`stock_exchange_history.py`、`stock_exchange_identity.py`、`stock_exchange_recovery.py`、`frontend/src/pages/StockExchange.tsx`、`frontend/src/stock/api.tsx` |
| O14 | 同版本 `frontend/src/stock/Auth.tsx`；交易所 `a1081f5:internal/exchange/http.go` 的 register/noticeVersion，两个仓库旧 Captcha/recaptcha 文件 |
| O15 | AzurPilot `dcf048910`：`frontend/src/stock/Modal.tsx`、`modal.css`、`theme.tsx`、`theme.css`、`OverviewLink.tsx` |
| O16 | 交易所 `a1081f5`：`deploy.sh`、`scripts/release.ps1`、`release.sh`、`check-deploy.py`、`relay_smoke.py`、`frontend/scripts/dev-mock.mjs`、`internal/exchange/mock.go`、`.github/workflows/check.yml` |
| O17 | 交易所 `391e9de:internal/exchange/http.go`、`frontend/src/Captcha.tsx`、`docs/API.md`；`46de22b` 替换为 reCAPTCHA，`089a39f` 补 gstatic.cn CSP |
| N1 | 当前交易所 `frontend/src/main.tsx:15` 的 App；`frontend/src/Market.tsx:6` 的 StockPanel、`:15` 的 Trade |
| N2 | 当前 `internal/exchange/types.go`、`dto.go`、`index.go`、`engine.go`：publicStock（340）、Market（343）、SetSettings（370）、equity（433）、accountPublic（459）、Portfolio（462）、AdminAccounts（582）、UpdateAccount（603） |
| N3 | 当前 `internal/exchange/orders.go`：validateOrder（93）、Preview（154）、fill（305）、accrue（539）、liquidateStock（577）、auction（612）；`market_runtime.go`：availableCash（60）、accrueFinancing（134）、closePosition（154）、riskLong（186）、advanceSeason（198）、delistStock（279） |
| N4 | 当前 `frontend/src/Admin.tsx`：MarketEditor（33）、Admin（39）；`internal/exchange/http.go` 管理路由（87 起） |
| N5 | 当前 `frontend/src/Rules.tsx:10`、`Auth.tsx`、`ui.tsx:3`、`styles.css`；包括仍有的规则说明、当前赛季窗口、基础 Modal |
| N6 | 当前 `internal/exchange/http.go` 路由（68–116）、条件响应和 SSE；`instance.go` 的签名、InstanceRegister/InstanceReport/Chart；`store.go:146` 的 Chart、Manifest；`cache.go:22` 的 cachedChart；`auth.go` |
| N7 | AzurPilot 当前 `module/api/stock_exchange_*.py`、`module/api/router.py:90`、`frontend/src/pages/StockExchange.tsx`、`frontend/src/components/StockExchangeBoundary.tsx`，以及 tests/test_stock_exchange* |
| N8 | 当前 QQ 插件 `README.md`、`main.py`、`stock_core.py` 及 tests；插件无旧提交历史，仅用于核实目前替代能力 |
| N9 | 当前交易所 `Dockerfile`、`docker-compose.yml`、`frontend/scripts/dev-test.mjs`、`.github/workflows/check.yml`、README；当前 AzurPilot 跨服务集成测试 |

## 附录：被删词典的全部 37 个词条

以下列的是独立词典入口缺失，并不表示当前规则说明完全没有提及这些概念；旧解释含无限流动性、旧开户方式等内容，不能直接复制回 v2。

1. 分时图 / 分时均价
2. K 线 / OHLC / M5
3. MA / 移动平均线
4. EXPMA / 指数平均线
5. BOLL / 布林带
6. ENE / 轨道线
7. BBI / 多空趋势
8. MACD / DIF / DEA
9. 逐笔成交 / 成交量
10. 股票 / 证券
11. 总资产 / 净值
12. 多头 / 买入
13. 杠杆 / 融资买入
14. 融资利息 / 还款
15. 空头 / 卖空
16. 回补 / 买入平仓
17. 市价委托
18. 限价委托
19. 止损触发市价
20. DAY / GTC
21. 一手 / 整手
22. T+0 / T+1 / T+2
23. 佣金
24. 印花税
25. 交易征费
26. 借券费
27. IMR / 初始保证金率
28. MMR / 维持保证金率
29. 强制平仓 / 爆仓
30. 可用资金 / 冻结资金
31. 浮动盈亏 / 已实现盈亏
32. 涨跌幅 / 开盘价
33. 强制退市
34. 报价过期 / 停牌
35. 价格跳空 / 滑点
36. 流动性 / 盘口
37. 赛季收益率

旧新手六步为：开通账户、后台自动更新、读懂报价、先练习买入、查看持仓与退出、理解杠杆与卖空。其开户、限价/额度和风险说明需按 v2 改写。

## 完整性检查与待确认边界

| 核对面 | 处置 |
| --- | --- |
| 全部旧 Go 功能模块 | engine/types、cycle、rules、orders、leverage/funding、binding/identity/identity_codes、users、market_data/history、realtime、store、http、mock、main 均已分配到 A–L 或“已保留”表 |
| 全部旧管理前端 | Console、Users、api/types、Captcha/recaptcha、样式与入口已核对，包含配置、收入和用户详情 |
| AzurPilot 26 个旧终端文件 | App、Securities/market、Trading、Charts/indicators、StockDetail/useStockDetail、Learn、IdentityCard、Auth、Modal、主题、OverviewLink、api/types 及对应样式/单元测试均已覆盖 |
| AzurPilot 注册页/实例代理/身份/历史/恢复 | 独立对照旧页面及四个 stock_exchange 模块，没有仅依据删除目录推断 |
| 旧全部 24 条显式业务/健康路由 | 已逐条映射；通配静态入口额外核对为当前公开独立站，属于新增/替换 |
| 旧测试和历史修复 | 已用全部旧 Go Test 名称及股票 E2E、指标/过滤测试反查规则，包含开盘价、DAY 跳空、晚加入排名、混合融资保证金、现金保护、恢复和弹窗；当前已有对应能力从缺失表排除 |
| 早于最后旧版的历史 | 已额外核对 Turnstile 等已在旧版期间淘汰的能力；未把旧 README 的 28 词条或过时的历史/成交截断描述当作实际实现 |
| 旧文档提及 iframe/postMessage、stock.bind/stock.unbind | 在本次可达股票实现中没有找到其实际代码版本；仅列为更早历史待确认，不能冒充确认存在过的功能。本次也不恢复这些明确禁止的路径 |
| QQ 历史及真实适配器 | 无旧插件提交可查；未做真实 QQ 会话或权限验收，不以单元夹具断言真机结果 |
| 线上和不可达历史 | 未核实生产产物，也没有删除分支/其他旧仓库的完整档案；如另有旧版本，需作为额外基线继续对照，不能承诺覆盖不存在于证据中的实现 |

本次验证为源码、历史、路由、字段、路径、文档编号及 diff 核对。只修改文档，未执行 Go、前端、PostgreSQL、Playwright、Docker 或真实 QQ 测试；本报告引用现有测试用于说明覆盖意图，不将其写成本次测试通过。未留下需要本次实现或部署的事项；恢复/改动其中任何业务能力均是后续单独任务。

文档静态校验另外核实了：209 个编号连续且不重复、各组统计一致、每项同时有已定义的 O/N 源码依据、24 条旧显式路由无遗漏映射、37 个词条与旧源码逐个同序匹配、27 个交易所历史证据路径可读取、旧 AzurPilot 终端 26 个文件在基线中存在。审计期间记录的本仓库原有 57 个文件 SHA256 均未变化，仅新增本文件；AzurPilot 工作区保持干净，插件只读。

## 本次更新范围、生效和兼容关系

| 分类 | 组件 | 原因与生效步骤 |
| --- | --- | --- |
| 必须更新 | 本审计文档 `docs/LEGACY_FEATURE_AUDIT.md` | 已新增，直接阅读生效；无需构建 |
| 可选更新 | 文档导航或后续需求清单 | 可以把本文件链接到项目文档或按编号转为任务；本次不改已有工作中的 README，不自动恢复功能 |
| 不需要更新 | 交易所 Go 后端 `cmd/exchange/`、`internal/exchange/` | 本次无运行代码改动，无需重建镜像或 docker compose up |
| 不需要更新 | 交易站/管理站 `frontend/` | 本次无前端代码改动，无需构建/部署 dist 或重启 Go |
| 不需要更新 | AzurPilot 股票前端 | 只读取，未修改，无需更新其前端产物 |
| 不需要更新 | AzurPilot 实例同步后端 | 只读取，未修改，无需发布或重启 |
| 不需要更新 | 官方 QQ AstrBot 插件 | 只读取，未修改，无需重载插件；不操作全局配置或 Docker |
| 不需要更新 | 数据库、nginx、挂载、证书、凭据 | 本次无写入，无迁移、备份恢复或发布顺序要求 |
| 待确认 | 如需继续核对另一个旧仓库/不可达版本/线上实际版本 | 须先取得对应版本证据；当前清单结论严格对应上述已核对基线 |

相对于本次审计前的仓库工作区，运行代码与接口未变，现有兼容关系不受影响：

- “本次后的后端＋本次前的交易站/管理站/AzurPilot 同步端”：实现相同，兼容关系不变。
- “本次后的前端＋本次前的后端”：实现相同，兼容关系不变。
- 如果“新/旧”特指当前 v2 与历史 v1，则两个方向均不兼容：路由、会话、实例签名及账户模型不同；旧 AzurPilot `stock.request` 也不能对接 v2。依据为上述路由表、`http.go/instance.go` 与旧 `stock_exchange_service.py` 的契约对照，非线上版本推测。
- 数据与身份不能以“前端兼容”概括：旧 SQLite、旧账户/本金/委托/排名/自选、旧交易会话和旧上传凭据不会自动迁入 v2；实例需按新协议登记，不能同名继承旧身份。后续真要迁移或回滚，必须另行制定字段映射与备份恢复、成套前后端/同步端发布顺序。本次文档没有改变这些既有边界。

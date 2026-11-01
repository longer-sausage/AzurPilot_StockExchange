# 茗交所 v2 API

路径前缀 `/api/v2`；金额、价格为整数分，数量为整数股，费率为 ppm。行情、成交、赛季、图表与交收时间均为 Unix 毫秒；签名 `issuedAt` 和 Cookie 会话时间为秒。错误为 `{error:{code,message}}`。域名 `https://stock.azurpilot.com`。v1 不兼容。

## 注册、会话与准入

交易账户只能由官方 QQ 机器人创建。网页注册与注册密钥已删除，`POST /auth/register` 返回 404。流程为群内 `注册 <用户名>` → 随机初始密码 → 网页首次登录强制改密 → 绑定 AzurPilot 实例股票。改密与绑定均完成后账户才可使用。

- `POST /auth/login` `{username,password,remember}` → `{account,forcePasswordChange}`。独立 HttpOnly Cookie `mmex_player`，未记住为浏览器会话，记住为 30 天。
- `GET /auth/me` → `{account,forcePasswordChange}`，允许受限账户检查自身绑定状态。
- `POST /auth/password` `{currentPassword,password}` → `{ok:true}`；12–72 字节、包含大小写/数字/符号，且不同于原密码。更新密码版本、撤销其他旧会话，并更新当前 Cookie。
- `POST /auth/logout` → `{ok:true}`。
- `GET /account` → `{account,positions,orders,trades}`，私人 ETag/304。
- `PUT /account/watchlist/{stockId}` `{enabled:boolean}` → `{ok:true}`。幂等、按账户持久保存，不公开。

`/account`、自选与交易端点统一检查：未改密为 `PASSWORD_CHANGE_REQUIRED`，未绑定/绑定被删除为 `BINDING_REQUIRED`。受限账户仍可登录、改密、重置、绑定及退出。机器人使用同样的账户准入；查询公开市场与排行榜无需账户。

account 包括 `id,username,identityCode,createdAt,initialCash,cash,reserved,availableCash,equity,returnPPM,realized,fees,watchlist,boundStockId,forcePasswordChange,disabled,settlements`。身份识别码与自选只供本人/管理员读取；公开排名只含账号 ID 与展示名。用户名新建/改名采用 NFKC + Unicode case folding 检查唯一性；旧库若已有等价名，原名仍可直接登录，歧义别名不会猜测对应账户。

positions 包括 `stockId,stock,quantity,sellable,cost,loan,lots,shortQuantity,shortProceeds,shortCost,collateral`。`shortCost` 为扣开仓交易费后的空头成本，部分回补按股数比例释放，最后一股释放全部余数。

`account.metrics` 包括多/空/总浮盈亏、融资借款、空头负债、独立冻结的卖空所得及保证金、当前市值下初始/维持保证金要求、待交收金额、未落账融资/借券费估算、挂单冻结。未落账估算单列，实际结算后影响资金；保证金要求展示不额外改变 v2 的冻结规则。

## 公开行情、赛季与成交

- `GET /public/market` → `{version,stocks,serverTime,open,reason,season}`，ETag/304。
- `GET /public/rules` → 下文设置对象，独立 ETag/304。
- `GET /public/events`：SSE `event: change`、`data: {version}`。重连首先发当前版本，25 秒注释保活；客户端合并请求期间通知，并在重连、重新可见及网络恢复时刷新。
- `GET /public/seasons` → `{season,rankings,archives,revenue,lifetimeRevenue,serverTime}`。当前按净资产降序、同资产账户 ID 升序；排除停用、未绑定及截止后加入的账户。已结算月份返回持久最终排名。`archives` 项为 `{season,rankings,revenue}`，排名为 `{rank,accountId,username,equity,returnPPM}`，收益率按个人实际起始本金计算。
- `GET /public/stocks/{id}/chart?from=毫秒&to=毫秒&interval=分钟` → `{stockId,interval,points:[{t,o,h,l,c,n}]}`；指定周期保持不变，不再自动改粗为约 2000 桶。日 K 按上海午夜分桶，原始历史不截断。ETag 支持弱标签、列表与 `*`。
- `GET /public/stocks/{id}/detail?from=毫秒&to=毫秒` → `{stock,trades,orders,legacyOrders,months}`，逐笔成交覆盖请求范围，orders 为当前全部公开待成交委托。
- `GET /public/stocks/{id}/chart-detail?period=time|day|m5|m10|m20|m30|m60&month=YYYY-MM&day=YYYY-MM-DD` → `{stock,period,month,day,displayFrom,bars,trades,orders,legacyOrders,summary,coverage}`。day 仅分时使用，省略时选该月最近有报价日。日 K 预取三个月、分钟 K 预取两天用于指标预热，`displayFrom` 标记显示范围。

股票摘要包含 `id,symbol,name,price,previous,open,openAt,openDay,observedAt,uploadedAt,historyVersion,tradeVersion,historyCheckedAt,bound,disabled,delisted,issuedShares,outstanding,hourlyVolume,normalUsed,borrowShares,borrowed,borrowHourlyVolume,borrowUsed`。删除股票不再出现在公开行情或查询中。涨跌幅以上海当日第一条已保存报价为基准；当日无开盘或开盘价为零显示未知，前次报价不作昨收。

bars 项为 `{time,open,high,low,close,samples,volume,turnover,buyVolume,sellVolume}`。按观测与实际成交时间确定开/收，成交无新观测时 `samples=0`；不补造空档。summary 含所选范围开高低、上一有行情日收盘、真实成交量额及方向统计。coverage 含所选月原始记录数、首末观测与最后成功摘要校对时间。图表缓存复用不同股票/周期/月/日，按最近使用淘汰。

trades 项为 `{id,orderId,stockId,accountId,username,side,kind,quantity,price,notional,at,forced,reason,fees}`。每次实际部分成交独立持久保存，与资金、库存、费用及补偿同事务提交。旧订单不能还原逐次成交，保留在 `legacyOrders`，其中 `updatedAt` 明确为旧订单更新时间，不伪称逐笔成交时间。

## 委托

- `POST /orders/preview` `{stock,side,quantity,price?,clientId,leverage?,kind?,timeInForce?}` → 预览。
- `POST /orders/confirm` `{previewId}` → `{order}`。
- `DELETE /orders/{id}` → `{order}`，撤销未成交余量、释放冻结。

side 为 `buy/sell/short/cover`，kind 缺省 `limit`，可选 `market`、`stop`；timeInForce 缺省 `GTC`，可选 `DAY`。price 是限价/止损触发价，市价单忽略其输入并用当前报价计算预览与资金预留。DAY 在市场时区次日零点到期，状态 `expired` 并解冻；GTC 至成交、撤销或本赛季截止。杠杆缺省 1，最大为当前 IMR 允许的整数、最多 10；卖出/回补固定 1。

市价、止损也遵守有限库存与独立小时量。止损买/回补在基础报价不低于触发价时触发，卖/卖空反之；触发后为市价，存在跳空风险。额度内立即成交，余量按后续整点处理；恢复这两类委托不恢复旧版无限流动性或逐次报价即时触发全部挂单。

预览含 `previewId,expiresAt,stock,side,kind,timeInForce,quantity,price,notional,taxRatePPM,tax,commission,stamp,levy,estimatedTotal,premiumPPM,leverage,financing,collateral,repayment,rules`，5 分钟有效。卖出预计净到账扣除按比例归还融资本金。买入为首付款及费税、卖空为自有保证金及费税，回补为成交总支出（解冻抵扣另按持仓计算）。

order 包括 `id,stockId,side,kind,timeInForce,expiresAt,triggered,leverage,quantity,remaining,filled,price,averagePrice,notional,tax,commission,stamp,levy,status,reserved,createdAt,updatedAt,reason`。`updatedAt` 不能代替全部逐笔成交时间。相同 clientId 修改标的/方向/类型/有效期/数量/限价/杠杆为 `IDEMPOTENCY_CONFLICT`；市价的参考报价不构成用户修改。重复确认不重复落账。禁止现有持仓或待开仓与新开仓方向冲突，成交时再次核验。

## 官方 QQ 机器人

使用 `Authorization: Bearer BOT_API_TOKEN`，officialUserId 为 `appid:openid`，群身份使用平台提供的 member_openid。全部指令可在群内执行并在原群回复，敏感信息同样公开；确认码仍约束发起人、应用与会话。

- `POST /bot/register` `{officialUserId,username,requestId}` → `{status:"registered",username,accountId,password,forcePasswordChange:true,bindingRequired:true}`。首次创建；同注册请求 15 分钟内重试可取回同一初始密码，回执加密持久化。改密/重置后不会返回过期初始密码；已注册的新请求返回 `{status:"already_registered",username,accountId,bindingRequired}`，不改名、不发密码、不重复开户。
- `POST /bot/reset-password` `{officialUserId,confirmed:true,requestId}` → `{password,forcePasswordChange:true}`，插件必须二次确认，requestId 幂等；撤销全部旧会话。
- `POST /bot/bind` `{officialUserId,bindingCode}` → `{stock}`。一对一、同绑定幂等；不能绑定已删除股票。只有管理员删股才解除原绑定，之后可重新绑定其他股票；绑定目标不得仍被本人持仓或挂单占用。
- `GET /bot/portfolio?officialUserId=...` → 与 `/account` 相同。
- `GET /bot/stock?officialUserId=...&query=编号或名称或official:appid:openid` → `{stock}`；省略 query 为本人股票。
- `POST /bot/orders/preview`：网页预览参数加 officialUserId。
- `POST /bot/orders/confirm` `{officialUserId,previewId}` → `{order}`。

## 实例（AzurPilot 服务端）

Ed25519 公钥/签名为标准 base64。请求 `{instanceId,publicKey,issuedAt,signature,username?,points?,months?}`；issuedAt 为秒，允许前后 300 秒；points 为 `[{t:毫秒,p:整数行动力}]`。

既有 action 的 canonical 不变：每行含末尾 LF，依次为 `mmex-v2`、action、instanceId、publicKey、issuedAt、username（仅 register，其他空）、points 紧凑 JSON（无为 `[]`，键序 t,p）。签名确认来源，不证明数据真实。

- `POST /instance/register`：action=register，username 必填 → 状态对象。
- `POST /instance/status`：action=status → 状态；未注册 404 `STOCK_NOT_REGISTERED`。
- `POST /instance/report`：action=report，points 必填，backfill 为兼容旧协议的额外布尔字段 → `{cursor,accepted}`。同毫秒修正 upsert；旧历史不重放交易与过去的退市。
- `POST /instance/manifest`：action=manifest → `{months:[{month,count,digest}]}`。上海月份，digest 为升序 `t:p\n` 的 SHA256；按月持久缓存，原始表语句级触发器只标脏受影响月份。
- `POST /instance/reconcile`：action=reconcile，months 为完整本地月摘要，**在既有 canonical 后额外附一行 months 紧凑 JSON，键序 month,count,digest**，无月份为 `[]`。服务端核对全部月份后同事务保存成功校对时间与历史版本；不匹配 `HISTORY_MISMATCH`，不更新校对状态。相同版本重复校对幂等。

状态为 `{stock,bindingCode,exchangeUrl,cursor,capabilities:["reconcile"]}`。新 AzurPilot 仅在能力已声明时发送 reconcile，可与旧 v2 后端继续增量同步；原四类请求签名不变。

被删除的原实例身份在注册、状态、上报、摘要、校对均返回 **410 `STOCK_DELETED`**，不会因重试复活。AzurPilot 每约 15 秒检查状态，收到删除后在受保护登记中保存 deleted、退出监视；重启、强制同步也不会再请求该身份。网络故障期间服务端仍拒绝所有上传，客户端在恢复连接后获知删除。

AzurPilot 本地 `stock.register` 增加 `noticeVersion:"stock-public-v2-20261101"`，要求先确认股票名称、行动力历史与交易数据公开及持续同步告知；未确认的新注册返回 `STOCK_NOTICE_REQUIRED`。这是 AzurPilot WebSocket 契约，与交易所实例签名无关。

## 管理与设置

- `POST /admin/login` `{password,remember}`，独立 `mmex_admin` Cookie；`POST /admin/logout`，`GET /admin/me`。
- `GET/PUT /admin/settings`：完整设置对象。已有本月赛期固定，起止配置从后续月份生效；空库首次初始化例外。
- `GET /admin/accounts` → `{accounts}`，含注册时间、持仓数、待成交数。`GET /admin/accounts/{id}` 返回完整账户、持仓、交收、委托及逐笔。
- `PUT /admin/accounts/{id}` `{username?,cash?,cashAdjustment?,expectedCash?,disabled?,confirmDisable?}`。现金绝对值与增量互斥，涉及现金必须提供匹配 expectedCash，变动则 `CASH_CHANGED`；不修改个人赛季本金。扣款检查冻结、交收、估算费用与 MMR。停用需要 confirmDisable=true，撤单、结清本人持仓并撤销会话；股票停牌独立管理。
- `POST /admin/accounts/{id}/reset-password` `{confirmed:true,requestId}` → `{username,password,forcePasswordChange:true}`；二次确认、幂等并撤销旧会话。
- `GET /admin/stocks`：含公钥、实例 UUID、绑定码与删除墓碑的管理列表。
- `PUT /admin/stocks/{id}` `{issuedShares,hourlyVolume,borrowShares,borrowHourlyVolume}`，不得缩到已占用库存以下。
- `POST /admin/stocks/{id}/status` `{disabled}`，停牌撤销该股票全部待成交委托。
- `DELETE /admin/stocks/{id}` `{confirmed:true}`：结清该股票多空持仓、计提旧区间费用、支付实际回补损失补偿、撤销相关挂单及原绑定账户其他挂单、解除绑定、移除自选并写删除墓碑。全部同事务；失败无内存变化和通知。历史保留，不可撤销，不重用原股票 ID/实例身份。

设置沿用 `initialCash,issuedShares,hourlyVolume,borrowShares,borrowHourlyVolume,premiumPPM,borrowAnnualPPM,commissionPPM,minCommission,quoteTTLSeconds,taxBrackets,paused,activeMarket,marketPresets,startDay,endDaysFromLast,delistThreshold`。默认值与现实预设见 `config/`，只空库初始化使用。管理页面可另存、更新、删除预设（至少一个），UI 费率用百分比、金额用模拟币，API 仍为 ppm/分。

MarketRules 为 `id,name,description,commissionPPM,minCommission,stampPPM,stampSide,stampRound,levyPPM,borrowAnnualPPM,financingAnnualPPM,imrPPM,mmrPPM,lotSize,sellDelayDays,cashDelayDays,timezone,sessions,weekdaysOnly,holidays,quoteTTLSeconds,source`。meow 的佣金、最低佣金、借息、报价有效期使用外层同名设置；其他模式使用 activeMarket。只有 meow 启用新增阶梯税与溢价。

## 账本与旧数据

所有模式保留有限发行/借券库存和独立小时量。限价不越界，整点按价格降序、同价先到先得。卖空仅限已绑定的其他股票；所得与保证金独立冻结，发行者仅收到实际回补价格亏损，不承担交易者债务。

价格/费率变化先结算旧区间，分以下余数结转。费用收入以实际收取金额记账；强平资金不足时不把未实际收取的费税计作收入。余额、冻结、库存、成交、回执、补偿、删除与解除绑定均原子提交，提交后才失效缓存和广播。

本次启动迁移为实体增补身份码、类型/有效期与旧成交汇总，清除遗留注册密钥。已有 v2 账户、密码、绑定、订单和原始报价保留；未绑定账户停止准入并取消挂单。旧成交只保留聚合证据，历史借息/融资计提不可反推，统计边界与回滚步骤见 [功能恢复交付说明](FEATURE_RESTORATION.md)。旧 SQLite/v1 账户不会自动迁入 v2。

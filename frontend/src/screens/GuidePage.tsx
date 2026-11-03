"use client";
import { useState } from "react";
import {
  Banner,
  Collapsible,
  Empty,
  InputGroup,
  LayerCard,
  LinkButton,
  Table,
  Tabs,
} from "@cloudflare/kumo";
import { MagnifyingGlassIcon } from "@phosphor-icons/react";
import { useExchange } from "../core/Exchange";
import { money, percent } from "../core/format";
const lessons = [
  [
    "注册交易账户",
    "先加入官方 QQ 群，向机器人发送“注册 <用户名>”。机器人会在原群直接公开回复你的初始随机密码。",
  ],
  [
    "设置密码与绑定",
    "首次用网页登录后，请先修改初始密码。接着在 AzurPilot 为你的实例注册股票，并在群内向机器人发送“绑定股票 <绑定码>”完成关联。",
  ],
  [
    "阅读行情",
    "当天的涨跌幅以北京时间（上海时区）当天第一笔报价为基准。分时图、K 线与指标都来自真实上报与撮合成交，无交易时段不会凭空捏造数据。",
  ],
  [
    "预览后再下单",
    "选好买卖方向、订单类型、价格与数量后，先点击预览查看系统预估的税费和所需保证金，确认无误再提交。注意：不能交易自己绑定的股票。",
  ],
  [
    "管理未成交余量",
    "每只股票都有库存和小时成交上限。超出的部分会自动挂单，在每个整点按价格从高到低、同价先到先得排队撮合；未成交的余量随时可以手动撤销。",
  ],
  [
    "核对损益与风险",
    "在“我的持仓”里可以随时查阅持仓成本、浮动盈亏、借券利息与交收进度。使用杠杆会同时放大收益与风险，保证金不足时系统会自动执行强制平仓。",
  ],
];
const terms = [
  [
    "分时图",
    "展示当天每一分钟的价格走势与真实成交量。图中的均价线，由开盘至今的累计成交总金额除以累计成交总股数计算得出。",
  ],
  [
    "K 线 / OHLC",
    "用柱状图形展示所选周期内的开盘价、最高价、最低价和收盘价。日 K 线按北京时间（上海时区）自然日切分，没有报价的时段不会凭空捏造数据。",
  ],
  [
    "M5 / M10 / M20",
    "分别代表 5 分钟、10 分钟、20 分钟周期的 K 线，系统同时还支持 M30（30 分钟）和 M60（1 小时）周期。",
  ],
  [
    "MA",
    "移动平均线（Moving Average）。将过去 N 个周期的收盘价相加求平均，直观呈现价格平滑后的运行趋势。",
  ],
  [
    "EXPMA",
    "指数平滑移动平均线。相比普通均线，它赋予越靠近当前的价格越高的权重，对最新的行情突破或回调更灵敏。",
  ],
  [
    "BOLL",
    "布林线指标。以 20 周期均线为中轨，上下各加减两倍标准差形成价格通道，用来观察价格的波动区间和超买超卖。",
  ],
  [
    "ENE",
    "轨道线（包络线指标）。在基准均线的上下方按固定百分比形成上下轨，常用来判断价格是否已经严重偏离正常中枢。",
  ],
  [
    "BBI",
    "多空指标。将 3 日、6 日、12 日、24 日四条不同周期的均线综合加权平均，用来一目了然地判断当前是多头占优还是空头占优。",
  ],
  [
    "MACD / DIF / DEA",
    "平滑异同移动平均线。DIF 为快慢均线差值（快线），DEA 为 DIF 的 9 日平滑线（慢线），红绿能量柱则是两线差值的两倍，常用于捕捉买卖动量与趋势背离。",
  ],
  [
    "发行库存",
    "单只股票由系统设定的最大允许发行总股数。一旦这只股票的市场总流通量达到库存上限，其他玩家就无法再买入，直到有人卖出释放配额。",
  ],
  [
    "普通小时量",
    "每个自然小时内，这只股票普通买入与卖出共用的最大成交股数额度，防止单小时内出现极端大单砸盘或暴拉。",
  ],
  [
    "借券库存",
    "系统专门预留用于融券做空的股票池上限，与普通买卖的发行库存完全独立；池子借空后将无法继续开空。",
  ],
  [
    "融券小时量",
    "每个自然小时内，卖空开仓与回补平仓共用的独立额度上限，与普通小时量互不挤占。",
  ],
  [
    "买入",
    "花费你的可用模拟现金买入其他实例的股票做多。平台禁止交易自己绑定实例的股票。",
  ],
  [
    "卖出",
    "将手头已持有并且已完成交收（变为可用状态）的持仓卖出，变现为模拟现金。",
  ],
  [
    "卖空",
    "看跌某只股票时，向系统借入该股票并以市价/限价卖出。卖空得到的资金连同你缴纳的初始保证金都会被冻结，平仓前不能挪作他用。",
  ],
  [
    "回补",
    "买入对应数量的股票归还此前借入的融券，实现平仓了结。如果做空最终产生亏损，亏损金额会自动赔付给该股票绑定的实例账户。",
  ],
  [
    "限价委托",
    "指定你的价格底线：买入或回补时成交价不会高于你的限价，卖出或卖空时成交价不会低于你的限价。",
  ],
  [
    "市价委托",
    "不限制具体价格，尽量按当前盘口最新报价以最快速度成交。最终能否成交依然受限于当前库存、小时量额度及系统风控校验。",
  ],
  [
    "止损委托",
    "预先设置一道防线，当市场最新价格达到你的触发价时，系统自动帮你转为市价单委托平仓。如果行情跳空剧烈，实际成交价可能会劣于触发价。",
  ],
  [
    "GTC",
    "长期有效委托（Good 'Til Cancelled）：挂单提交后将一直有效，直到完全成交、你手动撤单，或者本赛季结束结算清盘。",
  ],
  [
    "DAY",
    "当日有效委托：若在当天交易时段内未完全成交，挂单会在市场所属时区的次日零点自动失效，并解冻退还资金或股票。",
  ],
  [
    "部分成交",
    "当单批委托超过当前可成交的小时额度或发行库存时，会先成交额度内的部分。未成交的剩余股数会自动挂单排队，后续整点继续撮合，每笔分批均有完整记录。",
  ],
  [
    "撤单",
    "主动撤销订单中尚未成交的挂单股数。撤单成功后，系统会立即解冻并退回对应的资金或股票。",
  ],
  [
    "佣金",
    "每笔交易支付给交易所的基础手续费。系统会按成交金额的一定比例计算，若算出的金额低于设定的起征线，则按最低佣金计收。",
  ],
  [
    "总资产阶梯税",
    "茗喵默认市场的特色机制：按实际成交前一刻账户的“总资产”档位确定税率，对整笔成交金额统一征收阶梯交易税。",
  ],
  [
    "印花税",
    "依据所选交易市场的规则收取的交易印花税。不同市场可能针对卖出单向或买卖双向征收，税率与取整单位以该市场的具体设置为准。",
  ],
  [
    "交易征费",
    "部分参考现实规则的市场所征收的额外规费或结算征费，通常按成交额的微小比例扣除。",
  ],
  [
    "融资杠杆",
    "向系统借入模拟资金放大买入规模以博取更大收益。融资需要随持仓时间支付利息，且账户整体净资产必须始终维持在安全线以上。",
  ],
  [
    "初始保证金 IMR",
    "开仓保证金比例（Initial Margin Requirement）。开启杠杆买入或融券做空新仓位时，账户内必须具备的最低自有资金比例要求。",
  ],
  [
    "维持保证金 MMR",
    "维持保证金比例（Maintenance Margin Requirement）。持仓过程中必须始终保有的最低净资产占比底线，一旦跌破该比例将触发强平风控。",
  ],
  [
    "强制平仓",
    "俗称“爆仓”。当行情不利导致账户净资产跌破维持保证金（MMR）要求时，系统风控会自动以市价单将仓位强制平仓，以防亏损继续穿透。",
  ],
  [
    "借券 / 融资利息",
    "持仓过程中按时间持续计提的融券或融资利息。若遇标的价格或借贷利率调整，系统会先结算前一时段利息；不足 1 分钱的零头会保留到后续时段结转。",
  ],
  [
    "T+N 交收",
    "买入的股票或卖出换回的现金，需要等待 N 个交易日交收完成后才变为“可用”状态。例如 T+1 意味着今天买入的股票次日才能卖出。",
  ],
  [
    "浮盈亏 / 已实现盈亏",
    "浮动盈亏是持仓尚未卖出时按现价计算的纸面盈利或亏损；已实现盈亏则是平仓了结后真正打入账户余额的最终收益。",
  ],
  [
    "净资产 / 可用现金",
    "净资产代表你账户里的全部身家（总资产减去借款等负债）；可用现金则是扣除挂单冻结和保证金后，当前随时可以用来开新仓的自由资金。",
  ],
  [
    "月赛 / 退市",
    "月度赛季按北京时间运行。每月赛季结算时会重置玩家的赛季资金与持仓排名，但保留你的账号与历史数据；若绑定的实例长期低活跃或缺乏行动力，可能在当月被判退市。",
  ],
];
export function GuidePage() {
  const x = useExchange(),
    [tab, setTab] = useState("start"),
    [query, setQuery] = useState(""),
    settings = x.settings;
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">交易学堂</p>
          <h1>交易指南</h1>
          <p>从开户到第一笔委托，了解模拟市场的规则与风险。</p>
        </div>
        <LinkButton href="https://qm.qq.com/q/93NWF3S362" target="_blank">
          加入官方 QQ 群
        </LinkButton>
      </div>
      <Tabs
        variant="underline"
        value={tab}
        onValueChange={setTab}
        tabs={[
          { value: "start", label: "新手六步" },
          { value: "terms", label: `名词词典 ${terms.length}` },
          { value: "rules", label: "当前交易规则" },
        ]}
      />
      {tab === "start" && (
        <>
          <div className="guide-grid">
            {lessons.map(([title, text], i) => (
              <LayerCard key={title} className="card-body">
                <p className="eyebrow">0{i + 1}</p>
                <h2>{title}</h2>
                <p className="muted">{text}</p>
              </LayerCard>
            ))}
          </div>
          <Banner
            variant="secondary"
            title="这是一场模拟证券游戏"
            description="本平台纯属模拟演练游戏，不涉及任何真实资金，也不提供充值或提现功能。实例签名仅用于确认数据上报来源，并不代表游戏数据的真实性。"
          />
        </>
      )}
      {tab === "terms" && (
        <>
          <div className="toolbar-search">
            <InputGroup>
              <InputGroup.Addon>
                <MagnifyingGlassIcon size={18} />
              </InputGroup.Addon>
              <InputGroup.Input
                aria-label="搜索交易名词"
                placeholder="搜索做空、保证金、MACD…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </InputGroup>
          </div>
          <LayerCard className="card-body">
            {terms
              .filter((t) =>
                t.join(" ").toLowerCase().includes(query.toLowerCase()),
              )
              .map(([title, text]) => (
                <Collapsible.Root key={title}>
                  <Collapsible.DefaultTrigger>
                    {title}
                  </Collapsible.DefaultTrigger>
                  <Collapsible.DefaultPanel>
                    <p className="prose">{text}</p>
                  </Collapsible.DefaultPanel>
                </Collapsible.Root>
              ))}
          </LayerCard>
        </>
      )}
      {tab === "rules" &&
        (settings ? (
          <div className="stack">
            <LayerCard className="card-body">
              <h2>{settings.activeMarket.name}</h2>
              <p className="muted">{settings.activeMarket.description}</p>
              <dl className="definition-list">
                {[
                  ["新账户初始模拟币", money(settings.initialCash)],
                  ["每手股数", String(settings.activeMarket.lotSize)],
                  ["预设时区", settings.activeMarket.timezone],
                  ["交易时段", settings.activeMarket.sessions.join("，")],
                  [
                    "普通发行 / 小时量",
                    `${settings.issuedShares} / ${settings.hourlyVolume}`,
                  ],
                  [
                    "借券库存 / 小时量",
                    `${settings.borrowShares} / ${settings.borrowHourlyVolume}`,
                  ],
                  ["最大流动性溢价", percent(settings.premiumPPM)],
                  [
                    "佣金",
                    percent(
                      settings.activeMarket.id === "meow"
                        ? settings.commissionPPM
                        : settings.activeMarket.commissionPPM,
                    ),
                  ],
                  [
                    "最低佣金",
                    money(
                      settings.activeMarket.id === "meow"
                        ? settings.minCommission
                        : settings.activeMarket.minCommission,
                    ),
                  ],
                  [
                    "初始 / 维持保证金",
                    `${percent(settings.activeMarket.imrPPM)} / ${percent(settings.activeMarket.mmrPPM)}`,
                  ],
                  [
                    "股票 / 现金交收",
                    `T+${settings.activeMarket.sellDelayDays} / T+${settings.activeMarket.cashDelayDays}`,
                  ],
                  [
                    "休市日期",
                    settings.activeMarket.holidays.join("，") ||
                      "按常规时段交易，无额外休市",
                  ],
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            </LayerCard>
            <LayerCard className="data-card">
              <div className="card-heading">
                <h2>总资产阶梯税率</h2>
                <span className="caption">仅茗喵默认市场</span>
              </div>
              <Table>
                <Table.Header>
                  <Table.Row>
                    <Table.Head>总资产上限</Table.Head>
                    <Table.Head>税率</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {settings.taxBrackets.map((b, i) => (
                    <Table.Row key={i}>
                      <Table.Cell>
                        {b.upTo ? money(b.upTo) : "无上限"}
                      </Table.Cell>
                      <Table.Cell>{percent(b.ratePPM)}</Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table>
            </LayerCard>
            <Banner
              variant="alert"
              description="不同交易市场的预设参数仅供模拟教学参考，并未完整复刻现实中各大证券交易所的整手、币种、税收和节假日规则。所有资金变动、交易权限与最终撮合成交结果，均以系统服务端的校验为准。"
            />
          </div>
        ) : (
          <Empty title="正在读取规则" />
        ))}
    </>
  );
}

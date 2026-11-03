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
    "加入官方 QQ 群，向机器人发送“注册 <用户名>”。机器人在原群返回随机初始密码。",
  ],
  [
    "设置密码与绑定",
    "首次网页登录后修改密码。在 AzurPilot 注册实例股票，再把“绑定股票 <绑定码>”发给原群机器人。",
  ],
  [
    "阅读行情",
    "今日涨跌幅以上海当日首条报价为基准。分时、K 线和指标来自已保存观测与实际成交，不补造空档。",
  ],
  [
    "预览后再下单",
    "选择方向、类型、价格与股数。先预览服务器计算的税费和保证金，再确认提交；不得交易自己的股票。",
  ],
  [
    "管理未成交余量",
    "库存和小时成交量有限。超额部分挂单，后续整点按价格从高到低、同价先到先得处理，可撤销未成交余量。",
  ],
  [
    "核对损益与风险",
    "在我的资产检查持仓成本、浮盈亏、融资、借券和交收。杠杆会放大风险，保证金不足可能由服务器强制平仓。",
  ],
];
const terms = [
  [
    "分时图",
    "所选日期每分钟的价格与实际成交量。成交均价为累计成交额除以累计成交股数。",
  ],
  [
    "K 线 / OHLC",
    "周期内首价、最高、最低和末价。日 K 按上海日期分桶，无报价时不补造采样。",
  ],
  ["M5 / M10 / M20", "分别为 5、10、20 分钟 K 线，另支持 M30、M60。"],
  ["MA", "过去 N 个周期收盘价的算术平均值。"],
  ["EXPMA", "指数加权平均，近期报价拥有更高权重。"],
  ["BOLL", "20 周期均线加减两倍标准差形成的轨道。"],
  ["ENE", "均线的百分比包络线，用于观察价格偏离。"],
  ["BBI", "3、6、12、24 周期均线的平均值。"],
  [
    "MACD / DIF / DEA",
    "DIF 为 EMA12 减 EMA26；DEA 为 DIF 的 EMA9；柱为两者差的两倍。",
  ],
  ["发行库存", "单支股票可以发行的最大股数，不能无限买入。"],
  ["普通小时量", "本自然小时买入与卖出共用的成交额度。"],
  ["借券库存", "允许借出的股票总量，与普通发行库存独立。"],
  ["融券小时量", "卖空和回补共用的独立小时额度。"],
  ["买入", "花费模拟现金获得其他实例股票。"],
  ["卖出", "出售已经持有且可以交付的多头股票。"],
  ["卖空", "借入已绑定的其他股票并卖出，卖空所得和保证金冻结。"],
  ["回补", "买入股票归还借券；实际亏损补偿给股票绑定账户。"],
  ["限价委托", "买入或回补不高于限价，卖出或卖空不低于限价。"],
  ["市价委托", "以当前报价预览，最终仍受库存、小时量与成交核验约束。"],
  ["止损委托", "达到触发价格后转为市价委托，跳空可能造成更差成交价格。"],
  ["GTC", "委托持续至成交、撤销或本赛季截止。"],
  ["DAY", "委托在市场时区次日零点到期，释放冻结。"],
  ["部分成交", "额度内先成交，剩余股数继续挂单并保留完整逐笔记录。"],
  ["撤单", "撤销未成交余量，释放相应冻结资金和股票。"],
  ["佣金", "按成交额费率及最低佣金计算的交易费用。"],
  ["总资产阶梯税", "按成交前总资产所属档位，对本次整个成交额征税。"],
  ["印花税", "按所选市场预设的方向、费率和取整单位征收。"],
  ["交易征费", "部分市场预设的额外成交费用。"],
  ["融资杠杆", "借款买入放大敞口，需要支付融资费用并维持保证金。"],
  ["初始保证金 IMR", "开仓时要求的自有资金比例。"],
  ["维持保证金 MMR", "持仓需要维持的最低保证金要求。"],
  ["强制平仓", "保证金不足等风控条件下由服务器执行的平仓成交。"],
  [
    "借券 / 融资利息",
    "随持仓时间计提，价格或利率变更前结算旧区间，分以下余数结转。",
  ],
  ["T+N 交收", "股票或现金在预设的后续交易日到期可用。"],
  [
    "浮盈亏 / 已实现盈亏",
    "尚未平仓的市值变化与已经平仓落账的损益，不能混为一项。",
  ],
  ["净资产 / 可用现金", "净资产包含持仓与负债，可用现金需扣除冻结等占用。"],
  [
    "月赛 / 退市",
    "月份按上海时间；赛季结算重置赛季资金与持仓，保留账户和历史，实时低行动力可触发本月退市。",
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
            description="没有真实资金、充值或提现。实例签名确认上报来源，不保证游戏数据真实。"
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
                      "以配置时段为准",
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
              description="现实市场预设仅作教学参考，不完整复刻各交易所的整手、币种、税务与节假日规则。最终资金、权限与成交以服务器核验为准。"
            />
          </div>
        ) : (
          <Empty title="正在读取规则" />
        ))}
    </>
  );
}

import {Learn} from './Learn'
import {ArrowUpRight,BookOpen} from 'lucide-react'
import type {ReactNode} from 'react'
import {datetime,money,number,rate} from './api'
import type {Market,Settings} from './types'

function Explanation({index,title,children}:{index:string;title:string;children:ReactNode}){
  return <section><span className="section-number">{index}</span><div><h2>{title}</h2>{children}</div></section>
}

export function Rules({settings:s,market}:{settings?:Settings;market?:Market}){
  const r=s?.activeMarket,game=!r||r.id==='meow'
  const commission=game?s?.commissionPPM:r.commissionPPM
  const minCommission=game?s?.minCommission:r.minCommission
  const borrow=game?s?.borrowAnnualPPM:r.borrowAnnualPPM
  const ttl=game?s?.quoteTTLSeconds:r.quoteTTLSeconds
  const rows=[
    ['默认发行总量',`${number(s?.issuedShares??10000)} 股 / 股票`,'单只股票库存有限，管理员可单独调整'],
    ['普通交易额度',`${number(s?.hourlyVolume??1000)} 股 / 小时`,'买入与卖出共用该股票的普通交易额度'],
    ['可借券总量',`${number(s?.borrowShares??10000)} 股 / 股票`,'与普通发行库存独立计算'],
    ['融券交易额度',`${number(s?.borrowHourlyVolume??1000)} 股 / 小时`,'卖空与回补共用独立额度'],
    ['流动性溢价',game?`最高 ${rate(s?.premiumPPM??50000)}`:'现实参考预设不启用','茗喵新默认随小时利用率增加，成交不突破限价'],
    ['佣金',`${rate(commission??300)}，最低 ${money(minCommission??500)} 模拟币`,'委托预览展示明细，实际收费以成交回执为准'],
    ['印花税',rate(r?.stampPPM??0),r?.stampSide==='both'?'买卖双方征收':'卖出 / 卖空侧征收'],
    ['交易征费',rate(r?.levyPPM??0),'与佣金、印花税分别计算'],
    ['借券年费率',rate(borrow??80000),'按实际未平仓时间与借券市值结算'],
    ['融资年利率',rate(r?.financingAnnualPPM??80000),'融资买入才产生借款，按本金和时间计费'],
    ['报价有效时间',`${number(ttl??86400)} 秒`,'无有效报价时暂停新成交，行情空档不伪造数据'],
    ['开户初始模拟币',money(s?.initialCash??2000000000),'用于新开户和新月赛重置，不在保存配置时改写现有余额']
  ]
  return <>
    <div className="page-heading"><div><div className="eyebrow">KNOW THE RULES</div><h1>交易规则</h1><p>从开户、定价到多空交易，清楚理解每一次委托。</p></div><BookOpen className="heading-icon" size={36}/></div>
    <section className="panel rules-summary">
      <div className="panel-heading"><h2>市场规则一览</h2><span className="tag">{r?.name??'茗喵全天候'}</span></div>
      <div className="table-scroll"><table className="rules-table"><thead><tr><th>项目</th><th>当前规则</th><th>说明</th></tr></thead><tbody>{rows.map(([a,b,c])=><tr key={a}><th>{a}</th><td>{b}</td><td>{c}</td></tr>)}</tbody></table></div>
    </section>
    <section className="panel rules-summary">
      <div className="panel-heading"><h2>交易时段、交收与保证金</h2><span className="tiny-label">{market?.reason??'当前市场参数'}</span></div>
      <div className="table-scroll"><table className="rules-table"><thead><tr><th>项目</th><th>当前规则</th><th>说明</th></tr></thead><tbody>
        <tr><th>整手单位</th><td>{number(r?.lotSize??1)} 股</td><td>委托股数须为整手单位的整数倍</td></tr>
        <tr><th>交易时段</th><td>{r?.sessions.join(' / ')??'00:00-24:00'}</td><td>{r?.timezone??'Asia/Shanghai'}{r?.weekdaysOnly?'，周末休市':'，周末可交易'}；另遵守配置的休市日</td></tr>
        <tr><th>股票可卖延迟</th><td>T+{r?.sellDelayDays??0} 交易日</td><td>买入后未完成交收的股份不能提前卖出</td></tr>
        <tr><th>现金可用延迟</th><td>T+{r?.cashDelayDays??0} 交易日</td><td>卖出所得计入余额，到期前不能再次用于交易</td></tr>
        <tr><th>初始 / 维持保证金</th><td>{rate(r?.imrPPM??500000)} / {rate(r?.mmrPPM??300000)}</td><td>限制融资与做空杠杆；维持保证金不足触发风控平仓</td></tr>
        <tr><th>月赛时间</th><td>每月 {s?.startDay??5} 日开赛，月末前 {s?.endDaysFromLast??4} 天截止</td><td>上海时间，配置日期以当前赛季实际窗口为准</td></tr>
        {market?.season&&<tr><th>当前赛季</th><td>{market.season.id}</td><td>{datetime(market.season.startsAt)} 至 {datetime(market.season.endsAt)}{market.season.settled?'，已结算':''}</td></tr>}
        <tr><th>退市阈值</th><td>{s?.delistThreshold===0?'已关闭':`${number(s?.delistThreshold??500)} 行动力`}</td><td>每月 5 日起且开赛后，实时行情低于阈值触发本月退市结算</td></tr>
      </tbody></table></div>
    </section>
    <section className="panel rules-summary">
      <div className="panel-heading"><h2>总资产阶梯税率</h2><span className="tiny-label">{game?'当前启用':'仅茗喵新默认启用；当前现实预设不叠加此税'}</span></div>
      <div className="table-scroll"><table className="rules-table"><thead><tr><th>成交前总资产（模拟币）</th><th>税率</th></tr></thead><tbody>{(s?.taxBrackets??[{upTo:3000000000,ratePPM:0},{upTo:8000000000,ratePPM:80000},{upTo:10000000000,ratePPM:160000},{upTo:0,ratePPM:320000}]).map((b,i,all)=><tr key={i}><td>{i===0?`[0, ${money(b.upTo)}]`:b.upTo?`(${money(all[i-1].upTo)}, ${money(b.upTo)}]`:`大于 ${money(all[i-1].upTo)}`}</td><td><strong>{rate(b.ratePPM)}</strong></td></tr>)}</tbody></table></div>
    </section>
    {r&&<Learn rules={{...r,commissionPPM:commission??0,minCommission:minCommission??0,borrowAnnualPPM:borrow??0,quoteTTLSeconds:ttl??86400}} initialCash={s?.initialCash??2000000000} delistThreshold={s?.delistThreshold??0}/>}
    <article className="panel rules-article">
      <Explanation index="01" title="两个账户，各司其职">
        <p>交易账户用于持有模拟币、买卖股票和管理委托。在 QQ 群向官方机器人发送“注册 &lt;用户名&gt;”直接创建账号并获得随机初始密码，然后在本站登录并强制改密。每个官方机器人用户身份只能注册一个交易账户，网页登录使用账号与密码。股票账户代表一个 AzurPilot 实例，在实例页面点击“茗喵证券交易所”并填写用户名即可注册，无需先有交易账户。</p>
        <p>在 AzurPilot 获取私密绑定码，再在群内向机器人发送“绑定股票 绑定码”，即可建立一对一关系。交易账户未绑定时不可使用；管理员删除股票后解除绑定，需重新绑定其他股票。未绑定股票仍可上报和展示行情；绑定后才可被其他交易者卖空。不得交易自己绑定的股票，绑定前应先处理该股票的持仓和挂单。</p>
        <p>初始密码由机器人在原群内公开回复。首次开户或重置密码后，必须在本站设置新密码，才能进行交易。重置会撤销旧登录状态。选择“记住我”可维持 30 天登录，管理员与交易账户会话相互独立。</p>
      </Explanation>
      <Explanation index="02" title="股价、库存与行情图">
        <p>每股基础价等于实例最近上报的总行动力，1 点行动力对应 1 模拟币。图表以聚合开、高、低、收价格反映走势与波动，支持分时、日 K 和分钟 K，以及均线、MACD 等指标与逐笔成交量额。原始记录保存在服务器，不向交易客户端下载整份行动力历史。历史补传和同毫秒修正会更新图表，但不会重做过去的成交。</p>
        <p>普通发行库存有限，买入占用库存，卖出后返还；借券使用另一套独立库存。所有市场模式都遵守有限股数和小时额度。每支股票的实际额度以行情详情为准，管理员可分别调整，可能不同于新股票的默认值。行情空档不补造采样。</p>
      </Explanation>
      <Explanation index="03" title="限价、小时额度与挂单">
        <p>选择股票、方向、股数和限价，先查看明细，再确认提交。买入与回补只能以不高于限价的价格成交，卖出与卖空只能以不低于限价的价格成交。数量必须是当前市场整手单位的正整数倍，限价为正数且最多保留两位小数。</p>
        <p>额度内可以立即成交。额度、库存或价格条件不足时，未成交部分继续挂单，并冻结相应资金或股票。每个自然小时整点恢复额度，挂单按限价从高到低处理，同价先到先得。休市、月赛未开始或已截止时仍需遵守市场时间规则，不会为了撮合而越过限价。未成交部分可在“持仓与委托”撤销。</p>
        <p>茗喵新默认的溢价随当小时额度使用率增加。买入与回补承担向上的溢价，卖出与卖空承担向下的折价。因此基础价不保证就是成交价。普通买卖和融券交易分别计算小时用量；现实参考预设不启用这项新增溢价。</p>
      </Explanation>
      <Explanation index="04" title="买入、卖出与融资">
        <p>现金买入使用自己的模拟币取得股票；融资买入按允许的杠杆借入部分模拟币。融资借款不是免费资金，持仓资产扣除未偿借款后才是自己的权益，持仓期间按借款本金与时间产生利息。允许的杠杆由初始保证金参数决定，保证金不足会触发强平。</p>
        <p>例如，每股 100 模拟币买入 10 股，成交额为 1,000 模拟币。之后每股 120 模拟币卖出，未扣费用的价差为 200 模拟币。若使用融资，卖出时还需偿还对应借款，实际到账需扣税费和利息。相同价格涨跌下，杠杆会放大自有资金收益和损失。</p>
        <p>卖出只能使用实际持有、未冻结且已到可卖日的股票。A 股参考默认 T+1 可卖；港股参考默认卖出所得 T+2 可用；美股参考默认卖出所得 T+1 可用。交易日计算尊重所选市场的时区、周末与休市日，余额与可用现金可能不同。</p>
      </Explanation>
      <Explanation index="05" title="卖空、回补与发行者补偿">
        <p>卖空仅限已经绑定交易账户的股票，且不能卖空自己绑定的股票。卖空先借股出售，账户需提供现金保证金，卖空所得冻结；回补是买回并归还借券，结束对应空头持仓。不能把冻结的卖空所得当作可用现金无限加仓。</p>
        <p>例如，每股 100 模拟币卖空 10 股，之后以每股 80 模拟币回补，未扣税费和借券费的盈利为 200 模拟币；若以每股 120 模拟币回补，价格亏损为 200 模拟币。回补时实际收取的价格亏损，会以现金加入股票绑定的交易账户。回补盈利或仍未平仓时，发行者不会因此扣款，也不承担交易者债务。</p>
        <p>借券费用按实际持仓时间结算。价格或借息参数变化前先结算旧区间，不用新价或新费率追算之前时间。保证金不足触发风控平仓，损失补偿以实际可收取部分为限；发行者不承担回补缺口。</p>
      </Explanation>
      <Explanation index="06" title="税费、资产与成交回执">
        <p>茗喵新默认按每次成交前总资产所属档位，对该次整个成交金额征税，不是只对超过门槛的部分征税，也不是只对盈利征税。总资产包含现金与持仓权益，扣除融资借款和空头负债，冻结资金不重复计入。</p>
        <p>默认资产恰好 3,000 万模拟币适用 0%；超过 3,000 万至 8,000 万为 8%；超过 8,000 万至 1 亿为 16%；超过 1 亿为 32%。现实参考预设保留各自印花税、征费与佣金，不叠加这套新增阶梯税。费用以整数分结算，向上取整规则及印花税取整单位以当前参数为准。</p>
        <p>部分成交可能发生在不同时间、价格和资产档位，预览金额与最终费用可能不同。确认前请分别查看税、佣金、印花税、征费、融资金额和预估收支。网络重试不会重复确认同一委托；余额、冻结、库存、回执和发行者补偿必须一起成功保存才算生效。</p>
      </Explanation>
      <Explanation index="07" title="现实参考预设如何生效">
        <p>管理员可选择茗喵全天候、A 股参考、港股参考或美股参考，并调整当前生效参数。现实预设的原有费率、整手、交易时段、交收与保证金参数独立保留，新的默认发行、额度、溢价和阶梯税单独配置。选择预设不会改写仓库中的现实参考文件。</p>
        <p>现实参考是模拟游戏中的规则组合。共同的有限发行量、独立融券库存和小时额度仍然适用；行情仍来自实例行动力。切换市场模式不会绕过资金、身份、绑定、限价与库存检查。{r?.source&&<a href={r.source} target="_blank" rel="noreferrer">查看当前预设参考来源 <ArrowUpRight size={12}/></a>}</p>
      </Explanation>
      <Explanation index="08" title="月赛、退市与历史">
        <p>月赛按上海时间划分，默认每月 5 日开赛，倒数第 5 日结束时截止。截止时取消剩余挂单、结算持仓并保存赛季结果；进入新月份时重置赛季资金与持仓，保留交易账户、股票绑定、凭据和行情历史。日期配置调整的生效窗口以当前赛季实际起止时间为准。</p>
        <p>每月 5 日起且赛季已开始，实时行动力低于退市阈值的股票会在本月退市，相关持仓和挂单按后端规则结算。管理员将阈值设为 0 可关闭该机制。旧历史补传不重新触发过去的退市和交易，下一月按新赛季规则恢复。</p>
      </Explanation>
      <div className="rules-help"><p>需要开户、重置密码或进一步帮助？</p><a href="https://qm.qq.com/q/93NWF3S362" target="_blank" rel="noreferrer">加 QQ 群 1126124220 <ArrowUpRight size={16}/></a><small>本项目是模拟证券游戏，所有余额、税费和成交均为游戏数据。</small></div>
    </article>
  </>
}

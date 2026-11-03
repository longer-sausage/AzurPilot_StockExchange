"use client";
import { useState } from "react";
import {
  Banner,
  Button,
  Checkbox,
  Input,
  LayerCard,
  Select,
  Table,
  Tabs,
  Textarea,
} from "@cloudflare/kumo";
import type { Rules, Settings } from "../core/contracts";
export function SettingsEditor({
  value,
  onChange,
  onSave,
  busy,
}: {
  value: Settings;
  onChange: (s: Settings) => void;
  onSave: () => void;
  busy: boolean;
}) {
  const [tab, setTab] = useState("market"),
    [newId, setNewId] = useState(""),
    [newName, setNewName] = useState(""),
    r = value.activeMarket;
  const update = (key: keyof Settings, v: unknown) =>
      onChange({ ...value, [key]: v }),
    rule = (key: keyof Rules, v: unknown) =>
      update("activeMarket", { ...r, [key]: v });
  const number = (
    label: string,
    key: keyof Settings,
    scale = 1,
    description?: string,
  ) => (
    <Input
      key={key}
      label={label}
      type="number"
      min={0}
      step={scale === 100 ? ".01" : scale === 10000 ? ".0001" : "1"}
      value={Number(value[key]) / scale}
      description={description}
      onChange={(e) => update(key, Math.round(Number(e.target.value) * scale))}
    />
  );
  return (
    <>
      <LayerCard className="card-body">
        <div className="toolbar">
          <div>
            <h2>交易规则草稿</h2>
            <p className="caption">保存后由服务器验证并立即生效。</p>
          </div>
          <Button variant="primary" loading={busy} onClick={onSave}>
            保存交易规则
          </Button>
        </div>
        <Tabs
          value={tab}
          onValueChange={setTab}
          variant="underline"
          tabs={[
            { value: "market", label: "市场与预设" },
            { value: "capacity", label: "库存与额度" },
            { value: "fees", label: "费用与税率" },
            { value: "season", label: "赛季与退市" },
          ]}
        />
      </LayerCard>
      {tab === "market" && (
        <LayerCard className="card-body">
          <h2>市场预设</h2>
          <Select
            label="选用已保存预设"
            value={r.id}
            onValueChange={(id) => {
              const item = value.marketPresets.find((p) => p.id === id);
              if (item) update("activeMarket", structuredClone(item));
            }}
            items={value.marketPresets.map((p) => ({
              value: p.id,
              label: p.name,
            }))}
          />
          <Banner
            variant="alert"
            description="切换市场预设并保存会取消未成交余量、释放冻结，旧交易预览需重新生成。已成交记录保留。"
          />
          <div className="fields">
            <Input
              label="当前预设名称"
              value={r.name}
              onChange={(e) => rule("name", e.target.value)}
            />
            <Input
              label="市场时区"
              value={r.timezone}
              onChange={(e) => rule("timezone", e.target.value)}
            />
            <Input
              label="参考来源 URL"
              type="url"
              value={r.source}
              onChange={(e) => rule("source", e.target.value)}
            />
            <Input
              label="交易时段"
              value={r.sessions.join(",")}
              description="HH:MM-HH:MM，用英文逗号分隔。"
              onChange={(e) =>
                rule(
                  "sessions",
                  e.target.value.split(",").map((s) => s.trim()),
                )
              }
            />
            <Input
              label="休市日期"
              value={r.holidays.join(",")}
              description="YYYY-MM-DD，用英文逗号分隔。"
              onChange={(e) =>
                rule(
                  "holidays",
                  e.target.value
                    .split(",")
                    .map((s) => s.trim())
                    .filter(Boolean),
                )
              }
            />
          </div>
          <Textarea
            label="预设说明"
            value={r.description}
            onChange={(e) => rule("description", e.target.value)}
          />
          <Checkbox
            label="周末休市"
            checked={r.weekdaysOnly}
            onCheckedChange={(v) => rule("weekdaysOnly", v)}
          />
          <div className="fields">
            <Input
              label="新预设 ID"
              value={newId}
              onChange={(e) => setNewId(e.target.value)}
              description="英文小写开头，只允许英文、数字和连字符。"
            />
            <Input
              label="新预设名称"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
            />
          </div>
          <div className="toolbar-group">
            <Button
              disabled={
                !/^[a-z][a-z0-9-]*$/.test(newId) ||
                !newName ||
                value.marketPresets.some((p) => p.id === newId)
              }
              onClick={() => {
                const next = { ...r, id: newId, name: newName };
                onChange({
                  ...value,
                  activeMarket: next,
                  marketPresets: [...value.marketPresets, next],
                });
                setNewId("");
                setNewName("");
              }}
            >
              另存为新预设
            </Button>
            <Button
              onClick={() =>
                update(
                  "marketPresets",
                  value.marketPresets.map((p) =>
                    p.id === r.id ? structuredClone(r) : p,
                  ),
                )
              }
            >
              更新已保存预设
            </Button>
            <Button
              variant="secondary-destructive"
              disabled={value.marketPresets.length <= 1}
              onClick={() => {
                const presets = value.marketPresets.filter(
                  (p) => p.id !== r.id,
                );
                onChange({
                  ...value,
                  marketPresets: presets,
                  activeMarket: structuredClone(presets[0]),
                });
              }}
            >
              删除当前预设
            </Button>
          </div>
        </LayerCard>
      )}
      {tab === "capacity" && (
        <LayerCard className="card-body">
          <h2>公共数量参数</h2>
          <div className="fields-three">
            {number(
              "新账户初始模拟币",
              "initialCash",
              100,
              "只影响新开户，不改已有账户余额。",
            )}
            {number("默认发行股数", "issuedShares", 1, "仅新股票生效。")}
            {number("普通小时量", "hourlyVolume")}
            {number("借券库存", "borrowShares")}
            {number("融券小时量", "borrowHourlyVolume")}
            {number("报价有效期（秒）", "quoteTTLSeconds")}
          </div>
          <h3>当前市场交收与保证金</h3>
          <div className="fields-three">
            {(
              [
                ["每手股数", "lotSize", 1],
                ["买入后可卖延迟（交易日）", "sellDelayDays", 1],
                ["卖出后现金可用延迟（交易日）", "cashDelayDays", 1],
                ["初始保证金（%）", "imrPPM", 10000],
                ["维持保证金（%）", "mmrPPM", 10000],
                ["预设报价有效期（秒）", "quoteTTLSeconds", 1],
              ] as const
            ).map(([label, key, scale]) => (
              <Input
                key={key}
                label={label}
                type="number"
                min={0}
                value={r[key] / scale}
                onChange={(e) =>
                  rule(key, Math.round(Number(e.target.value) * scale))
                }
              />
            ))}
          </div>
          <Checkbox
            label="暂停市场成交"
            checked={value.paused}
            onCheckedChange={(v) => update("paused", v)}
          />
        </LayerCard>
      )}
      {tab === "fees" && (
        <div className="stack">
          <LayerCard className="card-body">
            <h2>茗喵默认费率</h2>
            <div className="fields-three">
              {number("最大溢价（%）", "premiumPPM", 10000)}
              {number("借券年费率（%）", "borrowAnnualPPM", 10000)}
              {number("佣金（%）", "commissionPPM", 10000)}
              {number("最低佣金（模拟币）", "minCommission", 100)}
            </div>
            <h3>当前预设费率</h3>
            <div className="fields-three">
              {(
                [
                  ["预设佣金（%）", "commissionPPM", 10000],
                  ["最低佣金（模拟币）", "minCommission", 100],
                  ["印花税（%）", "stampPPM", 10000],
                  ["印花税取整单位（模拟币）", "stampRound", 100],
                  ["交易征费（%）", "levyPPM", 10000],
                  ["融资年利率（%）", "financingAnnualPPM", 10000],
                  ["预设借券年费（%）", "borrowAnnualPPM", 10000],
                ] as const
              ).map(([label, key, scale]) => (
                <Input
                  key={key}
                  label={label}
                  type="number"
                  step={scale === 100 ? ".01" : ".0001"}
                  min={0}
                  value={r[key] / scale}
                  onChange={(e) =>
                    rule(key, Math.round(Number(e.target.value) * scale))
                  }
                />
              ))}
              <Select
                label="印花税方向"
                value={r.stampSide}
                onValueChange={(v) => rule("stampSide", String(v))}
                items={{ sell: "卖出与卖空", both: "买卖双方" }}
              />
            </div>
          </LayerCard>
          <LayerCard className="data-card">
            <div className="card-heading">
              <h2>总资产阶梯税率</h2>
              <span className="caption">末档上限 0 表示无上限</span>
            </div>
            <div className="table-region">
              <Table className="min-w-[480px]">
                <Table.Header>
                  <Table.Row>
                    <Table.Head>档位</Table.Head>
                    <Table.Head>总资产上限（模拟币）</Table.Head>
                    <Table.Head>税率（%）</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {value.taxBrackets.map((b, i) => (
                    <Table.Row key={i}>
                      <Table.Cell>{i + 1}</Table.Cell>
                      <Table.Cell>
                        <Input
                          aria-label={`第${i + 1}档资产上限`}
                          type="number"
                          min={0}
                          step=".01"
                          value={b.upTo / 100}
                          onChange={(e) =>
                            update(
                              "taxBrackets",
                              value.taxBrackets.map((v, j) =>
                                i === j
                                  ? {
                                      ...v,
                                      upTo: Math.round(
                                        Number(e.target.value) * 100,
                                      ),
                                    }
                                  : v,
                              ),
                            )
                          }
                        />
                      </Table.Cell>
                      <Table.Cell>
                        <Input
                          aria-label={`第${i + 1}档税率`}
                          type="number"
                          min={0}
                          max={100}
                          step=".0001"
                          value={b.ratePPM / 10000}
                          onChange={(e) =>
                            update(
                              "taxBrackets",
                              value.taxBrackets.map((v, j) =>
                                i === j
                                  ? {
                                      ...v,
                                      ratePPM: Math.round(
                                        Number(e.target.value) * 10000,
                                      ),
                                    }
                                  : v,
                              ),
                            )
                          }
                        />
                      </Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table>
            </div>
          </LayerCard>
        </div>
      )}
      {tab === "season" && (
        <LayerCard className="card-body">
          <h2>月赛与退市</h2>
          <Banner
            variant="secondary"
            description="时间统一按上海时区。已建立的本月窗口保持不变，日期调整从后续月份生效。"
          />
          <div className="fields-three">
            {number("每月开赛日", "startDay")}
            {number(
              "截止距月末天数",
              "endDaysFromLast",
              1,
              "倒数第 5 日填写 4。",
            )}
            {number(
              "退市行动力阈值",
              "delistThreshold",
              1,
              "0 关闭，历史补传不会重放退市。",
            )}
          </div>
        </LayerCard>
      )}
    </>
  );
}

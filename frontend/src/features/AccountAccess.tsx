"use client";
import { useEffect, useState } from "react";
import {
  Banner,
  Button,
  Checkbox,
  Dialog,
  Empty,
  Input,
  LinkButton,
} from "@cloudflare/kumo";
import { XIcon, LockKeyIcon } from "@phosphor-icons/react";
import { useExchange, useNotice } from "../core/Exchange";
import { request } from "../core/http";
import type { Account } from "../core/contracts";

export function AccountAccess() {
  const x = useExchange(),
    notice = useNotice();
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [current, setCurrent] = useState(""),
    [repeat, setRepeat] = useState(""),
    [remember, setRemember] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const forced = !!x.account?.forcePasswordChange,
    binding =
      !!x.account && !forced && (!x.account.boundStockId || x.bindingOpen);
  useEffect(() => {
    setError("");
    setPassword("");
    setCurrent("");
    setRepeat("");
  }, [x.loginOpen, forced]);
  async function login() {
    setBusy(true);
    setError("");
    try {
      const result = await request<{
        account: Account;
        forcePasswordChange: boolean;
      }>("/auth/login", { username, password, remember });
      x.setAccount({
        ...result.account,
        forcePasswordChange: result.forcePasswordChange,
      });
      x.setLoginOpen(false);
      await x.refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function change() {
    if (password !== repeat) {
      setError("两次输入的新密码不一致，请重新确认");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await request("/auth/password", { currentPassword: current, password });
      await x.refresh();
      notice("新密码已设置成功");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Dialog.Root
        open={x.loginOpen && !forced}
        onOpenChange={(open) => {
          if (!busy) x.setLoginOpen(open);
        }}
      >
        <Dialog size="lg" className="dialog-content">
          <div className="dialog-title">
            <div>
              <p className="caption">欢迎回来</p>
              <Dialog.Title>登录交易账户</Dialog.Title>
            </div>
            <Button
              variant="ghost"
              shape="square"
              aria-label="关闭登录"
              disabled={busy}
              onClick={() => x.setLoginOpen(false)}
            >
              <XIcon size={20} />
            </Button>
          </div>
          <Dialog.Description>
            还没有账号？在官方 QQ 群向机器人发送「注册 &lt;用户名&gt;」即可直接开户并获取初始密码。
          </Dialog.Description>
          <form
            className="form-stack"
            onSubmit={(e) => {
              e.preventDefault();
              void login();
            }}
          >
            <Input
              label="账号"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
            <Input
              label="密码"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
            <Checkbox
              label="记住登录状态（30 天）"
              checked={remember}
              onCheckedChange={setRemember}
            />
            {error && (
              <Banner variant="error" role="alert" description={error} />
            )}
            <Button
              type="submit"
              variant="primary"
              loading={busy}
              className="w-full justify-center"
            >
              登录
            </Button>
            <LinkButton
              variant="ghost"
              href="https://qm.qq.com/q/93NWF3S362"
              target="_blank"
            >
              加入官方交流群（1126124220）
            </LinkButton>
          </form>
        </Dialog>
      </Dialog.Root>
      <Dialog.Root open={forced} disablePointerDismissal>
        <Dialog size="lg" className="dialog-content">
          <Dialog.Title>设置你的交易密码</Dialog.Title>
          <Dialog.Description>
            首次登录或密码刚刚被重置，为了账户安全，请先设置一个专属的新密码。
          </Dialog.Description>
          <form
            className="form-stack"
            onSubmit={(e) => {
              e.preventDefault();
              void change();
            }}
          >
            <Input
              label="当前临时密码"
              type="password"
              autoComplete="current-password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
              required
            />
            <Input
              label="新密码"
              type="password"
              minLength={12}
              maxLength={72}
              autoComplete="new-password"
              description="密码长度 12–72 位，需同时包含大写字母、小写字母、数字和符号。"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
            <Input
              label="再次输入新密码"
              type="password"
              autoComplete="new-password"
              value={repeat}
              onChange={(e) => setRepeat(e.target.value)}
              required
            />
            {error && (
              <Banner variant="error" role="alert" description={error} />
            )}
            <Button type="submit" variant="primary" loading={busy}>
              设置密码并继续
            </Button>
            <Button variant="ghost" onClick={() => void x.logout()}>
              退出登录
            </Button>
          </form>
        </Dialog>
      </Dialog.Root>
      <Dialog.Root
        open={binding}
        onOpenChange={(open) => {
          if (x.account?.boundStockId) x.setBindingOpen(open);
        }}
        disablePointerDismissal={!x.account?.boundStockId}
      >
        <Dialog size="lg" className="dialog-content">
          <Dialog.Title>关联你的实例股票</Dialog.Title>
          <Dialog.Description>
            每个交易账户需要绑定一只专属的 AzurPilot 实例股票，绑定后即可解锁全部交易功能。
          </Dialog.Description>
          {x.account?.boundStockId ? (
            <>
              <Banner
                variant="secondary"
                description={`已成功绑定股票：${x.market?.stocks.find((s) => s.id === x.account?.boundStockId)?.name ?? x.account.boundStockId}`}
              />
              <p>绑定关系长期有效，退出登录不会自动解绑；如需更换绑定，请联系管理员处理。</p>
              <Button onClick={() => x.setBindingOpen(false)}>完成</Button>
            </>
          ) : (
            <>
              <ol className="instructions">
                <li>前往你的 AzurPilot 实例后台完成股票注册；</li>
                <li>在股票详情中复制系统生成的专属绑定码；</li>
                <li>回到官方 QQ 群向机器人发送「绑定股票 &lt;绑定码&gt;」即可完成绑定。</li>
              </ol>
              <Button variant="primary" onClick={() => void x.refresh()}>
                检查绑定状态
              </Button>
              <Button variant="ghost" onClick={() => void x.logout()}>
                退出登录
              </Button>
            </>
          )}
        </Dialog>
      </Dialog.Root>
    </>
  );
}
export function AccountRequired() {
  const x = useExchange();
  return (
    <Empty
      icon={<LockKeyIcon size={28} />}
      title="请先登录交易账户"
      description="登录并绑定属于你的 AzurPilot 股票后，即可查看资产净值、持仓明细与历史委托。"
      contents={
        <Button
          variant="primary"
          onClick={() =>
            x.account ? x.setBindingOpen(true) : x.setLoginOpen(true)
          }
        >
          {x.account ? "前往绑定股票" : "登录交易账户"}
        </Button>
      }
    />
  );
}

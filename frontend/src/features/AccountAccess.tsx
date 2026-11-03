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
      setError("两次新密码不一致");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await request("/auth/password", { currentPassword: current, password });
      await x.refresh();
      notice("密码已更新");
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
            在 QQ 群向机器人发送“注册 &lt;用户名&gt;”获得账户。
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
              label="记住我（30 天）"
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
              加入 QQ 群 1126124220
            </LinkButton>
          </form>
        </Dialog>
      </Dialog.Root>
      <Dialog.Root open={forced} disablePointerDismissal>
        <Dialog size="lg" className="dialog-content">
          <Dialog.Title>设置你的交易密码</Dialog.Title>
          <Dialog.Description>
            初始密码和重置后的密码仅用于首次登录，请先设置自己的密码。
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
              description="12–72 字节，包含大写、小写、数字和符号。"
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
            交易账户与 AzurPilot 股票一对一关联。
          </Dialog.Description>
          {x.account?.boundStockId ? (
            <>
              <Banner
                variant="secondary"
                description={`当前绑定：${x.market?.stocks.find((s) => s.id === x.account?.boundStockId)?.name ?? x.account.boundStockId}`}
              />
              <p>退出登录不会解除绑定。只有管理员删除股票后才能重新绑定。</p>
              <Button onClick={() => x.setBindingOpen(false)}>完成</Button>
            </>
          ) : (
            <>
              <ol className="instructions">
                <li>在 AzurPilot 中注册实例股票。</li>
                <li>复制该实例的股票绑定码。</li>
                <li>在原 QQ 群发送“绑定股票 &lt;绑定码&gt;”。</li>
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
      title="你的交易空间"
      description="登录并绑定实例股票后，查看资产、持仓和委托。"
      contents={
        <Button
          variant="primary"
          onClick={() =>
            x.account ? x.setBindingOpen(true) : x.setLoginOpen(true)
          }
        >
          {x.account ? "完成账户设置" : "登录交易账户"}
        </Button>
      }
    />
  );
}

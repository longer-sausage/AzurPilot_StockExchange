"use client";
import { Banner, Button, Checkbox, Dialog, Input } from "@cloudflare/kumo";
import { XIcon } from "@phosphor-icons/react";
import { useEffect, useState, type FormEvent } from "react";
import { api } from "./api";
import type { Account, Stock } from "./types";
export function Login({
  open,
  onClose,
  onDone,
}: {
  open: boolean;
  onClose: () => void;
  onDone: (a: Account) => void;
}) {
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [remember, setRemember] = useState(false),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) {
      setPassword("");
      setError("");
    }
  }, [open]);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const r = await api<{ account: Account; forcePasswordChange: boolean }>(
        "/auth/login",
        { username, password, remember },
      );
      onDone({ ...r.account, forcePasswordChange: r.forcePasswordChange });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(open) => {
        if (!open) (busy ? undefined : onClose)?.();
      }}
    >
      <Dialog size="lg" className="exchange-dialog">
        <div className="dialog-heading">
          <Dialog.Title>登录交易账户</Dialog.Title>
          <Button
            variant="ghost"
            shape="square"
            aria-label="关闭"
            disabled={!(busy ? undefined : onClose)}
            onClick={() => (busy ? undefined : onClose)?.()}
          >
            <XIcon size={20} />
          </Button>
        </div>
        <div className="dialog-body">
          <p className="muted">
            在 QQ 群向官方机器人发送“注册
            &lt;用户名&gt;”获得账号与随机初始密码。
          </p>
          <form className="form" onSubmit={submit}>
            <label>
              账号
              <Input
                required
                minLength={2}
                maxLength={32}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
              />
            </label>
            <label>
              密码
              <Input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
              />
            </label>
            <label className="check">
              <Checkbox
                checked={remember}
                onCheckedChange={(value) => setRemember(value)}
              />
              记住我（30 天）
            </label>
            {error && (
              <Banner variant="error" role="alert">
                {error}
              </Banner>
            )}
            <Button
              type="submit"
              variant="primary"
              className="primary wide"
              disabled={busy}
            >
              {busy ? "处理中…" : "登录"}
            </Button>
            <p className="small muted">
              首次登录与密码重置后必须改密，并完成股票绑定，才可使用账户。忘记密码请在群内发送“重置密码”。
            </p>
            <a
              href="https://qm.qq.com/q/93NWF3S362"
              target="_blank"
              rel="noreferrer"
            >
              加入 QQ 群 1126124220
            </a>
          </form>
        </div>
      </Dialog>
    </Dialog.Root>
  );
}
export function PasswordChange({
  open,
  onDone,
  onLogout,
}: {
  open: boolean;
  onDone: () => Promise<void>;
  onLogout: () => void;
}) {
  const [currentPassword, setCurrent] = useState(""),
    [password, setPassword] = useState(""),
    [repeat, setRepeat] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <Dialog.Root open={open} disablePointerDismissal>
      <Dialog size="lg" className="exchange-dialog">
        <div className="dialog-heading">
          <Dialog.Title>请先设置新密码</Dialog.Title>
        </div>
        <div className="dialog-body">
          <p className="muted">
            请将机器人发放的初始密码改为自己的密码。完成后继续绑定股票。
          </p>
          <form
            className="form"
            onSubmit={async (e) => {
              e.preventDefault();
              if (password !== repeat) {
                setError("两次新密码不一致");
                return;
              }
              setBusy(true);
              setError("");
              try {
                await api("/auth/password", { currentPassword, password });
                await onDone();
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            <label>
              当前临时密码
              <Input
                type="password"
                autoComplete="current-password"
                required
                value={currentPassword}
                onChange={(e) => setCurrent(e.target.value)}
              />
            </label>
            <label>
              新密码
              <Input
                type="password"
                minLength={12}
                maxLength={72}
                required
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
            <small className="muted">
              至少 12
              个字符，包含大写字母、小写字母、数字与符号，且不同于临时密码。
            </small>
            <label>
              再次输入新密码
              <Input
                type="password"
                required
                autoComplete="new-password"
                value={repeat}
                onChange={(e) => setRepeat(e.target.value)}
              />
            </label>
            {error && (
              <Banner variant="error" role="alert">
                {error}
              </Banner>
            )}
            <Button
              type="submit"
              variant="primary"
              className="primary"
              disabled={busy}
            >
              {busy ? "保存中…" : "设置新密码并继续"}
            </Button>
            <Button type="button" disabled={busy} onClick={onLogout}>
              退出登录
            </Button>
          </form>
        </div>
      </Dialog>
    </Dialog.Root>
  );
}
export function Binding({
  open,
  account,
  stocks,
  onClose,
  onRefresh,
  onLogout,
}: {
  open: boolean;
  account?: Account;
  stocks: Stock[];
  onClose?: () => void;
  onRefresh: () => void;
  onLogout: () => void;
}) {
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(open) => {
        if (!open) onClose?.();
      }}
    >
      <Dialog size="lg" className="exchange-dialog">
        <div className="dialog-heading">
          <Dialog.Title>股票账户绑定</Dialog.Title>
          <Button
            variant="ghost"
            shape="square"
            aria-label="关闭"
            disabled={!onClose}
            onClick={() => onClose?.()}
          >
            <XIcon size={20} />
          </Button>
        </div>
        <div className="dialog-body">
          {account?.boundStockId ? (
            <>
              <p>
                已绑定{" "}
                {stocks.find((s) => s.id === account?.boundStockId)?.name ??
                  account?.boundStockId}
              </p>
              <p className="muted">
                股票上报独立运行，退出不会解除绑定。只有管理员删除股票后才需要重新绑定。
              </p>
            </>
          ) : (
            <>
              <p role="status">
                账户尚未绑定股票，暂不可使用。股票被删除后，也需重新完成以下步骤。
              </p>
              <ol className="steps">
                <li>在 AzurPilot 当前实例的“茗喵证券交易所”注册股票。</li>
                <li>复制该实例的绑定命令。</li>
                <li>
                  在 QQ 群向官方机器人发送 <code>绑定股票 &lt;绑定码&gt;</code>
                  。
                </li>
                <li>绑定完成后点击检查，或等待行情通知自动更新。</li>
              </ol>
              <Button variant="primary" className="primary" onClick={onRefresh}>
                检查绑定状态
              </Button>
              <Button onClick={onLogout}>退出登录</Button>
            </>
          )}
          <a
            href="https://qm.qq.com/q/93NWF3S362"
            target="_blank"
            rel="noreferrer"
          >
            加入 QQ 群 1126124220
          </a>
        </div>
      </Dialog>
    </Dialog.Root>
  );
}

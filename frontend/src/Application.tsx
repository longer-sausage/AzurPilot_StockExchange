"use client";
import { useRoute } from "./core/router";
import { Component, type ReactNode } from "react";
import { Badge, Banner, Button, LinkButton, Tabs } from "@cloudflare/kumo";
import {
  MoonIcon,
  SunIcon,
  ArrowClockwiseIcon,
  SignOutIcon,
} from "@phosphor-icons/react";
import { Exchange, useExchange, useNotice } from "./core/Exchange";
import { AccountAccess } from "./features/AccountAccess";
import { MarketPage } from "./screens/MarketPage";
import { SecurityPage } from "./screens/SecurityPage";
import { PortfolioPage } from "./screens/PortfolioPage";
import { OrdersPage } from "./screens/OrdersPage";
import { SeasonsPage } from "./screens/SeasonsPage";
import { GuidePage } from "./screens/GuidePage";
import { AdminPage } from "./screens/AdminPage";
const links = [
  ["/", "证券市场"],
  ["/portfolio", "我的持仓"],
  ["/orders", "委托记录"],
  ["/seasons", "月赛排名"],
  ["/guide", "交易指南"],
];
class Recovery extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? (
      <main className="page">
        <Banner
          variant="error"
          title="页面遇到了一点问题"
          description="别担心，已确认提交的委托不会重复发送。请刷新页面重试。"
        />
        <Button onClick={() => location.reload()}>刷新页面</Button>
      </main>
    ) : (
      this.props.children
    );
  }
}
function Workspace() {
  const route = useRoute();
  const x = useExchange(),
    notice = useNotice(),
    path = route.path,
    admin = path === "/console";
  const current = path === "/security" ? "/" : path;
  const titles: Record<string, string> = {
    "/": "证券市场",
    "/security": "证券详情",
    "/portfolio": "我的持仓",
    "/orders": "委托记录",
    "/seasons": "月赛排名",
    "/guide": "交易指南",
    "/console": "管理控制台",
  };
  return (
    <>
      <header className="site-header">
        <div className="header-inner">
          <a
            className="wordmark"
            href="/"
            onClick={(e) => {
              e.preventDefault();
              route.navigate("/");
            }}
          >
            <img
              src="/favicon.webp"
              alt="茗喵"
              width={30}
              height={30}
              className="wordmark-icon"
            />
            <span>
              茗喵<span className="wordmark-secondary">证券交易所</span>
            </span>
          </a>
          <div className="header-actions">
            <Badge variant="secondary">模拟证券游戏</Badge>
            <Button
              variant="ghost"
              shape="square"
              aria-label={x.theme === "light" ? "切换暗色主题" : "切换亮色主题"}
              onClick={x.toggleTheme}
            >
              {x.theme === "light" ? (
                <MoonIcon size={18} />
              ) : (
                <SunIcon size={18} />
              )}
            </Button>
            {!admin &&
              (x.account ? (
                <>
                  <Button
                    variant="ghost"
                    onClick={() => x.setBindingOpen(true)}
                  >
                    {x.account.username}
                  </Button>
                  <Button
                    variant="ghost"
                    shape="square"
                    aria-label="退出登录"
                    onClick={() =>
                      void x.logout().catch((e) => notice(e.message, true))
                    }
                  >
                    <SignOutIcon size={18} />
                  </Button>
                </>
              ) : (
                <Button
                  variant={path === "/security" ? "secondary" : "primary"}
                  onClick={() => x.setLoginOpen(true)}
                >
                  登录交易账户
                </Button>
              ))}
            {admin ? (
              <LinkButton
                href="/"
                onClick={(e) => {
                  e.preventDefault();
                  route.navigate("/");
                }}
                variant="secondary"
              >
                返回交易站
              </LinkButton>
            ) : (
              <LinkButton
                href="/"
                onClick={(e) => {
                  e.preventDefault();
                  route.navigate("/console");
                }}
                variant="ghost"
              >
                管理
              </LinkButton>
            )}
          </div>
        </div>
        {!admin && (
          <div className="navigation">
            <Tabs
              variant="underline"
              value={current}
              onValueChange={route.navigate}
              tabs={links.map(([href, label]) => ({
                value: href,
                label,
              }))}
            />
            <span className="connection-label">
              {x.connected ? "行情已实时连接" : "网络波动，行情重连中..."}
            </span>
          </div>
        )}
      </header>
      <main
        className={`page ${admin ? "admin-page" : ""}`}
        aria-label={titles[path] ?? "证券市场"}
      >
        {x.error && (
          <Banner
            variant="error"
            title="行情数据未能及时同步"
            description={x.error}
            action={<Button onClick={() => void x.refresh()}>点击重试</Button>}
          />
        )}
        <Recovery>
          {path === "/security" ? (
            <SecurityPage />
          ) : path === "/portfolio" ? (
            <PortfolioPage />
          ) : path === "/orders" ? (
            <OrdersPage />
          ) : path === "/seasons" ? (
            <SeasonsPage />
          ) : path === "/guide" ? (
            <GuidePage />
          ) : admin ? (
            <AdminPage />
          ) : (
            <MarketPage />
          )}
        </Recovery>
      </main>
      <footer className="site-footer">
        <span>茗喵证券交易所 · 模拟游戏，不涉及真实资金</span>
        <div>
          <Button variant="ghost" size="sm" onClick={() => void x.refresh()}>
            <ArrowClockwiseIcon size={14} />
            刷新数据
          </Button>
          <LinkButton
            size="sm"
            variant="ghost"
            href="https://qm.qq.com/q/93NWF3S362"
            target="_blank"
          >
            帮助与反馈
          </LinkButton>
        </div>
      </footer>
      {!admin && <AccountAccess />}
    </>
  );
}
export default function Application() {
  return (
    <Exchange>
      <Workspace />
    </Exchange>
  );
}

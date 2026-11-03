# 交易站与管理控制台设计约定

本次前端迁移采用 Next.js App Router、React、TypeScript、Tailwind CSS 4、Cloudflare Kumo、Phosphor Icons 与 ECharts。具体版本固定在 `package.json` 与 `package-lock.json`，安装使用 `npm ci --prefix frontend`。

## 指定 Skill 与设计来源

- [Refactoring UI Skill](https://github.com/edisonmbli/refactoring-ui-skill/blob/main/skills/refactoring-ui/SKILL.md)：先识别框架、主题与令牌，再执行构建流程 B，最后按层级、布局、排版、色彩、深度、图像、收尾的顺序检查。使用其组件库适配、组件配方和审查规范。
- [awesome-design-md 的 Linear 设计分析](https://github.com/VoltAgent/awesome-design-md/blob/main/design-md/linear.app/DESIGN.md)：采用工作台的信息密度、中性色表面、细分隔和克制强调，未复制品牌、营销页面或自制视觉控件。参考版本 `fcc70284932276d085f01ef9f83edde90b1258cb`。
- [Kumo 官方设计 Skill](https://github.com/cloudflare/kumo/blob/main/skills/kumo-design/SKILL.md)：以 Kumo 的语义颜色、字号、组件、明暗模式与交互为准。官方仓库参考版本 `a39b1a293a0d7bcd8800c81e3ffda79c4bdf8ef9`。

## 内容和操作层级

主要内容是最新报价、账户资产和待提交委托。库存、成交、持仓成本为次要内容。时间、来源和协议说明作为辅助内容。

交易预览是交易区的主操作，确认弹窗内以提交为主操作。导航、筛选、图表指标、取消和帮助使用 Kumo 的 secondary 或 ghost 变体；删除操作进入明确的确认流程。颜色之外保留数值符号、状态文字和按钮名称。

正文和交互以 Kumo 的 14px 体系为准；标题使用 18/24/28px，主要报价使用 36px。组内使用 8/12px，组间使用 16/24/32px。数字使用等宽数字特性。表格在窄屏内容器横向滚动，页面本身不得横向溢出。

## 组件边界

直接导入 Kumo 的 Button、Input、Textarea、Checkbox、Select、Dialog、LayerCard、Table、Banner、Badge、Meter、Collapsible、Toasty。删除旧 `src/ui.tsx`，不保留自制基础控件包装层。业务页面仅组合官方控件和处理 v2 请求。

图标全部来自 `@phosphor-icons/react`；原手绘 SVG 趋势线改为官方趋势图标和 Badge。价格走势图由 ECharts Canvas 绘制，保留分时、日 K、分钟 K、技术指标、缩放、平移和全屏。不得以自画 SVG 替代组件库图标。

布局样式集中在 `app/globals.css`，颜色引用 Kumo 语义令牌；不定义第二套基础控件样式。明暗主题仍兼容原 `mmex-theme` 偏好，同时驱动 Kumo 的 `data-mode`。

## 构建、路由与接口兼容

Next.js 以 `output: export` 导出，`scripts/export.mjs` 将结果复制到 `dist/`，保持 Go 当前挂载目录。`/console` 的 Go HTML fallback 在客户端识别实际地址；Next.js 同时生成独立管理页面。跨入口保留完整页面导航，避免把静态 Go 服务当成 Next.js Node 服务。

全部 27 项前端 v2 端点的方法和路径不变；金额仍使用整数分、费率仍使用 ppm。保留 ETag 缓存、同源 Cookie、玩家和管理员会话隔离、请求期间 SSE 通知合并、重连及重新可见时刷新。已有密码、会话、账户、股票、历史、数据库和 AzurPilot 实例签名均不迁移。

兼容性结论相对于本次改动前仓库版本：新前端可配合原 v2 后端，原前端也不因本次变更失去兼容性。未核实线上版本，v1 仍不兼容。

## 验证与发布范围

生产构建执行 `npm run build --prefix frontend`。本地假数据 UI 测试执行 `npm run test:ui --prefix frontend`，产物截图位于忽略的 `test-results/`，覆盖 1280px 桌面和 375px 手机以及登录、交易预览与确认、图表、主题、管理配置、账户、股票操作。仅在本地运行夹具，不启用 Docker、不访问线上 API。

真实 PostgreSQL 全链路用例仍保留在 `e2e/`，从当前 Go 源码启动；只有提供专用回环 `EXCHANGE_E2E_DATABASE_URL` 时才可执行。未配置数据库时，假数据用例通过不能表述为真实数据库或线上验收通过。

| 范围 | 结论与生效步骤 |
| --- | --- |
| 必须更新 | 交易站与管理前端：构建并部署整个 `dist/`，含 `/_next/`，最后切换 `index.html` |
| 可选更新 | 无 |
| 不需要更新 | Go 后端、AzurPilot 前后端、QQ 插件、数据库与会话；静态挂载不变时无需重启 Go |
| 待确认 | 线上尚未部署；需单独发布静态目录后验证公网页面与资源 |

本次不改 nginx、证书、Compose 或生产数据。回滚只需恢复完整旧静态产物，保留新旧版本资产直至切换完成，无需数据库恢复。

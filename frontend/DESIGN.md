# 交易站与管理控制台重建设计约定

本轮删除原 `src/` 业务源码和原 Next.js 页面，重新建立 `core/`、`features/`、`screens/`。构建入口仅为 `app/page.tsx`，由它加载整个 React 应用。技术栈为 Next.js、React、TypeScript、Tailwind CSS 4、Cloudflare Kumo、Phosphor Icons 和 ECharts，版本固定在依赖声明与锁文件中。

## 指定 Skill 与设计依据

- [Refactoring UI Skill](https://github.com/edisonmbli/refactoring-ui-skill/blob/main/skills/refactoring-ui/SKILL.md)：先确认已有组件库的主题契约，再按内容层级、布局、字体、颜色、深度和交互检查；使用组件配方、组件库适配和审查规范。先实现内容，再由截图验证布局。
- [awesome-design-md 中的 Linear 分析](https://github.com/VoltAgent/awesome-design-md/blob/main/design-md/linear.app/DESIGN.md)：取其克制的中性色、清晰分隔、工作台信息密度与文字层级。参考版本 `fcc70284932276d085f01ef9f83edde90b1258cb`，不复制品牌或营销页面。
- [Kumo 官方设计 Skill](https://github.com/cloudflare/kumo/blob/main/skills/kumo-design/SKILL.md)：组件的原生结构、变体、尺寸、状态和主题优先。参考版本 `a39b1a293a0d7bcd8800c81e3ffda79c4bdf8ef9`。

## 页面结构与视觉层级

首页主体为完整宽度的证券表格：名称与价格优先，涨跌、库存和小时量其次，代码与行情更新时间最后。筛选与排序位于表格上方，表头只表达列名；自选使用原生方形图标按钮，不把排序按钮或切换控件塞进表头。首页没有旧的狭窄证券列表、交易面板和辅助栏三列结构。

行情行的普通单元格均可点击进入证券详情，名称链接保留键盘入口，自选与重试按钮独立响应。最新价下方使用 Kumo Chart 显示真实分钟收盘价，在手机初始视口内也能看到。数据复用已有 `GET /api/v2/public/stocks/{id}/chart?from=...&to=...&interval=1`，按该股票最近报价所在的上海日期取一天，不附加当前价格端点，不根据涨跌幅制造曲线。保留真实时间间隔，缺失分钟断开连线；无历史显示暂无行情，单点只画一个点，请求失败可单独重试。

小图仅在接近可视区域时请求，缓存按股票、日期、历史版本和最后观测时间复用。历史修正或新报价随市场 SSE 刷新失效；筛选、排序、主题或返回市场不重复拉取相同版本。图形由 ECharts Canvas 绘制，没有新增 SVG 或基础控件。

导航在顶部，直接由 Kumo Tabs 的受控值和 `onValueChange` 切换视图，保留原生方向键操作与选中指示器。详情页把走势图与交易单放在同一工作区，下方展示流动性、逐笔记录和证券资料。资产、委托、月赛、指南及管理为独立 React 视图。交易预览和确认是主要操作；筛选、指标和帮助使用次要或 ghost 变体。危险操作具有单独确认。

正文 14px，辅助文字 12px，标题 20/28/32px，重点价格 32/40px；数字使用等宽数字特性。间距采用 8/12/16/20/24/32/40/64px 的有限集合。页面最大宽度 1280px，窄屏将业务列堆叠，长表格仅在自身容器内滚动。

## 组件与主题边界

基础控件直接使用 Kumo 的 Button、LinkButton、Input、InputGroup、Checkbox、Select、Tabs、Dialog、Popover、LayerCard、Table、Banner、Badge、Toolbar、Chart、Meter、ClipboardText、Collapsible、Toasty 等。业务组件只组合这些控件和处理 v2 数据，不再提供自制基础控件包装层。

CSS 只负责页面布局、内容排版和语义涨跌色；不通过后代选择器修改 Kumo 的按钮、输入框或表格单元格尺寸。颜色引用 Kumo 语义变量，明暗主题使用 `data-mode`，保留已有 `mmex-theme` 偏好。图标全部来自 `@phosphor-icons/react`，没有手绘 SVG 图标。

价格图使用 Kumo Chart 注入 ECharts Canvas，组件管理初始化、主题、缩放和释放。周期、指标、全屏使用原生 Tabs、Popover、Checkbox、Toolbar 和 Dialog。保留分时、分钟 K、日 K、成交量及 MA、EXPMA、BBI、BOLL、ENE、MACD；指标计算保留预热数据，视口按接口 `displayFrom` 开始，样本不足不伪造数值。

## 固定根地址的单页应用

仅导出一个业务 `index.html`。所有站内视图均保持 `/`，React 外部状态保存当前视图，`history.state.exchangeView` 支持浏览器前进、后退与刷新。站内链接阻止文档导航，不产生 `/security`、`/portfolio` 或 `/console` 页面请求。

外部 `/?stock=ID`、旧 `/#/...` 链接及既有 Go fallback 的 `/console` 在客户端导入后归一到 `/`。复制根地址只分享首页；需要分享具体证券时使用旧查询参数入口。管理仍由独立管理员 Cookie 保护，隐藏地址不承担权限控制。

Next.js 使用 `output: export`，将 `out/` 完整复制为 `dist/`。生产仅提供 `/`、`/_next/` 等静态资源与同源 `/api/v2`，无需 Next.js Node 服务或 Worker 页面路由。Next.js 自带的 404 产物不属于业务页面。

## 契约与兼容性

保留原有前端 v2 API 的路径、方法、请求字段及单位；列表波动预览新增使用后端已有的轻量 `/chart` 读取端点，无需修改后端。金额和价格仍为整数分，费率 ppm，行情时间毫秒、会话时间秒。保留同源 HttpOnly Cookie、独立玩家与管理员会话、ETag/304、SSE 通知合并、重连和重新可见刷新。

相对于本轮改动前仓库版本，新前端可配合旧 v2 后端；后端未变，旧交易站、旧管理站与旧 AzurPilot 同步端的兼容关系不变。未核实线上版本，v1 仍不兼容。数据库、密码、已有会话、股票、历史、实例签名和机器人协议均不迁移；没有数据库发布顺序要求。

## 验证与已修复的视觉问题

- `npm run build --prefix frontend`：生产导出与严格 TypeScript 检查。
- `npm run test:ui --prefix frontend`：使用生产产物和独立本地夹具，覆盖固定根地址、单次文档加载、前进后退刷新、旧链接导入、筛选排序、自选、登录改密、交易预览确认、图表、主题、资产、撤单、月赛、指南、管理设置、账户和证券管理。
- 截图检查桌面与 375px 手机：完整表格的水平滚动局限在容器内，页面不横向溢出；控件尺寸保持 Kumo 原值。截图产物在忽略的 `test-results/`。
- 截图发现表格脚注的类名与 Tailwind `table-caption` 工具类重名，导致脚注变成狭长竖排；已改为独立 `.data-note` 类。
- 旧哈希链接在已挂载应用内切换时需监听 `hashchange`，否则不更新视图；已修复并在布局用例中覆盖。
- 行情预览回归验证整行进入详情、自选独立操作、ETag 与相同版本缓存、无数据/单点/失败重试和手机可视范围；使用端点相同、中段顺序不同的两组报价验证 Canvas 确实随历史数据更新，不能只根据涨跌方向绘制固定曲线。假数据轻量图表与详情共用同一组隔离历史。
- 长短视图切换时，传统滚动条出现或消失会改变可用宽度 15px，1440px 居中布局左右移动 7.5px。根元素使用 `scrollbar-gutter: stable` 保留占位，不修改 Kumo 控件。回归测试显式移除 Playwright 默认的 `--hide-scrollbars` 参数，在 1280/1440/375px 下逐帧检查导航位置、宽度与页面滚动，避免无滚动条的测试环境漏检。

真实 PostgreSQL 全链路测试在 `e2e/spa-integration.spec.ts`，需专用回环 `EXCHANGE_E2E_DATABASE_URL`。本轮未配置该数据库，未执行真实数据库验证。假数据用例通过不代表线上账本验收。预览仅监听本机回环，未启动 Docker，未调用线上 API，夹具不进入生产产物。

## 发布范围与回滚

| 范围 | 原因与生效步骤 |
| --- | --- |
| 必须更新 | 交易站与管理前端：构建并发布整个 `frontend/dist/`，含 `/_next/`，最后切换 `index.html` |
| 可选更新 | 无需配套升级其他组件 |
| 不需要更新 | Go 后端、数据库、AzurPilot 前后端、QQ 插件；挂载不变通常无需重启 Go |
| 待确认 | 本轮未部署线上，生产静态产物发布尚待安排 |

没有修改 nginx、证书、Compose 或生产数据。回滚时恢复完整旧静态产物，保留切换期间仍被引用的资产；无需恢复数据库或撤销已有会话。

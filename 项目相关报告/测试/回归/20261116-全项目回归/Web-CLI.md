# <span id="前言">Web／CLI 当前源码回归</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、范围与结论 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

核对现行[验收表](../../../../需求/IM功能验收表.md/IM功能验收表.md)、[五端资产表](../../../资产盘点/README.md)、两端源码及各自测试入口。CLI完整本地回归重新执行97 PASS／0 FAIL／2环境SKIP；Web各本地专项、实际浏览器App演示、类型检查及独立生产构建通过。局部组数不相加为需求完成数，所有真实HTTPS／目标OS／断电验证仍按下文区分。

发现并修复一项真实界面缺陷：浏览器没有所选Intl语言时，导出日期静默借用宿主语言。本机Chrome的`bo`不受支持，原藏文页面显示中文年月日；现在明确检查语言支持，无相应数据时显示本地时区的`YYYY-MM-DD HH:mm`。保持请求、身份、时区和持久化行为不变。另补齐Web测试总表的两个入口及CLI运行器过时的overlay清单。

仅修改Jobs自有Web组件、对应测试和说明。未修改CLI产品Go源码、依赖锁、第三方、真实用户库或全局环境；未提交／推送。未连接实际API／PG，不启停用户部署；私有测试Vite不读取工程代理配置。浏览器使用独立临时上下文。

## 二、源码实现、已验与待核 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 链路 | 源码已实现／入口 | 本轮实际已验 | 仍待核／缺口 |
| --- | --- | --- | --- |
| 注册／登录／会话刷新 | Web [App](../../../../代码/前端/Web/Src/App.tsx)、[认证协调](../../../../代码/前端/Web/Src/AuthSession.ts)；CLI [auth.go](../../../../代码/前端/CLI/cmd/jobsim/auth.go) | Web模拟BFF16组；CLI原nonce、未知退休链、并发刷新、身份切换及安全库失败反例纳完整97组 | 当前候选真实BFF／Cookie／CA及CLI实际TTY、系统安全库正向；两个显式环境SKIP保持未验 |
| 发送／会话／历史 | Web严格IDB原Outbox；CLI [store.go](../../../../代码/前端/CLI/cmd/jobsim/store.go)与[main.go](../../../../代码/前端/CLI/cmd/jobsim/main.go) | Web演示发送、旧view保护、原完整body／路由／sender的只读proof；CLI真实SQLite先提交后HTTP、丢响应重开、原准入保留／冻结 | 当前候选五端真实互通、移动浏览器矩阵、实服务器媒体与权限／故障窗口；演示发送不当真实消息 |
| 权威快照／增量 | Web [SyncEngine](../../../../代码/前端/Web/Src/SyncEngine.ts)／[SyncStore](../../../../代码/前端/Web/Src/SyncStore.ts)；CLI [sync_engine.go](../../../../代码/前端/CLI/cmd/jobsim/sync_engine.go) | 原始字节摘要、连续位置、缺页／未知、IDB／SQLite回滚、global终态、跨持有者清理、晚到媒体与原队列CAS | 新候选真实HTTPS／NATS；合格ACK、目标累计范围及after_receipt不可由同步游标推定 |
| 本人状态／草稿 | Web [SelfStateActions](../../../../代码/前端/Web/Src/SelfStateActions.ts)／[SelfDraftRecovery](../../../../代码/前端/Web/Src/SelfDraftRecovery.tsx)；CLI [self_state.go](../../../../代码/前端/CLI/cmd/jobsim/self_state.go) | Web本人状态26组、独立恢复10组；CLI原操作／CAS／保本机输入纳完整本地集 | 当前候选真实离群／只读草稿恢复授权、OS剪贴板与跨端真实一致性 |
| 正式本人导出 | Web设置 [DataExportPanel](../../../../代码/前端/Web/Src/DataExportPanel.tsx)；CLI [data_export.go](../../../../代码/前端/CLI/cmd/jobsim/data_export.go)与[file](../../../../代码/前端/CLI/cmd/jobsim/data_export_file.go) | Web最终19组真实组件／IDB／WebLocks、ZIP反例、原创建／取消与身份围栏；CLI实际SQLite／本地HTTP、ZIP校验与不覆盖发布 | 真实导出API／worker／签名源证明、浏览器用户保存及CLI目标OS；RAM票不进入URL或持久库的既定边界保持 |
| 本人主动改密 | CLI [password_change.go](../../../../代码/前端/CLI/cmd/jobsim/password_change.go)四命令已接主路由 | 非秘密原责任、原family显式首写、匿名纯查、OTP等待后身份／cap复核及后继family保护纳完整97组 | **Web入口仍未接线**，登录／恢复表单不能替代；CLI真实TTY／管理员TOTP／HTTPS／OS库与后端撤全family尚待独立验证 |
| 正文／对象可靠收讫 | CLI [receipt.go](../../../../代码/前端/CLI/cmd/jobsim/receipt.go)、[receipt_objects.go](../../../../代码/前端/CLI/cmd/jobsim/receipt_objects.go) | 本地真实文件／fsync、SQL／子进程崩溃反例纳完整集 | 本轮无真实批准API／签名／断电；Web保持transport-only；冻结targets／累计收讫／云删除不能宣称完成 |

现行完整I、注销／普通迁移、录音录像等尚未交付内容保持原要求，本报告不以通用附件或本地测试补齐缺失能力。

## 三、实际执行记录 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 专项 | 结果 | 原始记录 |
| --- | --- | --- |
| Web TypeScript | 初次及日期修复后均退出0 | [最终类型／构建与输入SHA](./Web-CLI/修复后生产构建.json) |
| Web同步协议 | 12组，通过 | [协议结果](./Web-CLI/协议/协议结果.json) |
| Web认证 | 16组，通过，模拟BFF | [认证结果](./Web-CLI/auth-session/浏览器协议结果.json) |
| Web同步持久化 | 12组，通过 | [存储结果](./Web-CLI/sync-store/结果.json) |
| Web同步投影 | 16组，通过 | [投影原件](../../前端/Web/执行记录/sync-view-1794841160200-69322/结果.json) |
| Web原完整请求proof | 10组，通过 | [原意图原件](../../前端/Web/执行记录/outbox-proof-1794841163895-69801/原请求队列结果.json) |
| Webglobal事实 | 6组，通过 | [global原件](../../前端/Web/执行记录/global-facts-1794841164538-70341/结果.json) |
| Web S2本人状态 | 26组，通过 | [本人状态](./Web-CLI/self-state/结果.json) |
| Web独立草稿恢复 | 10组，通过 | [独立恢复](./Web-CLI/draft-recovery/结果.json) |
| Web正式导出 | 原18组通过；新增日期反例修复前失败，最终19组通过 | [原18](./Web-CLI/data-export/结果.json)、[严格失败轮](./Web-CLI/导出日期-修复前-完整断言/结果.json)、[最终19](./Web-CLI/导出日期-修复后/结果.json) |
| Web App演示／外观／布局 | 8类检查通过，无页面JS异常 | [结果](../../前端/Web/执行记录/1794841423159-81821/browser-result.json)、[实际输出](./Web-CLI/browser执行器输出.log) |
| Web业务global | 9组，通过 | [业务global原件](../../前端/Web/执行记录/business-global-1794841429854-81906/业务终态结果.json) |
| CLI完整本地 | 97 PASS／0 FAIL／2 SKIP，Go退出0 | [运行器JSON](./Web-CLI/CLI本地/SQLite结果.json)、[原始日志](./Web-CLI/CLI本地/SQLite测试.log)、[99方法集合](./Web-CLI/CLI本地/顶层方法集合.json) |
| Web最终独立生产构建 | 退出0、构建期间源码未变化 | [最终构建记录](./Web-CLI/修复后生产构建.json)、[构建日志](./Web-CLI/修复后生产构建.log) |

CLI源码注册99个顶层方法，PASS／SKIP名称与源码集合完全相等，无多项或缺项。跳过的两个精确名称为`TestAuthCoordinatorActualHTTPSAndUnknownClosure`和`TestAuthNativeSystemVaultSeparateDiagnostic`，未提供对应环境，也未尝试读现有真实凭据。

源码产品改动仅导出日期组件；修复后重新验证该组件完整19组、十语三主题与窄屏、全工程类型检查及生产构建。其它组说明的是修复前相同的其它产品模块，不把它们声称为日期修订后的全量重跑。新增日期用例在失败／成功两轮字节相同，旧组件从Git对象取回后核对失败轮SHA才封存；[前](./Web-CLI/导出日期-修复前-完整断言/输入源码/代码/前端/Web/Src/DataExportPanel.tsx)与[后](./Web-CLI/导出日期-修复后/输入源码/代码/前端/Web/Src/DataExportPanel.tsx)均保留。初步`导出日期-修复前`轮的用例在执行期间补了非空断言，故正式反例使用独立的`修复前-完整断言`轮，不混作同输入证据。

已实看本轮App移动设置、英文深色独立恢复、藏文导出截图；[修复后的藏文截图](./Web-CLI/导出日期-修复后/Export-bo-system.png)日期为数字，本轮未进行屏幕阅读器、设备真机或完整视觉审查。

## 四、命令与环境 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

以下从项目根目录执行；工具复用本机已安装版本，未安装／升级任何依赖。实际Node24.22.1、Go1.27.4、Playwright1.65.1与现有Chrome。报告内的工具路径只作执行证据，不是产品运行依赖。

```sh
NODE="$HOME/Library/Application Support/JobsIM/Toolchains/node-v24.22.1-darwin-arm64/bin/node"
GO_BINARY="$HOME/Library/Application Support/JobsIM/Toolchains/go1.27.4/go/bin/go"
JOBSIM_PLAYWRIGHT_MODULE="$HOME/Library/Application Support/JobsIM/Toolchains/Playwright-1.65.1/node_modules/playwright"
"$NODE" 代码/前端/Web/node_modules/typescript/bin/tsc --noEmit -p 代码/前端/Web/tsconfig.json
JOBSIM_SYNC_REPORT="$PWD/项目相关报告/测试/回归/20261116-全项目回归/Web-CLI/协议" \
  "$NODE" 测试/前端/Web/sync-protocol.cjs/sync-protocol.cjs
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 GOFLAGS=-p=1 \
  GO_BINARY="$GO_BINARY" JOBSIM_CLI_TEST_TIMEOUT=120s \
  JOBSIM_CLI_REPORT_DIR="$PWD/项目相关报告/测试/回归/20261116-全项目回归/Web-CLI/CLI本地" \
  "$NODE" 测试/CLI/运行测试.mjs/运行测试.mjs
```

报告目录应换成新的未使用位置，不能照抄命令覆盖本轮证据。CLI执行环境明确移除了`JOBS_IM_CLI_AUTH_TEST_ENV`、`JOBS_IM_CLI_NATIVE_VAULT`及`JOBS_IM_CLI_AUTH_REPORT`，保持真实HTTPS／OS库诊断未运行。

浏览器执行的精确入口为`测试/前端/Web/{auth-session,sync-store,sync-view,outbox-proof,global-facts,self-state,draft-recovery,data-export,browser,business-global}.cjs/同名.cjs`，均由上述`$NODE`逐项运行，注入`JOBSIM_PLAYWRIGHT_MODULE`。前六项与App／业务global使用外层`createServer({root:web,configFile:false,publicDir:"Public",cacheDir:任务临时目录,server:{host:"127.0.0.1",port:0}})`，传实际回环URL到`JOBSIM_WEB_URL`；draft-recovery和data-export自行启动同样无代理的私有服务。API由专项明确注入／阻断；App演示只能得到本私有Vite的404，不能经开发代理触达真实服务。

各命令退出码、执行时刻、动态磁盘最小值、实际URL及stdout另见[浏览器清单](./Web-CLI/浏览器执行清单.json)、[App／业务global清单](./Web-CLI/UI与业务global执行清单.json)及各`监管.json`。外层每秒检查4GiB下限，并设每子进程180秒watchdog，只能结束本任务进程组；实际均正常完成，最小空闲约22GiB。构建在开始／结束核盘，实际少于1秒；未清理缓存／用户数据。

最终发行构建实际调用现有Vite的`build({root:web,configFile:web+"/vite.config.ts",cacheDir:独立临时目录,build:{outDir:独立临时目录+"/Production",emptyOutDir:false}})`，未覆盖原`Dist`。生成`index-B-U2cONr.js`，SHA256为`a78e21b9aa450f3a6c819cff0ee7899e741defaf5a18c0d7b9bb4eb86cd3878e`；产物的实际临时位置及其它摘要见构建JSON。生成包与现有运行服务无关联，未部署该包。

## 五、未消除的风险与交付边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- Web主动改密仍未接线；CLI已接源码与本地边界，真实改密链路仍待独立测试环境。未用恢复功能替代该要求。
- 正式导出、对象收讫与S2新切片的真实服务／目标系统验证缺口继续存在；历史真实HTTPS只证明其原包与原输入。
- 构建仍报告主JS超过500kB及lucide-react的`use client`指令告警；本轮保留真实输出，未以提高告警阈值、改第三方或无关拆包消除提示。未作弱网性能测量。
- 浏览器IDB／WebLocks与测试文件fsync不等于设备断电、OS安全库或生产灾备；临时构建产物可重建，不能当持续部署或用户文件交付。
- 与本报告无关的用户已有Xcode工程改动、部署修复及其它代理工作不回滚、不覆盖。中央资产表由主代理统一同步，避免并发覆盖。

<a id="🔚" href="#前言">返回首页</a>

# <span id="前言">Web 本人主动改密闭环</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、已实现与当前验证](#jobs-toc-heading-1)
- [二、产品入口与原责任](#jobs-toc-heading-2)
- [三、源码位置与网络边界](#jobs-toc-heading-3)
- [四、实际执行与原件](#jobs-toc-heading-4)
- [五、复现命令与资源](#jobs-toc-heading-5)
- [六、主流程继续项](#jobs-toc-heading-6)
- [七、当前B3生产包真实HTTPS链路](#jobs-toc-heading-7)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、已实现与当前验证 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

Web设置页已提供本人主动改密和独立原请求恢复入口。当前本人密码、新密码和再次确认、管理员OTP、明确风险确认、显示／隐藏密码、表单即时清空、十语言、三主题及完整空态／重载均已接线。该入口使用[后端2.3协议](../../../代码/后端/Go/接口契约/README.md#own-password-change)，没有改用登录／恢复码接口替代主动改密。

最初组件切片实际Chrome／React／IndexedDB严格事务／Web Locks **21组通过**，全工程TypeScript及独立生产构建通过；后续22组增量见文末。原认证16组、正式导出19组和App演示8类分别按原输入保留，均为明确隔离响应与密码夹具。最新B3生产包另完成自有Canonical真实HTTPS改密链路，7项链路检查通过，详第七节；不与mock组数相加，管理员TOTP及多设备组合仍待核。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、产品入口与原责任 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 入口／模块 | 行为与默认值 | 持久化、生命周期与隔离 |
| --- | --- | --- |
| 设置→修改本人密码 | 默认不发送；实际cap及当前本人／family／device／epoch齐备才可新建 | 新密码NFC后15～128码点、最多512字节；不trim显著空格。当前／新密码及OTP仅在表单／本次请求RAM；提交、换单／family或销毁清表单 |
| 管理员OTP及明确确认 | 管理员新建须填验证器代码；先明确全部设备退出／重新登录的后果 | OTP只加入首次有原family授权的改密POST，不进入纯结果查询、持久业务原单或原请求摘要 |
| 本浏览器原请求 | 显示本地原nonce、用户名、时间和状态；未登录可输入本人用户名重载并重新提供原证明 | 沿既有JobsIM v1／spaces数据库使用`password-changes\|origin\|deployment`键，不新增数据库或版本；只保存nonce、身份绑定、尝试数、状态与白名单结果 |
| 只核对原结果 | 始终匿名`password-change-results`，404／未知只保原单 | 固定原用户名／设备／epoch／nonce和重新输入的原规范化密码对；无Cookie、family、CSRF、OTP，也不自动改密或换ID |
| 明确重试原请求 | 先查原单；已prepared可匿名续原责任 | 未见原单时仅原family仍匹配、实时cap／epoch可用且原CSRF有效才可首建；不以另一个当前family续投旧意图 |
| unknown与冷启动门禁 | POST前严格提交非秘密unknown责任；未知后停止该原family的普通业务 | 普通API在HTTP前核责任；冷`auth/context`学习也不能重新激活该family；IDB等候结束后再次核原认证快照，不能借后来身份发送旧请求 |
| changed／pending／retired | 不将pending当全部完成；成功要求新登录，不自动代登录 | 只停止当前仍匹配的原family，必要时同事务退休原namespace。旧结果不能清后继Cookie／身份／namespace；历史请求`credentials:omit`忽略响应Set-Cookie |

应用不持久密码、OTP、完整密码请求body或无钥密码摘要。非秘密原责任使用字段白名单，即使调用者意外传入额外密码字段也不落盘；结果同样只保留协议声明字段。浏览器密码管理器的用户行为、内存物理擦除与设备断电不属于本轮证明范围。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、源码位置与网络边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 文件 | 职责 |
| --- | --- |
| [PasswordChangePanel.tsx](../../../代码/前端/Web/Src/PasswordChangePanel.tsx) | 设置内表单、本人原单选择、风险确认与原结果恢复；先展示本次主表单，再列历史 |
| [PasswordChangeLabels.ts](../../../代码/前端/Web/Src/PasswordChangeLabels.ts) | 十语表单、状态、恢复与错误文案 |
| [PasswordChangeProtocol.ts](../../../代码/前端/Web/Src/PasswordChangeProtocol.ts) | 身份／DTO白名单、精确大代际、NFC密码对校验 |
| [PasswordChangeStore.ts](../../../代码/前端/Web/Src/PasswordChangeStore.ts) | 严格IDB原责任、结果与原namespace退休；冷family门禁 |
| [PasswordChangeActions.ts](../../../代码/前端/Web/Src/PasswordChangeActions.ts) | 专用有界JSON transport、匿名纯查、原family／CSRF首写、双层WebLocks与迟到围栏 |
| [api.ts](../../../代码/前端/Web/Src/api.ts) | 普通HTTP与冷context学习的原责任门禁；异步IDB后再次核原快照 |
| [App.tsx](../../../代码/前端/Web/Src/App.tsx)／[styles.css](../../../代码/前端/Web/Src/styles.css) | 当前身份捕获、原family退出至登录、设置页挂载及既有语义色／响应式 |

通用`api`带Cookie、CSRF并可能自动刷新，不用于改密原单纯查。新transport只允许本模块固定路由，设置`X-JobsIM-Client:web`、JSON、`redirect:error`、`cache:no-store`，使用浏览器同源Origin；历史结果与匿名续尾始终omit凭据。只有明确未见原单且原身份仍一致的首次授权使用Cookie＋原family＋新取CSRF。响应限制32KiB、验证JSON类型和白名单DTO，不将原证明写日志。

改密与现有登录共用`jobsim-auth-lifecycle`外锁、与刷新共用`jobsim-auth-refresh`内锁；不调用自动refresh。能力或身份在CSRF／锁等待／请求期间改变时拒绝旧写；旧响应只可保留原责任，不能清后继状态。服务端已提供两条所需路由，本切片没有新增或修改后端API。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、实际执行与原件 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

执行发生于2026-11-17凌晨（客户端时区Asia/Shanghai）；目录沿主任务指定的20261116批次保留。

| 检查 | 结果 | 原件 |
| --- | --- | --- |
| 最终改密专项 | 21 PASS／0 FAIL，执行期间源码未变 | [最终结果](./Web改密/12-final-layout/结果.json)、[监管](./Web改密/12-final-layout/监管.json)、[冻结源码](./Web改密/12-final-layout/输入源码/) |
| 当前API／Auth认证回归 | 16组通过，模拟BFF | [原结果](./Web改密/08-final-auth/浏览器协议结果.json)、[输出](./Web改密/08-final-auth/执行器输出.log) |
| 正式导出回归 | 19组通过 | [原结果](./Web改密/09-final-export/结果.json) |
| App真实Chrome演示 | 8类通过，无页面JS异常；演示发送／主题／十语／字号／窄屏 | [原结果](../../测试/前端/Web/执行记录/1794846077269-79975/browser-result.json)、[实际输出](./Web改密/10-final-app/执行器输出.log) |
| TypeScript／独立生产构建 | 退出0，未覆盖Dist或切换运行服务 | [最终构建](./Web改密/13-final-build.json)、[构建日志](./Web改密/13-final-build.log) |
| 当前输入核对 | 43项最终源码全部匹配，认证依赖API／Auth与认证轮相同 | [输入核对](./Web改密/14-inputs-audit.json) |

21组覆盖：原责任先于HTTP；提交丢响应；投递前丢失和404零写；后继family与历史结果隔离；无会话已prepared匿名续尾；cap撤回；CSRF等待后换family；坏200／晚family；明确首拒无refresh；unknown普通API门禁；IDB中止零POST；错误原证明；真实生命周期锁排队失效；NFC和显著空格；额外身份字段不落秘；IDB异步等待不借后继身份；冷BFF学习门禁；实际匿名fetch拒Set-Cookie／不送后继Cookie；真实表单显示／确认／清空；管理员OTP仅首次授权；十语三主题窄屏与完整空态。

初次[01-local失败](./Web改密/01-local/结果.json)发现pending结果保留后显式续跑转unknown被校验误拒，已修正为允许unknown保留原历史结果；失败原件不改。[02-local的17组](./Web改密/02-local/结果.json)、[03-final-local的20组](./Web改密/03-final-local/结果.json)和[07-final-local的21组](./Web改密/07-final-local/结果.json)各按自己的输入保存；末次截图核查后只调整表单与历史的先后顺序，再执行最终12轮21组及13轮构建。没有用旧轮数量补最终轮。

已实际查看中／英／藏与明暗截图；最终[简中主表单](./Web改密/12-final-layout/Password-zh-CN-light.png)主操作在历史空态之前。[英文深色](./Web改密/12-final-layout/Password-en-dark.png)和[藏文跟随系统](./Web改密/12-final-layout/Password-bo-system.png)保留同轮原图。未测试屏幕阅读器、移动真机及OS密码管理器。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">五、复现命令与资源 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

从项目根目录运行；报告路径换成新目录。当前复用Node24.22.1、已有Playwright1.65.1与Chrome；不安装依赖／证书，不读取真实账户或运行域seed。

```sh
NODE="$HOME/Library/Application Support/JobsIM/Toolchains/node-v24.22.1-darwin-arm64/bin/node"
JOBSIM_PLAYWRIGHT_MODULE="$HOME/Library/Application Support/JobsIM/Toolchains/Playwright-1.65.1/node_modules/playwright" \
JOBSIM_PASSWORD_REPORT="$PWD/项目相关报告/实现进度/20261116-剩余能力闭环/Web改密/新的轮次" \
  "$NODE" 测试/前端/Web/password-change.cjs/password-change.cjs
"$NODE" 代码/前端/Web/node_modules/typescript/bin/tsc --noEmit -p 代码/前端/Web/tsconfig.json
```

认证／App回归外层私有Vite用`configFile:false`、随机回环端口与独立cacheDir，再将实际URL交`JOBSIM_WEB_URL`；改密及导出专项自行启动同样无代理的Vite。每子进程外层设180秒watchdog、每秒核4GiB下限，只能结束本任务进程组，实际均正常结束；最终组最小空闲约11GiB。未清理缓存或历史证据。

最终Vite构建使用工程原配置，但`outDir`指向Application Support下新的Web／PasswordChange构建目录，`emptyOutDir:false`。实际目录见构建JSON；当前JS为`index-DOFFbBfO.js`，SHA256 `868d9db3f500df05dbd9220b79a8d0275a5d22c821d7699d8902bb31b2ecef44`，CSS为`index-ZILFTibV.css`。没有覆盖旧Dist／旧包或更新真实服务；主包超过500kB及lucide-react指令告警保留于真实构建日志。

## <span id="jobs-toc-heading-6" data-jobs-toc="v2">六、主流程继续项 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

源码／组件本地闭环已完成，真实生产包与普通非管理员账号的主动改密／原family失效／新登录／原结果纯查已按第七节完成。管理员真实TOTP、多设备同时family撤销矩阵、实际网络未知与断电恢复分别待验，不借CLI／后端旧包或注入响应证明。浏览器仍不签发native耐久／删除资格。

Web根README、代码地图、测试总入口及本专项README已同步；中央资产与总进度交主代理统一更新，避免并发覆盖。完整群治理、注销、普通迁移及收讫／保留等原任务继续，不由此子切片冒充全部I完成。未提交、未推送，保留前序Web日期修复和用户其它已有改动。

2026-11-17增量：同轮[Web注销](./Web注销.md)审查把“任意400／401／403就视为未建拒绝”收紧为明确路由前置状态／错误码组合，其余保unknown；新增prepared后依赖403反例，[22组改密浏览器重跑](./Web注销/03-改密回归/结果.json)通过。同轮注销23／导出19／认证16／App8与TS／独立生产构建通过，原21组与原包记录不改写；新包与输入以注销报告为准。

## <span id="jobs-toc-heading-7" data-jobs-toc="v2">七、当前B3生产包真实HTTPS链路 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

2026-11-17下午在主代理实际原入口启动的自有Canonical `https://127.0.0.1:55443` 执行；deployment为既有`3334f58a-4d55-41e9-9c9c-09957b52a73a`，没有访问默认a4。Node逐请求严格验证实例CA、主机名和活动leaf，Chrome仅允许本次server.crt的精确SPKI，`ignoreHTTPSErrors:false`，并拦截其它origin；没有装系统CA或忽略全部证书。主页实际加载`index-B3U1AnNU.js`，线上SHA与[冻结私有成品](./Web逐存储收讫/15-当前输入与产物交接.json)一致：`ba564110169077f714cd2ece90cb18bc64ba2a1911754ccc54de5687cd16839b`。

[最终真实结果](./Web真实HTTPS改密/1794900127874-989/结果.json)为7项链路检查通过、pageErrors为空；[执行回执](./Web真实HTTPS改密/1794900127744-执行.json)exit0、2.84秒、最低约14.8GiB。82份Src／入口／配置／锁文件跑前后以及最终交接时全部与B3构建输入一致；[验收映射](./Web真实HTTPS改密/验收映射.json)封存源码／包／驱动／截图SHA与实际边界。没有注入HTTP响应或导入开发Src模块代替生产包。

| 实际行为 | 已核结果 |
| --- | --- |
| 随机普通账号注册，再退出并从生产UI登录 | register／confirm／context与login真实成功，响应明确非管理员；未使用真实用户数据 |
| 设置→当前密码／新密码／确认／明确同意→修改 | 真实password-change返回changed及requiresNewLogin，页面回到登录；原Cookie family经严格HTTPS context校验为401 |
| 旧密码与新密码 | 旧密码实际INVALID_CREDENTIALS；新密码由合法原认证责任链走reauthenticate并confirm，建立不同新family |
| 选择原改密请求→只核对原结果 | 原nonce查询200／changed，无Cookie／family／CSRF，改密POST数量不增加；查询后新family context仍200，旧结果没有清新身份 |
| 秘密与退出 | 浏览器localStorage／sessionStorage／IndexedDB未含本次两密码；截图前所有密码框均空。最终本人新family通过UI退出，原Cookie再查401；没有删除账号或历史 |

前三轮驱动失败均保留：首轮[1794899722922](./Web真实HTTPS改密/1794899722922-94698/结果.json)注册成功后服务端偏好切回中文，驱动按英文Settings定位超时；第二轮[1794899866906](./Web真实HTTPS改密/1794899866906-97329/结果.json)未等退出后的React状态落地，头像打开旧profile；第三轮[1794899981555](./Web真实HTTPS改密/1794899981555-99500/结果.json)实际改密、旧密拒绝和新密码reauthenticate均已成功，但驱动只等login路由超时。修正只在[本次驱动](./Web真实HTTPS改密/真实烟测.cjs)，生产源码未改；不把这三轮当完整通过。首轮注册family未显式logout，关闭浏览器不等于撤销；随机密码已清RAM，未借管理员清库。第二轮UI退出、第三轮finally当前本人退出均有200，第四轮最终也已退出。

已实际查看[退出后登录弹窗](./Web真实HTTPS改密/1794900127874-989/02-已改密-回登录.png)及[新登录下原单空密码表单](./Web真实HTTPS改密/1794900127874-989/03-新登录-原结果只查.png)。01原图内部滚动位置位于注销区，不用作改密表单布局证明。此次只覆盖英文白天普通账号安全链，不证明管理员TOTP、多设备family全矩阵、真实断网未知、移动真机、密码管理器或十语三主题全部组合，也不把前述147个mock检查或原生122项累加进本次7项。

复现只能在明确授权、已实际启动且匹配该Canonical身份和B3产物的隔离实例，不能直接套到a4：

```sh
JOBSIM_PLAYWRIGHT_MODULE="$HOME/Library/Application Support/JobsIM/Toolchains/Playwright-1.65.1/node_modules/playwright" \
  "$HOME/Library/Application Support/JobsIM/Toolchains/node-v24.22.1-darwin-arm64/bin/node" \
  项目相关报告/实现进度/20261116-剩余能力闭环/Web真实HTTPS改密/真实烟测.cjs
```

执行外层采用180秒／4GiB动态门禁，只管理本轮进程组；不用HAR或含凭据请求日志，密码／Cookie／CSRF／令牌仅RAM。完成后浏览器关闭，主代理按原入口精确停妥Canonical并保留数据；默认a4未被本测试操作。

<a id="🔚" href="#前言">回到首页</a>

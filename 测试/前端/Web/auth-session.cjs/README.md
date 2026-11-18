# <span id="前言">Web 登录续期浏览器协议回归</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行](#jobs-toc-heading-1)
- [二、范围](#jobs-toc-heading-2)
- [三、失败诊断与固定次数专项](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

在 Web 工程运行 Vite 后，设置 `JOBSIM_WEB_URL` 为其地址，运行同目录 `auth-session.cjs`。可用 `JOBSIM_PLAYWRIGHT_MODULE` 指定已安装的 Playwright，`BROWSER_EXECUTABLE` 指定 Chrome，`JOBSIM_AUTH_REPORT` 指定报告目录；不会安装依赖。默认访问本机5188端口。应在源码稳定后运行，避免 HMR 在用例中途替换模块。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

真实 Chrome 执行源码模块、Web Locks、IndexedDB、localStorage／sessionStorage 与 fetch，BFF 响应由测试路由模拟。当前 16 组覆盖并发续期、跨标签页单次消费、未知响应重载后停止重试、初始／后继原 nonce、迟到跨账号响应、私有媒体家族头、存储失败先于刷新 POST、冷启动公开上下文学习、部署隔离、旧设备队列冻结、原证明保护、完整认证生命周期锁及能力关闭后的请求拒绝。

本回归不证明服务端事务、真实 Cookie 保密、TLS、真实媒体权限或全产品登录 UI；这些必须另跑 HTTPS 与原生交互测试。报告不包含真实账号秘密。

最近一次专项结果见[认证输入保护报告](../../../../项目相关报告/测试/前端/Web/20261115-认证输入保护/README.md)。组数按脚本的 `checks` 数组计算，不代表功能覆盖率。

共享请求等待后的原身份围栏调整后，本入口新跑16组通过，合法冷启动公开上下文学习仍通过；输入前后摘要和本轮原始输出见[原身份围栏报告](../../../../项目相关报告/测试/前端/Web/20261117-原身份等待围栏/README.md)。等待切换family与同family轮换的三个新增定点反例在群治理入口执行，不算入本入口16组。


默认输出根据脚本位置写入工程根 `项目相关报告/测试/前端/Web/执行记录/本脚本-时间-PID/`；`JOBSIM_AUTH_REPORT` 继续优先，用它为当前范围选择新中央子项。报告迁移只改默认路径，未重跑浏览器或 HTTPS。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、失败诊断与固定次数专项 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

成功报告记录实际打开URL与api／AuthSession模块importURL。异常时另写`认证失败诊断.json`：原错误name／message／code／status、已完成检查、模拟BFF错误响应、仍打开的各tab公开authSnapshot／authBlocked／authRecovery、namespace、模块URL与实际import实例一致性；不改变断言、请求内容或生产保护。诊断只含隔离身份元数据，不保存密码、CSRF、AT／RT或Cookie。

当前驱动同时保留有界Storage写／删／事件、公开BroadcastChannel字段及跨tab请求／响应时序；每条记录关联tab、公开family／代际与阶段。实际未用BroadcastChannel时记录为零事件。关键双tab并发前不增加新的等待屏障；原16组断言保留。`JOBSIM_AUTH_CONCURRENT_COLD_IMPORTS=1`仅让测试驱动对已解析的同一AuthSession与api模块并发import，默认仍顺序；`JOBSIM_AUTH_VARIANT`写入明确实验变量。

[附件验证运行器](../../../../项目相关报告/测试/前端/Web/20261117-附件耐久发送/运行器.cjs)的`auth-five`提供固定五次专项：每次新Vite／浏览器／报告，前三次无尾斜杠、后两次固定带尾斜杠；不因红绿选择性跳过，总览保留全部尝试，每阶段仍有4 GiB／180秒门禁。当前五次均通过；此前另轮双tab失败仍未归因，不据五次绿宣称已修，具体原件与边界见[附件报告](../../../../项目相关报告/测试/前端/Web/20261117-附件耐久发送/README.md)。

后续[Web09定向运行器](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Web09定向诊断/本轮运行器.cjs)预先固定最多10次／首次失败即停：尾斜杠有无×冷模块顺序／并发，每轮新私有Vite cache及Chrome进程／context。实际到10次上限，原16组每轮完成；91项输入（82生产源码／配置、7Public及驱动／运行器）全部前后不变，最低约12.7GiB，没有触发资源门禁。双tab结束时同family／generation2、每tab仅一个AuthSession资源URL；有Storage事件，Broadcast为零。结果未出现夹具密码／CSRF／原proof字符串，见[脱敏核对](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Web09定向诊断/脱敏诊断核对.json)。

原09失败仍缺当时code／tab快照／模块URL，无法从本次未复现反推为fixture或冷启动原因；诊断本身也可能影响时序。[本轮结论](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Web09定向诊断/本轮结论.json)明确保留未归因，不把10次通过称产品修复，不继续扩大次数。生产Src未改，真实HTTPS改密7项为独立证据，不能消掉旧双tab失败。

<a id="🔚" href="#前言">回到首页</a>

# <span id="前言">Web 原身份等待围栏回归</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前实现与本轮改动](#jobs-toc-heading-1)
- [二、缺陷、修复与原始失败](#jobs-toc-heading-2)
- [三、关联回归与输入固定](#jobs-toc-heading-3)
- [四、截图与私有产物](#jobs-toc-heading-4)
- [五、验收边界](#jobs-toc-heading-5)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前实现与本轮改动 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

现有本人改密、注销／数据权利、群治理／邀请／入群入口已接实际组件、独立原责任存储和十语三主题。本轮依据当前后端处理代码和DTO核对这些入口，保留既有原nonce、所见版本、身份／设备／恢复代际、敏感证明仅RAM及严格事务先于HTTP的约束；未重复搭建已有功能。

| 入口 | 原有行为与默认值 | 本轮状态 |
| --- | --- | --- |
| 设置→修改本人密码 | 明确确认后提交；管理员OTP；原责任先落盘，历史只查匿名且不存密码对 | 22组新输入浏览器通过；中文白天／英文黑夜／藏文跟随系统截图已查看 |
| 设置／登录前→注销本人账号 | 默认不申请；无普通会话仍可查原单及受限权利；取消需原nonce、当前精确版本及用途证明 | 23组新输入通过；凭据清空、完整空态／重载和无会话入口已验 |
| 群详情／新会话→治理与凭码入群 | 所见群和成员资格固定；未知只查原结果；明确重试不换body；token／OTP／step-up仅RAM | 29组通过，包含本轮三个共享transport反例；藏文黑夜入群空态截图已查看 |
| 普通同源API | API等待期间仍属于调用时的原身份，同family刷新generation可继续 | 本轮补两处原快照检查，阻止后继family接管旧body |

源码入口见[Web说明](../../../../../代码/前端/Web/README.md)及[代码地图](../../../../../代码/前端/Web/代码地图/README.md)。现有功能的完整原始证据仍在[前序改密](../../../../实现进度/20261116-剩余能力闭环/Web改密.md)、[注销](../../../../实现进度/20261116-剩余能力闭环/Web注销.md)、[群治理](../../../../实现进度/20261116-剩余能力闭环/Web群治理.md)报告；本轮未把旧包结果归给新包。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、缺陷、修复与原始失败 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

普通`api()`调用在`waitForRefresh`或access过期后的`refreshSession`返回后，原先直接重新读取当前认证快照。即便空闲刷新协调器只让出一次微任务，另一次登录也可能已切换family；旧body因此携后继family和CSRF发送。组件在响应后核对身份不能撤回这次写请求。

有效修前[原始失败](./04-有效修前/失败.json)记录：切换已经发生，原调用却未拒绝，`/api/auth/csrf`和`/api/owned-family-write`均带后继family。该路径由真实浏览器中的源码`api()`执行，HTTP返回为显式夹具。

[api.ts](../../../../../代码/前端/Web/Src/api.ts)在等待已有刷新后立即`checkSnapshot(snapshot, signal)`；因access过期续期后，已有认证身份同样先核原快照再读最新同family元数据。身份变化返回`AUTH_REQUEST_ABORTED`，保留后继身份。原本无身份的合法冷启动公开上下文学习仍由既有认证流程核验，认证16组新跑通过。本轮没有放宽冷改密／注销门禁。

| 目录 | 记录性质 | 结论 |
| --- | --- | --- |
| `01-修前`、`02-修后`、`03-诊断` | 初始夹具未向页面暴露已导入的`api`，得到TypeError；原件和进程日志保留 | 夹具错误，不能证明产品缺陷或修复有效 |
| `04-有效修前`、`修前源码` | 修正夹具后，恢复原API字节重新执行 | 真实复现旧body被后继family发送 |
| `05-有效修后` | 固定修复源码执行29组；结果含源码SHA和成功输入副本 | 三个新反例及原26组均通过 |

三个新增反例分别检查空闲刷新等待中的换family零HTTP、同family仅更新refreshGeneration可正常提交、ACCESS_EXPIRED完成刷新后的换family不得重放旧body。两个拒绝分支都检查后继身份仍在。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、关联回归与输入固定 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[本轮运行器](./本轮运行器.cjs)先固定Web全部源码／本地资源、入口、锁文件、配置和五个关联脚本，保存[闭包SHA](./本轮源码闭包SHA.json)与`本轮输入源码/`副本；各阶段再记录执行前后摘要。全部阶段输入未改变。原操作ID、代际等测试值均为隔离夹具，未使用真实账号。

| 阶段 | 新执行结果 | 原始证据 |
| --- | --- | --- |
| 群治理与transport | 29组通过 | [结果](./05-有效修后/结果.json) |
| 全工程严格类型 | `tsc --noEmit`通过 | [进程回执](./06-类型检查/进程.json) |
| 登录／刷新协议 | 16组通过，包含合法冷启动学习 | [浏览器结果](./07-认证协议/浏览器/浏览器协议结果.json) |
| 本人改密 | 22组通过 | [结果](./08-改密/浏览器/结果.json) |
| 注销／数据权利 | 23组通过 | [初次结果](./09-注销/浏览器/结果.json)、[截图入口修正后结果](./13-注销稳定截图/浏览器/结果.json) |
| 正式本人导出 | 19组通过 | [结果](./10-本人导出/浏览器/结果.json) |
| 私有生产构建 | 成功；源码摘要未变 | [进程回执](./11-私有生产构建/进程.json)、[产物SHA](./11-私有生产构建/产物SHA.json) |

05组仅执行入口的启动前4GiB检查，并由入口finally关闭本次浏览器和[**Vite**](https://vite.dev)；没有把其它轮的持续监控追认为本轮。06～11由本轮运行器每秒检查4GiB余量、每子进程180秒上限并仅管理自己的进程组，串行单worker。最低采样余量4812910592字节，未触发停止；12诊断和13最终截图也独立受4GiB／180秒限制。未下载依赖、SDK或VM，未重启任何实际服务。

13只将注销脚本最后的窄屏抓图改为元素截图，产品源码未改；脚本SHA执行前后均为`7401276fba9774184db429b280a418593354499efe03f5e00ab76a743a7f9605`。05与06～11的原始输入不重写，分别以各自摘要为准。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、截图与私有产物 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

已实际查看入群1张、改密3张、导出3张以及注销桌面／窄屏截图。09的窄屏原图在刚切换viewport后立即抓全页，出现顶部重复的组合现象，原图保留。[12的DOM诊断](./12-注销截图诊断/浏览器/DOM诊断.json)确认仅1个注销面板，原App根仍隐藏；等待稳定后的[单夹具截图](./12-注销截图诊断/浏览器/窄屏-夹具稳定后.png)无重复。现有脚本改为`fixture.screenshot`等待元素稳定，13标准套件重新通过，最终图另存，不用旧图代替新验。

生产包使用已有[**Node.js**](https://nodejs.org)、[**Vite**](https://vite.dev)和锁定依赖，输出到用户级独立构建目录，未覆盖工程`Dist/`或切换现有服务。

| 产物 | SHA-256 |
| --- | --- |
| `assets/index-JftJd78R.js` | `d06621257fbffe03b39c6c7dce895aaeec616e0c4ed25dc083ce2288e7d3b343` |
| `assets/index-BsxsEHpf.css` | `d3730e836387ec79548f82a5ebe7d7994dc66696f531fe9da637826e501ff908` |
| `Src/api.ts` | `319dec1d73e4355b7f43517e4d5615302be20f21b45ed624d69c5317e447a89f` |

构建目录和所有文件大小／摘要以[产物记录](./11-私有生产构建/产物SHA.json)为准。执行回执保留执行时的原工作路径；随后整个工作树移入`JobsProds/JobsIM`，源码字节及相对证据仍在，本轮不重建旧位置或改写历史回执。

11构建冻结后，迁移并发工作新增`MigrationHistoryStore.ts`并调整`GlobalMessages.ts`；它们不在11包的原输入中。上表类型／浏览器和包只对应各自保存的输入，迁移整合后的最新Web另取新输入验证，不能把本包外推为当前全部工作树。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">五、验收边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

浏览器使用真实[**React**](https://react.dev)、[**IndexedDB**](https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API)和[**Web Locks**](https://developer.mozilla.org/en-US/docs/Web/API/Web_Locks_API)，HTTP、身份、密码及剪贴板是显式RAM夹具；实际`/api`网络阻断，不连接旧服务、当前[**Go**](https://go.dev/)服务或用户真实账号。单项结果不可相加当作产品功能数或第一阶段总完成率。

本轮不证明真实HTTPS、真实Cookie更新、服务端密码／MFA消费、跨域journal、真实群成员修改、账户抹除或移动原生交互；这些沿各自输入、当前服务包及独立窗口继续验收。源码存在、局部回归通过和私有构建成功均不等于全项目交付完成。

<a id="🔚" href="#前言">回到首页</a>

# JobsIM Web 注销与数据权利闭环

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、交付入口](#jobs-toc-heading-1)
- [二、未知与取消责任](#jobs-toc-heading-2)
- [三、实际验证](#jobs-toc-heading-3)
- [四、复现与边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、交付入口</span><!-- jobs-toc:heading-end -->

Web 设置增加本人注销表单，登录／注册／恢复弹窗增加无普通会话的“注销本人账号”入口。已有注销责任可以重新载入、只核对原申请、明确续原申请，或重新验明本人后查询／取消；没有普通会话不需要借另一个账号的Cookie操作。取消成功后仍需新登录。

| 文件 | 当前行为 |
| --- | --- |
| [AccountDeletionPanel.tsx](../../../代码/前端/Web/Src/AccountDeletionPanel.tsx)／[App.tsx](../../../代码/前端/Web/Src/App.tsx) | 当前本人发起、原单列表／完整空态／重载、已有注销ID登记、凭据选择、明确确认、服务器截止、未知撤销与原nonce展示；匿名入口直接复用同一组件 |
| [AccountDeletionProtocol.ts](../../../代码/前端/Web/Src/AccountDeletionProtocol.ts) | 结果／proof白名单、NFC密码、不trim显著空格、字符串大版本、当前用途／时效／主体绑定及单调结果 |
| [AccountDeletionStore.ts](../../../代码/前端/Web/Src/AccountDeletionStore.ts) | 原创建身份和nonce、所见注销ID／版本、取消nonce及阶段先严格IDB提交；家庭门禁跨冷启动保留，不存密码、恢复码、OTP、proof、证明body或无钥hash |
| [AccountDeletionActions.ts](../../../代码/前端/Web/Src/AccountDeletionActions.ts) | 有界32KiB JSON、15秒请求、redirect拒绝、无缓存；匿名只查与已准备续尾、原family／CSRF首写、query/cancel用途隔离及双层WebLocks |
| [api.ts](../../../代码/前端/Web/Src/api.ts) | 普通API及冷BFF context学习先核原family是否已有未知／既成注销责任；IDB等待后重验原身份，不能借新family继续 |
| [AccountDeletionLabels.ts](../../../代码/前端/Web/Src/AccountDeletionLabels.ts)／styles | 简中、繁中、藏、英、日、法、西、德、俄、韩完整文案；继承全局白天／黑夜／跟随系统语义色和字号，窄屏不越界 |

新建必须实时`own_account_deletion`、当前本人／原family／device／epoch及当前密码，管理员另需OTP。服务端接受或结果未知后只暂停原family，原namespace退休由其已成结果驱动；取消后不复活旧session。只认明确前置拒绝错误码可以解除未建原单；依赖的403保unknown，不能凭HTTP状态码断言未创建。

历史`deletion-results`和所有`data-rights`请求均`credentials:omit`，无普通family／CSRF；首次授权才携当前Cookie／原family／新CSRF。服务端注销公开响应也不发Set-Cookie，晚到旧结果无法清后继Cookie。表单提交、切换记录／凭据类型／family和销毁会清空输入，密码／恢复码／OTP／purpose-bound proof仅在本次RAM调用中存活。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、未知与取消责任</span><!-- jobs-toc:heading-end -->

| 情况 | 行为 |
| --- | --- |
| 创建已成但响应丢失 | 原nonce与非秘密上下文保留，重新输入原密码，只查原结果，不发第二个创建请求 |
| 创建未投递，查询404 | 不自动创建。只有用户明确重试且原family仍有资格时沿原请求／设备／epoch发送；另一个family和匿名状态不能首次授权旧意图 |
| 已prepared但未完成 | 本人重新输入原证明后可匿名明确续同一原责任；不重新登录借新family发布旧意图 |
| 取消响应未知 | 原取消nonce先持久化。禁止自动重投与换新nonce；先查询服务端原结果，比较`cancelRequestId`与当前版本 |
| 取消未成且仍requested | 查询先确认仍是原版本且没有既成取消，再由用户明确重试原取消nonce；使用新的cancel用途证明 |
| 取消已成／进入processing | 旧query版本可重新验当前凭据，服务器签出当前版本的query-only证明，随后读出当前结果。不得把新版本静默重基成旧取消授权 |
| family或挂载代际变化 | 排队／异步／迟到请求拒发送或提交旧结果；允许匿名只读历史但不清后继family／namespace |

记录使用现有JobsIM v1／spaces内`account-deletions|origin|deployment`。清站点数据会丢失本机原责任入口，不代替服务端撤销。手动登记已有注销ID／所见版本仅提供受限查询的非秘密定位；仍必须当前可信凭据，不形成普通登录资格。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、实际验证</span><!-- jobs-toc:heading-end -->

[最终23组](./Web注销/02-注销最终/结果.json)使用实际Chrome、React、IndexedDB严格事务及WebLocks。覆盖双未知、404零创建、原family／匿名续尾、冷门禁、purpose／时效／主体、恢复码、取消原nonce／两类丢响应、服务端当前版本、依赖403、旧版本／坏DTO／超限／存储拒秘、实时能力、十语、管理员确认／OTP／即时清空、完整空态和390宽布局。首轮22组保留于[01轮](./Web注销/01-首轮/结果.json)；随后新增prepared依赖403反例并收紧拒绝判定，最终23重新通过。

同一生产源码另跑：

| 验证 | 结果／证据 |
| --- | --- |
| 改密回归 | [22组通过](./Web注销/03-改密回归/结果.json)，包含新增prepared后403仍保原family门禁；早前21组及原包继续保留 |
| 正式本人导出 | [19组通过](./Web注销/04-导出回归/结果.json)；真实保存端仍为注入，不宣称用户磁盘落盘 |
| 认证协议 | [16组通过](./Web注销/07-认证与App回归.log)，BFF为明确模拟，未连接真实HTTPS |
| 主App／演示 | [8类别通过](../../测试/前端/Web/执行记录/1794850350852-69548/browser-result.json)：发送、日期／导航、原队列、主题、字号、十语、桌面／移动布局、无JS异常 |
| TypeScript／生产构建 | [类型](./Web注销/05-类型执行.json)及[构建](./Web注销/06-构建执行.json)均exit0；未覆盖原Dist或切换运行服务 |

原输入及构建设置见[最终输入索引](./Web注销/09-最终输入/索引.json)，全部注销浏览器候选源码与冻结时当前文件一致；响应夹具不冒充后端真实验证。[截图](./Web注销/02-注销最终/藏文-黑夜-无普通会话.png)和[窄屏](./Web注销/02-注销最终/窄屏-藏文-黑夜.png)均来自实际组件，已检查。

本轮构建在工程外`Build/Web/AccountDeletion/1794850284064-66969/Production`。JS `index-CI7uKxyI.js` SHA256为`441b213a8be9f8db4fb922f8c4f789fed011e24986c5af89e89f01328ae2dfbf`；CSS `index-BIH98Oqq.css`。精确产物路径／摘要见[产物记录](./Web注销/08-产物与回归摘要.json)。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、复现与边界</span><!-- jobs-toc:heading-end -->

```shell
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_DELETION_REPORT=/本轮未使用的新目录 \
node 测试/前端/Web/account-deletion.cjs/account-deletion.cjs
node 代码/前端/Web/node_modules/typescript/bin/tsc --noEmit -p 代码/前端/Web/tsconfig.json
```

Node24.22.1、现有Playwright1.65.1与Chrome；随机Vite不读开发代理，实际`/api`全部阻断。外层每秒核4GiB门禁、每子进程180秒，只能结束自己的进程组；末轮最低余量约7.3GiB。未清理缓存、旧报告、真实账号或第三方源码，没有提交／推送。

本片实际证明浏览器产品／存储／恢复边界，密码校验、MFA消费、journal与实际内容抹除属于[后端注销](../../../代码/后端/Go/account_deletion.go)及独立真实PG验证；真实HTTPS、本次部署的新包和真实账号全设备失效仍须独立联调，不借旧CLI／后端结果外推。

completed后密码／MFA／恢复码已清，不能据旧密码新签本人证明。原创建完整上下文及原密码仍能只查本人原单；之前签出的query证明在其短有效期内仍可观察completed。跨浏览器仅有注销ID和旧密码不能新签证明；需原创建上下文或受控管理员查询最小安全记录，不降低删除后的凭据清理要求。

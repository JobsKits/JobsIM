# JobsIM Web 普通聊天迁移

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前状态](#jobs-toc-heading-1)
- [二、源码已实现](#jobs-toc-heading-2)
- [三、实际结果与待核](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前状态</span><!-- jobs-toc:heading-end -->

源码已接入设置入口与[稳定后端合同](../../../代码/后端/Go/接口契约/普通迁移/README.md)。2026年11月17日先按4GiB门禁在约3.5GiB时停止构建，余量恢复后完成迁移33组浏览器验证及后续共享输入下33组复验，零未处理异常。并行附件及注销清理的类型问题由各自维护者修正后，TypeScript与私有生产构建已通过；共享业务结果、双tab认证失败与后续收讫声明增量分轮记录。本记录不将前序群或改密冻包追认为当前迁移验证。

工程整体迁到`JobsProds/JobsIM`后继续，不重建旧路径；历史JSON中的旧执行路径保持原证据，不改写。没有操作默认a4或Canonical实例、用户数据和第三方，没有提交／推送。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、源码已实现</span><!-- jobs-toc:heading-end -->

| 位置 | 行为／持久化／生命周期 |
| --- | --- |
| [MigrationPanel.tsx](../../../代码/前端/Web/Src/MigrationPanel.tsx)／[App.tsx](../../../代码/前端/Web/Src/App.tsx) | 设置入口、接收配对、源选择、加密包文件队列、原请求只查／明确重试、目标整批导入及独立本机历史；空态、重载、分页；关闭／后台／身份变化销毁输入与RAM证明 |
| [MigrationActions.ts](../../../代码/前端/Web/Src/MigrationActions.ts) | 实时capability、当前physical store、原family门禁、同体续原单、当前GET原JSON规范化、原件有界读取、独立取钥、整批validate、installed_reported与取消 |
| [MigrationProtocol.ts](../../../代码/前端/Web/Src/MigrationProtocol.ts) | 字符串代际、固定原ID／CV／模式、标准Base64原件和Base64URL envelope、256／64／64MiB／128MiB上限；不造cloud_only原收讫证明 |
| [MigrationStore.ts](../../../代码/前端/Web/Src/MigrationStore.ts) | 严格IDB非秘密原责任，nonce／身份／store／selection／普通数据digest；CAS式不可重基目标与原内容；未知保存原责任 |
| [MigrationArchive.ts](../../../代码/前端/Web/Src/MigrationArchive.ts) | 原manifest字节SHA、typed scope／user／device／store／epoch与完整manifest，AES-256-GCM及AAD、所有原件长度／SHA、未知字段／重复／凭据／Outbox拒绝 |
| [MigrationHistoryStore.ts](../../../代码/前端/Web/Src/MigrationHistoryStore.ts) | 密文隔离暂存，key不持久；原store／认证／guard／membership／global在原子事务内再核；独立history去重保较新CV，源缓存、cursor、Outbox及资格不动 |
| [SyncStore.ts](../../../代码/前端/Web/Src/SyncStore.ts)／[GlobalMessages.ts](../../../代码/前端/Web/Src/GlobalMessages.ts) | 只取既有registered physical store；已见global同时去除导入正文／Blob及整份对应密文stage，不凭缺文件判断清理资格 |
| [MigrationLabels.ts](../../../代码/前端/Web/Src/MigrationLabels.ts)／styles | 简中／繁中／藏／英／日／法／西／德／俄／韩；白天／黑夜／跟随系统继承语义色和字号 |
| [api.ts](../../../代码/前端/Web/Src/api.ts) | 新有界流式二进制读取复用原认证／CSRF／等待围栏，逐chunk核快照和总长，媒体末尾重核当前私有facts；保留另一线程原身份等待修复 |

原责任位于`migration-intents|origin|deployment|userId`。密文stage位于`migration-stage|namespace|migrationId`，独立正文／Blob历史位于`migration-history|namespace`。密码、OTP、pairingProof、key与installToken只RAM；nonce与普通内容摘要不授予资格。历史导入不构成qualified ACK，服务端回报也明确不证明本机durability，源不会自动清除。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、实际结果与待核</span><!-- jobs-toc:heading-end -->

| 验证 | 结果与边界 |
| --- | --- |
| 首轮浏览器 | [22组通过](./Web普通迁移/01-首轮/结果.json)，2.03秒，最低11.43GiB；保留当时源码与截图 |
| 新增反例首轮 | [30断言](./Web普通迁移/02-新增反例/结果.json)通过，但[日志](./Web普通迁移/02-新增反例.log)真实暴露strict事务已abort后再次abort的未处理异常；此轮不作为干净最终通过 |
| 事务修复重跑 | [30组通过](./Web普通迁移/03-事务异常修复/结果.json)，6.05秒，pageErrors=[]，最低约9.5GiB；源码跑前后哈希完全一致，含真实事务abort、包内毒字段、原JSON片段SHA、去重保新及forward零正文／实时核对 |
| 界面与global终轮 | [33组通过](./Web普通迁移/05-界面与全局终轮/结果.json)，pageErrors=[]；74份输入跑前后相同，最低约7.27GiB。新增实际global通知后旧正文即时消失、已报告状态禁取新key／capsule、入口进后台清RAM；[藏文黑夜窄屏](./Web普通迁移/05-界面与全局终轮/藏文-黑夜-迁移与原责任.png)已目视核对 |
| 前序全站TypeScript | [第6轮](./Web普通迁移/06-类型执行.json)未过；错误来自其他线程并行新增的AttachmentSendActions／AttachmentSendProtocol与AccountDeletionCleanup，见[原输出](./Web普通迁移/06-类型.log)，保留失败原义 |
| 当前全站TypeScript | 维护者修正后，[第7轮](./Web普通迁移/07-类型执行.json)exit0，0.66秒，执行前约5.07GiB；33组对应的Migration源码未变化，另7个附件／音频／清理文件已有增量，需共享业务回归，不能据旧浏览器输入追认 |
| 共享输入迁移复验 | [收讫阶段04](./Web逐存储收讫/04-迁移复验/结果.json)33组通过，同输入收讫13／群29／改密22／注销23／导出19／App8分别通过。独立认证09 exit1保留，后续通过未完成根因定位；不据其它组通过遮蔽 |
| 共享成品与最新类型 | [生产包界面11](./Web逐存储收讫/11-生产包界面执行.json)6检查通过，实际加载02私有成品；后续仅收讫声明旧7／新9兼容增量，[12类型](./Web逐存储收讫/12-声明兼容类型执行.json)与[14构建](./Web逐存储收讫/14-声明兼容构建执行.json)exit0，82输入前后不变。增量只定向重跑收讫13，不追认整站全部组当前通过 |

[脚本](../../../测试/前端/Web/migration.cjs/migration.cjs)使用实际Chrome／React／IndexedDB／WebLocks／WebCrypto，HTTP／身份／剪贴板为受控夹具。forward仅持久零正文与普通textDigest，读取实时GET精确核对；后续global即时移除RAM，普通已观测权限变化和5秒纯GET刷新清旧RAM。已报告状态只查已有本地事实，服务端不会再交付key／capsule。

```shell
node 代码/前端/Web/node_modules/typescript/bin/tsc --noEmit -p 代码/前端/Web/tsconfig.json
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_MIGRATION_REPORT=/本轮未使用的新目录 \
node 测试/前端/Web/migration.cjs/migration.cjs
```

真实HTTPS双device、后端随机PG、文件实际保存与服务端原件抹除分别独立验证；本片不以本地HTTP夹具替代。当前私有构建与共享业务回归范围见[收讫阶段记录](./Web逐存储收讫.md)，独立认证失败保留；若新增源码改变当前候选，追加当轮证据，不覆盖历史输入。

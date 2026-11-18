# JobsIM 工作树搬迁与原实例接续

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [前言](#%E5%89%8D%E8%A8%80)
- [一、已确认问题与修复](#jobs-toc-heading-2)
- [二、验证和当前限制](#jobs-toc-heading-3)
- [三、Xcode 与本地 Pods](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

## <span id="前言">前言</span>

2026-11-17 约08:55，工作树从 `/Users/jobs/Documents/GitHub/JobsIM` 整体移入 `/Users/jobs/Documents/GitHub/JobsProds/JobsIM`。本任务未移动或删除仓库，也未重建旧目录；沿实际新位置继续。103份iOS冻结输入在新根全部存在，除已知两份迁移Transport增量，其余与78项原生测试候选一致。冻结报告内旧绝对路径属于历史，不重写原件。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、已确认问题与修复 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- 运行中原实例的健康与bootstrap仍为200，但Web静态目录参数指向旧源码路径，首页404；必须切到新路径才能恢复完整入口。
- 原默认实例编号由仓库绝对路径派生。直接从新根运行会选择另一编号，不能接续原部署、CA与数据库。四份MacOS／Linux运行和停止入口增加本机 `.jobsim-local/instance.ref`，只记录公开实例ID、部署ID与当前uid，已Git忽略，不进入分发或克隆。
- 引用必须本人所有、普通文件、目录700／文件600，且格式和ID严格验证。只有原目录确实不存在、实例owner uid匹配、已存实例deployment匹配引用时，才提出接续；原目录仍存在、引用跨部署、权限不安全均拒绝。
- check／plan／status保持只读。run／stop在明确执行确认和实例锁之后原子更新路径owner，保存精确旧owner原件；实例身份、CA、数据库与秘密不改。新的默认实例成功启动后登记本机引用；显式其他state不借默认引用接管。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、验证和当前限制 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[可重跑测试](../../../../测试/部署/MacOS/工作树搬迁.py/工作树搬迁.py)只在临时目录工作。两份MacOS入口共16检查通过：只读不改owner、无锁拒变更、保部署材料、旧根仍存在拒接管、跨部署拒绝、不安全引用拒绝，并覆盖引用所指state缺失不生成替代身份／同路径部署漂移拒绝。四份Unix入口语法检查通过，前后源SHA相同，见[第五轮结果](./第五轮Shell/结果.json)。Linux仅语法，未原生运行；Windows未加入该引用协议。

原a4实例先登记公开本机引用，status只读匹配且owner SHA不变；随后用新根原run入口明确执行，先加密备份并解密验真，再原子接续owner、重建当前Go／Web并启动原双PG／NATS／九角色。部署ID与CA指纹均保持原值。健康、bootstrap、首页、JS及CSS经严格原CA验证全部200，见[接续成功](./原入口接续成功.json)。运行Web改为实例内私有只读资产目录，源码后续移动不再改变已启动首页的读取位置；64份实际编译Web源码从sourceMap独立核对无差异。

[重启前Go纯回归](./重启前Go纯回归/结果.json)71顶层通过、115项按各显式数据库环境跳过、0失败，全部输入前后相同；数据库专项另有迁移10真实PG通过，不把115跳过称通过。原默认实例未造测试账号／消息，保持运行；独立Canonical测试实例此前已正常停止且数据保留。过程中磁盘曾跌破4GiB，重型工作暂停；空间自然恢复后才继续，未清理任何缓存或数据。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、Xcode 与本地 Pods <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

搬迁后本地Pod默认相对层级也失效，首次构建在Jobsl10n／JobsSwiftTimer隐私资源处停止。[工程Podfile](../../../../代码/前端/iOS/Podfile)现从自身目录的祖先链寻找同族Swift基础工程；多份候选拒绝猜测，明确的 `JOBS_SWIFT_TEMPLATE` 优先。必要的 `pod install --no-repo-update` 已执行，46依赖的版本和spec校验和与原锁一致，只更新外部源相对路径和Podfile校验；Pods根仍为PBXProject，Storage／Transport等targets及两红钻Ruby引用完整。未手改生成Pods或上游源码。修正后完整应用／测试包实际编译通过，后续96项候选另记。

已实际打开新根JobsIM工作区并点击两个Behaviors菜单，两者均启动对应可执行脚本，且传入该工作区 `--xcode-path`，见[Install](./Xcode菜单Install.json)／[Update](./Xcode菜单Update.json)。确认前取消本次验证进程；菜单路径已实证，未借Pod Update升级依赖。Terminal界面操作被工具限制，交接核对采用本次自有进程／当前工作区实参，不称已完成交互式完整安装流程。

<a id="🔚" href="#前言">返回前言</a>

# JobsIM iOS 逐物理库收讫

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前实现与边界](#jobs-toc-heading-1)
- [二、明确操作与原责任](#jobs-toc-heading-2)
- [三、合格原字节与普通迁移](#jobs-toc-heading-3)
- [四、验证与未验](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前实现与边界</span><!-- jobs-toc:heading-end -->

设置→本机收讫资格已接[合格收讫合同](../../../代码/后端/Go/接口契约/合格收讫/README.md)。本端实际耐久、部署精确批准、服务器运行保护、每条消息／对象已成事实和原范围完成分别判断；旧bootstrap／SyncContext仍为transport_only，不依据其顶层false否定新协商，也不把server ready当已保存事实。默认保留cloud_history，不自动发布删除策略。

本片最终真实原生122项全部通过，0失败／0跳过，47秒；126输入及App／main／debug／test／fixture／xctestrun产物跑前后SHA全部一致。首轮111通过／11失败和合同修正轮112通过／10失败原件保留：前者修正十进制guard误作UUID的生产校验，后者仅补夹具缺失的当前permission checkpoint并验证异permission零落盘／ACK，未放松生产身份门槛。真实HTTPS新收讫、系统UI／后台、真机及断电验证仍按下文单列待核。

| 代码 | 入口／默认／持久化／生命周期 |
| --- | --- |
| [IMReceiptProtocol.swift](../../../代码/前端/iOS/JobsIM/Services/IMReceiptService/IMReceiptProtocol.swift) | 当前物理库上下文、保护七项、精确profiles、rc1／ro1原DG与manifest成对、原body白名单及已成结果；原JSON字节校验，不借跨语言重编码猜摘要 |
| [IMReceiptService.swift](../../../代码/前端/iOS/JobsIM/Services/IMReceiptService/IMReceiptService.swift)／[IMReceiptBuild.swift](../../../代码/前端/iOS/JobsIM/Services/IMReceiptService/IMReceiptBuild.swift) | 当前App代码集合摘要→用户明确实测本库→匹配批准→明确声明；正文与原件独立阶段；每次await前后核family／namespace／store／epoch，未知先原结果，404不自动换nonce／proof |
| [IMReceiptCoordinator.swift](../../../代码/前端/iOS/JobsIM/Services/IMReceiptService/IMReceiptCoordinator.swift) | 默认关闭、用户明确启用，仅本进程原family／namespace／store的RAM同意；同步通知与3秒补查，正文自动走完整本端保存／ACK，原件仍需明确下载 |
| [IMReceiptVC.swift](../../../代码/前端/iOS/JobsIM/UI/IMReceiptVC/IMReceiptVC.swift)／[设置](../../../代码/前端/iOS/JobsIM/UI/IMSettingsVC/IMSettingsVC.swift) | 独立验证／声明、当前资格／服务器保护、原单只查／明确同体重试、正文选择与完整原件、自动协调状态；十语、三主题、空态／重载／分页 |
| [IMAPIClient.swift](../../../代码/前端/iOS/JobsByPods/JobsIMTransport/Core/IMAPIClient/IMAPIClient.swift)／[IMInstanceAgent.swift](../../../代码/前端/iOS/JobsByPods/JobsIMTransport/Core/IMInstanceAgent/IMInstanceAgent.swift) | 新增独立receipt固定路由；HTTPS／实例CA／原store头、无Cookie／redirect、取消、32KiB请求、2MiB JSON与64MiB原件上限；原migration传输逻辑保留 |
| [IMLocalStore.swift](../../../代码/前端/iOS/JobsByPods/JobsIMStorage/Core/IMLocalStore/IMLocalStore.swift) | 主代理维护的FULL原正文＋原意图、原件完整SHA／文件与目录fsync＋描述／原意图、物理库／inode／member／global门禁；跨SQL与文件清理责任可恢复，不先unlink后冒充SQL成功 |

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、明确操作与原责任</span><!-- jobs-toc:heading-end -->

验证实际测量当前App主Mach-O、自有debug.dylib、嵌入framework及其余bundle内Mach-O代码集合，以排序相对路径、长度和逐文件SHA构造带域摘要；拒绝symlink、越界、未知代码来源及超限读取，不只测启动器。当前真实SQLite路径由主代理probe检查WAL／FULL／fullfsync／checkpoint_fullfsync、FULL提交后第二连接原bytes，原件合同另测完整写入、文件与父目录fsync、重开SHA。验证报告本身不证明真实断电安全，也不写入服务端批准；原build／report／contract须与部署profiles逐项相同，用户才可明确声明。不能手填或复制服务器hash冒充本端通过。

报告只在本进程内存，绑定实际build／store／epoch与时效。测量时guard只作审计，正常同步推进不会使同库报告失效；每次正文／原件／声明阶段仍以最新完整guard进入FULL CAS，物理库／inode／epoch、所见CV／成员期及global改变继续拒绝。App进入后台销毁页内Service与报告，普通重载可重新查批准但不自动声明。

原声明保存在当前namespace的`receipt-declarations`七字段元数据，原body固定六字段。历史结果接受原七字段，或成对新增`qualified`／`objectQualified`的九字段；新增字段须严格bool且与合同一致，未知字段与缺对拒绝，落库统一不可变七字段fact。变化的current／storage响应和历史快照flag都不覆盖当前资格。正文／对象原意图固定二十字段，在`receipt-intents`保存原nonce、family、完整typed tuple和原7／10字段body；rc1／ro1随原回执意图耐久。用户密码、OTP和认证凭据不进入这些表。

正文原`bodyEnvelope`／SHA与意图同FULL事务存`receipt-bodies`，完成后才可发送；原件先完整字节核对，经主代理私有文件／目录fsync，再将描述与原对象意图同FULL提交。持久化不接收任意路径。只查结果永不变成创建POST；明确重试沿原nonce／原body／proof，旧family只能纯查。已过期、撤权或global原单保留查询责任，不能重签成新原单。自己已记录不表示全人类或全设备范围完成。

自动正文协调器退出页面后可继续运行。它仅处理用户启用后的新ID／CV，并轮转有界补查已有候选：只有当前本库原冻结target且未contentRecorded才补入，绑定所见QID／DG／manifest，不能用新资格加入旧range。已有prepared／unknown只纯查，不自动重投；失败行冷却并轮转，不阻塞其他条目。后台取消工作并清RAM验证，前台同原family重新实际probe／匹配profiles，不自动PUT声明；退出、换身份／store／epoch或进程结束不继承同意。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、合格原字节与普通迁移</span><!-- jobs-toc:heading-end -->

[迁移Service](../../../代码/前端/iOS/JobsIM/Services/IMMigrationService/IMMigrationService.swift)新增从本库已completed的合格content receipt及原raw envelope读取来源。仅在当前GET明确`deleteScope=cloud_only`时使用，403／404／未知不冒充cloud_only。向receipt-targets显式携原历史QID，重核messageCurrent、原contentRecorded／CV／MG／DG／manifest／原SHA；不强求仍在旧清理targets或当前profile仍获批准，可靠历史事实不被当成新资格。

full模式还须原parent／object receipt已完成、objectRecorded、长度／SHA及安全原件文件，缺任一项整批拒绝，不降级metadata-only。无DG的legacy只走原完整permission hash仍相同的明确旧合同，不造DG。active或owner_unavailable的合法历史读不因send readOnly被误拒，dissolved与当前失权仍拒。global先清raw／文件，迟到源不能复活；产物和公开manifest不携带原收讫资格。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、验证与未验</span><!-- jobs-toc:heading-end -->

新Service／Protocol／Coordinator／VC、传输与测试源码均`swiftc -frontend -parse` exit0；pbx及十语strings的plutil通过。语言[首增35键](./iOS逐存储收讫/十语旧键保留.json)和[自动协调再增7键](./iOS逐存储收讫/自动收讫十语增量.json)分别保留旧字节前缀，每种现360键。

[IMReceiptServiceTests.swift](../../../测试/前端/iOS/IMReceiptServiceTests.swift)现14方法：原12项覆盖批准不替本测、真实file probe／明确声明／正文FULL先保存、未知只查、DG缺对、后继family、毒body、普通sync推进不失效报告、历史qualified cloud_only原bytes、full缺原件证明、冷启原target补查、自动未知／family、pause／resume；另两项代码集合摘要反例证明debug／Storage代码改变会改变build摘要，并拒绝坏代码／外部symlink。HTTP与前后台状态是明确夹具，SQLite与probe为真实原生路径。主代理[IMReceiptStorageTests.swift](../../../测试/前端/iOS/IMReceiptStorageTests.swift)8项覆盖跨介质失败／重开、global与后继inode等。

| 实际轮次 | 结果与限制 |
| --- | --- |
| 120候选首次编译 | 新设备build-for-testing exit65，首个真实类型错误为counter的UInt64返回值未转String；已修。此前旧设备被外部移除的exit70未进入编译，不算代码失败 |
| [122项首轮](./iOS收讫证据/122项实际结果.json) | 111通过／11失败／0跳过；原基线78、迁移协议／Store12、迁移Service10、收讫Storage8、代码集合摘要2及批准不替本测1项通过；另11项在verify／qualify触发MIGRATION_INVALID |
| [122合同修正轮](./iOS收讫证据/122项合同修正实际结果.json) | build-for-testing exit0，113秒、126输入零变化；实际方法112通过／10失败／0跳过。日志中的11 failures是断言数，不能当11方法失败。声明7／9和guard已能继续，失败集中在夹具未登记permission checkpoint导致Store invalidIdentity；未放松生产身份门槛 |
| [122最终权限夹具修正轮](./iOS收讫证据/122项权限夹具修正实际结果.json) | 实际122通过／0失败／0跳过，xcode exit0、47秒；126输入及应用／测试相关产物SHA零变化。测试注册当前permission hash与rc1一致，已有正例审计最终intent同当前checkpoint；原缺manifest方法另循环异permission，零raw／intent／ACK断言通过；未改Store门禁 |

最终122＝原基线78＋迁移协议／Store12＋迁移Service10＋收讫Storage8＋收讫Service14，全部实际执行，不把断言数当方法数相加；原99轮98通过／1夹具失败和所有旧证据保留。本轮Service HTTP与前后台是明确夹具，真实SQLite／CryptoKit／文件probe已执行；不能外推真实后台UI／系统文件选择器／真实HTTPS新收讫／真机或断电安全。未清理用户资料、第三方或历史证据，未提交／推送。

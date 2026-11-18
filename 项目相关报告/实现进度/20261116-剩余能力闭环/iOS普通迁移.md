# JobsIM iOS 普通迁移

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、源码与入口](#jobs-toc-heading-1)
- [二、验证边界](#jobs-toc-heading-2)
- [三、剩余收口](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、源码与入口</span><!-- jobs-toc:heading-end -->

设置→普通聊天迁移接入目标配对、源已确认内容／原件模式选择、加密文件队列、原单查询／明确同体重试、目标整批导入及独立档案／正文／原件读取。十语完整继承现有主题与字号，动态列表有空态、重载和更多入口。源码已完成，真实原生验证按下面边界分别记录。

| 文件 | 责任与生命周期 |
| --- | --- |
| [IMMigrationService.swift](../../../代码/前端/iOS/JobsIM/Services/IMMigrationService/IMMigrationService.swift) | 当前namespace／family／registered physical store，非秘密原nonce及所见选择先FULL CAS；未知先原query，旧family／epoch／store只读；源GET原JSON白名单与稳定普通摘要、原件完整字节 |
| [IMMigrationVC.swift](../../../代码/前端/iOS/JobsIM/UI/IMMigrationVC/IMMigrationVC.swift)／[设置](../../../代码/前端/iOS/JobsIM/UI/IMSettingsVC/IMSettingsVC.swift) | Jobs工厂／DSL／SnapKit，配对／文件／原责任／档案；提交后清输入，后台和离开取消任务、退休旧生命周期，前台重建当前服务 |
| [IMMigrationProtocol.swift](../../../代码/前端/iOS/JobsIM/Services/IMMigrationService/IMMigrationProtocol.swift)／[IMMigrationJSON.swift](../../../代码/前端/iOS/JobsIM/Services/IMMigrationService/IMMigrationJSON.swift) | 主代理维护的严格DTO、原manifest与原message JSON片段SHA、重复key、AES-GCM和整批原件校验；客户端不重编码猜Go摘要 |
| [IMAPIClient.swift](../../../代码/前端/iOS/JobsByPods/JobsIMTransport/Core/IMAPIClient/IMAPIClient.swift)／专用transport | 主代理维护的严格HTTPS实例CA、固定路由、无Cookie／redirect、取消及64／128MiB界限，独立取钥 |
| [IMLocalStore.swift](../../../代码/前端/iOS/JobsByPods/JobsIMStorage/Core/IMLocalStore/IMLocalStore.swift) | 主代理维护的19字段原责任CAS、私有密文stage、完整同步guard／physical／membership／global的FULL原子archive；cursor／Outbox／资格不动 |

非秘密原责任位于真实user的`migration-protocol` metadata namespace、collection `migration-intents`。目标密文在当前真实namespace `migration-stages`，key／密码／OTP／proof永不入库。发布后独立`migration-archives`、`migration-messages:<id>`与`migration-objects:<id>`；成功同事务删stage，取消／退休只移除本stage，源与已有历史保留。管理员OTP只随明确本人授权调用，不新造或继承原收讫资格。

每次请求经过session.authorized并在调用前后重核原family／namespace／physical；取钥、解密之后再整批validate，解析短install token后无await直接FULL提交。原来源任一未知、截止、所见CV改变、本地guard／membership／global变化则整批拒绝。installed_reported只读本机已有原事实，不重新取key／capsule。引用历史的当前GET失败、失权／global／forwardUnavailable或不确定，会通过独立scope清除旧引用正文；普通owned内容不被此引用清理扩大。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、验证边界</span><!-- jobs-toc:heading-end -->

| 验证 | 当前事实 |
| --- | --- |
| 新Service／VC语法 | `xcrun swiftc -frontend -parse` exit0，4空格格式收口；语法不替代target类型或真实运行 |
| 工程与语言资源 | pbx与十语strings的plutil全部通过；每种旧277键原字节保留，新增41键，[记录](./iOS普通迁移/十语旧键保留.json) |
| App及原10个迁移候选编译 | 主代理确认修正搬迁后的本地Pod相对路径并实际`pod install --no-repo-update`后，完整build-for-testing exit0；原46依赖版本／SpecChecksums不变。第一次仅因旧Pod路径失败，未执行到Service类型，不记为迁移代码失败 |
| 首轮96实际执行 | [结果](./iOS普通迁移/96项实际失败结果.json)：89通过／7失败／0跳过。7项均在Service夹具构造处因refreshGeneration=0触发INVALID_AUTH_PAIR；原78、协议／Store10和Service的NFC证明方法通过。选择器首轮未命中属无效执行，保留[记录](./iOS普通迁移/96项选择器未命中.json)，不作通过 |
| Service夹具修正 | [IMMigrationServiceTests.swift](../../../测试/前端/iOS/IMMigrationServiceTests.swift)只把夹具refreshGeneration改为合法正数1，保留生产认证校验和原8方法断言；已随下轮99实际执行验证原8项通过 |
| 纯协议／Store | 主代理维护[IMMigrationTests.swift](../../../测试/前端/iOS/IMMigrationTests.swift)，首轮10方法已实际通过；只读续查另发现真实Go RFC3339Nano过期时间不能由Storage默认日期器解析，主代理补fraction／plain解析和第11个stage→AES-GCM→原子发布反例，已随下轮99实际执行通过。原10项使用无fraction夹具不足以证明此路径 |
| 第二轮99实际执行 | [结果](./iOS普通迁移/99项实际结果.json)98通过／1失败／0跳过，输入跑前后不变；原8Service与11协议／Store包括真实Go九位fraction expiry通过。新增global竞争夹具用普通save时forward被现有保护先清，GET读数0，因而没有触发race；第二条redact也同样未真正入流，不能据其pass称竞争已验 |
| global补片实际执行 | Service最后重读独立holder和global，VC投影通知先清RAM、revision拒迟到回填、忙时完成后重载；文件选择器身份不匹配清原文件名与owner。两条竞争夹具改经真实FULL commitMigrationArchive安装，并强制先有正文、GET读数1。已随[122首轮](./iOS收讫证据/122项实际结果.json)真实执行，10个Service方法全部通过；旧99夹具失败不删除 |
| 迁移当前原生范围 | 同轮协议／Store12方法全部通过，含Go九位fraction expiry、stage／AES-GCM／原子发布及owner_unavailable合法历史读。该122轮整体111通过／11失败／0跳过，11失败属于新增收讫Service的guard审计校验，不能把迁移22项通过称整轮全绿 |
| 最终完整原生执行 | [122权限夹具修正轮](./iOS收讫证据/122项权限夹具修正实际结果.json)122通过／0失败／0跳过，47秒，126输入与应用／测试产物零SHA漂移。迁移协议／Store12＋Service10保持通过，新增收讫Service中的两条qualified cloud_only来源反例亦通过；真实HTTPS及UI仍独立待核 |

上述迁移Service候选用`IMSession(store: IMLocalStore(path: ":memory:"))`，不恢复共享Keychain、不连接默认a4或用户库。新增合格cloud_only来源桥另从真实私有file SQLite原receipt与raw envelope取源，只有明确cloud_only才沿原历史QID核messageCurrent、DG／manifest／MG及已成事实；full缺原对象证明整批拒绝，不重编码猜原SHA。这两条新增收讫Service反例已在最终122轮实际通过，HTTP与服务端已成事实是明确夹具，不能据其证明真实双端服务链，详[收讫记录](./iOS逐存储收讫.md)。

其余真实Canonical realm、本机Files实际保存、界面动态布局／主题、后台及真机分别另验；此片没有运行任何新账号创建或重启实例，不把旧78项或Web33组当iOS迁移执行。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、剩余收口</span><!-- jobs-toc:heading-end -->

待新增Service target增量构建、原生测试结果和精确输入／产物冻结；界面与系统文件交互仍需单独真实操作验证。中央资产表由主代理统一更新，不在并行代理间覆盖；没有提交或推送。

# <span id="前言">CLI 同实例普通迁移</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、入口与行为](#cli-migration-%E4%B8%80)
- [二、存储与生命周期](#cli-migration-%E4%BA%8C)
- [三、验证状态](#cli-migration-%E4%B8%89)

</details>
<!-- jobs-toc:end -->

---

## 一、<span id="cli-migration-一">入口与行为</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

登录后使用当前已登记的 personal 物理库；不会创建空同步库或继承其他设备的资格。默认保留源，不提交收讫、不改同步游标、不迁待发消息。目标保存到独立迁移历史。

| 命令 | 行为 |
| --- | --- |
| `migration-capabilities` | 只读核对当前实例合同与上限 |
| `migration-pair` | 目标先保存原requestId，再输入当前密码及管理员可选OTP；proof只在本次终端交接 |
| `migration-create SELECTION_JSON_FILE` | 源显式选择原messageId／contentVersion／objectMode；当前GET核对正文，full读取完整原件；proof、密码、OTP不进入SQLite |
| `migration-import MIGRATION_ID` | 目标核原manifest字节SHA／上下文、暂存加密capsule，独立取钥及AES-GCM验证；最终validate后同FULL事务发布整批历史，再报installed |
| `migration-list`／`migration-status REQUEST_ID` | 列非秘密原单，沿原结果纯查；不会发替代请求 |
| `migration-retry REQUEST_ID` | 显式沿原nonce／目标／选择／源摘要续作；本地已提交后只查原状态，不重新取钥 |
| `migration-cancel REQUEST_ID` | 沿原迁移及manifest取消，退出加密stage，源及既成历史保留 |
| `migration-history ARCHIVE_ID` | 独立历史；转发每次显示前当前GET，未知或撤权清本地投影，最终重读global及原行围栏 |
| `migration-file ARCHIVE_ID OBJECT_ID NEW_DESTINATION` | 校完整原件长度及SHA后独占创建用户指定文件，拒覆盖；已交付到用户目录的文件不作远程追回承诺 |

选择文件仅为数组，例如`[{"messageId":"22222222-2222-4222-8222-222222222222","contentVersion":"1","objectMode":"none"}]`。每包最多256消息／64原件／128MiB，每原件最多64MiB；只接受受控HTTPS，不执行包内容。

## 二、<span id="cli-migration-二">存储与生命周期</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

`migration_intents`保原nonce、当前family／namespace／物理库、目标和选择／输入摘要，不保存密码、proof或密钥。`migration_stages`仅保密文，关联消息清单让global整包清stage。`migration_archives`只保公开清单；正文／原件分别在独立表，global精确清除，已完成注销的全库擦除覆盖新增表。目标同事务比较同步guard、成员代际、已见global及较新本地版本，失败整批回滚。原件与正文保原ID、contentVersion和creation tuple，不制造新消息／ACK。

普通available来源及cloud_only原qualified来源均已接入当前候选。fresh GET必须明确cloud_only，再纯查本库原content／object收讫结果，核原qualifiedAtCommit事实、QID历史声明、成员代际、DG／manifest及messageCurrent，用先前同FULL事务保存的精确原bodyEnvelope与原contentReceiptId；full另沿原objectReceiptId从同一个已完整核SHA的打开文件读取。部署后来撤回批准不抹除历史合格事实，也不授予新ACK；legacy无DG还须原完整permission与当前合法cloud_only。不能用重编码JSON或普通缓存冒充原bodyDigest；无原资格事实继续保源阻断。

## 三、<span id="cli-migration-三">验证状态</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

当前[52输入候选](./逐store收讫证据/CLI-升级门禁最终-20261117-142600/编译结果.json)已完成编译与[完整本地执行](./逐store收讫证据/CLI-升级门禁最终-20261117-142600/完整本地结果.json)：135 PASS／0 FAIL／2环境SKIP，跑前后52项Go源码／外置测试及锁文件SHA一致。迁移10个顶层方法和逐store收讫12方法均包含在135内，不与旧115方法或其他轮相加；cloud_only精确原Envelope／receipt来源由收讫专项覆盖。

[迁移10方法](../../../测试/CLI/migration_test.go)全部通过，覆盖原字节／完整原件、跨目标／AAD／GCM篡改、未知字段／重复键、global与迟到安装、guard／过期／成员代际／新版本整批回滚、原nonce／family不可重基、转发副本清理、owner_unavailable合法只读历史、零字节完整原件及双SQLite连接中原件发布与global写入串行。SQLite和文件操作真实执行，HTTP为注入夹具；真实HTTPS双设备迁移、系统秘密库正向及Windows／Linux运行仍待验。2项跳过分别为未启专用真实HTTPS协调器环境和未启原生系统秘密库诊断。

[实际MacOS arm64可执行候选](./逐store收讫证据/CLI-升级门禁最终-20261117-142600/jobsim-darwin-arm64)为Mach-O 64-bit arm64，SHA256为`e17f5340cfabe6b5b3e4ced7ab3100107f594a8c155bc9131066af1c5e7e2c25`，`--help`实际退出0且未发网络请求；[构建与代码／锁文件摘要](./逐store收讫证据/CLI-升级门禁最终-20261117-142600/可执行构建结果.json)独立保存。该产品与测试二进制SHA`7e10c4f003c2d2007d59b13377f527a70dc3d1d1a42dedc5bbeb792c3f406ba6`分别登记，不能混用；产品源码与锁文件集合SHA为`e3f70ca8aca35763dbee116e7b1154a08d718aa14f4a11603b13f7300193e114`。本地可执行候选不等于签名发行包或生产资格批准。

旧[49输入只编译检查点](./逐store收讫证据/CLI候选编译-结果.json)及首轮[133 PASS／1 FAIL／2 SKIP](./逐store收讫证据/CLI-完整候选-20261117-130619/完整本地结果.json)保留；失败为global测试夹具在合并SQL中复用位置参数，拆开Exec并增加实际messageId读回断言后，窄global及完整回归通过。未放宽生产global删除条件或零残留断言。后续134通过检查点与最终135轮分别封存，详见[逐store报告](./逐store合格收讫.md)和[最终源码／产物映射](./逐store收讫证据/最终已验源码与可执行候选-142600.json)。

[返回进度](./README.md) · [后端普通迁移合同](../../../代码/后端/Go/接口契约/普通迁移/README.md)

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>

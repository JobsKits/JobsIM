# Android／HarmonyOS 普通个人历史迁移

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [当前状态](#jobs-toc-heading-1)
- [协议与原责任](#jobs-toc-heading-2)
- [正文、原件与生命周期](#jobs-toc-heading-3)
- [构建修复与产物](#jobs-toc-heading-4)
- [真实缺陷与本机闭环](#jobs-toc-heading-5)
- [集中待验清单](#jobs-toc-heading-6)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">当前状态</span><!-- jobs-toc:heading-end -->

两端已新增真实协议服务、隔离存储和产品入口：设置／当前会话→普通历史迁移→另一设备配对→选当前会话消息→明确发送→目标独立验证口令并解密→最终validate→整批本地事务→原事实报告。归档显示实际正文、时间和原件保存按钮；附件元数据与不可用引用有独立说明。40键×10语言继承现有白天／黑夜／系统主题，三类动态列表有空态与重载、每页20项。

当前最终Android候选为150输入、11套54 JVM通过及双APK构建；独立自有API37 arm64模拟器真实执行当前102本地方法：86通过／16明确Assume跳过／0失败；同包另3项生产HTTPS通过，105方法逐一核齐、无重复或缺项。[本机逐方法证据](./普通迁移两端证据/Android独立AVD-20261117-153544-d52437/最终105逐方法实际核对.json)与[四包完整复核](./普通迁移两端证据/最终移动端存储修复候选完整复核.json)保留实际范围。Harmony迁移22／收讫23／同步36项host通过，70输入应用及63方法test HAP编译成功、ABC核名63／63；未鸿蒙设备执行。旧首次33 JVM、首构建错误和各中间包保历史，不累加为当前通过。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">协议与原责任</span><!-- jobs-toc:heading-end -->

依据[普通迁移合同](../../../代码/后端/Go/接口契约/普通迁移/README.md)。先登记原sync physical store，再使用原store header。原nonce／本人／device／store／epoch／family、原选择ID／CV／附件模式、源摘要和目标公开元组先耐久；密码／OTP／配对proof／AES key不入Room／RDB／日志。第一次及未知都先查原results；not_found不自动创建，只由原family显式重试原body。旧family／device／store责任可读，不借后继发写请求。

接收严格核原manifestEnvelope字节SHA、scope／user／personal／两端device-store／epoch、对象关系与计数、capsule SHA及JIMMIGR1／nonce／AES256-GCM AAD。primaryDigest对解密UTF8中完整Go JSON对象的原片段计算，保留字段序、HTML转义、时间精度和十进制字符串，不重编码Go正文。5秒mi1绑定本机当前store／epoch／digest；最终本地FULL事务再次核guard、auth生命周期、global floor、原创建元组，原件先校字节／SHA并文件及目录fsync。旧较新正文不被覆盖；cursor／Outbox／资格／ACK保持独立。

`installed_reported`仅报告本机事实；原localCommitted＋同digest存在时不再索取已停止交付的key。报告未知保留本机事实，通过原ID继续查。成功安装后删除隔离capsule及未引用对象，保留独立历史及其已索引原件。

普通available消息使用fresh GET重核。lawful cloud_only来源已接本库先前完整合格receipt-bodies原bodyEnvelope＋sourceReceiptId；full还须同parent已成object ACK、原完整文件及sourceObjectReceiptId。源发送先纯查原receipt result，再按原QID查询目标，核原DG／manifest和messageCurrent，不因旧manifest自然过期或originalTarget=false拒合法历史；无DG旧事实仍须完整permission hash一致及fresh GET明确cloud_only。没有本库原合格事实时保源阻断，不能重编码Message子集碰旧摘要。逐store收讫独立测量与批准门禁见[限定报告](./Android-HarmonyOS逐store收讫.md)，本任务未给生产实例代授native资格，也不将迁入历史升级为合格目标。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">正文、原件与生命周期</span><!-- jobs-toc:heading-end -->

Android为`MigrationProtocol.kt`、`MigrationStore.kt`、`MigrationService.kt`、`MigrationScreen.kt`、`MigrationText.kt`，ChatModel与MainActivity仅入口／当前身份工厂；原件在noBackup的Migrations／namespace散列目录。Harmony为Data/MigrationProtocol、MigrationStore、MigrationService、MigrationNative、MigrationText与Index迁移段；RDB五独立表、filesDir/personal-migrations下原namespace隔离。Android namespace的epoch位于第7段，Harmony位于第5段，各自严格验证本端布局。

历史跨同本人空间按消息ID／原创建epoch及sequence去重，保较新CV。完整原件读前再核本机global／当前store与SHA；系统保存选择器返回后重建当前服务并精确比原family／device／store，不借后继身份写用户文件。外部显式保存文件不纳入应用远程擦除承诺。

转发值只作引用投影：每次加载先fresh GET对原消息元组／文字／available状态核验；未知仅显示零正文不可用占位，明确撤权／global／不存在清本机副本与索引。global还保守清所有派生forward归档，源ID不公开时不把其当ownedText。每5秒重载，后台／离页先清秘密与UI正文并取消当前网络；Android重计算与文件IO离开主线程。旧原件替换以同事务先落cleanup责任再删索引，冷启动正常cleanup可续；最终本人注销精确纳入本模块所有epoch私有表／文件，保其他身份与外部文件。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">构建修复与产物</span><!-- jobs-toc:heading-end -->

Harmony首轮保留失败日志：三处ArkTS `delete`不支持、一处可空state传给includes。改为显式白名单投影及空值兜底后，应用与同快照测试HAP均BUILD SUCCESSFUL；384处应用警告与47处测试警告保留于原始日志，主要为既有deprecated／may-throw提示。两包unsigned，未安装或执行Hypium。

| 产物 | SHA256 |
| --- | --- |
| Android应用APK | `9660aa3856ceb28d06ac1212ca1a82a54e4c0fbcd272b82f9440c503c7e45c45` |
| Android测试APK | `7b3f14b431e42a5745fbe3b2f269cfb34aba9dfb573847223a1d257e1b010e04` |
| Harmony应用HAP | `5ecddaa4e818d55fa101bd755e97a65fa2323621acdc4660fee6fb4aa0c6346e` |
| Harmony测试HAP | `7f71c4d5dc186f1aa47379b0a1251fe8c2f6f42a61e065106c9d2ca128e80c74` |

最终新HAP位于`普通迁移两端证据/Harmony移动端存储修复-20261117-161426-default`与`Harmony移动端存储修复-20261117-161426-ohosTest`，70项source／build／current SHA逐项映射；仅测试相对import和私有快照Hypium本地绑定转译，生产源码不改路径。测试ABC含全部63方法名。原Dist未写入，旧包及失败证据保留。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">真实缺陷与本机闭环</span><!-- jobs-toc:heading-end -->

跨审有效复现迟到global：先真实安装forward holder及owned正文共2行，挂起唯一GET，调用生产eraseGlobal并确认holder为0，再释放原合法200；旧Service仍回填两条。修前19项中2项失败原件见`HarmonyHost-I8zkBO`，修后对所有await结束重新读当前Store、精确比原正文事实，拒global／deleted／较高CV；普通forward持久占位仍可用新的合法GET恢复。UI在已验证消息事务后立即清RAM／退休旧Service，早于cleanup await；不会仅凭身份未变发布旧正文。Android3个相同Room边界已真实通过。

Android旧冻结包全本地98方法曾78通过／4失败／16跳过：迁移3与注销1被系统提供的`/data/user/0`别名误当不安全子路径。现只对可信OS root先canonical，再保留生成子目录与文件的symlink拒绝；16项定向及最终当前102方法已真实复验，4个原红消除。原始日志不改、不用后轮绿色覆盖首失败。

移动端不再机械要求4GiB空闲。`NativeStorageBudget`按本次有界待写字节＋64MiB事务／回滚reserve判断；负数、非整数、溢出与超限拒绝，包128MiB／原件64MiB、SHA、FULL／Atomic／fsync及IO失败回滚不放宽。宿主新构建／VM的4GiB资源门禁继续保持。纯策略JVM／host反例覆盖不足4GiB但足够成功和确不足拒写。

真实HTTPS三方法为`nativeSelfStateAndCloudDraftControlsRenderAndOperateThroughHttps`、`realSchemaTwoReadViewPreferencesAndAtomicOriginalQueue`、`realDraftLossUsesReadonlyOriginalBodyAndRetiredSourceKeepsIndependentText`。使用独立Canonical55443、实例公开CA与自有emulator-5580，6个测试账号均已真实退出；不触默认a4／现有pixel5554。此轮HTTPS范围为S2／独立草稿／原生UI，不扩称迁移或合格收讫生产HTTPS。

## <span id="jobs-toc-heading-6" data-jobs-toc="v2">集中待验清单</span><!-- jobs-toc:heading-end -->

- Harmony设备cryptoFramework／RDB NAPI／文件目录fsync与Hypium实际执行仍待；Android已在独立AVD执行Room／文件和global等当前本机方法，物理真机、断电与系统保存选择器另验。
- 两端设备真实HTTPS配对、明确选消息及完整0字节普通原件、目标解密安装、可读历史／另存、forward即时重验证、多语言／三主题与生命周期。
- 本端逐store真实build测量／耐久探针与部署批准独立核验，不能用host／HAP构建产生qualified资格。Harmony需设备确认OS bundleCodeDir中的自身HAP可读，否则保持未资格。

未提交／推送，未启停默认a4；Canonical由主代理原入口提供并精确收尾，本片只用公开CA。自有5580已精确停止并保AVD数据，5554未触；未清缓存、第三方或用户数据。新构建均在源码外私有持久Build域完成。

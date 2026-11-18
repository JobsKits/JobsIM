# Android／HarmonyOS 逐物理库正文与对象收讫

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [当前实现与验证层次](#jobs-toc-heading-1)
- [不漂移的本机测量与批准](#jobs-toc-heading-2)
- [正文、原件和未知原单](#jobs-toc-heading-3)
- [正常收件协调](#jobs-toc-heading-4)
- [归档、迁移与清理](#jobs-toc-heading-5)
- [本机路径与空间修复](#jobs-toc-heading-6)
- [待验](#jobs-toc-heading-7)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">当前实现与验证层次</span><!-- jobs-toc:heading-end -->

两端新增可靠保存设置入口、真实本机探针、精确批准声明、独立正文／原件落盘和原回执意图、未知原结果、历史QID目标查询、可读归档与原件另存。28键×10语言沿现有三主题。默认不启用自动收讫、不自动探针或声明，不从服务端Profile列表复制出本机验证资格。

最终[四包与220输入复核](./普通迁移两端证据/最终移动端存储修复候选完整复核.json)：Android150输入、11套54 JVM通过，当前105方法全部逐项核齐，其中本地102为86通过／16明确Assume跳过、另3真实HTTPS通过，0失败；[完整仪器记录](./普通迁移两端证据/Android独立AVD-20261117-153544-d52437/最终105逐方法实际核对.json)不借旧包计数。新增Receipt4项在真实Room／BundledSQLite和Android文件系统执行：原字节及intent同FULL事务、重开、SQL失败回滚、0字节普通原件fd／目录fsync、对象提交失败只清新文件、global清理、实际本库probe与生成子目录symlink拒绝。真实HTTPS三项覆盖S2／草稿／原生UI，不冒称新receipt生产ACK已联调。

Harmony当前迁移22／收讫23／同步36与注销16项host按各自冻结输入通过；Python SQLite、文件／launch替身不是RDB NAPI或真实手机。最终70输入应用及63方法test HAP均编译成功、CRC正常、63／63方法名在ABC；两包unsigned，Hypium未执行。首轮4项ArkTS诊断、迟到global两个有效修前失败、Android4个首轮真实本机失败及测试导入编译失败全部保留。最终包摘要：Android应用`9660aa3856ceb28d06ac1212ca1a82a54e4c0fbcd272b82f9440c503c7e45c45`、测试`7b3f14b431e42a5745fbe3b2f269cfb34aba9dfb573847223a1d257e1b010e04`；Harmony应用`5ecddaa4e818d55fa101bd755e97a65fa2323621acdc4660fee6fb4aa0c6346e`、测试`7f71c4d5dc186f1aa47379b0a1251fe8c2f6f42a61e065106c9d2ca128e80c74`。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">不漂移的本机测量与批准</span><!-- jobs-toc:heading-end -->

依据[当前合格收讫合同](../../../代码/后端/Go/接口契约/合格收讫/README.md)。独立GET receipt-context使用当前原store header，旧bootstrap／sync/device／sync/context的transport_only与false保持原义。新context分开本库qualification、服务端readiness与after-body／after-object能力；读取Profile不创建资格。

探针报告只绑定profile／deployment／user／device／namespace／physical store／epoch／当前安装build和存储合同；不绑定每次sync变化的guard或family。单次动作仍核当前guard、原family和精确身份，不能借后继原请求发写。

Android流式测OS给的当前安装APK SHA；可信sourceDir别名可canonical，存在splitSourceDirs则保持未资格，不能仅测base批准未知代码；在实际Room writer核synchronous=FULL，同事务写两条公开探针body＋intent，从当前writer的PRAGMA database_list核OS数据库目录／受控库名，再用独立Room连接重开同一physical读回；原件探针还校完整字节SHA、文件fd.sync及目录fsync。Harmony只在OS bundleCodeDir中的唯一自身HAP实际可读时测SHA，不可读保持未资格；首次在当前RDB FULL事务保存公开探针body＋intent／已fsync原件，下一App launch才读回并生成报告，同launch只提示重启，不产生验证digest。主机用例以替身模拟launch，未称真实手机进程重启已验。

升级兼容已只读核源码并增加host反例：storage原结果旧7字段与新成对资格布尔9字段均可读；context与原results不要求旧服务端build等于当前进程。查询新批准Profile使用当前本机测量，不先要求旧storage已匹配新build；实际新ACK仍核当前精确资格。声明结果严格接受required原7字段、或加成对真bool的9字段；qualified须true、object位须与contract相同，未知／缺对／错误类型／错误合同均拒绝。首写和纯查均统一canonical7落库，历史原fact不借当前view复活资格。

本端测量通过后，只展示与本机build／verification／contract精确一致的部署批准项，由用户明确声明。原六字段与nonce先耐久，storage-results只查、未知不换单，批准撤回不恢复旧资格。生产实例未由本任务代为批准任何native profile。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">正文、原件和未知原单</span><!-- jobs-toc:heading-end -->

rc1／ro1先核实例／本人／device／store／epoch／qualification、原CV／正文摘要／权限、有效期；deliveryGeneration与manifestDigest必须成对保留。正文对原bodyEnvelope字节核SHA，不重编码后碰摘要。原完整正文与原nonce／proof／请求在同一FULL事务保存，成功后才能POST原回执。

原件与正文分开。必须有同原parent已成body ACK，完整下载独立原件并校size／SHA，文件和目录fsync后，描述及原object intent同FULL事务提交；完整0字节普通file按真实descriptor＋SHA(empty)处理，不按空数组判缺原件，不因此放宽图片／音频格式。

首次和冷恢复都先原results；明确404才由原family显式同体续跑。prepared依赖失败及未知保持责任，不自动换挑战、nonce、QID或内容。原结果只确认历史事实，targetRangeComplete／deletionEligible仍false；target接口按原QID查询独立累计body／object事实，不从本库一条ACK推断全部目标。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">正常收件协调</span><!-- jobs-toc:heading-end -->

用户明确开启当前namespace／family／store会话后，正常sync成功提交的primary正文ID／CV才进入协调器，forward／pending／frozen／unavailable排除；不开启不会因新协议自动发ACK。每轮最多处理8项，冷恢复重新开启后按本机已缓存候选有界查询原target，只有当前本库contentTarget且未contentRecorded才接正文流程，不借新QID加入旧冻结范围。候选游标仅用于本机分页，不作为ACK事实。

已有pending／unknown只查原结果，不自动重POST。原件仍需用户明确下载；不后台下载全部附件、不自动声明批准。后台／退出／换family／store取消任务；新单各自捕获最新guard。Android在SyncInstall提交后窄接Coordinator；Harmony用可选committed回调通知，收讫异常不阻断普通同步。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">归档、迁移与清理</span><!-- jobs-toc:heading-end -->

正文及原件可读归档均与普通同步cursor／Outbox独立。系统保存选择器返回后重新核原family／store和当前global，再读私有文件SHA，只有用户明确选择的外部副本可离开应用；外部文件不宣称可远程擦除。

普通迁移的lawful cloud_only来源必须使用本库先前完整合格receipt-bodies的原bodyEnvelope＋sourceReceiptId；full还需同parent已成object ACK、原完整字节及sourceObjectReceiptId。发送前纯查原receipt result及历史QID目标，核原DG／manifest和messageCurrent；旧manifest自然expiry的current=false不假判消息失效，不要求originalTarget。无DG旧事实只允许原完整permission hash仍相同，且fresh GET明确cloud_only路径。普通available仍fresh GET；没有原合格事实不能伪造证明。

global在事务中优先清正文／原件索引及原challenge，退休原责任，先落精确私有字节cleanup队列；晚回调不能恢复。最终本人注销将两端receipt与migration私有表／文件纳入原账号所有epoch清理，保其他profile／user和外部文件。只读群owner_unavailable不因canSend／readOnly被假拒；当前MG、global、dissolved和服务器最终validate仍独立核验。

## <span id="jobs-toc-heading-6" data-jobs-toc="v2">本机路径与空间修复</span><!-- jobs-toc:heading-end -->

可信OS noBackup root先canonical，生成的namespace目录及文件仍拒绝symlink。修复真实AVD中系统root别名导致迁移／注销误拒，Receipt root及清理同类路径同步规范，VoiceUpload既有canonical实现和清理调用保留。测量不再固定重开默认库，真实probe专门验证当前独立physical库；当前生产用户名／store的批准仍需各自实际测量和部署精确授权。

移动端写入只要求本次有界bytes＋64MiB事务／回滚reserve；校验无效／溢出需求和原件64MiB／包128MiB上限，FULL、fsync、SHA与写错回滚不放宽。元数据纯查不套4GiB门禁；宿主构建／集成的4GiB门禁保持。迟到global同时以最终Store原事实与UI已提交消息revision围栏拒回填，不把普通forward缓存占位当永久撤权。

## <span id="jobs-toc-heading-7" data-jobs-toc="v2">待验</span><!-- jobs-toc:heading-end -->

- 63方法Hypium已编译且ABC核名；其中新增6个SDK／协议／十语方法只在未来目标实际执行才算设备证据。
- 自建独立API37 arm64 AVD仅使用私有emulator-5580；当前102本地及3HTTPS完成后已精确停止、保数据，现有pixel5554未触。HDC仍无目标，未安装unsigned HAP。
- Harmony自身HAP实际可读性与真实冷进程探针需设备确认；不可读保持未资格，不以服务器批准列表替代本机执行测量。
- 新receipt／迁移生产HTTPS、真正批准后的自动收件／冷进程、系统保存选择器及全十语／三主题新页面仍待相应链路；Android原件fsync／目录及本库FULL／重开已有原生实际证据，物理断电与Harmony NAPI另验。

旧失败、旧Dist及全部历史报告保留；未提交／推送，未修改a4原身份／CA；Canonical由主代理提供并收尾，本片不发服务stop。

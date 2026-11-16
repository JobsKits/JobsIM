# <span id="前言">Android S2 本人状态与云草稿候选</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、实现范围](#jobs-toc-heading-1)
- [二、验证状态](#jobs-toc-heading-2)
- [三、最终同源与真实 HTTPS](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、实现范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

沿用 Kotlin／Compose／Room9／BundledSQLite。本人偏好、会话显示、读点及稀疏观察通过 S2 安装；schema1 原登记／游标独立归档，不把升级当新物理库。原始 envelope SHA、字符串计数、完整页＋final validation＋首次权限 changes、同 Room 事务与持久 guard 沿用。原消息同 clientID 仍不是 Outbox 证明，正式 Outbox 查单规则不改。

原生界面已接阅读与本机缓存计数分离、独立手动未读、置顶／静音／归档／隐藏及三档字号。云草稿另受 live `cloud_drafts` 门禁，独立文字与引用分开、原 CAS／da1／dr1 意图持久，显式载入不自动覆盖当前输入／发送。原操作未知先只读结果，不补新许可／成员快照／nonce。入口与生命周期详见[Android README](../../../../../代码/前端/Android/README.md)。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、验证状态 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

首轮[有界构建](./Build-20261116-133140-94446-94c3d2/构建结果.json)已通过，应用／androidTest APK 均编译成功，10项 JVM（6 Isolation＋4 SyncWire）分别0失败／错误／跳过，包含十语完整性。输入前后 SHA 一致，自有 no-daemon 已退出。产品 APK SHA256 为 `b1304104a56dde9e2254cbb79830c955c1939111fafca1e39fac5a855873fb1f`，测试 APK 为 `76794de7fdca26ba68066f6bf4c0c034417c6fb720f7ea415821f09f6cb456ff`；这还不是原生仪器或界面验证。

最新[Build-20261116-140306-18190-f588c8](./Build-20261116-140306-18190-f588c8/构建结果.json)含12个新原生反例，重新编译／10JVM通过，65输入前后SHA稳定，Room9与旧原件逐字节一致。产品APK SHA256 `a135ef54037e571f8903a0acc1821fe9e8aad1088212631872000a01d513df59`，测试APK `8a30ab5cf565be6c339cd75b28cdfb2762db24fe1fda60b7b51ac222eb00f808`。

[最终同包22项原生验证](./Instrument-20261116-141315-22176-ef9002/仪器结果.json)已22通过、0失败／跳过：原S1同步8＋原Api家族1＋V8→9迁移1＋新SelfState12。实际应用／测试包安装读回SHA与上列一致。新12项覆盖 S1→S2 物理登记／原事件位置隔离、错本人／非字符串计数回滚、未知原体冷重开、404不自动创建／显式原体重试、稀疏观察与独立手动未读、草稿两类原许可／Outbox不改、来源变化重选、身份与持久guard、同身份reset／首次启用、S2投影＋cursor同事务回滚、排队前identity捕获，以及retired不冒充已保存／独立文字显式恢复。

使用原`JobsIMReview_20261114_1424`／`emulator-5582`，启动PID21924；完成后保留数据、`no-snapshot-save`正常关闭，已确认退出。没有wipe／uninstall／clear，没有访问真实服务器。每方法使用独立UUID Room／profile夹具，不把原22与旧轮累计。

资源一度低于4GiB且临时文件创建失败；本端当时未启动构建／模拟器／API，未清除任何缓存、用户库、旧包或报告。空间恢复后经主流程协调才启动本轮受限编译；原始失败或后续结果各留独立目录。

以上是早期 a135 注入检查点；后续结果见下节。旧 e295 认证 APK、978 同步候选及原报告保持各自原始范围；不把限定验证当作完整 I、qualified ACK、after_receipt 或正式设备迁移完成。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、最终同源与真实 HTTPS <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[最终构建](./Build-20261116-155610-95376-8caaec/构建结果.json)通过，66项输入原件与当前源码逐字节 SHA 一致。产品 APK `5774b93a9c5c5d930cc682d91d55336addb414f53a8374cfb65dcd2106a6d13d`，测试 APK `7067f7b67dd39ae56ab536583489de98b4c55bb61c8365d2c39d0e3acd9be9cb`。[同包原生23项](./Instrument-20261116-155736-1436-dea38c/仪器结果.json)全部通过、0失败／跳过，组成是原10＋SelfState13，不与旧22累计。JVM为11项（Isolation6＋SyncWire5），实际执行原XML保存在 Build-20261116-151122-75356-d7ab8c；后续 Gradle UP-TO-DATE 复用了同一XML，不算新增执行。

[新 E2 实例真实 HTTPS 三项](./HTTPS-20261116-155802-1622-da2100/真实HTTPS结果.json)全部通过、0失败／跳过：实际 schema2 快照与本人 read／view／font、原完整草稿丢响应后冷 Room 只读查单、来源真实编辑导致 retired 但保独立文字，以及生产 Activity 原生控件的手动未读、云草稿保存／显式载入、字号／主题。六个本轮 family 均完成明确 logout＋原 AT 返回401，端口映射已撤；部署 `24fb0a3c-99a0-4ada-9d01-07467724e829`／epoch2，仅使用本轮新账号。只在客户端丢弃已收到的真实草稿响应，不声称真实网络丢包。

首轮真实1通过／2失败、后续2通过／1界面失败及原始日志均保留。首轮发现服务器合法 RFC3339 数值时区被客户端误拒，产品仅修严格 offset→Instant 解析，保留原JSON字符串与原冻结body，写出的静音时间仍UTC/Z；新增JVM与原生反例后重跑23＋真实3。界面夹具曾重复建立并清理默认模型，影响共享会话；最终改为直接使用生产 Activity 的模型，无替换内容或清库。Build-151122 的原失败是工具仍硬编码JVM10，而Gradle与11项XML已过；其失败标记保留，后续改为逐名核对实际方法。

[5张原始截图](./HTTPS-20261116-155802-1622-da2100/截图/)已逐张查看：01／02／04为重绘或后台同步的过渡帧，可能显示禁用按钮或前一个选中项，不用它们宣称稳定最终值或全页面视觉通过。服务端 manualUnread、fontPreset 与编辑框显式载入另有真实断言；步骤原件保留一次 `SYNC_LOCAL_RESTRICTION_CHANGED`，没有把晚同步拒绝改写为成功。未做物理设备、全十语／主题／字号组合验证。

AVD 原PID99437经名称及argv核对后正常退出，保留userdata且不保存快照；没有wipe／uninstall／clear。清理后曾由 pm 证实user0缺包才按授权安装同一a135，原因未推断，也未声称恢复旧Keystore。当前 canonical 与 e295认证归档包 SHA 均保持原值。[最终封存](./最终封存.json)记录新旧范围、真实结果、输入及截图摘要；未把新源码追认到旧基线。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

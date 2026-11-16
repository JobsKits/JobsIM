# Android 引用式转发与投影围栏限定证据

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、最终结果与安装包](#result)
- [二、转发与恢复的实际行为](#behavior)
- [三、派生正文与附件保护](#projection)
- [四、证据边界](#limits)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮交付 Android 原生引用式转发、原操作恢复及派生缓存保护，并回归受影响的搜索／收藏、提及／公告、角色资格与 Room 迁移。它是 I 的端侧切片，不代表完整 I／五端／243 项验收。此前[搜索协作](../20261115-搜索协作/README.md)、[发送准入](../20261115-发送准入围栏/README.md)和[基础闭环](../20261115-基础闭环/README.md)保留原源码／APK 范围，项数不跨轮相加。

## 一、<span id="result">最终结果与安装包</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

冻结后的源码构建成功，6 JVM＋10 项受影响定向仪器全部通过，0 失败／错误／跳过，命令退出 0，耗时 163 秒。复用 API37／Android17 ARM64 专用模拟器，经实例 CA、主机名和部署 ID 核对连接独立 HTTPS 28443 实例；没有改全局代理／信任或重启后端。

| 证据 | 位置 |
| --- | --- |
| 52 份源码／构建输入、目标与测试 APK 指纹 | [验证摘要](./验证摘要.json) |
| JVM 6 项 | [原始 XML](./JVM/TEST-com.jobs.im.IsolationTest.xml) |
| 仪器 10 项 | [原始 XML](<./仪器结果/TEST-JobsIMReview_20261114_1424(AVD) - 17-_app-.xml>) |
| 构建和回归 | [Gradle 原始日志](./构建测试日志.txt) |
| 原生转发 | [深色目标选择](./原生UI/转发/01-Native-Forward-Selection-Dark.png)、[未知结果与明确原发送重试](./原生UI/转发/03-Native-Forward-Explicit-Retry.png)、[真实双引用送达](./原生UI/转发/04-Native-Forward-Delivered.png) |
| 原生既有业务 | [精确源及提及](./原生UI/搜索协作/02-Exact-Source-Mention.png)、[群公告](./原生UI/搜索协作/03-Native-Announcement.png)、[深色收藏](./原生UI/搜索协作/06-Native-Dark-Favorites.png) |

最终 APK 位于工程 `Dist/JobsIM-debug.apk`，基线副本为 `Dist/Baselines/20261115-forward-projection-fence-debug.apk`；SHA256=`624785dc9619898162917aad32c443ab19a992129895f87d25c1d5e3523078e1`。已从该专用模拟器读回实际安装 APK，字节指纹与本地交付包相同。14 张原生截图已归档，表中六张已逐图核对可读；仅限定页面／英文配色，不外推全语言／字号／设备矩阵。

本轮选择的类为 `ForwardLifecycleTest`、`ForwardUiTest`、`RoleAdmissionLifecycleTest`、`CollaborationLifecycleTest`、`DiscoveryLifecycleTest`、`StorageLifecycleTest`、`AdmissionFenceTest`、`DiscoveryCollaborationUiTest`。构建任务为 `assembleDebug`、`testDebugUnitTest`、`connectedDebugAndroidTest`，真实服务参数与独立部署 ID 见摘要；CA 通过运行参数提供，连接档案和队列不混用主 8443 或旧 20443 实例。

## 二、<span id="behavior">转发与恢复的实际行为</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- 选择源先精确 GET 重新鉴权；正文版本变化时显示更新后的原文并要求再次明确选择。目标会话也重新 GET。新意图只保存 `clientMsgId/type/forwardOfId/forwardSourceVersion/forwardAdmission/restoreEpoch/sendAdmission` 七字段原 body，先入 Room，再 HTTP；不混入文字副本、源附件 ID、回复或提及元数据。
- 未知结果先只读 `message-results`。404 仍是未知，界面提供“重试原发送”供用户明确选择；只沿同一原 ID、原 body、原 proof 重投，不补最新资格或更换 ID。真实已提交后丢响应则只读核对，不再创建 POST。
- 新创建须实时 `forwarding=true` 与 `send_admission=true`；无 forwarding 能力只能查原结果。旧行缺原完整 body／proof 直接冻结，不从当前服务补齐。源／目标资格、源版本、原成员期历史读取等明确拒绝永久冻结，保留原 body；503 保持未知。
- 源成员退出重入后的 `NOT_A_MEMBER/HISTORY_NOT_ALLOWED`、旧 source proof 及原准入均不会借新资格复活；ViewModel 重建与数据库关闭重开后原意图仍保持原样。
- Room schema7 为 6→7 增量添加转发字段，不破坏旧记录；本轮 schema2→7 升级、原载荷／ID／epoch 保护、未读和草稿修订回归通过。

原生 UI 真实点击选择来源、选择目标、发送、核对未知结果和明确重试；服务端读回确认生成引用式 Message。没有调用 ViewModel 业务方法来替代 UI 点击，读取 ViewModel 仅用于等待最终稳定显示。

## 三、<span id="projection">派生正文与附件保护</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- 打开会话、刷新及重新回到前台均精确 GET 转发消息重新投影，不只等待 events。来源不可用或暂时无法核验时隐藏旧正文／地址，清本机缓存附件；后续成功鉴权会恢复当前投影。
- UI 必须匹配最近一次精确 GET 的完整投影与 version。晚到的同版本历史缓存不能恢复已经隐藏的文字／URL；每个源的请求次序也有围栏，切会话、前后台变化和明确 ACL 失效会废弃旧结果。该边界使用真实 Room 重插旧投影、正常收藏触发首绘再检查，确认没有短暂重显旧正文。
- 搜索／收藏选择原文时，会话或源 GET 明确拒绝会同时擦掉该列表的旧转发正文／URL／proof、Room 派生缓存及本机附件文件。下载前同样精确 GET，读取失败不会继续使用旧 sidecar。
- 转发文件只从当前实例同主机、同端口的固定 `/api/messages/{forwardMsgId}/attachment` 路径鉴权读取，不使用源 attachmentId；外域、其它端口及带查询参数地址被拒绝。真实 file fixture 的字节下载和原源撤回后拒绝再次下载已通过。
- 当前没有会话 JSON 导出／普通备份入口，系统备份关闭，临时 forwardAdmission 不进入普通导出；私有 Room 的原 pending body 专用于恢复。既有单附件 Open／Save As 是用户明确发起的鉴权下载，不等于会话离线备份。

## 四、<span id="limits">证据边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

HTTP 前离线、已提交后丢响应、能力缺失、精确投影 503 与源会话 ACL 拒绝由测试传输层注入；业务引用、源编辑／撤回、成员退出重入和幂等结果均为真实 HTTPS。晚历史缓存重插用真实 Room 模拟，不冒充真实网络乱序测试。

投影并发围栏加入前已过 6 JVM＋10 仪器的原始结果及 APK 指纹保留在 `投影并发围栏前/`；最终结果以本报告主目录为准，不累计两轮数量。

未真机／公网／系统强杀／断电验收。Android 本轮转发媒体只实际验证 file，不声称已跑 PNG／GIF／音频／视频／Range 或大流撤权矩阵；这些后端专项结果不能替代 Android 实测。完整账号删除与派生源生命周期、录制／内置播放、上传未知结果对账、合格设备 ACK／清云、受控迁移／安全恢复和全部 I 仍未闭环。远程推送、RTC、地图、支付、产品 SDK、E2EE 按后续阶段处理。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

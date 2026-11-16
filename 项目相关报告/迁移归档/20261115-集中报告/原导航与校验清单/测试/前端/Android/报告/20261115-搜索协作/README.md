# Android 搜索与群协作限定证据

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、结果与产物](#result)
- [二、实现与断言](#behavior)
- [三、诊断与边界](#diagnostics)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮覆盖 Android 原生服务端搜索／收藏、精确原文、结构化提及／群公告及原角色资格围栏，不代表完整 I 或 243 项验收。此前[基础闭环](../20261115-基础闭环/README.md)与[发送准入围栏](../20261115-发送准入围栏/README.md)保留各自被测源码／APK，不将不同轮次的项数相加。

## 一、<span id="result">结果与产物</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

最终构建成功，6 JVM＋8 项受影响定向仪器通过，0 失败／错误／跳过，完整命令退出 0，耗时 68 秒。复用 API37／Android17 ARM64 专用模拟器，经实例 CA 与主机名校验访问独立 HTTPS 20443 实例，不改全局信任、代理或后端。

| 证据 | 位置 |
| --- | --- |
| 46 份源码／构建输入及 APK 指纹 | [验证摘要](./验证摘要.json) |
| JVM 6 项 | [原始 XML](./JVM/TEST-com.jobs.im.IsolationTest.xml) |
| 定向仪器 8 项 | [原始 XML](<./仪器结果/TEST-JobsIMReview_20261114_1424(AVD) - 17-_app-.xml>) |
| 实际构建与测试 | [Gradle 日志](./构建测试日志.txt) |
| 已核对原生截图 | [浅色收藏](./原生UI/01-Native-Favorites.png)、[精确原文与提及](./原生UI/02-Exact-Source-Mention.png)、[群公告](./原生UI/03-Native-Announcement.png)、[清除后空态](./原生UI/04-Native-Cleared-Announcement.png)、[搜索](./原生UI/05-Native-Search.png)、[深色收藏](./原生UI/06-Native-Dark-Favorites.png) |

APK 已导出至工程 `Dist/JobsIM-debug.apk`，并在 `Dist/Baselines/20261115-discovery-role-debug.apk` 留存本轮基线，SHA256 为 `682be642852e96e4086fc879a54f95ceba17a0aa34b8d268311d1c90d907e80c`。后续代码继续开发时以该基线与本报告摘要定位本轮，不能将新源码冒充已测。

## 二、<span id="behavior">实现与断言</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- 搜索／收藏真实 51 条、50＋1 翻页，原样游标绑定查询／会话／身份；切换条件或账号后的晚回包不会写入新状态。选择结果精确 GET 会话及原消息，不受首 100 条目录限制；正文已编辑时取当前授权正文，退出重入后旧原文 403 清掉对应旧预览。
- 提及目标为当前群成员，群主 @所有人须 `role_admission=true`。草稿、原提及数组和原发送资格先写入 Room；丢响应沿原 ID／原完整 body 核对。明确权限／目标拒绝永久冻结，429 与暂时策略失败不误判为永久资格失效。
- 公告引用 `contentVersion`，个人收藏／反应不误使正文失效；原文编辑后公告清除正文显示不可用。原 requestId、CAS 版本、成员／角色代际及完整 body 先持久化，未知结果原操作核对，`current=false` 只触发当前 GET，不回滚新公告。
- 真实 owner→member→owner 后成员代际未变、角色代际已变，旧未建 @所有人和公告不能借新身份复活。旧公告缺角色快照不补新值；无保护能力只读原结果，不向旧服务 PUT／DELETE／创建 @所有人。
- Room schema6 为增量迁移，schema2→6 保留旧原 ID／载荷／epoch，旧字段默认空值。原发送准入、事务回滚、未读及草稿修订保护在本轮受影响回归中继续通过。
- 原生界面真实点击收藏、精确源、成员勾选发送、公告一天有效期发布／清除和搜索；服务端读回确认提及与公告实际状态。主题切换核对偏好确为 `dark`，再目视深色截图；不依赖仅返回“点击成功”。

测试传输层只注入离线／丢响应、旧能力标记缺失和目录过滤，业务状态、资格变化与写入均由真实后端产生。`DiscoveryLifecycleTest` 的目录遗漏由传输层过滤模拟，不声称真实创建 101 个会话。

## 三、<span id="diagnostics">诊断与边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

角色围栏加入前的局部证据保留在 `角色围栏前/`；首次原生自动化点击失配保留在 `原生UI首轮诊断/`。最终测试通过清除过期 Accessibility 节点缓存、等待原生 UI 空闲及精确文字优先定位完成；这些失败不计入最终通过数。未修改业务逻辑来规避测试。

未真机／公网验收，未跑全部页面、十语与字体组合；当前截图限英文浅色／深色页面。ViewModel 重建和数据库重开不等于系统强杀／断电。原媒体上传未知结果、内置录制与播放、完整转发源生命周期、后台远程通知、清云／安全恢复、RTC、地图、支付、产品 SDK、E2EE 不在本轮交付范围。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

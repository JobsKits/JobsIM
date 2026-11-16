# <span id="前言">JobsIM Android 正式本人导出</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、前言](#jobs-toc-heading-1)
- [二、行为与存储](#jobs-toc-heading-2)
- [三、下载与生命周期](#jobs-toc-heading-3)
- [四、输入与待验](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、前言 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

本轮原生Kotlin／Compose本人导出入口已构建主包与测试包，16 JVM／0失败／0跳过通过（原6 Isolation＋原5 SyncWire＋新5 ExportFile），输入SHA前后一致。5项新Room／模型仪器方法仅编译，未安装／执行；系统选择器、实际保存与真实HTTPS未验，5774原S2包及历史证据保持原义。资源门禁为4GiB、离线单worker／180秒；未启动AVD、新建模拟器、访问API或修改系统信任。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、行为与存储 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

设置入口→独立任务页，十语、三主题、字号、系统返回、完整空态与重载。只列当前本机记录，不冒充服务器全设备任务枚举；演示／未认证仅显示连接后可用，不产出假私人ZIP。

Room9已有`sync_records`存完整原create／cancel字符串、SHA和非秘密nonce，当前本人／profile／namespace／family／device／epoch及已有物理UUID核验同事务完成，不做schema迁移或创建假sync登记。新建前实时cap；未知纯查单，明确fresh404后才由用户同ID同body重试。cap撤回保留原任务核对／取消，新建与下载停止。AT／RT、下载票、制品key、聊天正文和ZIP不进入新增Room记录。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、下载与生命周期 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

ready每次取新RAM短票，认证流先写私有noBackup暂存；文件／ZIP目录有界校验、文件及父目录fsync后才进入系统CreateDocument。保存前重核当前权限／期限、私有长度／SHA与ZIP，再写目标文件描述符并同步；选择取消不报已保存。复制失败尽力删除本轮新文档，仍提示目标可能残留不完整文件，不声称远程存储或硬件断电资格。

关闭／身份变化拒迟到结果并清暂存，流取消断开连接；进程重启只清本模块命名且不在活跃登记中的孤儿文件，不动用户外部已保存ZIP。真实系统选择器、取消／URI提供方及网络流行为必须原生设备验证，源码不能代替。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、输入与待验 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

新增[5 JVM方法](../../../../../测试/前端/Android/Unit/PersonalExportFileTest.kt)已通过，覆盖安全文件名、截断ZIP、目录限额／错计数、ZIP64及十语“取消≠已保存”；[5原生注入方法](../../../../../测试/前端/Android/Instrumentation/PersonalExportLifecycleTest.kt)仅编译，目标为Room原体耐久、创建响应未知、明确原体重试、cap撤回取消、取消原nonce以及系统选择取消／旧family文件投影。未执行的方法不作native通过，即使之后注入通过也不构成真实HTTPS／系统选择器证据。

| 独立输入／证据 | 结果与边界 |
| --- | --- |
| [原始source-only](./SourceOnly-原始检查点.json) | 保留当时未编译语义 |
| [首轮FAIL](./Build-20261116-204257-30135-df22bb/构建结果.json) | Android公开SDK无`O_DIRECTORY`，13秒编译失败；未改SDK或依赖，原输入／日志保留 |
| [修后PASS](./Build-20261116-204403-30408-b75f3d/构建结果.json) | 公开`Os.open(O_RDONLY)`与canonical私有目录／`fsync`，主包、testAPK和16 JVM通过；不表示native目录fsync已经运行 |
| [界面静态检测](./界面静态检测.log) | scoped detector返回`[]`，不代替实际截图 |

当前App SHA256 `1588c450cc0f45e0b512c41ff56803eee8c56f1d0ca165facbe1f2296914a56d`，testAPK `76826edf7a3d38a209b56c8503c08b572aa31e5c25d63c29dffeada4d134ed06`。两包仅在本轮报告中归档，未安装或覆盖旧Dist；canonical e295及旧S2 5774证据未替换。新截图、文件／父目录真实fsync、流取消、系统URI提供方及身份切换完整UI均待协调原生窗口。

后续构建每轮用本目录新子项保原FAIL、源码SHA及产物；不覆盖canonical e295／5774历史APK。入口：[工程](../../../../../代码/前端/Android/README.md#personal-export)／[代码地图](../../../../../代码/前端/Android/代码地图/README.md)／[测试](../../../../../测试/前端/Android/README.md)。完整I与243主项仍未验收。

<a id="🔚" href="#前言">回到首页</a>

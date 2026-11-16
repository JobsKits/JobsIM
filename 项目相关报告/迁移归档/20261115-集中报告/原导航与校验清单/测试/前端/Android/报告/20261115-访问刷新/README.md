# <span id="前言">Android 原生访问／刷新家族</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

## 一、范围与当前状态 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本轮已完成 Android 原生 AT／RT 家族、安全库存储及认证未知结果恢复。最终同一产品 APK 的 **6 JVM＋10 仪器通过，0失败／错误／跳过**，包含旧epoch／旧协议迟到写入安全库守卫与三项真实HTTPS。此前引用转发、搜索协作及200占位报告保留各自源码／APK边界，数量不跨轮相加。

当前 `Dist/JobsIM-debug.apk` 与独立 `Dist/Baselines/20261115-native-auth-refresh-debug.apk` 均为 SHA256 `e295688485e0caa73932670e48da227eae804108416625b62fba5b0e01cee8b3`，18,827,119字节；已从专用模拟器读回实际安装包核对相同。旧引用转发基线仍为 `624785dc9619898162917aad32c443ab19a992129895f87d25c1d5e3523078e1`，旧报告的全部封存文件校验不变。
## 二、证据与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 项目 | 最终证据 |
| --- | --- |
| 同包10项仪器＋6 JVM | [原始日志](./最终同包复核/构建仪器日志.txt)、[仪器XML](<./最终同包复核/仪器结果/TEST-JobsIMReview_20261114_1424(AVD) - 17-_app-.xml>)、[JVM XML](./最终同包复核/JVM/TEST-com.jobs.im.IsolationTest.xml) |
| 38份源码／构建输入及APK指纹 | [验证摘要](./最终同包复核/验证摘要.json)、[安装读回与测试APK指纹](./最终同包复核/安装读回核验.json) |
| 初始未知认证／浅色 | [截图01](./最终同包复核/原生UI/Auth-01-Unknown-Light.png) |
| 初始未知认证／深色与明确新验证动作 | [截图02](./最终同包复核/原生UI/Auth-02-Unknown-Dark.png)、[动作区02b](./最终同包复核/原生UI/Auth-02b-Unknown-Actions-Dark.png) |
| 已验证／深色、冷恢复／浅色 | [截图03](./最终同包复核/原生UI/Auth-03-Verified-Dark.png)、[截图04](./最终同包复核/原生UI/Auth-04-Restored-Light.png) |

五图均已逐图核对主题和可读性；动作区明确展示原认证重试与新本人验证按钮。仅英文／标准字号／专用模拟器，不外推完整语言或设备矩阵。实际服务为新CA的 `https://127.0.0.1:36443`，deployment=`8196a425-b530-49d5-b57f-4663f416e909`、epoch1，未复用旧Caches实例的身份。

[运行入口及完整副作用说明](../../native_refresh.py/README.md)使用现有 SDK／JDK／Gradle 与专用 API37 ARM64 模拟器；不下载镜像或安装工具，不修改系统信任／代理，不停止后台服务或模拟器。中间[受控9项记录](./宿主协调修正/验证摘要.json)保留2项测试边界失败；[后续3项HTTPS复核](./三项HTTPS复核/验证摘要.json)为epoch清理守卫补丁前的中间产品，不与最终10项相加。

## 三、实现与测试边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

整对 AT／RT及家族／来源／设备／实例／epoch绑定由 Keystore AES-GCM密封，AtomicFile同步并原子替换。非秘密提交回执使“安全库已存、Room交接未完”可重启续接；保存失败或网络／解析未知均保留 family unknown，禁止自动复用原RT。只有业务 `ACCESS_EXPIRED`进入单航班，原业务 payload与ID不改；迟到请求不能套新family凭据。

刷新保持原device namespace；重新认证换device，原队列原payload／准入／ID保留并冻结，晚到原空间enqueue也冻结且不清草稿。Room schema2→8真实升级与隔离数据库重开单独验证。旧epoch失效与旧协议安全库写入改走同profile互斥，只能清理原绑定，不能覆盖已恢复的新epoch凭据。

初始认证先保存非秘密原requestId／用途／用户名及完整前序链，不保存密码、OTP或秘密摘要。明确新证明才创建关联意图；新多层恢复须实时 `auth_retirement_chain`，已发原body不补字段。真实链测试为R0实际已成并视作丢响应、R1已持久但未POST、R2携[R0,R1]提交；随后实际HTTPS拒原R0 AT、迟到原R1返回 `AUTHENTICATION_RETIRED`。

两次期限夹具只修改匹配本轮runId、user／family／deployment／epoch及原精确generation的access截止，真实HTTPS返回过期后实际旋转；不是等了15分钟。安全库写失败和响应丢失、R1未POST、并发迟到旧模型为明确受控故障；冷恢复为ViewModel／数据库关闭重建，不冒充进程强杀或断电。中间截图逐图发现主题切换旧浅色帧被标成深色，最终截图增加真实像素帧等待；旧中间图片不作为深色验证。

## 四、失败保留与未验证范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

前置宿主缺失文件返回码处理、JUnit `runBlocking`非void、原始期限夹具起始代际假设、旧run同步残留及adb短超时均已收口，已有目录不覆盖；新增守卫首编漏填测试必填text的编译诊断也保留。编译失败目录中若存在拷贝的旧测试输出，不代表该轮执行；以最终同包XML为准，失败不累计成通过。压力期间只取消过本次 Gradle客户端及本应用仪器进程，不停止其它应用／模拟器／服务器。所有新运行资料使用新 Application Support 实例与新CA、身份；未重建旧Caches路径或伪称旧缓存已恢复。

尚未真机、系统强杀／断电、安全库硬件失效、管理员真实OTP丢失／重新证明、90天真实等待及全十语／字号／设备矩阵验证。本轮不代表完整 I、五端或243项验收；媒体／设备ACK／清云／受控恢复和后续推送、RTC、地图、支付、产品SDK、E2EE范围保留。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

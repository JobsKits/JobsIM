# <span id="前言">Android 原生刷新定向测试</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、范围与依赖](#jobs-toc-heading-1)
- [二、调用与私密输入](#jobs-toc-heading-2)
- [三、产物与边界](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、范围与依赖 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[native_refresh.py](./native_refresh.py) 使用现有 Python 3 标准库、JDK、Gradle wrapper、Android SDK/adb、已经启动且授权的指定模拟器，以及隔离实例现有 PostgreSQL 客户端。当前在macOS宿主验证，Windows／Linux未原生验收。不安装或升级依赖，不启动／停止服务或模拟器，不改全局代理与证书，不覆盖 `Dist/JobsIM-debug.apk` 或旧封存报告。

运行会在选定目标安装本项目及测试包、创建独立测试账号／会话／群，执行 6 条 Keystore／Room／协程用例、1 条真实 schema2→8 迁移及 3 条真实 HTTPS 用例。访问过期使用两次**仅该测试本人 family**的 `access_expires_at` SQL 期限夹具，随后由真实 HTTPS 返回 `ACCESS_EXPIRED` 并真实旋转 RT；不声称实际等待了15分钟。丢响应、R1首POST未达由明确测试传输／本地意图夹具注入，R0/R2认证、退休、旧 AT 拒绝、迟到 R1 拒绝均读真实服务。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、调用与私密输入 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

从任意目录调用实际脚本路径，输入明确的新实例身份与专用设备。默认只进行只读环境／HTTPS能力预检；`--execute` 明确授权上述测试副作用。需要 `auth_refresh=true` 和 `auth_retirement_chain=true`。

```sh
python3 /项目路径/测试/前端/Android/native_refresh.py/native_refresh.py \
  --environment-file /私密路径/environment.json \
  --deployment-id 本次核对的新部署ID \
  --serial 专用模拟器序号 \
  --report-dir /报告路径/本次新目录 \
  --execute
```

私密环境文件必须仅所有者可访问，包含 `serverURL`、`caPath`、`pgBin`、`businessURL`，可选 `pgPassFile`；通过文件传入，不把数据库口令或认证凭据放命令行。脚本把数据库秘密只传给 PostgreSQL 子进程环境，捕获的敏感失败输出不回显；报告只留状态、指纹、公开实例身份和非秘密计数。JDK与SDK按当前进程的 `JAVA_HOME`／`ANDROID_HOME` 或 `ANDROID_SDK_ROOT` 选择；SDK缺失即停止，不补装。

只对已枚举的指定设备设置本次服务端口的 `adb reverse`，不影响其它目标。设备测试在 App 私有文件中写入非秘密 family/user/deployment/epoch／原refreshGeneration／随机请求ID；宿主校验部署与 UUID、仅匹配当前未撤销且仍为原精确代际的本人家族，期限更新唯一成功后写确认。设备完成即移除这两个自己创建的同步文件。不得对正式用户实例使用此入口。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、产物与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

报告目录必须事先不存在，以免覆盖历史。产出构建仪器日志、XML、JVM结果、APK／输入 SHA256摘要及五张原生明暗／动作区／冷恢复截图；业务 APK仍在工程标准 `app/build/outputs`，不自动发布至冻结 `Dist`。`--https-only`只跑3条HTTPS，用于产品APK未变且已有本机证据的修复复核；不把它独自记作10条通过。退出0仅代表所选范围这10条定向仪器全部通过且没有跳过、两次时钟夹具完成；失败保留报告与非零退出码。Ctrl+C只终止本次 Gradle客户端，不停止现存服务／模拟器。

它验证普通原生家族与本机安全库，不证明物理断电、进程强杀、真实手机、全十语／设备矩阵、后台远程通知、90天等待、灾备或完整 I 全部验收。直接跑 `NativeSessionHttpsTest` 时还需显式 `nativeRefresh=true`；缺少本宿主期限协调器会超时，不应用普通 `live-*` 模式冒充本专项通过。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

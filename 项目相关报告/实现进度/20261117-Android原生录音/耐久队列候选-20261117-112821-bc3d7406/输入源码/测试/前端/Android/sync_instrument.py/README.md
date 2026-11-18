# <span id="前言">Android 权威同步定向仪器</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

## 一、作用与前提

只使用本任务已有 `emulator-5582`／`JobsIMReview_20261114_1424`，核AVD名、qemu PID／port及当前应用SHA后，安装已验证构建报告中的自有应用／测试两包。默认只执行8项AuthoritativeSyncTest与1项 `retainedApiObjectCannotAdoptANewFamilyWhenInvokedAfterSwitch`；没有真实HTTPS、SQL、后台seed或旧联调类。

从仓库根示例：

```sh
python3 测试/前端/Android/sync_instrument.py/sync_instrument.py \
  --build-report 项目相关报告/测试/前端/Android/20261116-权威同步/Build-本轮目录 \
  --execute
```

`--with-migration`另执行可靠原schema8迁移1项，与原9组成完整10项；源schema只在testAPK，工具校验封内原字节SHA并拒绝进入产品包。重跑时 `--previous-build-report` 可显式验证本任务此前已安装的中间包来源；不接受未知当前包。无 `--execute` 只自述。需要现有Python3与Android SDK adb，不下载依赖。`ANDROID_SDK_ROOT` 可指定已有SDK。先协调原生资源窗口；工具不新建／启动／擦除／重启模拟器，不运行 `pm clear` 或删用户库，不覆盖Dist旧包。

## 二、输出与失败处理

每轮中央新建 `Instrument-<timestamp>-<pid>-<random>`，保留逐命令原始输出、实际安装前后拉回SHA、精确9／10方法结果及资源门禁。安装读回放当前用户Application Support／JobsIM专属临时目录，不触旧Caches；原 e295 及固定APK始终保留。

总时限360秒、仪器180秒，每半秒检查宿主至少4GiB；失败保留非零退出与raw。超时仅停止本次adb进程组，必要时force-stop此AVD的 `com.jobs.im`／`.test` 自有测试进程，不停模拟器或其它应用。每方法使用新UUID数据库／profile／缓存夹具，完成后只清自己创建的夹具。

仪器通过只证明本窗口受控注入与实际Android安全库／SQLite行为，不代表真实HTTPS或完整 I 验收。失败修正后需新构建报告及新结果目录，不能覆盖旧失败。

## 三、本人状态候选 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

`--with-self-state` 追加外置 `SelfStateTest` 的13个方法，逐名核对实际原生结果；`--with-migration` 仍单独选择旧迁移方法，同时指定时为本轮23项，不能与旧10项相加作累计验收。`--report-root` 指定独立中央报告根，相对路径按仓库根解析。这些方法只用独立 UUID Room 库及注入 API，不使用私密种子、不连接服务器，也不启动模拟器；模拟器已关闭时须先由主流程协调原实例窗口。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

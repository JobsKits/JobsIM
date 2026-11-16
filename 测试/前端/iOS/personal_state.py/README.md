# <span id="前言">iOS 本人状态原生构建与本地回归</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行范围](#jobs-toc-heading-1)
- [二、隔离与证据](#jobs-toc-heading-2)
- [三、明确真实HTTPS与系统点按](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行范围</span><!-- jobs-toc:heading-end -->

复用现有 Xcode、Pods 与单任务 Swift 编译包装器，不下载依赖、不清理缓存或用户库，不启动／关闭模拟器。`build/test`不请求真实API；`real/ui`仅在主流程明确释放新域后执行。每步 Data 至少4GiB，总时限不超过600秒，越界只停止本次自有进程组。第三方仅参与正常编译，源码不修改。

先协调构建资源，传入现有 JobsIM 专用模拟器 UUID。无 `--execute` 仅自述。

```sh
python3 测试/前端/iOS/personal_state.py/personal_state.py \
  --simulator <已确认的JobsIM模拟器UUID> --mode build --execute
```

原生测试须先由主流程明确开启同一模拟器；工具只接受 Booted 的 JobsIM 目标。输入、主程序及测试二进制必须仍匹配指定构建，禁止把旧包结果套到新源。

```sh
python3 测试/前端/iOS/personal_state.py/personal_state.py \
  --simulator <同一UUID> --mode test --build-report <本轮Build目录> \
  --only JobsIMTests/IMPersonalStateTests \
  --only JobsIMTests/IMAuthoritativeSyncTests --execute
```

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、隔离与证据</span><!-- jobs-toc:heading-end -->

产物使用工程外 `Application Support/JobsIM/Build/iOS/PersonalStateUI`，保留此前 SelfState 及 S1 产品。中央 `项目相关报告/测试/前端/iOS/20261116-S2原生界面/` 每轮唯一目录保存原始日志、资源采样、Jobs 自有源原件／SHA、单worker编译器审计、原测试配置及当前 App 包。失败不覆盖。

每次build的物理产物改为`PersonalStateUI/Products/<本轮Build名>/Debug-iphonesimulator`，不覆盖旧Products。`--report-parent`可显式选择本项目中央iOS目录下的切片，例如`项目相关报告/测试/前端/iOS/20261116-正式本人导出`；入口拒绝工程外报告目录。后续test只沿指定Build记录的原xctestrun／原包SHA运行，不以新的默认目录寻找旧包。build时`--only`只记录拟执行分组，build-for-testing不表示这些方法已执行。

本地测试复制实际构建的 xctestrun，仅在测试host环境加 `JOBS_IM_SYNC_INJECTED_TESTS=1`，并固定原测试根；该 DEBUG 标记隔离共享 IMSession 恢复／verifyRestoreEpoch，不是 IMAPIClient 全局 HTTP 阻断。入口拒绝 Integration 类，真实 HTTPS 必须另行协调并使用新 fixture。本入口的 exit0 只表示 Xcode 此步成功，具体方法数、跳过与实际断言以原 xcresult 为准，不称完整 I 或真机通过。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、明确真实HTTPS与系统点按</span><!-- jobs-toc:heading-end -->

先核新实例公开CA／部署／epoch fixture，只对本轮新账号执行；密码不保存，整对及控制AT仅进Keychain。`--mode real`仅接受`IMSelfStateIntegrationTests`的精确方法，`--mode ui`仅接受`IMSelfStateUITests`的精确方法；两者必须共享新的公开`--run-id` UUID，不能借用旧profile／family。真实suite在隔离host启动之后才启用它自己的HTTP，实际App由既有`--ui-test-profile`只选择已进入安全库的档案，不传密码／AT到启动参数。

顺序为真实业务2项、准备原生界面fixture1项、XCUITest1项、最后原生核服务端状态与关闭1项。UI失败也必须执行最后关闭方法。示例沿用同一`--simulator`和`--build-report`：

```sh
python3 测试/前端/iOS/personal_state.py/personal_state.py \
  --simulator <已确认UUID> --build-report <本轮Build目录> \
  --mode real --run-id <本轮公开UUID> \
  --only JobsIMTests/IMSelfStateIntegrationTests/testRealSchemaTwoReadViewAndFontKeepOriginalOutbox \
  --only JobsIMTests/IMSelfStateIntegrationTests/testRealDraftLostDeliveryReopensOriginalAndRetiredKeepsOwnedText --execute
```

后续精确方法分别为`testPrepareNativeUIFixture`、`JobsIMUITests/IMSelfStateUITests/testNativeSelfStateViewsCloudDraftAndExplicitAdoption`和`testVerifyNativeUIFixtureClosed`。本轮公开ID journal在App Documents按run原子保存；最后原AT401及六条关闭事实经逐项校验才复制到报告。任何未知或失败不猜已关闭；旧失败／截图／中间包均保留。每方法成功／失败／跳过均逐名解析，Xcode exit0不单独当作全部方法通过。

初始认证前，原公开nonce／身份journal须已由host逐字节fsync并回同SHA确认；密码、AT／RT不进该journal。S2界面档案另外使用Documents中固定`s2-UUID`的原子plist，包含公开实例CA、用户DTO、档案标识与UI设置，App在读取共享Session前导入。host也保留原plist与SHA；Keychain整对与GRDB认证责任仍是实际恢复来源。旧UserDefaults快照只保留既有夹具兼容，不作为跨XCTest阶段的唯一交接。该交接修补已随[后续导出切片](../../../../项目相关报告/测试/前端/iOS/20261116-正式本人导出/README.md)完整编译；31原S2＋10导出共41在同冻结包已原生本地通过，完整跨进程UI仍待复验，不追认此前30项结果。

<a id="🔚" href="#前言">返回前言</a>

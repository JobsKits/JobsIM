# <span id="前言">iOS 正式本人数据导出本地验证</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、范围与当前状态](#jobs-toc-heading-1)
- [二、源码检查](#jobs-toc-heading-2)
- [三、后续验证](#jobs-toc-heading-3)
- [四、实际编译记录](#jobs-toc-heading-4)
- [五、实际本地执行](#jobs-toc-heading-5)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、范围与当前状态</span><!-- jobs-toc:heading-end -->

本轮App与测试包完整build-for-testing通过，随后同冻结包实际执行31原S2＋10导出共41本地方法，0失败／跳过。仅启动本端既有自有sim，结束已正常关闭；未执行UITest／真实HTTPS／系统Files保存或真机验证。[执行后123输入与包核验](./Test-20261116-212217-3c6929/执行后只读核验.json)已封存。后端合同见[正式本人导出](../../../../../代码/后端/Go/接口契约/README.md#personal-data-export)。旧JSON备份保持独立，不用旧30项结果追认本轮输入。

| 入口／默认值 | 行为与持久化 | 生命周期／围栏 | 源码 | 验证与缺口 |
| --- | --- | --- | --- | --- |
| 设置→本人数据导出 | 仅列当前本机命名空间发起的原请求；每次进入／重载请求真实状态；完整空态与重载 | demo不伪造任务；捕profile／部署／user／personal／epoch／device及family | IMSettingsVC／IMDataExportVC | 已编译，UI未运行 |
| 请求导出 | fresh chat_export后，原nonce／UTF8 body／SHA先FULL事务；先只读原单，精确未建才首次POST | 未知保原意图，仅用户明确同原体重试，不自动换nonce | IMDataExportService／Protocol | 10项导出本地反例已执行，通过；HTTP为显式注入 |
| 查原单／当前状态 | 原body只读查单；只有精确EXPORT_OPERATION_NOT_FOUND表示本次未查到，不当未提交证明 | capability撤回仍查原责任；只取声明元数据，陌生响应字段不落库 | 同上 | 真实接口未验 |
| 取消／原取消重试 | 独立稳定requestId原body／SHA一次落盘，CAS不能替换；不混入create epoch | terminal但cleanup未cleared仍留原取消；不把handoff pending当已清理 | IMDataExportVC／IMLocalStore | GRDB新连接／冲突及同体取消反例已原生本地通过 |
| ready→ZIP→系统保存 | fresh状态、当前epoch、published和新短票；票只RAM，固定同源路由和bearer；原字节流到独立临时文件 | 取消Task停止自有下载；拒重定向／非ZIP／非no-store／超1GiB；失效不交系统 | IMAPIClient／IMInstanceAgent／IMDataExportArchive | 类型／链接通过，真实行为未验 |
| 完整制品交付 | 交Files前校EOCD、ZIP64、完整中央目录、双manifest入口、本地头、重复与越界路径；不解包重写 | 临时文件不进GRDB／普通备份；取消／保存回调清自有目录；外部已存ZIP不称远程可抹除 | 导出页／DocumentPicker与FileHandle工厂 | 合成ZIP／ZIP64及坏目录文件反例已本地通过；非真实服务制品／Files保存，不宣称逐文件CRC已验 |

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、源码检查</span><!-- jobs-toc:heading-end -->

十语均209键且逐个plist lint通过；三主题沿用IMUI语义配色，实际主题／大字号／窄屏和Files选择未实看。源码阶段11个新增／修改Swift文件parse、项目plist及76个Jobs自有文件构造审计issues=0分别保留。[22项原件及SHA](./Source-20261116-203136/结果.json)只证明当时源码检查点；后续Swift6窄修后的现行123项输入以本轮Build为准，不能用旧22项冒称当前全量编译输入。

链式审计原始输出保留：首次全范围88组候选；合并新导出页连续table装配／reload链后为86组、26文件，脚本未加载外部DSL定义。候选含既有布局与self业务控制流，不能标为全绿；不扩成全工程风格重写。ZIP目录检查在可取消utility任务执行，不把大文件解析放主线程。

导出10项已本地执行通过：严格DTO／字符串计数、opaque nonce／原字节摘要、GRDB原record CAS、未知只查及显式同体重投、能力撤回仍查取消、迟到身份、短票期限／类型、ZIP／ZIP64完整目录、缺项／重复／路径、GRDB新连接原体且不建立sync责任。SQLite与文件操作运行在原生测试host，HTTP为显式闭包注入；新连接不冒称进程断电。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、后续验证</span><!-- jobs-toc:heading-end -->

41本地已在主流程授权窗口沿[有界包装器](../../../../../测试/前端/iOS/personal_state.py/README.md)和固定Build执行，本端模拟器已Shutdown。完整UI、系统Files保存、真实导出HTTPS还需独立窗口与新fixture，不沿旧包、旧CA或旧family伪称已验。执行后root另读到低于4GiB，现仅封小报告，不新增VM／构建／API；不据此改变本轮原始资源采样或推断空间波动原因。完整I／243项尚未验收。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、实际编译记录</span><!-- jobs-toc:heading-end -->

| 轮次 | 结果与输入 | 原始证据 |
| --- | --- | --- |
| 首轮 | 372.54秒FAIL；新IMDataExportVC的createButton嵌套weak-self捕获触发Swift6隔离诊断 | [结果](./Build-20261116-204840-02fdf2/结果.json)、原始日志／输入原件均保留 |
| 窄修后 | 377.42秒PASS；创建动作移至MainActor方法，不加unsafe／Sendable豁免；App与test／UITestRunner全构建 | [结果](./Build-20261116-205705-4c8f38/结果.json)、[包与源码核验](./Build-20261116-205705-4c8f38/编译后只读核验.json) |

本轮123项输入的live与归档SHA全部一致；App共109文件、UITestRunner共7文件逐字节一致。App二进制SHA为`89e027360aae25ea66521e68eb4737bc0e6edc3fdcbda6fd664cbce3cfeef8f1`，测试二进制SHA为`94a1cbb88479e42fce297f9d2a55d56742cd213602fee956dde2a865f9a83b93`。45次driver实际末尾-j1，构建采样最小可用5,604,089,856字节，未触4GiB门禁。每轮使用新的物理Products目录，旧主／测试两项冻结SHA仍一致；未修改Pods／Vendor、未下载SDK或清缓存。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">五、实际本地执行</span><!-- jobs-toc:heading-end -->

[Test-20261116-212217-3c6929](./Test-20261116-212217-3c6929/结果.json)共22.59秒，IMAuthoritativeSyncTests 21、IMPersonalStateTests 10、IMDataExportTests 10均PASS，0失败／跳过。只选择这三类，未选择UITests或Integration；host的DEBUG隔离标记只隔离共享Session恢复，不声称全局HTTP阻断。执行前后123输入与89e／94a1两包SHA一致，旧两项冻结包也一致。原生本地资源采样最低5,116,923,904字节（约4.77GiB），自有既有模拟器正常关闭；另两项目模拟器未操作。

原[最终编译检查点](./最终编译检查点.json)保留当时“未执行”范围，后续以[最终本地检查点](./最终本地检查点.json)和原xcresult／逐名outcomes为准，不把两阶段相加为82或把旧30加成71。

<a id="🔚" href="#前言">返回前言</a>

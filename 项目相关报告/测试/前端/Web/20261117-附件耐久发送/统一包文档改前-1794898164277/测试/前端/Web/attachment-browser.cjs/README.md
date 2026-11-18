# <span id="前言">Web 附件原单浏览器验证</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、执行与资源门禁 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

复用已有[**Chrome**](https://www.google.com/chrome/)、[**Playwright**](https://playwright.dev)和Vite，不安装依赖。测试用随机回环地址、私有缓存和`configFile:false`，不连接真实服务，不覆盖当前运行包。所有API真实网络阻断，身份、HTTP结果与文件字节为显式隔离夹具。

```sh
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_ATTACHMENT_BROWSER_REPORT=/本轮未使用目录 \
node 测试/前端/Web/attachment-browser.cjs/attachment-browser.cjs
```

必须先过4 GiB门禁并获得本任务串行窗口；[外层运行器](../../../../项目相关报告/测试/前端/Web/20261117-附件耐久发送/运行器.cjs)每秒监测空间、每阶段180秒、单worker，保留完整当前Src／Public／工程配置／相关脚本／三份修前源，前后重枚举并核SHA。它只终止本次子进程组，不清缓存、不重启既有服务。支持`type`、三项`before-*`、`browser`、完整`regression`、限定global／outbox／auth／audio的`focused-regression`、固定次数认证`auth-five`、收讫查询注销清理`receipt-cleanup`及`build`，每阶段使用新的证据目录。

## 二、真实与注入的边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

真实执行React、严格IndexedDB事务、WebLocks、页面reload、列表交互与窄屏截图。确认发送后Blob／原manifest先提交、配额abort零HTTP、重启原bytes／nonce、纯查不创建、明确同体继续、原Outbox linked事务／保Blob及已接受释放均在实际浏览器检查。HTTP容器字节是固定虚拟音频，**不证明媒体格式兼容**；真实MediaRecorder合成WebM另见[原生音频证据](../../../../项目相关报告/测试/前端/Web/20261117-Web基础音频/README.md)，两种夹具不混用。

取消检查精确原责任／body、队列缺失／变更时保文件、旧family仅本机清理、在途上传取消、最终creating锁与迟到接受不复活。global检查完整复合映射、未挂载namespace／archive、错key和活动原单假messageId保Blob＋待办、最小floor在GC后拒同seed、坏floor及repair对prepare／update／linked／最终creating门禁。另覆盖坏floor的alias已随权威global清完后，外部raw替换合法空floor，再观察已知与新global仍保repair、原seed零Blob／HTTP；正常新floor成功不虚设repair作对照。completed原注销擦全epoch／device并拒晚到写；无原identity的导入收据不能按用户名删。

## 三、有效修前反例 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

`JOBSIM_ATTACHMENT_GLOBAL_BEFORE_FILE`通过Vite虚拟模块载入封存的Global错key修前源；`JOBSIM_ATTACHMENT_PROTOCOL_BEFORE_FILE`仅替换当前Global的协议import，Actions／Store仍为当前源。两者只有看到`blobPreserved:false`、`globalCommitted:true`及具体AssertionError才算有效红。`JOBSIM_ATTACHMENT_REPAIR_BEFORE_FILE`载入第三份封存Global，必须实际先清旧原单且设repair，再错误清repair／重建Blob、零HTTP且权威global已提交，最终AssertionError才算有效红。三者均执行真实IDB断言，不回滚生产文件；import／工具错误不算产品红证据。当前三项有效红均已实际成立，修后41项与5张实际原单截图已执行，逐轮冻结输入、原件及对应私有成品见报告。

当前执行状态、实际结果与输出包见[报告](../../../../项目相关报告/测试/前端/Web/20261117-附件耐久发送/README.md)。脚本存在或语法通过不代表用例已经执行，不将旧音频包或旧注销23组追认为本轮结果。目标HTTPS、真实账号、实际麦克风和系统断电仍需独立验收。

## 四、收讫查询记录的定点清理 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

`receipt-cleanup`向本脚本传`JOBSIM_ATTACHMENT_RECEIPT_CLEANUP_ONLY=1`，仅执行三个实际IDB用例后退出，不运行41项附件矩阵：验证completed删除本人所有device／epoch及archive的`receipt-queries|namespace`六字段查询metadata，保其它user／deployment／origin及未知键；保原注销与受限数据权利收据；本人原AuthSession仍匹配时，旧`ReceiptContextActions.rows()`确实进入strict读写事务并被completed marker中止，两次均不重建查询记录、零HTTP。

此模式已针对Root冻结的新Cleanup／ReceiptContext执行，三个实际IDB用例通过，完整103输入前后无漂移；[新结果](../../../../项目相关报告/测试/前端/Web/20261117-附件耐久发送/08-收讫查询注销清理-1794897771473-27751/浏览器/结果.json)保留实际输入和零真实API边界。旧41输入与旧Production仍为独立冻结证据，不覆盖这次新源；最终类型与对应包由协调方统一验证，本模式不重复大构建。

<a id="🔚" href="#前言">回到首页</a>

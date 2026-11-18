# <span id="前言">Web 附件耐久发送候选与验证</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、当前状态 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

前序冻结候选提供可编辑源码、工程／运行入口与匹配私有Production：录音明确发送／文件选择先严格IDB保存原Blob和固定manifest，再请求稳定上传；输入区有原单恢复、纯查、明确同体继续、导出与确认本机停止。新附件／清理41项真实Chrome／严格IDB／WebLocks通过，三项有效修前反例成立；类型与私有构建复用同输入新证据，全部77 Src、构建5项配置／锁及7个Public与11个产物SHA逐项核齐。**没有替换实际运行a4实例；真实API、账号、麦克风和完整第一阶段未验收。**另轮双tab认证失败仍未归因，固定五次专项未复现，原件与全部尝试保留。

当前另有收讫查询metadata清理增量：Root补completed的`receipt-queries|namespace`归属清理，另一owner更新ReceiptContext解析；本端`receipt-cleanup`三个真实IDB用例已通过，103输入前后齐、最低可用空间13,612,511,232 bytes，零真实API。[定点结果](./08-收讫查询注销清理-1794897771473-27751/浏览器/结果.json)与[执行围栏](./08-收讫查询注销清理-1794897771473-27751/进程.json)独立封存。旧41／103输入与旧Production不能追认这批新源；统一新类型／最终包由协作owner另行补证，本端未重复构建或不相关矩阵。

已实际执行[最新27项附件纯规则](./纯规则-1794889862189/结果.json)及[当前8 MiB录音／续点22项纯规则](../20261117-Web基础音频/纯规则-1794889863065/结果.json)，各自生产规则与脚本输入前后SHA一致；前者包含boxed String最小floor反例。上述纯规则不执行Store／浏览器fixture；实际IDB／UI执行依据为本报告的41项浏览器原件及各阶段冻结输入，脚本语法检查另记，不相互替代。

Root另执行[当前8份TS的Node语法检查](./廉价语法-1794890800076/结果.json)，输入前后SHA一致；不执行imports、不含TSX，也不等于全工程TypeScript类型检查或IDB／UI验证。

## 二、源码入口和行为 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 入口 | 实现与生命周期 | 当前验证 |
| --- | --- | --- |
| [录音面板](../../../../../代码/前端/Web/Src/AudioRecorderPanel.tsx)／[控制器](../../../../../代码/前端/Web/Src/AudioRecorderController.ts) | 显式开始、停止试听／取消、明确发送；1秒～5分钟／8 MiB，迟到许可／隐藏／卸载／身份切换关track；同步双击只交接一次 | 当前8 MiB规则22／新Chrome音频19通过；仅合成音轨，无真实麦克风 |
| [App](../../../../../代码/前端/Web/Src/App.tsx)／[Actions](../../../../../代码/前端/Web/Src/AttachmentSendActions.ts) | 捕获原family／epoch／会话／准入／回复CV，Blob先落盘；原8字段查单，明确同nonce／bytes重试，无能力不回退非幂等上传 | 新41覆盖实际Actions与原单Panel／IDB；App录音入口为实际演示交接，HTTP为显式夹具 |
| [Store](../../../../../代码/前端/Web/Src/AttachmentSendStore.ts)／[storage](../../../../../代码/前端/Web/Src/storage.ts) | 32项／64 MiB同scope，linked与原Outbox同strict tx且保Blob，接受后释放；本机取消精确冻结原队列，最终POST锁防确认后新增发送 | 严格IDB、WebLocks、页面reload／abort／错路由及原件保留已新验 |
| [Panel](../../../../../代码/前端/Web/Src/AttachmentSendPanel.tsx)／[十语](../../../../../代码/前端/Web/Src/AttachmentSendLabels.ts) | 默认空态／重载，原单分页、纯查／明确继续、导出、确认停止；旧family仅本机保存／清理，坏holder／repair提示清理不足；继承三主题 | 十语纯规则、真实组件5图、英文白天／藏文黑夜390px已核 |
| [原责任规则](../../../../../代码/前端/Web/Src/AttachmentSendProtocol.ts) | 身份／manifest／原body／收据白名单，prepared非可用收据；物理holder键、server ID与完整user／conversation／client关联 | 27纯规则覆盖相关边界 |
| [最小拒旧floor](../../../../../代码/前端/Web/Src/AttachmentErasureProtocol.ts) | 独立四标识事实，原单GC后仍拒旧seed；坏floor保原值＋repair，Store同tx拒写 | 27纯规则及新IDB的GC／global先到／repair保留／写门禁已验 |
| [Global](../../../../../代码/前端/Web/Src/GlobalMessages.ts)／[注销清理](../../../../../代码/前端/Web/Src/AccountDeletionCleanup.ts) | Root维护双cursor、先floor再清holder、坏值保Blob／精确pending；原completed同scope全epoch／device擦除与late-write门禁 | 新41中实际IDB擦除、7类迟到写拒绝、错key／假messageId／repair及正常对照已验 |

可移植工程／准备／运行入口见[Web README](../../../../../代码/前端/Web/README.md)，完整当前代码地图见[Web代码地图](../../../../../代码/前端/Web/代码地图/README.md)。稳定协议定义以[后端媒体上传测试说明](../../../../../测试/后端/接口/media_upload/README.md)及后端当前契约为准；当前用户运行服务未在本轮替换。

## 三、验证入口与保留原件 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- [纯规则入口](../../../../../测试/前端/Web/attachment-rules.cjs/README.md)：Node原生类型擦除，零编译产物／浏览器／网络。
- [真实浏览器入口](../../../../../测试/前端/Web/attachment-browser.cjs/README.md)：实际React／IDB／WebLocks／reload，显式HTTP和身份夹具，零真实API／麦克风。
- [串行运行器](./运行器.cjs)：完整103文件闭包冻结、4 GiB持续监控、每阶段180秒。三项有效红／修后41与限定global／outbox／auth／音频已执行；同输入类型／私有构建及其它回归复用原证据，未重复构建。五次认证专项另封修改后fixture／运行器输入，未改变产品源码。
- [活动原单假messageId修前源](./修前-活动原单messageId.ts)及[SHA](./修前-活动原单messageId-SHA.json)、[错namespace修前Global](../20261117-注销本机清理/修前-global错namespace.ts)均封存；只有真实Blob被误删且global提交的AssertionError才算有效红证据。
- [初始音频报告](../20261117-Web基础音频/README.md)保留实际MediaRecorder合成WebM和原403失败；它与本轮虚拟音频bytes／耐久新候选分开，不作相互替代。

repair反例使用[第三份修前Global](./修前-repair被观察清除.ts)：坏floor时权威global仍清旧alias持有者，随后模拟外部raw替换为空floor、清空全部alias视图／原单，再观察已知与新global。修前实际重建Blob、global提交、repair曾设又被清并触发AssertionError；当前代码保repair并拒旧seed零Blob／HTTP，正常新floor提交alias且无repair的对照也通过。修前虚拟模块没有回滚生产文件，import或工具错误未算作有效红。

| 独立输入与原件 | 实际结果 |
| --- | --- |
| [错namespace修前](./02-修前错namespace-1794892621441-18710/进程.json)／[假messageId修前](./03-修前活动messageId-1794892646059-21017/进程.json)／[repair修前](./03-修前repair被观察清除-1794892680521-23545/进程.json) | 三项均为真实Blob错误变化＋权威global提交＋AssertionError，`validProductReproduction:true`，输入前后齐 |
| [修后附件／清理](./04-附件与清理浏览器-1794892718400-26457/浏览器/结果.json) | 41 PASS；真实React／严格IDB／WebLocks／页面reload；无真实API或麦克风 |
| [global](./05-global-facts-1794892796580-30213/浏览器/结果.json)／[Outbox](./05-outbox-proof-1794892801537-30213/浏览器/原请求队列结果.json)／[认证](./05-auth-session-1794892806839-30213/浏览器/浏览器协议结果.json)／[音频](./05-audio-browser-1794892813129-30213/浏览器/结果.json) | 6／10／16／19分别通过；均单独封存103输入，最低可用空间超过4 GiB |
| [并发新证据复核](./并发新证据复核-1794892533745.json) | 类型01、构建02及迁移04／群05／改密06／注销07／导出08原执行0且当前Src无漂移；认证09原失败保留，不复用为通过 |
| [本轮汇总](./当前验证汇总-1794894919672.json) | 所有已执行阶段的当前产品Src／Public／配置无漂移；测试数不累计为功能覆盖率 |

截图位于[本轮浏览器目录](./04-附件与清理浏览器-1794892718400-26457/浏览器/)：英文白天正常／完整空态／清理不足、藏文黑夜390px正常／旧身份动作共5图。Root实际查看前述英文3图及藏文旧身份动作4图；本端作者查看全部5图。长文件名与按钮可换行，旧身份的网络动作禁用、本机保存／明确删除可用；260px原单区可独立滚动，顶部重载可达。

三份README已精确接入目录清单190→193，原190原序保留。193份只读`verify`通过，本任务三份正文在接入窗口内SHA不变，真实index staged／flags未变；期间根README／资产／进度三份正文由并发owner更新并保留。`install`在60秒有界阶段超时，原日志保留且未重试；只回收已核实退出PID本轮留下的空锁／原索引相同临时副本。原始index `check`列39份未暂存，不宣称发布通过。详见[接入结果](./目录接入/14-交付结果.json)、[超时原件](./目录接入/10-install.json)与[精确回收证明](./目录接入/16-精确残留回收.json)。本段是在目录独占释放后按父任务授权增补。

## 四、交付与未验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

前序41候选的可修改源码、[工程与构建／运行入口](../../../../../代码/前端/Web/README.md)及同输入私有Production均可核对。[私有产物复用证明](./同输入私有产物复用-1794892597771.json)记录实际位置和全部11产物SHA：JS为`index-CCqfIXzK.js`，SHA-256为`d3dff62c24c3a6febffe7018b38ad416f2149e881cafe82d5aabc54754599cfa`。原类型报告封77份Src；其后原构建封77 Src＋5项配置／锁，补核全部7个Public与原产物逐字节相同，未漏新增模块。该验证窗产品源码未改、复用原私有包；后来收讫查询增量另按首节新证据，不更新或外推这份旧包。未覆盖a4运行包、CA／实例ID或历史证据。

strict IDB成功只证明浏览器事务完成，不等于系统断电、站点被清理后恢复或qualified ACK。本机停止不能撤回已经送到服务器的消息；导出操作只交给浏览器，不能宣称已写用户磁盘。真实HTTPS、账号、麦克风／权限设备和目标浏览器格式仍独立验收；本报告不宣称第一阶段整体完成。

## 五、认证专项未归因项 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

另轮[09双tab失败原件](../../../../实现进度/20261116-剩余能力闭环/Web逐存储收讫/09-认证复验执行.json)在第二组并发read得到Authentication required，原日志未记录具体code，原因尚未确认。本轮初始认证16组通过后，仅增加fixture失败诊断，不改变断言、业务body或生产保护，再按预定五次执行；[完整五次总览](./固定5次认证-1794893088191-40862.json)保留全部尝试。前三次无尾斜杠、后两次固定尾斜杠，每次独立Vite／浏览器／报告，各16组通过，实际URL与AuthSession importURL逐份记录。未复现不等于已归因或已修，保留此限制，不追加盲重试。

<a id="🔚" href="#前言">回到首页</a>

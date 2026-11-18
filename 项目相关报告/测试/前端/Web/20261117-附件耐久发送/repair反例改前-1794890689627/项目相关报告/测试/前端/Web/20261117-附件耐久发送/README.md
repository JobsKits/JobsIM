# <span id="前言">Web 附件耐久发送候选与验证</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、当前状态 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前新候选已落可编辑源码及实际输入区接线：录音明确发送／文件选择先严格IDB保存原Blob和固定manifest，再请求稳定上传；输入区有原单恢复、纯查、明确同体继续、导出与确认本机停止。**本候选尚未执行新的TypeScript、真实Chrome／IDB和生产构建**，等待磁盘超过4 GiB及串行资源窗口。初始音频候选的已验包保持独立，不代表本候选可运行成品已交付。

已实际执行[最新27项附件纯规则](./纯规则-1794889862189/结果.json)及[当前8 MiB录音／续点22项纯规则](../20261117-Web基础音频/纯规则-1794889863065/结果.json)，各自生产规则与脚本输入前后SHA一致；前者包含boxed String最小floor反例。Store／浏览器fixture不在纯规则执行范围，须用新冻结输入执行，不将纯规则扩大为IDB／UI证明。浏览器脚本与外层运行器仅语法检查通过。

## 二、源码入口和行为 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 入口 | 实现与生命周期 | 当前验证 |
| --- | --- | --- |
| [录音面板](../../../../../代码/前端/Web/Src/AudioRecorderPanel.tsx)／[控制器](../../../../../代码/前端/Web/Src/AudioRecorderController.ts) | 显式开始、停止试听／取消、明确发送；1秒～5分钟／8 MiB，迟到许可／隐藏／卸载／身份切换关track；同步双击只交接一次 | 初始16 MiB冻包22纯／19Chrome已验；本候选增量待新验 |
| [App](../../../../../代码/前端/Web/Src/App.tsx)／[Actions](../../../../../代码/前端/Web/Src/AttachmentSendActions.ts) | 捕获原family／epoch／会话／准入／回复CV，Blob先落盘；原8字段查单，明确同nonce／bytes重试，无能力不回退非幂等上传 | 源码接通；实际新HTTP／浏览器待验 |
| [Store](../../../../../代码/前端/Web/Src/AttachmentSendStore.ts)／[storage](../../../../../代码/前端/Web/Src/storage.ts) | 32项／64 MiB同scope，linked与原Outbox同strict tx且保Blob，接受后释放；本机取消精确冻结原队列，最终POST锁防确认后新增发送 | 新实际IDB待验 |
| [Panel](../../../../../代码/前端/Web/Src/AttachmentSendPanel.tsx)／[十语](../../../../../代码/前端/Web/Src/AttachmentSendLabels.ts) | 默认空态／重载，原单分页、纯查／明确继续、导出、确认停止；旧family仅本机保存／清理，坏holder／repair提示清理不足；继承三主题 | 文案纯规则已验，UI待新Chrome |
| [原责任规则](../../../../../代码/前端/Web/Src/AttachmentSendProtocol.ts) | 身份／manifest／原body／收据白名单，prepared非可用收据；物理holder键、server ID与完整user／conversation／client关联 | 27纯规则覆盖相关边界 |
| [最小拒旧floor](../../../../../代码/前端/Web/Src/AttachmentErasureProtocol.ts) | 独立四标识事实，原单GC后仍拒旧seed；坏floor保原值＋repair，Store同tx拒写 | 纯规则已验，实际IDB待验 |
| [Global](../../../../../代码/前端/Web/Src/GlobalMessages.ts)／[注销清理](../../../../../代码/前端/Web/Src/AccountDeletionCleanup.ts) | Root维护双cursor、先floor再清holder、坏值保Blob／精确pending；原completed同scope全epoch／device擦除与late-write门禁 | Root新纯key规则另报，本候选实际IDB待验 |

可移植工程／准备／运行入口见[Web README](../../../../../代码/前端/Web/README.md)，完整当前代码地图见[Web代码地图](../../../../../代码/前端/Web/代码地图/README.md)。稳定协议定义以[后端媒体上传测试说明](../../../../../测试/后端/接口/media_upload/README.md)及后端当前契约为准；当前用户运行服务未在本轮替换。

## 三、验证入口与保留原件 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- [纯规则入口](../../../../../测试/前端/Web/attachment-rules.cjs/README.md)：Node原生类型擦除，零编译产物／浏览器／网络。
- [真实浏览器入口](../../../../../测试/前端/Web/attachment-browser.cjs/README.md)：实际React／IDB／WebLocks／reload，显式HTTP和身份夹具，零真实API／麦克风。
- [串行运行器](./运行器.cjs)：完整闭包冻结、4 GiB持续监控、每阶段180秒、有效修前错key／假messageId反例、修后新输入、受影响global／outbox／auth／音频／注销／改密／群回归及私有生产构建。
- [活动原单假messageId修前源](./修前-活动原单messageId.ts)及[SHA](./修前-活动原单messageId-SHA.json)、[错namespace修前Global](../20261117-注销本机清理/修前-global错namespace.ts)均封存；只有真实Blob被误删且global提交的AssertionError才算有效红证据。
- [初始音频报告](../20261117-Web基础音频/README.md)保留实际MediaRecorder合成WebM和原403失败；它与本轮虚拟音频bytes／耐久新候选分开，不作相互替代。

## 四、交付与未验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

源码、工程／构建／运行入口已提供；**本候选对应最终可运行包待新构建**。完成后须在本报告补实际冻结SHA、产物路径／SHA及浏览器结果，不能只凭源码宣称GUI交付完成。现有运行包、CA／实例ID及历史证据保留，不覆盖或重启服务。

strict IDB成功只证明浏览器事务完成，不等于系统断电、站点被清理后恢复或qualified ACK。本机停止不能撤回已经送到服务器的消息；导出操作只交给浏览器，不能宣称已写用户磁盘。真实HTTPS、账号、麦克风／权限设备和目标浏览器格式仍独立验收；本报告不宣称第一阶段整体完成。

<a id="🔚" href="#前言">回到首页</a>

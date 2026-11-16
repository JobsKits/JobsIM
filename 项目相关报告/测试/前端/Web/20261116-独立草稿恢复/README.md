# Web独立草稿恢复

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、本端资产增量](#assets)
- [二、本轮证据](#evidence)
- [三、交回owner后的真实窗口](#live-plan)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

2026-11-16，补Web设置内的本人独立草稿恢复，不要求目标会话仍在当前列表。不改后端、S2原服务／Store、会话列表构造或其它端；不访问E2／真实API，不启停服务。当前TypeScript检查、独立真实Chrome10组注入及私有生产构建通过，新候选`index-D_uZ6muW.js`，SHA256 `28137a92f8e3fd3579876bd769fe05bcf181eec71e2aec826e743bf5805c9ce2`。旧D5包／归档／11真实HTTPS／26本地原件保留，不追认新包。

## 一、<span id="assets">本端资产增量</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 入口 | 行为／默认 | 持久化／生命周期／代码位置 | 实际验证 |
| --- | --- | --- | --- |
| 设置→独立草稿恢复 | 正式当前family且S2／cloud_drafts支持、sync ready才显示；候选只标会话标识，不预览旧body | [App Settings](../../../../../代码/前端/Web/Src/App.tsx)接[SelfDraftRecovery](../../../../../代码/前端/Web/Src/SelfDraftRecovery.tsx)；沿sync-personal／self-state-operations原namespace读取，不新建库／会话 | TypeScript＋10组组件注入；未实际登录走正式Settings／真接口 |
| 查看文字 | 非当前或writable=false的本人draft，以及pending／prepared／retired原draft CID；点选精确GET | 当前GET ownedText及服务端明确返回的recovery仅RAM展示；不取原body／quote／proof作云当前值，不读source／云history | retired-only无投影、错误kind／channel／UUID／scope及未知GET反例通过 |
| 复制／清空 | 复制前重取并比对所见version／文字／恢复ID；清空需确认并送原所见version CAS | 不采纳到其它会话，不发消息；新clear仍由原服务保存原operation，旧pending不被本页退休或换proof | stale copy／clear拒、另会话完整输入不变；剪贴板是注入sink |
| 只核对原操作 | 固定resume(id,false)，未知不重投PUT／DELETE，结果不当当前正文 | 原method／route／body／nonce／hash只读；family／namespace／mount序号及排队锁检查，不借新上下文续旧动作 | 原IDB读回、404零写、迟到GET和锁尾排空后零写均通过 |
| 空态／重载／十语主题 | 无候选、失败及加载都有状态与reload，原会话选择有边框／底色 | 复用已有theme tokens／字体／响应式class；[SelfStateLabels](../../../../../代码/前端/Web/Src/SelfStateLabels.ts)新增11键×10语，不新加框架或素材 | 10语×light／dark／解析system的390宽无横溢出；中英藏3截图已查看 |

当前活动会话仍用原details面板。本组件不伪造Conversation以绕当前列表，也不把陌生源、旧epoch、旧family或schema责任缺失当作合法恢复。原channel存在pending／prepared且纯查未知时，既有服务会阻止新clear；显示失败／保原单并保留只读核对，不擅自重投或退休它。

## 二、<span id="evidence">本轮证据</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- `01-local`首轮TypeScript0及Chrome10／10，原日志／截图保留。查看后发现仅aria-pressed不足辨认，随后用现accent／accent-soft补选择态，并加强原IDB意图实际读回与锁尾排空检查；首轮不追认后续输入。
- `02-final-local`最终TypeScript0、Chrome10／10、Vite生产构建0，同轮源码保持不变。[浏览器结果](./02-final-local/Browser/结果.json)明确全部`/api`阻断、私有Vite无proxy、HTTP函数／剪贴板sink为注入；实际操作Chrome／React／IndexedDB／Web Locks。不是后端HTTPS或剪贴板系统集成验证。
- 新Dist在`~/Library/Application Support/JobsIM/Build/Web/DraftRecovery/20261116-174629/Dist`，只新增独立产物，没有切换任何服务web-dir或覆盖仓内Dist／旧包。TypeScript和构建日志及180秒／4GiB硬门禁记录见[Process.json](./02-final-local/Process.json)，门禁未触发；旧D5逐文件摘要核对另封。
- 构建保留第三方Lucide `use client`告警；不修改第三方或SDK。clean后已重新实读Node24.22.1、现有node_modules及磁盘；无下载安装或全局环境／CA／代理变化。

[输入与产品SHA](./输入与产品SHA.json)保存40个当前输入、11个新产物及服务Core未改边界；[旧D5保留核对](./旧D5保留核对.json)逐文件核11个原产物与41个原源码归档，原11真实／26本地结果摘要另存。旧报告与产物没有覆盖。

截图：[中文白天](./02-final-local/Browser/Recovery-zh-CN-light.png)、[英文黑夜](./02-final-local/Browser/Recovery-en-dark.png)、[藏文跟随系统解析](./02-final-local/Browser/Recovery-bo-system.png)。当前独立组件截图不是完整已登录Settings真服务截图。

## 三、<span id="live-plan">交回owner后的真实窗口</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

需在独立授权窗口用新账号／新profile：先在群中保存本人独立文字及回复引用，再真实离群／移除或让source撤回；同步后在Settings发现原群不在当前列表的候选，精确GET只能取当前本人获准文字。验证retired-only原操作恢复、原单未知不创建、source无权仍不读消息／云历史、复制只含独立文字、所见version清空冲突／成功及本机另会话输入保持；退出验证family实际撤销。只有该新包真实结果才能补本节，旧D5／iOS或其它端结果不补算。

本轮没有qualified ACK／after_receipt、正式导出／迁移或全I完成声明。

<a id="🔚" href="#前言">回到首页</a>

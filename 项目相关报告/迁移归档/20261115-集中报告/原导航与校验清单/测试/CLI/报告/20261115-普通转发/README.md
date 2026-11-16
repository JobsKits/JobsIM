# CLI 普通转发限定验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、实际通过](#jobs-toc-heading-2)
- [二、缓存与限制](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮实现引用式文本／附件转发，保持既有本地schema2和治理原操作。不是完整第一阶段验收。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、实际通过</span><!-- jobs-toc:heading-end -->

17组真实SQLite／故障测试通过（含原14组，子用例不另相加）；最终公网JSON／媒体锁版真实HTTPS／PostgreSQL转发专项复跑通过，使用实例CA且验证主机名。[SQLite结果](./SQLite结果.json)、[真实转发结果](./ForwardHTTPS结果.json)、[运行日志](./ForwardHTTPS测试.log)、[源码／二进制摘要](./验证摘要.json)可核对。

`message 源ID`查看当前源内容版本；`forward 目标会话ID 源ID 源内容版本`明确选择该版本。版本变化要求重选；原source proof、目标sendAdmission、ID和epoch先持久化，绝不复制正文／媒体ID／提及。目标草稿保留。

已提交但丢响应时先message-results，只读确认而不重复POST；未到达时先查404，仅明确retry再同ID／原body投递。进程重开保留原proof；明确失效冻结，reconcile找不到也不创建旧意图。临时503不被错误冻结。两类未知由故障服务验证原请求和真实SQLite重开，不冒充生产宕机。

文本与附件真实转发、源版本重选、源编辑失效、本人撤回、进程重开、目标草稿保留和固定消息附件代理精确字节均通过。`download-forward 消息ID | 输出路径`每次先GET当前消息并使用固定同源代理，拒绝覆盖已有文件。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、缓存与限制</span><!-- jobs-toc:heading-end -->

普通消息打印去除临时forwardAdmission；转发缓存只保存无正文／无附件地址的占位，不作为离线旧正文回退。原待发proof属于恢复原请求的必要持久数据，不写入普通展示。明确下载到用户路径的文件属于用户本机副本，不承诺后续可远程撤销；不是强保护或受控导出。

终端已打印的内容无法自动撤回；不把CLI打印／本机文件下载称为强保护。完整导出／迁移、合格ACK、清云、真机、原生Linux／Windows运行及全部I门禁仍须单独验证。未提交推送。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

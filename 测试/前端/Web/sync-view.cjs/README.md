# <span id="前言">Web 同步投影与原意图查单测试</span>

![Jobs出品，必属精品](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、运行与依赖](#jobs-toc-heading-1)
- [二、验证范围](#jobs-toc-heading-2)
- [三、限定验证结果](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、运行与依赖 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[sync-view.cjs](./sync-view.cjs) 使用已有 Node、Playwright、Chrome 与运行中的 Vite，动态导入真实投影模块，并在真实 IndexedDB 中核对原 Outbox proof 的事务 CAS。设置 `JOBSIM_WEB_URL`、`JOBSIM_PLAYWRIGHT_MODULE`，可用 `BROWSER_EXECUTABLE` 指定现有浏览器；运行 `node ./sync-view.cjs`。不安装依赖，不修改全局代理／证书。

默认报告从脚本位置解析到工程根 `项目相关报告/测试/前端/Web/执行记录/sync-view-时间-PID/`；`JOBSIM_SYNC_VIEW_REPORT` 可指定新目录，已有目录拒绝覆盖。执行中相关源码 SHA 改变则失败，仅关闭本脚本创建的浏览器。失败与部分结果均保留，不覆盖旧轮报告。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、验证范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

覆盖单凭 sync 既成消息不能 ACK 原队列、对方伪相同 clientId、初始历史与增量未读／提及、当前阅读会话、最小 global 墓碑与晚源、forward 的 RAM／持久占位分离，以及 sender／route／原 body／sendAdmission 完全匹配才提交的原队列查单 CAS。

所有 `/api/` 网络请求都阻断；投影与 proof callback 明确为本地测试替身，未调用真实消息发送或查单 API，不能记作后端 HTTPS、NATS、合格 ACK 或完整 I 验收。每轮逐项 PASS／FAIL 和源码摘要写入 `结果.json`。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、限定验证结果 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

2026-11-16 本地真实 Chrome 模块／IndexedDB [最终 16 组全部通过](../../../../项目相关报告/测试/前端/Web/执行记录/sync-view-1794762407428-64949/结果.json)，另对完全相同的 7 份产品源码重新执行[原同步存储 12 组](../../../../项目相关报告/测试/前端/Web/执行记录/sync-store-20261115-170917-66499-proof-final/结果.json)，全部通过；两类范围分别记录，不相加为产品验收项。Node24 类型检查通过。

[首次 13 通过／1 失败](../../../../项目相关报告/测试/前端/Web/执行记录/sync-view-1794760837505-53272/结果.json)保留了晚 exact source 覆盖当前 global 墓碑的真实模块反例。修复后新增持久 global stamp 与隔离 holder 的同／更高版本终态反例。各轮分别保存 8 项输入源码快照及 SHA；失败轮的 SyncView 原件由唯一已知补丁在内存逆向恢复，只有精确匹配原结果 SHA 后归档，未改 live 源或原结果。测试没有实际等待服务端期限或访问真实消息数据。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

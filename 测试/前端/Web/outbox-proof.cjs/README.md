# 原请求队列的兼容路径事务反例

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

暂无章节。

</details>
<!-- jobs-toc:end -->

运行同目录 `outbox-proof.cjs`；需要已运行 Vite、现有 Playwright 与 Chrome。变量与[浏览器回归](../browser.cjs/README.md)一致，可用 `JOBSIM_OUTBOX_REPORT` 指定输出目录。默认写入中央唯一时间目录。

真实 IndexedDB 验证旧／新 namespace 下的原 route／完整 body CAS、同 ID 异正文／准入／目标／发送者、查单后队列变化、活动设备指针、迟到身份和畸形 proof 的事务回滚。全部 API 被阻断，proof 为明确测试替身，不证明服务器查单或真实发送；真实路径另看[同步 HTTPS](../sync-https.cjs/README.md)。

当前10组通过，另含opaque32hex／UTF-8字节上限和缺字段／错形态200不能产生proof，见[本轮原始结果](../../../../项目相关报告/测试/前端/Web/执行记录/outbox-proof-1794770379428-9253/原请求队列结果.json)。旧8组原件保留，不累加。

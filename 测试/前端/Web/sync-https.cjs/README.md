# Web 完整快照与增量真实 HTTPS 回归

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

暂无章节。

</details>
<!-- jobs-toc:end -->

先准备新的同步生产构建，由独立网关同源提供。设置 `JOBSIM_SYNC_TEST_SEED` 指向工程外明确标记 `freshSyncRealm` 和 `persistentData` 的私密 seed，运行同目录 `sync-https.cjs`。本入口不安装依赖；可用 `JOBSIM_PLAYWRIGHT_MODULE`、`BROWSER_EXECUTABLE`、`JOBSIM_SYNC_REPORT` 指定既有工具和输出。

该专项创建自己的两个测试账号、好友关系、群和 96 条初始消息，随后对自己的夹具编辑、转让、移除及撤回；不修改数据库、限流、他人账号，不启停服务。浏览器故意中断第二页后验证原始请求恢复；另在独立空浏览器存储中仅复制本测试自己的不透明 Cookie，验证旧设备登记不可继承。Cookie 和账号秘密仅留本轮内存，不写报告。

范围为真实多页快照、原始字节摘要、首次增量权限核验、IndexedDB 完整安装与连续游标、刷新页面、存储实例隔离、失权保留合法普通本地副本及后续 global 清理。不发送 qualified ACK，不证明全部第一阶段同步、云草稿／服务端已读、正式迁移或目标移动系统。

当前生产包`index-DXdl9E2N`已通过10组。新增一条opaque32hex client ID消息，以及未提交原意图的辅助只读查单503：仅该查单的503由浏览器注入，随后真实撤回仍提交global，原发送body／ID保持且不重发。此前`index-mqQfztei`的9组原件保留，不累加。详见[最新固定检查点](../../../../项目相关报告/测试/前端/Web/20261116-权威同步检查点/README.md)。

Node 明确验证本实例 CA 和主机名，浏览器只信任本轮公开证书的 SPKI，不导入系统信任或忽略所有证书错误。默认报告写入[中央执行记录](../../../../项目相关报告/测试/前端/Web/执行记录/)下的新目录。失败摘要和截图标为未验收，不覆盖旧成功证据。

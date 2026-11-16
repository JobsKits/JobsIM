# Web 同步协议校验反例

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

暂无章节。

</details>
<!-- jobs-toc:end -->

运行同目录 `sync-protocol.cjs`，使用 Web 工程已安装的 TypeScript 和 Node 内置 WebCrypto；不安装依赖、不请求 API、不操作数据库。`JOBSIM_SYNC_REPORT` 可指定报告目录，默认按时间和进程号写入[中央执行记录](../../../../项目相关报告/测试/前端/Web/执行记录/)。

覆盖超出 JavaScript 安全整数的精确位置、原始 Go JSON 转义字节、页面／正文摘要替换、版本绑定矛盾、缺页、分类水位推进、显式 redacted 连续范围、事件缺口与重复 origin，以及权限／恢复代际围栏。仅验证协议解析器，不能证明浏览器持久化、服务端或完整同步已交付。

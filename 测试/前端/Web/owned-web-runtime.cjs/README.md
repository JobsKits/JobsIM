# 切换受管 Web 测试产物

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

这个测试工具只短暂停启显式选定的 `54443` 隔离域 App，用固定后端运行另一个已构建 Web 目录。当前是 `--service=all` 单 App 的十个 listeners，不存在可单独停启 gateway 的 PID。PG、NATS、数据库、证书与实例身份保留。

入口为 `node owned-web-runtime.cjs`，必须同时显式提供：

- `--environment-file <当前有效 manifest>`：包含原 PID、完整 argv、实例与私有依赖寻址。
- `--web-dir <已构建目录>` 与 `--expected-binary-sha <已核对后端 SHA256>`。
- `--allow-owned-app-restart --clients-closed`：调用方已获当前任务授权，且所有占用该域的客户端测试已明确关闭并释放窗口。

脚本先打印影响，核固定二进制、完整 argv、listener PID、实例／epoch、S2／云草稿能力与 4GiB 操作门禁。精确备份原 manifest 为 0600；仅向匹配 PID 发送 SIGTERM，有界等待、不强杀。启动复用完整原 DB／PUBLIC_URL／NATS 环境，严格 CA 就绪及静态产物核验后原子更新 manifest PID。失败保留原始日志，尝试用完整原环境恢复原 argv；恢复失败会明确报告，不宣称原实例已恢复。

不安装依赖、不修改代理、不发消息、不清理用户数据或任何共享缓存。私密输入和日志保留在工程外 manifest 同目录的独立 `WebRuntime-*`；公开摘要只含身份、SHA、PID、范围及结果。后续工具必须读取新 manifest，不能继续使用退休 PID。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

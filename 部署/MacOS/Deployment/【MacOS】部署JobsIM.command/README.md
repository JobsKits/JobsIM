# <span id="前言">【MacOS】部署JobsIM.command</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、职责与边界](#jobs-toc-heading-1)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、职责与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

同名[原生入口](./【MacOS】部署JobsIM.command)内嵌生命周期及资源原点，按[平台说明](../../README.md)操作。默认进入fzf双模式部署；服务器模式须明确URL及SAN证书，本地模式有独立公开确认。普通stop/backup为运维子动作，不代替反部署。

选择和网页菜单使用备用屏全屏重绘，退出归还原屏；Esc收起、回车重开、Ctrl+C中断本轮的行为及旧运行会话的生效边界见[平台菜单说明](../../README.md)。

二维码先以真实TTY尺寸核可显示空间；足尺寸用背景方格，扫码后回车打开网页菜单；尺寸不足或NO_COLOR自动预览本轮本地PNG，非TTY不等待或打开图片，回环仍仅本机可用。
本入口同样自包含自托管NATS生命周期：只读check不运行工具探针；正常部署执行官方目标与能力核验；仅loopback、私有token／配置、Application/JetStream持久化，先应用后broker停止，备份范围明确含broker。角色／端口按当前Go源码发现，当前9内部角色。参数、私密路径及原生未验边界见[平台完整说明](../../README.md#nats)。

业务与独立 journal 集群均使用回环 TCP，禁用 Unix socket；长实例路径不会触发 socket 路径上限。数据库目录、集群标识和密钥继续保留。

TLS 先私有暂存并严格验真，再提交到实例；旧材料不合格或残缺时保留并停止，不自动重置 CA。完整处理与受控修复边界见[平台说明](../../README.md#connection)。

本机默认实例使用Git忽略的 `.jobsim-local/instance.ref` 接续工作树搬迁。只记录公开实例ID／部署ID／uid；私有权限、原目录缺失与原state身份均须匹配。只读模式不转移owner；确认操作且取得锁后保留旧owner并原子接续。显式其他state不借此引用接管，克隆不携带本机引用。详见[搬迁实现与验证](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/工作树搬迁/README.md)。

清理默认保留独立journal、Application/instance.json、Secrets、TLS、外置备份/恢复钥及原点审计，结果为partial；链接、未知归属、备份失败或确认后变化均保留并停止。已有反部署断点不能用run重新初始化。

<a id="🔚" href="#前言">回到首页</a>

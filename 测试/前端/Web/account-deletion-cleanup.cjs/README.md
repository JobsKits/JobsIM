# JobsIM Web 注销本机清理规则

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、执行与范围](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

[入口](./account-deletion-cleanup.cjs)直接加载生产`AccountDeletionCleanup.ts`，核对已验证原身份的精确本机数据归属。使用现有[**Node.js**](https://nodejs.org/)24的原生类型剥离，不下载依赖、不启动浏览器或服务。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、执行与范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

从本目录执行：

```shell
node ./account-deletion-cleanup.cjs
```

14组规则涵盖原实例／用户／设备／epoch、旧命名空间与archive、所有已维护载荷前缀、阶段UUID、原操作后缀、多账号隔离、原收据和最小门禁保留、未知key与畸形／装箱身份拒绝。`receipt-queries`原查询任务按完整命名空间归属清理，其他用户／实例／来源保留，账号注销与data-rights收据不因此删除。结果与执行前后源码SHA写入项目中央`项目相关报告/测试/前端/Web/20261117-注销本机清理/`，不改暂存或运行实例。

本入口只证明生产归属规则；实际严格IDB事务、Blob清理、完成收据与迟到写阻断必须由浏览器用例另外验证。不能把规则通过写成完整注销或本机物理擦除通过。

<a id="🔚" href="#前言">回到首页</a>

# 注销普通准入门禁测试

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行](#jobs-toc-heading-1)
- [二、边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行</span><!-- jobs-toc:heading-end -->

[account_deletion_admission.mjs](./account_deletion_admission.mjs)沿[主动改密专项](../password_change.mjs/README.md)的显式隔离PG与overlay方式，增加注销状态反例，并重跑原改密规则。任意启动目录均从脚本位置解析根目录；Go可执行文件通过`--go`显式指定，公共模块缓存沿既有`GOCACHE`，不重装、清理或迁移环境。

```sh
node 测试/后端/接口/account_deletion_admission.mjs/account_deletion_admission.mjs --go "$GO_BINARY" --with-pg
```

需显式`JOBS_IM_SYNC_SCHEMAS_TEST=1`、`JOBS_IM_SYNC_SCHEMAS_REALM_URL`、`JOBS_IM_SYNC_SCHEMAS_ADMIN_URL`和私有`PGPASSFILE`；只在已授权维护连接上新建随机空库并逐库清理。4GiB门禁、单worker、离线模块及有界超时保持；每轮将前后输入SHA、Go原始JSONL及退出码追加到[中央报告](../../../../项目相关报告/测试/后端/20261116-注销普通准入门禁/README.md)。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、边界</span><!-- jobs-toc:heading-end -->

测试直接设置本轮随机库的deletion_state，是状态夹具，不是执行注销申请／取消或最终物理清理。生产auth／journal在该随机库，跨owner依赖是显式进程内确认夹具；不证明真实HTTPS、独立日志故障域恢复、五端UI或全I验收。通过事件含表驱动父组与子项，报告按名称分列，不相加为产品完成数。

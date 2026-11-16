# 本人主动改密专项入口

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行范围](#jobs-toc-heading-1)
- [二、真实PostgreSQL](#jobs-toc-heading-2)
- [三、反例与敏感信息](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行范围</span><!-- jobs-toc:heading-end -->

[password_change.mjs](./password_change.mjs)通过Go overlay加载工程外置测试，不复制测试到生产模块，不下载依赖；4 GiB磁盘门禁、有界超时、单worker及前后输入SHA固定。结果追加到[中央报告](../../../../项目相关报告/测试/后端/20261116-本人主动改密/README.md)，失败原件不覆盖。

从项目任意启动目录都可执行入口；`--go`传入已验证的Go可执行文件。需要外置warm cache时通过`GOCACHE`指定，本入口不清理或迁移共享缓存。

```sh
node 测试/后端/接口/password_change.mjs/password_change.mjs --go "$GO_BINARY"
```

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、真实PostgreSQL</span><!-- jobs-toc:heading-end -->

显式配置`JOBS_IM_SYNC_SCHEMAS_TEST=1`、`JOBS_IM_SYNC_SCHEMAS_REALM_URL`、`JOBS_IM_SYNC_SCHEMAS_ADMIN_URL`及私有`PGPASSFILE`后增加`--with-pg`。沿已有受控PG guard，只在指定维护库创建随机`jobsim_sync_schemas_*`空库，结束逐库删除，不结束共享连接或删除既有业务库。TLS地址仅作隔离门禁，本专项不访问它。

测试重复执行完整identity迁移，并使用生产auth／journal处理器、真实Argon2id、两种session family与撤销触发器。journal仅位于本次随机库，message／governance是显式进程内确认夹具；不证明真实HTTPS、独立物理日志集群恢复、跨owner实际下载排空或任一原生客户端。真实恢复与源排空已有旧切片证据，不能追认覆盖新改密源。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、反例与敏感信息</span><!-- jobs-toc:heading-end -->

覆盖未知单只查、错误本人／设备／epoch／密码、管理员MFA、intent失败后密封恢复、提交后result失败、迟到旧单Cookie、全family撤销、旧epoch退休、独立日志门禁、并发同单及匿名查单限流。公开报告只留断言、SHA和结果，不记录密码、OTP、token、替换哈希或密文。需要复现的私有连接凭据仍在工程外。

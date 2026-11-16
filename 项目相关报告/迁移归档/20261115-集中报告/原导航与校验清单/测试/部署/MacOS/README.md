# MacOS 部署测试

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [运行方式](#jobs-toc-heading-1)
- [2026-11-14 实际结果](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">运行方式</span><!-- jobs-toc:heading-end -->

[自包含测试入口](./运行测试.command/运行测试.command)默认执行不启动服务的检查：

```sh
./运行测试.command/运行测试.command unit
```

验证 MacOS／Linux 实现语法、check／plan不创建state、拒绝陌生非空state并保留哨兵文件。PowerShell解析与非Windows拒绝是另外执行的宿主检查，不在此unit入口中冒充Windows测试。

集成模式会创建真实测试账户、七个数据库、独立journal集群和加密备份，必须显式指定源码外测试目录：

```sh
JOBS_IM_TEST_STATE="$HOME/Library/Caches/JobsIM/DeploymentChecks/isolated" \
  ./运行测试.command/运行测试.command integration --yes
```

默认端口19443／17543／17544／20101～20108，可通过部署变量覆盖。冲突时拒绝接管；不要指向正在使用的实例。使用正式部署／停止实现完成构建、启动、严格TLS、Node内置fetch双账户链路、默认加密备份和保留数据停止。运行日志与结果位于报告时间目录；私钥、口令和数据库不进入报告。退出时只终止本次启动的控制器；数据留供核查。

[ContractSmoke.mjs](./ContractSmoke.mjs)要求显式 HTTPS 地址和当前进程专用 `NODE_EXTRA_CA_CERTS`。只注册测试账户、接受联系人、创建单聊、发送带restoreEpoch的文本、以原clientMsgId重试、使用epoch:seq读取对端历史并登出；不放宽TLS或把账号令牌打印到日志。不能指向生产账号实例。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">2026-11-14 实际结果</span><!-- jobs-toc:heading-end -->

[自动集成记录](./报告/20261114-233934/result.txt)、[启动日志](./报告/20261114-233934/run.log)、[聊天结果](./报告/20261114-233934/smoke.log)、[备份停止日志](./报告/20261114-233934/backup-stop.log)均来自本机实际执行。工具为 Go1.27.4、PostgreSQL18.7、Node26.13.1、OpenSSL4.1.0；这些是本轮实际版本，未声称第三方最新。

- 两个独立PG数据目录、六业务域库／用户与journal库真实创建；Go／Web构建成功，HTTPS严格验证通过。
- 两账户注册、联系人接受、文本发送／原ID重试对账和接收历史通过，restoreEpoch为十进制字符串。
- 七库custom dump、pg_restore目录、CMS AES256-GCM加密、解密目录验真和SHA256成功；默认stop后本轮服务和两PG退出，原数据与密钥保留。
- 另一次手动同state重开保留deploymentId；对该运行控制器发送SIGINT后，本实例服务与两PG均退出，原有8443联调服务仍运行。该手动检查未自动化为GUI终端关闭测试。
- 最新路径隔离保护变更已通过unit检查；核心服务／备份链路无改动。Linux仅Bash解析、Windows仅PowerShell宿主解析／帮助／平台阻断。

未执行真实断电、数据库还原／恢复代次演练、升级回滚、异机恢复、目标容量、二维码、完整管理后台或生产强保护验收。备份目录可解密不等于数据库能恢复。独立journal不能随业务库回滚，部署入口未提供自动restore。

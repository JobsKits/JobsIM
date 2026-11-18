# MacOS 部署测试

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [运行方式](#jobs-toc-heading-1)
- [2026-11-14 实际结果](#jobs-toc-heading-2)
- [自托管 NATS 独立专项](#jobs-toc-heading-3)
- [成对反部署的当前入口](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

当前验收入口为[成对部署／反部署汇总](../../../项目相关报告/测试/部署/20261118-成对部署反部署/MacOS/summary.json)：真实服务器模式与仅本机模式、50项安全夹具、16项搬迁门禁、独立NATS协议专项及最终H5严格HTTPS资产增量分别记录源SHA和范围。默认用户实例不参与测试，公网穿透与正式生产环境仍需另外验收。

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">运行方式</span><!-- jobs-toc:heading-end -->

[自包含测试入口](./运行测试.command/运行测试.command)默认执行不启动服务的检查：

```sh
./运行测试.command/运行测试.command unit
```

验证MacOS及Ubuntu／Debian／RockyLinux／AlibabaCloudLinux当前实现语法、check／plan不创建state、拒绝陌生非空state并保留哨兵文件。PowerShell解析与非Windows拒绝是另外执行的宿主检查，不在此unit入口中冒充Windows测试。

[成对原生集成](./成对原生集成.py/README.md)使用已有Python作为测试驱动，自动创建全新的源码外私密目录、独立端口、合成SAN证书及真实测试账户，实际经PTY/fzf操作正式服务器部署和默认备份反部署；不使用已有state或全局信任：

```sh
./运行测试.command/运行测试.command integration --yes
```

端口由本轮动态选择；冲突拒绝接管，不指向用户已运行实例。当前正式反部署按原点清理业务资源，保留独立journal、Application/instance.json、Secrets/TLS、外置备份和恢复钥，结果为partial。日志、完整域dump目录验真、前后SHA和源码/严格HTTPS静态资产绑定由独立报告记录；真实公网Quick和生产服务器可达性另验。

[ContractSmoke.mjs](./ContractSmoke.mjs)要求显式 HTTPS 地址和当前进程专用 `NODE_EXTRA_CA_CERTS`。只注册测试账户、接受联系人、创建单聊、发送带原restoreEpoch／sendAdmission的文本、以原clientMsgId重试、使用epoch:seq读取对端历史并登出；不放宽TLS或把账号令牌打印到日志。不能指向生产账号实例。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">2026-11-14 实际结果</span><!-- jobs-toc:heading-end -->

[自动集成记录](../../../项目相关报告/测试/部署/MacOS/20261114-233934/result.txt)、[启动日志](../../../项目相关报告/测试/部署/MacOS/20261114-233934/run.log)、[聊天结果](../../../项目相关报告/测试/部署/MacOS/20261114-233934/smoke.log)、[备份停止日志](../../../项目相关报告/测试/部署/MacOS/20261114-233934/backup-stop.log)均来自本机实际执行。工具为 Go1.27.4、PostgreSQL18.7、Node26.13.1、OpenSSL4.1.0；这些是本轮实际版本，未声称第三方最新。

- 两个独立PG数据目录、六业务域库／用户与journal库真实创建；Go／Web构建成功，HTTPS严格验证通过。
- 两账户注册、联系人接受、文本发送／原ID重试对账和接收历史通过，restoreEpoch为十进制字符串。
- 七库custom dump、pg_restore目录、CMS AES256-GCM加密、解密目录验真和SHA256成功；默认stop后本轮服务和两PG退出，原数据与密钥保留。
- 另一次手动同state重开保留deploymentId；对该运行控制器发送SIGINT后，本实例服务与两PG均退出，原有8443联调服务仍运行。该手动检查未自动化为GUI终端关闭测试。
- 最新路径隔离保护变更已通过unit检查；核心服务／备份链路无改动。Linux仅Bash解析、Windows仅PowerShell宿主解析／帮助／平台阻断。

历史该轮未执行真实断电、数据库还原／恢复代次演练、升级回滚、异机恢复、目标容量、二维码、完整管理后台或生产强保护验收。备份目录可解密不等于数据库能恢复。独立journal不能随业务库回滚，部署入口未提供自动restore。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">自托管 NATS 独立专项</span><!-- jobs-toc:heading-end -->

[专用入口](./NATS专项.command/NATS专项.command)只启动本轮随机端口broker，不构建Go／Web、不初始化PG、不接管既有realm。必须显式`--yes`；可用`JOBS_IM_NATS_BIN`指定已安装工具，测试另支持`JOBS_IM_TEST_NODE`指定既有健康Node。token只从私有文件读取，协议测试不把秘密放argv／报告。

```sh
zsh ./NATS专项.command/NATS专项.command --yes
```

本节[NATS限定证据](../../../项目相关报告/测试/部署/NATS/README.md)记录缺／错token拒绝、JetStream file发布／重启读回、token保留、秘密不进argv／日志、配置漂移／陌生PID拒绝及冷态broker清单；专项自身不执行PG／应用／加密备份。完整链路见上方当前汇总，2026-11-14七库旧记录仍保持原义；本机证据不代表Linux／Windows原生验收或broker灾难恢复。

双集群长路径的限定验证见[双PG长路径报告](../../../项目相关报告/测试/部署/MacOS/20261116-双PG长路径/README.md)：真实执行两份 MacOS 实现的集群函数，在中文、空格且旧式 socket 路径为189字节的隔离目录中启停双集群，并验证禁用 Unix socket、仅回环 TCP、重启保持身份／密码／测试行及最终端口释放。该专项未执行应用、NATS或加密备份流程，未操作既有实例。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">成对反部署的当前入口</span><!-- jobs-toc:heading-end -->

[安全夹具](./反部署安全.mjs/README.md)读取两份正式MacOS源码，验证真实文件系统清理与协议注入反例；[原生集成](./成对原生集成.py/README.md)独立验证真实双PG/NATS、严格TLS、双账户、完整加密备份及保护性partial清理。失败与后续审计单独保留，不改历史SHA，不凭退出0或旧summary追认缺项归档。

[菜单缩放专项](./菜单缩放.py/README.md)只运行真实PTY／fzf和临时假服务，覆盖备用屏尺寸变化、选择／收起／重开及控制器中断，不为菜单验证重启真实部署。

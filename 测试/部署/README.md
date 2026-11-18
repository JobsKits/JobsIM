# JobsIM 部署测试

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

暂无章节。

</details>
<!-- jobs-toc:end -->

| 范围 | 测试入口／记录 | 证据边界 |
| --- | --- | --- |
| MacOS | [运行测试](./MacOS/README.md) | 受控双PG／NATS／HTTPS、真实PTY选择及配对反部署，按当前报告输入核对 |
| 四Linux发行版 | [隔离验证](./Linux/README.md) | 八个Bash实现／desktop转交、真实私有文件／CMS与保护反例；非目标Linux原生执行 |
| Windows | [生命周期测试](./Windows/README.md) | PowerShell解析／主机函数、真实CMS／SAN及故障反例；非Windows原生SQL／ACL／Job或桌面验收 |
| 公共版本 | [官方目标及模块报告](./依赖版本/README.md) | 显式transport反例、数值比较和进程回收，真实网络查询另列 |
| 离线二维码 | [生成及实际解码](./二维码/README.md) | HTML／SVG／PNG、凭据URL拒绝、Mac实际PNG解码；不等于真实手机扫码 |

[当前需求符合性与分项证据](../../项目相关报告/测试/部署/20261118-成对部署反部署/README.md)以需求2.11、T-44～46／ARCH-33逐项对照，不以某入口退出0代替完整交付。

测试源码放在本目录，部署实现位于[部署](../../部署/README.md)。测试不接管正在使用的联调服务；集成模式必须明确选择外置测试state，创建的测试账户、数据库、媒体和备份默认保留。不得用演示、语法检查或归档可列目录代替真实服务与恢复验收。

[NATS部署限定验证](../../项目相关报告/测试/部署/NATS/README.md)：新增loopback JetStream归属、私有token／配置、持久范围及先应用后broker停止。Mac真实独立broker与只读检查通过；Linux语法、Windows Parser／C#桥仅宿主检查，非目标系统执行。当前完整八库＋NATS聚合恢复仍未验，旧PG报告不覆盖broker。

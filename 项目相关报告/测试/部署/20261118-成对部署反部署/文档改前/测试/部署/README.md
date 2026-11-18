# JobsIM 部署测试

[toc]

| 平台 | 入口／记录 | 当前证据 |
| --- | --- | --- |
| MacOS | [运行测试](./MacOS/README.md) | 真实独立双PG、严格TLS聊天、源码构建、加密备份及停止；恢复未验 |
| Linux | [部署实现](../../部署/Linux/README.md) | Bash语法检查；本机拒绝非Linux。未目标机启动／停止／备份 |
| WindowsOS | [部署实现](../../部署/WindowsOS/README.md) | PowerShell7.7解析、帮助及拒绝非Windows。未目标机ACL／运行／Ctrl+C／备份 |

测试源码放在本目录，部署实现位于[部署](../../部署/README.md)。测试不接管正在使用的联调服务；集成模式必须明确选择外置测试state，创建的测试账户、数据库、媒体和备份默认保留。不得用演示、语法检查或归档可列目录代替真实服务与恢复验收。

[NATS部署限定验证](../../项目相关报告/测试/部署/NATS/README.md)：新增loopback JetStream归属、私有token／配置、持久范围及先应用后broker停止。Mac真实独立broker与只读检查通过；Linux语法、Windows Parser／C#桥仅宿主检查，非目标系统执行。当前完整八库＋NATS聚合恢复仍未验，旧PG报告不覆盖broker。

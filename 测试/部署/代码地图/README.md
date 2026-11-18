# 部署测试代码地图

| 文件 | 职责 |
| --- | --- |
| [MacOS运行入口](../MacOS/运行测试.command/运行测试.command) | unit只读保护与显式integration真实生命周期 |
| [聊天契约smoke](../MacOS/ContractSmoke.mjs) | 严格TLS、双账户、联系人、发送对账与epoch游标 |
| [MacOS结果](../MacOS/README.md) | 实际执行记录与未验边界 |
| [MacOS反部署安全](../MacOS/反部署安全.mjs/README.md) | 归属／快照／保护目录、条件链错误传播与访问模式故障 |
| [MacOS成对原生集成](../MacOS/成对原生集成.py/README.md) | 独立双PG／NATS、严格TLS、完整归档与真实反部署 |
| [Linux入口验证](../Linux/验证入口.mjs/README.md) | 四发行版同合同、desktop路径、隔离函数及加密备份夹具 |
| [Windows生命周期](../Windows/生命周期.ps1/README.md) | PowerShell主机函数、归属保护、备份失败及平台拒绝 |
| [依赖版本](../依赖版本/运行测试.mjs/README.md) | 官方元数据、目标包及校验规则 |
| [二维码](../二维码/验证二维码.mjs/README.md) | 真实PNG解码、受限URL与离线HTML／SVG |
| [平台总览](../README.md) | 六系统独立入口、主机与目标机验收区分 |

[NATS独立专项](../MacOS/NATS专项.command/NATS专项.command)直接加载当前生产函数的测试副本，禁用其入口／清理钩子；[协议验证](../MacOS/NATS协议.mjs/NATS协议.mjs)仅以Node内置TCP实现私有测试broker的认证与JetStream持久回读，无新增包依赖。

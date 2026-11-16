# 部署测试代码地图

| 文件 | 职责 |
| --- | --- |
| [MacOS运行入口](../MacOS/运行测试.command/运行测试.command) | unit只读保护与显式integration真实生命周期 |
| [聊天契约smoke](../MacOS/ContractSmoke.mjs) | 严格TLS、双账户、联系人、发送对账与epoch游标 |
| [MacOS结果](../MacOS/README.md) | 实际执行记录与未验边界 |
| [平台总览](../README.md) | Linux／Windows语法与目标机验收区分 |

[NATS独立专项](../MacOS/NATS专项.command/NATS专项.command)直接加载当前生产函数的测试副本，禁用其入口／清理钩子；[协议验证](../MacOS/NATS协议.mjs/NATS协议.mjs)仅以Node内置TCP实现私有测试broker的认证与JetStream持久回读，无新增包依赖。

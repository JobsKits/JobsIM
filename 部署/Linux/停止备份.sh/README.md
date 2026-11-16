# 停止备份.sh

同名目录中的 [停止备份.sh](./停止备份.sh) 为原生自包含实现。环境、变量、确认和恢复边界以[平台 README](../README.md)为准；执行前先 `check`。默认动作停止并加密备份，数据不会删除。

本入口同样自包含自托管NATS生命周期：缺既有工具check失败；仅loopback、私有token／配置、Application/JetStream持久化，先应用后broker停止，备份范围明确含broker。角色／端口按当前Go源码发现，当前9内部角色。参数、私密路径及原生未验边界见[平台完整说明](../README.md#自托管-nats-jetstream)。

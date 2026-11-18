# 停止备份.sh

备份所需双集群只使用回环 TCP，禁用 Unix socket；显式 `run` 才应用 TLS 私有暂存与严格验真，`stop / backup` 不替换证书。旧材料保留规则见[平台说明](../README.md)。

同名目录中的 [停止备份.sh](./停止备份.sh) 为原生自包含实现。环境、变量、确认和恢复边界以[平台 README](../README.md)为准；执行前先 `check`。默认动作停止并加密备份，数据不会删除。

本入口同样自包含自托管NATS生命周期：缺既有工具check失败；仅loopback、私有token／配置、Application/JetStream持久化，先应用后broker停止，备份范围明确含broker。角色／端口按当前Go源码发现，当前9内部角色。参数、私密路径及原生未验边界见[平台完整说明](../README.md#自托管-nats-jetstream)。

本机默认实例使用Git忽略的 `.jobsim-local/instance.ref` 接续工作树搬迁。只记录公开实例ID／部署ID／uid；私有权限、原目录缺失与原state身份均须匹配。只读模式不转移owner；确认操作且取得锁后保留旧owner并原子接续。显式其他state不借此引用接管，克隆不携带本机引用。详见[搬迁实现与验证](../../../项目相关报告/实现进度/20261116-剩余能力闭环/工作树搬迁/README.md)。

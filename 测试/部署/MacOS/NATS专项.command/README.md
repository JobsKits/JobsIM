# NATS 隔离测试入口

显式`--yes`后创建本轮Application Support私有目录和随机端口，只启停受管NATS；不创建PG、不构建应用、不触现有realm。直接载入生产函数前缀，起止源码SHA一致且完整执行标志成立才报告通过。测试退出保留数据与中央报告。

[完整范围与证据](../README.md#自托管-nats-独立专项)。Node仅为外置测试依赖，部署／停止本体不增加Python／Node辅助守护依赖（原Web构建仍需Node／npm）。

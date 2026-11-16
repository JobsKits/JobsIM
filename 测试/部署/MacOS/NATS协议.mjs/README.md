# NATS 协议验证

测试专用Node脚本，需由隔离Shell入口设置本轮端口、token文件、固定测试ID和自有PID。令牌只在内存／loopback网络，不作参数；检查错／缺token拒绝、JetStream file写入和重启回读。不要独立指向共享或生产broker。

[完整范围与证据](../README.md#自托管-nats-独立专项)。Node仅为外置测试依赖，部署／停止本体不增加Python／Node辅助守护依赖（原Web构建仍需Node／npm）。

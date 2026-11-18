# 群治理主机回归

从任意工作目录执行 `node RunGroup.mjs`。脚本从自身定位项目，复用本机已有 Harmony CLT TypeScript 编译器，不联网、不安装工具、不连接实例。

运行前保留至少 4 GiB 可用磁盘；每次将当前源码摘要、类型结果和测试原始输出写入 `项目相关报告/实现进度/20261116-剩余能力闭环/群控证据/HarmonyHost-*`，保留历史。服务及实际 `GroupStore.ets` 在主机 HTTP/RDB 适配下执行，不能替代 ArkTS 编译或设备验证。

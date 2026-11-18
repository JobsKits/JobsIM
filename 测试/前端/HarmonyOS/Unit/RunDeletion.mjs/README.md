# 鸿蒙本人注销主机入口

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

[运行入口](./RunDeletion.mjs)转译当前Jobs自有注销service、规则与RDB仓库，复用现有镜像SDK TypeScript和系统SQLite桥接，不下载工具、不连接实际账号。

```shell
node 测试/前端/HarmonyOS/Unit/RunDeletion.mjs/RunDeletion.mjs
```

运行前检查4GiB空闲门禁；当轮独立写入中央`注销证据/HarmonyHost-*`，保存类型日志、实际结果、源码摘要与中间模块，不覆盖旧证据。12项生产service／SQLite适配反例已通过，不等于设备／HUKS／HTTPS。

## 一、证据与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

实际包与源码冻结见[两端注销报告](../../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS注销.md)，其他主机入口见[上级README](../README.md)。原生HAP／Hypium执行分别验证；测试失败保留实际非零退出码，不把设备缺席记作通过。

<a id="🔚" href="#前言">返回前言</a>

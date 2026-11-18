# 鸿蒙主机回归入口

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

这些入口转译当前 Jobs 自有 ArkTS，使用现有 [**Node.js**](https://nodejs.org/) 与镜像 SDK 中的 [**TypeScript**](https://www.typescriptlang.org/) 执行主机反例；不下载工具，不连接正式服务，不代表鸿蒙设备、Hypium、RDB NAPI、AssetStore 或系统文件提供方通过。

## 一、运行与依赖 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

从仓库根运行：

```shell
node 测试/前端/HarmonyOS/Unit/RunAuth.mjs
node 测试/前端/HarmonyOS/Unit/RunSync.mjs
node 测试/前端/HarmonyOS/Unit/RunExport.mjs
node 测试/前端/HarmonyOS/Unit/RunPassword.mjs/RunPassword.mjs
node 测试/前端/HarmonyOS/Unit/RunDeletion.mjs/RunDeletion.mjs
node 测试/前端/HarmonyOS/Unit/RunGroup.mjs/RunGroup.mjs
```

`JOBS_IM_HARMONY_CLT_HOME` 可指定已有 CLT，默认从 JobsIM 用户持久工具链目录读取镜像 SDK；缺失时停止。Sync／Export 的 SQLite 适配另使用已有系统 Python。来源与原生构建前置条件见[上级测试说明](../README.md)。

| 入口 | 行为与输出 |
| --- | --- |
| `RunAuth.mjs` | 转译 Models／Strings／AuthCoordinator／Api 及 Api 所需的 ExportProtocol→SyncProtocol／SelfProtocol 纯依赖；测试只替换原生网络／文件端口，按暂存编译模块自身位置解析内部 require。每轮独立输出到用户持久 Build/Harmony 的 `AuthRules-*`，保存结果与源摘要；当前 41 项 |
| `RunSync.mjs` | 实际同步／本人状态实现与 Python SQLite 桥接；每轮独立 `Authoritative-*`，保存结果及摘要；当前 35 项 |
| `RunExport.mjs` | 原意图、仓库 CAS、流下载／刷新、Node 文件与选择器适配，执行前检查 4GiB 空闲门禁；中央正式导出目录中新建 `Host-*`，保存结果、类型日志和源摘要，清除本轮生成 JS／TS；当前 18 项 |
| [RunPassword.mjs](./RunPassword.mjs/README.md) | 本人改密原责任、纯查／明确重试、旧 family 围栏、秘密不落库及实际仓库 SQLite CAS；中央实现进度中新建 `HarmonyHost-*`，当前14项；认证入口现另含两条改密暂停／新family反例，共43项 |

测试或类型失败保留非零退出码；每轮使用独立目录，不覆盖旧证据。生成的主机中间文件与原生 HAP 分开记录，不能据主机结果宣称设备可编译或可运行。

## 二、验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[当前全项目回归](../../../../项目相关报告/测试/回归/20261116-全项目回归/Android-HarmonyOS.md)分别保留认证 36 过／5 失败及修后 41 过、同步 35 过、导出 18 过；原生应用／42 方法测试 HAP 编译另验。没有 HDC 目标时不会安装或执行设备用例。



本轮追加能力撤回与恢复代际反例：旧原单允许匿名纯查询；全部续跑必须重新通过能力门禁，404 未建立原单须由原家族 AT 创建。更新验证为鸿蒙15项主机通过、两端应用与测试包重新编译；Android新增第5项 Room 反例仅已编译，未执行设备测试。


新增[注销主机入口](./RunDeletion.mjs/README.md)和[协议反例](./Deletion.test.cjs/README.md)，12项真实生产service＋系统SQLite适配通过。当前认证45项、改密16项host通过；各自范围重叠不相加成产品项；原生应用／50方法test HAP只编译未执行设备。

[群控主机入口](./RunGroup.mjs/README.md)现执行20项，含当前生产service、严格分享码、原nonce／代际与实际GroupStore的SQLite/CAS；[注销12项](./RunDeletion.mjs/README.md)追加群控表按本人前缀清理。两者均不连接用户实例；本轮54方法test HAP只编译，不据host推断设备。

<a id="🔚" href="#前言">返回前言</a>

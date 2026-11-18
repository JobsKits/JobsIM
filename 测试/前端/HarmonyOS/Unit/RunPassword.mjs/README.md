# 鸿蒙本人主动改密主机回归

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

[入口](./RunPassword.mjs)执行当前 Jobs 自有改密协议与仓库代码的主机反例；使用现有 Node.js、镜像 SDK 的 TypeScript 和已有 Python SQLite 桥接，不下载工具或连接实际服务器。网络、凭据与原生 RDB 端口均为明确的测试适配，不是鸿蒙设备证据。

## 一、运行与输出 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

在仓库根执行：

```shell
node 测试/前端/HarmonyOS/Unit/RunPassword.mjs/RunPassword.mjs
```

运行前检查4GiB空闲门禁；`JOBS_IM_HARMONY_CLT_HOME`可指定已有镜像 CLT。每轮在中央实现进度的改密证据目录中新建 `HarmonyHost-*`，保存类型日志、原始结果、源摘要和本轮执行中间模块，不覆盖旧轮。类型或测试失败返回非零退出码。覆盖 NFC／长度、原 nonce 与密码对、只读查询、明确重试、未知暂停、换家族不借 AT、秘密不落库、仓库 CAS 及暂停标记同事务。

## 二、验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前15项主机反例通过；RDB 用系统 SQLite 适配，原生应用／测试 HAP 及设备行为分别验证。实际结果见[改密实现报告](../../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS改密.md)，其他主机入口见[上级说明](../README.md)。



本轮追加能力撤回与恢复代际反例：旧原单允许匿名纯查询；全部续跑必须重新通过能力门禁，404 未建立原单须由原家族 AT 创建。更新验证为鸿蒙15项主机通过、两端应用与测试包重新编译；Android新增第5项 Room 反例仅已编译，未执行设备测试。
<a id="🔚" href="#前言">返回前言</a>

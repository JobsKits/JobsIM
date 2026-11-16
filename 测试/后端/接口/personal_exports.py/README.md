# 正式本人导出后端测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、工具与安全边界](#tools)
- [二、执行与续作](#run)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

真实执行证据统一在[中央正式导出报告](../../../../项目相关报告/测试/后端/20261116-正式本人导出/README.md)。本目录仅保留可审测试工具；秘密、数据库、State、CA私钥及备份不进入工程。只承认对应源／binary SHA的限定结果，不修改243验收状态。

## 一、<span id="tools">工具与安全边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 文件 | 用途 |
| --- | --- |
| [realm.py](./realm.py) | 显式`--create-owned-realm`新建独立UUID持久Safety、两套物理[**PostgreSQL**](https://www.postgresql.org/)与[**NATS**](https://nats.io/)；全新身份／CA／端口／候选binary。不得对旧seed调用创建。4GiB门禁、离线已有工具、低并发Go与180秒每步界限。 |
| [personal_exports.py](./personal_exports.py) | 基本ZIP、原nonce丢响应、源EX排空、真实TLS断开／取消、最终journal ACK调度等分组；私密进度耐久，原失败不覆写。 |
| [restore.py](./restore.py) | 精确现有正式域制品损坏／截断／票据到期、真实加密备份、global／退重入及新独占目标恢复；先原操作进度核对，不另造nonce或重建原域。 |
| [negative.py](./negative.py) | 明确本人job的prepared-holder与时间SQL夹具；源未知／过期先拒交付，再核worker清理。 |
| [compatibility.py](./compatibility.py) | 当前角色／公告安全与显式开启旧实验后的purpose隔离。当前角色轮不模拟旧版本迁移。 |
| [reports.py](./reports.py)／[report_restore.py](./report_restore.py) | 仅本人report公开字段、真实operator＋TOTP管理员处置／源排空与active报告holder实际恢复拒旧。 |
| [final.py](./final.py)／[all_media.py](./all_media.py) | 默认非实验最终能力、实际解压全包秘密排除；真实PNG／动GIF／AAC-M4A／MP4／普通文件类型和精确字节。 |
| [安全规则](../personal_export_test.go)／[注册清单](../personal_export_inventory_test.go) | overlay挂载到后端包的无外部状态测试；完整认证零明文、私有实时锁scope、伪造proof／service auth、响应buffer和精确注册枚举。 |

所有SQL先校127.0.0.1、所选63381／63382、新用户名和数据库前缀，清除继承的旧连接变量；不改固定57／7ef／客户端E2。进程信号仅在exact PID、完整argv、binary SHA及本轮State归属核对后发送；测试不裸用pkill、不关闭TLS、不改全局CA、不清零限流。备份独立存Documents下的本任务目录，密码／RT／TOTP及完整未知意图保存在工程外0600文件，不通过argv或中央日志输出。

## 二、<span id="run">执行与续作</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

先只读对应私密seed及进度，核实际活跃manifest；restore会保留旧源manifest作历史，不能因旧文件存在就重新启动退休源。分组失败保留原report文件，使用新的报告名记录原意图续作。原job终态不得改回ready，失败任务也占24小时额度；明确新用户意图才使用新nonce，不以数据库改限额或新建域掩盖未知。

```shell
python3 -B personal_exports.py --environment-file "$OWNED_FORMAL_SEED" --group source_stream --report "$NEW_REPORT_JSON"
python3 -B compatibility.py --environment-file "$OWNED_FORMAL_SEED" --group roles --report "$NEW_ROLE_REPORT_JSON"
```

示例中的seed与输出路径必须由调用者明确提供。具体测试组有私密fixture前置条件；不能对已终态原组机械重复整轮。恢复脚本涉及本人独立候选App短停与新目标库，按已有原操作阶段续作；PG／NATS和原数据保留，不停其他域。新脚本追加在[补充测试输入](../../../../项目相关报告/测试/后端/20261116-正式本人导出/最终候选/补充测试输入.json)单列，不能改旧源ZIP以追认新验证。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>

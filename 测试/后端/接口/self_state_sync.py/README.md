# S2 本人状态真实同步定向测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、运行前提和副作用](#run)
- [二、覆盖和证据](#coverage)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

[测试脚本](./self_state_sync.py)只验证显式新建的S2 PhaseB双物理PG／HTTPS／JetStream后端，不启动通用共享服务，也不替代客户端存储资格、ACK或完整I验收。字段见[9.5契约](../../../../代码/后端/Go/接口契约/README.md#sync-self-state)。

## 一、<span id="run">运行前提和副作用</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

先由受管准备流程创建全新私密PhaseB目录、独立State／CA、主PG62381上的新业务DB、物理journal PG62382及loopback NATS51222，再启动50443候选App。该固定端口组合是本轮测试安全围栏，不是产品部署默认。环境必须s2PhaseB=true、runDir末级PhaseB、serverURL严格HTTPS50443；实际身份从真bootstrap记入私密seed。所有凭据及会话夹具留工程外Secrets0600，禁止填入报告或命令参数。

仅用标准Python及已有PG工具。四份实际测试输入是本脚本与相对导入的sync_snapshot／forward_refs／mentions_announcements helper；无第三方下载。脚本创建三个本人测试账号、一次同人新设备登录、好友／群及正常HTTP消息，修改仅本轮业务DB。H会精确核自有NATS完整命令／PID，真实停止并同store重启，须独占该域窗口；不改原服务限流或旧报告。

从a到h串行执行，每组前4GiB门禁、外部180秒超时，显式报告路径使用中央新目录，勿覆盖原记录：

```shell
python3 测试/后端/接口/self_state_sync.py/self_state_sync.py \
  --environment-file /明确选择的新私密PhaseB/environment.json \
  --group a \
  --report 项目相关报告/测试/后端/新的S2运行目录/A-真实结果.json
```

后续将group依次换为b～h，保留同一私密夹具。中断不能盲目删除nonce／换账号重跑；先读取当前原事实和保留首FAIL，再决定从明确可重试的步骤续验。脚本只维护自有NATS测试重连；受管调用方负责结束后按精确PID／cmd／datadir停App→broker→journalPG→主PG，保留数据和私密日志。

## 二、<span id="coverage">覆盖和证据</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

8组覆盖真实认证／ownerRPC、S1／S2物理store围栏、read洞与精确当前计数、view／manualUnread／字号偏好、原body只读结果查询、trigger Outbox→PubAck→网络origin、S2原字节分页及原boundary增量、成员ABA与历史global、实际丢成功响应及NATS停机补偿／重连去重。网络origin证明先SQL观察，之后才调用HTTP补偿，避免将补偿冒称总线已交付。

[中央实际报告](../../../../项目相关报告/测试/后端/2026-11-16-S2双PG与真实同步/README.md)记录8组125项JSON HTTP检查／334语义／17页；另3条真实丢响应写、1条预期CAS冲突、10条健康检查单列。原B测试错误路径404与修补后输入SHA分别保留，未覆盖首FAIL或旧报告。候选及源码zip均有SHA。

未覆盖真实S2备份恢复、客户端S2原子安装／存储资格、qualified ACK、draft／holder、正式导出／注销／普通迁移、完整偏好矩阵、跨主机或强保护；不将metadata成功当作各端UI已应用，不改主验收状态。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>

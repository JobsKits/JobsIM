# JobsIM 转发响应围栏与未决续跑

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、隔离步骤](#jobs-toc-heading-1)
- [二、结果与限制](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、隔离步骤</span><!-- jobs-toc:heading-end -->

执行`python3 测试/后端/接口/forward_fences.py/forward_fences.py --environment-file <独立域私密environment.json> --prepare`生成仓库外600权限夹具，保存原body／会话／源引用。只有该私密路径可传给Go overlay环境`JOBS_IM_FORWARD_JSON_FIXTURE`；普通运行器未显式给路径时跳过联网测试。准备后不得将夹具或原JSON复制进仓库。

通过[Go外置测试](../forward_gateway_test.go)核公共JSON写出阻塞、客户端取消、私有证明不能从公共入口注入，以及marker在外层SH之后出现也不嵌套提升；`JOBS_IM_FORWARD_JSON_REPORT`可指定结构化报告位置。临时资格测试用同夹具，仅在测试进程拦截forward principal RPC503，`JOBS_IM_FORWARD_TEMPORARY_REPORT`指定其结果；不改运行服务，不伪造源撤权。

然后执行同脚本`--environment-file <原环境> --fixture-file <刚生成夹具>`。真实TLS慢客户端限制接收缓冲，200后代清理触发 bounded pending；只在各端API窗口释放后运行。结束注销夹具会话；它不再可供后续Go联网测试复用，重测须新prepare。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、结果与限制</span><!-- jobs-toc:heading-end -->

[JSON／私有证明报告](../../报告/转发公共JSON持锁与私有证明.json)覆盖history／search／favorites／announcement／message-results五类；[原始日志](../../报告/转发公共JSON持锁原始日志.txt)含预期的客户端取消输出。[真实慢TLS／持久续跑](../../报告/转发公共JSON与持久续跑.json)为218次HTTP／12语义，核源动作503不假报完成、后台批次收尾、原操作续跑和失效引用不挤合法页。

owner pending-before-RPC窗口使用明确SQL夹具，只写该测试成员的原待交接标记，再观察worker并经真实退群API完成；不把它称为完整管理员／所有账号状态故障注入。marker测试只移除测试自己创建的惰性夹具文件，不删除产品提交记录。共享锁排空证明的是服务端响应生命周期，不是终端耐久ACK；单机MacOS／真实PG，不外推跨主机或Linux／Windows原生。

[临时资格投影报告](../../报告/转发临时资格投影恢复实测.json)另核精确GET／history／message-results保持200空占位、搜索收藏503、依赖恢复后同version精确GET恢复且ref仍active。此占位不等于新发原意图的永久403／409资格拒绝。

# 完整原件收讫专项测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、使用方式](#run)
- [二、验证边界](#scope)
- [三、独立HTTPS后续入口](#https)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

该入口将外置测试以临时 overlay 挂入 [**Go**](https://go.dev/) 工程，源码不复制到生产目录。验证部署批准、正文与原件分离、原请求查单、文件完整性和过期提交；实际输入与结果写入[中央报告](../../../../项目相关报告/测试/后端/20261116-完整对象收讫/README.md)。

## 一、<span id="run">使用方式</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

从任意工作目录运行本脚本，或从仓库根执行以下命令。需要已准备的 [**Node.js**](https://nodejs.org/) 和项目锁定的 Go 工具链；不自动下载、升级或清缓存。

```shell
node 测试/后端/接口/object_receipts.mjs/object_receipts.mjs --go /已准备的工具目录/go
```

默认只运行纯规则，不连接数据库。原始可用空间少于4 GiB时在启动编译与数据库连接前停止；复用调用方的 `GOCACHE`，工具链强制本地、模块代理关闭、编译并发限为1、单命令最长180秒。

真实数据库模式需要按既有 [S2连接门禁](../s2_pg_guard_test.go)准备显式私有运行目录、0600 passfile、专属owner及连接环境，再追加 `--with-pg`。只在该owner名下新建随机测试库并用后删除，不写任何业务库，不结束共享连接；未提供门禁资料即停止，不降级为跳过后报通过。

```shell
node 测试/后端/接口/object_receipts.mjs/object_receipts.mjs --go /已准备的工具目录/go --with-pg
```

环境变量为 `JOBS_IM_SYNC_SCHEMAS_TEST=1`、`JOBS_IM_SYNC_SCHEMAS_ADMIN_URL`、`JOBS_IM_SYNC_SCHEMAS_REALM_URL` 和 `PGPASSFILE`。完整格式与owner归属由上述门禁校验；`REALM_URL` 在本专项只是隔离地址校验，不会访问该地址，不能据此声称真实TLS服务已运行。不要将数据库密码放进命令或普通报告。

## 二、<span id="scope">验证边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本地组验证独立 `ro1` 用途、完整字节数、摘要、时限、大整数与新文件耐久批准门禁。数据库组使用真实 [**PostgreSQL**](https://www.postgresql.org/) 事务、唯一约束、行锁及真实临时原件文件；身份、消息和媒体描述依赖是明确的进程内协议夹具。文件反例包括同大小篡改和符号链接；过期反例实际持有资格行锁，让证明在等待期间过期。

测试不证明真实HTTPS、客户端文件或目录已经fsync、原生断电、完整目标／连续累计ACK或云删除策略。旧正文回执只能证明描述，`targetRangeComplete/deletionEligible/after_receipt`继续为false。任一失败保留原JSONL、stderr和输入SHA，不把后续成功改写为原轮通过。

## 三、<span id="https">独立HTTPS后续入口</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[realm.py](./realm.py)提供显式 `--create-owned-realm`，接收已核的 `--cluster-seed`、`--binary`、`--binary-sha256` 和 `--profile-file`。它只在已归属的两个物理集群新建本轮随机库，使用新State／CA／NATS与56443端口，不重启原E2或正式导出服务。启动资料及日志放工程外私有持久目录；任一步失败保留seed和已成资源，不自动清零或伪造重建成功。

[https.py](./https.py)接受 `--environment-file`，先核实际PID／argv、冻结binary、实例／epoch及本实例CA，再验证正文与原件分离、原件完整下载、原请求查单／重试、错误字节拒绝、global后的历史事实。该脚本是服务协议夹具，**不证明Python进程实现了原生文件耐久合同**；真实CLI落盘另有独立入口与证据。认证和业务原body在发送前保存到0600私密seed，报告只存断言与脱敏计数；已获会话在finally实际logout并核原AT401，认证未知则保留原申请材料，不自动换新身份。

结束后可运行以下精确停止入口。它先核本轮路径、binary及App／NATS原PID和完整argv，再发SIGTERM并有界等待；没有强杀，不停共享数据库，不删除数据、CA或密钥。

```shell
python3 测试/后端/接口/object_receipts.mjs/realm.py --stop-owned-realm /本轮私有目录/environment.json
```

当前两个Python入口仅AST解析通过，因磁盘门禁尚未创建该域或执行真实请求，不能据此写HTTPS通过。候选39506f／79本地之后的CLI准备记录源修需要新验证／新binary，旧批准文件已标为未启用，不加载到服务。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>

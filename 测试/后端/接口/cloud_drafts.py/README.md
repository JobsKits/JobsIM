# 基础文本云草稿真实测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、执行和副作用](#run)
- [二、证据边界](#evidence)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

[功能入口](./cloud_drafts.py)复用[现有S2 helper](../self_state_sync.py/self_state_sync.py)，验证公开草稿、真实source／成员／principal／forward回调和JetStream；[恢复入口](./restore.py)另做本域新加密backup／offline restore及新epoch门禁。只操作显式受管新域，不接受固定57／7ef，不把实验导出或云附件字节算入草稿。

## 一、<span id="run">执行和副作用</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

前置为新私密Safety目录、独立State／CA、62381新业务DB、物理62382独立journal、NATS55222及54443 App；seed必须cloudDraftRealm=true，HTTPS54443，逐DSN核127／端口／owner／新库名，PGpass仅0600文件。受管准备阶段通过真实注册／好友／群和首次草稿PUT生成Secrets/draft-fixture.json（三独立账号、密码、会话及原smoke body），没有把模拟账号写成已认证。每次新执行必须用新域及自己的夹具，不复用他端actor。

按basic→source→membership→forwarding→principal→sync顺序运行，每组180秒上限／4GiB门禁，只CAS内2writer：

```shell
python3 测试/后端/接口/cloud_drafts.py/cloud_drafts.py \
  --environment-file /明确选择的新Safety/environment.json \
  --group basic \
  --report 项目相关报告/测试/后端/新的云草稿记录/basic-结果.json
```

principal会真实生成／消费恢复码并修改自己的测试账号密码，夹具私密文件同步保存；不得与其他端共用actor。功能脚本不停止App。prepared受控中断专项先锁本域journal行，观察原intent落盘，再精确停止／重启本域App，只读查原body；本轮宿主失误及续验边界在中央报告分列，没有把第二PUT算成查单。

恢复入口只接受上述新域，另需同源jobsimops及MatchingPG工具。它在Documents/JobsIMBackups新目录创建加密备份，key在运行域Secrets分开放；停自己精确PID的App，创建独立target DB／State，经真实journal重放后启动target。**服务最终切至同实例新epoch，旧source账号／会话夹具会退休。** 原进度存在时入口拒绝盲目重跑，需查Secrets/draft-restore-progress.json沿原operation收尾；不换restore ID，不删除成功备份。

```shell
python3 测试/后端/接口/cloud_drafts.py/restore.py \
  --environment-file /明确选择的新Safety/environment.json \
  --report 项目相关报告/测试/后端/新的云草稿记录/恢复结果.json
```

## 二、<span id="evidence">证据边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[中央报告](../../../../项目相关报告/测试/后端/20261116-云草稿/README.md)分列6纯规则、初始9HTTP、6组89HTTP／114语义／6页、prepared原事实续验、恢复51HTTP／130语义／9页与final标记窄验。严格CA／hostname／validity不放宽，privates／正文／proof不写公开报告；普通独立text保留、回复失效和备份floor拒旧分别核验。

没有验收五端UI、qualified ACK、after_receipt删除、云附件待上传队列、正式本人导出、注销或普通迁移；不能因HTTP200便把prepared／retired说成保存成功。字段见[契约9.6](../../../../代码/后端/Go/接口契约/README.md#cloud-text-drafts)。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>

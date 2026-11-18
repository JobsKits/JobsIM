# <span id="前言">iOS 当前物理库可靠收讫</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前源码与默认值](#ios-receipts-%E4%B8%80)
- [二、实际验证入口](#ios-receipts-%E4%BA%8C)
- [三、验证状态](#ios-receipts-%E4%B8%89)

</details>
<!-- jobs-toc:end -->

---

## 一、<span id="ios-receipts-一">当前源码与默认值</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

默认保持cloud_history／transport_only。逐库协商分别展示服务器保护和本端资格；不根据旧bootstrap的false拒绝新合同，也不根据server ready自动声明本端耐久。

[IMLocalStore](../../../代码/前端/iOS/JobsByPods/JobsIMStorage/Core/IMLocalStore/IMLocalStore.swift)新增严格原六字段声明CAS、原20字段正文／对象意图、原bodyEnvelope字节和意图同FULL事务、完整原件私有文件fsync／目录fsync及描述和意图同事务、原对象重开校验与未成preparation恢复。提交比较当前namespace、物理库UUID及inode、同步guard、原成员代际、permission、DG／manifest与global。原nonce／body／family／版本禁止重基，已成结果只留元数据。

私有原件在应用Application Support的`ReceiptObjects/<namespace>`，路径由规范UUID生成，不接受包或服务端文件路径。global先在同一SQLite事务落持久floor和包含原inode的文件清理任务，清raw envelope及描述并冻结原单；事务成功后才在数据库writer围栏内精确unlink／fsync。SQL失败保留原文件和元数据，重启继续排空清理任务；同名后继inode不被旧任务删除。账号注销按原账户所有已登记namespace清理，晚写受持久floor拒绝。

## 二、<span id="ios-receipts-二">实际验证入口</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

`verifyReceiptDurability`测当前真实SQLite的WAL／FULL／fullfsync／checkpoint fullfsync、同FULL探针提交与第二连接原字节读取；对象合同另完整文件写入、fsync／目录fsync、重开长度／SHA校验。内存库不能通过。报告绑定实际store／epoch和调用端测得的build SHA，guard保留为测量审计；普通同步推进不使同一物理库的测量失效，每次写仍比较最新完整guard。验证摘要由排序的固定检查结果计算，完成后清自有探针。此为当前路径运行检查，不是断电或远程硬盘证明。

Service／设置页／固定HTTPS路由已接。构建摘要覆盖主程序、debug dylib及嵌入的代码组件，拒绝符号链接或包外组件。用户先明确验证本库，只有当前build／本库实际验证与部署批准的profile／build／verification／合同精确匹配后，才能明确声明；不允许手填hash替代验证，不自动批准用户原实例。自动正文收讫须本人明确开启，默认关闭，同意只在原family／namespace／store的RAM保存；未知原单只查询，完整原件另行明确下载。详见[Service及生命周期](./iOS逐存储收讫.md)。

## 三、<span id="ios-receipts-三">验证状态</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

Storage和注销文件联动已随126输入候选完成真实模拟器编译与执行。8个Storage反例全通过：原字节与原单重开、guard／成员／摘要整批拒绝、完整原件与fsync索引、global擦除并保冻结原单、原tuple／family不可重基、真实物理库probe、global事务失败保原件及旧inode清理任务不删后继文件。

最终[122项原生方法](./iOS收讫证据/122项权限夹具修正实际结果.json)全部通过，0失败／0跳过，126源码输入和执行产物逐SHA零漂移，包含78基线、22迁移、8个Storage及14个Service方法。原生SQLite／文件探针、完整构建组件摘要、原声明7／9字段兼容和自动收讫生命周期均随同轮执行；真实改密HTTPS使用独立Canonical实例，收讫HTTP响应为显式夹具，未给默认实例批准资格。

另有[1项真实演示UI](./iOS收讫证据/最终演示UI执行.json)通过，覆盖发送、重启后消息仍在、白天／黑夜及中英切换；两幅截图已实看，不能替代新收讫页面或真机端到端验收。首轮[111通过／11失败](./iOS收讫证据/122项实际结果.json)、第二轮[112通过／10失败](./iOS收讫证据/122项合同修正实际结果.json)及更早99项失败原件保留；第二轮11次失败断言不是11个失败方法。修正分别是生产guard数字校验与7／9兼容，以及夹具漏记permission checkpoint，未放宽Store权限比较。磁盘低于4GiB暂停新增重型构建／集成，不删除既有数据或缓存。

[返回进度](./README.md) · [合格收讫合同](../../../代码/后端/Go/接口契约/合格收讫/README.md)

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>

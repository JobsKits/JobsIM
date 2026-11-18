# JobsIM Web 逐存储收讫协商

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、源码已实现](#jobs-toc-heading-1)
- [二、实际验证与待核](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、源码已实现</span><!-- jobs-toc:heading-end -->

依据[合格收讫合同](../../../代码/后端/Go/接口契约/合格收讫/README.md)，设置→本机收讫资格分别显示服务器当前运行保护、服务器观察到的当前physical store资格，以及本浏览器尚无等价耐久实证的阻断声明。默认仍为transport_only／cloud_history，旧SyncEngine与旧同步cursor／Outbox不改变。

| 源码 | 入口／默认／持久化／生命周期 |
| --- | --- |
| [ReceiptContext.ts](../../../代码/前端/Web/Src/ReceiptContext.ts) | 精确当前store、schema、本人／family／device／epoch；receipt-context严格响应、readiness逻辑一致性；不读取批准列表冒充本端验证 |
| [ReceiptPanel.tsx](../../../代码/前端/Web/Src/ReceiptPanel.tsx) | 设置入口、资格／服务器保护分列、七项阻断检查、完整空态／重载；后台、关闭与身份变化取消请求并销毁RAM输入 |
| [ReceiptLabels.ts](../../../代码/前端/Web/Src/ReceiptLabels.ts) | 十语27键，继承白天／黑夜／跟随系统语义色与字号；原声明列表分页 |
| [SyncStore.ts](../../../代码/前端/Web/Src/SyncStore.ts) | 只读既有registered physical store／schema，不新造登记或资格 |

高级原声明查询只接完整原六字段，严格白名单、同nonce异体拒绝。先在当前namespace的`receipt-queries|namespace`严格IDB事务保存非秘密原body／时间／状态，再POST只读storage-results。未知、404和已成原事实分别显示；404不创建声明，不生成新nonce，重查沿同体。历史结果兼容旧七字段与成对增加`qualified`／`objectQualified`的新版九字段，严格检查bool与合同一致性，拒绝未知／缺对；快照flag不用于当前资格。撤回批准后的历史结果不覆盖当前context资格。查询表单允许输入既有原记录，不提供批准列表自动复制或PUT资格声明。

单条原消息目标读取当前成员期／store自有范围，contentVersion与deliveryGeneration保持十进制字符串，manifestDigest严格成对保留。只展示本人目标和累计事实，不发送正文／对象ACK，也不把自己已记录推断成全部设备完成。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、实际验证与待核</span><!-- jobs-toc:heading-end -->

本片十语10×27完整，[真实浏览器反例入口](../../../测试/前端/Web/receipt-context.cjs/README.md)已实际执行。以下轮次分别保留原始输入SHA、日志及结果；HTTP、身份与服务器资格为明确夹具，浏览器／React／IndexedDB为真实运行。

| 实际轮次 | 结果与边界 |
| --- | --- |
| 类型及首个私有成品 | [01类型](./Web逐存储收讫/01-类型执行.json)exit0；[02构建](./Web逐存储收讫/02-私有构建执行.json)exit0，输入不变、逐产物SHA封存 |
| 首轮及共享业务 | [收讫13](./Web逐存储收讫/03-浏览器首轮/结果.json)、迁移33、群29、改密22、注销23、正式导出19、App8共147项通过；分别见同目录04～08与10的执行记录。这是该轮输入的结果，后续增量不追认全147当前通过 |
| 认证失败保留 | [09认证](./Web逐存储收讫/09-认证复验执行.json)exit1：双tab并发读取遇Authentication required。原日志保留，未改Auth／api绕过；[只读边界](./Web逐存储收讫/09-只读归因边界.json)与后续最多10轮[定向诊断](./Web09定向诊断/本轮结论.json)仍不足归因，不把失败归咎资源或抹除 |
| 实际生产包界面 | [11界面](./Web逐存储收讫/11-生产包界面执行.json)6检查通过，实际加载02产物，英文白天／藏文黑夜390px截图已目视核对；不是Src开发模块代替生产包 |
| 新旧声明兼容增量 | [12类型](./Web逐存储收讫/12-声明兼容类型执行.json)exit0；[13 Chrome](./Web逐存储收讫/13-声明兼容浏览器/结果.json)13通过、pageErrors=[]；[14私有构建](./Web逐存储收讫/14-声明兼容构建执行.json)exit0，82输入跑前后SHA均一致，最低约12.8GiB。现有用例循环旧7／新9及缺对／假flag／合同冲突／毒字段，不扩大用例数 |

新成品位于14执行记录的`production`目录，11份产物SHA随同封存，未替换运行服务。Vite保留既有大chunk与lucide指令警告，没有修改第三方或用这些警告推断构建失败。首轮生产包界面结果不冒充新版九字段增量的整站／全主题UI回归。

Web09后续只增强认证测试驱动诊断：实际完成预定10轮上限，尾斜杠有无×冷模块顺序／并发，每轮新Vite cache／Chrome context、原16组断言保留；91输入每轮前后不变。已捕获各tab公开快照、错误入口、实际模块URL及Storage／Broadcast时序，本轮双tab均同family／generation2、各tab单一AuthSession资源，Broadcast实际零事件。[脱敏核对](./Web09定向诊断/脱敏诊断核对.json)无夹具密码／CSRF／proof字符串。未复现不建立旧失败因果，诊断也可能影响时序；仍标未归因，不改生产、不将后续绿当已修，不继续扩大。

本浏览器普通IndexedDB缓存不满足native SQLite FULL与文件／目录fsync合同；server ready和部署批准都不能替代本端实际耐久实证。当前不发布after_receipt／after_object_receipt策略；浏览器等价耐久验证与真正qualified ACK仍未交付。服务器真实双PG／JetStream运行保护由后端独立验证，本地HTTP夹具不替代。

未操作a4／Canonical用户数据，未修改第三方、清理历史证据或提交／推送。中央资产表由主代理统一同步。

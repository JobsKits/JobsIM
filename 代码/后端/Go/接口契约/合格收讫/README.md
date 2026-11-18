# 当前物理库的合格收讫协商

![JobsIM](../../../../前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、兼容与资格](#jobs-toc-heading-1)
- [二、逐store读取](#jobs-toc-heading-2)
- [三、声明与未知原单](#jobs-toc-heading-3)
- [四、每条消息的冻结与累计事实](#jobs-toc-heading-4)
- [五、发布保留策略](#jobs-toc-heading-5)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、兼容与资格</span><!-- jobs-toc:heading-end -->

协议标识`qualified-receipts-v1`。原bootstrap的`features.after_receipt:false`和原`sync/device`、`sync/context`的`transport_only/qualifiedAck:false/afterReceiptEnabled:false`保持兼容；它们不承担本物理库的合格收讫协商。旧`GET/PUT /api/sync/storage`顶层`afterReceiptEnabled`也保持false，既有storage资格字段照常返回。客户端不得因为新协商响应中的true改写旧同步游标或跳过旧边界验证。

合格端必须先完成自身当前build及本物理库的真实耐久验证，再声明部署者已批准的精确`profileId/buildDigest/verificationDigest/storageContract`。部署批准不是远程硬盘证明，也不能由客户端复制服务端列表冒充本端通过。`native_sqlite_full_v1`要求原正文与回执意图同FULL事务；`native_sqlite_full_file_fsync_v1`还要求原件完整字节校验、文件／目录fsync及描述／回执原意图同FULL事务。浏览器普通IndexedDB缓存不自报原生SQLite资格；尚无等价实际证据时继续transport_only和cloud_history。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、逐store读取</span><!-- jobs-toc:heading-end -->

`GET /api/sync/receipt-context?schemaVersion=1|2`须当前认证及`X-JobsIM-Store-Generation`原物理库。该库先用原`PUT /api/sync/device`登记；未登记、换库、旧epoch或非personal不通过。所有版本／代际均十进制字符串。

响应：

```text
protocol: qualified-receipts-v1
scopeId, sourceContextId: personal, principalId, deviceId
storeGeneration, restoreEpoch, schemaVersion
qualification: transport_only | qualified_content | qualified_content_and_objects
storage: {qualificationId?,storeGeneration,restoreEpoch,profileId?,buildDigest?,verificationDigest?,storageContract?,qualified,objectQualified}
qualifiedAck, contentReceiptEnabled, objectReceiptEnabled
afterReceiptEnabled, afterObjectReceiptEnabled
readiness: {transportConfigured,publishConnected,consumeConnected,messageDurable,journalDurable,journalIndependent,epochMatches,ready,blockers:[]}
defaultRetentionMode: cloud_history
deletionEligible: false
targetRangeComplete: false
```

`qualifiedAck/contentReceiptEnabled`只表示当前声明仍匹配部署批准；不表示任何消息已保存。对象资格只来自file-fsync合同。`readiness.ready`描述服务端运行保护，和本库资格分别判断；after-body需qualified＋ready，after-object需objectQualified＋ready。运行保护由真实JetStream发布／消费连接、message与journal当前fsync／full_page_writes／synchronous_commit、不同PG物理system identifier及同恢复epoch推导；不读管理员开关伪造ready。同一PG的两个数据库不能冒充独立journal。

资格／服务器保护不足时仍可正常走旧传输同步，默认cloud_history。可靠保存事实与是否可发布删除策略分开；body/object ACK本身不宣称整个人类／设备范围完成。

`GET /api/sync/receipt-profiles?schemaVersion=2&buildDigest=<本地实测当前构建SHA256>`使用相同认证和store header，只返回部署配置中精确build匹配的`profiles:[{id,buildDigest,verificationDigest,storageContract}]`。响应`qualifiedAck:false,declarationRequired:true,localDurabilityVerificationRequired:true`。空列表表示未批准，不自动写生产策略，不从服务器列表反推本地探针成功。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、声明与未知原单</span><!-- jobs-toc:heading-end -->

实际验证通过后显式`PUT /api/sync/storage`，原body仍为`{requestId,storeGeneration,revisionEpoch,profileId,buildDigest,verificationDigest}`。原nonce／六字段先耐久保存；新声明旋转资格时，不继承旧qualification对应目标或ACK。

新增`POST /api/sync/storage-results`接完全相同原六字段，只读既成声明，未知404 `STORAGE_DECLARATION_NOT_FOUND`，异体409 `IDEMPOTENCY_CONFLICT`，不会创建或重新批准。批准撤回后也可查原result，但`current:false/storage.qualified:false`不能覆盖当前状态或恢复旧资格。未知保持原责任，不自动换nonce／build／verification／store。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、每条消息的冻结与累计事实</span><!-- jobs-toc:heading-end -->

`GET /api/sync/receipt-targets/{messageId}?schemaVersion=1|2`使用相同当前store header，只给当前原人类成员代际的自有目标信息：

```text
protocol, messageId, contentVersion, restoreEpoch, qualificationId
deliveryGeneration, manifestDigest, receiptScope
originalTarget, contentTarget, objectTarget
contentRecorded, objectRecorded, contentCompleteAt, objectCompleteAt
current, messageCurrent, deletionEligible:false
```

不列他人设备／store／资格。`contentRecorded/objectRecorded`直接核本库、原qualification、原CV／epoch／DG／manifest的已提交事实，不等后台汇总字段更新；两种completeAt是原范围第一次完成的时间，仅发件人可见，其他成员为null。自己已提交不代表全范围完成。新的device／store／qualification／成员重入不加入旧冻结目标；old target缺失不推断已达。

默认按本库当前qualification查询；可显式加`qualificationId=<原QID>`只读历史事实。该QID必须属于原本人／device／store／epoch已成功提交的storage_declarations；新结果保存历史`qualified:true/objectQualified`快照。旧成功记录只有原七字段时，必须完整匹配原store／epoch／profile／build／verification／受支持合同；不补写或重新批准，显式false／null或残缺tuple拒绝。未知或别人的QID返回404 `RECEIPT_QUALIFICATION_NOT_FOUND`，格式错400。批准撤回或同库重新声明后，历史`contentRecorded/objectRecorded`仍可核；`contentTarget/objectTarget`仍要求当前active QID与当前资格，不恢复新ACK或清理目标。`messageCurrent`仅表示当前消息CV／DG／epoch和原成员范围仍匹配，且无已到期global／账号擦除；`current`还包含原manifest范围状态与自然有效期。已到期manifest不会自动抹去仍合法持有的旧qualifiedAtCommit事实。到期global即使仍protection_pending也不放行旧来源。

普通cloud_only迁移按已成qualifiedAtCommit、原store／bodyDigest及DG／原成员代际验证，不要求设备曾在发送冻结目标中；晚声明的真实本机回执不会据此加入旧清理范围。无DG旧事实仅在完整原permission仍相同时核验，不补造manifest。历史查询本身不证明本机字节仍存在；客户端还需原结果纯查、完整原始envelope及原件字节核验，最终由迁移owner再次重验当前分类／ACL／global。

挑战／提交／只读结果继续按原正文及对象接口。rc1／ro1证明新增的`deliveryGeneration/manifestDigest`必须成对核验及保留，严格客户端不能当未知字段丢弃；旧无manifest消息可为空对，不能替新消息造代际。正文挑战给原bodyEnvelope，先核SHA／身份／版本，正文与receipt_outbox同本地耐久事务后才提交原body；对象独立完整字节、文件与目录fsync、描述／原对象意图同事务后才提交。range按原事实集合累计，不用传输cursor跨缺口冒充完整正文，重复ACK不重算起点。

恢复、撤权、旧family、过期proof或变更原代际均先查原结果，未知不自动补新proof／改nonce。原已成ACK只回历史metadata，不确认新内容或新物理库。global优先，不把cloud_only当本地焚毁。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">五、发布保留策略</span><!-- jobs-toc:heading-end -->

`PUT /api/conversations/{id}/retention`原body不变：`{requestId,expectedVersion,policy}`。`policy.mode=after_receipt`或`attachmentMode=after_object_receipt`时必须带`X-JobsIM-Store-Generation`；message私有核sync当前actor/device/store资格及实时保护，body与对象资格分别校验。缺条件409 `RETENTION_QUALIFICATION_REQUIRED`，不以Profile列表非空或全局bool替代。

仍保持原成员／群主权限、CAS、明确n（含0）、只影响新消息和原策略快照。原已提交同nonce结果先读回；资格后来撤回不重新授予新策略写。默认cloud_history及既有普通收发不因未声明资格失效；已冻结目标的空用户不伪装范围齐备。测试配置批准项不等于生产a4或任何实际设备已获批准。

`PUT /api/sync/storage`在gateway与sync owner均用同一EX围栏，先排空旧正文／原件ACK和交付读取，再旋转QID。profiles、targets、storage-results只读使用SH；不在外层SH内升级EX，也不在资格检查后允许并行声明替换原QID。

# 同实例个人普通历史迁移

![JobsIM](../../../../前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、身份与上限](#jobs-toc-heading-1)
- [二、目标配对与源创建](#jobs-toc-heading-2)
- [三、加密包与独立密钥交接](#jobs-toc-heading-3)
- [四、目标整批发布](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、身份与上限</span><!-- jobs-toc:heading-end -->

本合同对应需求2.15受控端侧迁移，格式`jobsim-personal-migration-v1`。所有入口通过本人已认证HTTPS网关；不得接受任意HTTP服务、第三方归档或跨实例数据。source固定`personal`，源与目标必须是同scope、同user、当前restoreEpoch下的不同device，分别使用已登记physical store。版本和代际采用十进制字符串。

请求使用现有认证／CSRF／来源上下文规则；目标GET、key、validate、installed、cancel和源status必须携`X-JobsIM-Store-Generation`。网关剥离客户端伪造的内部TLS证明，只有真实TLS连接才给后端签出内部证明。`GET /api/sync/migration-capabilities`返回`enabled/format/sourceContextId/maxMessages/maxObjects/maxRequestBytes/maxObjectBytes/maxLifetimeSeconds/pairingLifetimeSeconds/installLifetimeSeconds`，并明确`qualifiedAck:false,targetQualified:false`。

单包1～256条消息、最多64个原件、单原件64MiB、整个JSON请求128MiB，只有一个并发包构建；超过上限明确拒绝，不截断。制品最长600秒，配对300秒，最终发布验证5秒；源已成立的更早截止优先。磁盘必须保留4GiB余量。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、目标配对与源创建</span><!-- jobs-toc:heading-end -->

目标`POST /api/sync/migration-pairings`：`{requestId,storeGeneration,restoreEpoch,password,otp?}`。管理员需当前OTP。响应`{pairingId,pairingProof,targetDeviceId,targetStoreGeneration,restoreEpoch,expiresAt,sourcePreserved:true}`；proof只作目标配对，不能充当下载密钥或消息收讫。相同原request只查回原proof，不再次消费OTP。响应未知先`POST /api/sync/migration-pairing-results {requestId,storeGeneration}`；未见返回`{status:"not_found"}`，不会创建。过期或已被原迁移占用的proof不能换目标或创建第二份迁移。

源`POST /api/sync/migrations`：

```json
{
  "requestId":"原稳定nonce",
  "sourceContextId":"personal",
  "sourceStoreGeneration":"源物理库UUID",
  "restoreEpoch":"1",
  "targetPairingProof":"mp1...",
  "password":"仅本次RAM输入",
  "messages":[{"bodyEnvelope":"Base64URL无补位的原Message JSON","objectMode":"none"}],
  "objects":[]
}
```

消息`objectMode`为`none`、`metadata_only`或`full`。正文仍在云端时，可用原Message字段子集稳定构造bodyEnvelope：`id/conversationId/senderId/type/text/replyToId/attachmentId/createdAt/contentVersion/creationEpoch/creationSequence/forwarded`。展示状态、updatedAt、ClientMsgID、转发／回复准入证明不参加包内历史；服务端再次核对当前immutable primary digest。只允许当前可证明的普通内容。当前转发文本可保留，转发附件只迁当前投影元数据，不能冒用另一个messageId复制原件。

每个`full`须且只能对应一个`objects`条目：`{objectId,messageId,bytes}`，bytes采用标准Base64。必须是原件全部字节，长度／SHA256由media owner原绑定核对；metadata_only不取原件，不把URL／缩略图当原件。

cloud_only源必须另携`sourceReceiptId`，且bodyEnvelope必须精确命中原qualified store已提交的完整原bodyDigest与当前membership generation；重构Message子集不能代替原收讫字节。cloud_only原件还须`sourceObjectReceiptId`及原parent/length/SHA256。换store、退重入、当前ACL撤回、global或未知分类都拒绝。部署后来撤回profile只停止新声明／新ACK／新after策略资格，不抹掉原已成`storageQualifiedAtCommit`事实；同store重新声明也不把新QID继承为旧ACK。历史事实可通过[逐store原QID查询](../合格收讫/README.md)核验，仍须原receipt／raw／DG-MG与当前合法分类；不要求该设备曾在发送冻结目标中。普通transport_only客户端不得伪造旧qualified证明。

响应为`{id,requestId,status,manifest,manifestEnvelope,manifestDigest,capsuleSHA256,sizeBytes,expiresAt,sourcePreserved:true,qualifiedAck:false,targetQualified:false}`。状态为`building/ready/installed_reported/revoking/retired`。未知先`POST /api/sync/migration-results {requestId,sourceStoreGeneration}`，未见只返回not_found；原nonce重试完整输入必须相同，密码／OTP不持久化到摘要。原件字节在私有加密capsule中，不新建服务器消息、不补投。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、加密包与独立密钥交接</span><!-- jobs-toc:heading-end -->

目标`GET /api/sync/migrations/{id}`查询原状态；`GET .../{id}/capsule`下载加密制品，可用Range，只有当前原target device/store可读。每次交付重验源／目标family、安全代际、成员历史范围、内容版本和生命周期。源变化、取消或到期在EX边界排空构建／下载，先撤钥再删精确capsule路径；未知结果沿原迁移查，不能自动生成替代。

`manifestEnvelope`是服务端公开manifest的原始UTF-8 JSON字节，以Base64URL无补位编码；SHA256必须等于`manifestDigest`。客户端先核摘要再解析，不能用跨语言JSON重编码猜摘要。公开manifest不含源receipt/proof。全部消息／原件清单、scope/user/source/target/store/epoch/security/sourceGeneration/期限必须与本次上下文逐项匹配。解密后的messages每个完整原JSON对象片段与该条primaryDigest所用Go原序列化字节一致，可用严格JSON字符串／转义／嵌套扫描取得原片段直接SHA；不用重编码对象猜字段序或转义。

目标另`POST .../{id}/key {manifestDigest,password,otp?}`，经独立已认证HTTPS请求及当前密码／管理员OTP取得`{id,manifestDigest,algorithm:"AES-256-GCM",key,aad,expiresAt,targetQualified:false}`。key为32字节Base64URL无补位，aad为服务端提供的精确AAD字节Base64URL。密钥只留RAM，不写公开清单／包／日志。

capsule二进制依次为8字节ASCII`JIMMIGR1`、12字节随机nonce、AES-256-GCM密文及末尾16字节tag；使用key、nonce、aad解密并验证tag。明文为`{format,manifestDigest,messages,objects}`；messages是上述普通历史字段＋objectMode，objects仅`{objectId,messageId,bytes}`。拒绝重复ID、未知字段、未列文件、路径穿越、任何凭据／Outbox／收讫资格，不执行包内容。先核capsuleSHA256、manifest摘要、完整数量及每原件长度／SHA256，再写隔离暂存。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、目标整批发布</span><!-- jobs-toc:heading-end -->

暂存完整后`POST .../{id}/validate {manifestDigest}`。返回`{id,status:"validated",manifestDigest,installToken,installBefore,requiresAtomicLocalCommit:true,targetQualified:false,qualifiedAck:false}`。任一源不可证明即整批失败，源端与目标原数据保留；不能只导入成功行。

客户端还须在本地原子事务中比较原scope/user/personal/device/store/restoreEpoch及认证生命周期快照，按原ID、creationEpoch/creationSequence/contentVersion去重，更新独立迁移历史后再发布；较新本地版本不被较旧包覆盖。不得改sync cursor、Outbox、原qualification或ACK。global与本地已见撤权floor优先，未知保持暂存并保留源。

`POST .../{id}/installed {manifestDigest,installToken}`再次联网重验后记录`installed_reported`，明确`serverVerifiedLocalDurability:false`。该状态停止新capsule／key交付；未知响应先GET原id核事实，已localCommitted且manifestDigest一致时只展示原结果，不再取钥或制造替代任务。该回执不证明客户端真实fsync，不授予target qualified ACK，也不删除源。服务端校验时刻之后新成立的撤权／global继续通过正常同步与客户端本地floor执行；已交付字节不承诺远程追回。转发投影须继续读前实时GET及后续引用失效清副本；不能仅凭available分类把引用正文变成永久ownedText。

`POST .../{id}/cancel {manifestDigest}`由原源或目标store取消；源及目标原历史保留，隔离暂存与尚未交付包退出。恢复时旧pairing、key、capsule和安装token退休，新epoch不能继承旧迁移资格。可控制品与正式导出分别存储／清理，服务端整库恢复不接收个人迁移包。

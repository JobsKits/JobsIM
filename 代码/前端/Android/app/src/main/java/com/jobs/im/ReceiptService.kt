package com.jobs.im

import android.app.Application
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject

class ReceiptService(private val app:Application,private val database:ChatDatabase,val identity:MigrationIdentity,private val api:()->Api,private val current:()->Boolean) {
    private val store=ReceiptStore(app,database,identity){!stopped&&current()}
    private val lock=Mutex()
    @Volatile private var stopped=false
    fun clear(){stopped=true}
    private fun check(){if(stopped||!current())throw CancellationException("RECEIPT_CONTEXT_CHANGED")}
    private suspend fun request(path:String,method:String="GET",body:JSONObject?=null):JSONObject{check();val value=api().receipt(path,method,body,identity.store){!stopped&&current()};check();return value}
    suspend fun objectBytes(parentId:String):ByteArray{check();val selected=store.migrationSources().map{JSONObject(it)}.single{it.opt("sourceReceiptId")==parentId};selected.remove("text");selected.put("objectMode","full");val bytes=store.migrationSource(selected).second?.second?:error("RECEIPT_LOCAL_OBJECT_MISSING");try{check();return bytes}catch(e:Throwable){bytes.fill(0);throw e}}
    suspend fun archive():List<String>{check();return store.migrationSources().also{check()}}
    suspend fun rows():List<ReceiptIntent>{check();return store.rows().also{check()}}
    suspend fun reconcile(){for(row in rows().filter{it.state in setOf("pending","unknown")&&it.identity.namespace==identity.namespace&&it.identity.store==identity.store}){try{resume(row.id,false)}catch(e:Exception){check()}}}
    suspend fun context():JSONObject {val schema=store.schema();return ReceiptProtocol.context(request("/sync/receipt-context?schemaVersion=$schema"),identity,schema)}
    suspend fun measurement():JSONObject? {check();return store.measured(store.buildDigest())}
    suspend fun measure(objects:Boolean):JSONObject=lock.withLock{check();store.measure(objects).also{check()}}
    private suspend fun measured():JSONObject {check();return measurement()?:error("RECEIPT_LOCAL_VERIFICATION_REQUIRED")}
    suspend fun profiles():List<JSONObject>{
        val local=measured();val build=local.getString("buildDigest");val raw=request("/sync/receipt-profiles?schemaVersion=${store.schema()}&buildDigest=$build");require(raw.opt("protocol")==ReceiptProtocol.protocol&&raw.opt("qualifiedAck")==false&&raw.opt("declarationRequired")==true&&raw.opt("localDurabilityVerificationRequired")==true)
        val rows=raw.getJSONArray("profiles");return (0 until rows.length()).map{rows.getJSONObject(it)}.filter{MigrationProtocol.exact(it,setOf("id","buildDigest","verificationDigest","storageContract"));it.opt("buildDigest")==local.opt("buildDigest")&&it.opt("verificationDigest")==local.opt("verificationDigest")&&it.opt("storageContract")==local.opt("storageContract")}.also{check()}
    }
    private suspend fun qualified(objects:Boolean=false):JSONObject {
        store.binding();val local=measured();val c=context();val q=c.getJSONObject("storage");require(q.opt("qualified")==true&&(!objects||q.opt("objectQualified")==true)&&q.opt("buildDigest")==local.opt("buildDigest")&&q.opt("verificationDigest")==local.opt("verificationDigest")&&q.opt("storageContract")==local.opt("storageContract")){"RECEIPT_CURRENT_QUALIFICATION_REQUIRED"};return q
    }
    suspend fun declaration(profile:String):ReceiptIntent=lock.withLock{
        check();require(rows().none{it.kind=="storage"&&it.state in setOf("pending","unknown")&&MigrationStore.samePrincipal(it.identity,identity)}){"RECEIPT_ORIGINAL_PENDING"};val local=measured();val selected=profiles().single{it.opt("id")==profile};val id=UUID.randomUUID().toString();val b=JSONObject().put("requestId",id).put("storeGeneration",identity.store).put("revisionEpoch",identity.epoch).put("profileId",selected.getString("id")).put("buildDigest",local.getString("buildDigest")).put("verificationDigest",local.getString("verificationDigest"));val row=ReceiptIntent(id,"storage",identity,store.schema(),b.toString());store.stage(row);row
    }
    private fun original(row:ReceiptIntent){check();require(MigrationStore.samePrincipal(row.identity,identity)){"RECEIPT_ORIGINAL_FAMILY_REQUIRED"}}
    private fun route(row:ReceiptIntent,result:Boolean)=when(row.kind){"storage"->if(result)"/sync/storage-results" else "/sync/storage";"message_content"->if(result)"/sync/receipt-results" else "/sync/receipts";else->if(result)"/sync/object-receipt-results" else "/sync/object-receipts"}
    private fun missing(row:ReceiptIntent,error:ApiFailure)=error.status==404&&error.code==when(row.kind){"storage"->"STORAGE_DECLARATION_NOT_FOUND";"message_content"->"RECEIPT_OPERATION_NOT_FOUND";else->"OBJECT_RECEIPT_OPERATION_NOT_FOUND"}
    private suspend fun lookup(row:ReceiptIntent):ReceiptIntent?{
        if(row.state=="retired")return row
        val response=try{request(route(row,true),"POST",JSONObject(row.body))}catch(e:ApiFailure){if(missing(row,e))return null;throw e}
        ReceiptProtocol.result(response,row);return store.replace(row,row.copy(state="recorded",result=response.toString())).also{check()}
    }
    suspend fun resume(id:String,retry:Boolean=false):ReceiptIntent?=lock.withLock{resumeOriginal(rows().single{it.id==id},retry)}
    private suspend fun resumeOriginal(input:ReceiptIntent,retry:Boolean):ReceiptIntent? {
        check();var row=input;if(row.state=="retired")return row;val found=lookup(row);if(found!=null)return found;if(!retry)return null
        original(row);store.binding()
        if(row.kind=="storage"){val b=JSONObject(row.body);val local=measured();require(b.opt("buildDigest")==local.opt("buildDigest")&&b.opt("verificationDigest")==local.opt("verificationDigest")&&profiles().any{it.opt("id")==b.opt("profileId")})}
        else {val q=qualified(row.kind=="object_received");require(q.opt("qualificationId")==JSONObject(row.body).opt("qualificationId")&&Instant.parse(row.expires).isAfter(Instant.now()));if(row.kind=="object_received")withContext(Dispatchers.IO){store.verifyObject(JSONObject(row.descriptor))} else {val cached=database.syncRecords().one("${row.identity.namespace}|receipt-body|${row.id}")?:error("RECEIPT_LOCAL_BODY_MISSING");val body=JSONObject(cached.payload);SyncWire.originalBytes(body.opt("bodyEnvelope"),row.digest)}}
        row=store.replace(row,row.copy(state="unknown",attempts=row.attempts+1));original(row);store.binding();val response=request(route(row,false),if(row.kind=="storage")"PUT" else "POST",JSONObject(row.body));ReceiptProtocol.result(response,row);return store.replace(row,row.copy(state="recorded",result=response.toString()))
    }
    private suspend fun proof(response:JSONObject,prefix:String,qualification:String):JSONObject {
        val p=ReceiptProtocol.proof(response.getString("challenge"),prefix,identity,qualification,store.permission());val expiry=Instant.parse(response.getString("expiresAt"));require(expiry.epochSecond==p.getLong("expires")&&expiry.isAfter(Instant.now())&&expiry.isBefore(Instant.now().plusSeconds(3660)));for((key,proof) in mapOf("messageId" to "message","conversationId" to "conversation","contentVersion" to "contentVersion","restoreEpoch" to "epoch","storeGeneration" to "store","qualificationId" to "qualification","bodyDigest" to "bodyDigest"))require(response.opt(key)==p.opt(proof));require(response.opt("objectComplete")==false&&response.opt("deletionEligible")==false);return p
    }
    suspend fun content(message:String,expectedVersion:String?=null,expectedPair:Pair<String,String>?=null):ReceiptIntent?=lock.withLock{
        check();SyncWire.id(message);rows().firstOrNull{it.kind=="message_content"&&it.message==message&&(expectedVersion==null||it.content==expectedVersion)&&it.state in setOf("pending","unknown")&&it.identity.namespace==identity.namespace}?.let{return@withLock resumeOriginal(it,false)}
        val q=qualified();val response=request("/sync/receipt-challenges","POST",JSONObject().put("messageId",message).put("schemaVersion",store.schema()));val p=proof(response,"rc1",q.getString("qualificationId"));require(response.opt("requiresAtomicLocalCommit")==true&&p.opt("message")==message);require(expectedVersion==null||p.opt("contentVersion")==expectedVersion);require(expectedPair==null||ReceiptProtocol.pair(p)==expectedPair)
        val body=SyncWire.originalBytes(response.opt("bodyEnvelope"),response.opt("bodyDigest"));val m=JSONObject(body);for(k in listOf("id","conversationId","senderId"))SyncWire.id(m.opt(k));require(m.opt("id")==message&&m.opt("conversationId")==p.opt("conversation")&&m.opt("contentVersion")==p.opt("contentVersion")&&m.opt("deleted")==false&&!m.optBoolean("forwarded")&&!m.optBoolean("forwardUnavailable")&&!m.optBoolean("contentUnavailable")&&m.optString("type") in setOf("text","image","audio","video","file")&&m.optString("attachmentURL").isEmpty());require((m.opt("type")=="text")==m.optString("attachmentId").isEmpty());if(m.optString("attachmentId").isNotEmpty())SyncWire.id(m.opt("attachmentId"));MigrationProtocol.positive(m.opt("creationEpoch"));MigrationProtocol.positive(m.opt("creationSequence"));require(m.getString("text").toByteArray().size<=16384);Instant.parse(m.getString("createdAt"))
        rows().firstOrNull{it.kind=="message_content"&&it.message==message&&it.content==p.opt("contentVersion")&&MigrationStore.samePrincipal(it.identity,identity)&&it.state=="recorded"&&JSONObject(it.body).opt("qualificationId")==q.opt("qualificationId")}?.let{return@withLock it}
        val id=UUID.randomUUID().toString();val request=JSONObject().put("requestId",id).put("storeGeneration",identity.store).put("revisionEpoch",identity.epoch).put("qualificationId",q.getString("qualificationId")).put("kind","message_content").put("challenge",response.getString("challenge")).put("bodyDigest",p.getString("bodyDigest"));val row=ReceiptIntent(id,"message_content",identity,store.schema(),request.toString(),message,p.getString("contentVersion"),p.getString("bodyDigest"),p.toString(),expires=response.getString("expiresAt"));store.stage(row,body);resumeOriginal(row,true)
    }
    suspend fun objectOriginal(parentId:String):ReceiptIntent?=lock.withLock{
        val parent=rows().single{it.id==parentId};original(parent);require(parent.kind=="message_content"&&parent.state=="recorded");rows().firstOrNull{it.kind=="object_received"&&it.identity.namespace==identity.namespace&&it.state in setOf("pending","unknown")&&JSONObject(it.body).opt("parentReceiptId")==parentId}?.let{return@withLock resumeOriginal(it,false)}
        val q=qualified(true);require(JSONObject(parent.body).opt("qualificationId")==q.opt("qualificationId"));val response=request("/sync/object-receipt-challenges","POST",JSONObject().put("parentReceiptId",parentId).put("schemaVersion",store.schema()));val p=proof(response,"ro1",q.getString("qualificationId"));require(response.opt("requiresDurableOriginalObject")==true&&response.opt("targetRangeComplete")==false&&response.opt("parentReceiptId")==parentId&&p.opt("parentReceiptId")==parentId&&p.opt("message")==parent.message&&p.opt("contentVersion")==parent.content&&p.opt("bodyDigest")==parent.digest&&ReceiptProtocol.pair(p)==ReceiptProtocol.pair(JSONObject(parent.proof)))
        val ref=response.getJSONObject("object");for(k in listOf("objectId","sizeBytes"))require(ref.opt(k)==p.opt(k));require(ref.opt("sha256")==p.opt("objectDigest"));SyncWire.id(ref.opt("objectId"));require(SyncWire.counter(ref.opt("sizeBytes")).toLong()<=64L*1024*1024);MigrationProtocol.digest(ref.opt("sha256"));require(ref.getString("contentType").length<=256&&!ref.getString("contentType").contains(Regex("[\\r\\n]")))
        val id=UUID.randomUUID().toString();val bytes=api().migrationBytes("/media/${ref.getString("objectId")}",identity.store,maxOf(1,ref.getString("sizeBytes").toInt())){!stopped&&current()};try{check();
        val b=JSONObject().put("requestId",id).put("storeGeneration",identity.store).put("revisionEpoch",identity.epoch).put("qualificationId",q.getString("qualificationId")).put("kind","object_received").put("parentReceiptId",parentId).put("challenge",response.getString("challenge")).put("objectId",ref.getString("objectId")).put("sizeBytes",ref.getString("sizeBytes")).put("objectDigest",ref.getString("sha256"));val row=ReceiptIntent(id,"object_received",identity,store.schema(),b.toString(),parent.message,parent.content,parent.digest,p.toString(),"",response.getString("expiresAt"));val committed=withContext(Dispatchers.IO){store.stageObject(row,ref,bytes)};resumeOriginal(committed,true)}finally{bytes.fill(0)}
    }
    suspend fun receiveIfTarget(message:String,version:String):ReceiptIntent? {
        SyncWire.id(message);MigrationProtocol.positive(version);val q=qualified();val raw=request("/sync/receipt-targets/$message?schemaVersion=${store.schema()}&qualificationId=${q.getString("qualificationId")}");require(raw.opt("protocol")==ReceiptProtocol.protocol&&raw.opt("messageId")==message&&raw.opt("qualificationId")==q.opt("qualificationId")&&raw.opt("restoreEpoch")==identity.epoch&&raw.opt("contentVersion")==version&&raw.opt("deletionEligible")==false)
        if(raw.opt("messageCurrent")!=true||raw.opt("contentTarget")!=true||raw.opt("contentRecorded")!=false)return null
        val pair=ReceiptProtocol.pair(raw);require(pair.first.isNotEmpty());return content(message,version,pair)
    }
    suspend fun targets(id:String):JSONObject {val row=rows().single{it.id==id};require(row.state!="retired"&&row.kind!="storage");return ReceiptProtocol.target(request("/sync/receipt-targets/${row.message}?schemaVersion=${row.schema}&qualificationId=${JSONObject(row.body).getString("qualificationId")}"),row)}
}

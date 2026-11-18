package com.jobs.im

import java.time.Instant
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONArray
import org.json.JSONObject

class MigrationService(private val store:MigrationStore,val identity:MigrationIdentity,private val api:()->Api,private val current:()->Boolean,private val receipts:ReceiptStore?=null) {
    private val lock=Mutex()
    private val alive={ !stopped&&current() }
    private val proofs=mutableMapOf<String,String>()
    @Volatile private var stopped=false
    fun clear(){stopped=true;proofs.clear()}
    private fun check(){if(stopped||!current())throw CancellationException("MIGRATION_CONTEXT_CHANGED")}
    private fun original(row:MigrationIntent){check();require(MigrationStore.samePrincipal(row.identity,identity)){"MIGRATION_ORIGINAL_SESSION_REQUIRED"}}
    private suspend fun request(path:String,method:String="GET",body:JSONObject?=null):JSONObject{check();val value=api().migration(path,method,body,identity.store){!stopped&&current()};check();return value}
    suspend fun qualifiedSources():List<String>{check();return receipts?.migrationSources().orEmpty().also{check()}}
    suspend fun rows():List<MigrationIntent>{check();return store.rows(identity).also{check()}}
    suspend fun capability(){
        val c=request("/sync/migration-capabilities");require(c.opt("enabled")==true&&c.opt("format")==MigrationProtocol.format&&c.opt("sourceContextId")=="personal"&&c.opt("qualifiedAck")==false&&c.opt("targetQualified")==false)
        for((key,n) in mapOf("maxMessages" to 256,"maxObjects" to 64,"maxRequestBytes" to MigrationProtocol.limit,"maxObjectBytes" to 64*1024*1024,"maxLifetimeSeconds" to 600,"pairingLifetimeSeconds" to 300,"installLifetimeSeconds" to 5))require(c.opt(key) is Number&&c.getLong(key)==n.toLong());store.binding(identity,alive)
    }
    suspend fun stage(kind:String,selection:String="[]",migration:String=""):MigrationIntent{check();capability();return store.stage(identity,kind,selection,migration,alive)}
    private suspend fun save(before:MigrationIntent,after:MigrationIntent)=store.replace(before,after,identity,alive).also{check()}
    private fun proof(password:String,otp:String):JSONObject {
        require(otp.isEmpty()||otp.matches(Regex("[0-9]{6}")));return JSONObject().put("password",OwnDeletionRules.password(password)).also{if(otp.isNotEmpty())it.put("otp",otp)}
    }
    private fun pairing(token:String):JSONObject {
        require(token.length<=8192);val parts=token.split('.');require(parts.size==3&&parts[0]=="mp1"&&MigrationProtocol.decode(parts[2],32,true).size==32)
        val p=JSONObject(MigrationProtocol.utf8(MigrationProtocol.decode(parts[1],8192,true)));require(p.opt("scopeId")==identity.deployment&&p.opt("userId")==identity.user&&p.opt("restoreEpoch")==identity.epoch);for(key in listOf("pairingId","targetDeviceId","targetStoreGeneration"))SyncWire.id(p.opt(key));for(key in listOf("securityVersion","sourceGeneration"))MigrationProtocol.positive(p.opt(key));val expiry=Instant.ofEpochSecond(p.getLong("expires"));require(expiry.isAfter(Instant.now())&&expiry.isBefore(Instant.now().plusSeconds(301)))
        return JSONObject().also{out->for(key in listOf("pairingId","targetDeviceId","targetStoreGeneration","restoreEpoch","securityVersion","sourceGeneration","expires"))out.put(key,p.get(key))}
    }
    private suspend fun pairResult(row:MigrationIntent,value:JSONObject):MigrationIntent {
        val token=value.getString("pairingProof");val p=pairing(token);require(value.opt("sourcePreserved")==true&&value.opt("pairingId")==p.opt("pairingId")&&value.opt("targetDeviceId")==row.identity.device&&value.opt("targetStoreGeneration")==row.identity.store&&value.opt("restoreEpoch")==row.identity.epoch&&Instant.parse(value.getString("expiresAt")).epochSecond==p.getLong("expires"))
        val after=save(row,row.copy(state="ready",target=p.toString()));proofs[row.id]=token;return after
    }
    private fun publicView(raw:JSONObject):String=JSONObject().also{out->for(key in listOf("id","requestId","status","manifest","manifestEnvelope","manifestDigest","capsuleSHA256","sizeBytes","expiresAt","sourcePreserved","qualifiedAck","targetQualified"))if(raw.has(key))out.put(key,raw.get(key))}.toString()
    private suspend fun remember(row:MigrationIntent,raw:JSONObject):MigrationView {
        val v=MigrationProtocol.view(raw,identity,if(row.kind=="create")row.id else null);val m=JSONObject(v.manifest)
        if(row.view.isNotEmpty())require(JSONObject(row.view).opt("manifestDigest")==v.digest){"MIGRATION_ORIGINAL_CHANGED"}
        if(row.kind=="create"){
            val selected=JSONArray(row.selection);val refs=m.getJSONArray("messages");require(selected.length()==refs.length())
            for(at in 0 until selected.length()){val s=selected.getJSONObject(at);val r=refs.getJSONObject(at);require(r.opt("messageId")==s.opt("id")&&r.opt("conversationId")==s.opt("conversationId")&&r.opt("contentVersion")==s.opt("contentVersion")&&r.opt("objectId")==s.opt("attachmentId")&&r.opt("objectMode")==s.opt("objectMode"))}
            if(row.target.isNotEmpty()){val t=JSONObject(row.target);require(m.opt("targetDeviceId")==t.opt("targetDeviceId")&&m.opt("targetStoreGeneration")==t.opt("targetStoreGeneration"))}
        }
        save(row,row.copy(migration=v.id,view=publicView(raw),state=if(v.state in setOf("revoking","retired"))"retired" else if(v.state=="building")"unknown" else if(row.state=="installed")"installed" else "ready"))
        if(v.state in setOf("revoking","retired"))store.discard(identity,v.id,alive);return v
    }
    private suspend fun lookup(row:MigrationIntent):JSONObject? {
        if(row.kind=="target"){val v=request("/sync/migrations/${row.migration}");remember(row,v);return v}
        val path=if(row.kind=="pairing")"/sync/migration-pairing-results" else "/sync/migration-results"
        val value=request(path,"POST",JSONObject().put("requestId",row.id).put(if(row.kind=="pairing")"storeGeneration" else "sourceStoreGeneration",row.identity.store))
        if(value.opt("status")=="not_found"){require(value.keys().asSequence().toSet()==setOf("status"));return null}
        if(row.kind=="pairing")pairResult(row,value) else remember(row,value);return value
    }
    suspend fun checkOriginal(id:String):JSONObject?=lock.withLock{val row=rows().single{it.id==id};lookup(row)}
    suspend fun pair(id:String,password:String,otp:String,retry:Boolean):String?=lock.withLock{
        var row=rows().single{it.id==id};require(row.kind=="pairing");if(lookup(row)!=null)return@withLock proofs[row.id];if(!retry)return@withLock null
        original(row);capability();store.disk(identity);require(row.attempts==0||Instant.parse(row.created).plusSeconds(300).isAfter(Instant.now())){"MIGRATION_PAIRING_EXPIRED"};val body=proof(password,otp).put("requestId",row.id).put("storeGeneration",row.identity.store).put("restoreEpoch",row.identity.epoch)
        row=save(row,row.copy(attempts=row.attempts+1,state="unknown"));original(row);pairResult(row,request("/sync/migration-pairings","POST",body));proofs[row.id]
    }
    fun sharePair(id:String):String{check();return (proofs[id]?:error("MIGRATION_PAIRING_RECHECK_REQUIRED")).also{pairing(it)}}
    private fun memory(bytes:Long){val r=Runtime.getRuntime();require(r.maxMemory()-(r.totalMemory()-r.freeMemory())>bytes*6+8*1024*1024){"MIGRATION_MEMORY_LIMIT"}}
    private suspend fun validateLocalSource(selected:JSONObject){
        val repository=receipts?:error("MIGRATION_LOCAL_PROVENANCE_REQUIRED");val row=repository.intent(selected.getString("sourceReceiptId"));val p=JSONObject(row.proof)
        val result=api().receipt("/sync/receipt-results","POST",JSONObject(row.body),identity.store,alive);ReceiptProtocol.result(result,row);check()
        val (generation,_)=ReceiptProtocol.pair(p)
        if(generation.isNotEmpty()){val target=api().receipt("/sync/receipt-targets/${row.message}?schemaVersion=${row.schema}&qualificationId=${JSONObject(row.body).getString("qualificationId")}","GET",null,identity.store,alive);ReceiptProtocol.target(target,row);require(target.opt("messageCurrent")==true){"MIGRATION_MEMBERSHIP_CHANGED"}}
        else {val context=api().request("/sync/context?schemaVersion=${row.schema}");require(context.opt("permissionGeneration")==p.opt("permission")){"MIGRATION_MEMBERSHIP_CHANGED"}}
        if(selected.opt("objectMode")=="full"){val objectRow=repository.intent(selected.getString("sourceObjectReceiptId"));val response=api().receipt("/sync/object-receipt-results","POST",JSONObject(objectRow.body),identity.store,alive);ReceiptProtocol.result(response,objectRow)};check()
    }
    private suspend fun source(row:MigrationIntent):Pair<MigrationIntent,JSONObject>{
        original(row);store.binding(identity,alive);store.disk(identity);val selected=JSONArray(row.selection);val messages=JSONArray();val objects=JSONArray();var used=0L
        for(at in 0 until selected.length()){
            original(row);val s=selected.getJSONObject(at)
            if(s.has("sourceReceiptId")){
                try{val live=api().request("/messages/${s.getString("id")}");check();require(live.opt("id")==s.opt("id")&&live.opt("contentVersion")==s.opt("contentVersion")&&!live.optBoolean("deleted")&&!live.optBoolean("forwarded"));if(live.optBoolean("contentUnavailable"))require(live.opt("deleteScope")=="cloud_only")}catch(e:ApiFailure){if(e.status!=409||e.code!="RETENTION_CLOUD_EXPIRED")throw e;check()}
                validateLocalSource(s)
                val (envelope,objectData)=(receipts?:error("MIGRATION_LOCAL_PROVENANCE_REQUIRED")).migrationSource(s);messages.put(envelope);used+=envelope.toString().toByteArray().size
                if(objectData!=null){val (metadata,bytes)=objectData;try{original(row);val encoded=MigrationProtocol.encode(bytes);used+=encoded.length;objects.put(metadata.put("bytes",encoded))}finally{bytes.fill(0)}};require(used<MigrationProtocol.limit-32768);continue
            }
            val raw=api().request("/messages/${s.getString("id")}");check();val envelope=MigrationProtocol.envelope(raw,s);messages.put(envelope);used+=envelope.toString().toByteArray().size
            if(s.opt("objectMode")=="full"){
                require(used<MigrationProtocol.limit-32768);val max=minOf(64L*1024*1024,(MigrationProtocol.limit-used-32768)*3/4).toInt();val bytes=api().migrationBytes("/media/${s.getString("attachmentId")}",identity.store,max){!stopped&&current()}
                try{check();val encoded=MigrationProtocol.encode(bytes);used+=encoded.length;objects.put(JSONObject().put("objectId",s.getString("attachmentId")).put("messageId",s.getString("id")).put("bytes",encoded))}finally{bytes.fill(0)}
            }
            require(used<MigrationProtocol.limit-32768){"MIGRATION_SOURCE_LIMIT"}
        }
        val data=JSONObject().put("messages",messages).put("objects",objects);val digest=MigrationProtocol.hash(data.toString().toByteArray());require(row.sourceDigest.isEmpty()||row.sourceDigest==digest){"MIGRATION_ORIGINAL_CHANGED"};return save(row,row.copy(sourceDigest=digest)) to data
    }
    suspend fun create(id:String,targetProof:String,password:String,otp:String,retry:Boolean):MigrationView?=lock.withLock{
        var row=rows().single{it.id==id};require(row.kind=="create");val found=lookup(row);if(found!=null)return@withLock MigrationProtocol.view(found,identity,row.id);if(!retry)return@withLock null
        original(row);capability();val target=pairing(targetProof);require(target.getString("targetDeviceId")!=identity.device);row=save(row,row.copy(target=target.toString()));val (saved,data)=withContext(Dispatchers.Default){source(row)};row=saved
        val body=proof(password,otp).put("requestId",row.id).put("sourceContextId","personal").put("sourceStoreGeneration",row.identity.store).put("restoreEpoch",row.identity.epoch).put("targetPairingProof",targetProof).put("messages",data.getJSONArray("messages")).put("objects",data.getJSONArray("objects"));require(body.toString().toByteArray().size<=MigrationProtocol.limit)
        row=save(row,row.copy(attempts=row.attempts+1,state="unknown"));original(row);remember(row,request("/sync/migrations","POST",body))
    }
    suspend fun status(id:String):MigrationView {check();return MigrationProtocol.view(request("/sync/migrations/${SyncWire.id(id)}"),identity)}
    suspend fun install(id:String,password:String,otp:String):Boolean=lock.withLock{
        capability();val v=status(id);val m=JSONObject(v.manifest);require(m.opt("targetDeviceId")==identity.device&&m.opt("targetStoreGeneration")==identity.store);if(v.state=="installed_reported"){require(store.installed(identity,v,alive)){"MIGRATION_LOCAL_COMMIT_MISSING"};return@withLock true};var row=rows().firstOrNull{it.kind=="target"&&it.migration==id&&MigrationStore.samePrincipal(it.identity,identity)}?:store.stage(identity,"target",migration=id,current=alive)
        original(row);store.disk(identity);row=save(row,row.copy(attempts=row.attempts+1,state=if(row.state=="installed")"installed" else "unknown",view=JSONObject().put("id",v.id).put("manifestDigest",v.digest).put("expiresAt",v.expires).toString()));memory(v.size);val bytes=withContext(Dispatchers.IO){store.loadCapsule(identity,v)}?:api().migrationBytes("/sync/migrations/$id/capsule",identity.store,v.size.toInt()){!stopped&&current()};var history:MigrationHistory?=null
        try{
            check();withContext(Dispatchers.IO){store.saveCapsule(identity,v,bytes,alive)};val key=request("/sync/migrations/$id/key","POST",proof(password,otp).put("manifestDigest",v.digest));val opened=withContext(Dispatchers.Default){MigrationProtocol.open(v,bytes,key,identity)};history=opened;check()
            val validation=request("/sync/migrations/$id/validate","POST",JSONObject().put("manifestDigest",v.digest));val until=MigrationProtocol.validateInstall(validation,v,identity);withContext(Dispatchers.IO){store.install(identity,v,opened,until,alive);store.discard(identity,id,alive)};val currentRow=rows().single{it.id==row.id};save(currentRow,currentRow.copy(state="installed",view=JSONObject().put("manifestDigest",v.digest).toString()))
            try{val reported=request("/sync/migrations/$id/installed","POST",JSONObject().put("manifestDigest",v.digest).put("installToken",validation.getString("installToken")));require(reported.opt("id")==id&&reported.opt("manifestDigest")==v.digest&&reported.opt("status")=="installed_reported"&&reported.opt("sourcePreserved")==true&&reported.opt("qualifiedAck")==false&&reported.opt("targetQualified")==false&&reported.opt("serverVerifiedLocalDurability")==false);true}catch(error:Exception){check();false}
        }catch(error:Exception){runCatching{store.discard(identity,id,alive)};throw error}finally{bytes.fill(0);history?.clear()}
    }
    suspend fun cancel(id:String):MigrationView=lock.withLock{
        val v=status(id);val originals=rows().filter{it.migration==id&&it.identity.namespace==identity.namespace};require(originals.isNotEmpty()){"MIGRATION_ORIGINAL_SESSION_REQUIRED"};originals.forEach{original(it)};val raw=request("/sync/migrations/$id/cancel","POST",JSONObject().put("manifestDigest",v.digest));val result=MigrationProtocol.view(raw,identity);for(row in rows().filter{it.migration==id})remember(row,raw);if(result.state in setOf("retired","revoking"))store.discard(identity,id,alive);result
    }
    suspend fun objectBytes(id:String):Pair<String,ByteArray>{check();val message=history().single{JSONObject(it).getString("id")==id};require(!JSONObject(message).optBoolean("forwarded")&&!JSONObject(message).optBoolean("contentUnavailable"));return store.objectBytes(identity,id){!stopped&&current()}.also{check()}}
    suspend fun history():List<String> {
        val out=mutableListOf<String>()
        val initial=store.history(identity,alive)
        val observed=initial.associateBy{JSONObject(it).getString("id")}
        for(raw in initial){
            val message=JSONObject(raw);if(!message.optBoolean("forwarded")){out.add(raw);continue}
            try{
                val live=api().request("/messages/${message.getString("id")}");check()
                val valid=live.opt("deleted")==false&&!live.optBoolean("contentUnavailable")&&!live.optBoolean("forwardUnavailable")&&live.opt("forwarded")==true&&listOf("id","conversationId","senderId","type","text","replyToId","contentVersion","creationEpoch","creationSequence","createdAt").all{live.opt(it)==message.opt(it)}
                if(valid)out.add(raw)else store.retireMessage(identity,message.getString("id"),alive)
            }catch(error:ApiFailure){check();if(error.status in setOf(403,404)&&error.code in setOf("MESSAGE_NOT_FOUND","NOT_FOUND","HISTORY_NOT_ALLOWED","NOT_A_MEMBER","FORWARD_UNAVAILABLE","RETENTION_CLOUD_EXPIRED"))store.retireMessage(identity,message.getString("id"),alive)
                else out.add(message.put("text","").put("forwardUnavailable",true).put("contentUnavailable",true).toString())
            }catch(error:Exception){check();out.add(message.put("text","").put("forwardUnavailable",true).put("contentUnavailable",true).toString())}
        };check();store.drainCleanup(identity.profile,identity.deployment)
        val fresh=store.history(identity,alive).associateBy{JSONObject(it).getString("id")};check()
        return out.filter{raw->val id=JSONObject(raw).getString("id");val latest=fresh[id];val original=observed[id];latest!=null&&original!=null&&ReceiptProtocol.canonical(JSONObject(latest))==ReceiptProtocol.canonical(JSONObject(original))}
    }
}

package com.jobs.im

import android.app.Application
import android.os.StatFs
import android.util.AtomicFile
import java.io.File
import java.time.Instant
import java.util.UUID
import org.json.JSONArray
import org.json.JSONObject

data class MigrationIntent(val id:String,val kind:String,val identity:MigrationIdentity,val created:String,val selection:String="[]",val attempts:Int=0,val state:String="unsent",val sourceDigest:String="",val target:String="",val migration:String="",val view:String="") {
    fun json()=JSONObject().put("id",id).put("kind",kind).put("identity",MigrationStore.identityJSON(identity)).put("created",created).put("selection",selection).put("attempts",attempts).put("state",state).put("sourceDigest",sourceDigest).put("target",target).put("migration",migration).put("view",view)
}
class MigrationStore(private val app:Application,private val database:ChatDatabase) {
    companion object {
        suspend fun eraseGlobalInTransaction(database:ChatDatabase,i:SyncIdentity,ids:Set<String>) {
            val records=database.syncRecords();val prefix="live|${i.profile}|${i.deployment}|${i.user}|";val removed=ids.toMutableSet()
            for(entry in records.across(prefix,"migration-message")){val m=JSONObject(entry.payload).getJSONObject("message");if(m.getString("id") in ids||m.optBoolean("forwarded")){removed.add(m.getString("id"));records.remove(entry.key)}}
            for(entry in records.across(prefix,"migration-object")){val o=JSONObject(entry.payload);if(o.getString("messageId") in removed){records.put(SyncRecord(entry.key+"|cleanup",entry.namespace,"migration-cleanup",JSONObject().put("path",o.getString("path")).toString()));records.remove(entry.key)}}
        }
        fun identityJSON(i:MigrationIdentity)=JSONObject().put("profile",i.profile).put("deployment",i.deployment).put("user",i.user).put("device",i.device).put("family",i.family).put("epoch",i.epoch).put("namespace",i.namespace).put("store",i.store).put("guard",i.guard)
        fun identity(row:JSONObject)=MigrationIdentity(row.getString("profile"),row.getString("deployment"),row.getString("user"),row.getString("device"),row.getString("family"),row.getString("epoch"),row.getString("namespace"),row.getString("store"),row.getString("guard")).also{MigrationProtocol.exact(row,setOf("profile","deployment","user","device","family","epoch","namespace","store","guard"));MigrationProtocol.identity(it)}
        fun samePrincipal(a:MigrationIdentity,b:MigrationIdentity)=a.copy(guard="")==b.copy(guard="")
        fun parse(value:JSONObject):MigrationIntent {
            MigrationProtocol.exact(value,setOf("id","kind","identity","created","selection","attempts","state","sourceDigest","target","migration","view"))
            val row=MigrationIntent(SyncWire.id(value.opt("id")),value.getString("kind"),identity(value.getJSONObject("identity")),value.getString("created"),value.getString("selection"),value.getInt("attempts"),value.getString("state"),value.getString("sourceDigest"),value.getString("target"),value.getString("migration"),value.getString("view"))
            require(row.kind in setOf("pairing","create","target")&&row.state in setOf("unsent","unknown","ready","installed","retired")&&row.attempts>=0);Instant.parse(row.created)
            val selected=JSONArray(row.selection);require(selected.length()<=256&&(row.kind=="create")== (selected.length()>0));val ids=mutableSetOf<String>();var full=0
            for(at in 0 until selected.length()){
                val s=selected.getJSONObject(at);MigrationProtocol.exact(s,setOf("id","conversationId","contentVersion","type","attachmentId","forwarded","objectMode","sourceReceiptId","sourceObjectReceiptId"));require(ids.add(SyncWire.id(s.opt("id"))));SyncWire.id(s.opt("conversationId"));MigrationProtocol.positive(s.opt("contentVersion"));require(s.getString("type") in setOf("text","image","audio","video","file")&&s.getString("objectMode") in setOf("none","metadata_only","full"));require((s.getString("type")=="text")== (s.getString("objectMode")=="none"));if(s.has("attachmentId"))SyncWire.id(s.opt("attachmentId"));if(s.has("sourceReceiptId"))SyncWire.id(s.opt("sourceReceiptId"));if(s.has("sourceObjectReceiptId")){SyncWire.id(s.opt("sourceObjectReceiptId"));require(s.has("sourceReceiptId")&&s.opt("objectMode")=="full")};if(s.has("forwarded"))require(s.opt("forwarded")==true&&!s.has("attachmentId")&&s.opt("objectMode")==if(s.getString("type")=="text")"none" else "metadata_only");if(s.opt("objectMode")=="full"){full++;require(s.has("attachmentId"))}
            }
            require(full<=64);if(row.sourceDigest.isNotEmpty())MigrationProtocol.digest(row.sourceDigest);if(row.migration.isNotEmpty())SyncWire.id(row.migration)
            if(row.target.isNotEmpty()){val t=JSONObject(row.target);MigrationProtocol.exact(t,setOf("pairingId","targetDeviceId","targetStoreGeneration","restoreEpoch","securityVersion","sourceGeneration","expires"));for(k in listOf("pairingId","targetDeviceId","targetStoreGeneration"))SyncWire.id(t.opt(k));for(k in listOf("restoreEpoch","securityVersion","sourceGeneration"))MigrationProtocol.positive(t.opt(k));require(t.getLong("expires")>0)}
            if(row.kind=="target")require(row.migration.isNotEmpty())
            if(row.view.isNotEmpty()){val view=JSONObject(row.view);MigrationProtocol.exact(view,setOf("id","requestId","status","manifest","manifestEnvelope","manifestDigest","capsuleSHA256","sizeBytes","expiresAt","sourcePreserved","qualifiedAck","targetQualified"));MigrationProtocol.digest(view.opt("manifestDigest"))}
            return row
        }
    }
    private val records get()=database.syncRecords()
    private fun prefix(i:MigrationIdentity)="live|${i.profile}|${i.deployment}|${i.user}|"
    private fun scope(i:MigrationIdentity)="${prefix(i)}personal"
    private fun key(row:MigrationIntent)="${row.identity.namespace}|migration-intent|${row.id}"
    suspend fun rows(i:MigrationIdentity):List<MigrationIntent> = records.across(prefix(i),"migration-intent").map{parse(JSONObject(it.payload)).also{row->require(key(row)==it.key&&row.identity.namespace==it.namespace)}}.sortedByDescending{it.created}
    suspend fun binding(i:MigrationIdentity,current:()->Boolean){
        require(current());MigrationProtocol.identity(i);val raw=records.one("${i.namespace}|sync-control")?:error("MIGRATION_STORE_NOT_READY");val c=JSONObject(raw.payload)
        SyncIdentity(i.namespace,i.profile,i.deployment,i.user,i.epoch,i.device,i.family).checkBinding(c,i.store,c.getString("streamId"))
        require(c.opt("familyId")==i.family&&c.opt("profile")==i.profile&&c.opt("guardGeneration")==i.guard&&c.has("cursor")&&!c.has("restriction")&&database.messages().nativeScope(i.namespace)?.namespace==i.namespace&&current())
    }
    suspend fun stage(i:MigrationIdentity,kind:String,selection:String="[]",migration:String="",current:()->Boolean):MigrationIntent=database.withTransaction{
        binding(i,current);val row=MigrationIntent(UUID.randomUUID().toString(),kind,i,Instant.now().toString(),selection,migration=migration);parse(row.json());records.put(SyncRecord(key(row),i.namespace,"migration-intent",row.json().toString()));require(current());row
    }
    suspend fun replace(before:MigrationIntent,after:MigrationIntent,i:MigrationIdentity,current:()->Boolean):MigrationIntent=database.withTransaction{
        require(current()&&before.identity.namespace.startsWith(prefix(i)));parse(after.json());require(before.id==after.id&&before.identity==after.identity&&before.kind==after.kind&&before.created==after.created&&before.selection==after.selection&&after.attempts>=before.attempts)
        require((before.sourceDigest.isEmpty()||before.sourceDigest==after.sourceDigest)&&(before.target.isEmpty()||before.target==after.target)&&(before.migration.isEmpty()||before.migration==after.migration));require(records.one(key(before))?.payload==before.json().toString())
        records.put(SyncRecord(key(after),after.identity.namespace,"migration-intent",after.json().toString()));require(current());after
    }
    private fun root(i:MigrationIdentity):File {
        val base=File(app.noBackupFilesDir,"Migrations");if(!base.exists()){check(base.mkdir());PersonalExportFiles.syncDirectory(base.parentFile!!)};require(base.absoluteFile==base.canonicalFile)
        val dir=File(base,Isolation.digest(i.namespace));if(!dir.exists()){check(dir.mkdir());PersonalExportFiles.syncDirectory(base)};require(dir.absoluteFile==dir.canonicalFile);return dir
    }
    fun disk(i:MigrationIdentity){require(StatFs(root(i).absolutePath).availableBytes>=4L*1024*1024*1024){"MIGRATION_STORAGE_LIMIT"}}
    private fun capsule(i:MigrationIdentity,id:String)=File(root(i),SyncWire.id(id)+".capsule")
    fun saveCapsule(i:MigrationIdentity,v:MigrationView,bytes:ByteArray,current:()->Boolean){
        disk(i);require(current()&&bytes.size.toLong()==v.size&&MigrationProtocol.hash(bytes)==v.capsuleDigest);val target=capsule(i,v.id);require(target.absoluteFile==target.canonicalFile);val file=AtomicFile(target);var stream:java.io.FileOutputStream?=null
        try{stream=file.startWrite();stream.write(bytes);stream.fd.sync();require(current());file.finishWrite(stream);PersonalExportFiles.syncDirectory(root(i))}catch(e:Throwable){stream?.let{file.failWrite(it)};throw e}
    }
    fun loadCapsule(i:MigrationIdentity,v:MigrationView):ByteArray? {
        val file=capsule(i,v.id);if(!file.exists()&&!File(file.path+".bak").exists())return null;require(file.absoluteFile==file.canonicalFile);return AtomicFile(file).openRead().use{readLimited(it,MigrationProtocol.limit+4096)}.also{require(it.size.toLong()==v.size&&MigrationProtocol.hash(it)==v.capsuleDigest)}
    }
    suspend fun install(i:MigrationIdentity,v:MigrationView,history:MigrationHistory,before:String,current:()->Boolean){
        disk(i);val expires=Instant.parse(before);require(expires.isAfter(Instant.now())&&current());val paths=mutableMapOf<String,File>()
        database.withTransaction{
            binding(i,current);require(expires.isAfter(Instant.now()))
            val manifestRefs=JSONObject(v.manifest).getJSONArray("messages")
            for(raw in history.messages){val m=JSONObject(raw);val id=m.getString("id");require(records.one("${scope(i)}|global|$id")==null){"MIGRATION_GLOBAL_SOURCE"};val live=database.messages().one(i.namespace,id);val ref=(0 until manifestRefs.length()).map{manifestRefs.getJSONObject(it)}.single{it.opt("messageId")==id};require(live==null||!live.retracted&&(!live.contentUnavailable||ref.opt("classification")=="cloud_only"&&!m.optBoolean("forwarded"))){"MIGRATION_LOCAL_FLOOR"}}
        for(o in history.objects){require(current());val file=File(root(i),"${v.id}-${SyncWire.id(o.id)}.object");require(file.absoluteFile==file.canonicalFile);val atomic=AtomicFile(file);var stream:java.io.FileOutputStream?=null;try{stream=atomic.startWrite();stream.write(o.bytes);stream.fd.sync();atomic.finishWrite(stream)}catch(e:Throwable){stream?.let{atomic.failWrite(it)};throw e};paths[o.id]=file};PersonalExportFiles.syncDirectory(root(i))

            for(raw in history.messages){val m=JSONObject(raw);val id=m.getString("id");require(records.one("${scope(i)}|global|$id")==null){"MIGRATION_GLOBAL_SOURCE"};val key="${i.namespace}|migration-message|$id";val old=records.one(key)?.let{JSONObject(it.payload).getJSONObject("message")};val live=database.messages().one(i.namespace,id)
                val references=JSONObject(v.manifest).getJSONArray("messages");val reference=(0 until references.length()).map{references.getJSONObject(it)}.single{it.opt("messageId")==id};require(live==null||!live.retracted&&(!live.contentUnavailable||reference.opt("classification")=="cloud_only"&&!m.optBoolean("forwarded"))){"MIGRATION_LOCAL_FLOOR"};if(old!=null){require(old.opt("creationEpoch")==m.opt("creationEpoch")&&old.opt("creationSequence")==m.opt("creationSequence"));if(java.math.BigInteger(old.getString("contentVersion"))>java.math.BigInteger(m.getString("contentVersion")))continue};if(live!=null&&live.contentVersion.isNotEmpty()&&java.math.BigInteger(live.contentVersion)>java.math.BigInteger(m.getString("contentVersion")))continue
                for(entry in records.across(i.namespace,"migration-object").filter{it.namespace==i.namespace&&JSONObject(it.payload).opt("messageId")==id}){val prior=JSONObject(entry.payload);val replacement=history.objects.firstOrNull{it.message==id&&it.id==prior.opt("id")};if(replacement==null||paths[replacement.id]?.name!=prior.opt("path")){records.put(SyncRecord(entry.key+"|cleanup",entry.namespace,"migration-cleanup",JSONObject().put("path",prior.getString("path")).toString()))};records.remove(entry.key)}
                records.put(SyncRecord(key,i.namespace,"migration-message",JSONObject().put("migrationId",v.id).put("message",m).toString()))
                for(o in history.objects.filter{it.message==id}){val objectKey="${i.namespace}|migration-object|$id|${o.id}";val payload=JSONObject().put("id",o.id).put("messageId",id).put("path",paths.getValue(o.id).name).put("sha256",MigrationProtocol.hash(o.bytes)).put("contentType",o.mime).put("migrationId",v.id);records.put(SyncRecord(objectKey,i.namespace,"migration-object",payload.toString()))}
            }
            records.put(SyncRecord("${i.namespace}|migration-installed|${v.id}",i.namespace,"migration-installed",JSONObject().put("id",v.id).put("manifestDigest",v.digest).put("localCommitted",true).put("qualifiedAck",false).toString()));require(current()&&expires.isAfter(Instant.now()))
        }
    }
    suspend fun history(i:MigrationIdentity,current:()->Boolean):List<String> {
        binding(i,current);return records.across(prefix(i),"migration-message").mapNotNull{row->if(row.namespace.split('|').getOrNull(6)!=i.epoch)return@mapNotNull null;val m=JSONObject(row.payload).getJSONObject("message");if(records.one("${scope(i)}|global|${m.getString("id")}")!=null)null else m.toString()}.groupBy{JSONObject(it).getString("id")}.map{(_,values)->values.reduce{old,new->val x=JSONObject(old);val y=JSONObject(new);require(x.opt("creationEpoch")==y.opt("creationEpoch")&&x.opt("creationSequence")==y.opt("creationSequence"));if(java.math.BigInteger(x.getString("contentVersion"))>=java.math.BigInteger(y.getString("contentVersion")))old else new}}.sortedBy{JSONObject(it).getString("createdAt")}.also{require(current())}
    }
    suspend fun objectBytes(i:MigrationIdentity,id:String,current:()->Boolean):Pair<String,ByteArray>{
        binding(i,current);SyncWire.id(id);require(records.one("${scope(i)}|global|$id")==null);val candidates=records.across(prefix(i),"migration-object").filter{it.namespace.split('|').getOrNull(6)==i.epoch&&JSONObject(it.payload).opt("messageId")==id};val row=candidates.lastOrNull()?:error("MIGRATION_OBJECT_METADATA_ONLY");val value=JSONObject(row.payload);val name=value.getString("path");require(name.matches(Regex("[0-9a-f-]{36}-[0-9a-f-]{36}\\.object")));val directory=File(app.noBackupFilesDir,"Migrations/${Isolation.digest(row.namespace)}");val file=File(directory,name);require(file.absoluteFile==file.canonicalFile&&file.isFile&&file.length()<=64L*1024*1024);val bytes=file.inputStream().use{readLimited(it,64*1024*1024)};try{require(MigrationProtocol.hash(bytes)==value.getString("sha256"));binding(i,current);return value.getString("contentType") to bytes}catch(e:Throwable){bytes.fill(0);throw e}
    }
    suspend fun drainCleanup(profile:String,deployment:String) {
        val records=database.syncRecords();for(row in records.across("live|$profile|$deployment|","migration-cleanup")){
            val parts=row.namespace.split('|');require(parts.size==7&&parts[0]=="live"&&parts[1]==profile&&parts[2]==deployment&&parts[4]=="personal");SyncWire.id(parts[3]);SyncWire.id(parts[5]);MigrationProtocol.positive(parts[6]);val name=JSONObject(row.payload).getString("path");require(name.matches(Regex("[0-9a-f-]{36}-[0-9a-f-]{36}\\.object")))
            val root=File(app.noBackupFilesDir,"Migrations/${Isolation.digest(row.namespace)}");val file=File(root,name);require(file.absoluteFile==file.canonicalFile);if(file.exists()){check(file.delete());PersonalExportFiles.syncDirectory(root)}
            database.withTransaction{if(records.one(row.key)?.payload==row.payload)records.remove(row.key)}
        }
    }
    suspend fun retireMessage(i:MigrationIdentity,id:String,current:()->Boolean)=database.withTransaction{
        binding(i,current);SyncWire.id(id);for(row in records.across(prefix(i),"migration-message")){if(JSONObject(row.payload).getJSONObject("message").opt("id")==id)records.remove(row.key)}
        for(row in records.across(prefix(i),"migration-object")){val value=JSONObject(row.payload);if(value.opt("messageId")==id){records.put(SyncRecord(row.key+"|cleanup",row.namespace,"migration-cleanup",JSONObject().put("path",value.getString("path")).toString()));records.remove(row.key)}};require(current())
    }
    suspend fun installed(i:MigrationIdentity,v:MigrationView,current:()->Boolean):Boolean {
        binding(i,current);val record=records.one("${i.namespace}|migration-installed|${v.id}")?:return false;val value=JSONObject(record.payload);require(value.opt("id")==v.id&&value.opt("manifestDigest")==v.digest&&value.opt("localCommitted")==true&&value.opt("qualifiedAck")==false);return true
    }
    suspend fun discard(i:MigrationIdentity,id:String,current:()->Boolean)=database.withTransaction{
        MigrationProtocol.identity(i);SyncWire.id(id);AtomicFile(capsule(i,id)).delete();val referenced=records.across(prefix(i),"migration-object").filter{it.namespace==i.namespace}.map{JSONObject(it.payload).getString("path")}.toSet()
        root(i).listFiles()?.filter{it.name.startsWith("$id-")&&it.name.matches(Regex("[0-9a-f-]{36}-[0-9a-f-]{36}\\.object(?:\\.new|\\.bak)?"))}?.map{it.name.removeSuffix(".new").removeSuffix(".bak")}?.distinct()?.filter{it !in referenced}?.forEach{val file=File(root(i),it);require(file.absoluteFile==file.canonicalFile);AtomicFile(file).delete()};PersonalExportFiles.syncDirectory(root(i))
    }
}

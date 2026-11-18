package com.jobs.im

import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import java.time.Instant
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec
import org.json.JSONArray
import org.json.JSONObject

data class MigrationIdentity(val profile:String,val deployment:String,val user:String,val device:String,val family:String,val epoch:String,val namespace:String,val store:String,val guard:String)
data class MigrationView(val id:String,val request:String,val state:String,val manifest:String,val digest:String,val capsuleDigest:String,val size:Long,val expires:String)
data class MigrationObject(val id:String,val message:String,val mime:String,val bytes:ByteArray) { override fun toString()="MigrationObject(redacted)" }
data class MigrationHistory(val messages:List<String>,val objects:List<MigrationObject>) {
    fun clear(){objects.forEach{it.bytes.fill(0)}}
}
object MigrationProtocol {
    const val format="jobsim-personal-migration-v1"
    const val limit=128*1024*1024
    fun positive(value:Any?):String=SyncWire.counter(value).also{require(it!="0")}
    fun digest(value:Any?):String=(value as? String ?: error("MIGRATION_INVALID")).also{require(it.matches(Regex("[0-9a-f]{64}")))}
    fun hash(value:ByteArray)=MessageDigest.getInstance("SHA-256").digest(value).joinToString(""){"%02x".format(it)}
    fun utf8(value:ByteArray):String=Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(value)).toString()
    fun encode(value:ByteArray,url:Boolean=false):String=if(url)Base64.getUrlEncoder().withoutPadding().encodeToString(value) else Base64.getEncoder().encodeToString(value)
    fun decode(value:String,max:Int,url:Boolean=false):ByteArray {
        require(value.length.toLong()<=((max.toLong()+2)/3)*4)
        val bytes=if(url)Base64.getUrlDecoder().decode(value) else Base64.getDecoder().decode(value)
        require(bytes.size<=max&&encode(bytes,url)==value);return bytes
    }
    fun exact(row:JSONObject,allowed:Set<String>){require(row.keys().asSequence().all{it in allowed})}
    fun identity(i:MigrationIdentity){listOf(i.deployment,i.user,i.device,i.family,i.store,i.guard).forEach{SyncWire.id(it)};positive(i.epoch);require(i.profile.isNotEmpty()&&!i.profile.contains('|')&&i.namespace==Isolation.namespace(i.profile,i.deployment,i.user,false,i.epoch,"personal",i.device))}
    fun envelope(raw:JSONObject,selected:JSONObject):JSONObject {
        require(raw.getString("id")==selected.getString("id")&&raw.getString("conversationId")==selected.getString("conversationId")&&raw.getString("contentVersion")==selected.getString("contentVersion")&&raw.getString("type")==selected.getString("type"))
        require(raw.opt("deleted")==false&&!raw.optBoolean("contentUnavailable")&&!raw.optBoolean("forwardUnavailable")&&!raw.optBoolean("pending")&&!raw.optBoolean("frozen")&&raw.optBoolean("forwarded")==selected.optBoolean("forwarded"))
        val body=JSONObject();for(k in listOf("id","conversationId","senderId"))body.put(k,SyncWire.id(raw.opt(k)))
        val type=raw.getString("type");require(type in setOf("text","image","audio","video","file"));body.put("type",type)
        val text=raw.getString("text");require(text.toByteArray().size<=16384);body.put("text",text)
        if(raw.optString("replyToId").isNotEmpty())body.put("replyToId",SyncWire.id(raw.opt("replyToId")))
        val attachment=selected.optString("attachmentId");if(attachment.isNotEmpty()){require(raw.optString("attachmentId")==attachment&&!raw.optBoolean("forwarded"));body.put("attachmentId",SyncWire.id(attachment))}
        body.put("createdAt",raw.getString("createdAt").also{Instant.parse(it)})
        for(k in listOf("contentVersion","creationEpoch","creationSequence"))body.put(k,positive(raw.opt(k)))
        if(raw.optBoolean("forwarded"))body.put("forwarded",true)
        val mode=selected.getString("objectMode");require(mode in setOf("none","metadata_only","full")&&(type=="text")== (mode=="none")&&(!raw.optBoolean("forwarded")||mode!="full"&&attachment.isEmpty()))
        return JSONObject().put("bodyEnvelope",encode(body.toString().toByteArray(),true)).put("objectMode",mode)
    }
    fun view(raw:JSONObject,i:MigrationIdentity,request:String?=null):MigrationView {
        identity(i);val bytes=decode(raw.getString("manifestEnvelope"),1024*1024,true);val manifestDigest=digest(raw.opt("manifestDigest"));require(hash(bytes)==manifestDigest)
        val manifest=JSONObject(utf8(bytes));require(SyncWire.same(manifest,raw.getJSONObject("manifest")))
        exact(manifest,setOf("format","id","scopeId","userId","sourceContextId","sourceDeviceId","sourceStoreGeneration","targetDeviceId","targetStoreGeneration","restoreEpoch","securityVersion","sourceGeneration","messages","objects","expiresAt"))
        require(manifest.opt("format")==format&&manifest.opt("sourceContextId")=="personal"&&manifest.opt("scopeId")==i.deployment&&manifest.opt("userId")==i.user&&manifest.opt("restoreEpoch")==i.epoch)
        for(k in listOf("id","sourceDeviceId","sourceStoreGeneration","targetDeviceId","targetStoreGeneration"))SyncWire.id(manifest.opt(k))
        require(manifest.getString("sourceDeviceId")!=manifest.getString("targetDeviceId"));positive(manifest.opt("securityVersion"));positive(manifest.opt("sourceGeneration"));Instant.parse(manifest.getString("expiresAt"))
        val messages=manifest.getJSONArray("messages");val objects=manifest.optJSONArray("objects")?:JSONArray();require(messages.length() in 1..256&&objects.length()<=64)
        val ids=mutableSetOf<String>();val objectIDs=mutableSetOf<String>();var full=0
        for(at in 0 until messages.length()){
            val ref=messages.getJSONObject(at);exact(ref,setOf("messageId","conversationId","contentVersion","membershipGeneration","classification","primaryDigest","objectId","objectMode","deadline"))
            require(ids.add(SyncWire.id(ref.opt("messageId"))));SyncWire.id(ref.opt("conversationId"));positive(ref.opt("contentVersion"));positive(ref.opt("membershipGeneration"));digest(ref.opt("primaryDigest"));require(ref.optString("classification") in setOf("available","cloud_only"))
            val mode=ref.getString("objectMode");require(mode in setOf("none","metadata_only","full"));if(ref.has("objectId"))SyncWire.id(ref.opt("objectId"));require(mode!="none"||!ref.has("objectId"));if(mode=="full"){full++;require(ref.has("objectId"))};if(ref.has("deadline"))Instant.parse(ref.getString("deadline"))
        }
        for(at in 0 until objects.length()){
            val ref=objects.getJSONObject(at);exact(ref,setOf("objectId","messageId","sizeBytes","sha256","contentType","classification","deadline"));require(objectIDs.add(SyncWire.id(ref.opt("objectId"))));SyncWire.id(ref.opt("messageId"));digest(ref.opt("sha256"));val size=SyncWire.counter(ref.opt("sizeBytes")).toLong();require(size<=64L*1024*1024);require(ref.optString("classification") in setOf("available","cloud_only"));require(ref.getString("contentType").length<=256&&ref.getString("contentType").none{it=='\r'||it=='\n'})
            require((0 until messages.length()).any{val parent=messages.getJSONObject(it);parent.opt("messageId")==ref.opt("messageId")&&parent.opt("objectId")==ref.opt("objectId")&&parent.opt("objectMode")=="full"});if(ref.has("deadline"))Instant.parse(ref.getString("deadline"))
        }
        require(full==objects.length()&&raw.opt("id")==manifest.opt("id")&&raw.opt("expiresAt")==manifest.opt("expiresAt")&&raw.opt("sourcePreserved")==true&&raw.opt("qualifiedAck")==false&&raw.opt("targetQualified")==false)
        val state=raw.getString("status");require(state in setOf("building","ready","installed_reported","revoking","retired"));val nonce=raw.getString("requestId");require(nonce.length in 8..128);val size=SyncWire.counter(raw.opt("sizeBytes")).toLong();require(size<=limit.toLong()+4096)
        if(request!=null)require(nonce==request&&manifest.opt("sourceDeviceId")==i.device&&manifest.opt("sourceStoreGeneration")==i.store)
        val capsule=raw.getString("capsuleSHA256");if(state in setOf("ready","installed_reported")||capsule.isNotEmpty())digest(capsule)
        return MigrationView(manifest.getString("id"),nonce,state,manifest.toString(),manifestDigest,capsule,size,manifest.getString("expiresAt"))
    }
    private fun aad(v:MigrationView):ByteArray {val m=JSONObject(v.manifest);return (listOf("JobsIM migration v1")+listOf("scopeId","id","userId","sourceDeviceId","sourceStoreGeneration","targetDeviceId","targetStoreGeneration","restoreEpoch","securityVersion","sourceGeneration").map{m.getString(it)}+v.digest).joinToString("\u0000").toByteArray()}
    private fun space(text:String,start:Int):Int {var at=start;while(at<text.length&&text[at] in " \t\r\n")at++;return at}
    private fun stringEnd(text:String,start:Int):Int {require(text[start]=='"');var at=start+1;while(at<text.length){if(text[at]=='\\'){at+=2;continue};if(text[at]=='"')return at+1;at++};error("MIGRATION_JSON_INVALID")}
    private fun valueEnd(text:String,start:Int):Int {
        if(text[start]=='"')return stringEnd(text,start)
        if(text[start]=='{'||text[start]=='['){var depth=0;var at=start;while(at<text.length){val ch=text[at];if(ch=='"'){at=stringEnd(text,at);continue};if(ch=='{'||ch=='[')depth++;if(ch=='}'||ch==']'){depth--;if(depth==0)return at+1};at++};error("MIGRATION_JSON_INVALID")}
        var at=start;while(at<text.length&&text[at] !in ",]} \t\r\n")at++;require(at>start);return at
    }
    fun rawMessages(text:String):List<String> {
        var at=space(text,0);require(text[at]=='{');at++;val keys=mutableSetOf<String>();val result=mutableListOf<String>()
        while(at<text.length){at=space(text,at);if(text[at]=='}')break;val end=stringEnd(text,at);val key=text.substring(at,end);require(key in setOf("\"format\"","\"manifestDigest\"","\"messages\"","\"objects\"")&&keys.add(key));at=space(text,end);require(text[at]==':');val value=space(text,at+1);val finish=valueEnd(text,value)
            if(key=="\"messages\""){at=value;require(text[at]=='[');at=space(text,at+1);while(text[at]!=']'){require(text[at]=='{');val stop=valueEnd(text,at);result.add(text.substring(at,stop));at=space(text,stop);if(text[at]==']')break;require(text[at]==',');at=space(text,at+1)}}
            at=space(text,finish);if(text[at]=='}')break;require(text[at]==',');at++
        };require(keys.size==4&&at<text.length&&text[at]=='}'&&space(text,at+1)==text.length);return result
    }
    fun open(v:MigrationView,capsule:ByteArray,keyResponse:JSONObject,i:MigrationIdentity):MigrationHistory {
        val m=JSONObject(v.manifest);require(m.opt("targetDeviceId")==i.device&&m.opt("targetStoreGeneration")==i.store&&v.state in setOf("ready","installed_reported")&&Instant.parse(v.expires).isAfter(Instant.now()))
        require(capsule.size.toLong()==v.size&&capsule.size>=36&&hash(capsule)==v.capsuleDigest&&String(capsule.copyOfRange(0,8),Charsets.US_ASCII)=="JIMMIGR1")
        require(keyResponse.opt("id")==v.id&&keyResponse.opt("manifestDigest")==v.digest&&keyResponse.opt("algorithm")=="AES-256-GCM"&&keyResponse.opt("expiresAt")==v.expires&&keyResponse.opt("targetQualified")==false)
        var key=ByteArray(0)
        val plain=try{key=decode(keyResponse.getString("key"),32,true);val aadBytes=decode(keyResponse.getString("aad"),8192,true);require(key.size==32&&aadBytes.contentEquals(aad(v)));val cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.DECRYPT_MODE,SecretKeySpec(key,"AES"),GCMParameterSpec(128,capsule.copyOfRange(8,20)));cipher.updateAAD(aadBytes);cipher.doFinal(capsule.copyOfRange(20,capsule.size))}finally{key.fill(0);keyResponse.remove("key")}
        val objects=mutableListOf<MigrationObject>()
        try{
            require(plain.size<=limit);val text=utf8(plain);val data=JSONObject(text);val originalMessages=rawMessages(text);exact(data,setOf("format","manifestDigest","messages","objects"));require(data.opt("format")==format&&data.opt("manifestDigest")==v.digest)
            val rows=data.getJSONArray("messages");val refs=m.getJSONArray("messages");val input=data.optJSONArray("objects")?:JSONArray();val files=m.optJSONArray("objects")?:JSONArray();require(rows.length()==refs.length()&&input.length()==files.length()&&originalMessages.size==rows.length());val messages=mutableListOf<String>()
            for(at in 0 until rows.length()){
                val row=rows.getJSONObject(at);val ref=refs.getJSONObject(at);exact(row,setOf("id","conversationId","senderId","type","text","replyToId","attachmentId","createdAt","contentVersion","creationEpoch","creationSequence","forwarded","objectMode"))
                require(row.opt("id")==ref.opt("messageId")&&row.opt("conversationId")==ref.opt("conversationId")&&row.opt("contentVersion")==ref.opt("contentVersion")&&row.opt("attachmentId")==ref.opt("objectId")&&row.opt("objectMode")==ref.opt("objectMode"));SyncWire.id(row.opt("senderId"));if(row.has("replyToId"))SyncWire.id(row.opt("replyToId"));positive(row.opt("creationEpoch"));positive(row.opt("creationSequence"));Instant.parse(row.getString("createdAt"));require(row.getString("text").toByteArray().size<=16384&&row.optString("type") in setOf("text","image","audio","video","file"));require((row.opt("type")=="text")== (row.opt("objectMode")=="none"));if(row.has("forwarded"))require(row.opt("forwarded")==true&&row.opt("objectMode")!="full"&&!row.has("attachmentId"));require(hash(originalMessages[at].toByteArray())==ref.getString("primaryDigest"));messages.add(row.toString())
            }
            for(at in 0 until input.length()){
                val row=input.getJSONObject(at);val ref=files.getJSONObject(at);exact(row,setOf("objectId","messageId","bytes"));require(row.opt("objectId")==ref.opt("objectId")&&row.opt("messageId")==ref.opt("messageId"));val bytes=decode(row.getString("bytes"),64*1024*1024);if(bytes.size.toLong()!=ref.getString("sizeBytes").toLong()||hash(bytes)!=ref.getString("sha256")){bytes.fill(0);error("MIGRATION_OBJECT_INVALID")};objects.add(MigrationObject(row.getString("objectId"),row.getString("messageId"),ref.getString("contentType"),bytes))
            }
            return MigrationHistory(messages,objects)
        }catch(error:Throwable){objects.forEach{it.bytes.fill(0)};throw error}finally{plain.fill(0)}
    }
    fun validateInstall(raw:JSONObject,v:MigrationView,i:MigrationIdentity):String {
        require(raw.opt("id")==v.id&&raw.opt("status")=="validated"&&raw.opt("manifestDigest")==v.digest&&raw.opt("requiresAtomicLocalCommit")==true&&raw.opt("targetQualified")==false&&raw.opt("qualifiedAck")==false)
        val before=Instant.parse(raw.getString("installBefore"));require(before.isAfter(Instant.now())&&before.isBefore(Instant.now().plusSeconds(6)));val parts=raw.getString("installToken").split('.');require(parts.size==3&&parts[0]=="mi1"&&decode(parts[2],32,true).size==32)
        val proof=JSONObject(utf8(decode(parts[1],8192,true)));require(proof.opt("id")==v.id&&proof.opt("manifestDigest")==v.digest&&proof.opt("userId")==i.user&&proof.opt("deviceId")==i.device&&proof.opt("storeGeneration")==i.store&&proof.opt("restoreEpoch")==i.epoch&&proof.getLong("expires")==before.epochSecond);return before.toString()
    }
}

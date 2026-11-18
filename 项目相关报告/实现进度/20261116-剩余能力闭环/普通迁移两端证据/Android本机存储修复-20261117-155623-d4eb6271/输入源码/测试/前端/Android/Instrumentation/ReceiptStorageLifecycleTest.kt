package com.jobs.im

import android.app.Application
import androidx.room.usePrepared
import androidx.room.useWriterConnection
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.nio.file.Files
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ReceiptStorageLifecycleTest {
    private fun uuid()=UUID.randomUUID().toString()
    private class Fixture(val app:Application,val db:ChatDatabase,val name:String,val i:MigrationIdentity,val sync:SyncIdentity) {
        val store=ReceiptStore(app,db,i){true};val qualification=UUID.randomUUID().toString();val message=UUID.randomUUID().toString();val conversation=UUID.randomUUID().toString();val objectId=UUID.randomUUID().toString()
        fun parent(file:Boolean=false):Pair<ReceiptIntent,String> {
            val raw=JSONObject().put("id",message).put("conversationId",conversation).put("senderId",i.user).put("type",if(file)"file" else "text").put("text","durable original body").put("createdAt","2026-11-17T00:00:00Z").put("contentVersion","1").put("creationEpoch","1").put("creationSequence","1").put("deleted",false)
            if(file)raw.put("attachmentId",objectId)
            val body=raw.toString();val digest=MigrationProtocol.hash(body.toByteArray());val expires=Instant.now().plusSeconds(1800);val p=JSONObject().put("scope",i.deployment).put("user",i.user).put("device",i.device).put("store",i.store).put("epoch",i.epoch).put("qualification",qualification).put("message",message).put("conversation",conversation).put("contentVersion","1").put("bodyDigest",digest).put("permission","a".repeat(64)).put("expires",expires.epochSecond)
            val id=UUID.randomUUID().toString();val request=JSONObject().put("requestId",id).put("storeGeneration",i.store).put("revisionEpoch",i.epoch).put("qualificationId",qualification).put("kind","message_content").put("challenge",challenge("rc1",p)).put("bodyDigest",digest)
            return ReceiptIntent(id,"message_content",i,"2",request.toString(),message,"1",digest,p.toString(),"",expires.toString()) to body
        }
        private fun challenge(kind:String,p:JSONObject)=kind+"."+MigrationProtocol.encode(p.toString().toByteArray(),true)+"."+MigrationProtocol.encode(ByteArray(32),true)
        fun original(parent:ReceiptIntent):Pair<ReceiptIntent,JSONObject> {
            val id=UUID.randomUUID().toString();val digest=MigrationProtocol.hash(ByteArray(0));val p=JSONObject(parent.proof).put("parentReceiptId",parent.id).put("objectId",objectId).put("sizeBytes","0").put("objectDigest",digest)
            val body=JSONObject().put("requestId",id).put("storeGeneration",i.store).put("revisionEpoch",i.epoch).put("qualificationId",qualification).put("kind","object_received").put("parentReceiptId",parent.id).put("challenge",challenge("ro1",p)).put("objectId",objectId).put("sizeBytes","0").put("objectDigest",digest)
            val row=ReceiptIntent(id,"object_received",i,"2",body.toString(),message,"1",parent.digest,p.toString(),"",parent.expires)
            return row to JSONObject().put("objectId",objectId).put("sizeBytes","0").put("sha256",digest).put("contentType","application/octet-stream")
        }
        suspend fun trigger(enabled:Boolean) {db.useWriterConnection{writer->writer.usePrepared(if(enabled)"CREATE TRIGGER fail_receipt_intent BEFORE INSERT ON sync_records WHEN NEW.kind='receipt-intent' BEGIN SELECT RAISE(ABORT,'controlled receipt intent failure'); END" else "DROP TRIGGER fail_receipt_intent"){it.step()}}}
    }
    private fun fixture(action:suspend(Fixture)->Unit)=runBlocking {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application;val name="JobsIM-test-receipt-${uuid()}.db";val db=ChatDatabase.open(app,false,name)
        val profile=uuid();val deployment=uuid();val user=uuid();val device=uuid();val family=uuid();val ns=Isolation.namespace(profile,deployment,user,false,"1","personal",device);val sync=SyncIdentity(ns,profile,deployment,user,"1",device,family);val i=MigrationIdentity(profile,deployment,user,device,family,"1",ns,uuid(),uuid())
        try {db.messages().putNativeScope(NativeScope(uuid(),"live|$profile|$deployment|",ns,device,"test"));val control=sync.binding(i.store,uuid()).put("profile",profile).put("familyId",family).put("guardGeneration",i.guard).put("cursor","fixture-cursor").put("schemaVersion","2").put("permissionGeneration","a".repeat(64));db.syncRecords().put(SyncRecord("$ns|sync-control",ns,"control",control.toString()));action(Fixture(app,db,name,i,sync))}
        finally {db.close();app.deleteDatabase(name);val directory=File(app.noBackupFilesDir.canonicalFile,"ReceiptOriginals/${Isolation.digest(ns)}");if(directory.isDirectory&&!Files.isSymbolicLink(directory.toPath()))directory.listFiles()?.forEach{it.delete()};directory.delete();File(app.noBackupFilesDir.canonicalFile,"Migrations/${Isolation.digest(ns)}").delete()}
    }
    @Test fun fullTransactionPreservesExactBodyAndIntentOnReopenAndRollsBothBackOnFailure()=fixture { f ->
        val (first,body)=f.parent();f.store.stage(first,body)
        val reopened=ChatDatabase.open(f.app,false,f.name)
        try {assertEquals(first.id,ReceiptStore(f.app,reopened,f.i){true}.rows().single().id);val cached=JSONObject(reopened.syncRecords().one("${f.i.namespace}|receipt-body|${first.id}")!!.payload);assertEquals(body,SyncWire.originalBytes(cached.opt("bodyEnvelope"),first.digest))}finally{reopened.close()}
        val (second,other)=f.parent();f.trigger(true);try {assertTrue(runCatching{f.store.stage(second,other)}.isFailure)}finally{f.trigger(false)}
        assertNull(f.db.syncRecords().one("${f.i.namespace}|receipt-intent|${second.id}"));assertNull(f.db.syncRecords().one("${f.i.namespace}|receipt-body|${second.id}"));assertNotNull(f.db.syncRecords().one("${f.i.namespace}|receipt-body|${first.id}"))
    }
    @Test fun zeroOriginalFsyncSharesItsDescriptorIntentTransactionAndGlobalCleansOnlyOwnedBytes()=fixture { f ->
        val (parent,body)=f.parent(true);f.store.stage(parent,body);val (row,ref)=f.original(parent);val committed=f.store.stageObject(row,ref,ByteArray(0));val file=f.store.objectFile(JSONObject(committed.descriptor));assertTrue(file.isFile);assertEquals(0L,file.length());f.store.verifyObject(JSONObject(committed.descriptor));assertNotNull(f.db.syncRecords().one("${f.i.namespace}|receipt-object|${row.id}"))
        val (failed,otherRef)=f.original(parent);f.trigger(true);try{assertTrue(runCatching{f.store.stageObject(failed,otherRef,ByteArray(0))}.isFailure)}finally{f.trigger(false)}
        assertFalse(File(file.parentFile,failed.id+".bin").exists());assertTrue(file.isFile);assertNull(f.db.syncRecords().one("${f.i.namespace}|receipt-object|${failed.id}"))
        f.db.withTransaction{ReceiptStore.eraseGlobalInTransaction(f.db,f.sync,setOf(f.message))};ReceiptStore.drain(f.app,f.db);assertFalse(file.exists());assertNull(f.db.syncRecords().one("${f.i.namespace}|receipt-body|${parent.id}"));assertTrue(f.store.rows().all{it.state=="retired"&&it.body.isEmpty()&&it.proof.isEmpty()})
    }
    @Test fun actualProbeReopensItsCurrentPhysicalDatabaseAndMeasuresInstalledApk()=fixture { f ->
        val measured=f.store.measure(true);assertEquals(f.i.store,measured.getJSONObject("identity").getString("store"));assertTrue(measured.getBoolean("writerFull"));assertTrue(measured.getBoolean("reopenBody"));assertTrue(measured.getBoolean("reopenIntent"));assertTrue(measured.getBoolean("objectFsync"));assertTrue(measured.getBoolean("directoryFsync"));ReceiptProtocol.localMeasurement(measured,f.i,f.store.buildDigest())
        val reopened=ChatDatabase.open(f.app,false,f.name);try{assertNotNull(ReceiptStore(f.app,reopened,f.i){true}.measured(measured.getString("buildDigest")))}finally{reopened.close()}
    }
    @Test fun trustedOsRootAliasDoesNotPermitAnUntrustedGeneratedChildSymlink()=fixture { f ->
        val base=File(f.app.noBackupFilesDir.canonicalFile,"ReceiptOriginals");base.mkdir();val link=File(base,Isolation.digest(f.i.namespace));val outside=File(f.app.noBackupFilesDir.canonicalFile,"JobsIM-test-receipt-outside-${uuid()}");outside.mkdir();Files.createSymbolicLink(link.toPath(),outside.toPath())
        try{val (_,ref)=f.original(f.parent(true).first);assertTrue(runCatching{f.store.saveObject(uuid(),ref,ByteArray(0))}.isFailure);assertTrue(outside.listFiles().orEmpty().isEmpty())}finally{link.delete();outside.delete()}
    }
}

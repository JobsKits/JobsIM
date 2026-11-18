package com.jobs.im

import android.app.Application
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MigrationHistoryLifecycleTest {
    private fun uuid()=UUID.randomUUID().toString()
    private suspend fun fixture(action:suspend(Application,ChatDatabase,MigrationStore,MigrationIdentity,SyncIdentity,JSONObject,JSONObject)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val name="JobsIM-test-migration-history-${uuid()}.db";val db=ChatDatabase.open(app,false,name)
        val profile=uuid();val deployment=uuid();val user=uuid();val device=uuid();val family=uuid()
        val ns=Isolation.namespace(profile,deployment,user,false,"1","personal",device);val sync=SyncIdentity(ns,profile,deployment,user,"1",device,family)
        val identity=MigrationIdentity(profile,deployment,user,device,family,"1",ns,uuid(),uuid())
        try {
            db.messages().putNativeScope(NativeScope(uuid(),"live|$profile|$deployment|",ns,device,"test"))
            val control=sync.binding(identity.store,uuid()).put("profile",profile).put("familyId",family).put("guardGeneration",identity.guard).put("cursor","fixture-cursor")
            db.syncRecords().put(SyncRecord("$ns|sync-control",ns,"control",control.toString()))
            val reference=JSONObject().put("id",uuid()).put("conversationId",uuid()).put("senderId",user).put("type","text").put("text","held reference").put("createdAt","2026-11-17T00:00:00Z").put("contentVersion","1").put("creationEpoch","1").put("creationSequence","1").put("forwarded",true).put("objectMode","none")
            val owned=JSONObject(reference.toString()).put("id",uuid()).put("forwarded",false).put("text","owned before awaited reference").put("createdAt","2026-11-16T00:00:00Z").put("creationSequence","2")
            val store=MigrationStore(app,db);val manifest=JSONObject().put("objects",JSONArray()).put("messages",JSONArray(listOf(owned,reference).map{JSONObject().put("messageId",it.getString("id")).put("classification","available")}))
            val view=MigrationView(uuid(),uuid(),"ready",manifest.toString(),"a".repeat(64),"b".repeat(64),1,Instant.now().plusSeconds(60).toString())
            store.install(identity,view,MigrationHistory(listOf(owned.toString(),reference.toString()),emptyList()),Instant.now().plusSeconds(4).toString()){true}
            action(app,db,store,identity,sync,reference,owned)
        } finally {db.close();app.deleteDatabase(name);File(app.noBackupFilesDir,"Migrations/${Isolation.digest(ns)}").delete()}
    }
    @Test fun validLate200CannotReviveGloballyErasedForwardOrPreviouslyCollectedOwnedBody()=runBlocking {
        fixture { _,db,store,i,sync,reference,owned ->
            val entered=CompletableDeferred<Unit>();val release=CompletableDeferred<Unit>();var reads=0
            val api=object:Api("https://migration.invalid","synthetic") {override suspend fun request(path:String,method:String,data:JSONObject?):JSONObject {reads++;entered.complete(Unit);release.await();return JSONObject(reference.toString()).put("deleted",false)}}
            val service=MigrationService(store,i,{api},{true});val result=async(Dispatchers.Default){service.history()}
            withTimeout(5000){entered.await()};assertEquals(1,reads);assertEquals(2,db.syncRecords().rows(i.namespace,"migration-message").size)
            db.withTransaction {for(id in listOf(reference.getString("id"),owned.getString("id")))db.syncRecords().put(SyncRecord("live|${i.profile}|${i.deployment}|${i.user}|personal|global|$id",i.namespace,"global","{}"));MigrationStore.eraseGlobalInTransaction(db,sync,setOf(reference.getString("id"),owned.getString("id")))}
            assertTrue(db.syncRecords().rows(i.namespace,"migration-message").isEmpty());release.complete(Unit);assertTrue(withTimeout(5000){result.await()}.isEmpty());assertEquals(1,reads)
        }
    }
    @Test fun newerLocalContentVersionAndDeletedFloorRejectOldReferenceAfterAwait()=runBlocking {
        for(deleted in listOf(false,true))fixture { _,db,store,i,_,reference,_ ->
            val entered=CompletableDeferred<Unit>();val release=CompletableDeferred<Unit>();var reads=0
            val api=object:Api("https://migration.invalid","synthetic") {override suspend fun request(path:String,method:String,data:JSONObject?):JSONObject {reads++;entered.complete(Unit);release.await();return JSONObject(reference.toString()).put("deleted",false)}}
            val result=async(Dispatchers.Default){MigrationService(store,i,{api},{true}).history()};withTimeout(5000){entered.await()};assertEquals(1,reads)
            val id=reference.getString("id");db.messages().put(MessageRow("${i.namespace}|$id",i.namespace,reference.getString("conversationId"),id,senderId=i.user,text="newer local",state="received",contentVersion=if(deleted)"1" else "2",retracted=deleted))
            release.complete(Unit);val rows=withTimeout(5000){result.await()};assertFalse(rows.any{JSONObject(it).getString("id")==id});assertEquals(1,reads)
        }
    }
    @Test fun durableUnavailableForwardPlaceholderCanStillUseFreshLegitimate200()=runBlocking {
        fixture { _,db,store,i,_,reference,_ ->
            val id=reference.getString("id");db.messages().put(MessageRow("${i.namespace}|$id",i.namespace,reference.getString("conversationId"),id,senderId=i.user,text="",state="received",contentVersion="1",forwarded=true,forwardUnavailable=true,contentUnavailable=true))
            var reads=0;val api=object:Api("https://migration.invalid","synthetic") {override suspend fun request(path:String,method:String,data:JSONObject?):JSONObject {reads++;return JSONObject(reference.toString()).put("deleted",false)}}
            val rows=MigrationService(store,i,{api},{true}).history();assertEquals(1,reads);assertEquals("held reference",JSONObject(rows.single{JSONObject(it).getString("id")==id}).getString("text"))
        }
    }

}

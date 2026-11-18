package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.UUID
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class GroupLifecycleTest {
    private fun uuid() = UUID.randomUUID().toString()
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private inner class Fixture(val db: ChatDatabase) {
        val profile=uuid();val deployment=uuid();val user=uuid();val device=uuid();var family=uuid();val group=uuid();val invite=uuid();val member=uuid()
        var namespace=Isolation.namespace(profile,deployment,user,false,"1","personal",device)
        val token=invite+"."+"A".repeat(43);var stamp="original";var version="9007199254740993";var role="owner";var mode="ok"
        val scope=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate);val calls=mutableListOf<Triple<String,String,String>>();val results=mutableMapOf<String,JSONObject>()
        fun model()=GroupTools(db,scope,{GroupEnvironment(profile,deployment,user,"1",namespace,stamp,family)},{object:Api("https://group.invalid",""){
            override suspend fun request(path:String,method:String,data:JSONObject?):JSONObject {
                calls.add(Triple(path,method,data?.toString().orEmpty()))
                if(path.endsWith("/governance")&&method=="GET")return JSONObject().put("version",version).put("membershipGeneration","8").put("roleGeneration","9").put("role",role).put("readOnly",false).put("canRecover",true).put("policy",JSONObject().put("muteAll",false).put("slowSeconds",0).put("maxMembers",200))
                if(path.contains("/members?"))return JSONObject().put("items",JSONArray().put(JSONObject().put("id",member).put("username","fixture_member").put("displayName","Fixture member").put("role","member").put("membershipGeneration","2").put("roleGeneration","3"))).put("nextCursor","")
                if(path.contains("/invites?"))return JSONObject().put("items",JSONArray().put(JSONObject().put("id",invite).put("token",token).put("expiresAt","2026-12-17T12:00:00Z").put("maxUses",1).put("uses",0).put("approvalRequired",true).put("revokedAt",JSONObject.NULL))).put("nextCursor","")
                if(path.contains("/join-requests?"))return JSONObject().put("items",JSONArray()).put("nextCursor","")
                if(path.contains("/operations/"))return results[path]?:throw ApiFailure(404,"GROUP_OPERATION_NOT_FOUND")
                val body=data!!;val row=GroupRules.parse(JSONObject(db.syncRecords().rows(namespace,"group-operation").first{JSONObject(it.payload).getString("id")==body.getString("requestId")}.payload));assertEquals("unknown",row.state)
                if(mode=="before-lost")throw ApiFailure(0,"TRANSPORT_UNKNOWN")
                if(row.kind=="governance"&&body.getString("version")!=version)throw ApiFailure(409,"GROUP_PERMISSION_CHANGED")
                val result=when(row.kind){
                    "invite"->JSONObject().put("requestId",row.id).put("id",invite).put("token",token).put("expiresAt","2026-12-17T12:00:00Z").put("maxUses",1).put("approvalRequired",true)
                    "join"->JSONObject().put("requestId",row.id).put("id",uuid()).put("state","pending").put("membershipGeneration",JSONObject.NULL)
                    else->JSONObject().put("requestId",row.id).put("status","applied").put("action",body.getString("action")).put("version","9007199254740994")
                }
                results[row.lookup]=result
                if(mode=="lost")throw ApiFailure(0,"TRANSPORT_UNKNOWN")
                if(mode=="late")stamp="successor"
                return result
            }
        }},{})
        suspend fun open(model:GroupTools,id:String=group){withContext(Dispatchers.Main){model.open(id)};await(model)}
        suspend fun await(model:GroupTools){withTimeout(5000){while(model.busy)delay(10)}}
        suspend fun submit(model:GroupTools,row:GroupIntent,retry:Boolean=true,code:String=""){withContext(Dispatchers.Main){model.submit(row,retry,code)};await(model)}
        fun code()=GroupRules.code(GroupInvitation(deployment,group,invite,token))
    }
    private fun fixture(action:suspend(Fixture)->Unit)=runBlocking{val name="JobsIM-test-group-${uuid()}.db";val db=ChatDatabase.open(context,false,name);val f=Fixture(db);try{action(f)}finally{f.scope.cancel();db.close();context.deleteDatabase(name)}}
    @Test fun originalTransferNeverRebasesAfterConfirmation()=fixture{f->
        val model=f.model();f.open(model);val row=withContext(Dispatchers.Main){model.draft("transfer",model.members.single())};f.version="9007199254740998";f.submit(model,row)
        val stored=model.operations.single();assertEquals("unknown",stored.state);val body=JSONObject(stored.body);assertEquals("9007199254740993",body.getString("version"));assertEquals("2",body.getString("targetMembershipGeneration"));assertEquals("3",body.getString("targetRoleGeneration"));assertEquals(1,f.calls.count{it.first.endsWith("/governance")&&it.second=="GET"})
    }
    @Test fun lostMutationUsesOnlyOriginalResultAndTokensRemainRamOnly()=fixture{f->
        val model=f.model();f.open(model);val row=withContext(Dispatchers.Main){model.draft("invite",fields=JSONObject().put("expiresAt","2026-12-17T12:00:00Z").put("maxUses",1).put("approvalRequired",true))};f.mode="lost";f.submit(model,row);assertEquals("unknown",model.operations.single().state);f.submit(model,model.operations.single(),false)
        assertEquals(1,f.calls.count{it.second=="POST"});assertFalse(f.db.syncRecords().rows(f.namespace,"group-operation").single().payload.contains(f.token));assertEquals(f.invite,GroupRules.invitation(model.share(f.invite),f.deployment).id);model.close();assertThrows(IllegalStateException::class.java){model.share(f.invite)}
    }
    @Test fun coldJoinReusesOneNonceAndRequiresTheOriginalReenteredCode()=fixture{f->
        var model=f.model();f.open(model,"");f.mode="before-lost";withContext(Dispatchers.Main){model.join(f.code())};f.await(model);val row=model.operations.single();assertFalse(row.body.contains("token"));model.close();model=f.model();f.open(model,"");f.submit(model,row,true,"");assertEquals(1,f.calls.count{it.second=="POST"})
        f.mode="ok";withContext(Dispatchers.Main){model.join(f.code())};f.await(model);assertEquals(1,model.operations.size);assertEquals(row.id,model.operations.single().id);assertEquals(row.id,JSONObject(f.calls.last{it.second=="POST"}.third).getString("requestId"))
    }
    @Test fun lateSuccessfulMutationCannotPublishIntoAnotherIdentity()=fixture{f->
        val model=f.model();f.open(model);val row=withContext(Dispatchers.Main){model.draft("dissolve")};f.mode="late";f.submit(model,row);assertEquals("unknown",GroupRules.parse(JSONObject(f.db.syncRecords().rows(f.namespace,"group-operation").single().payload)).state)
    }
    @Test fun roleWithdrawalClearsVisibleInvitesAndRamTokens()=fixture{f->
        val model=f.model();f.open(model);assertTrue(model.share(f.invite).isNotEmpty());f.role="member";withContext(Dispatchers.Main){model.reload()};f.await(model);assertTrue(model.invites.isEmpty());assertTrue(model.requests.isEmpty());assertThrows(IllegalStateException::class.java){model.share(f.invite)}
    }

    @Test fun laterFamilyOrDeviceCannotPublishTheOriginal404AtSameEpoch()=fixture{f->
        val model=f.model();f.open(model);val row=withContext(Dispatchers.Main){model.draft("dissolve")};f.mode="before-lost";f.submit(model,row);val count=f.calls.count{it.second=="POST"};f.family=uuid();f.submit(model,model.operations.single(),false);f.submit(model,model.operations.single(),true);assertEquals(count,f.calls.count{it.second=="POST"});assertEquals("unknown",model.operations.single().state)
        f.namespace=f.namespace+"|successor-device";f.submit(model,model.operations.single(),true);assertEquals(count,f.calls.count{it.second=="POST"})
    }
    @Test fun legacyResponsibilityWithoutOriginalFamilyStaysReadOnly()=fixture{f->
        val model=f.model();f.open(model);val row=withContext(Dispatchers.Main){model.draft("dissolve")}.copy(family="");f.db.syncRecords().put(SyncRecord(row.namespace+"|group-operation|"+row.id,row.namespace,"group-operation",row.json().toString()));withContext(Dispatchers.Main){model.reload()};f.await(model);f.submit(model,model.operations.single(),false);f.submit(model,model.operations.single(),true);assertEquals(0,f.calls.count{it.second=="POST"});assertEquals("unsent",model.operations.single().state)
    }

}

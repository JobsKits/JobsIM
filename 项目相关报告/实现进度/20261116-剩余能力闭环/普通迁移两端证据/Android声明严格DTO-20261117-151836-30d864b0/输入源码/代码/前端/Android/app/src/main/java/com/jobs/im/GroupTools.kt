package com.jobs.im

import androidx.compose.runtime.*
import java.time.Instant
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.util.Base64
import java.util.UUID
import kotlinx.coroutines.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONObject
import org.json.JSONTokener

data class GroupEnvironment(val profile: String, val deployment: String, val user: String, val epoch: String, val namespace: String, val stamp: String, val family: String = "")
data class GroupIntent(val id: String, val conversation: String, val namespace: String, val epoch: String, val kind: String, val method: String,
    val path: String, val lookup: String, val body: String, val created: String, val attempts: Int = 0, val state: String = "unsent", val result: String = "", val family: String = "") {
    fun json() = JSONObject().put("id", id).put("conversation", conversation).put("namespace", namespace).put("epoch", epoch).put("kind", kind)
        .put("method", method).put("path", path).put("lookup", lookup).put("body", body).put("created", created).put("attempts", attempts).put("state", state).put("result", result).also { if(family.isNotEmpty()) it.put("family",family) }
}
data class GroupInvitation(val deployment: String, val conversation: String, val id: String, val token: String) {
    override fun toString() = "GroupInvitation(redacted)"
}
object GroupRules {
    fun counter(value: String): String = SyncWire.counter(value).also { require(it != "0") }
    fun code(invitation: GroupInvitation): String {
        val value = JSONObject().put("deploymentId", invitation.deployment).put("conversationId", invitation.conversation).put("inviteId", invitation.id).put("token", invitation.token)
        return "JobsIM-Invite-1:" + Base64.getEncoder().encodeToString(value.toString().toByteArray(Charsets.UTF_8))
    }
    fun invitation(text: String, deployment: String): GroupInvitation {
        require(text.startsWith("JobsIM-Invite-1:") && text.length < 4096)
        val encoded = text.removePrefix("JobsIM-Invite-1:")
        val bytes = Base64.getDecoder().decode(encoded)
        require(Base64.getEncoder().encodeToString(bytes) == encoded)
        val textValue = Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(bytes)).toString()
        val reader=JSONTokener(textValue); val value=reader.nextValue() as? JSONObject ?: error("GROUP_INVALID_CODE"); require(reader.nextClean()=='\u0000')
        require(value.keys().asSequence().toSet() == setOf("deploymentId", "conversationId", "inviteId", "token") && value.getString("deploymentId") == deployment)
        val group = SyncWire.id(value.opt("conversationId")); val id = SyncWire.id(value.opt("inviteId")); val token = value.getString("token")
        require(token.matches(Regex(Regex.escape(id) + "\\.[A-Za-z0-9_-]{43}")))
        return GroupInvitation(deployment, group, id, token)
    }
    fun parse(value: JSONObject): GroupIntent {
        val fields=setOf("id", "conversation", "namespace", "epoch", "kind", "method", "path", "lookup", "body", "created", "attempts", "state", "result")
        require(value.keys().asSequence().toSet() in setOf(fields,fields+"family"))
        val row = GroupIntent(value.getString("id"), value.getString("conversation"), value.getString("namespace"), value.getString("epoch"), value.getString("kind"), value.getString("method"), value.getString("path"), value.getString("lookup"), value.getString("body"), value.getString("created"), value.getInt("attempts"), value.getString("state"), value.getString("result"), if(value.has("family")) SyncWire.id(value.opt("family")) else "")
        SyncWire.id(row.id); SyncWire.id(row.conversation); require(SyncWire.counter(row.epoch) != "0" && row.namespace.isNotEmpty() && row.attempts >= 0 && row.state in setOf("unsent", "unknown", "applied"))
        require(row.kind in setOf("governance", "invite", "join", "revoke", "decide") && row.method in setOf("POST", "DELETE"))
        val base = "/conversations/${row.conversation}/"
        val expected = when(row.kind) {
            "invite" -> Triple("POST", base+"invites", base+"invites/operations/"+row.id)
            "join" -> Triple("POST", base+"join-requests", base+"join-requests/operations/"+row.id)
            "revoke" -> Triple("DELETE", base+"invites/"+SyncWire.id(row.path.removePrefix(base+"invites/")), base+"governance/operations/"+row.id)
            "decide" -> Triple("POST", base+"join-requests/"+SyncWire.id(row.path.removePrefix(base+"join-requests/")), base+"governance/operations/"+row.id)
            else -> Triple("POST", base+"governance", base+"governance/operations/"+row.id)
        }
        require(row.method==expected.first && row.path==expected.second && row.lookup==expected.third)
        val body = JSONObject(row.body)
        require(body.getString("requestId") == row.id && !body.has("token") && !body.has("stepUpId") && !body.has("otp"))
        require(body.keys().asSequence().all { it in setOf("requestId", "version", "membershipGeneration", "roleGeneration", "action", "targetId", "targetMembershipGeneration", "targetRoleGeneration", "role", "mutedUntil", "muteAll", "slowSeconds", "maxMembers", "expiresAt", "maxUses", "approvalRequired", "inviteId", "approve") })
        Instant.parse(row.created); if (row.result.isNotEmpty()) require(!JSONObject(row.result).has("token"))
        return row
    }
    fun manager(snapshot: JSONObject) = snapshot.optString("role") in setOf("owner", "admin") && snapshot.optBoolean("readOnly").not()
}

class GroupTools(private val database: ChatDatabase, private val scope: CoroutineScope, private val environment: () -> GroupEnvironment,
    private val api: () -> Api, private val updated: suspend () -> Unit) {
    var conversation by mutableStateOf(""); private set
    var snapshot by mutableStateOf(""); private set
    val members = mutableStateListOf<Person>()
    val invites = mutableStateListOf<String>()
    val requests = mutableStateListOf<String>()
    val operations = mutableStateListOf<GroupIntent>()
    var membersNext by mutableStateOf(""); private set
    var invitesNext by mutableStateOf(""); private set
    var requestsNext by mutableStateOf(""); private set
    var busy by mutableStateOf(false); private set
    var failed by mutableStateOf(false); private set
    var notice by mutableStateOf(""); private set
    private val tokens = mutableMapOf<String, String>()
    private var active: Job? = null
    private var life = 0
    private val lock = Mutex()
    private val records get() = database.syncRecords()
    private fun key(row: GroupIntent) = "${row.namespace}|group-operation|${row.id}"
    private fun prefix(env: GroupEnvironment) = "live|${env.profile}|${env.deployment}|${env.user}|"
    private fun check(env: GroupEnvironment, revision: Int) { if (environment() != env || life != revision) throw CancellationException("GROUP_CONTEXT_CHANGED") }
    fun close() { life++; active?.cancel(); tokens.clear(); busy = false }
    private fun run(action: suspend (GroupEnvironment, Int) -> Unit) {
        if (busy) return
        val env = environment(); val revision = life; busy = true; failed = false; notice = ""
        active = scope.launch {
            try { lock.withLock { check(env, revision); require(env.user.isNotEmpty()); action(env, revision) } }
            catch (_: CancellationException) { }
            catch (_: Exception) { if (environment() == env && life == revision) { failed = true; notice = "error" } }
            finally { if (life == revision) { busy = false; active = null } }
        }
    }
    private suspend fun loadOperations(env: GroupEnvironment, revision: Int) {
        val rows = records.across(prefix(env), "group-operation").map { GroupRules.parse(JSONObject(it.payload)).also { row -> require(key(row) == it.key) } }.sortedByDescending { it.created }
        check(env, revision); operations.clear(); operations.addAll(rows)
    }
    fun open(id: String) {
        close(); conversation = id; snapshot = ""; members.clear(); invites.clear(); requests.clear(); membersNext = ""; invitesNext = ""; requestsNext = ""
        reload()
    }
    fun reload() = run { env, revision ->
        loadOperations(env, revision)
        if (conversation.isNotEmpty()) {
            val value = api().request("/conversations/$conversation/governance"); check(env, revision)
            for (field in listOf("version", "membershipGeneration", "roleGeneration")) require(GroupRules.counter(value.getString(field)).isNotEmpty())
            snapshot = value.toString(); page("members", true, env, revision)
            if (GroupRules.manager(value)) { page("invites", true, env, revision); page("join-requests", true, env, revision) }
            else { invites.clear(); requests.clear(); tokens.clear(); invitesNext=""; requestsNext="" }
        }
    }
    fun more(kind: String) = run { env, revision -> page(kind, false, env, revision) }
    private suspend fun page(kind: String, first: Boolean, env: GroupEnvironment, revision: Int) {
        require(kind in setOf("members", "invites", "join-requests"))
        val cursor = if (first) "" else when (kind) { "members" -> membersNext; "invites" -> invitesNext; else -> requestsNext }
        if (!first && cursor.isEmpty()) return
        val value = api().request("/conversations/$conversation/$kind?limit=50" + if (cursor.isNotEmpty()) "&after=$cursor" else ""); check(env, revision)
        val next = value.optString("nextCursor"); if (next.isNotEmpty()) SyncWire.id(next); require(next.isEmpty() || next != cursor)
        val items = value.getJSONArray("items"); require(items.length() <= 50)
        if (kind == "members") {
            val rows = (0 until items.length()).map { person(items.getJSONObject(it)).also { p -> SyncWire.id(p.id); require(SyncWire.counter(p.membershipGeneration) != "0" && SyncWire.counter(p.roleGeneration) != "0") } }
            if (first) members.clear(); for (row in rows) { members.removeAll { it.id == row.id }; members.add(row) }; membersNext = next
        } else if (kind == "invites") {
            if (first) { invites.clear(); tokens.clear() }
            for (i in 0 until items.length()) { val item = items.getJSONObject(i); val id = SyncWire.id(item.opt("id")); tokens[id] = item.getString("token"); item.remove("token"); invites.removeAll { JSONObject(it).optString("id") == id }; invites.add(item.toString()) }; invitesNext = next
        } else { if (first) requests.clear(); for (i in 0 until items.length()) { val item = items.getJSONObject(i); SyncWire.id(item.opt("id")); requests.removeAll { JSONObject(it).optString("id") == item.getString("id") }; requests.add(item.toString()) }; requestsNext = next }
    }
    fun draft(action: String, target: Person? = null, fields: JSONObject = JSONObject()): GroupIntent {
        require(snapshot.isNotEmpty()); val seen = JSONObject(snapshot); val env = environment(); val id = UUID.randomUUID().toString()
        require(if(action=="recover") seen.optBoolean("canRecover") else GroupRules.manager(seen))
        if(action in setOf("settings","role","transfer","dissolve")) require(seen.optString("role")=="owner")
        if(action in setOf("role","mute","transfer","recover")) require(target!=null)
        if(action in setOf("role","mute","transfer")) require(target?.role!="owner")
        val body = JSONObject().put("requestId", id).put("version", seen.getString("version")).put("membershipGeneration", seen.getString("membershipGeneration")).put("roleGeneration", seen.getString("roleGeneration"))
        var kind = "governance"; var path = "/conversations/$conversation/governance"; var lookup = "/conversations/$conversation/governance/operations/$id"; var method = "POST"
        when (action) {
            "invite" -> { kind = "invite"; path = "/conversations/$conversation/invites"; lookup = "/conversations/$conversation/invites/operations/$id" }
            "revoke" -> { kind = "revoke"; path = "/conversations/$conversation/invites/${SyncWire.id(fields.opt("id"))}"; method = "DELETE" }
            "decide" -> { kind = "decide"; path = "/conversations/$conversation/join-requests/${SyncWire.id(fields.opt("id"))}" }
            else -> { require(action in setOf("settings", "role", "mute", "transfer", "dissolve", "recover")); body.put("action", action) }
        }
        fields.keys().forEach { field -> if (field != "id") body.put(field, fields.get(field)) }
        target?.let { body.put("targetId", SyncWire.id(it.id)).put("targetMembershipGeneration", GroupRules.counter(it.membershipGeneration)).put("targetRoleGeneration", GroupRules.counter(it.roleGeneration)) }
        return GroupIntent(id, conversation, env.namespace, env.epoch, kind, method, path, lookup, body.toString(), Instant.now().toString(), family=env.family).also { GroupRules.parse(it.json()) }
    }
    fun join(code: String) {
        val invite = try { GroupRules.invitation(code.trim(), environment().deployment) } catch (_: Exception) { failed = true; notice = "error"; return }
        val env = environment(); val id = UUID.randomUUID().toString(); val base = "/conversations/${invite.conversation}"
        val row = GroupIntent(id, invite.conversation, env.namespace, env.epoch, "join", "POST", "$base/join-requests", "$base/join-requests/operations/$id", JSONObject().put("requestId", id).put("inviteId", invite.id).toString(), Instant.now().toString(), family=env.family)
        submit(row, code = code)
    }
    private suspend fun save(before: GroupIntent?, after: GroupIntent, env: GroupEnvironment, revision: Int) = database.withTransaction {
        check(env, revision); GroupRules.parse(after.json()); require(after.namespace.startsWith(prefix(env)))
        if (before != null) require(records.one(key(before))?.payload == before.json().toString())
        records.put(SyncRecord(key(after), after.namespace, "group-operation", after.json().toString())); check(env, revision)
    }
    fun submit(row: GroupIntent, retry: Boolean = true, code: String = "", otp: String = "") = run { env, revision ->
        require(row.namespace.startsWith(prefix(env)))
        var original = records.one(key(row))?.let { GroupRules.parse(JSONObject(it.payload)) }
        if (original == null && row.kind == "join") {
            loadOperations(env,revision)
            original=operations.firstOrNull { it.kind=="join" && it.conversation==row.conversation && JSONObject(it.body).getString("inviteId")==JSONObject(row.body).getString("inviteId") && (it.state!="applied" || it.result.isNotEmpty() && JSONObject(it.result).optString("state")=="pending") }
        }
        if (original == null) { save(null,row,env,revision);original=row }

        val network = api()
        var result = try { network.request(original.lookup) } catch (error: ApiFailure) { check(env, revision); if (error.status == 404 && error.code == "GROUP_OPERATION_NOT_FOUND") null else throw error }
        check(env, revision)
        if (result == null && !retry) { notice = "unknown"; loadOperations(env, revision); return@run }
        if (result == null) {
            require(original.epoch == env.epoch && original.namespace == env.namespace && original.family.isNotEmpty() && original.family == env.family) { "GROUP_ORIGINAL_SESSION_REQUIRED" }
            val body = JSONObject(original.body)
            if (original.kind == "join") { val invite = GroupRules.invitation(code.trim(), env.deployment); require(invite.conversation == original.conversation && invite.id == body.getString("inviteId")); body.put("token", invite.token) }
            if (body.optString("action") == "recover") {
                require(otp.matches(Regex("[0-9]{6}")))
                val proof = network.request("/auth/step-up", "POST", JSONObject().put("requestId", original.id).put("action", "group.recover").put("targetId", original.conversation).put("payload", JSONObject(original.body)).put("otp", otp)); check(env, revision)
                body.put("stepUpId", SyncWire.id(proof.opt("authorizationId")))
            }
            val before = original; original = original.copy(state = "unknown", attempts = original.attempts + 1); save(before, original, env, revision); loadOperations(env, revision)
            result = network.request(original.path, original.method, body); check(env, revision)
        }
        require(result.optString("requestId") == original.id)
        for (field in listOf("version","membershipGeneration")) if (result.has(field) && !result.isNull(field)) require(SyncWire.counter(result.opt(field)) != "0")
        if (original.kind=="governance") require(result.optString("status")=="applied" && result.optString("action")==JSONObject(original.body).getString("action"))
        if(original.kind in setOf("revoke","decide")) {
            require(result.getString("status")=="applied" && result.getString("action")==if(original.kind=="revoke") "invite_revoke" else "join_decide")
            require(result.getString("targetId")==original.path.substringAfterLast('/'))
            require(if(original.kind=="revoke") result.getString("state")=="revoked" else result.getString("state")==if(JSONObject(original.body).getBoolean("approve")) "approved" else "rejected")
            if(original.kind=="decide" && result.getString("state")=="approved") GroupRules.counter(result.getString("membershipGeneration"))
        }
        if(original.kind=="join") { require(result.getString("state") in setOf("pending","approved","rejected")); SyncWire.id(result.opt("id")); if(result.getString("state")=="approved") GroupRules.counter(result.getString("membershipGeneration")) }
        if (result.has("token")) { require(original.kind=="invite"); val id = SyncWire.id(result.opt("id")); val token=result.getString("token"); require(token.matches(Regex(Regex.escape(id)+"\\.[A-Za-z0-9_-]{43}"))); tokens[id] = token; result.remove("token") }
        val public = JSONObject()
        for (field in listOf("requestId", "status", "action", "version", "id", "expiresAt", "maxUses", "uses", "approvalRequired", "revokedAt", "state", "membershipGeneration", "targetId")) if (result.has(field)) public.put(field, result.get(field))
        save(original, original.copy(state = "applied", result = public.toString()), env, revision); loadOperations(env, revision); notice = if(original.kind=="join") when(public.optString("state")){"pending"->"joinPending";"approved"->"joinApproved";"rejected"->"joinRejected";else->"applied"} else "applied"; updated()
    }
    fun share(id: String): String { val token = tokens[id] ?: error("GROUP_INVITE_RELOAD_REQUIRED"); return GroupRules.code(GroupInvitation(environment().deployment, conversation, id, token)) }
}

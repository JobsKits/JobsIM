package com.jobs.im

import android.app.Application
import android.content.Context
import android.net.Uri
import androidx.compose.runtime.*
import androidx.compose.runtime.snapshots.Snapshot
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import java.io.File
import java.math.BigInteger
import java.util.UUID
import kotlinx.coroutines.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONArray
import org.json.JSONObject

class ChatModel(app: Application, private val makeApi: (String, String, ByteArray?) -> Api) :
    AndroidViewModel(app) {
    constructor(app: Application) : this(app, { base, token, ca -> Api(base, token, ca) })

    val preferences = app.getSharedPreferences("JobsIM.settings", 0)
    var language by mutableStateOf(preferences.getString("language", "zh-Hans")!!)
    var theme by mutableStateOf(preferences.getString("theme", "system")!!)
    var font by mutableFloatStateOf(preferences.getFloat("font", 1f))
    var url by mutableStateOf(preferences.getString("url", "https://localhost:8443")!!)
    var deployment by mutableStateOf(preferences.getString("deployment", "")!!)
    var restoreEpoch by mutableStateOf(preferences.getString("restoreEpoch", "1")!!)
    var profile by mutableStateOf(preferences.getString("profile", UUID.randomUUID().toString())!!)
    var token by mutableStateOf("")
    var userId by mutableStateOf("")
    var username by mutableStateOf("")
    var me by mutableStateOf<Person?>(null)
    var demo by mutableStateOf(false)
    var offline by mutableStateOf(false)
    var busy by mutableStateOf(false)
    var error by mutableStateOf("")
    var errorCode by mutableStateOf("")
    var info by mutableStateOf("")
    var selected by mutableStateOf<Conversation?>(null)
    var reply by mutableStateOf<MessageRow?>(null)
    var draft by mutableStateOf("")
    var draftReady by mutableStateOf(false)
        private set

    var draftMentionIds by mutableStateOf<List<String>>(emptyList())
        private set

    var draftMentionAll by mutableStateOf(false)
        private set

    var avatarPending by mutableStateOf(false)
    var pendingReport by mutableStateOf<LocalOperation?>(null)
    var alertsEnabled by mutableStateOf(preferences.getBoolean("alerts", false))

    fun setAlerts(value: Boolean) {
        alertsEnabled = value
        preferences.edit().putBoolean("alerts", value).apply()
    }

    var foreground = true
    var recoveryCodes by mutableStateOf<List<String>>(emptyList())
    var caBytes: ByteArray? =
        preferences.getString("ca", null)?.let {
            android.util.Base64.decode(it, android.util.Base64.DEFAULT)
        }
    val conversations = mutableStateListOf<Conversation>()
    val people = mutableStateListOf<Person>()
    val contacts = mutableStateListOf<Person>()
    val requests = mutableStateListOf<ContactRequest>()
    val members = mutableStateListOf<Person>()
    val devices = mutableStateListOf<DeviceSession>()
    val reports = mutableStateListOf<ReportRecord>()
    val messages = mutableStateListOf<MessageRow>()
    val unread = mutableStateMapOf<String, Int>()
    val features = mutableStateMapOf<String, Boolean>()
    val profiles = mutableStateMapOf<String, Person>()
    val relationships = mutableStateMapOf<String, Relationship>()
    private val liveDb = ChatDatabase.open(app, false)
    private val demoDb = ChatDatabase.open(app, true)
    private val syncStore = SyncStore(liveDb)
    private val syncEngine = NativeSyncEngine(liveDb)
    val selfState = SelfStateModel(liveDb, { syncIdentity() }, { api() }) { font = it }
    var selfObservationRevision by mutableLongStateOf(0)
        private set
    var showArchived by mutableStateOf(false)
    var showHidden by mutableStateOf(false)
    private var syncReady = false
    private var syncLifecycle = 0L
    private val vault = TokenVault(app)
    private val notifications = LocalAlerts(app)
    private var authDigest = ""
    private var authRequestId = ""
    private val sessionMarkers = NonSecretSessionState(app)
    var pendingAuthentication by mutableStateOf<InitialAuthIntent?>(null)
        private set
    var verificationRequired by mutableStateOf(false)
        private set
    var linkedAuthenticationRequired by mutableStateOf(false)
        private set
    private var nativeDevice = ""
    private var nativeSource = ""
    private var generation = 0
    private fun sessionBinding() = SessionBinding(profile, url, deployment, restoreEpoch)
    private fun ownerTag() = "$generation|$profile|$url|$deployment|$restoreEpoch"
    private val sessions = SessionCoordinator(
        vault, sessionMarkers, { liveDb.messages() },
        { binding, credential -> makeApi(binding.origin, credential, caBytes?.copyOf()) },
        { ownerTag() },
    )
    private var polling = false
    private var notifySince = System.currentTimeMillis()
    private var peopleQuery = ""
    private var draftRevision = 0L
    private val synchronization = Mutex()
    private val verifiedForwards = mutableMapOf<String, MessageRow>()
    private var projectionRevision = 0L
    private val forwardProjectionRequests = mutableMapOf<String, Long>()
    val missingForwardResults = mutableStateListOf<String>()
    var forwardSource by mutableStateOf<MessageRow?>(null)
        private set

    var forwardNeedsReselect by mutableStateOf(false)
        private set

    val discovery =
        MessageDiscovery(
            viewModelScope,
            { "$generation|$namespace|$syncLifecycle" },
            { api() },
            { namespace },
            { restoreEpoch },
            { verifyEpoch() },
            { rows -> clearDerivedForwards(rows, generation) },
        )
    val announcement =
        AnnouncementModel(
            viewModelScope,
            { "$generation|$namespace|$syncLifecycle" },
            { api() },
            { liveDb.messages() },
            { namespace },
            { restoreEpoch },
            { userId },
            {
                verifyEpoch()
                features["role_admission"] == true
            },
        )
    var focusRequest by mutableStateOf("")
        private set

    var focusedMessage by mutableStateOf("")
        private set

    private val namespace
        get() = Isolation.namespace(profile, deployment, userId, demo, restoreEpoch, nativeSource, nativeDevice)

    private val db
        get() = if (demo) demoDb else liveDb

    val storageNamespace get() = namespace

    private fun api(): Api {
        val stamp = generation
        val ns = namespace
        val native = sessions.session
        val transport = if (native != null && !demo) {
            val trustedCa = caBytes?.copyOf()
            val authenticated = SessionApi(native.binding, sessions,
                { credential -> makeApi(native.binding.origin, credential, trustedCa) },
                { failure -> showFailure(failure) }, trustedCa)
            if (features["auth_refresh"] == false) LegacyReadOnlyApi(url, authenticated, trustedCa)
                else authenticated
        } else {
            val raw = makeApi(url, token, caBytes)
            if (!demo && token.isNotBlank()) LegacyReadOnlyApi(url, raw, caBytes) else raw
        }
        if (demo) return transport
        return SyncGuardApi(url, transport, { generation == stamp && namespace == ns }) {
            syncStore.projectionGuard(ns) ?: if (requiresDurableSync())
                LocalProjectionGuard("SYNC_UNREGISTERED", false) else null
        }
    }

    private fun syncIdentity(): SyncIdentity? {
        val native = sessions.session ?: return null
        if (demo || native.userId != userId || native.deviceId != nativeDevice) return null
        return SyncIdentity(namespace, profile, deployment, userId, restoreEpoch, nativeDevice, native.familyId)
    }

    private suspend fun requiresDurableSync(): Boolean =
        !demo && (features["authoritative_sync"] == true || syncIdentity()?.let { syncEngine.responsibility(it) } == true)

    private fun restrictCloud() {
        selfState.reset()
        syncReady = false
        syncLifecycle++
        clearForwardApprovals()
        clearForwardSelection()
        discovery.reset()
        announcement.reset()
        selected = null
        reply = null
        messages.clear()
        conversations.clear()
        members.clear()
        contacts.clear()
        people.clear()
        requests.clear()
        profiles.clear()
        relationships.clear()
        unread.clear()
        focusRequest = ""
        focusedMessage = ""
        notifications.clear()
    }

    private fun loadAuthenticationState() {
        pendingAuthentication = sessionMarkers.intent(profile)
        verificationRequired = sessionMarkers.unknownFamily(profile).isNotBlank()
        linkedAuthenticationRequired = verificationRequired || pendingAuthentication != null
    }


    private fun requireCurrent(stamp: Int) {
        if (stamp != generation) throw CancellationException("CONTEXT_CHANGED")
    }

    private fun showFailure(e: Exception) {
        if (e is CancellationException) return
        if (e is SessionVerificationRequired) {
            clearIdentity()
            loadAuthenticationState()
            verificationRequired = true
            errorCode = e.reason
            error = Strings.text(language, "verifySession")
            return
        }
        offline = e is java.io.IOException || e is org.json.JSONException
        errorCode = if (e is ApiFailure) e.code else e.message.orEmpty()
        val label =
            when {
                errorCode.startsWith("MFA_") -> "otpRequired"
                errorCode == "INVALID_CREDENTIALS" -> "authFailed"
                errorCode.startsWith("SYNC_") -> "cloudUnavailable"
                e is ApiFailure && e.status == 401 -> "sessionExpired"
                errorCode in
                    listOf(
                        "SEND_ADMISSION_CHANGED",
                        "QUALIFICATION_CHANGED",
                        "MENTION_FORBIDDEN",
                        "MENTION_ALL_FORBIDDEN",
                        "MENTION_MEMBER_REQUIRED",
                    ) -> "cloudUnavailable"
                e is ApiFailure && e.status == 403 -> "cloudUnavailable"
                e is ApiFailure && e.status == 413 -> "attachmentLimit"
                else -> "failure"
            }
        error = Strings.text(language, label)
        if (e is ApiFailure && e.status == 401 && token.isNotBlank() && sessions.session == null) {
            val binding = sessionBinding()
            val original = token
            clearIdentity()
            viewModelScope.launch { sessions.clearLegacy(binding, original) }
        }
    }

    private fun job(block: suspend () -> Unit): Job {
        if (busy) return viewModelScope.launch {}
        busy = true
        error = ""
        return viewModelScope.launch {
            try {
                block()
                offline = false
            } catch (e: Exception) {
                showFailure(e)
            } finally {
                busy = false
            }
        }
    }

    init {
        viewModelScope.launch {
            runCatching { drainGlobalCleanup() }.onFailure { if (it is Exception) showFailure(it) }
            loadAuthenticationState()
            if (deployment.isNotBlank())
                runCatching { restoreSession() }.onFailure { if (it is Exception) showFailure(it) }
            while (isActive) {
                delay(5000)
                if (!busy && !polling && !demo && token.isNotBlank()) {
                    polling = true
                    val stamp = generation
                    try {
                        reload()
                    } catch (e: Exception) {
                        if (stamp == generation) showFailure(e)
                    } finally {
                        polling = false
                    }
                }
            }
        }
    }

    private fun clearIdentity(forgetSession: Boolean = true) {
        selfState.reset()
        font = preferences.getFloat("font", 1f)
        showArchived = false; showHidden = false
        syncReady = false
        syncLifecycle++
        if (forgetSession) sessions.forgetMemory()
        nativeDevice = ""
        nativeSource = ""
        clearForwardSelection()
        clearForwardApprovals()
        forwardProjectionRequests.clear()
        missingForwardResults.clear()
        discovery.reset()
        announcement.reset()
        focusRequest = ""
        focusedMessage = ""
        notifications.clear()
        peopleQuery = ""
        notifySince = System.currentTimeMillis()
        generation++
        token = ""
        userId = ""
        username = ""
        me = null
        selected = null
        reply = null
        draft = ""
        draftReady = false
        draftMentionIds = emptyList()
        draftMentionAll = false
        messages.clear()
        conversations.clear()
        contacts.clear()
        people.clear()
        requests.clear()
        members.clear()
        devices.clear()
        reports.clear()
        unread.clear()
        profiles.clear()
        relationships.clear()
        features.clear()
        avatarPending = false
        pendingReport = null
    }

    fun setting(key: String, value: String) {
        preferences.edit().putString(key, value).apply()
        if (key == "language") language = value else if (key == "theme") theme = value
    }

    fun fontScale(value: Float) {
        if (!demo && selfState.enabled) {
            selfJob { selfState.setFont(when (value) { .85f -> "small"; 1.2f -> "large"; else -> "standard" }) }
            return
        }
        font = value
        preferences.edit().putFloat("font", value).apply()
    }

    fun listedConversations(): List<Conversation> = conversations.filter {
        val view = selfState.views[it.id]
        (showArchived || view?.opt("archived") != true) && (showHidden || view?.opt("hidden") != true)
    }.sortedWith(compareBy<Conversation> { selfState.views[it.id]?.takeUnless { v -> v.isNull("pinnedRank") }?.optInt("pinnedRank") ?: Int.MAX_VALUE }.thenBy { it.title })

    fun unreadDescription(conversation: String): String {
        val read = selfState.reads[conversation]
        if (selfState.enabled && read?.opt("countStatus") == "exact_current_source")
            return " · " + Strings.text(language, "serverUnread") + ": " + read.getString("unreadCount")
        val count = unread[conversation] ?: 0
        return if (count > 0) " · " + (if (selfState.enabled) Strings.text(language, "localUnread") + ": " else "") + count else ""
    }

    private fun selfJob(block: suspend () -> Unit): Job {
        val stamp = generation; val ns = namespace
        return job {
            requireCurrent(stamp)
            if (ns != namespace) throw CancellationException("SELF_STATE_CONTEXT_CHANGED")
            block()
        }
    }

    fun refreshSelf(conversation: String? = selected?.id) = selfJob { selfState.refresh(conversation) }
    fun changeView(conversation: String, changes: JSONObject) = selfJob { selfState.changeView(conversation, changes) }
    fun recoverSelf(intent: SelfIntent, retry: Boolean = false) = selfJob { selfState.recover(intent, retry) }
    fun archiveRejectedSelf(intent: SelfIntent) = selfJob { selfState.archiveRejected(intent) }
    fun saveCloudDraft(clear: Boolean = false) {
        val conversation = selected ?: return
        val text = draft
        val source = reply
        selfJob { selfState.saveDraft(conversation.id, text, source, clear) }
    }

    fun loadCloudDraft(recovery: Boolean = false) = selfJob {
        val conversation = selected ?: return@selfJob
        val ns = namespace
        val stamp = generation
        val revision = draftRevision
        selfState.refresh(conversation.id)
        requireCurrent(stamp)
        if (ns != namespace || selected?.id != conversation.id || revision != draftRevision) throw SyncFailure("DRAFT_LOCAL_INPUT_CHANGED")
        val cloud = selfState.drafts[conversation.id] ?: throw SyncFailure("DRAFT_UNAVAILABLE")
        val text = if (recovery) cloud.optJSONObject("recovery")?.optString("ownedText")
            else cloud.opt("ownedText") as? String
        if (text == null) throw SyncFailure("DRAFT_UNAVAILABLE")
        var reference: MessageRow? = null
        if (!recovery && cloud.opt("replyState") == "available") cloud.optJSONObject("reply")?.let {
            val source = try { api().request("/messages/${it.getString("messageId")}") }
                catch (error: ApiFailure) { if (error.status in setOf(403, 404)) null else throw error }
            requireCurrent(stamp)
            if (source != null && source.opt("id") == it.opt("messageId") && source.opt("conversationId") == conversation.id &&
                source.opt("contentVersion") == it.opt("sourceVersion") && source.opt("deleted") != true &&
                source.opt("contentUnavailable") != true && source.opt("forwardUnavailable") != true) reference = message(source, ns, restoreEpoch)
        }
        requireCurrent(stamp)
        if (ns != namespace || selected?.id != conversation.id || revision != draftRevision) throw SyncFailure("DRAFT_LOCAL_INPUT_CHANGED")
        reply = reference
        draftMentionIds = emptyList(); draftMentionAll = false
        updateDraft(text)
    }

    fun copyIndependentDraft(conversation: String) = selfJob {
        val stamp = generation; val ns = namespace
        val text = selfState.independentDraftText(conversation) ?: throw SyncFailure("DRAFT_UNAVAILABLE")
        requireCurrent(stamp)
        if (ns != namespace) throw CancellationException("SELF_STATE_CONTEXT_CHANGED")
        val clipboard = getApplication<Application>().getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
        clipboard.setPrimaryClip(android.content.ClipData.newPlainText(Strings.text(language, "cloudDraft"), text))
        info = Strings.text(language, "copied")
    }

    suspend fun observeVisible(keys: Set<String>) {
        val conversation = selected ?: return
        if (demo || !foreground || !syncReady || !selfState.enabled) return
        val ids = messages.filter { it.key in keys && it.conversationId == conversation.id && it.state == "received" &&
            !it.retracted && !it.contentUnavailable && !it.forwardUnavailable }.map { it.id }
        try { selfState.observe(conversation, ids) } catch (error: CancellationException) { throw error }
        catch (error: Exception) { showFailure(error) }
    }

    fun connect(address: String, expected: String, ca: ByteArray?, confirmed: () -> Unit) = job {
        val base = Isolation.endpoint(address)
        val result = Api(base, ca = ca).request("/bootstrap")
        val identity = result.getString("deploymentId")
        require(expected.isNotBlank() && expected == identity) { "DEPLOYMENT_MISMATCH" }
        require(result.optInt("protocolVersion", 1) == 1) { "PROTOCOL_UNSUPPORTED" }
        val changed = base != url || identity != deployment || !(caBytes contentEquals ca)
        if (changed) {
            clearIdentity()
            profile = UUID.randomUUID().toString()
        }
        val next = result.optString("restoreEpoch", "1")
        require(next.matches(Regex("[1-9][0-9]*")))
        if (!changed && restoreEpoch != next) rotateEpoch(next) else restoreEpoch = next
        url = base
        deployment = identity
        caBytes = ca
        demo = false
        preferences
            .edit()
            .putString("url", base)
            .putString("deployment", identity)
            .putString("profile", profile)
            .putString("restoreEpoch", restoreEpoch)
            .putString(
                "ca",
                ca?.let { android.util.Base64.encodeToString(it, android.util.Base64.NO_WRAP) },
            )
            .apply()
        loadAuthenticationState()
        info = Strings.text(language, "verified")
        confirmed()
    }

    private fun publishSession(session: NativeSession) {
        clearIdentity(forgetSession = false)
        nativeDevice = session.deviceId
        nativeSource = session.sourceContextId
        token = session.token
        userId = session.userId
        username = session.username
        demo = false
        verificationRequired = false
        pendingAuthentication = null
        linkedAuthenticationRequired = false
    }

    private suspend fun restoreSession() {
        val stamp = generation
        val binding = sessionBinding()
        val trustedCa = caBytes?.copyOf()
        val boot = makeApi(binding.origin, "", trustedCa).request("/bootstrap")
        requireCurrent(stamp)
        require(boot.getString("deploymentId") == binding.deployment)
        if (boot.getString("restoreEpoch") != binding.epoch) {
            rotateEpoch(boot.getString("restoreEpoch"))
            error("RESTORE_EPOCH_CHANGED")
        }
        val caps = boot.optJSONObject("features")
        caps?.keys()?.forEach { features[it] = caps.optBoolean(it) }
        loadAuthenticationState()
        if (features["auth_refresh"] == true) {
            val restored = sessions.restore(binding, ownerTag())
            if (restored == null) {
                if (vault.loadLegacy(profile) != null) verificationRequired = true
                return
            }
            val context = api().request("/auth/context")
            requireCurrent(stamp)
            val active = sessions.capture()?.let { sessions.requireCurrent(it) }
                ?: error("SESSION_NOT_ACTIVE")
            require(context.getString("restoreEpoch") == active.binding.epoch &&
                context.getString("familyId") == active.familyId &&
                context.getString("deviceId") == active.deviceId &&
                context.getString("sourceContextId") == active.sourceContextId &&
                context.getString("refreshGeneration") == active.refreshGeneration &&
                context.getJSONObject("user").getString("id") == active.userId)
            publishSession(active)
        } else {
            val saved = vault.loadLegacy(profile) ?: return
            val transport = makeApi(url, saved, trustedCa)
            val context = transport.request("/auth/context")
            requireCurrent(stamp)
            require(context.getString("restoreEpoch") == restoreEpoch)
            val user = context.getJSONObject("user")
            clearIdentity()
            token = saved
            userId = user.getString("id")
            username = user.getString("username")
            demo = false
        }
        me = person(api().request("/me"))
        reload()
    }

    fun authenticate(
        mode: String, name: String, password: String, code: String = "", otp: String = "",
        replaceUnknown: Boolean = false,
    ) = job {
        require(deployment.isNotBlank()) { "VERIFY_SERVER_FIRST" }
        require(name.matches(Regex("[A-Za-z0-9_]{3,32}"))) { "INVALID_USERNAME" }
        verifyEpoch()
        val stamp = generation
        val binding = sessionBinding()
        val tag = ownerTag()
        val native = features["auth_refresh"] == true
        if (!native) require(sessionMarkers.intent(profile) == null &&
            sessionMarkers.unknownFamily(profile).isBlank() && vault.loadSession(binding) == null) {
            "AUTH_REFRESH_UNSUPPORTED"
        }
        val anonymous = makeApi(url, "", caBytes?.copyOf())
        if (mode == "recover") {
            require(sessionMarkers.intent(profile) == null) { "AUTH_INTENT_PENDING" }
            anonymous.request("/auth/recover", "POST", JSONObject().put("username", name)
                .put("password", password).put("code", code)
                .put("requestId", UUID.randomUUID().toString()).put("deviceName", "JobsIM Android"))
            requireCurrent(stamp)
            info = Strings.text(language, "login")
            return@job
        }
        require(mode in listOf("login", "register", "reauthenticate"))
        var intent: InitialAuthIntent? = null
        if (native) {
            val original = sessionMarkers.intent(profile)
            val family = sessionMarkers.unknownFamily(profile)
            intent = when {
                replaceUnknown -> {
                    require(original != null || family.isNotBlank()) { "AUTH_PREVIOUS_REQUIRED" }
                    val chain = sessionMarkers.unresolvedRequestIds(profile)
                    require(chain.size <= 1 || features["auth_retirement_chain"] == true) {
                        "AUTHENTICATION_CHAIN_VERIFICATION_REQUIRED"
                    }
                    require(original == null || original.username == name) { "AUTH_INTENT_CONFLICT" }
                    InitialAuthIntent(UUID.randomUUID().toString(), "reauthenticate", name,
                        family, original?.requestId.orEmpty(), previousRequestIds = chain)
                }
                original != null -> {
                    require(original.username == name) { "AUTH_INTENT_CONFLICT" }
                    require(!original.deliveryExpired) { "DELIVERY_EXPIRED" }
                    original
                }
                family.isNotBlank() -> error("AUTH_VERIFICATION_REQUIRED")
                else -> InitialAuthIntent(UUID.randomUUID().toString(), mode, name)
            }
            sessions.prepareInitial(binding, intent, tag)
            loadAuthenticationState()
        } else {
            val digest = Isolation.digest(mode + "\u0000" + name + "\u0000" + password + "\u0000" + otp)
            if (authDigest != digest) { authDigest = digest; authRequestId = UUID.randomUUID().toString() }
        }
        val purpose = intent?.purpose ?: mode
        val requestId = intent?.requestId ?: authRequestId
        val payload = JSONObject().put("username", name).put("password", password)
            .put("requestId", requestId).put("deviceName", "JobsIM Android")
        if (otp.isNotBlank()) payload.put("otp", otp)
        intent?.previousFamilyId?.takeIf { it.isNotBlank() }?.let { payload.put("previousFamilyId", it) }
        intent?.previousRequestId?.takeIf { it.isNotBlank() }?.let { payload.put("previousRequestId", it) }
        intent?.previousRequestIds?.takeIf { it.isNotEmpty() }?.let { payload.put("previousRequestIds", JSONArray(it)) }
        if (intent != null && intent.previousRequestIds.size > 1)
            require(features["auth_retirement_chain"] == true) { "AUTHENTICATION_CHAIN_VERIFICATION_REQUIRED" }
        val result = try {
            anonymous.request("/auth/$purpose", "POST", payload)
        } catch (failure: ApiFailure) {
            requireCurrent(stamp)
            if (native && failure.code == "DELIVERY_EXPIRED") sessionMarkers.expired(profile)
            loadAuthenticationState()
            throw failure
        }
        requireCurrent(stamp)
        if (native) {
            val delivered = NativeSession.delivery(result, binding)
            val committed = sessions.commitInitial(delivered, requestId, tag)
            publishSession(committed)
        } else {
            require(result.optString("restoreEpoch", restoreEpoch) == restoreEpoch)
            val saved = result.getString("token")
            sessions.commitLegacy(binding, saved, tag)
            requireCurrent(stamp)
            val user = result.getJSONObject("user")
            clearIdentity()
            token = saved
            userId = user.getString("id")
            username = user.getString("username")
            demo = false
        }
        authDigest = ""
        authRequestId = ""
        me = person(api().request("/me"))
        reload()
    }

    fun preview() = job {
        clearIdentity()
        demo = true
        userId = "preview-user"
        username = "Preview"
        conversations.addAll(
            listOf(
                Conversation("welcome", "JobsIM"),
                Conversation("team", "Studio", "group", userId),
            )
        )
        contacts.addAll(listOf(Person("alex", "Alex", "alex"), Person("lin", "Lin", "lin")))
        people.addAll(contacts)
        val ns = namespace
        if (demoDb.messages().list(ns, "welcome").isEmpty())
            demoDb
                .messages()
                .put(
                    MessageRow(
                        "$ns|welcome",
                        ns,
                        "welcome",
                        "welcome",
                        senderId = "guide",
                        text = "Hello. This conversation stays on this device.",
                    )
                )
    }

    private suspend fun rotateEpoch(next: String) {
        sessions.invalidateEpoch(sessionBinding(), token, ownerTag())
        clearIdentity()
        restoreEpoch = next
        preferences.edit().putString("restoreEpoch", next).apply()
    }

    private suspend fun verifyEpoch() {
        if (demo || deployment.isBlank()) return
        val stamp = generation
        val result = makeApi(url, "", caBytes?.copyOf()).request("/bootstrap")
        requireCurrent(stamp)
        require(result.getString("deploymentId") == deployment) { "DEPLOYMENT_MISMATCH" }
        val next = result.optString("restoreEpoch", "1")
        require(next.matches(Regex("[1-9][0-9]*")))
        if (next != restoreEpoch) {
            rotateEpoch(next)
            throw IllegalStateException("RESTORE_EPOCH_CHANGED")
        }
        val caps = result.optJSONObject("features")
        features.clear()
        caps?.keys()?.forEach { features[it] = caps.optBoolean(it) }
    }

    suspend fun reload() {
        if (demo) return
        val stamp = generation
        drainGlobalCleanup()
        requireCurrent(stamp)
        verifyEpoch()
        requireCurrent(stamp)
        if (requiresDurableSync()) {
            reloadAuthoritative(stamp)
            return
        }
        val transport = api()
        val array = transport.request("/conversations").optJSONArray("items") ?: JSONArray()
        val next = mutableListOf<Conversation>()
        for (i in 0 until array.length()) {
            val value = array.getJSONObject(i)
            val ids = value.optJSONArray("memberIds") ?: JSONArray()
            var title = value.optString("title")
            val kind = value.optString("kind", "direct")
            if (title.isBlank()) {
                val peers =
                    transport
                        .request("/conversations/${value.getString("id")}/members")
                        .optJSONArray("items") ?: JSONArray()
                val users = (0 until peers.length()).map { person(peers.getJSONObject(it)) }
                title =
                    users.firstOrNull { it.id != userId }?.name
                        ?: Strings.text(language, if (kind == "self") "private" else "group")
            }
            next.add(
                Conversation(
                    value.getString("id"),
                    title,
                    kind,
                    value.optString("ownerId"),
                    (0 until ids.length()).map { ids.getString(it) },
                    value.optBoolean("readOnly"),
                    value.optString("membershipGeneration"),
                    value.optString("sendAdmission"),
                    value.optString("roleGeneration"),
                )
            )
        }
        requireCurrent(stamp)
        conversations.clear()
        conversations.addAll(next)
        val oldSelection = selected
        if (oldSelection != null) {
            val listed = next.find { it.id == oldSelection.id }
            if (listed != null) selected = listed
            else {
                try {
                    val current =
                        conversation(
                            transport.request("/conversations/${oldSelection.id}"),
                            oldSelection.title,
                        )
                    requireCurrent(stamp)
                    if (selected?.id == oldSelection.id) {
                        selected = current
                        if (conversations.none { it.id == current.id }) conversations.add(current)
                        if (next.none { it.id == current.id }) next.add(current)
                    }
                } catch (e: ApiFailure) {
                    requireCurrent(stamp)
                    if (e.status in listOf(403, 404) && selected?.id == oldSelection.id)
                        closeConversation()
                    else throw e
                }
            }
        }
        val ns = namespace
        val avatar = liveDb.messages().operation(ns, "avatar")
        val report = liveDb.messages().operation(ns, "report")
        requireCurrent(stamp)
        avatarPending = avatar != null
        pendingReport = report
        refreshContacts(transport, stamp)
        profiles.clear()
        contacts.forEach { profiles[it.id] = it }
        selected?.let {
            val peers =
                transport.request("/conversations/${it.id}/members").optJSONArray("items")
                    ?: JSONArray()
            requireCurrent(stamp)
            for (i in 0 until peers.length()) {
                val user = person(peers.getJSONObject(i))
                profiles[user.id] = user
            }
        }
        for (conversation in next) {
            requireCurrent(stamp)
            syncConversation(conversation, transport, stamp)
        }
        selected?.let {
            refreshForwardProjections(it, stamp)
            refreshVisible(it, namespace, stamp)
        }
        val states = liveDb.messages().states(namespace)
        requireCurrent(stamp)
        unread.clear()
        states.forEach { unread[it.conversationId] = it.unread }
    }

    private suspend fun reloadAuthoritative(stamp: Int) {
        val identity = syncIdentity() ?: run {
            restrictCloud()
            throw SyncFailure("SYNC_NATIVE_SESSION_REQUIRED")
        }
        val capture = sessions.capture() ?: throw CancellationException("SESSION_CHANGED")
        val ns = namespace
        val oldSelection = selected?.id
        val wasReady = syncReady
        val current = {
            generation == stamp && namespace == ns &&
                runCatching { sessions.requireCurrent(capture) }.isSuccess
        }
        if (!syncReady) restrictCloud()
        val result = try {
            syncEngine.run(identity, api(), if (foreground) oldSelection else null, current) {
                if (current()) restrictCloud()
            }
        } catch (failure: Exception) {
            if (current() && (failure is ApiFailure && failure.code == "STORE_GENERATION_CHANGED" ||
                    failure is SyncFailure && failure.reason == "SYNC_STORE_INVALID"))
                sessions.reject(capture, "STORE_GENERATION_CHANGED")
            throw failure
        }
        if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
        syncReady = true
        syncLifecycle++
        selfState.activate(features["sync_self_state"] == true, features["cloud_drafts"] == true)
        if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
        selfObservationRevision++
        val available = result.objects.filter { it.opt("state") == "available" }
        val next = available.filter { it.getJSONObject("ref").opt("kind") == "conversation" }
            .map { conversation(it.getJSONObject("value"), Strings.text(language, "private")) }
        conversations.clear()
        conversations.addAll(next)
        val blocked = available.firstOrNull { it.getJSONObject("ref").opt("kind") == "relationship" }
            ?.optJSONObject("value")?.optJSONArray("items") ?: JSONArray()
        val blockedIds = (0 until blocked.length()).map { blocked.getJSONObject(it).getString("peerId") }.toSet()
        val directory = available.firstOrNull { it.getJSONObject("ref").opt("kind") == "contacts" }
            ?.optJSONObject("value")?.optJSONArray("items") ?: JSONArray()
        contacts.clear()
        relationships.clear()
        for (index in 0 until directory.length()) {
            val item = directory.getJSONObject(index)
            val peer = item.getString("peerId")
            val friend = item.optString("state") == "friend"
            relationships[peer] = Relationship(peer in blockedIds, friend, item.optString("inboundGate"))
            if (friend) contacts.add(profiles[peer] ?: Person(peer, peer, ""))
        }
        requests.clear()
        available.filter { it.getJSONObject("ref").opt("kind") == "contact_request" }.forEach {
            val item = it.getJSONObject("value")
            if (item.opt("state") == "pending" && item.opt("recipientId") == userId) {
                val requester = item.getString("requesterId")
                val known = profiles[requester]
                requests.add(ContactRequest(item.getString("id"), requester, known?.name ?: requester, known?.username.orEmpty()))
            }
        }
        if (peopleQuery.isBlank()) { people.clear(); people.addAll(contacts) }
        available.firstOrNull { it.getJSONObject("ref").opt("kind") == "profile" }?.optJSONObject("value")?.let {
            if (it.opt("id") == userId) {
                val currentProfile = person(it)
                me = currentProfile
                profiles[userId] = currentProfile
            }
        }
        val states = liveDb.messages().states(ns)
        if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
        unread.clear()
        states.forEach { unread[it.conversationId] = it.unread }
        if (result.globalIds.isNotEmpty()) {
            notifications.clear()
            clearForwardApprovals()
            clearForwardSelection()
            discovery.reset()
            announcement.reset()
            if (reply?.forwarded == true) reply = null
            val holders = liveDb.messages().forwardHolders("live|${identity.profile}|${identity.deployment}|${identity.user}|")
            if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
            holders.forEach {
                clearForwardSidecar(it.namespace, it.id)
            }
        }
        result.messages.filter { it.forwardUnavailable || it.contentUnavailable || it.retracted }.forEach {
            verifiedForwards.remove(it.id)
            clearForwardSidecar(ns, it.id)
            if (reply?.id == it.id) reply = null
        }
        for (id in result.globalIds) {
            verifiedForwards.remove(id)
            forwardProjectionRequests[id] = ++projectionRevision
            val holders = liveDb.messages().across("live|${identity.profile}|${identity.deployment}|${identity.user}|", id)
            if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
            holders.forEach { clearForwardSidecar(it.namespace, id) }
            discovery.invalidate(id)
            if (reply?.id == id) reply = null
            if (forwardSource?.id == id) clearForwardSelection()
        }
        drainGlobalCleanup()
        if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
        val chosen = next.firstOrNull { it.id == oldSelection }
        if (oldSelection != null && chosen == null) closeConversation()
        if (chosen != null) {
            selected = chosen
            refreshForwardProjections(chosen, stamp)
            refreshVisible(chosen, ns, stamp)
        }
        if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
        if (wasReady) result.newIncoming.filter { it.createdAt >= notifySince && it.conversationId != selected?.id && !selfState.isMuted(it.conversationId) }.forEach {
            val title = next.firstOrNull { conversation -> conversation.id == it.conversationId }?.title ?: return@forEach
            notifications.deliver(ns, namespace, it, title, language)
        }
        val avatarOperation = liveDb.messages().operation(ns, "avatar")
        val reportOperation = liveDb.messages().operation(ns, "report")
        if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
        avatarPending = avatarOperation != null
        pendingReport = reportOperation
    }

    private suspend fun refreshContacts(transport: Api = api(), stamp: Int = generation) {
        val result = transport.request("/contacts")
        val users = result.optJSONArray("items") ?: JSONArray()
        val inbox = result.optJSONArray("requests") ?: JSONArray()
        requireCurrent(stamp)
        contacts.clear()
        contacts.addAll((0 until users.length()).map { person(users.getJSONObject(it)) })
        requests.clear()
        for (i in 0 until inbox.length()) {
            val r = inbox.getJSONObject(i)
            if (r.optString("state") == "pending" && r.optString("recipientId") == userId) {
                val who = r.optJSONObject("requester")
                requests.add(
                    ContactRequest(
                        r.getString("id"),
                        r.getString("requesterId"),
                        who?.optString("displayName")?.ifBlank { who.optString("username") }
                            ?: Strings.text(language, "unavailable"),
                        who?.optString("username").orEmpty(),
                    )
                )
            }
        }
        if (peopleQuery.isBlank()) {
            people.clear()
            people.addAll(contacts)
        }
    }

    private suspend fun syncConversation(conversation: Conversation, transport: Api, stamp: Int) =
        synchronization.withLock {
            requireCurrent(stamp)
            val ns = namespace
            var cursor = liveDb.messages().state(ns, conversation.id)?.cursor ?: "$restoreEpoch:0"
            repeat(10) {
                val page =
                    transport.request(
                        "/conversations/${conversation.id}/messages?after=" +
                            java.net.URLEncoder.encode(cursor, "UTF-8") +
                            "&limit=200"
                    )
                requireCurrent(stamp)
                val items = page.optJSONArray("items") ?: JSONArray()
                val next = page.getString("cursor")
                val rows =
                    (0 until items.length()).map {
                        message(items.getJSONObject(it), ns, restoreEpoch)
                    }
                val visible = foreground && selected?.id == conversation.id
                val fresh =
                    liveDb
                        .messages()
                        .page(ns, conversation.id, restoreEpoch, next, rows, userId, visible)
                if (!visible)
                    fresh
                        .filter { it.createdAt >= notifySince }
                        .forEach {
                            notifications.deliver(ns, namespace, it, conversation.title, language)
                        }
                if (next == cursor || items.length() < 200) return@withLock
                cursor = next
            }
        }

    fun refresh() = job { if (demo) selected?.let { loadMessages(it) } else reload() }

    fun open(conversation: Conversation) = job {
        selectConversation(conversation)
        loadMessages(conversation, loadDraft = true)
    }

    private fun selectConversation(conversation: Conversation) {
        clearForwardApprovals()
        announcement.reset()
        Snapshot.withMutableSnapshot {
            selected = conversation
            messages.clear()
            reply = null
            draft = ""
            draftReady = false
            draftMentionIds = emptyList()
            draftMentionAll = false
            members.clear()
            focusRequest = ""
            focusedMessage = ""
        }
    }

    fun closeConversation() {
        clearForwardApprovals()
        announcement.reset()
        selected = null
        messages.clear()
        reply = null
        draft = ""
        draftReady = false
        draftMentionIds = emptyList()
        draftMentionAll = false
        focusRequest = ""
        focusedMessage = ""
    }

    fun onForeground(value: Boolean) {
        foreground = value
        clearForwardApprovals()
        for (i in messages.indices) if (messages[i].forwarded)
            messages[i] = Forwarding.hidden(messages[i])
        if (value && !demo && token.isNotBlank())
            viewModelScope.launch {
                val stamp = generation
                try {
                    verifyEpoch()
                    selected?.let { loadMessages(it) }
                } catch (e: Exception) {
                    if (generation == stamp) showFailure(e)
                }
            }
    }

    fun consumeFocus(id: String) {
        if (focusRequest == id) focusRequest = ""
    }

    fun openSource(row: MessageRow, ready: () -> Unit = {}) = job {
        require(!demo && row.namespace == namespace)
        val stamp = generation
        val ns = namespace
        verifyEpoch()
        requireCurrent(stamp)
        var conversationAuthorized = false
        try {
            val transport = api()
            val currentConversation =
                conversation(
                    transport.request("/conversations/${row.conversationId}"),
                    Strings.text(language, "private"),
                )
            conversationAuthorized = true
            val current = message(transport.request("/messages/${row.id}"), ns, restoreEpoch)
            requireCurrent(stamp)
            require(current.id == row.id && current.conversationId == currentConversation.id)
            synchronization.withLock {
                requireCurrent(stamp)
                val cursor =
                    liveDb.messages().state(ns, row.conversationId)?.cursor ?: "$restoreEpoch:0"
                liveDb
                    .messages()
                    .page(
                        ns,
                        row.conversationId,
                        restoreEpoch,
                        cursor,
                        listOf(current),
                        userId,
                        true,
                    )
            }
            requireCurrent(stamp)
            selectConversation(currentConversation)
            if (conversations.none { it.id == currentConversation.id })
                conversations.add(currentConversation)
            focusRequest = current.id
            focusedMessage = current.id
            loadMessages(currentConversation, loadDraft = true)
            ready()
        } catch (e: ApiFailure) {
            if (generation == stamp && e.status in listOf(401, 403, 404, 409)) {
                val affected =
                    discovery.items.filter {
                        if (!conversationAuthorized || e.status == 403)
                            it.conversationId == row.conversationId
                        else it.id == row.id
                    } + row
                clearDerivedForwards(affected, stamp)
                if (!conversationAuthorized || e.status == 403)
                    discovery.invalidateConversation(row.conversationId, row.id)
                else discovery.invalidate(row.id)
            }
            throw e
        }
    }

    private suspend fun refreshVisible(conversation: Conversation, ns: String, stamp: Int) {
        val loaded = db.messages().list(ns, conversation.id)
        if (generation != stamp || namespace != ns || selected?.id != conversation.id) return
        // Room can suspend while another refresh completes. Commit one merged snapshot on Main.
        val combined =
            (messages.toList() + loaded)
                .filter { it.namespace == ns && it.conversationId == conversation.id }
                .groupBy { it.key }
                .values
                .map { versions ->
                    versions.reduce { old, row ->
                        when {
                            old.retracted -> old
                            row.retracted -> row
                            BigInteger(old.version) > BigInteger(row.version) -> old
                            old.version == row.version &&
                                old.contentUnavailable &&
                                !row.forwarded -> old
                            old.version == row.version &&
                                old.state == "frozen" &&
                                row.state == "pending" -> old
                            else -> row
                        }
                    }
                }
        val next =
            combined
                .sortedWith(
                    compareBy<MessageRow> { if (it.state in setOf("pending", "frozen")) 1 else 0 }
                        .thenBy { it.createdAt }
                        .thenBy { BigInteger(it.sequence) }
                        .thenBy { it.id }
                )
        Snapshot.withMutableSnapshot {
            messages.clear()
            messages.addAll(
                next.map { row ->
                    if (row.forwarded && !row.retracted && !row.contentUnavailable)
                        verifiedForwards[row.id]?.takeIf { !it.retracted && it.version == row.version }
                            ?: Forwarding.hidden(row)
                    else row
                }
            )
        }
    }

    private suspend fun clearDerivedForwards(rows: List<MessageRow>, stamp: Int) {
        val ns = namespace
        val affected = rows.filter { it.namespace == ns && it.forwarded }.distinctBy { it.id }
        synchronization.withLock {
            requireCurrent(stamp)
            for (row in affected) {
                clearForwardSidecar(ns, row.id)
                verifiedForwards.remove(row.id)
                forwardProjectionRequests[row.id] = ++projectionRevision
                val stored = liveDb.messages().one(ns, row.id) ?: continue
                requireCurrent(stamp)
                val cursor =
                    liveDb.messages().state(ns, row.conversationId)?.cursor ?: "$restoreEpoch:0"
                liveDb
                    .messages()
                    .page(
                        ns,
                        row.conversationId,
                        restoreEpoch,
                        cursor,
                        listOf(Forwarding.hidden(stored)),
                        userId,
                        false,
                    )
            }
        }
        requireCurrent(stamp)
        selected?.let { refreshVisible(it, ns, stamp) }
    }

    private fun clearForwardApprovals() {
        verifiedForwards.clear()
        forwardProjectionRequests.keys.toList().forEach { id ->
            forwardProjectionRequests[id] = ++projectionRevision
        }
    }

    private fun clearForwardSidecar(ns: String, id: String): Boolean {
        return SyncMediaCleanup.remove(getApplication<Application>().cacheDir, ns, id)
    }

    private suspend fun drainGlobalCleanup() {
        val stamp = generation
        // Verified local cleanup is independent of the active login, so a profile switch cannot strand it.
        val plans = syncStore.pendingCleanup()
        requireCurrent(stamp)
        if (plans.isEmpty()) return
        notifications.clear()
        val currentScope = "live|$profile|$deployment|$userId|personal"
        val currentIds = plans.filter { it.scope == currentScope }.map { it.id }.toSet()
        if (currentIds.isNotEmpty()) {
            clearForwardApprovals()
            clearForwardSelection()
            discovery.reset()
            announcement.reset()
            messages.removeAll { it.id in currentIds }
            for (index in messages.indices) if (messages[index].forwarded) messages[index] = Forwarding.hidden(messages[index])
            if (reply?.id in currentIds || reply?.forwarded == true) reply = null
        }
        SyncMediaCleanup(liveDb, getApplication<Application>().cacheDir).drain(plans)
    }

    private suspend fun refreshForwardProjections(conversation: Conversation, stamp: Int) {
        val ns = namespace
        val cached = liveDb.messages().list(ns, conversation.id).filter { it.forwarded && !it.retracted }
        requireCurrent(stamp)
        verifiedForwards.keys.removeAll(cached.map { it.id }.toSet())
        val revisions = cached.associate { it.id to ++projectionRevision }
        forwardProjectionRequests.putAll(revisions)
        refreshVisible(conversation, ns, stamp)
        for (old in cached) {
            val revision = revisions.getValue(old.id)
            if (forwardProjectionRequests[old.id] != revision) continue
            val current =
                try {
                    val result = message(api().request("/messages/${old.id}"), ns, restoreEpoch)
                    requireCurrent(stamp)
                    require(result.id == old.id && result.conversationId == old.conversationId)
                    result
                } catch (e: Exception) {
                    if (e is CancellationException) throw e
                    requireCurrent(stamp)
                    Forwarding.hidden(old)
                }
            if (forwardProjectionRequests[old.id] != revision) continue
            if (current.forwardUnavailable || current.contentUnavailable || current.retracted)
                clearForwardSidecar(ns, current.id)
            synchronization.withLock {
                requireCurrent(stamp)
                val cursor =
                    liveDb.messages().state(ns, conversation.id)?.cursor ?: "$restoreEpoch:0"
                liveDb
                    .messages()
                    .page(
                        ns,
                        conversation.id,
                        restoreEpoch,
                        cursor,
                        listOf(current),
                        userId,
                        foreground && selected?.id == conversation.id,
                    )
                val stored = liveDb.messages().one(ns, old.id)
                requireCurrent(stamp)
                if (forwardProjectionRequests[old.id] == revision && stored?.retracted != true &&
                    (stored == null || BigInteger(current.version) >= BigInteger(stored.version)))
                    verifiedForwards[current.id] = current
            }
        }
    }

    private suspend fun loadMessages(conversation: Conversation, loadDraft: Boolean = false) {
        val ns = namespace
        val stamp = generation
        refreshVisible(conversation, ns, stamp)
        if (loadDraft) {
            val savedDraft = db.messages().state(ns, conversation.id)
            if (namespace != ns || generation != stamp || selected?.id != conversation.id) return
            draft = savedDraft?.draft.orEmpty()
            draftRevision = savedDraft?.revision ?: 0
            val mentions =
                savedDraft?.draftMentions?.let { runCatching { JSONArray(it) }.getOrNull() }
                    ?: JSONArray()
            draftMentionIds = (0 until mentions.length()).map { mentions.getString(it) }
            draftMentionAll = savedDraft?.draftMentionAll ?: false
            draftReady = true
        }
        if (!demo) {
            if (requiresDurableSync() && !syncReady) throw SyncFailure("SYNC_NOT_READY")
            val transport = api()
            val peers =
                transport.request("/conversations/${conversation.id}/members").optJSONArray("items")
                    ?: JSONArray()
            requireCurrent(stamp)
            for (i in 0 until peers.length()) {
                val user = person(peers.getJSONObject(i))
                profiles[user.id] = user
            }
            if (!requiresDurableSync()) syncConversation(conversation, transport, stamp)
            refreshForwardProjections(conversation, stamp)
        }
        if (namespace != ns || generation != stamp || selected?.id != conversation.id) return
        refreshVisible(conversation, ns, stamp)
        if (!demo && selfState.enabled) {
            selfState.refresh(conversation.id)
            if (namespace == ns && generation == stamp) selfObservationRevision++
        }
        val count =
            if (foreground && !selfState.enabled) {
                db.messages().markRead(ns, conversation.id)
                0
            } else {
                db.messages().state(ns, conversation.id)?.unread ?: 0
            }
        if (namespace == ns && generation == stamp && selected?.id == conversation.id)
            unread[conversation.id] = count
    }

    fun updateDraft(value: String) {
        if (!draftReady) return
        draft = value
        val revision = ++draftRevision
        val ns = namespace
        val conversation = selected ?: return
        val database = db
        val epoch = restoreEpoch
        val mentions = JSONArray(draftMentionIds).toString()
        val mentionAll = draftMentionAll
        viewModelScope.launch(start = CoroutineStart.UNDISPATCHED) {
            database
                .messages()
                .saveDraft(ns, conversation.id, epoch, value, revision, mentions, mentionAll)
        }
    }

    fun setDraftMentions(ids: List<String>, all: Boolean) {
        val group = selected ?: return
        if (!draftReady || group.kind != "group" || group.readOnly) return
        require(ids.distinct().size == ids.size && ids.size <= 20 && (!all || ids.isEmpty()))
        require(!all || group.ownerId == userId)
        require(!all || (features["role_admission"] == true && group.roleGeneration.isNotBlank()))
        require(ids.all { id -> members.any { it.id == id && it.state == "active" } })
        draftMentionIds = ids.toList()
        draftMentionAll = all
        updateDraft(draft)
    }

    fun send(text: String, type: String = "text", attachment: String = ""): Job {
        val revision = draftRevision
        val mentionIds = draftMentionIds.toList()
        val mentionAll = draftMentionAll
        return job {
            val stamp = generation
            val conversation = selected ?: return@job
            require(!conversation.readOnly) { "READ_ONLY" }
            require(demo || conversation.sendAdmission.isNotBlank()) { "SEND_ADMISSION_CHANGED" }
            require(text.isNotBlank() || attachment.isNotBlank())
            if (!demo) verifyEpoch()
            require(
                !mentionAll ||
                    (features["role_admission"] == true && conversation.roleGeneration.isNotBlank())
            ) {
                "QUALIFICATION_CHANGED"
            }
            val ns = namespace
            val id = UUID.randomUUID().toString()
            val row =
                MessageRow(
                    "$ns|$id",
                    ns,
                    conversation.id,
                    id,
                    id,
                    userId,
                    type,
                    text,
                    attachment,
                    reply?.id ?: "",
                    if (demo) "demo" else "pending",
                    restoreEpoch = restoreEpoch,
                    membershipGeneration = conversation.membershipGeneration,
                    sendAdmission = conversation.sendAdmission,
                    mentionedUserIds = JSONArray(mentionIds).toString(),
                    mentionAll = mentionAll,
                )
            db.messages().enqueue(row, revision)
            requireCurrent(stamp)
            if (draftRevision == revision) {
                draft = ""
                draftMentionIds = emptyList()
                draftMentionAll = false
            }
            reply = null
            refreshVisible(conversation, ns, stamp)
            if (!demo) submit(row)
        }
    }

    private suspend fun submit(row: MessageRow, explicitForwardCreate: Boolean = false) {
        if (row.type == "forward") {
            submitForward(row, explicitForwardCreate)
            return
        }
        require(Isolation.mayTransmit(row, namespace, demo, restoreEpoch))
        val stamp = generation
        val ns = namespace
        val persisted = liveDb.messages().one(ns, row.id)
        requireCurrent(stamp)
        require(persisted == row && persisted.state == "pending") { "OUTBOX_FROZEN_OR_CHANGED" }
        verifyEpoch()
        requireCurrent(stamp)
        require(features["send_admission"] == true) { "SEND_ADMISSION_UNSUPPORTED" }
        val result =
            try {
                api()
                    .request(
                        "/conversations/${row.conversationId}/" +
                            if (row.mentionAll && features["role_admission"] != true)
                                "message-results"
                            else "messages",
                        "POST",
                        originalMessageBody(row),
                    )
            } catch (e: ApiFailure) {
                requireCurrent(stamp)
                val hasMentions = row.mentionAll || JSONArray(row.mentionedUserIds).length() > 0
                val mentionRejected =
                    hasMentions &&
                        e.code in
                            listOf(
                                "INVALID_MENTIONS",
                                "MENTION_MEMBER_REQUIRED",
                                "MENTION_FORBIDDEN",
                                "MENTION_ALL_FORBIDDEN",
                                "FEATURE_DISABLED",
                            )
                if (
                    e.code in listOf("SEND_ADMISSION_CHANGED", "QUALIFICATION_CHANGED") ||
                        mentionRejected ||
                        (row.mentionAll && features["role_admission"] != true && e.status == 404)
                ) {
                    liveDb.messages().freeze(ns, row.id)
                    selected?.let { refreshVisible(it, ns, stamp) }
                }
                throw e
            }
        requireCurrent(stamp)
        val committed = result.optJSONObject("message") ?: result
        SyncProtocol.requireCommittedMessage(committed, row.clientMsgId, row.conversationId, userId)
        val accepted = message(committed, ns, restoreEpoch)
        acceptOriginalResult(row, accepted, stamp)
        selected?.let { loadMessages(it) }
    }

    private suspend fun acceptOriginalResult(original: MessageRow, accepted: MessageRow, stamp: Int) =
        synchronization.withLock {
            requireCurrent(stamp)
            val ns = original.namespace
            require(ns == namespace && accepted.namespace == ns && accepted.senderId == userId &&
                accepted.conversationId == original.conversationId && accepted.clientMsgId == original.clientMsgId) {
                "OUTBOX_RESULT_INVALID"
            }
            liveDb.withTransaction {
                requireCurrent(stamp)
                val cursor = liveDb.messages().state(ns, original.conversationId)?.cursor ?: "$restoreEpoch:0"
                // The exact request result is proof; a history page with the same client ID is not.
                liveDb.messages().page(ns, original.conversationId, restoreEpoch, cursor, listOf(accepted), userId, true)
                val pending = liveDb.messages().one(ns, original.id)
                if (pending != null && pending.state in setOf("pending", "frozen") &&
                    pending.senderId == userId && pending.conversationId == original.conversationId &&
                    SyncWire.same(originalMessageBody(pending), originalMessageBody(original)))
                    liveDb.messages().remove(ns, original.id)
                requireCurrent(stamp)
            }
            if (accepted.retracted) drainGlobalCleanup()
            requireCurrent(stamp)
        }

    fun clearForwardSelection() {
        forwardSource = null
        forwardNeedsReselect = false
    }

    fun prepareForward(seen: MessageRow) = job {
        require(!demo && seen.namespace == namespace)
        val stamp = generation
        val ns = namespace
        clearForwardSelection()
        verifyEpoch()
        require(features["forwarding"] == true && features["send_admission"] == true) {
            "FORWARD_PROTOCOL_REQUIRED"
        }
        val current =
            try {
                message(api().request("/messages/${seen.id}"), ns, restoreEpoch)
            } catch (e: ApiFailure) {
                requireCurrent(stamp)
                if (e.status in listOf(403, 404)) {
                    synchronization.withLock {
                        requireCurrent(stamp)
                        val cursor =
                            liveDb.messages().state(ns, seen.conversationId)?.cursor
                                ?: "$restoreEpoch:0"
                        liveDb
                            .messages()
                            .page(
                                ns,
                                seen.conversationId,
                                restoreEpoch,
                                cursor,
                                listOf(
                                    seen.copy(
                                        text = "",
                                        attachmentId = "",
                                        attachmentURL = "",
                                        forwardAdmission = "",
                                        contentUnavailable = true,
                                    )
                                ),
                                userId,
                                true,
                            )
                    }
                    discovery.invalidate(seen.id)
                    selected?.let { refreshVisible(it, ns, stamp) }
                }
                throw e
            }
        requireCurrent(stamp)
        require(current.id == seen.id && current.conversationId == seen.conversationId)
        synchronization.withLock {
            requireCurrent(stamp)
            val cursor =
                liveDb.messages().state(ns, seen.conversationId)?.cursor ?: "$restoreEpoch:0"
            liveDb
                .messages()
                .page(ns, seen.conversationId, restoreEpoch, cursor, listOf(current), userId, true)
        }
        selected?.let { refreshVisible(it, ns, stamp) }
        require(
            !current.retracted &&
                !current.contentUnavailable &&
                !current.forwardUnavailable &&
                current.forwardAdmission.isNotBlank()
        ) {
            "FORWARD_SOURCE_UNAVAILABLE"
        }
        forwardSource = current
        forwardNeedsReselect = current.contentVersion != seen.contentVersion
    }

    fun acceptUpdatedForwardSource() {
        // This explicit selection is a new intent; it never mutates an existing Outbox body.
        forwardNeedsReselect = false
    }

    fun sendForward(source: MessageRow, target: Conversation) = job {
        require(
            !demo &&
                source.namespace == namespace &&
                source == forwardSource &&
                !forwardNeedsReselect
        )
        val stamp = generation
        verifyEpoch()
        requireCurrent(stamp)
        require(features["forwarding"] == true && features["send_admission"] == true) {
            "FORWARD_PROTOCOL_REQUIRED"
        }
        val destination = conversation(api().request("/conversations/${target.id}"), target.title)
        requireCurrent(stamp)
        require(
            destination.id == target.id &&
                !destination.readOnly &&
                destination.sendAdmission.isNotBlank()
        ) {
            "SEND_ADMISSION_CHANGED"
        }
        val ns = namespace
        val id = UUID.randomUUID().toString()
        val body =
            JSONObject()
                .put("clientMsgId", id)
                .put("type", "forward")
                .put("forwardOfId", source.id)
                .put("forwardSourceVersion", source.contentVersion)
                .put("forwardAdmission", source.forwardAdmission)
                .put("restoreEpoch", restoreEpoch)
                .put("sendAdmission", destination.sendAdmission)
        val row =
            MessageRow(
                "$ns|$id",
                ns,
                destination.id,
                id,
                id,
                userId,
                type = "forward",
                text = "",
                state = "pending",
                restoreEpoch = restoreEpoch,
                membershipGeneration = destination.membershipGeneration,
                sendAdmission = destination.sendAdmission,
                forwardPayload = body.toString(),
            )
        liveDb.messages().enqueue(row)
        requireCurrent(stamp)
        clearForwardSelection()
        if (conversations.none { it.id == destination.id }) conversations.add(destination)
        selected?.let { refreshVisible(it, ns, stamp) }
        submitForward(row, explicitCreate = true)
    }

    private suspend fun submitForward(row: MessageRow, explicitCreate: Boolean) {
        require(Isolation.mayTransmit(row, namespace, demo, restoreEpoch))
        val ns = namespace
        val stamp = generation
        require(liveDb.messages().one(ns, row.id) == row && row.state == "pending") {
            "OUTBOX_FROZEN_OR_CHANGED"
        }
        requireCurrent(stamp)
        val body =
            try {
                Forwarding.payload(row)
            } catch (e: Exception) {
                liveDb.messages().freeze(ns, row.id)
                throw e
            }
        verifyEpoch()
        requireCurrent(stamp)
        require(features["send_admission"] == true) { "SEND_ADMISSION_UNSUPPORTED" }
        val transport = api()
        val result =
            try {
                try {
                    transport.request(
                        "/conversations/${row.conversationId}/message-results",
                        "POST",
                        body,
                    )
                } catch (e: ApiFailure) {
                    requireCurrent(stamp)
                    if (e.status != 404 || e.code != "MESSAGE_RESULT_NOT_FOUND") throw e
                    if (!explicitCreate) {
                        if (!missingForwardResults.contains(row.id))
                            missingForwardResults.add(row.id)
                        selected?.let { refreshVisible(it, ns, stamp) }
                        info = Strings.text(language, "forwardUnknown")
                        return
                    }
                    require(features["forwarding"] == true) { "FORWARD_PROTOCOL_REQUIRED" }
                    require(liveDb.messages().one(ns, row.id) == row) { "OUTBOX_FROZEN_OR_CHANGED" }
                    requireCurrent(stamp)
                    transport.request("/conversations/${row.conversationId}/messages", "POST", body)
                }
            } catch (e: ApiFailure) {
                requireCurrent(stamp)
                if (e.code in Forwarding.permanentErrors) {
                    liveDb.messages().freeze(ns, row.id)
                    missingForwardResults.remove(row.id)
                    selected?.let { refreshVisible(it, ns, stamp) }
                }
                throw e
            }
        requireCurrent(stamp)
        val committed = result.optJSONObject("message") ?: result
        SyncProtocol.requireCommittedMessage(committed, row.clientMsgId, row.conversationId, userId)
        val accepted = message(committed, ns, restoreEpoch)
        acceptOriginalResult(row, accepted, stamp)
        missingForwardResults.remove(row.id)
        selected?.let { loadMessages(it) }
        info = Strings.text(language, "saved")
    }

    fun retryOriginalForward(row: MessageRow) = job {
        require(row.id in missingForwardResults) { "FORWARD_CHECK_REQUIRED" }
        submitForward(row, explicitCreate = true)
    }

    fun retry(row: MessageRow) = job { submit(row) }

    fun action(row: MessageRow, action: String, value: String = "") = job {
        require(row.namespace == namespace)
        require((!row.contentUnavailable && !row.forwardUnavailable) || action == "retract") {
            "CONTENT_UNAVAILABLE"
        }
        require(!row.forwarded || action != "edit") { "INVALID_FORWARD" }
        if (demo) {
            val updated =
                when (action) {
                    "edit" -> row.copy(text = value, edited = true)
                    "retract" -> row.copy(text = "", retracted = true)
                    "pin" -> row.copy(pinned = !row.pinned)
                    "favorite" -> row.copy(favorite = !row.favorite)
                    else -> row.copy(reaction = if (row.reaction.isEmpty()) "👍" else "")
                }
            db.messages().put(updated)
            loadMessages(selected ?: return@job)
            return@job
        }
        val active =
            when (action) {
                "pin" -> !row.pinned
                "favorite" -> !row.favorite
                else -> row.reaction.isEmpty()
            }
        val method =
            if (action == "edit") "PATCH"
            else if (action == "retract" || !active) "DELETE"
            else if (action == "pin" || action == "favorite") "PUT" else "POST"
        val tail =
            when (action) {
                "pin" -> "/pin"
                "favorite" -> "/favorite"
                "reaction" -> if (active) "/reactions" else "/reactions/%F0%9F%91%8D"
                else -> ""
            }
        val payload =
            when (action) {
                "edit" ->
                    JSONObject()
                        .put("text", value)
                        .put("expectedVersion", row.version)
                        .put("requestId", UUID.randomUUID().toString())
                "reaction" -> if (active) JSONObject().put("emoji", "👍") else null
                else -> null
            }
        val stamp = generation
        val ns = namespace
        val response = api().request("/messages/${row.id}$tail", method, payload)
        requireCurrent(stamp)
        val body = response.optJSONObject("message") ?: response
        SyncProtocol.requireCommittedMessage(body, row.clientMsgId, row.conversationId, row.senderId)
        val current = message(body, ns, restoreEpoch)
        require(current.id == row.id)
        synchronization.withLock {
            requireCurrent(stamp)
            val cursor = liveDb.messages().state(ns, row.conversationId)?.cursor ?: "$restoreEpoch:0"
            liveDb.messages().page(ns, row.conversationId, restoreEpoch, cursor, listOf(current), userId, foreground && selected?.id == row.conversationId)
        }
        if (current.retracted) drainGlobalCleanup()
        requireCurrent(stamp)
        selected?.let { loadMessages(it) }
    }

    fun findPeople(query: String) = job {
        if (demo) return@job
        peopleQuery = query
        val stamp = generation
        val array =
            api()
                .request("/users?q=" + java.net.URLEncoder.encode(query, "UTF-8"))
                .optJSONArray("items") ?: JSONArray()
        requireCurrent(stamp)
        people.clear()
        people.addAll((0 until array.length()).map { person(array.getJSONObject(it)) })
    }

    fun addContact(id: String) = job {
        if (!demo) {
            val ns = namespace
            val purpose = "contact-" + id
            val op =
                liveDb.messages().operation(ns, purpose)
                    ?: LocalOperation(
                            ns + "|" + purpose,
                            ns,
                            purpose,
                            UUID.randomUUID().toString(),
                            JSONObject().put("userId", id).toString(),
                            restoreEpoch = restoreEpoch,
                        )
                        .also { liveDb.messages().putOperation(it) }
            api()
                .request(
                    "/contacts/requests",
                    "POST",
                    JSONObject(op.payload).put("requestId", op.requestId),
                )
            liveDb.messages().clearOperation(ns, purpose, op.requestId)
        }
        info = Strings.text(language, "requestSent")
    }

    fun resolveRequest(id: String, accept: Boolean) = job {
        val stamp = generation
        api()
            .request(
                "/contacts/requests/$id/" + if (accept) "accept" else "reject",
                "POST",
                JSONObject(),
            )
        requireCurrent(stamp)
        refreshContacts()
    }

    fun relationship(id: String, action: String) = job {
        require(!demo)
        val stamp = generation
        when (action) {
            "remove" -> api().request("/contacts/$id", "DELETE")
            "block" -> api().request("/blocks/$id", "PUT")
            "unblock" -> api().request("/blocks/$id", "DELETE")
            "reopen" -> {
                val ns = namespace
                val purpose = "reopen-$id"
                val op =
                    liveDb.messages().operation(ns, purpose)
                        ?: LocalOperation(
                                "$ns|$purpose",
                                ns,
                                purpose,
                                UUID.randomUUID().toString(),
                                "{}",
                                restoreEpoch = restoreEpoch,
                            )
                            .also { liveDb.messages().putOperation(it) }
                requireCurrent(stamp)
                api()
                    .request(
                        "/contacts/$id/reopen",
                        "POST",
                        JSONObject().put("requestId", op.requestId),
                    )
                requireCurrent(stamp)
                liveDb.messages().clearOperation(ns, purpose, op.requestId)
            }
        }
        requireCurrent(stamp)
        refreshContacts()
        refreshRelationship(id, stamp)
        info = Strings.text(language, "saved")
    }

    private suspend fun refreshRelationship(id: String, stamp: Int) {
        val value = api().request("/contacts/$id/relationship")
        requireCurrent(stamp)
        require(value.getString("peerId") == id)
        relationships[id] =
            Relationship(
                value.getBoolean("blockedByMe"),
                value.getBoolean("friend"),
                value.getString("inboundGate"),
            )
    }

    fun loadRelationship(id: String) = job {
        if (!demo) refreshRelationship(id, generation)
    }

    fun create(title: String, members: List<String>, group: Boolean) = job {
        val stamp = generation
        val result =
            if (demo) null
            else
                api()
                    .request(
                        "/conversations",
                        "POST",
                        JSONObject()
                            .put("kind", if (group) "group" else "direct")
                            .put("title", title)
                            .put("memberIds", JSONArray(members))
                            .put("requestId", UUID.randomUUID().toString()),
                    )
        requireCurrent(stamp)
        val id = result?.getString("id") ?: UUID.randomUUID().toString()
        val conversation =
            Conversation(
                id,
                title,
                if (group) "group" else "direct",
                result?.optString("ownerId").orEmpty(),
            )
        if (!demo) reload() else conversations.add(0, conversation)
        selectConversation(conversations.find { it.id == id } ?: conversation)
        loadMessages(selected!!, loadDraft = true)
    }

    fun loadMembers() = job {
        val conversation = selected ?: return@job
        val stamp = generation
        val items =
            if (demo) JSONArray()
            else
                api().request("/conversations/${conversation.id}/members").optJSONArray("items")
                    ?: JSONArray()
        requireCurrent(stamp)
        members.clear()
        members.addAll((0 until items.length()).map { person(items.getJSONObject(it)) })
    }

    fun groupChange(action: String, value: String = "", member: Person? = null) = job {
        val conversation = selected ?: return@job
        require(!conversation.readOnly) { "READ_ONLY" }
        val stamp = generation
        when (action) {
            "rename" ->
                api()
                    .request(
                        "/conversations/${conversation.id}",
                        "PATCH",
                        JSONObject().put("title", value),
                    )
            "transfer" ->
                api()
                    .request(
                        "/conversations/${conversation.id}",
                        "PATCH",
                        JSONObject().put("ownerId", member!!.id),
                    )
            "add" ->
                api()
                    .request(
                        "/conversations/${conversation.id}/members",
                        "POST",
                        JSONObject().put("userId", value),
                    )
            "remove",
            "leave" -> {
                val who = member ?: members.find { it.id == userId } ?: error("MEMBERSHIP_REQUIRED")
                require(who.membershipGeneration.isNotBlank())
                api()
                    .request(
                        "/conversations/${conversation.id}/members/${who.id}?membershipGeneration=${who.membershipGeneration}",
                        "DELETE",
                    )
            }
        }
        requireCurrent(stamp)
        if (action == "leave") closeConversation()
        reload()
        if (selected != null) {
            val items =
                api().request("/conversations/${conversation.id}/members").getJSONArray("items")
            requireCurrent(stamp)
            members.clear()
            members.addAll((0 until items.length()).map { person(items.getJSONObject(it)) })
        }
    }

    fun upload(context: Context, uri: Uri) = job {
        val conversation = selected ?: return@job
        require(!conversation.readOnly) { "READ_ONLY" }
        require(demo || conversation.sendAdmission.isNotBlank()) { "SEND_ADMISSION_CHANGED" }
        val stamp = generation
        val (name, mime, bytes) = readAsset(context, uri, 64 * 1024 * 1024)
        requireCurrent(stamp)
        val attachment =
            if (demo) "demo-" + UUID.randomUUID()
            else api().upload(name, mime, bytes).getString("id")
        requireCurrent(stamp)
        if (!demo) verifyEpoch()
        val ns = namespace
        val id = UUID.randomUUID().toString()
        val kind =
            when {
                mime.startsWith("image/") -> "image"
                mime.startsWith("audio/") -> "audio"
                mime.startsWith("video/") -> "video"
                else -> "file"
            }
        val row =
            MessageRow(
                "$ns|$id",
                ns,
                conversation.id,
                id,
                id,
                userId,
                kind,
                name,
                attachment,
                reply?.id ?: "",
                if (demo) "demo" else "pending",
                restoreEpoch = restoreEpoch,
                membershipGeneration = conversation.membershipGeneration,
                sendAdmission = conversation.sendAdmission,
            )
        db.messages().enqueue(row)
        requireCurrent(stamp)
        reply = null
        refreshVisible(conversation, ns, stamp)
        if (!demo) submit(row)
    }

    fun download(row: MessageRow, ready: (File, String) -> Unit) = job {
        require(row.namespace == namespace)
        require(!demo && !row.retracted && !row.contentUnavailable && !row.forwardUnavailable)
        val stamp = generation
        verifyEpoch()
        requireCurrent(stamp)
        val ns = namespace
        val storageGuard = syncStore.projectionGuard(ns)
        val revision = ++projectionRevision
        if (row.forwarded) {
            forwardProjectionRequests[row.id] = revision
            verifiedForwards.remove(row.id)
            clearForwardSidecar(namespace, row.id)
            selected?.let { refreshVisible(it, namespace, stamp) }
        }
        val current =
            if (row.forwarded) {
                val projected =
                    try {
                        message(api().request("/messages/${row.id}"), namespace, restoreEpoch)
                    } catch (e: Exception) {
                        if (e is CancellationException) throw e
                        requireCurrent(stamp)
                        clearDerivedForwards(listOf(row), stamp)
                        throw e
                    }
                projected.also {
                    requireCurrent(stamp)
                    require(it.id == row.id && it.conversationId == row.conversationId)
                    require(forwardProjectionRequests[row.id] == revision) {
                        "FORWARD_QUALIFICATION_UNAVAILABLE"
                    }
                    synchronization.withLock {
                        requireCurrent(stamp)
                        val cursor =
                            liveDb.messages().state(namespace, row.conversationId)?.cursor
                                ?: "$restoreEpoch:0"
                        liveDb
                            .messages()
                            .page(
                                namespace,
                                row.conversationId,
                                restoreEpoch,
                                cursor,
                                listOf(it),
                                userId,
                                true,
                            )
                    }
                    require(forwardProjectionRequests[row.id] == revision) {
                        "FORWARD_QUALIFICATION_UNAVAILABLE"
                    }
                    verifiedForwards[it.id] = it
                    selected?.let { selected -> refreshVisible(selected, namespace, stamp) }
                }
            } else row
        require(!current.retracted && !current.contentUnavailable && !current.forwardUnavailable)
        val path =
            if (current.forwarded) Forwarding.attachmentPath(url, current.id, current.attachmentURL)
            else {
                require(current.attachmentId.isNotBlank())
                "/media/" + current.attachmentId
            }
        val (bytes, mime) =
            try {
                api().download(path)
            } catch (e: Exception) {
                if (e is CancellationException) throw e
                requireCurrent(stamp)
                if (row.forwarded) clearDerivedForwards(listOf(current), stamp)
                throw e
            }
        requireCurrent(stamp)
        val dir =
            File(
                    getApplication<Application>().cacheDir,
                    "Attachments/" + Isolation.digest(namespace),
                )
                .apply { mkdirs() }
        val file = File(dir, Isolation.digest(row.id) + ".bin")
        file.outputStream().use { it.write(bytes) }
        val stored = liveDb.messages().one(ns, row.id)
        if (generation != stamp || namespace != ns || syncStore.projectionGuard(ns) != storageGuard || stored?.retracted == true) {
            file.delete()
            throw CancellationException("SYNC_LOCAL_RESTRICTION_CHANGED")
        }
        ready(file, mime)
    }

    suspend fun image(path: String): ByteArray? {
        if (token.isBlank() || demo) return null
        val stamp = generation
        val result = api().download(path, 2 * 1024 * 1024).first
        requireCurrent(stamp)
        return result
    }

    fun updateProfile(name: String) = job {
        val stamp = generation
        val user = api().request("/me", "PATCH", JSONObject().put("displayName", name))
        requireCurrent(stamp)
        me = person(user)
        info = Strings.text(language, "saved")
    }

    fun publishAvatar(context: Context, uri: Uri? = null) = job {
        require(!demo && features["avatars"] == true)
        val ns = namespace
        val stamp = generation
        var op = liveDb.messages().operation(ns, "avatar")
        if (op == null) {
            require(uri != null)
            val (name, mime, bytes) = readAsset(context, uri, 2 * 1024 * 1024)
            require(mime == "image/jpeg" || mime == "image/png") { "AVATAR_FORMAT" }
            requireCurrent(stamp)
            val requestId = UUID.randomUUID().toString()
            val dir =
                File(
                        getApplication<Application>().noBackupFilesDir,
                        "AvatarSources/" + Isolation.digest(ns),
                    )
                    .apply { mkdirs() }
            val file = File(dir, requestId)
            java.io.FileOutputStream(file).use {
                it.write(bytes)
                it.fd.sync()
            }
            val payload =
                JSONObject()
                    .put("name", name)
                    .put("mime", mime)
                    .put("expectedVersion", me?.profileVersion ?: "1")
            op =
                LocalOperation(
                    ns + "|avatar",
                    ns,
                    "avatar",
                    requestId,
                    payload.toString(),
                    file.absolutePath,
                    restoreEpoch,
                )
            liveDb.messages().putOperation(op)
            avatarPending = true
        }
        require(op.restoreEpoch == restoreEpoch)
        val payload = JSONObject(op.payload)
        val source = File(op.filePath)
        val result =
            api()
                .upload(
                    payload.getString("name"),
                    payload.getString("mime"),
                    source.readBytes(),
                    "/me/avatar",
                    mapOf(
                        "requestId" to op.requestId,
                        "expectedVersion" to payload.getString("expectedVersion"),
                    ),
                )
        requireCurrent(stamp)
        if (result.optString("status") in listOf("applied", "cancelled", "superseded")) {
            val user = api().request("/me")
            requireCurrent(stamp)
            me = person(user)
            liveDb.messages().clearOperation(ns, "avatar", op.requestId)
            avatarPending = false
            source.delete()
            info = Strings.text(language, "saved")
        } else info = Strings.text(language, "pending")
    }

    fun inspectAvatar() = job {
        val ns = namespace
        val stamp = generation
        val op = liveDb.messages().operation(ns, "avatar") ?: return@job
        val result = api().request("/me/avatar/operations/" + op.requestId)
        requireCurrent(stamp)
        if (result.optString("status") in listOf("applied", "cancelled", "superseded")) {
            val user = api().request("/me")
            requireCurrent(stamp)
            me = person(user)
            liveDb.messages().clearOperation(ns, "avatar", op.requestId)
            avatarPending = false
            File(op.filePath).delete()
        }
    }

    fun loadReports() = job {
        val stamp = generation
        val a = api().request("/reports").optJSONArray("items") ?: JSONArray()
        requireCurrent(stamp)
        reports.clear()
        for (i in 0 until a.length()) {
            val r = a.getJSONObject(i)
            reports.add(
                ReportRecord(
                    r.getString("id"),
                    r.getString("targetId"),
                    r.getString("state"),
                    r.getString("reason"),
                )
            )
        }
    }

    fun report(type: String, target: String, reason: String) = job {
        require(!demo && features["reports"] == true)
        require(reason.isNotBlank() && reason.codePointCount(0, reason.length) <= 1000)
        val ns = namespace
        val stamp = generation
        var op = liveDb.messages().operation(ns, "report")
        val payload =
            JSONObject().put("targetType", type).put("targetId", target).put("reason", reason)
        if (op == null) {
            op =
                LocalOperation(
                    ns + "|report",
                    ns,
                    "report",
                    UUID.randomUUID().toString(),
                    payload.toString(),
                    restoreEpoch = restoreEpoch,
                )
            liveDb.messages().putOperation(op)
            pendingReport = op
        } else
            require(JSONObject(op.payload).toString() == payload.toString()) {
                "ORIGINAL_REPORT_PENDING"
            }
        api().request("/reports", "POST", JSONObject(op.payload).put("requestId", op.requestId))
        requireCurrent(stamp)
        liveDb.messages().clearOperation(ns, "report", op.requestId)
        pendingReport = null
        info = Strings.text(language, "saved")
    }

    fun codes(password: String) = job {
        val stamp = generation
        if (!demo) {
            val a =
                api()
                    .request(
                        "/auth/recovery-codes",
                        "POST",
                        JSONObject()
                            .put("password", password)
                            .put("requestId", UUID.randomUUID().toString()),
                    )
                    .optJSONArray("codes") ?: JSONArray()
            requireCurrent(stamp)
            recoveryCodes = (0 until a.length()).map { a.getString(it) }
        }
    }

    fun loadDevices() = job {
        val stamp = generation
        val a = api().request("/auth/sessions").optJSONArray("items") ?: JSONArray()
        requireCurrent(stamp)
        devices.clear()
        for (i in 0 until a.length()) {
            val d = a.getJSONObject(i)
            devices.add(
                DeviceSession(
                    d.optString("deviceId", d.optString("id")),
                    d.optString("deviceName", d.optString("name")),
                )
            )
        }
    }

    fun revokeDevice(id: String) = job {
        val stamp = generation
        val current = sessions.session
        val ownDevice = current?.deviceId ?: api().request("/auth/context").getString("deviceId")
        requireCurrent(stamp)
        api().request("/auth/sessions/$id", "DELETE")
        requireCurrent(stamp)
        if (ownDevice == id) {
            if (current != null) sessions.clearKnown(current) else sessions.clearLegacy(sessionBinding(), token)
            clearIdentity()
        } else devices.removeAll { it.id == id }
    }

    fun logout() = job {
        if (demo) {
            clearIdentity()
            demo = false
            loadAuthenticationState()
            return@job
        }
        val stamp = generation
        val current = sessions.session
        if (!demo && token.isNotBlank()) api().request("/auth/logout", "POST", JSONObject())
        requireCurrent(stamp)
        if (current != null) sessions.clearKnown(current) else sessions.clearLegacy(sessionBinding(), token)
        clearIdentity()
        demo = false
        loadAuthenticationState()
    }

    override fun onCleared() {
        sessions.forgetMemory()
        liveDb.close()
        demoDb.close()
        super.onCleared()
    }
}

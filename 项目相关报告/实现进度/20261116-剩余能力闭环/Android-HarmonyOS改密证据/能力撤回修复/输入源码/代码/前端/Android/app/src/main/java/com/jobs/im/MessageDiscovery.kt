package com.jobs.im

import androidx.compose.runtime.*
import androidx.compose.runtime.snapshots.Snapshot
import java.math.BigInteger
import java.net.URLEncoder
import kotlinx.coroutines.*

class MessageDiscovery(
    private val scope: CoroutineScope,
    private val context: () -> String,
    private val transport: () -> Api,
    private val namespace: () -> String,
    private val epoch: () -> String,
    private val verify: suspend () -> Unit,
    private val clearDerived: suspend (List<MessageRow>) -> Unit,
) {
    var kind by mutableStateOf("search")
        private set

    var query by mutableStateOf("")
        private set

    var conversationId by mutableStateOf("")
        private set

    var cursor by mutableStateOf("")
        private set

    var hasMore by mutableStateOf(false)
        private set

    var loading by mutableStateOf(false)
        private set

    var problem by mutableStateOf("")
        private set

    val items = mutableStateListOf<MessageRow>()
    private var revision = 0
    private var request: Job? = null

    fun reset() {
        request?.cancel()
        revision++
        items.clear()
        cursor = ""
        hasMore = false
        loading = false
        problem = ""
    }

    fun start(kind: String, query: String = "", conversationId: String = ""): Job {
        reset()
        require(kind == "search" || kind == "favorites")
        this.kind = kind
        this.query = query
        this.conversationId = conversationId
        if (kind == "search" && query.isBlank()) return scope.launch {}
        return load(false)
    }

    fun reload() = start(kind, query, conversationId)

    fun more(): Job = if (hasMore && !loading) load(true) else scope.launch {}

    fun invalidate(id: String) {
        val index = items.indexOfFirst { it.id == id }
        if (index >= 0)
            items[index] =
                items[index].copy(
                    text = "",
                    attachmentId = "",
                    attachmentURL = "",
                    forwardAdmission = "",
                    contentUnavailable = true,
                )
        problem = "sourceUnavailable"
    }

    fun invalidateConversation(conversation: String, sourceId: String) {
        val source = items.firstOrNull { it.id == sourceId }
        items.removeAll { it.conversationId == conversation }
        if (source != null)
            items.add(
                source.copy(
                    text = "",
                    attachmentId = "",
                    attachmentURL = "",
                    forwardAdmission = "",
                    contentUnavailable = true,
                )
            )
        problem = "sourceUnavailable"
    }

    private fun load(append: Boolean): Job {
        val stamp = context()
        val revision = this.revision
        val api = transport()
        val ns = namespace()
        val restoreEpoch = epoch()
        val originalCursor = if (append) cursor else ""
        val lookupKind = kind
        val originalQuery = query
        val filter = conversationId
        loading = true
        problem = ""
        return scope
            .launch {
                try {
                    verify()
                    if (
                        !isActive ||
                            context() != stamp ||
                            this@MessageDiscovery.revision != revision
                    )
                        return@launch
                    if (
                        lookupKind == "search" &&
                            (originalQuery.isBlank() ||
                                originalQuery.toByteArray(Charsets.UTF_8).size !in 1..256)
                    ) {
                        problem = "searchLimit"
                        return@launch
                    }
                    val parameters = mutableListOf("limit=50")
                    fun encode(value: String) = URLEncoder.encode(value, "UTF-8")
                    if (lookupKind == "search") parameters.add("q=" + encode(originalQuery))
                    if (filter.isNotEmpty()) parameters.add("conversationId=" + encode(filter))
                    if (originalCursor.isNotEmpty())
                        parameters.add("cursor=" + encode(originalCursor))
                    val result = api.request("/" + lookupKind + "?" + parameters.joinToString("&"))
                    if (
                        !isActive ||
                            context() != stamp ||
                            this@MessageDiscovery.revision != revision
                    )
                        return@launch
                    verify()
                    if (
                        !isActive ||
                            context() != stamp ||
                            this@MessageDiscovery.revision != revision
                    )
                        return@launch
                    val array = result.getJSONArray("items")
                    val fresh =
                        (0 until array.length()).map {
                            message(array.getJSONObject(it), ns, restoreEpoch)
                        }
                    val next = result.getString("cursor")
                    val more = result.getBoolean("hasMore")
                    require(!more || (next.isNotEmpty() && next != originalCursor))
                    val merged =
                        ((if (append) items.toList() else emptyList()) + fresh)
                            .groupBy { it.id }
                            .values
                            .map { versions ->
                                versions.reduce { old, row ->
                                    if (BigInteger(old.version) > BigInteger(row.version)) old
                                    else row
                                }
                            }
                    Snapshot.withMutableSnapshot {
                        items.clear()
                        items.addAll(merged)
                        cursor = next
                        hasMore = more
                    }
                } catch (e: Exception) {
                    if (
                        e !is CancellationException &&
                            context() == stamp &&
                            this@MessageDiscovery.revision == revision
                    ) {
                        if (e is ApiFailure && e.status in listOf(401, 403, 404, 409)) {
                            clearDerived(items.toList())
                            if (context() != stamp || this@MessageDiscovery.revision != revision)
                                return@launch
                            items.clear()
                            cursor = ""
                            hasMore = false
                        }
                        problem = "failure"
                    }
                } finally {
                    if (context() == stamp && this@MessageDiscovery.revision == revision)
                        loading = false
                }
            }
            .also { request = it }
    }
}

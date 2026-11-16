package com.jobs.im

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import java.time.Instant
import org.json.JSONObject

@Composable
fun SelfStateActions(model: ChatModel, conversation: Conversation? = null) {
    if (!model.selfState.enabled || model.demo) return
    fun t(key: String) = Strings.text(model.language, key)
    var open by remember(conversation?.id) { mutableStateOf(false) }
    TextButton(onClick = { open = true; model.refreshSelf(conversation?.id) }) { Text(t("selfState")) }
    if (!open) return
    val state = model.selfState
    val view = conversation?.let { state.views[it.id] }
    val read = conversation?.let { state.reads[it.id] }
    AlertDialog(
        onDismissRequest = { open = false },
        title = { Text(t("selfState")) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(t("selfStateHelp"))
                if (conversation != null) {
                    if (view == null && read == null) Text(t("emptyHelp"))
                    read?.let {
                        val point = it.getJSONObject("readPoint")
                        val applicable = it.opt("writable") == true && it.opt("membershipGeneration") == conversation.membershipGeneration &&
                            it.opt("continuityStatus") !in setOf("needs_current_observation", "membership_unavailable")
                        Text(t(if (applicable) "readProgress" else "readRetained") + " ${point.getString("epoch")}:${point.getString("sequence")}")
                        Text(if (it.opt("countStatus") == "exact_current_source")
                            t("serverUnread") + ": " + it.getString("unreadCount") else t("countNotComputed"))
                        Text(t("localUnread") + ": " + (model.unread[conversation.id] ?: 0))
                    }
                    view?.let { value ->
                        fun patch(changes: JSONObject) { model.changeView(conversation.id, changes) }
                        OutlinedButton(onClick = { patch(JSONObject().put("manualUnread", !value.getBoolean("manualUnread"))) }, enabled = !model.busy) {
                            Text(t(if (value.getBoolean("manualUnread")) "clearManualUnread" else "markManualUnread"))
                        }
                        OutlinedButton(onClick = { patch(JSONObject().put("pinnedRank", if (value.isNull("pinnedRank")) 0 else JSONObject.NULL)) }, enabled = !model.busy) {
                            Text(t(if (value.isNull("pinnedRank")) "pinChat" else "unpinChat"))
                        }
                        OutlinedButton(onClick = { patch(JSONObject().put("mutedUntil", if (state.isMuted(conversation.id)) JSONObject.NULL else Instant.now().plusSeconds(86400).toString())) }, enabled = !model.busy) {
                            Text(t(if (state.isMuted(conversation.id)) "unmuteChat" else "muteChat"))
                        }
                        OutlinedButton(onClick = { patch(JSONObject().put("archived", !value.getBoolean("archived"))) }, enabled = !model.busy) {
                            Text(t(if (value.getBoolean("archived")) "unarchiveChat" else "archiveChat"))
                        }
                        OutlinedButton(onClick = { patch(JSONObject().put("hidden", !value.getBoolean("hidden"))) }, enabled = !model.busy) {
                            Text(t(if (value.getBoolean("hidden")) "showChat" else "hideChat"))
                        }
                    }
                    if (state.draftEnabled) {
                        HorizontalDivider()
                        Text(t("cloudDraft"), style = MaterialTheme.typography.titleMedium)
                        Text(t("cloudDraftHelp"))
                        val cloud = state.drafts[conversation.id]
                        if (cloud?.opt("replyState") == "unavailable") Text(t("draftReplyUnavailable"))
                        cloud?.opt("ownedText")?.takeIf { it is String }?.let { Text((it as String).take(160)) }
                        OutlinedButton(onClick = { model.saveCloudDraft() }, enabled = !model.busy && model.draftReady) { Text(t("saveCloudDraft")) }
                        OutlinedButton(onClick = { model.loadCloudDraft() }, enabled = !model.busy && cloud?.opt("ownedText") is String) { Text(t("loadCloudDraft")) }
                        OutlinedButton(onClick = { model.saveCloudDraft(true) }, enabled = !model.busy) { Text(t("clearCloudDraft")) }
                        if (cloud?.optJSONObject("recovery") != null) OutlinedButton(onClick = { model.loadCloudDraft(true) }, enabled = !model.busy) { Text(t("recoverCloudDraft")) }
                    }
                }
                if (state.pending.isEmpty()) Text(t("noPendingState"))
                state.pending.filter { conversation == null || it.conversation == conversation.id }.forEach { intent ->
                    HorizontalDivider()
                    Text(t("pendingSelfState") + ": " + t(when (intent.kind) { "preferences" -> "font"; "read_state" -> "readProgress"; "draft" -> "cloudDraft"; else -> "conversationDisplay" }))
                    if (intent.status.startsWith("rejected:")) {
                        TextButton(onClick = { model.archiveRejectedSelf(intent) }, enabled = !model.busy) { Text(t("archiveRejectedState")) }
                    } else {
                        TextButton(onClick = { model.recoverSelf(intent) }, enabled = !model.busy) { Text(t("checkStateResult")) }
                        OutlinedButton(onClick = { model.recoverSelf(intent, true) }, enabled = !model.busy) { Text(t("retryOriginalState")) }
                    }
                }
                TextButton(onClick = { model.refreshSelf(conversation?.id) }, enabled = !model.busy) { Text(t("refresh")) }
            }
        },
        confirmButton = { TextButton(onClick = { open = false }) { Text(t("close")) } },
    )
}

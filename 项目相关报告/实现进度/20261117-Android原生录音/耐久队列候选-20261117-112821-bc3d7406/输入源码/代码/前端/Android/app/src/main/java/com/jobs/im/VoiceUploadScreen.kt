// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import kotlinx.coroutines.Job

@Composable
fun VoiceUploadRecoveryButton(model: ChatModel) {
    var visible by remember { mutableStateOf(false) }
    var work by remember { mutableStateOf<Job?>(null) }
    var discard by remember { mutableStateOf<VoiceUploadIntent?>(null) }
    val lifecycle = (LocalContext.current as ComponentActivity).lifecycle
    val owner = model.stableVoiceUploadOwner()
    fun t(key: String) = VoiceUploadText.text(model.language, key)
    fun close() { work?.cancel(); work = null; discard = null; visible = false }
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_PAUSE || event == Lifecycle.Event.ON_STOP) close()
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer); close() }
    }
    LaunchedEffect(owner) { if (visible) { close() } }
    IconButton(onClick = { visible = true; model.reloadVoiceUploads() }, enabled = owner != null && !model.demo && !model.busy) {
        Icon(painterResource(android.R.drawable.ic_menu_recent_history), contentDescription = t("title"))
    }
    if (!visible) return
    AlertDialog(
        onDismissRequest = { close() },
        title = { Text(t("title")) },
        text = {
            LazyColumn(Modifier.fillMaxWidth().heightIn(max = 440.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                item {
                    Text(t("retained"), style = MaterialTheme.typography.bodyMedium)
                    TextButton(onClick = { model.reloadVoiceUploads() }, enabled = !model.busy) { Text(t("reload")) }
                    if (model.voiceUploadProblem.isNotEmpty()) Text(t("problem"), color = MaterialTheme.colorScheme.error)
                }
                if (model.voiceUploadEntries.isEmpty()) item {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(t("empty"), style = MaterialTheme.typography.titleMedium)
                        Text(t("emptyHelp"), style = MaterialTheme.typography.bodyMedium)
                        Button(onClick = { model.reloadVoiceUploads() }, enabled = !model.busy) { Text(t("reload")) }
                    }
                }
                items(model.voiceUploadEntries, key = { it.requestId }) { entry ->
                    val intent = entry.intent
                    if (intent == null) Text(t("problem"), color = MaterialTheme.colorScheme.error)
                    else {
                        val sameOwner = intent.source.owner == model.stableVoiceUploadOwner()
                        val frozen = intent.frozen || !sameOwner
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(t(if (frozen) "frozen" else intent.phase.name), style = MaterialTheme.typography.titleMedium)
                            Text(t("origin") + " · " + intent.source.owner.origin, style = MaterialTheme.typography.bodySmall)
                            Text(intent.source.conversationTitle.ifBlank { intent.source.conversation.take(12) },
                                style = MaterialTheme.typography.bodyMedium)
                            if (entry.problem.isNotEmpty()) Text(t("problem"), color = MaterialTheme.colorScheme.error)
                            if (!model.mayResumeVoiceUpload(intent) && intent.phase !in VoiceUploadRules.terminal)
                                Text(t("originalChat"), style = MaterialTheme.typography.bodyMedium)
                            TextButton(onClick = { work = model.actVoiceUpload(intent.requestId, false) },
                                enabled = !model.busy && sameOwner && !frozen && entry.problem.isEmpty() && intent.phase !in VoiceUploadRules.terminal) {
                                Text(t("check"))
                            }
                            Button(onClick = { work = model.actVoiceUpload(intent.requestId, true) },
                                enabled = !model.busy && entry.problem.isEmpty() && model.mayResumeVoiceUpload(intent) && intent.phase !in VoiceUploadRules.terminal) {
                                Text(t("resume"))
                            }
                            if (intent.phase == VoiceUploadPhase.PREPARED && !frozen) TextButton(
                                onClick = { discard = intent }, enabled = !model.busy && sameOwner) { Text(t("discard")) }
                            HorizontalDivider()
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = { close() }) { Text(t("cancel")) } },
    )
    discard?.let { original ->
        AlertDialog(onDismissRequest = { discard = null }, title = { Text(t("discard")) },
            text = { Text(t("discardConfirm")) }, confirmButton = {
                TextButton(onClick = { model.cancelUnsubmittedVoiceUpload(original.requestId); discard = null }, enabled = !model.busy) {
                    Text(t("confirm"))
                }
            }, dismissButton = { TextButton(onClick = { discard = null }) { Text(t("cancel")) } })
    }
}

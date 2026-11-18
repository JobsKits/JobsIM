package com.jobs.im

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale
import kotlinx.coroutines.delay

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PersonalExportScreen(model: ChatModel, onClose: () -> Unit) {
    val state = model.personalExports
    fun t(key: String) = Strings.text(model.language, key)
    var confirming by remember { mutableStateOf("") }
    var saveId by remember { mutableStateOf("") }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/zip")) { uri ->
        val id = saveId; saveId = ""
        if (id.isNotEmpty()) state.save(id, uri)
    }
    DisposableEffect(state) { onDispose { state.reset() } }
    LaunchedEffect(model.storageNamespace, state.connected) {
        confirming = ""
        if (state.connected) state.refresh().join()
    }
    LaunchedEffect(state.preparedId) {
        val id = state.preparedId
        if (id.isNotEmpty()) state.claimSaveName(id)?.let { name ->
            saveId = id
            try { picker.launch(name) } catch (_: Exception) { saveId = ""; state.save(id, null) }
        }
    }
    LaunchedEffect(model.storageNamespace, state.connected) {
        while (state.connected) {
            delay(5000)
            if (!state.busy && state.preparedId.isEmpty() && state.views.values.any { it.state in setOf("requested", "building") }) state.refresh().join()
        }
    }
    val formatter = remember(model.language) {
        DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(Locale.forLanguageTag(model.language)).withZone(ZoneId.systemDefault())
    }
    val interactive = !state.busy && state.preparedId.isEmpty()
    Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(Modifier.fillMaxSize()) {
            Scaffold(
                modifier = Modifier.safeDrawingPadding(),
                topBar = {
                    TopAppBar(title = { Text(t("exportTitle")) }, navigationIcon = {
                        IconButton(onClick = onClose) { Icon(Icons.AutoMirrored.Filled.ArrowBack, t("back")) }
                    }, actions = {
                        IconButton(onClick = { state.refresh() }, enabled = interactive && state.connected) { Icon(Icons.Default.Refresh, t("refresh")) }
                    })
                },
            ) { padding ->
                LazyColumn(Modifier.padding(padding).fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    if (state.busy) item { LinearProgressIndicator(Modifier.fillMaxWidth()) }
                    item { Text(t("exportSummary"), style = MaterialTheme.typography.bodyMedium) }
                    if (!state.connected) item {
                        Text(t("exportConnect"), style = MaterialTheme.typography.titleMedium)
                        Text(t("exportUnavailable"), style = MaterialTheme.typography.bodyMedium)
                        OutlinedButton(onClick = onClose, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("back")) }
                    } else {
                        item {
                            Text(t("exportLimits"), style = MaterialTheme.typography.bodySmall)
                            if (!state.enabled && !state.busy) Text(t("exportRestricted"), style = MaterialTheme.typography.bodyMedium)
                            Button(onClick = { state.create() }, enabled = interactive && state.enabled && state.rows.none { it.jobId.isEmpty() }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("exportCreate")) }
                        }
                        item {
                            Text(t("exportLocalList"), style = MaterialTheme.typography.titleMedium)
                            Text(t("exportLocalOnly"), style = MaterialTheme.typography.bodySmall)
                        }
                        if (state.rows.isEmpty()) item {
                            Column(Modifier.fillMaxWidth().padding(vertical = 24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                Text(t(if (state.problem.isEmpty()) "exportEmpty" else "exportNotConfirmed"), style = MaterialTheme.typography.titleMedium)
                                OutlinedButton(onClick = { state.refresh() }, enabled = interactive, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("refresh")) }
                            }
                        }
                        items(state.rows.take(state.shown), key = { it.requestId }) { row ->
                            val job = state.views[row.requestId]
                            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                HorizontalDivider()
                                Text(runCatching { formatter.format(SelfStateWire.utc(row.createdAt)) }.getOrDefault(row.createdAt), style = MaterialTheme.typography.titleSmall)
                                Text(t("export" + (job?.state ?: "unknown").replaceFirstChar { it.uppercase() }), style = MaterialTheme.typography.bodyMedium)
                                if (job != null) {
                                    Text(t("exportDeadline") + " · " + formatter.format(job.expiresAt), style = MaterialTheme.typography.bodySmall)
                                    if (job.cleanup == "source_handoff_pending" || job.terminal && job.cleanup !in setOf("published", "cleared")) Text(t("exportCleanup"), style = MaterialTheme.typography.bodySmall)
                                } else Text(t("exportPendingHint"), style = MaterialTheme.typography.bodySmall)
                                OutlinedButton(onClick = { state.check(row.requestId) }, enabled = interactive, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("exportCheck")) }
                                if (row.jobId.isEmpty()) OutlinedButton(onClick = { state.check(row.requestId, true) }, enabled = interactive && state.enabled, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("exportRetry")) }
                                if (state.enabled && job?.ready == true && row.cancelBody.isEmpty()) Button(onClick = { state.download(row.requestId) }, enabled = interactive, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("exportDownload")) }
                                if (row.jobId.isNotEmpty() && job?.terminal != true) TextButton(onClick = { confirming = row.requestId }, enabled = interactive, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t(if (row.cancelBody.isEmpty()) "exportCancel" else "exportRetryCancel")) }
                            }
                        }
                        if (state.rows.size > state.shown) item { OutlinedButton(onClick = { state.more() }, enabled = interactive, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("exportMore")) } }
                        if (state.downloading) item {
                            Text(t("exportDownloading") + " · " + state.downloadedBytes / 1024 + " KiB", style = MaterialTheme.typography.bodyMedium)
                            OutlinedButton(onClick = { state.stopDownload() }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("exportStopDownload")) }
                        }
                    }
                    if (state.problem.isNotEmpty()) item {
                        val key = when (state.problem) {
                            "EXPORT_FILE_LIMIT" -> "exportLimit"
                            "EXPORT_FILE_INVALID", "EXPORT_INTEGRITY_FAILED" -> "exportFileInvalid"
                            "EXPORT_SAVE_FAILED" -> "exportSaveFailed"
                            "EXPORT_STORE_NOT_READY" -> "exportStoreNotReady"
                            "EXPORT_RATE_LIMITED" -> "exportLimited"
                            "EXPORT_CONNECT_REQUIRED" -> "exportConnect"
                            else -> "exportNotConfirmed"
                        }
                        Text(t(key), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                    }
                    if (state.notice.isNotEmpty()) item { Text(t(state.notice), style = MaterialTheme.typography.bodyMedium) }
                    item { Text(t("exportDownloadHint") + " " + t("exportLimit"), style = MaterialTheme.typography.bodySmall) }
                }
            }
        }
    }
    if (confirming.isNotEmpty()) AlertDialog(onDismissRequest = { confirming = "" }, title = { Text(t("exportCancel")) }, text = { Text(t("exportCancelConfirm")) },
        confirmButton = { TextButton(onClick = { val id = confirming; confirming = ""; state.cancel(id) }, enabled = interactive) { Text(t("exportConfirmCancel")) } },
        dismissButton = { TextButton(onClick = { confirming = "" }) { Text(t("exportKeep")) } })
}

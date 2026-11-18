// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
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
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay

@Composable
fun VoiceRecordingComposer(model: ChatModel) {
    val context = LocalContext.current
    val lifecycle = (context as ComponentActivity).lifecycle
    val owner = model.voiceRecordingOwner()
    var visible by remember { mutableStateOf(false) }
    var boundOwner by remember { mutableStateOf<VoiceRecordingOwner?>(null) }
    var state by remember { mutableStateOf(VoiceRecordingState()) }
    var elapsed by remember { mutableStateOf(0L) }
    var notice by remember { mutableStateOf("") }
    var permissionPending by remember { mutableStateOf(false) }
    var uploadJob by remember { mutableStateOf<Job?>(null) }
    val engine = remember(context.applicationContext) { VoiceRecordingEngine(context) { state = it } }
    fun t(key: String) = VoiceRecordingText.text(model.language, key)
    fun current(): Boolean = boundOwner != null && boundOwner == model.voiceRecordingOwner() &&
        lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)
    fun close() {
        uploadJob?.cancel()
        uploadJob = null
        engine.discard()
        boundOwner = null
        permissionPending = false
        visible = false
    }
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        permissionPending = false
        // Returning from a permission sheet never starts capture, even if its original screen survived.
        if (visible && current()) notice = if (granted) "" else "permissionDenied"
    }
    DisposableEffect(lifecycle, engine) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_PAUSE || event == Lifecycle.Event.ON_STOP) close()
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer); close() }
    }
    LaunchedEffect(owner) {
        if (visible && boundOwner != owner) close()
    }
    LaunchedEffect(state.phase) {
        while (state.phase == VoiceRecordingPhase.RECORDING) {
            if (!current()) { close(); break }
            elapsed = engine.elapsedMillis()
            if (elapsed >= VoiceRecordingRules.MAX_MILLIS) engine.stop()
            delay(250)
        }
    }
    IconButton(
        onClick = { boundOwner = model.voiceRecordingOwner(); notice = ""; engine.discard(); visible = boundOwner != null },
        enabled = owner != null && !model.busy,
    ) {
        Icon(painterResource(android.R.drawable.ic_btn_speak_now), contentDescription = t("recordVoice"))
    }
    VoiceUploadRecoveryButton(model)
    if (!visible) return
    val phase = state.phase
    val granted = ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED
    val primary = when (phase) {
        VoiceRecordingPhase.RECORDING -> "stop"
        VoiceRecordingPhase.PREVIEW -> "send"
        VoiceRecordingPhase.SENDING -> "sending"
        VoiceRecordingPhase.READY, VoiceRecordingPhase.ERROR -> if (granted) "start" else "allow"
    }
    AlertDialog(
        onDismissRequest = { close() },
        title = { Text(t("title")) },
        text = {
            Column(Modifier.widthIn(max = 480.dp).heightIn(max = 400.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(t("help"), style = MaterialTheme.typography.bodyMedium)
                Text(t("limit"), style = MaterialTheme.typography.bodySmall)
                if (phase == VoiceRecordingPhase.RECORDING || phase == VoiceRecordingPhase.PREVIEW) {
                    val duration = if (phase == VoiceRecordingPhase.RECORDING) elapsed else state.durationMillis
                    Text(t(if (phase == VoiceRecordingPhase.RECORDING) "recording" else "preview") + " " +
                        (duration / 60000).toString().padStart(2, '0') + ":" +
                        (duration / 1000 % 60).toString().padStart(2, '0'), style = MaterialTheme.typography.titleMedium)
                }
                val failure = notice.ifBlank { state.error }
                if (failure.isNotBlank()) Text(t(failure), color = MaterialTheme.colorScheme.error)
                if (phase == VoiceRecordingPhase.PREVIEW) {
                    TextButton(onClick = { if (current()) engine.play() else close() }) {
                        Text(t(if (state.playing) "pause" else "play"))
                    }
                    TextButton(onClick = { engine.discard(); notice = "" }) { Text(t("retry")) }
                }
            }
        },
        confirmButton = {
            Button(enabled = !permissionPending && phase != VoiceRecordingPhase.SENDING &&
                (phase == VoiceRecordingPhase.RECORDING || !model.busy), onClick = {
                if (!current()) { close(); return@Button }
                notice = ""
                when (phase) {
                    VoiceRecordingPhase.RECORDING -> engine.stop()
                    VoiceRecordingPhase.PREVIEW -> {
                        val candidate = engine.prepareUpload(model.voiceRecordingOwner()) ?: return@Button
                        if (model.demo) uploadJob = model.uploadVoice(candidate, queued = {
                            engine.queued(candidate); visible = false; boundOwner = null; model.info = t("queued")
                        }, failed = { engine.uploadFailed(candidate) })
                        else uploadJob = model.saveVoiceRecording(candidate, state.durationMillis, saved = {
                            engine.queued(candidate); visible = false; boundOwner = null
                        }, failed = { engine.uploadFailed(candidate) })
                    }
                    VoiceRecordingPhase.READY, VoiceRecordingPhase.ERROR -> {
                        if (!granted) {
                            permissionPending = true
                            permission.launch(Manifest.permission.RECORD_AUDIO)
                        } else engine.begin(checkNotNull(boundOwner))
                    }
                    VoiceRecordingPhase.SENDING -> Unit
                }
            }) { Text(t(primary)) }
        },
        dismissButton = { TextButton(onClick = { close() }) { Text(t("cancel")) } },
    )
}

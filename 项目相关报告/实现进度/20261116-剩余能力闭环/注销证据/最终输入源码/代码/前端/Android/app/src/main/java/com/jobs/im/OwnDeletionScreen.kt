package com.jobs.im

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.LifecycleOwner
import org.json.JSONObject

@Composable
fun OwnDeletionScreen(model: ChatModel, onClose: () -> Unit) {
    val state = model.ownDeletions
    fun t(key: String) = OwnDeletionText.text(model.language, key)
    var password by remember { mutableStateOf("") }
    var recovery by remember { mutableStateOf("") }
    var otp by remember { mutableStateOf("") }
    var confirmation by remember { mutableStateOf<Pair<String, String>?>(null) }
    var shown by remember { mutableIntStateOf(20) }
    val keyboard = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Password)
    fun clear() { password = ""; recovery = ""; otp = ""; confirmation = null }
    fun request(id: String?, retry: Boolean) { state.request(id, password, otp, retry); clear() }
    fun rights(id: String, cancel: Boolean, version: String = "") { state.rights(id, password, recovery, otp, cancel, version); clear() }
    LaunchedEffect(model.storageNamespace) { state.refresh().join() }
    DisposableEffect(state) { onDispose { clear(); state.cancelSensitive() } }
    val lifecycle = (LocalContext.current as? LifecycleOwner)?.lifecycle
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event -> if (event == Lifecycle.Event.ON_STOP) { clear(); state.cancelSensitive() } }
        lifecycle?.addObserver(observer); onDispose { lifecycle?.removeObserver(observer) }
    }
    val originalReady = password.isNotEmpty() && recovery.isEmpty() && !state.busy
    val proofReady = (password.isNotEmpty() != recovery.isNotEmpty()) && !state.busy
    Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(Modifier.fillMaxSize()) {
            LazyColumn(Modifier.safeDrawingPadding().imePadding().fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                item {
                    TextButton(onClick = onClose) { Text(Strings.text(model.language, "back")) }
                    Text(t("title"), style = MaterialTheme.typography.headlineSmall)
                    Text(t("help"))
                    if (state.busy) LinearProgressIndicator(Modifier.fillMaxWidth())
                    if (state.notice.isNotEmpty()) Text(t(state.notice), color = if (state.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface)
                    OutlinedTextField(password, { password = it }, label = { Text(t("current")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = keyboard, modifier = Modifier.fillMaxWidth(), singleLine = true)
                    OutlinedTextField(recovery, { recovery = it }, label = { Text(t("recovery")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = keyboard, modifier = Modifier.fillMaxWidth(), singleLine = true)
                    OutlinedTextField(otp, { otp = it }, label = { Text(t("otp")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = keyboard, modifier = Modifier.fillMaxWidth(), singleLine = true)
                    Button(onClick = { confirmation = "" to "" }, enabled = originalReady && model.userId.isNotEmpty() && !model.demo, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("submit")) }
                    Text(t("requests"), style = MaterialTheme.typography.titleMedium)
                    OutlinedButton(onClick = { state.refresh() }, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(Strings.text(model.language, "refresh")) }
                }
                if (state.rows.isEmpty()) item {
                    Column(Modifier.fillMaxWidth().padding(vertical = 24.dp)) {
                        Text(t(if (state.failed) "error" else "empty"))
                        TextButton(onClick = { state.refresh() }, enabled = !state.busy) { Text(Strings.text(model.language, "refresh")) }
                    }
                }
                items(state.rows.take(shown), key = { it.id }) { row ->
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        HorizontalDivider(); Text(row.meta.username + " · " + row.meta.created); Text(t(row.state))
                        if (row.result.isNotEmpty()) {
                            val result = JSONObject(row.result)
                            if (!result.isNull("cancelUntil")) Text(t("deadline") + " · " + result.getString("cancelUntil"))
                            OutlinedButton(onClick = { rights(row.id, false) }, enabled = proofReady, modifier = Modifier.fillMaxWidth()) { Text(t("proofQuery")) }
                            if (row.state == "requested") OutlinedButton(onClick = { confirmation = row.id to result.getString("requestVersion") }, enabled = proofReady, modifier = Modifier.fillMaxWidth()) { Text(t("proofCancel")) }
                            if (row.cancelState == "unknown") Text(t("cancelUnknown"))
                        }
                        OutlinedButton(onClick = { request(row.id, false) }, enabled = originalReady, modifier = Modifier.fillMaxWidth()) { Text(t("check")) }
                        if (row.state in setOf("unsent", "unknown", "pending")) OutlinedButton(onClick = { request(row.id, true) }, enabled = originalReady, modifier = Modifier.fillMaxWidth()) { Text(t("retry")) }
                    }
                }
                if (state.rows.size > shown) item { TextButton(onClick = { shown += 20 }) { Text(Strings.text(model.language, "loadMore")) } }
            }
        }
    }
    confirmation?.let { (selected, version) ->
        AlertDialog(onDismissRequest = { confirmation = null }, title = { Text(t("title")) }, text = { Text(t(if (selected.isEmpty()) "help" else "confirmCancel")) },
            confirmButton = { TextButton(onClick = { if (selected.isEmpty()) request(null, false) else rights(selected, true, version) }, enabled = if (selected.isEmpty()) originalReady else proofReady) { Text(t(if (selected.isEmpty()) "submit" else "proofCancel")) } },
            dismissButton = { TextButton(onClick = { confirmation = null }) { Text(Strings.text(model.language, "cancel")) } })
    }
}

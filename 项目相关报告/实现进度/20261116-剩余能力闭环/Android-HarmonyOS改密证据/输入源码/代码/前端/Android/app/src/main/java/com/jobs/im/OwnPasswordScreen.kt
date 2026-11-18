package com.jobs.im

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.LifecycleOwner

@Composable
fun OwnPasswordScreen(model: ChatModel, onClose: () -> Unit) {
    val state = model.passwordChanges
    fun t(key: String) = OwnPasswordText.text(model.language, key)
    var old by remember { mutableStateOf("") }
    var next by remember { mutableStateOf("") }
    var confirmation by remember { mutableStateOf("") }
    var otp by remember { mutableStateOf("") }
    var confirm by remember { mutableStateOf(false) }
    var shown by remember { mutableIntStateOf(20) }
    val secureKeyboard = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Password)
    fun submit(id: String?, retry: Boolean) {
        state.perform(id, old, next, confirmation, otp, retry)
        old = ""; next = ""; confirmation = ""; otp = ""; confirm = false
    }
    LaunchedEffect(model.storageNamespace) { state.refresh().join() }
    DisposableEffect(state) { onDispose { old = ""; next = ""; confirmation = ""; otp = "" } }
    val lifecycle = (LocalContext.current as? LifecycleOwner)?.lifecycle
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event -> if (event == Lifecycle.Event.ON_STOP) { old = ""; next = ""; confirmation = ""; otp = ""; confirm = false } }
        lifecycle?.addObserver(observer)
        onDispose { lifecycle?.removeObserver(observer) }
    }
    val proofReady = old.isNotEmpty() && next.isNotEmpty() && confirmation.isNotEmpty() && !state.busy
    Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(Modifier.fillMaxSize()) {
            LazyColumn(Modifier.safeDrawingPadding().imePadding().fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                item {
                    TextButton(onClick = onClose) { Text(Strings.text(model.language, "back")) }
                    Text(t("title"), style = MaterialTheme.typography.headlineSmall)
                    Text(t("help"), style = MaterialTheme.typography.bodyMedium)
                    if (state.busy) LinearProgressIndicator(Modifier.fillMaxWidth())
                    if (state.notice.isNotEmpty()) Text(t(state.notice), color = if (state.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface)
                    OutlinedTextField(old, { old = it }, label = { Text(t("current")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secureKeyboard, singleLine = true, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(next, { next = it }, label = { Text(t("new")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secureKeyboard, singleLine = true, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(confirmation, { confirmation = it }, label = { Text(t("confirm")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secureKeyboard, singleLine = true, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(otp, { otp = it }, label = { Text(t("otp")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secureKeyboard, singleLine = true, modifier = Modifier.fillMaxWidth())
                    Button(onClick = { confirm = true }, enabled = proofReady && model.userId.isNotEmpty() && !model.demo, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("submit")) }
                    if (model.userId.isEmpty() || model.demo) Text(t("unavailable"))
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
                        HorizontalDivider()
                        Text(row.username + " · " + row.created)
                        Text(t(row.state))
                        if (OwnPasswordRules.blocked(row) && row.state != "changed") Text(t("blocked"))
                        if (row.state == "changed") Text(t("login"))
                        OutlinedButton(onClick = { submit(row.id, false) }, enabled = proofReady, modifier = Modifier.fillMaxWidth()) { Text(t("check")) }
                        if (row.state in setOf("unsent", "unknown", "pending")) OutlinedButton(onClick = { submit(row.id, true) }, enabled = proofReady, modifier = Modifier.fillMaxWidth()) { Text(t("retry")) }
                    }
                }
                if (state.rows.size > shown) item { TextButton(onClick = { shown += 20 }) { Text(Strings.text(model.language, "loadMore")) } }
            }
        }
    }
    if (confirm) AlertDialog(onDismissRequest = { confirm = false }, title = { Text(t("title")) }, text = { Text(t("help")) },
        confirmButton = { TextButton(onClick = { submit(null, false) }, enabled = proofReady) { Text(t("submit")) } },
        dismissButton = { TextButton(onClick = { confirm = false }) { Text(Strings.text(model.language, "cancel")) } })
}

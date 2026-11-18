package com.jobs.im

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.LifecycleOwner
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import org.json.JSONArray
import org.json.JSONObject

@Composable
fun MigrationScreen(model: ChatModel, onClose: () -> Unit) {
    fun t(key: String) = MigrationText.text(model.language, key)
    val context = LocalContext.current
    val coroutine = rememberCoroutineScope()
    val clipboard = LocalClipboardManager.current
    var visible by remember { mutableStateOf(true) }
    var active by remember { mutableStateOf(true) }
    var service by remember { mutableStateOf<MigrationService?>(null) }
    var rows by remember { mutableStateOf<List<MigrationIntent>>(emptyList()) }
    var history by remember { mutableStateOf<List<String>>(emptyList()) }
    var password by remember { mutableStateOf("") }
    var otp by remember { mutableStateOf("") }
    var code by remember { mutableStateOf("") }
    var packageId by remember { mutableStateOf("") }
    var notice by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var full by remember { mutableStateOf(false) }
    var selected by remember { mutableStateOf<Set<String>>(emptySet()) }
    var shown by remember { mutableIntStateOf(20) }
    var pendingSave by remember { mutableStateOf<Pair<String, android.net.Uri>?>(null) }
    var pickingId by remember { mutableStateOf("") }
    var pickingOwner by remember { mutableStateOf<MigrationIdentity?>(null) }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/octet-stream")) { uri ->
        if (uri != null && pickingId.isNotEmpty()) pendingSave = pickingId to uri
        pickingId = ""
    }
    suspend fun reload(s: MigrationService) { val next = s.rows(); val archive = s.history(); if (service === s && active) { rows = next; history = archive } }
    fun action(block: suspend (MigrationService, String, String, String) -> Unit) {
        val s = service ?: return
        if (busy) return
        val p = password; val o = otp; val c = code
        password = ""; otp = ""; code = ""; busy = true; notice = ""
        coroutine.launch {
            try { block(s, p, o, c); if (service === s && active) { reload(s); if (notice.isEmpty()) notice = "success" } }
            catch (_: Exception) { if (service === s && active) notice = "error" }
            finally { if (service === s) busy = false }
        }
    }
    DisposableEffect(Unit) { model.migrationsVisible = true; onDispose { visible = false; service?.clear(); model.migrationsVisible = false; password = ""; otp = ""; code = ""; history = emptyList() } }
    val lifecycle = (context as? LifecycleOwner)?.lifecycle
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_STOP) { active = false; password = ""; otp = ""; code = ""; history = emptyList(); service?.clear(); service = null; busy = false }
            if (event == Lifecycle.Event.ON_START) active = true
        }
        lifecycle?.addObserver(observer); onDispose { lifecycle?.removeObserver(observer) }
    }
    LaunchedEffect(active, model.storageNamespace, model.userId) {
        service?.clear(); service = null; rows = emptyList(); history = emptyList()
        if (active && visible) try { val s = model.migrationService { visible && active }; service = s; s.capability(); reload(s) } catch (_: Exception) { notice = "connect" }
    }
    LaunchedEffect(service) {
        val s = service ?: return@LaunchedEffect
        while (visible && active && service === s) { delay(5000); if (!busy) try { reload(s) } catch (_: Exception) { if (service === s) { history = emptyList(); notice = "error" } } }
    }
    LaunchedEffect(service, pendingSave) {
        val s = service; val pending = pendingSave
        if (s != null && pending != null && active) {
            pendingSave = null; busy = true
            try {
                require(pickingOwner?.let { MigrationStore.samePrincipal(it, s.identity) } == true)
                val (_, bytes) = s.objectBytes(pending.first)
                try { require(service === s && active); context.contentResolver.openOutputStream(pending.second, "wt").use { out -> requireNotNull(out); out.write(bytes); out.flush() }; notice = "success" } finally { bytes.fill(0) }
            } catch (_: Exception) { notice = "error" } finally { if (service === s) busy = false; pickingOwner = null }
        }
    }
    val candidates = model.messages.filter { it.state !in setOf("pending", "frozen") && !it.retracted && !it.contentUnavailable && !it.forwardUnavailable && it.contentVersion.matches(Regex("[1-9][0-9]*")) && it.type in setOf("text", "image", "audio", "video", "file") }
    fun selection(): String = JSONArray().also { array -> candidates.filter { it.id in selected }.forEach { m ->
        val mode = if (m.type == "text") "none" else if (full && !m.forwarded && m.attachmentId.isNotEmpty()) "full" else "metadata_only"
        array.put(JSONObject().put("id", m.id).put("conversationId", m.conversationId).put("contentVersion", m.contentVersion).put("type", m.type).put("objectMode", mode).also { if (m.forwarded) it.put("forwarded", true) else if (m.attachmentId.isNotEmpty()) it.put("attachmentId", m.attachmentId) })
    } }.toString()
    val secure = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Password)
    Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(Modifier.fillMaxSize()) {
            LazyColumn(Modifier.safeDrawingPadding().imePadding().fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                item {
                    TextButton(onClick = onClose) { Text(t("back")) }; Text(t("title"), style = MaterialTheme.typography.headlineSmall); Text(t("help"))
                    if (busy) LinearProgressIndicator(Modifier.fillMaxWidth())
                    if (notice.isNotEmpty()) Text(t(notice))
                    OutlinedButton(onClick = { action { s, _, _, _ -> reload(s) } }, enabled = !busy && service != null, modifier = Modifier.fillMaxWidth()) { Text(t("reload")) }
                    OutlinedTextField(password, { password = it }, label = { Text(t("password")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secure, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(otp, { otp = it }, label = { Text(t("otp")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secure, modifier = Modifier.fillMaxWidth())
                    Button(onClick = { action { s, p, o, _ -> val row = s.stage("pairing"); s.pair(row.id, p, o, true) } }, enabled = !busy && service != null && password.isNotEmpty(), modifier = Modifier.fillMaxWidth()) { Text(t("pair")) }
                    Text(t("source"), style = MaterialTheme.typography.titleMedium)
                    OutlinedTextField(code, { code = it }, label = { Text(t("code")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secure, modifier = Modifier.fillMaxWidth())
                    Row { Checkbox(full, { full = it }); Text(t("full")) }
                }
                if (candidates.isEmpty()) item { EmptyPanel(model) { action { s, _, _, _ -> reload(s) } } }
                items(candidates.take(shown), key = { "source-" + it.id }) { m -> Row { Checkbox(m.id in selected, { checked -> selected = if (checked) selected + m.id else selected - m.id }); Text(m.text.ifBlank { t("metadata") }, modifier = Modifier.weight(1f)) } }
                item {
                    Button(onClick = { val body = selection(); action { s, p, o, c -> val row = s.stage("create", body); s.create(row.id, c, p, o, true)?.let { packageId = it.id } } }, enabled = !busy && service != null && selected.size in 1..256 && password.isNotEmpty() && code.isNotEmpty(), modifier = Modifier.fillMaxWidth()) { Text(t("send")) }
                    Text(t("target"), style = MaterialTheme.typography.titleMedium)
                    OutlinedTextField(packageId, { packageId = it }, label = { Text(t("package")) }, modifier = Modifier.fillMaxWidth())
                    Button(onClick = { val id = packageId; action { s, p, o, _ -> if (!s.install(id, p, o)) notice = "local" } }, enabled = !busy && service != null && password.isNotEmpty() && packageId.matches(Regex("[0-9a-f-]{36}")), modifier = Modifier.fillMaxWidth()) { Text(t("install")) }
                    Text(t("operations"), style = MaterialTheme.typography.titleMedium)
                    if (rows.isEmpty()) EmptyPanel(model) { action { s, _, _, _ -> reload(s) } }
                }
                items(rows.take(shown), key = { "operation-" + it.identity.namespace + it.id }) { row -> Column {
                    Text(t(row.kind) + " · " + t(row.state)); Text(row.created)
                    if (row.migration.isNotEmpty()) Text(row.migration)
                    Row { TextButton(onClick = { action { s, _, _, _ -> s.checkOriginal(row.id) } }, enabled = !busy) { Text(t("check")) }
                        if (row.kind == "pairing") TextButton(onClick = { action { s, _, _, _ -> clipboard.setText(AnnotatedString(s.sharePair(row.id))); notice = "copied" } }, enabled = !busy) { Text(t("copy")) } }
                    if (row.kind != "target" && row.state in setOf("unsent", "unknown")) TextButton(onClick = { action { s, p, o, c -> if (row.kind == "pairing") s.pair(row.id, p, o, true) else s.create(row.id, c, p, o, true) } }, enabled = !busy && password.isNotEmpty()) { Text(t("retry")) }
                    if (row.migration.isNotEmpty() && row.state != "retired") TextButton(onClick = { action { s, _, _, _ -> s.cancel(row.migration) } }, enabled = !busy) { Text(t("cancel")) }
                } }
                item { Text(t("archive"), style = MaterialTheme.typography.titleMedium); if (history.isEmpty()) EmptyPanel(model) { action { s, _, _, _ -> reload(s) } } }
                items(history.take(shown), key = { "archive-" + JSONObject(it).getString("id") }) { raw -> val m = JSONObject(raw); Column { Text(m.getString("createdAt")); Text(if (m.optBoolean("contentUnavailable")) t("unavailable") else m.getString("text")); if (m.optString("objectMode") == "full" && !m.optBoolean("contentUnavailable")) TextButton(onClick = { pickingId = m.getString("id"); pickingOwner = service?.identity; picker.launch(m.optString("attachmentId", pickingId)) }, enabled = !busy) { Text(t("original")) } else if (m.getString("type") != "text") Text(t("metadata")) } }
                if (candidates.size > shown || rows.size > shown || history.size > shown) item { TextButton(onClick = { shown += 20 }) { Text(t("more")) } }
            }
        }
    }
}

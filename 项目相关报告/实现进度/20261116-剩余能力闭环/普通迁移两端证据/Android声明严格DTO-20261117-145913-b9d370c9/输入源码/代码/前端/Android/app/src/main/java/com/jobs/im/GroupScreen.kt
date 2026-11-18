package com.jobs.im

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
import java.time.Instant
import org.json.JSONObject

@Composable
fun GroupScreen(model: ChatModel, conversation: String = "", onClose: () -> Unit) {
    val state = model.groupTools
    fun t(key: String) = GroupText.text(model.language, key)
    var code by remember { mutableStateOf("") }
    var otp by remember { mutableStateOf("") }
    var confirmation by remember { mutableStateOf<Pair<GroupIntent, String>?>(null) }
    var inputNotice by remember { mutableStateOf("") }
    var shown by remember { mutableIntStateOf(20) }
    val clipboard = LocalClipboardManager.current
    val snapshot = state.snapshot.takeIf { it.isNotEmpty() }?.let { JSONObject(it) }
    val owner = snapshot?.optString("role") == "owner" && snapshot.optBoolean("readOnly").not()
    val manager = snapshot?.let { GroupRules.manager(it) } == true
    val recover = snapshot?.optBoolean("canRecover") == true
    var muteAll by remember(state.snapshot) { mutableStateOf(snapshot?.optJSONObject("policy")?.optBoolean("muteAll") ?: false) }
    var slow by remember(state.snapshot) { mutableStateOf(snapshot?.optJSONObject("policy")?.optInt("slowSeconds")?.toString() ?: "0") }
    var capacity by remember(state.snapshot) { mutableStateOf(snapshot?.optJSONObject("policy")?.optInt("maxMembers")?.toString() ?: "200") }
    var uses by remember { mutableStateOf("1") }
    var approval by remember { mutableStateOf(true) }
    fun stage(action: String, target: Person? = null, fields: JSONObject = JSONObject(), label: String = t(action)) {
        runCatching { state.draft(action, target, fields) }.onSuccess { confirmation = it to label }.onFailure { inputNotice = t("reloadRequired") }
    }
    LaunchedEffect(conversation, model.storageNamespace) { state.open(conversation) }
    DisposableEffect(state) { onDispose { code = ""; otp = ""; confirmation = null; state.close() } }
    val lifecycle = (LocalContext.current as? LifecycleOwner)?.lifecycle
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event -> if (event == Lifecycle.Event.ON_STOP) { code = ""; otp = ""; confirmation = null; state.close() } }
        lifecycle?.addObserver(observer); onDispose { lifecycle?.removeObserver(observer) }
    }
    val secure = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Password)
    Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(Modifier.fillMaxSize()) {
            LazyColumn(Modifier.safeDrawingPadding().imePadding().fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                item {
                    TextButton(onClick = onClose) { Text(t("back")) }
                    Text(t("title"), style = MaterialTheme.typography.headlineSmall)
                    Text(t("help"))
                    Text(t("privileged"))
                    if (state.busy) LinearProgressIndicator(Modifier.fillMaxWidth())
                    if (state.notice.isNotEmpty()) Text(t(state.notice), color = if (state.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface)
                    if (inputNotice.isNotEmpty()) Text(inputNotice)
                    OutlinedButton(onClick = { confirmation = null; inputNotice = ""; state.reload() }, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(t("refresh")) }
                    OutlinedTextField(code, { code = it }, label = { Text(t("code")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secure, modifier = Modifier.fillMaxWidth())
                    Button(onClick = { state.join(code); code = "" }, enabled = !state.busy && !model.demo && model.userId.isNotEmpty() && code.isNotBlank(), modifier = Modifier.fillMaxWidth()) { Text(t("join")) }
                }
                if (owner) item {
                    Text(t("settings"), style = MaterialTheme.typography.titleMedium)
                    Row { Checkbox(muteAll, { muteAll = it }); Text(t("muteAll")) }
                    OutlinedTextField(slow, { slow = it }, label = { Text(t("slow")) }, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(capacity, { capacity = it }, label = { Text(t("capacity")) }, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth())
                    Button(onClick = { stage("settings", fields = JSONObject().put("muteAll", muteAll).put("slowSeconds", slow.toInt()).put("maxMembers", capacity.toInt()), label = t("save")) }, enabled = !state.busy && (slow.toIntOrNull()?.let { it in 0..3600 } == true) && (capacity.toIntOrNull()?.let { it in 2..10000 } == true)) { Text(t("save")) }
                    OutlinedButton(onClick = { stage("dissolve") }, enabled = !state.busy) { Text(t("dissolve")) }
                }
                if (recover) item { OutlinedTextField(otp, { otp = it }, label = { Text(Strings.text(model.language, "otp")) }, visualTransformation = PasswordVisualTransformation(), keyboardOptions = secure, modifier = Modifier.fillMaxWidth()) }
                if (conversation.isNotEmpty()) item { Text(t("membersList"), style = MaterialTheme.typography.titleMedium) }
                if (conversation.isNotEmpty() && state.members.isEmpty()) item { EmptyPanel(model) { state.reload() } }
                items(state.members, key = { it.id }) { person ->
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text(person.name + " · " + person.username)
                        if (owner && person.id != model.userId) {
                            Row { TextButton(onClick = { stage("transfer", person, label = t("transfer") + " · " + person.name) }, enabled = !state.busy) { Text(t("transfer")) }
                                TextButton(onClick = { stage("role", person, JSONObject().put("role", if (person.role == "admin") "member" else "admin"), t(if (person.role == "admin") "makeMember" else "makeAdmin") + " · " + person.name) }, enabled = !state.busy) { Text(t(if (person.role == "admin") "makeMember" else "makeAdmin")) } }
                        }
                        if (manager && person.role != "owner" && (owner || person.role != "admin")) {
                            Row { TextButton(onClick = { stage("mute", person, JSONObject().put("mutedUntil", Instant.now().plusSeconds(3600).toString()), t("mute") + " · " + person.name) }, enabled = !state.busy) { Text(t("mute")) }
                                TextButton(onClick = { stage("mute", person, JSONObject().put("mutedUntil", ""), t("unmute") + " · " + person.name) }, enabled = !state.busy) { Text(t("unmute")) } }
                        }
                        if (recover && person.state == "active") TextButton(onClick = { stage("recover", person, label = t("recover") + " · " + person.name) }, enabled = !state.busy && otp.matches(Regex("[0-9]{6}"))) { Text(t("recover")) }
                    }
                }
                if (state.membersNext.isNotEmpty()) item { TextButton(onClick = { state.more("members") }, enabled = !state.busy) { Text(t("loadMore")) } }
                if (manager) item {
                    Text(t("invites"), style = MaterialTheme.typography.titleMedium)
                    OutlinedTextField(uses, { uses = it }, label = { Text(t("uses")) }, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth())
                    Row { Checkbox(approval, { approval = it }); Text(t("approval")) }
                    Text(t("expires"))
                    Button(onClick = { stage("invite", fields = JSONObject().put("maxUses", uses.toInt()).put("approvalRequired", approval).put("expiresAt", Instant.now().plusSeconds(86400).toString()), label = t("invite")) }, enabled = !state.busy && (uses.toIntOrNull()?.let { it in 1..10000 } == true)) { Text(t("invite")) }
                    if (state.invites.isEmpty()) EmptyPanel(model) { state.reload() }
                }
                items(state.invites, key = { JSONObject(it).getString("id") }) { raw ->
                    val invite = JSONObject(raw)
                    Column {
                        Text(invite.getString("expiresAt") + " · " + invite.optInt("uses") + "/" + invite.getInt("maxUses"))
                        if (invite.isNull("revokedAt")) Row {
                            TextButton(onClick = { runCatching { state.share(invite.getString("id")) }.onSuccess { clipboard.setText(AnnotatedString(it)); inputNotice = t("copied") }.onFailure { inputNotice = t("reloadRequired") } }, enabled = !state.busy) { Text(t("copy")) }
                            TextButton(onClick = { stage("revoke", fields = JSONObject().put("id", invite.getString("id"))) }, enabled = !state.busy) { Text(t("revoke")) }
                        }
                    }
                }
                if (state.invitesNext.isNotEmpty()) item { TextButton(onClick = { state.more("invites") }, enabled = !state.busy) { Text(t("loadMore")) } }
                if (manager) item { Text(t("requests"), style = MaterialTheme.typography.titleMedium); if (state.requests.isEmpty()) EmptyPanel(model) { state.reload() } }
                items(state.requests, key = { JSONObject(it).getString("id") }) { raw ->
                    val request = JSONObject(raw)
                    Column { Text(request.optString("displayName").ifBlank { request.optString("username").ifBlank { t("unavailable") } } + " · " + request.optString("username") + " · " + t(if(request.getString("state")=="pending") "joinPending" else if(request.getString("state")=="approved") "joinApproved" else "joinRejected"))
                        if (request.getString("state") == "pending") Row {
                            TextButton(onClick = { stage("decide", fields = JSONObject().put("id", request.getString("id")).put("approve", true), label = t("approve")) }, enabled = !state.busy) { Text(t("approve")) }
                            TextButton(onClick = { stage("decide", fields = JSONObject().put("id", request.getString("id")).put("approve", false), label = t("reject")) }, enabled = !state.busy) { Text(t("reject")) }
                        }
                    }
                }
                if (state.requestsNext.isNotEmpty()) item { TextButton(onClick = { state.more("join-requests") }, enabled = !state.busy) { Text(t("loadMore")) } }
                item { Text(t("operations"), style = MaterialTheme.typography.titleMedium); if (state.operations.none { conversation.isEmpty() || it.conversation==conversation }) EmptyPanel(model) { state.reload() } }
                items(state.operations.filter { conversation.isEmpty() || it.conversation == conversation }.take(shown), key = { it.namespace + it.id }) { row ->
                    Column { Text(t(when(row.kind){"governance"->"title";"decide"->"requests";else->row.kind}) + " · " + t(if(row.kind=="join" && row.state=="applied") when(JSONObject(row.result).optString("state")){"pending"->"joinPending";"approved"->"joinApproved";"rejected"->"joinRejected";else->row.state} else row.state)); Text(row.created)
                        Row { TextButton(onClick = { state.submit(row, false) }, enabled = !state.busy) { Text(OwnPasswordText.text(model.language, "check")) }
                            if (row.state != "applied") TextButton(onClick = { state.submit(row, true, code, otp); code = ""; otp = "" }, enabled = !state.busy) { Text(OwnPasswordText.text(model.language, "retry")) } }
                    }
                }
                if (state.operations.size > shown) item { TextButton(onClick = { shown += 20 }) { Text(t("loadMore")) } }
            }
        }
    }
    confirmation?.let { (row, label) -> AlertDialog(onDismissRequest = { confirmation = null }, title = { Text(t("confirm")) }, text = { Text(label) },
        confirmButton = { TextButton(onClick = { state.submit(row, otp = otp); otp = ""; confirmation = null }, enabled = !state.busy) { Text(t("confirm")) } },
        dismissButton = { TextButton(onClick = { confirmation = null }) { Text(t("cancel")) } }) }
}

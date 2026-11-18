package com.jobs.im

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp

@Composable
fun ContactsPanel(model: ChatModel, onPerson: (Person) -> Unit) {
    fun t(key: String) = Strings.text(model.language, key)
    var query by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize()) {
        OutlinedTextField(
            query,
            { query = it },
            label = { Text(t("search")) },
            modifier = Modifier.fillMaxWidth().padding(16.dp),
            singleLine = true,
        )
        Row {
            TextButton(onClick = { model.findPeople(query) }, enabled = !model.busy) {
                Text(t("search"))
            }
            TextButton(onClick = { model.refresh() }, enabled = !model.busy) { Text(t("refresh")) }
        }
        LazyColumn(contentPadding = PaddingValues(16.dp), modifier = Modifier.fillMaxSize()) {
            if (model.requests.isNotEmpty())
                item { Text(t("requests"), style = MaterialTheme.typography.titleMedium) }
            items(model.requests, key = { "request-" + it.id }) { request ->
                ListItem(
                    headlineContent = { Text(request.name) },
                    supportingContent = { Text(request.username) },
                    trailingContent = {
                        Row {
                            TextButton(
                                onClick = { model.resolveRequest(request.id, true) },
                                enabled = !model.busy,
                            ) {
                                Text(t("accept"))
                            }
                            TextButton(
                                onClick = { model.resolveRequest(request.id, false) },
                                enabled = !model.busy,
                            ) {
                                Text(t("reject"))
                            }
                        }
                    },
                )
            }
            items(model.people, key = { it.id }) { person ->
                ListItem(
                    headlineContent = { Text(person.name) },
                    supportingContent = { Text("@" + person.username) },
                    leadingContent = { ProfileAvatar(person, model) },
                    modifier = Modifier.clickable { onPerson(person) },
                    trailingContent = {
                        if (model.contacts.none { it.id == person.id })
                            TextButton(
                                onClick = { model.addContact(person.id) },
                                enabled = !model.busy,
                            ) {
                                Text(t("add"))
                            }
                    },
                )
            }
            if (model.people.isEmpty() && model.requests.isEmpty()) item { EmptyPanel(model) }
        }
    }
}

@Composable
fun PersonDialog(person: Person, model: ChatModel, onDismiss: () -> Unit) {
    fun t(key: String) = Strings.text(model.language, key)
    val relationship = model.relationships[person.id]
    LaunchedEffect(person.id) { model.loadRelationship(person.id) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(person.name) },
        text = {
            Column {
                Text("@" + person.username)
                TextButton(
                    onClick = {
                        model.create(person.name, listOf(person.id), false)
                        onDismiss()
                    },
                    enabled = !model.busy,
                ) {
                    Text(t("newChat"))
                }
                val actions =
                    if (relationship == null) emptyList()
                    else
                        buildList {
                            if (relationship.friend) add("remove")
                            add(if (relationship.blockedByMe) "unblock" else "block")
                            if (relationship.inboundGate == "denied" && !relationship.blockedByMe)
                                add("reopen")
                        }
                actions.forEach { action ->
                    TextButton(
                        onClick = { model.relationship(person.id, action) },
                        enabled = !model.busy && !model.demo,
                    ) {
                        Text(t(action))
                    }
                }
                if (!model.demo)
                    TextButton(
                        onClick = { model.loadRelationship(person.id) },
                        enabled = !model.busy,
                    ) {
                        Text(t("refresh"))
                    }
                ReportForm("user", person.id, model)
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(t("close")) } },
    )
}

@Composable
fun GroupDialog(model: ChatModel, onDismiss: () -> Unit) {
    val conversation = model.selected ?: return
    fun t(key: String) = Strings.text(model.language, key)
    var name by remember(conversation.id) { mutableStateOf(conversation.title) }
    var memberId by remember(conversation.id) { mutableStateOf("") }
    var announcement by remember(conversation.id) { mutableStateOf(false) }
    var governance by remember(conversation.id) { mutableStateOf(false) }
    if (governance) GroupScreen(model, conversation.id) { governance = false }
    LaunchedEffect(conversation.id) { model.loadMembers() }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(conversation.title) },
        text = {
            LazyColumn(modifier = Modifier.fillMaxWidth().heightIn(max = 520.dp)) {
                if (conversation.readOnly) item { Text(t("readOnly")) }
                if (conversation.kind=="group" && !model.demo) item { TextButton(onClick={governance=true},enabled=!model.busy){Text(GroupText.text(model.language,"title"))} }
                if (
                    !model.demo &&
                        conversation.kind == "group" &&
                        model.features["announcements"] == true
                )
                    item {
                        TextButton(onClick = { announcement = true }, enabled = !model.busy) {
                            Text(t("announcement"))
                        }
                    }
                if (
                    conversation.kind == "group" &&
                        conversation.ownerId == model.userId &&
                        !conversation.readOnly
                ) {
                    item {
                        OutlinedTextField(name, { name = it }, label = { Text(t("title")) })
                        TextButton(
                            onClick = { model.groupChange("rename", name) },
                            enabled = name.isNotBlank() && !model.busy,
                        ) {
                            Text(t("rename"))
                        }
                    }
                    item {
                        OutlinedTextField(
                            memberId,
                            { memberId = it },
                            label = { Text(t("addMember")) },
                        )
                        TextButton(
                            onClick = { model.groupChange("add", memberId) },
                            enabled = memberId.isNotBlank() && !model.busy,
                        ) {
                            Text(t("addMember"))
                        }
                    }
                }
                items(model.members, key = { it.id }) { person ->
                    Column {
                        ListItem(
                            headlineContent = { Text(person.name) },
                            supportingContent = {
                                Text(
                                    if (person.id == conversation.ownerId) t("owner")
                                    else person.username
                                )
                            },
                            leadingContent = { ProfileAvatar(person, model) },
                        )
                        if (
                            conversation.kind == "group" &&
                                conversation.ownerId == model.userId &&
                                person.id != model.userId &&
                                !conversation.readOnly
                        )
                            Row {
                                TextButton(
                                    onClick = { governance = true },
                                    enabled = !model.busy,
                                ) {
                                    Text(t("transfer"))
                                }
                                TextButton(
                                    onClick = { model.groupChange("remove", member = person) },
                                    enabled = !model.busy,
                                ) {
                                    Text(t("removeMember"))
                                }
                            }
                    }
                }
                if (model.members.isEmpty()) item { EmptyPanel(model) { model.loadMembers() } }
                if (model.membersNext.isNotEmpty()) item { TextButton(onClick={model.loadMembers(true)},enabled=!model.busy){Text(t("loadMore"))} }
                if (
                    conversation.kind == "group" &&
                        conversation.ownerId != model.userId &&
                        !conversation.readOnly
                )
                    item {
                        TextButton(
                            onClick = {
                                model.groupChange("leave")
                                onDismiss()
                            },
                            enabled = !model.busy,
                        ) {
                            Text(t("leave"))
                        }
                    }
                item { ReportForm("conversation", conversation.id, model) }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(t("close")) } },
    )
    if (announcement)
        AnnouncementDialog(model) {
            announcement = false
            model.announcement.reset()
            onDismiss()
        }
}

@Composable
fun ReportForm(type: String, target: String, model: ChatModel) {
    if (model.demo || model.features["reports"] != true) return
    val pending = model.pendingReport
    val original = pending?.let { org.json.JSONObject(it.payload) }
    val same =
        original == null ||
            (original.optString("targetId") == target && original.optString("targetType") == type)
    var reason by
        remember(type, target, pending?.requestId) {
            mutableStateOf(if (same) original?.optString("reason").orEmpty() else "")
        }
    OutlinedTextField(
        reason,
        { reason = it },
        label = { Text(Strings.text(model.language, "reportReason")) },
        maxLines = 4,
        enabled = pending == null,
        modifier = Modifier.fillMaxWidth(),
    )
    TextButton(
        onClick = { model.report(type, target, reason) },
        enabled = reason.isNotBlank() && same && !model.busy,
    ) {
        Text(Strings.text(model.language, "report"))
    }
}

@Composable
fun AccountTools(model: ChatModel) {
    fun t(key: String) = Strings.text(model.language, key)
    var panel by remember { mutableStateOf("") }
    var name by remember(model.me?.name) { mutableStateOf(model.me?.name.orEmpty()) }
    val context = LocalContext.current
    val avatar =
        rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
            uri?.let { model.publishAvatar(context, it) }
        }
    val permission =
        rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
            model.setAlerts(granted)
        }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
        if (!model.demo && model.token.isNotBlank()) {
            ProfileAvatar(model.me ?: Person(model.userId, model.username, model.username), model)
            OutlinedTextField(
                name,
                { name = it },
                label = { Text(t("displayName")) },
                modifier = Modifier.fillMaxWidth(),
            )
            TextButton(
                onClick = { model.updateProfile(name) },
                enabled = name.isNotBlank() && !model.busy,
            ) {
                Text(t("save"))
            }
            if (model.features["avatars"] == true) {
                TextButton(
                    onClick = { avatar.launch(arrayOf("image/jpeg", "image/png")) },
                    enabled = !model.busy,
                ) {
                    Text(t("avatar"))
                }
                if (model.avatarPending)
                    Row {
                        TextButton(onClick = { model.inspectAvatar() }, enabled = !model.busy) {
                            Text(t("refresh"))
                        }
                        TextButton(
                            onClick = { model.publishAvatar(context) },
                            enabled = !model.busy,
                        ) {
                            Text(t("retryAvatar"))
                        }
                    }
            }
            TextButton(
                onClick = {
                    model.loadDevices()
                    panel = "devices"
                },
                enabled = !model.busy,
            ) {
                Text(t("devices"))
            }
            TextButton(
                onClick = {
                    model.loadReports()
                    panel = "reports"
                },
                enabled = !model.busy,
            ) {
                Text(t("myReports"))
            }
        }
        Text(t("privacyNotice"), style = MaterialTheme.typography.bodySmall)
        val enabled = model.alertsEnabled
        Row(verticalAlignment = Alignment.CenterVertically) {
            Switch(
                enabled,
                { value ->
                    if (value && android.os.Build.VERSION.SDK_INT >= 33)
                        permission.launch(android.Manifest.permission.POST_NOTIFICATIONS)
                    else model.setAlerts(value)
                },
            )
            Text(t("alerts"))
        }
    }
    if (panel.isNotEmpty())
        AlertDialog(
            onDismissRequest = { panel = "" },
            title = { Text(t(if (panel == "devices") "devices" else "myReports")) },
            text = {
                LazyColumn(Modifier.heightIn(max = 460.dp)) {
                    if (panel == "devices") {
                        items(model.devices, key = { it.id }) { device ->
                            ListItem(
                                headlineContent = { Text(device.name) },
                                supportingContent = { Text(device.id.take(12)) },
                                trailingContent = {
                                    TextButton(
                                        onClick = { model.revokeDevice(device.id) },
                                        enabled = !model.busy,
                                    ) {
                                        Text(t("revoke"))
                                    }
                                },
                            )
                        }
                        if (model.devices.isEmpty())
                            item { EmptyPanel(model) { model.loadDevices() } }
                    } else {
                        model.pendingReport?.let { pending ->
                            item {
                                val original = org.json.JSONObject(pending.payload)
                                Text(t("pending"))
                                ReportForm(
                                    original.getString("targetType"),
                                    original.getString("targetId"),
                                    model,
                                )
                            }
                        }
                        items(model.reports, key = { it.id }) { report ->
                            ListItem(
                                headlineContent = { Text(report.reason) },
                                supportingContent = {
                                    Text(t(if (report.state == "open") "pending" else report.state))
                                },
                            )
                        }
                        if (model.reports.isEmpty())
                            item { EmptyPanel(model) { model.loadReports() } }
                    }
                }
            },
            confirmButton = { TextButton(onClick = { panel = "" }) { Text(t("close")) } },
        )
}

@Composable
fun EmptyPanel(model: ChatModel, reload: () -> Unit = { model.refresh() }) {
    Column(
        Modifier.fillMaxWidth().padding(28.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(Strings.text(model.language, "empty"), style = MaterialTheme.typography.titleMedium)
        Text(Strings.text(model.language, "emptyHelp"))
        TextButton(onClick = reload, enabled = !model.busy) {
            Text(Strings.text(model.language, "refresh"))
        }
    }
}

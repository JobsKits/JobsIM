package com.jobs.im

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp

@Composable
fun MentionsDialog(model: ChatModel, onDismiss: () -> Unit) {
    val group = model.selected ?: return
    fun t(key: String) = Strings.text(model.language, key)
    fun update(ids: List<String>, all: Boolean) {
        runCatching { model.setDraftMentions(ids, all) }
            .onFailure { model.error = t("cloudUnavailable") }
    }
    LaunchedEffect(group.id) { model.loadMembers() }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(t("mentions")) },
        text = {
            LazyColumn(Modifier.fillMaxWidth().heightIn(max = 480.dp)) {
                if (model.busy) item { LinearProgressIndicator(Modifier.fillMaxWidth()) }
                if (
                    ((group.ownerId == model.userId && model.features["role_admission"] == true) ||
                        model.draftMentionAll) && !group.readOnly
                )
                    item {
                        Row(
                            Modifier.fillMaxWidth()
                                .heightIn(min = 48.dp)
                                .toggleable(
                                    value = model.draftMentionAll,
                                    enabled = !model.busy,
                                    role = Role.Checkbox,
                                    onValueChange = { update(emptyList(), it) },
                                ),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Checkbox(
                                model.draftMentionAll,
                                null,
                                enabled = !model.busy,
                            )
                            Text(t("mentionAll"))
                        }
                    }
                val members = model.members.filter { it.id != model.userId && it.state == "active" }
                items(members, key = { it.id }) { person ->
                    val checked = person.id in model.draftMentionIds
                    val enabled =
                        !model.busy &&
                            !group.readOnly &&
                            (checked || model.draftMentionIds.size < 20)
                    Row(
                        Modifier.fillMaxWidth()
                            .heightIn(min = 48.dp)
                            .toggleable(
                                value = checked,
                                enabled = enabled,
                                role = Role.Checkbox,
                                onValueChange = { value ->
                                    update(
                                        if (value) model.draftMentionIds + person.id
                                        else model.draftMentionIds - person.id,
                                        false,
                                    )
                                },
                            ),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Checkbox(
                            person.id in model.draftMentionIds,
                            null,
                            enabled =
                                !model.busy &&
                                    !group.readOnly &&
                                    (person.id in model.draftMentionIds ||
                                        model.draftMentionIds.size < 20),
                        )
                        Text(person.name.ifBlank { t("unavailable") })
                    }
                }
                if (members.isEmpty() && !model.busy)
                    item { EmptyPanel(model) { model.loadMembers() } }
                item {
                    TextButton(
                        onClick = { update(emptyList(), false) },
                        enabled = !model.busy,
                    ) {
                        Text(t("clear"))
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(t("done")) } },
        dismissButton = {
            TextButton(onClick = { model.loadMembers() }, enabled = !model.busy) {
                Text(t("refresh"))
            }
        },
    )
}

@Composable
fun AnnouncementDialog(model: ChatModel, initialSource: MessageRow? = null, onDismiss: () -> Unit) {
    val group = remember { model.selected } ?: return
    val state = model.announcement
    fun t(key: String) = Strings.text(model.language, key)
    var expiryDays by remember { mutableIntStateOf(0) }
    var expiryMenu by remember { mutableStateOf(false) }
    var sourceQuery by remember { mutableStateOf("") }
    LaunchedEffect(group.id) {
        state.load(group).join()
        if (initialSource != null) state.choose(initialSource)
    }
    val canCompose =
        state.roleProtected &&
            state.current?.canManage == true &&
            (state.pending == null || state.terminalPending) &&
            !state.loading
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(t("announcement")) },
        text = {
            LazyColumn(
                Modifier.fillMaxWidth().heightIn(max = 520.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                if (state.loading) item { LinearProgressIndicator(Modifier.fillMaxWidth()) }
                if (state.problem.isNotEmpty())
                    item { Text(t(state.problem), color = MaterialTheme.colorScheme.error) }
                item {
                    val current = state.current
                    val message = current?.source
                    if (message != null) {
                        Surface(
                            color = MaterialTheme.colorScheme.secondaryContainer,
                            shape = MaterialTheme.shapes.medium,
                        ) {
                            Column(Modifier.fillMaxWidth().padding(16.dp)) {
                                Text(message.text, style = MaterialTheme.typography.bodyLarge)
                                if (current.expiresAt.isNotEmpty())
                                    Text(
                                        t("expiry") +
                                            " · " +
                                            runCatching {
                                                    java.text.DateFormat.getDateTimeInstance(
                                                            java.text.DateFormat.MEDIUM,
                                                            java.text.DateFormat.SHORT,
                                                            java.util.Locale.forLanguageTag(
                                                                model.language
                                                            ),
                                                        )
                                                        .format(
                                                            java.util.Date.from(
                                                                java.time.Instant.parse(
                                                                    current.expiresAt
                                                                )
                                                            )
                                                        )
                                                }
                                                .getOrDefault(t("unavailable")),
                                        style = MaterialTheme.typography.bodySmall,
                                    )
                                TextButton(
                                    onClick = { model.openSource(message, onDismiss) },
                                    enabled = !model.busy,
                                ) {
                                    Text(t("viewOriginal"))
                                }
                            }
                        }
                    } else {
                        Text(
                            t(
                                when (current?.state) {
                                    "unavailable" -> "announcementUnavailable"
                                    "expired" -> "announcementExpired"
                                    else -> "announcementEmpty"
                                }
                            ),
                            style = MaterialTheme.typography.bodyLarge,
                        )
                        TextButton(onClick = { state.reload() }, enabled = !state.loading) {
                            Text(t("refresh"))
                        }
                    }
                }
                if (state.pending != null)
                    item {
                        Text(t("pendingAnnouncement"), style = MaterialTheme.typography.bodySmall)
                        Row {
                            TextButton(onClick = { state.inspect() }, enabled = !state.loading) {
                                Text(t("refresh"))
                            }
                            if (!state.terminalPending)
                                TextButton(onClick = { state.retry() }, enabled = !state.loading) {
                                    Text(t("retry"))
                                }
                        }
                    }
                if (state.roleProtected && state.current?.canManage == true) {
                    item {
                        Text(t("chooseMessage"), style = MaterialTheme.typography.titleSmall)
                        OutlinedTextField(
                            sourceQuery,
                            { sourceQuery = it },
                            label = { Text(t("search")) },
                            modifier = Modifier.fillMaxWidth(),
                            enabled = canCompose,
                            singleLine = true,
                        )
                    }
                    val candidates =
                        model.messages.filter {
                            it.state == "received" &&
                                !it.retracted &&
                                !it.contentUnavailable &&
                                !it.forwardUnavailable &&
                                (sourceQuery.isBlank() || it.text.contains(sourceQuery, true))
                        }
                    items(candidates, key = { it.key }) { row ->
                        ListItem(
                            headlineContent = { Text(row.text, maxLines = 2) },
                            trailingContent = {
                                RadioButton(
                                    state.source?.id == row.id,
                                    onClick = { state.choose(row) },
                                    enabled = canCompose,
                                )
                            },
                            modifier =
                                Modifier.clickable(enabled = canCompose) { state.choose(row) },
                        )
                    }
                    if (candidates.isEmpty()) item { EmptyPanel(model) }
                    state.source?.let { source ->
                        item {
                            Text(source.text, style = MaterialTheme.typography.bodyMedium)
                            Box {
                                TextButton(onClick = { expiryMenu = true }, enabled = canCompose) {
                                    Text(
                                        t(
                                            when (expiryDays) {
                                                1 -> "oneDay"
                                                7 -> "sevenDays"
                                                else -> "noExpiry"
                                            }
                                        )
                                    )
                                    Icon(Icons.Default.ArrowDropDown, null)
                                }
                                DropdownMenu(
                                    expanded = expiryMenu,
                                    onDismissRequest = { expiryMenu = false },
                                ) {
                                    listOf(0 to "noExpiry", 1 to "oneDay", 7 to "sevenDays")
                                        .forEach { (days, key) ->
                                            DropdownMenuItem(
                                                text = { Text(t(key)) },
                                                onClick = {
                                                    expiryDays = days
                                                    expiryMenu = false
                                                },
                                            )
                                        }
                                }
                            }
                            TextButton(
                                onClick = { state.publish(expiryDays) },
                                enabled = canCompose,
                            ) {
                                Text(t("publishAnnouncement"))
                            }
                        }
                    }
                    if (state.current?.state != "empty")
                        item {
                            TextButton(onClick = { state.clear() }, enabled = canCompose) {
                                Text(t("clearAnnouncement"))
                            }
                        }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(t("close")) } },
        dismissButton = {
            TextButton(onClick = { state.reload() }, enabled = !state.loading) {
                Text(t("refresh"))
            }
        },
    )
}

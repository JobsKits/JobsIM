package com.jobs.im

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

@Composable
fun DiscoveryDialog(kind: String, model: ChatModel, onDismiss: () -> Unit) {
    fun t(key: String) = Strings.text(model.language, key)
    val currentConversation = remember { model.selected }
    var query by remember { mutableStateOf("") }
    var onlyCurrent by remember { mutableStateOf(currentConversation != null) }
    val state = model.discovery
    fun filter() = if (onlyCurrent) currentConversation?.id.orEmpty() else ""
    LaunchedEffect(kind) { state.start(kind, query, filter()) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(t(if (kind == "favorites") "favoritesList" else "search")) },
        text = {
            Column(Modifier.fillMaxWidth()) {
                if (kind == "search") {
                    OutlinedTextField(
                        query,
                        { query = it },
                        label = { Text(t("search")) },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                    )
                    TextButton(
                        onClick = { state.start(kind, query, filter()) },
                        enabled = !state.loading && query.isNotBlank(),
                    ) {
                        Text(t("search"))
                    }
                }
                if (currentConversation != null)
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(
                            checked = onlyCurrent,
                            onCheckedChange = {
                                onlyCurrent = it
                                state.start(kind, query, filter())
                            },
                        )
                        Text(t("conversationSearch"))
                    }
                if (state.loading) LinearProgressIndicator(Modifier.fillMaxWidth())
                if (state.problem.isNotEmpty())
                    Text(t(state.problem), color = MaterialTheme.colorScheme.error)
                if (model.error.isNotEmpty())
                    Text(model.error, color = MaterialTheme.colorScheme.error)
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 480.dp)) {
                    items(state.items, key = { it.key }) { row ->
                        ListItem(
                            headlineContent = {
                                Text(
                                    if (row.forwardUnavailable) t("forwardUnavailable")
                                    else if (row.contentUnavailable) t("sourceUnavailable")
                                    else if (row.retracted) t("retracted") else row.text,
                                    maxLines = 3,
                                )
                            },
                            supportingContent = {
                                Text(t("viewOriginal") + " · " + row.conversationId.take(8))
                            },
                            modifier =
                                Modifier.clickable(
                                    enabled =
                                        !model.busy &&
                                            !row.contentUnavailable &&
                                            !row.forwardUnavailable
                                ) {
                                    model.openSource(row, onDismiss)
                                },
                        )
                    }
                    if (state.items.isEmpty() && !state.loading)
                        item {
                            Column(
                                Modifier.fillMaxWidth().padding(20.dp),
                                horizontalAlignment = Alignment.CenterHorizontally,
                            ) {
                                Text(t("empty"), style = MaterialTheme.typography.titleMedium)
                                Text(t("emptyHelp"), style = MaterialTheme.typography.bodySmall)
                                TextButton(onClick = { state.reload() }) { Text(t("refresh")) }
                            }
                        }
                    if (state.hasMore)
                        item {
                            TextButton(
                                onClick = { state.more() },
                                enabled =
                                    !state.loading && (kind != "search" || query == state.query),
                            ) {
                                Text(t("loadMore"))
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

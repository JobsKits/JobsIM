package com.jobs.im

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

@Composable
fun ForwardDialog(model: ChatModel) {
    val source = model.forwardSource ?: return
    fun t(key: String) = Strings.text(model.language, key)
    var filter by remember(source.id) { mutableStateOf("") }
    var target by remember(source.id) { mutableStateOf<Conversation?>(null) }
    val destinations =
        model.conversations.filter { !it.readOnly && it.title.contains(filter, true) }
    AlertDialog(
        onDismissRequest = { if (!model.busy) model.clearForwardSelection() },
        title = { Text(t("forward")) },
        text = {
            LazyColumn(
                Modifier.fillMaxWidth().heightIn(max = 500.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                item {
                    Surface(
                        color = MaterialTheme.colorScheme.surfaceContainer,
                        shape = MaterialTheme.shapes.medium,
                    ) {
                        Text(
                            source.text.ifBlank { t("attachment") + " · " + source.type },
                            Modifier.padding(12.dp),
                        )
                    }
                }
                if (model.forwardNeedsReselect)
                    item {
                        Text(t("forwardChanged"))
                        TextButton(onClick = { model.acceptUpdatedForwardSource() }) {
                            Text(t("forwardReselect"))
                        }
                    }
                else {
                    item {
                        OutlinedTextField(
                            filter,
                            { filter = it },
                            label = { Text(t("forwardTarget")) },
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                    if (destinations.isEmpty()) item { EmptyPanel(model) { model.refresh() } }
                    items(destinations, key = { it.id }) { candidate ->
                        ListItem(
                            headlineContent = { Text(candidate.title) },
                            trailingContent = {
                                RadioButton(
                                    target?.id == candidate.id,
                                    onClick = { target = candidate },
                                )
                            },
                            modifier = Modifier.clickable { target = candidate },
                        )
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { target?.let { model.sendForward(source, it) } },
                enabled = target != null && !model.forwardNeedsReselect && !model.busy,
            ) {
                Text(t("forward"))
            }
        },
        dismissButton = {
            TextButton(onClick = { model.clearForwardSelection() }, enabled = !model.busy) {
                Text(t("cancel"))
            }
        },
    )
}

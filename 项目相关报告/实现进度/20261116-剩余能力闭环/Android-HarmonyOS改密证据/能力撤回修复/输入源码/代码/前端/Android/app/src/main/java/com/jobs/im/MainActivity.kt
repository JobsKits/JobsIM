package com.jobs.im

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.core.view.WindowCompat
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.collectLatest

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { JobsIM() }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobsIM(model: ChatModel = viewModel()) {
    val dark =
        when (model.theme) {
            "dark" -> true
            "light" -> false
            else -> isSystemInDarkTheme()
        }
    val colors =
        if (dark)
            darkColorScheme(
                primary = Color(0xFF83CFB6),
                onPrimary = Color(0xFF102B20),
                primaryContainer = Color(0xFF253A31),
                onPrimaryContainer = Color(0xFFEAF0EB),
                onSecondaryContainer = Color(0xFFEAF0EB),
                secondary = Color(0xFF83CFB6),
                onSecondary = Color(0xFF102B20),
                secondaryContainer = Color(0xFF253A31),
                surfaceContainer = Color(0xFF1A241F),
                surfaceContainerHigh = Color(0xFF233129),
                surfaceContainerHighest = Color(0xFF2D3C33),
                surfaceContainerLow = Color(0xFF171F1A),
                surfaceContainerLowest = Color(0xFF0E1410),
                background = Color(0xFF121615),
                surface = Color(0xFF1A201D),
                onSurface = Color(0xFFEAF0EB),
                onSurfaceVariant = Color(0xFFBFCBC3),
                outline = Color(0xFF8D9A91),
                outlineVariant = Color(0xFF46554B),
            )
        else
            lightColorScheme(
                primary = Color(0xFF287C66),
                onPrimary = Color.White,
                primaryContainer = Color(0xFFDCEDE4),
                onPrimaryContainer = Color(0xFF18251F),
                onSecondaryContainer = Color(0xFF18251F),
                background = Color(0xFFFAF9F5),
                surface = Color(0xFFFAF9F5),
                onSurface = Color(0xFF18251F),
                onSurfaceVariant = Color(0xFF46564C),
                outline = Color(0xFF728277),
                outlineVariant = Color(0xFFC3CEC4),
                secondaryContainer = Color(0xFFDCEDE4),
                surfaceContainer = Color(0xFFF0F2EB),
                surfaceContainerHigh = Color(0xFFEEF1E9),
                surfaceContainerHighest = Color(0xFFE7ECE3),
                surfaceContainerLow = Color(0xFFF4F5EF),
                surfaceContainerLowest = Color(0xFFFFFFFF),
                secondary = Color(0xFF287C66),
                onSecondary = Color.White,
            )
    val activity = LocalContext.current as? ComponentActivity
    SideEffect {
        activity?.let {
            WindowCompat.getInsetsController(it.window, it.window.decorView).apply {
                isAppearanceLightStatusBars = !dark
                isAppearanceLightNavigationBars = !dark
            }
        }
    }
    DisposableEffect(activity) {
        val observer =
            androidx.lifecycle.LifecycleEventObserver { _, event ->
                if (event == androidx.lifecycle.Lifecycle.Event.ON_START) model.onForeground(true)
                else if (event == androidx.lifecycle.Lifecycle.Event.ON_STOP)
                    model.onForeground(false)
            }
        activity?.lifecycle?.addObserver(observer)
        onDispose { activity?.lifecycle?.removeObserver(observer) }
    }
    val density = LocalDensity.current
    CompositionLocalProvider(
        LocalDensity provides Density(density.density, density.fontScale * model.font)
    ) {
        MaterialTheme(colorScheme = colors) { Surface(Modifier.fillMaxSize()) { Home(model) } }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun Home(model: ChatModel) {
    fun t(key: String) = Strings.text(model.language, key)
    var tab by remember { mutableIntStateOf(0) }
    var connection by remember { mutableStateOf(false) }
    var newChat by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }
    val input = model.draft
    var person by remember { mutableStateOf<Person?>(null) }
    var details by remember { mutableStateOf(false) }
    var edit by remember { mutableStateOf<MessageRow?>(null) }
    var editing by remember { mutableStateOf("") }
    var lookup by remember { mutableStateOf<String?>(null) }
    var mentions by remember { mutableStateOf(false) }
    var announcementSource by remember { mutableStateOf<MessageRow?>(null) }
    val messageList = rememberLazyListState()
    val selected = model.selected
    val visibleMessages = model.messages.filter { query.isBlank() || it.text.contains(query, true) }
    LaunchedEffect(selected?.id, model.selfObservationRevision, model.selfState.enabled) {
        snapshotFlow { messageList.layoutInfo.visibleItemsInfo.map { it.key.toString() }.toSet() }
            .collectLatest { keys ->
                kotlinx.coroutines.delay(350)
                model.observeVisible(keys)
            }
    }
    LaunchedEffect(model.focusRequest) { if (model.focusRequest.isNotEmpty()) query = "" }
    LaunchedEffect(model.focusRequest, visibleMessages.map { it.key }) {
        val target = model.focusRequest
        val index = visibleMessages.indexOfFirst { it.id == target }
        if (target.isNotEmpty() && index >= 0) {
            messageList.scrollToItem(index)
            model.consumeFocus(target)
        }
    }
    BackHandler(enabled = selected != null) { model.closeConversation() }
    Scaffold(
        topBar = {
            TopAppBar(
                colors =
                    TopAppBarDefaults.topAppBarColors(
                        containerColor = MaterialTheme.colorScheme.surface,
                        scrolledContainerColor = MaterialTheme.colorScheme.surface,
                        titleContentColor = MaterialTheme.colorScheme.onSurface,
                        navigationIconContentColor = MaterialTheme.colorScheme.onSurface,
                        actionIconContentColor = MaterialTheme.colorScheme.primary,
                    ),
                title = {
                    Column {
                        Text(
                            selected?.title ?: if (tab == 2) t("settings") else "JobsIM",
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        Text(
                            if (model.demo) t("demo")
                            else if (model.deployment.isBlank()) t("private") else model.url,
                            style = MaterialTheme.typography.labelSmall,
                            maxLines = 1,
                        )
                    }
                },
                navigationIcon = {
                    if (selected != null)
                        IconButton(onClick = { model.closeConversation() }) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, t("back"))
                        }
                },
                actions = {
                    if (model.userId.isNotBlank() && !model.demo) {
                        IconButton(onClick = { lookup = "search" }, enabled = !model.busy) {
                            Icon(Icons.Default.Search, t("search"))
                        }
                        IconButton(onClick = { lookup = "favorites" }, enabled = !model.busy) {
                            Icon(Icons.Default.Favorite, t("favoritesList"))
                        }
                    }
                    if (selected != null)
                        TextButton(onClick = { details = true }, enabled = !model.busy) {
                            Text(t("details"))
                        }
                    else
                        TextButton(onClick = { connection = true }, enabled = !model.busy) {
                            Text(t("connect"))
                        }
                },
            )
        },
        bottomBar = {
            if (selected == null)
                NavigationBar(
                    containerColor = MaterialTheme.colorScheme.surfaceContainer,
                    contentColor = MaterialTheme.colorScheme.onSurface,
                ) {
                    listOf("chats", "contacts", "settings").forEachIndexed { index, key ->
                        NavigationBarItem(
                            colors =
                                NavigationBarItemDefaults.colors(
                                    selectedIconColor =
                                        MaterialTheme.colorScheme.onSecondaryContainer,
                                    selectedTextColor = MaterialTheme.colorScheme.primary,
                                    indicatorColor = MaterialTheme.colorScheme.secondaryContainer,
                                    unselectedIconColor =
                                        MaterialTheme.colorScheme.onSurfaceVariant,
                                    unselectedTextColor =
                                        MaterialTheme.colorScheme.onSurfaceVariant,
                                ),
                            selected = tab == index,
                            onClick = { tab = index },
                            icon = {
                                Icon(
                                    when (index) {
                                        0 -> Icons.Default.Email
                                        1 -> Icons.Default.AccountBox
                                        else -> Icons.Default.Settings
                                    },
                                    null,
                                )
                            },
                            label = { Text(t(key)) },
                        )
                    }
                }
        },
        floatingActionButton = {
            if (selected == null && tab == 0 && model.userId.isNotBlank())
                FloatingActionButton(onClick = { newChat = true }) {
                    Icon(Icons.Default.Add, t("newChat"))
                }
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize().imePadding()) {
            if (model.busy) LinearProgressIndicator(Modifier.fillMaxWidth())
            if (model.demo || model.offline)
                Surface(color = MaterialTheme.colorScheme.secondaryContainer) {
                    Text(
                        if (model.demo) t("demoNote") else t("offline"),
                        Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 10.dp),
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
            if (model.error.isNotEmpty())
                Text(
                    model.error,
                    Modifier.padding(horizontal = 20.dp, vertical = 8.dp),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                )
            if (selected?.readOnly == true) Text(t("readOnly"), Modifier.padding(16.dp))
            if (model.info.isNotEmpty())
                Text(
                    model.info,
                    Modifier.padding(horizontal = 20.dp, vertical = 4.dp),
                    style = MaterialTheme.typography.bodySmall,
                )
            when {
                tab == 2 && selected == null -> Settings(model)
                model.userId.isEmpty() -> Authentication(model, onConnect = { connection = true })
                selected != null -> {
                    SelfStateActions(model, selected)
                    OutlinedTextField(
                        query,
                        { query = it },
                        label = { Text(t("search")) },
                        modifier =
                            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
                        singleLine = true,
                    )
                    LazyColumn(
                        Modifier.weight(1f).fillMaxWidth(),
                        state = messageList,
                        contentPadding = PaddingValues(16.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        val visible = visibleMessages
                        if (visible.isEmpty()) item { Empty(model) }
                        items(visible, key = { it.key }) { message ->
                            MessageBubble(
                                message,
                                model,
                                highlighted = model.focusedMessage == message.id,
                                onAnnouncement = { announcementSource = it },
                                onEdit = {
                                    edit = message
                                    editing = message.text
                                },
                            )
                        }
                    }
                    if (model.reply != null)
                        Row(
                            Modifier.padding(horizontal = 16.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(
                                t("reply") +
                                    ": " +
                                    (if (model.reply!!.contentUnavailable) t("unavailable")
                                    else model.reply!!.text.take(60)),
                                Modifier.weight(1f),
                                style = MaterialTheme.typography.labelMedium,
                            )
                            IconButton(onClick = { model.reply = null }) {
                                Icon(Icons.Default.Close, t("cancel"))
                            }
                        }
                    if (
                        selected.kind == "group" &&
                            !model.demo &&
                            model.features["mentions"] == true
                    ) {
                        Row(
                            Modifier.fillMaxWidth().padding(horizontal = 12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            TextButton(
                                onClick = { mentions = true },
                                enabled = !model.busy && !selected.readOnly && model.draftReady,
                            ) {
                                Text(t("mentions"))
                            }
                            if (model.draftMentionAll || model.draftMentionIds.isNotEmpty())
                                Text(
                                    if (model.draftMentionAll) t("mentionAll")
                                    else
                                        t("selectedMentions") +
                                            " · " +
                                            model.draftMentionIds.joinToString {
                                                model.profiles[it]?.name?.ifBlank {
                                                    t("unavailable")
                                                } ?: t("unavailable")
                                            },
                                    style = MaterialTheme.typography.labelSmall,
                                    modifier = Modifier.weight(1f),
                                )
                        }
                    }
                    Row(
                        Modifier.fillMaxWidth().padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        val context = LocalContext.current
                        val attachment =
                            rememberLauncherForActivityResult(
                                ActivityResultContracts.OpenDocument()
                            ) { uri ->
                                uri?.let { model.upload(context, it) }
                            }
                        IconButton(
                            onClick = { attachment.launch(arrayOf("*/*")) },
                            enabled =
                                !model.busy &&
                                    !selected.readOnly &&
                                    (model.demo || selected.sendAdmission.isNotBlank()) &&
                                    (model.demo || model.features["media"] == true),
                        ) {
                            Icon(Icons.Default.Add, t("attachment"))
                        }
                        OutlinedTextField(
                            input,
                            { model.updateDraft(it) },
                            label = { Text(t("message")) },
                            modifier = Modifier.weight(1f),
                            enabled = model.draftReady && !selected.readOnly,
                            maxLines = 5,
                        )
                        IconButton(
                            onClick = { model.send(input) },
                            enabled =
                                input.isNotBlank() &&
                                    !model.busy &&
                                    !selected.readOnly &&
                                    (model.demo || selected.sendAdmission.isNotBlank()),
                        ) {
                            Icon(Icons.AutoMirrored.Filled.Send, t("send"))
                        }
                    }
                }
                tab == 1 -> ContactsPanel(model) { person = it }
                else -> {
                    Row(
                        Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            t("chats"),
                            style = MaterialTheme.typography.headlineMedium,
                            fontWeight = FontWeight.SemiBold,
                            modifier = Modifier.weight(1f),
                        )
                        IconButton(onClick = { model.refresh() }, enabled = !model.busy) {
                            Icon(Icons.Default.Refresh, t("refresh"))
                        }
                    }
                    LazyColumn(
                        Modifier.fillMaxSize(),
                        contentPadding = PaddingValues(bottom = 88.dp),
                    ) {
                        if (model.selfState.enabled) item {
                            Column(Modifier.padding(horizontal = 20.dp)) {
                                FilterChip(model.showArchived, { model.showArchived = !model.showArchived }, label = { Text(t("showArchived")) })
                                FilterChip(model.showHidden, { model.showHidden = !model.showHidden }, label = { Text(t("showHidden")) })
                            }
                        }
                        val listed = model.listedConversations()
                        if (listed.isEmpty()) item { Empty(model) }
                        items(listed, key = { it.id }) { c ->
                            ListItem(
                                headlineContent = { Text(c.title, fontWeight = FontWeight.Medium) },
                                supportingContent = {
                                    Text(
                                        (if (c.kind == "group") t("group") else t("private")) +
                                            model.unreadDescription(c.id) +
                                            (if (model.selfState.views[c.id]?.opt("manualUnread") == true) " · " + t("manualUnread") else "")
                                    )
                                },
                                leadingContent = { Avatar(c.title) },
                                modifier =
                                    Modifier.clickable(enabled = !model.busy) { model.open(c) },
                            )
                            HorizontalDivider(
                                Modifier.padding(start = 76.dp),
                                color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = .4f),
                            )
                        }
                    }
                }
            }
        }
    }
    if (person != null) PersonDialog(person!!, model) { person = null }
    if (details && model.selected != null) GroupDialog(model) { details = false }
    LaunchedEffect(model.selected?.id) { details = false }
    if (connection) ConnectionDialog(model) { connection = false }
    if (newChat) {
        var name by remember { mutableStateOf("") }
        var members by remember { mutableStateOf("") }
        var group by remember { mutableStateOf(true) }
        AlertDialog(
            onDismissRequest = { newChat = false },
            title = { Text(t("newChat")) },
            text = {
                Column {
                    OutlinedTextField(name, { name = it }, label = { Text(t("title")) })
                    OutlinedTextField(members, { members = it }, label = { Text(t("members")) })
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(group, { group = it })
                        Text(t("group"))
                    }
                }
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        model.create(
                            name,
                            members.split(',').map { it.trim() }.filter { it.isNotBlank() },
                            group,
                        )
                        newChat = false
                    },
                    enabled = name.isNotBlank() && !model.busy,
                ) {
                    Text(t("create"))
                }
            },
            dismissButton = { TextButton(onClick = { newChat = false }) { Text(t("cancel")) } },
        )
    }
    if (edit != null)
        AlertDialog(
            onDismissRequest = { edit = null },
            title = { Text(t("edit")) },
            text = { OutlinedTextField(editing, { editing = it }) },
            confirmButton = {
                TextButton(
                    onClick = {
                        model.action(edit!!, "edit", editing)
                        edit = null
                    }
                ) {
                    Text(t("save"))
                }
            },
            dismissButton = { TextButton(onClick = { edit = null }) { Text(t("cancel")) } },
        )
    lookup?.let { kind ->
        DiscoveryDialog(kind, model) {
            lookup = null
            model.discovery.reset()
        }
    }
    if (mentions && selected?.kind == "group") MentionsDialog(model) { mentions = false }
    if (model.forwardSource != null) ForwardDialog(model)
    announcementSource?.let { source ->
        if (selected?.id == source.conversationId)
            AnnouncementDialog(model, source) {
                announcementSource = null
                model.announcement.reset()
            }
    }
    if (model.recoveryCodes.isNotEmpty())
        AlertDialog(
            onDismissRequest = { model.recoveryCodes = emptyList() },
            title = { Text(t("codes")) },
            text = {
                Column(Modifier.verticalScroll(rememberScrollState())) {
                    model.recoveryCodes.forEach { Text(it, Modifier.padding(8.dp)) }
                }
            },
            confirmButton = {
                TextButton(onClick = { model.recoveryCodes = emptyList() }) { Text(t("close")) }
            },
        )
}

@Composable
private fun Avatar(name: String) {
    Surface(
        color = MaterialTheme.colorScheme.secondaryContainer,
        shape = MaterialTheme.shapes.medium,
        modifier = Modifier.size(44.dp),
    ) {
        Box(contentAlignment = Alignment.Center) {
            Text(name.take(1).uppercase(), fontWeight = FontWeight.Medium)
        }
    }
}

@Composable
private fun Empty(model: ChatModel) {
    Column(
        Modifier.fillMaxWidth().padding(36.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(Strings.text(model.language, "empty"), style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(8.dp))
        Text(Strings.text(model.language, "emptyHelp"), style = MaterialTheme.typography.bodyMedium)
        TextButton(onClick = { model.refresh() }, enabled = !model.busy) {
            Text(Strings.text(model.language, "refresh"))
        }
    }
}

@Composable
private fun MessageBubble(
    row: MessageRow,
    model: ChatModel,
    highlighted: Boolean = false,
    onAnnouncement: ((MessageRow) -> Unit)? = null,
    onEdit: () -> Unit,
) {
    fun t(key: String) = Strings.text(model.language, key)
    var expanded by remember { mutableStateOf(false) }
    var report by remember { mutableStateOf(false) }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var exportedFile by remember { mutableStateOf<java.io.File?>(null) }
    val saveFile =
        rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { result
            ->
            val uri = result.data?.data
            val file = exportedFile
            val current = model.storageNamespace
            if (
                result.resultCode == android.app.Activity.RESULT_OK &&
                    uri != null &&
                    file != null &&
                    current == row.namespace
            ) {
                scope.launch {
                    try {
                        kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
                            context.contentResolver.openOutputStream(uri)?.use { output ->
                                file.inputStream().use { it.copyTo(output) }
                            } ?: error("OUTPUT_UNAVAILABLE")
                        }
                        model.info = t("saved")
                    } catch (_: Exception) {
                        model.error = t("failure")
                    }
                }
            }
        }

    Column(
        Modifier.fillMaxWidth(),
        horizontalAlignment = if (row.senderId == model.userId) Alignment.End else Alignment.Start,
    ) {
        Surface(
            border =
                if (highlighted) BorderStroke(2.dp, MaterialTheme.colorScheme.primary) else null,
            color =
                if (row.senderId == model.userId) MaterialTheme.colorScheme.secondaryContainer
                else MaterialTheme.colorScheme.surfaceContainer,
            shape = MaterialTheme.shapes.medium,
            modifier = Modifier.widthIn(max = 340.dp).clickable { expanded = !expanded },
        ) {
            Column(Modifier.padding(14.dp)) {
                if (highlighted)
                    Text(
                        t("originalMessage"),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                Text(
                    (if (row.senderId == model.userId) model.me?.name ?: model.username
                    else model.profiles[row.senderId]?.name) ?: row.senderId.take(8),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.primary,
                )
                if (
                    row.mentionedMe &&
                        !row.retracted &&
                        !row.contentUnavailable &&
                        !row.forwardUnavailable
                )
                    Text(
                        t("mentionedYou"),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.primary,
                    )
                if (!row.retracted && !row.contentUnavailable && !row.forwardUnavailable) {
                    val ids = runCatching { org.json.JSONArray(row.mentionedUserIds) }.getOrNull()
                    val targets =
                        ids?.let { values ->
                                (0 until values.length())
                                    .map { values.optString(it) }
                                    .filter { it.isNotEmpty() }
                            }
                            .orEmpty()
                    if (row.mentionAll || targets.isNotEmpty())
                        Text(
                            if (row.mentionAll) "@all · " + t("mentionAll")
                            else
                                targets.joinToString { id ->
                                    "@" +
                                        (if (id == model.userId) model.me?.name
                                            else model.profiles[id]?.name)
                                            .orEmpty()
                                            .ifBlank { t("unavailable") }
                                },
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )
                }
                if (row.replyToId.isNotBlank())
                    Text(
                        t("reply") + " · " + row.replyToId.take(8),
                        style = MaterialTheme.typography.labelSmall,
                    )
                if (row.forwarded)
                    Text(
                        t("forwarded"),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                Text(
                    if (row.retracted) t("retracted")
                    else if (row.forwardUnavailable) t("forwardUnavailable")
                    else if (row.contentUnavailable) t("contentUnavailable")
                    else if (row.type == "forward") t("forward")
                    else row.text.ifBlank { t("attachment") + " · " + row.type },
                    style = MaterialTheme.typography.bodyLarge,
                )
                Text(
                    java.text.DateFormat.getTimeInstance(
                            java.text.DateFormat.SHORT,
                            java.util.Locale.forLanguageTag(model.language),
                        )
                        .format(java.util.Date(row.createdAt)),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (row.pinned || row.favorite || row.reaction.isNotEmpty())
                    Text(
                        listOfNotNull(
                                if (row.pinned) t("pin") else null,
                                if (row.favorite) t("favorite") else null,
                                if (row.reaction.isNotEmpty()) t("react") else null,
                            )
                            .joinToString(" · "),
                        style = MaterialTheme.typography.labelSmall,
                    )
                if (row.state == "pending")
                    Column {
                        TextButton(onClick = { model.retry(row) }, enabled = !model.busy) {
                            Text(
                                if (row.type == "forward") t("forwardCheck")
                                else t("pending") + " · " + t("retry")
                            )
                        }
                        if (row.type == "forward" && row.id in model.missingForwardResults)
                            TextButton(
                                onClick = { model.retryOriginalForward(row) },
                                enabled = !model.busy,
                            ) {
                                Text(t("forwardRetry"))
                            }
                    }
                if (row.state == "frozen")
                    Text(t("cloudUnavailable"), style = MaterialTheme.typography.labelSmall)
            }
        }
        if (
            (row.contentUnavailable || row.forwardUnavailable) &&
                !row.retracted &&
                row.senderId == model.userId
        )
            TextButton(onClick = { model.action(row, "retract") }, enabled = !model.busy) {
                Text(t("retract"))
            }
        if (
            (row.attachmentId.isNotBlank() || row.attachmentURL.isNotBlank()) &&
                !row.retracted &&
                !row.contentUnavailable &&
                !row.forwardUnavailable &&
                !model.demo
        ) {
            val context = LocalContext.current
            TextButton(
                onClick = {
                    model.download(row) { file, mime ->
                        val uri =
                            androidx.core.content.FileProvider.getUriForFile(
                                context,
                                context.packageName + ".files",
                                file,
                            )
                        val intent =
                            android.content
                                .Intent(android.content.Intent.ACTION_VIEW)
                                .setDataAndType(uri, mime)
                                .addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION)
                        try {
                            context.startActivity(intent)
                        } catch (_: android.content.ActivityNotFoundException) {
                            model.info = t("download")
                        }
                    }
                },
                enabled = !model.busy,
            ) {
                Text(t("open"))
            }
        }
        if (
            (row.attachmentId.isNotBlank() || row.attachmentURL.isNotBlank()) &&
                !row.retracted &&
                !row.contentUnavailable &&
                !row.forwardUnavailable &&
                !model.demo
        )
            TextButton(
                onClick = {
                    model.download(row) { file, mime ->
                        exportedFile = file
                        saveFile.launch(
                            android.content
                                .Intent(android.content.Intent.ACTION_CREATE_DOCUMENT)
                                .addCategory(android.content.Intent.CATEGORY_OPENABLE)
                                .setType(mime.substringBefore(';'))
                                .putExtra(android.content.Intent.EXTRA_TITLE, row.text.take(120))
                        )
                    }
                },
                enabled = !model.busy,
            ) {
                Text(t("download"))
            }
        if (
            expanded &&
                row.state in listOf("received", "demo") &&
                !row.retracted &&
                !row.contentUnavailable &&
                !row.forwardUnavailable
        )
            Column {
                Row {
                    TextButton(onClick = { model.reply = row }) { Text(t("reply")) }
                    TextButton(onClick = { model.action(row, "reaction") }, enabled = !model.busy) {
                        Text(t("react"))
                    }
                    TextButton(onClick = { model.action(row, "favorite") }, enabled = !model.busy) {
                        Text(t("favorite"))
                    }
                }
                Row {
                    if (row.senderId == model.userId) {
                        if (!row.forwarded)
                            TextButton(onClick = onEdit, enabled = row.state != "pending") {
                                Text(t("edit"))
                            }
                        TextButton(
                            onClick = { model.action(row, "retract") },
                            enabled = !model.busy,
                        ) {
                            Text(t("retract"))
                        }
                    }
                    TextButton(onClick = { model.action(row, "pin") }, enabled = !model.busy) {
                        Text(t("pin"))
                    }
                    TextButton(onClick = { report = true }, enabled = !model.busy) {
                        Text(t("report"))
                    }
                }
                if (
                    !model.demo &&
                        model.features["forwarding"] == true &&
                        model.features["send_admission"] == true
                )
                    TextButton(onClick = { model.prepareForward(row) }, enabled = !model.busy) {
                        Text(t("forward"))
                    }
                if (
                    !model.demo &&
                        model.features["announcements"] == true &&
                        model.features["role_admission"] == true &&
                        model.selected?.kind == "group" &&
                        model.selected?.ownerId == model.userId &&
                        model.selected?.readOnly == false
                ) {
                    TextButton(
                        onClick = { onAnnouncement?.invoke(row) },
                        enabled = !model.busy && onAnnouncement != null,
                    ) {
                        Text(t("publishAnnouncement"))
                    }
                }
            }
        if (report)
            AlertDialog(
                onDismissRequest = { report = false },
                title = { Text(t("report")) },
                text = { ReportForm("message", row.id, model) },
                confirmButton = { TextButton(onClick = { report = false }) { Text(t("close")) } },
            )
    }
}

@Composable
private fun Authentication(model: ChatModel, onConnect: () -> Unit) {
    fun t(key: String) = Strings.text(model.language, key)
    var mode by remember(model.profile) { mutableStateOf("login") }
    var name by remember(model.profile) { mutableStateOf(model.pendingAuthentication?.username.orEmpty()) }
    var password by remember(model.profile) { mutableStateOf("") }
    var code by remember(model.profile) { mutableStateOf("") }
    var otp by remember(model.profile) { mutableStateOf("") }
    val pending = model.pendingAuthentication
    LaunchedEffect(pending?.requestId) {
        pending?.let { name = it.username; mode = if (it.purpose == "register") "register" else "login" }
    }
    Column(
        Modifier.fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 28.dp, vertical = 32.dp),
        verticalArrangement = Arrangement.spacedBy(18.dp),
    ) {
        Text(
            t("welcome"),
            style = MaterialTheme.typography.headlineLarge,
            fontWeight = FontWeight.SemiBold,
        )
        Text(t("connectionHelp"), style = MaterialTheme.typography.bodyLarge)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("login", "register", "recover").forEach { key ->
                FilterChip(
                    selected = mode == key,
                    onClick = { mode = key },
                    enabled = pending == null && !model.linkedAuthenticationRequired,
                    label = { Text(t(key)) },
                )
            }
        }
        if (pending != null || model.verificationRequired) {
            Surface(color = MaterialTheme.colorScheme.secondaryContainer,
                shape = MaterialTheme.shapes.medium) {
                Text(t(if (pending != null) "authUnknown" else "verifySession"), Modifier.padding(16.dp))
            }
        }
        OutlinedTextField(
            name,
            { name = it },
            enabled = pending == null,
            label = { Text(t("username")) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            password,
            { password = it },
            label = { Text(t(if (mode == "recover") "newPassword" else "password")) },
            visualTransformation = PasswordVisualTransformation(),
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        if (mode == "recover")
            OutlinedTextField(
                code,
                { code = it },
                label = { Text(t("code")) },
                modifier = Modifier.fillMaxWidth(),
            )
        if (mode != "recover")
            OutlinedTextField(
                otp,
                { otp = it },
                label = { Text(t("otp")) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
        Button(
            onClick = { model.authenticate(mode, name, password, code, otp) },
            enabled =
                !model.busy &&
                    name.isNotBlank() &&
                    password.isNotBlank() &&
                    model.deployment.isNotBlank() &&
                    pending?.deliveryExpired != true &&
                    (!model.linkedAuthenticationRequired || pending != null),
            modifier = Modifier.fillMaxWidth().heightIn(min = 50.dp),
        ) {
            Text(t(if (pending != null) "authRetryOriginal" else "continue"))
        }
        if (model.linkedAuthenticationRequired) {
            Text(t("authProofHelp"), style = MaterialTheme.typography.bodySmall)
            OutlinedButton(
                onClick = { model.authenticate("reauthenticate", name, password, otp = otp, replaceUnknown = true) },
                enabled = !model.busy && name.isNotBlank() && password.isNotBlank() && model.deployment.isNotBlank(),
                modifier = Modifier.fillMaxWidth().heightIn(min = 50.dp),
            ) { Text(t("authVerifyNew")) }
        }
        OutlinedButton(
            onClick = onConnect,
            modifier = Modifier.fillMaxWidth().heightIn(min = 50.dp),
        ) {
            Text(t("connect"))
        }
        TextButton(
            onClick = { model.preview() },
            enabled = !model.busy,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(t("demo"))
        }
    }
}

@Composable
private fun Settings(model: ChatModel) {
    fun t(key: String) = Strings.text(model.language, key)
    var password by remember { mutableStateOf("") }
    var exports by remember { mutableStateOf(false) }
    var passwordChange by remember { mutableStateOf(false) }
    if (exports) PersonalExportScreen(model) { exports = false }
    if (passwordChange) OwnPasswordScreen(model) { passwordChange = false }
    LazyColumn(
        contentPadding = PaddingValues(20.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        item {
            var expanded by remember { mutableStateOf(false) }
            Text(t("language"), style = MaterialTheme.typography.titleMedium)
            Box {
                OutlinedButton(onClick = { expanded = true }, modifier = Modifier.fillMaxWidth()) {
                    Text(
                        Strings.languages[model.language].orEmpty(),
                        modifier = Modifier.weight(1f),
                    )
                    Icon(
                        Icons.Default.ArrowDropDown,
                        null,
                        modifier = Modifier.padding(start = 16.dp),
                    )
                }
                DropdownMenu(
                    expanded = expanded,
                    onDismissRequest = { expanded = false },
                    modifier = Modifier.widthIn(min = 240.dp, max = 300.dp),
                ) {
                    Strings.languages.forEach { (key, name) ->
                        DropdownMenuItem(
                            text = { Text(name) },
                            onClick = {
                                model.setting("language", key)
                                expanded = false
                            },
                            modifier =
                                Modifier.background(
                                    if (model.language == key)
                                        MaterialTheme.colorScheme.secondaryContainer
                                    else MaterialTheme.colorScheme.surface
                                ),
                        )
                    }
                }
            }
        }
        item {
            HorizontalDivider()
            Text(t("theme"), style = MaterialTheme.typography.titleMedium)
            Row {
                listOf("system", "light", "dark").forEach { key ->
                    FilterChip(
                        model.theme == key,
                        { model.setting("theme", key) },
                        label = { Text(t(key)) },
                        modifier = Modifier.padding(end = 8.dp),
                    )
                }
            }
        }
        item {
            Text(t("font"), style = MaterialTheme.typography.titleMedium)
            Row {
                listOf("small" to .85f, "normal" to 1f, "large" to 1.2f).forEach { (key, value) ->
                    FilterChip(
                        model.font == value,
                        { model.fontScale(value) },
                        label = { Text(t(key)) },
                        modifier = Modifier.padding(end = 8.dp),
                    )
                }
            }
            SelfStateActions(model)
        }
        item { AccountTools(model) }
        item {
            OutlinedButton(onClick = { passwordChange = true }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(OwnPasswordText.text(model.language, "title")) }
        }
        item {
            OutlinedButton(onClick = { exports = true }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(t("exportTitle")) }
        }
        if (model.token.isNotBlank())
            item {
                OutlinedTextField(
                    password,
                    { password = it },
                    label = { Text(t("password")) },
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                TextButton(
                    onClick = {
                        model.codes(password)
                        password = ""
                    },
                    enabled = password.isNotEmpty() && !model.busy,
                ) {
                    Text(t("codes"))
                }
            }
        item {
            Text(model.url, style = MaterialTheme.typography.bodyMedium)
            Text(model.deployment, style = MaterialTheme.typography.labelSmall)
            OutlinedButton(onClick = { model.logout() }, enabled = !model.busy) {
                Text(t("logout"))
            }
        }
    }
}

@Composable
private fun ConnectionDialog(model: ChatModel, onDismiss: () -> Unit) {
    fun t(key: String) = Strings.text(model.language, key)
    var address by remember { mutableStateOf(model.url) }
    var expected by remember { mutableStateOf(model.deployment) }
    var ca by remember { mutableStateOf<ByteArray?>(model.caBytes) }
    var trust by remember { mutableStateOf(false) }
    val context = LocalContext.current
    val picker =
        rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
            uri?.let {
                try {
                    ca =
                        context.contentResolver.openInputStream(it)?.use { stream ->
                            readLimited(stream, 65536)
                        }
                } catch (_: Exception) {
                    ca = null
                    model.error = t("failure")
                }
                trust = false
            }
        }
    val fingerprint =
        remember(ca) {
            ca?.let { bytes ->
                try {
                    val certificate =
                        java.security.cert.CertificateFactory.getInstance("X.509")
                            .generateCertificate(bytes.inputStream())
                    java.security.MessageDigest.getInstance("SHA-256")
                        .digest(certificate.encoded)
                        .joinToString(":") { "%02X".format(it) }
                } catch (_: Exception) {
                    Strings.text(model.language, "failure")
                }
            }
        }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(t("connect")) },
        text = {
            Column(
                Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Text(t("connectionHelp"))
                OutlinedTextField(
                    address,
                    { address = it },
                    label = { Text(t("server")) },
                    singleLine = true,
                )
                OutlinedTextField(
                    expected,
                    { expected = it },
                    label = { Text(t("expected")) },
                    singleLine = true,
                )
                TextButton(
                    onClick = {
                        picker.launch(
                            arrayOf(
                                "application/x-x509-ca-cert",
                                "application/pkix-cert",
                                "application/octet-stream",
                            )
                        )
                    }
                ) {
                    Text(t("ca"))
                }
                if (ca != null) {
                    Text(
                        t("fingerprint") + "\n" + fingerprint,
                        style = MaterialTheme.typography.bodySmall,
                    )
                    Text(t("trustHelp"), style = MaterialTheme.typography.bodySmall)
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(trust, { trust = it })
                        Text(t("trust"))
                    }
                    TextButton(
                        onClick = {
                            ca = null
                            trust = false
                        }
                    ) {
                        Text(t("clear"))
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { model.connect(address, expected, ca, onDismiss) },
                enabled = !model.busy && expected.isNotBlank() && (ca == null || trust),
            ) {
                Text(t("connect"))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(t("cancel")) } },
    )
}

package com.jobs.im

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.LifecycleOwner
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import org.json.JSONObject

@Composable
fun ReceiptScreen(model:ChatModel,onClose:()->Unit) {
    fun t(key:String)=ReceiptText.text(model.language,key)
    fun m(key:String)=MigrationText.text(model.language,key)
    val appContext=LocalContext.current
    val coroutine=rememberCoroutineScope();val clipboard=LocalClipboardManager.current
    var active by remember{mutableStateOf(true)};var visible by remember{mutableStateOf(true)};var revision by remember{mutableIntStateOf(0)}
    var service by remember{mutableStateOf<ReceiptService?>(null)};var busy by remember{mutableStateOf(false)};var notice by remember{mutableStateOf("")}
    var context by remember{mutableStateOf("")};var report by remember{mutableStateOf("")};var profiles by remember{mutableStateOf<List<String>>(emptyList())}
    var rows by remember{mutableStateOf<List<ReceiptIntent>>(emptyList())};var archive by remember{mutableStateOf<List<String>>(emptyList())};var target by remember{mutableStateOf("")};var shown by remember{mutableIntStateOf(20)}
    var pendingSave by remember{mutableStateOf<Pair<String,android.net.Uri>?>(null)};var pickingId by remember{mutableStateOf("")};var pickingOwner by remember{mutableStateOf<MigrationIdentity?>(null)}
    val picker=rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/octet-stream")){uri->if(uri!=null&&pickingId.isNotEmpty())pendingSave=pickingId to uri;pickingId=""}
    suspend fun reload(s:ReceiptService){val c=s.context().toString();val next=s.rows();val p=runCatching{s.profiles().map{it.toString()}}.getOrDefault(emptyList());val measured=runCatching{s.measurement()?.toString().orEmpty()}.getOrDefault("");val history=s.archive();if(service===s&&active){context=c;rows=next;profiles=p;report=measured;archive=history}}
    fun action(block:suspend(ReceiptService)->Unit){val s=service?:return;if(busy)return;busy=true;notice="";coroutine.launch{try{block(s);if(service===s&&active){reload(s);if(notice.isEmpty())notice=m("success")}}catch(_:Exception){if(service===s&&active)notice=m("error")}finally{if(service===s)busy=false}}}
    DisposableEffect(Unit){model.migrationsVisible=true;onDispose{visible=false;service?.clear();model.migrationsVisible=false;archive=emptyList();report=""}}
    val lifecycle=(LocalContext.current as? LifecycleOwner)?.lifecycle
    DisposableEffect(lifecycle){val observer=LifecycleEventObserver{_,event->if(event==Lifecycle.Event.ON_STOP){active=false;revision++;service?.clear();service=null;archive=emptyList();report="";target="";busy=false};if(event==Lifecycle.Event.ON_START)active=true};lifecycle?.addObserver(observer);onDispose{lifecycle?.removeObserver(observer)}}
    LaunchedEffect(active,revision,model.storageNamespace,model.userId){service?.clear();service=null;rows=emptyList();archive=emptyList();if(active&&visible)try{val version=revision;val s=model.receiptService{visible&&active&&revision==version};service=s;s.reconcile();reload(s)}catch(_:Exception){notice=m("connect")}}
    LaunchedEffect(service){val s=service?:return@LaunchedEffect;while(active&&visible&&service===s){delay(5000);if(!busy)try{reload(s)}catch(_:Exception){if(service===s){archive=emptyList();notice=m("error")}}}}
    LaunchedEffect(service,pendingSave){val s=service;val pending=pendingSave;if(s!=null&&pending!=null&&active){pendingSave=null;busy=true;try{require(pickingOwner?.let{MigrationStore.samePrincipal(it,s.identity)}==true);val bytes=withContext(Dispatchers.IO){s.objectBytes(pending.first)};try{withContext(Dispatchers.IO){require(service===s&&active);appContext.contentResolver.openOutputStream(pending.second,"wt").use{out->requireNotNull(out);var offset=0;while(offset<bytes.size){require(service===s&&active);val n=minOf(65536,bytes.size-offset);out.write(bytes,offset,n);offset+=n};out.flush()}};notice=m("success")}finally{bytes.fill(0)}}catch(_:Exception){notice=m("error")}finally{if(service===s)busy=false;pickingOwner=null}}}
    val c=context.takeIf{it.isNotEmpty()}?.let{JSONObject(it)}
    val local=report.takeIf{it.isNotEmpty()}?.let{JSONObject(it)};val q=c?.optJSONObject("storage");val verified=local!=null&&q!=null&&listOf("buildDigest","verificationDigest","storageContract").all{local.opt(it)==q.opt(it)}

    Dialog(onDismissRequest=onClose,properties=DialogProperties(usePlatformDefaultWidth=false)){
        Surface(Modifier.fillMaxSize()){
            LazyColumn(Modifier.safeDrawingPadding().fillMaxSize(),contentPadding=PaddingValues(20.dp),verticalArrangement=Arrangement.spacedBy(12.dp)){
                item{
                    TextButton(onClick=onClose){Text(m("back"))};Text(t("title"),style=MaterialTheme.typography.headlineSmall);Text(t("help"))
                    if(busy)LinearProgressIndicator(Modifier.fillMaxWidth());if(notice.isNotEmpty())Text(notice)
                    Text(t(if(verified)c?.optString("qualification")?.takeUnless{it=="transport_only"}?:"transport" else "transport"))
                    OutlinedButton(onClick={if(service==null)revision++ else action{reload(it)}},enabled=!busy,modifier=Modifier.fillMaxWidth()){Text(m("reload"))}
                    Button(onClick={action{report=it.measure(true).toString()}},enabled=!busy&&service!=null,modifier=Modifier.fillMaxWidth()){Text(t("verify"))}
                    if(report.isEmpty())Text(t("notVerified")) else {
                        Text(t("measured"));TextButton(onClick={clipboard.setText(AnnotatedString(report));notice=m("copied")}){Text(t("report"))}
                    }
                    Row{Checkbox(model.receiptAutomaticFor(service?.identity),{value->service?.let{model.setReceiptAutomatic(value,it.identity)}},enabled=!busy&&verified&&c?.optBoolean("contentReceiptEnabled")==true);Text(t("automatic"),modifier=Modifier.weight(1f))};Text(t("automaticHelp"))
                    Text(t("profiles"),style=MaterialTheme.typography.titleMedium)
                    if(profiles.isEmpty())EmptyPanel(model){action{reload(it)}}
                }
                items(profiles,key={JSONObject(it).getString("id")}){raw->val p=JSONObject(raw);Column{Text(p.getString("id"));Button(onClick={action{s->val row=s.declaration(p.getString("id"));s.resume(row.id,true)}},enabled=!busy){Text(t("declare"))}}}
                item{Text(t("saveBody"),style=MaterialTheme.typography.titleMedium);if(model.messages.none{it.state !in setOf("pending","frozen")&&!it.retracted&&!it.forwarded})EmptyPanel(model){action{reload(it)}}}
                items(model.messages.filter{it.state !in setOf("pending","frozen")&&!it.retracted&&!it.forwarded}.take(shown),key={"receive-"+it.id}){message->Column{Text(message.text.ifBlank{m("metadata")});TextButton(onClick={action{it.content(message.id,message.contentVersion)}},enabled=!busy&&verified&&c?.optBoolean("contentReceiptEnabled")==true){Text(t("saveBody"))}}}
                item{Text(m("operations"),style=MaterialTheme.typography.titleMedium);if(rows.isEmpty())EmptyPanel(model){action{reload(it)}}}
                items(rows.take(shown),key={it.identity.namespace+it.id}){row->Column(verticalArrangement=Arrangement.spacedBy(6.dp)){
                    Text(t(if(row.kind=="message_content")"content" else if(row.kind=="object_received")"object" else "storage")+" · "+t(row.state));if(row.message.isNotEmpty())Text(row.message)
                    if(row.state!="retired")Row{TextButton(onClick={action{it.resume(row.id,false)}},enabled=!busy){Text(m("check"))};if(row.state!="recorded")TextButton(onClick={action{it.resume(row.id,true)}},enabled=!busy){Text(m("retry"))}}
                    if(row.kind=="message_content"&&row.state=="recorded")TextButton(onClick={action{it.objectOriginal(row.id)}},enabled=!busy&&verified&&c?.optBoolean("objectReceiptEnabled")==true){Text(t("saveObject"))}
                    if(row.kind!="storage"&&row.state!="retired")TextButton(onClick={action{target=it.targets(row.id).toString()}},enabled=!busy){Text(t("target"))}
                }}
                if(target.isNotEmpty())item{val result=JSONObject(target);Text(t("contentRecorded")+" · "+if(result.getBoolean("contentRecorded"))t("recorded") else t("pending"));Text(t("objectRecorded")+" · "+if(result.getBoolean("objectRecorded"))t("recorded") else t("pending"));Text(t("rangeTime")+" · "+if(result.isNull("contentCompleteAt"))t("notComplete") else result.getString("contentCompleteAt"))}
                item{Text(m("archive"),style=MaterialTheme.typography.titleMedium);if(archive.isEmpty())EmptyPanel(model){action{reload(it)}}}
                items(archive.take(shown),key={"archive-"+JSONObject(it).getString("id")}){raw->val value=JSONObject(raw);Column{Text(value.getString("text"));if(value.has("sourceObjectReceiptId"))TextButton(onClick={pickingId=value.getString("sourceReceiptId");pickingOwner=service?.identity;picker.launch(value.getString("id")+".bin")},enabled=!busy){Text(m("original"))}}}
                if(model.messages.size>shown||rows.size>shown||archive.size>shown)item{TextButton(onClick={shown+=20}){Text(m("more"))}}
            }
        }
    }
}

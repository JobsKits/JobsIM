import common from '@ohos.app.ability.common';
import ConfigurationConstant from '@ohos.app.ability.ConfigurationConstant';
import preferences from '@ohos.data.preferences';
import util from '@ohos.util';
import http from '@ohos.net.http';
import picker from '@ohos.file.picker';
import fs from '@ohos.file.fs';
import cryptoFramework from '@ohos.security.cryptoFramework';
import { SecureSession, NativeServices } from '../Data/NativeServices';
import { Api } from '../Data/Api';
import { AuthCoordinator } from '../Data/AuthCoordinator';
import { LocalStore } from '../Data/Store';
import { SyncStore } from '../Data/SyncStore';
import { SyncEngine, SyncEnginePorts } from '../Data/SyncEngine';
import { SyncIdentity, SyncJSON, SyncRules } from '../Data/SyncProtocol';
import { SyncText } from '../Data/SyncText';
import { SelfState, SelfPorts } from '../Data/SelfState';
import { SelfRules, SelfView, SelfDraft, SelfRead, SelfResult, SelfOperation } from '../Data/SelfProtocol';
import { SelfText } from '../Data/SelfText';
import { ExportRules, ExportIntent, ExportView } from '../Data/ExportProtocol';
import { ExportStore } from '../Data/ExportStore';
import { ExportService, ExportPorts } from '../Data/ExportService';
import { ExportFiles } from '../Data/ExportFiles';
import { ExportText } from '../Data/ExportText';
import { PasswordChange, PasswordRules, PasswordIntent, PasswordOwner, PasswordProof, PasswordPorts } from '../Data/PasswordChange';
import { PasswordStore } from '../Data/PasswordStore';
import { PasswordText } from '../Data/PasswordText';
import { Deletion, DeletionRules, DeletionIntent, DeletionPorts, DeletionResult } from '../Data/Deletion';
import { DeletionStore, DeletionLocalPlan } from '../Data/DeletionStore';
import { DeletionFiles } from '../Data/DeletionFiles';
import { DeletionText } from '../Data/DeletionText';
import { GroupTools, GroupPorts, GroupEnvironment, GroupIntent, GroupRules, GroupPage } from '../Data/GroupTools';
import { GroupStore } from '../Data/GroupStore';
import { GroupText } from '../Data/GroupText';
import { MigrationService, MigrationPorts } from '../Data/MigrationService';
import { MigrationIdentity, MigrationRules } from '../Data/MigrationProtocol';
import { MigrationStore, MigrationIntent } from '../Data/MigrationStore';
import { MigrationFiles, MigrationNative } from '../Data/MigrationNative';
import { MigrationText } from '../Data/MigrationText';
import { ReceiptFiles } from '../Data/ReceiptNative';
import { ReceiptCleanup } from '../Data/ReceiptCleanup';
import { ReceiptIntent } from '../Data/ReceiptProtocol';
import { ReceiptStore } from '../Data/ReceiptStore';
import { ReceiptService, ReceiptPorts } from '../Data/ReceiptService';
import { ReceiptText } from '../Data/ReceiptText';
import { ReceiptCoordinator, ReceiptCoordinatorPorts, ReceiptCandidate, ReceiptCachePage } from '../Data/ReceiptCoordinator';
import relationalStore from '@ohos.data.relationalStore';
import pasteboard from '@ohos.pasteboard';
import { Strings } from '../Data/Strings';
import { User, Conversation, Message, Bootstrap, AuthResult, ConversationList, MessageList, LookupPage, UserList, Codes, Isolation, MediaResult, ContactRequest, ContactsResult, GroupMember, MemberList, ReopenResult, PendingOperation, Rules, DraftState, AuthContext, SessionPointer, DeviceSession, SessionList, AvatarPending, AvatarOperation, Report, ReportList, ReportDraft, Reaction, ConversationDraft, Announcement, AnnouncementIntent, AnnouncementMutation, AnnouncementOperation, AuthConnection, AuthPorts, AuthStateEntry, CredentialPair, ApiFailure } from '../Data/Models';

interface SelfOperationItem { id: string; kind: string; state: string; reason: string; }
interface SelfDraftItem { conversation: string; title: string; writable: boolean; }

@Entry
@Component
struct Index {
  @State receiptAutoEnabled:boolean=false;
  private receiptAutoIdentity?:MigrationIdentity;
  private receiptAutomatic?:ReceiptCoordinator;
  @State receiptRows:ReceiptIntent[]=[];
  @State receiptContext:string='';
  @State receiptReport:string='';
  @State receiptProfiles:string[]=[];
  @State receiptArchive:string[]=[];
  @State receiptTarget:string='';
  @State receiptNotice:string='';
  @State receiptBusy:boolean=false;
  @State receiptShown:number=20;
  private receiptLife:number=0;
  private receiptTimer:number=-1;
  private receiptService?:ReceiptService;
  @State migrationQualified:string[]=[];
  @State migrationRows: MigrationIntent[] = [];
  @State migrationHistory: string[] = [];
  @State migrationCandidates: Message[] = [];
  @State migrationSelected: string[] = [];
  @State migrationFull: boolean = false;
  @State migrationPassword: string = '';
  @State migrationOTP: string = '';
  @State migrationCode: string = '';
  @State migrationID: string = '';
  @State migrationNotice: string = '';
  @State migrationBusy: boolean = false;
  @State migrationShown: number = 20;
  private migrationLife: number = 0;
  private migrationTimer: number = -1;
  private migrationService?: MigrationService;
  private migrationIdentity?: MigrationIdentity;
  @State groupSnapshot: string = '';
  @State groupInvites: string[] = [];
  @State groupRequests: string[] = [];
  @State groupOperations: GroupIntent[] = [];
  @State groupMembersNext: string = '';
  @State groupInvitesNext: string = '';
  @State groupRequestsNext: string = '';
  @State groupControlId: string = '';
  @State groupBusy: boolean = false;
  @State groupNotice: string = '';
  @State groupCode: string = '';
  @State groupOTP: string = '';
  @State groupMuteAll: boolean = false;
  @State groupSlow: string = '0';
  @State groupCapacity: string = '200';
  @State groupUses: string = '1';
  @State groupApproval: boolean = true;
  @State groupShown: number = 20;
  private groupLife: number = 0;
  private groupService?: GroupTools;
  @State deletionRows: DeletionIntent[] = [];
  @State deletionPassword: string = '';
  @State deletionRecovery: string = '';
  @State deletionOTP: string = '';
  @State deletionBusy: boolean = false;
  @State deletionNotice: string = '';
  @State deletionShown: number = 20;
  private deletionLife: number = 0;
  @State passwordRows: PasswordIntent[] = [];
  @State passwordOld: string = '';
  @State passwordNew: string = '';
  @State passwordConfirm: string = '';
  @State passwordOTP: string = '';
  @State passwordBusy: boolean = false;
  @State passwordNotice: string = '';
  @State passwordShown: number = 20;
  private passwordLife: number = 0;
  @StorageLink('JobsIM.foreground') @Watch('abilityVisibilityChanged') appVisible: boolean = true;
  @State syncRestricted: boolean = false;
  @State localEntries: boolean = false;
  @State selfSupported: boolean = false;
  @State selfAll: boolean = false;
  @State selfViews: Record<string, SelfView> = {};
  @State selfDraft: SelfDraft | undefined = undefined;
  @State selfRead: SelfRead | undefined = undefined;
  @State selfView: SelfView | undefined = undefined;
  @State selfOperations: SelfOperationItem[] = [];
  @State selfDraftEntries: SelfDraftItem[] = [];
  @State cloudFont: string = 'standard';
  @State @Watch('exportPageChanged') page: string = 'chats';
  @State exportRows: ExportIntent[] = [];
  @State exportViews: Record<string, ExportView> = {};
  @State exportSupported: boolean = false;
  @State exportBusy: boolean = false;
  @State exportDownloading: boolean = false;
  @State exportNotice: string = '';
  @State exportVisibleCount: number = 20;
  private exportService?: ExportService;
  private exportFiles?: ExportFiles;
  private exportLife: number = 0;
  private exportTransfer: number = 0;
  @State language: string = 'zh-Hans';
  @State theme: string = 'system';
  @State fontScale: number = 1;
  @State dark: boolean = false;
  @State base: string = 'https://localhost:9443';
  @State connectionAddress: string = '';
  @State expectedId: string = '';
  @State caPem: string = '';
  @State caFingerprint: string = '';
  private caPath: string = '';
  @State deploymentId: string = '';
  @State restoreEpoch: string = '1';
  @State profile: string = '';
  @State token: string = '';
  @State userId: string = '';
  @State deviceId: string = '';
  @State me: User | undefined = undefined;
  @State deviceSessions: DeviceSession[] = [];
  @State reports: Report[] = [];
  @State reportType: string = 'message';
  @State reportTarget: string = '';
  @State reportReason: string = '';
  @State avatarPending: boolean = false;
  @State avatarPath: string = '';
  private secure: SecureSession = new SecureSession();
  private sessionAlias: string = '';
  private sourceContextId: string = '';
  private auth?: AuthCoordinator;
  private authIdentity: string = '';
  private authOwner: string = '';
  private ownAvatarRef: string = '';
  @State mode: string = 'login';
  @State username: string = '';
  @State password: string = '';
  @State code: string = '';
  @State info: string = '';
  @State offline: boolean = false;
  @State demo: boolean = false;
  @State busy: boolean = false;
  @State conversations: Conversation[] = [];
  @State people: User[] = [];
  @State requests: ContactRequest[] = [];
  @State results: User[] = [];
  @State blockedIds: string[] = [];
  @State managedPeer: string = '';
  @State members: GroupMember[] = [];
  @State groupTitle: string = '';
  @State groupNewMember: string = '';
  @State groupNewOwner: string = '';
  @State rows: Message[] = [];
  @State lookupKind: string = 'favorites';
  @State lookupRows: Message[] = [];
  @State lookupQuery: string = '';
  @State lookupSubmitted: string = '';
  @State lookupCursor: string = '';
  @State lookupMore: boolean = false;
  @State lookupFocusId: string = '';
  private lookupScope: string = '';
  @State conversation: Conversation | undefined = undefined;
  @State draft: string = '';
  @State mentionedUserIds: string[] = [];
  @State mentionAll: boolean = false;
  @State mentionsSupported: boolean = false;
  @State announcementsSupported: boolean = false;
  @State roleAdmissionSupported: boolean = false;
  @State forwardingSupported: boolean = false;
  @State forwardSource: Message | undefined = undefined;
  @State mentionUnread: Record<string, number> = {};
  @State announcement: Announcement | undefined = undefined;
  @State announcementCandidate: Message | undefined = undefined;
  @State announcementExpiry: string = 'never';
  @State announcementPending: boolean = false;
  @State announcementRejected: boolean = false;
  @State query: string = '';
  @State replyId: string = '';
  @State editId: string = '';
  @State editVersion: string = '';
  @State newTitle: string = '';
  @State memberIds: string = '';
  @State newGroup: boolean = true;
  @State codes: string[] = [];
  @State unread: Record<string, number> = {};
  @State historyMore: boolean = false;
  @State notificationsEnabled: boolean = false;
  private pollTimer: number = -1;
  private foregroundVisible: boolean = true;
  private draftTail: Promise<void> = Promise.resolve();
  private draftRevision: number = 0;
  private cloudReplyVersion: string = '';
  private observedVisible: Map<string, string> = new Map();
  private selfService?: SelfState;
  private selfKey: string = '';
  private cloudFontKey: string = '';
  private store: LocalStore = new LocalStore();
  private syncData: SyncStore = new SyncStore(this.store, NativeServices.syncCodec(), () => util.generateRandomUUID(true));
  private synchronizer?: SyncEngine;
  private syncKey: string = '';
  private syncLife: number = 0;
  private forwardViews: Map<string, Message> = new Map();
  private prefs?: preferences.Preferences;

  aboutToAppear(): void {
    this.run(async () => {
      const context = getContext(this) as common.UIAbilityContext;
      await this.store.open(context);
      ExportFiles.cleanup(context);
      await this.syncData.open(); await this.drainSyncCleanup();
      this.prefs = await preferences.getPreferences(context, { name: 'JobsIM.settings' });
      this.language = await this.prefs.get('language', 'zh-Hans') as string;
      this.base = await this.prefs.get('base', this.base) as string;
      this.connectionAddress = this.base;
      this.deploymentId = await this.prefs.get('deploymentId', '') as string;
      this.restoreEpoch = await this.prefs.get('restoreEpoch', '1') as string;
      this.caPath = await this.prefs.get('caPath', '') as string;
      this.caFingerprint = await this.prefs.get('caFingerprint', '') as string;
      this.expectedId = this.deploymentId;
      this.profile = await this.prefs.get('profile', '') as string;
      if (this.profile.length === 0) { this.profile = util.generateRandomUUID(true); }
      this.theme = await this.prefs.get('theme', 'system') as string;
      this.fontScale = Number(await this.prefs.get('fontScale', '1'));
      this.notificationsEnabled = await this.prefs.get('notifications', false) as boolean;
      this.applyTheme(this.theme);
      await this.restoreSession();
      this.startPolling();
    });
  }
  onPageShow(): void { this.foregroundVisible = true; this.startPolling(); if(this.page==='migration'){this.openMigration(false);} if(this.page==='receipts'){this.openReceipt();} }
  onPageHide(): void { this.receiptAutomatic?.stop(); this.closeReceipt(); this.closeMigration(); this.closeGroupTools(); this.closeDeletion(); this.clearPasswordInput(); this.passwordLife++; this.passwordBusy = false; this.foregroundVisible = false; this.closeExport(); this.stopSync(); this.stopPolling(); }
  aboutToDisappear(): void { this.receiptAutomatic?.stop(); this.closeReceipt(); this.closeMigration(); this.closeGroupTools(); this.closeDeletion(); this.clearPasswordInput(); this.passwordLife++; this.passwordBusy = false; this.closeExport(); this.stopSync(); this.stopPolling(); }
  private abilityVisibilityChanged(): void { if (!this.appVisible) { this.receiptAutomatic?.stop(); this.closeReceipt(); this.closeMigration(); this.closeGroupTools(); this.closeDeletion(); this.clearPasswordInput(); this.passwordLife++; this.passwordBusy = false; this.stopSync(); this.stopPolling(); } else if (this.foregroundVisible) { this.startPolling(); if(this.page==='migration'){this.openMigration(false);} if(this.page==='receipts'){this.openReceipt();} } }
  private stopSync(): void { this.syncLife++; this.synchronizer?.cancel(); this.synchronizer = undefined; this.syncKey = ''; }
  private startPolling(): void {
    if (this.pollTimer >= 0) { return; }
    this.pollTimer = setInterval(() => {
      if (this.foregroundVisible && this.appVisible && !this.busy && !this.demo && this.token.length > 0 && this.page !== 'exports' && this.page !== 'password' && this.page !== 'deletion' && this.page !== 'groupTools' && this.page !== 'migration' && this.page !== 'receipts' && !this.passwordPaused() && !this.deletionPaused()) { this.run(async () => this.reload()); }
    }, 5000);
  }
  private stopPolling(): void {
    if (this.pollTimer >= 0) { clearInterval(this.pollTimer); this.pollTimer = -1; }
  }
  private persistDraft(): void {
    if (!this.conversation) { return; }
    this.draftRevision++;
    const namespace = this.namespace();
    const key = 'draft:' + this.conversation.id;
    const demo = this.demo;
    const payload = JSON.stringify({ text: this.draft, replyId: this.replyId, editId: this.editId, editVersion: this.editVersion, mentionedUserIds: this.mentionedUserIds.slice(), mentionAll: this.mentionAll });
    this.draftTail = this.draftTail.then(async () => { await this.store.setValue(namespace, key, payload, demo); }).catch((error: Error) => { this.info = error.message; });
  }
  private updateDraft(value: string): void { this.draft = value; this.persistDraft(); }
  onBackPress(): boolean {
    if (this.page === 'exports') { this.page = 'settings'; return true; }
    if (this.page === 'mentions' || this.page === 'announcement' || this.page === 'forward' || this.page === 'self') { this.page = 'chats'; return true; }
    if (this.conversation) { this.conversation = undefined; this.rows = []; this.replyId = ''; return true; }
    if (this.page !== 'chats') { this.page = 'chats'; return true; }
    return false;
  }
  private receiptAutomaticCurrent():boolean{const i=this.receiptAutoIdentity;return Boolean(i)&&this.receiptAutoEnabled&&this.foregroundVisible&&this.appVisible&&!this.demo&&this.token.length>0&&this.namespace()===i?.namespace&&(this.auth?.currentFamily()??'')===i?.family&&this.deviceId===i?.device&&this.restoreEpoch===i?.epoch&&!['receipts','migration','password','deletion','groupTools','exports'].includes(this.page)&&!this.passwordPaused()&&!this.deletionPaused();}
  private receiptAutomaticFor(i:MigrationIdentity):boolean{return this.receiptAutoEnabled&&Boolean(this.receiptAutoIdentity)&&MigrationStore.samePrincipal(this.receiptAutoIdentity as MigrationIdentity,i);}
  private setReceiptAutomatic(enabled:boolean):void{const service=this.receiptService;if(!enabled){this.receiptAutoEnabled=false;this.receiptAutoIdentity=undefined;this.receiptAutomatic?.stop();this.receiptAutomatic=undefined;return;}if(!service||!this.receiptQualified()){return;}const i=service.principal();if(this.namespace()!==i.namespace||(this.auth?.currentFamily()??'')!==i.family){return;}this.receiptAutomatic?.stop();this.receiptAutomatic=undefined;this.receiptAutoIdentity=i;this.receiptAutoEnabled=true;}
  private async backgroundReceiptService(owned:()=>boolean):Promise<ReceiptService>{
    const original=this.receiptAutoIdentity;if(!original||!owned()||!this.receiptAutomaticCurrent()){MigrationRules.fail('RECEIPT_NOT_ENABLED');}const c=await this.syncData.syncControl(this.syncIdentity());if(!c||c.physical!==original.store){MigrationRules.fail('RECEIPT_STORE_CHANGED');}const i:MigrationIdentity={profile:original.profile,deployment:original.deployment,user:original.user,device:original.device,family:original.family,epoch:original.epoch,namespace:original.namespace,store:original.store,guard:c.guard};const owner=this.authOwner;const connection=this.authConnectionIdentity();const current=():boolean=>owned()&&this.receiptAutomaticCurrent()&&this.authOwner===owner&&this.authConnectionIdentity()===connection&&Boolean(this.receiptAutoIdentity)&&MigrationStore.samePrincipal(this.receiptAutoIdentity as MigrationIdentity,i);
    const files=new ReceiptFiles(getContext(this) as common.UIAbilityContext,i,current);const store=new ReceiptStore(this.store,i,current,files,MigrationNative.codec());await store.open();if(!current()){MigrationRules.fail();}const api=this.api();const ports:ReceiptPorts={identity:i,current:current,codec:MigrationNative.codec(),uuid:()=>util.generateRandomUUID(true),store:store,request:async(path:string,method:string,body:string,physical:string)=>JSON.stringify(await api.receipt<Object>(path,method as http.RequestMethod,body,physical,current)),binary:(path:string,physical:string,limit:number)=>api.migrationBytes(path,physical,limit,current)};return new ReceiptService(ports);
  }
  private offerReceiptAutomatic(raws:string[]):void{
    if(!this.receiptAutomaticCurrent()){return;}if(!this.receiptAutomatic){const ports:ReceiptCoordinatorPorts={enabled:()=>this.receiptAutomaticCurrent(),factory:(owned:()=>boolean)=>this.backgroundReceiptService(owned),cached:async(after:string,limit:number)=>this.store.syncAccess(async(db:relationalStore.RdbStore)=>{const i=this.receiptAutoIdentity;if(!i||!this.receiptAutomaticCurrent()){MigrationRules.fail();}const rows=await db.querySql("SELECT id,payload FROM messages WHERE namespace=? AND id>? AND state NOT IN('pending','frozen') ORDER BY id LIMIT ?",[i.namespace,after,limit]);const candidates:ReceiptCandidate[]=[];let next='';try{while(rows.goToNextRow()){next=rows.getString(0);const m=SyncJSON.parse(rows.getString(1));if(m.get('deleted').value!==true&&m.get('forwarded').value!==true&&m.get('contentUnavailable').value!==true&&m.get('contentVersion').value!==null){const row:ReceiptCandidate={id:MigrationRules.id(next),version:MigrationRules.positive(m.get('contentVersion').string())};candidates.push(row);}}}finally{rows.close();}const result:ReceiptCachePage={rows:candidates,after:next};return result;})};this.receiptAutomatic=new ReceiptCoordinator(ports);}this.receiptAutomatic.offer(raws);
  }
  private r(key:string):string{return ReceiptText.text(this.language,key);}
  private closeReceipt():void{this.receiptLife++;this.receiptService?.clear();this.receiptService=undefined;this.receiptBusy=false;this.receiptArchive=[];this.receiptReport='';this.receiptTarget='';if(this.receiptTimer>=0){clearInterval(this.receiptTimer);this.receiptTimer=-1;}}
  private async openReceipt():Promise<void>{
    this.closeReceipt();this.page='receipts';this.receiptNotice='';this.receiptRows=[];this.receiptProfiles=[];this.receiptContext='';const life=this.receiptLife;
    try{if(this.busy||this.demo||!this.token||!this.auth?.currentFamily()){MigrationRules.fail();}const c=await this.syncData.syncControl(this.syncIdentity());if(!c||!c.cursor||c.restriction){MigrationRules.fail();}const family=this.auth?.currentFamily()??'';const owner=this.authOwner;const connection=this.authConnectionIdentity();const i:MigrationIdentity={profile:this.profile,deployment:this.deploymentId,user:this.userId,device:this.deviceId,family:family,epoch:this.restoreEpoch,namespace:this.namespace(),store:c.physical,guard:c.guard};const current=():boolean=>this.page==='receipts'&&life===this.receiptLife&&this.foregroundVisible&&this.appVisible&&!this.demo&&this.token.length>0&&this.namespace()===i.namespace&&(this.auth?.currentFamily()??'')===family&&this.authOwner===owner&&this.authConnectionIdentity()===connection;
      const files=new ReceiptFiles(getContext(this) as common.UIAbilityContext,i,current);const store=new ReceiptStore(this.store,i,current,files,MigrationNative.codec());await store.open();if(!current()){return;}const api=this.api();const ports:ReceiptPorts={identity:i,current:current,codec:MigrationNative.codec(),uuid:()=>util.generateRandomUUID(true),store:store,request:async(path:string,method:string,body:string,physical:string)=>JSON.stringify(await api.receipt<Object>(path,method as http.RequestMethod,body,physical,current)),binary:(path:string,physical:string,limit:number)=>api.migrationBytes(path,physical,limit,current)};const service=new ReceiptService(ports);this.receiptService=service;await service.reconcile();await this.reloadReceipt(service);if(current()){this.receiptTimer=setInterval(()=>{if(current()&&!this.receiptBusy){this.reloadReceipt(service).catch((_error:Error)=>{if(current()){this.receiptArchive=[];this.receiptNotice=this.m('error');}});}},5000);}
    }catch(_){if(life===this.receiptLife){this.receiptNotice=this.m('connect');}}
  }
  private async reloadReceipt(service:ReceiptService):Promise<void>{const context=await service.context();const rows=await service.rows();let profiles:string[]=[];let report='';try{profiles=await service.profiles();report=await service.measurement()??'';}catch(_){}const archive=await service.archive();await this.drainSyncCleanup();if(this.receiptService===service){this.receiptContext=context;this.receiptRows=rows;this.receiptProfiles=profiles;this.receiptReport=report;this.receiptArchive=archive;}}
  private receiptAction(action:(service:ReceiptService)=>Promise<void>):void{const service=this.receiptService;const life=this.receiptLife;if(!service||this.receiptBusy){return;}this.receiptBusy=true;this.receiptNotice='';action(service).then(async()=>{if(life===this.receiptLife&&service===this.receiptService){await this.reloadReceipt(service);if(!this.receiptNotice){this.receiptNotice=this.m('success');}}}).catch((_error:Error)=>{if(life===this.receiptLife){this.receiptNotice=this.m('error');}}).finally(()=>{if(life===this.receiptLife){this.receiptBusy=false;}});}
  private receiptQualified(objects:boolean=false):boolean{if(!this.receiptContext||!this.receiptReport){return false;}const c=SyncJSON.parse(this.receiptContext);const q=c.get('storage');const local=SyncJSON.parse(this.receiptReport);return q.get('qualified').value===true&&(!objects||q.get('objectQualified').value===true)&&['buildDigest','verificationDigest','storageContract'].every((key:string)=>q.get(key).value===local.get(key).value);}
  private receiptQualification():string{return this.receiptQualified(true)?'qualified_content_and_objects':this.receiptQualified()?'qualified_content':'transport';}
  private receiptTargetText(key:string):string{if(!this.receiptTarget){return '';}const value=SyncJSON.parse(this.receiptTarget).get(key);return typeof value.value==='boolean'?this.r(value.value?'recorded':'pending'):value.value===null?this.r('notComplete'):value.string();}
  private async saveReceiptOriginal(parent:string):Promise<void>{
    const original=this.receiptService?.principal();if(!original){return;}try{const context=getContext(this) as common.UIAbilityContext;const options=new picker.DocumentSaveOptions();options.newFileNames=[MigrationRules.id(parent)+'.bin'];const uris=await new picker.DocumentViewPicker(context).save(options);if(!uris.length){return;}if(uris.length!==1||!uris[0].startsWith('file://')){MigrationRules.fail();}if(!this.receiptService){await this.openReceipt();}const service=this.receiptService;if(!service||!MigrationStore.samePrincipal(original,service.principal())){MigrationRules.fail();}const bytes=await service.objectBytes(parent);try{if(service!==this.receiptService||!this.appVisible||!this.foregroundVisible){MigrationRules.fail();}const file=fs.openSync(uris[0],fs.OpenMode.WRITE_ONLY|fs.OpenMode.TRUNC);try{let offset=0;while(offset<bytes.length){if(service!==this.receiptService||!this.appVisible||!this.foregroundVisible){MigrationRules.fail();}const n=fs.writeSync(file.fd,bytes.buffer.slice(bytes.byteOffset+offset,bytes.byteOffset+bytes.length));if(n<=0){MigrationRules.fail();}offset+=n;}fs.fsyncSync(file.fd);}finally{fs.closeSync(file);}this.receiptNotice=this.m('success');}finally{bytes.fill(0);}}catch(_){this.receiptNotice=this.m('error');}
  }
  private async copyReceiptReport():Promise<void>{const report=this.receiptReport;const life=this.receiptLife;if(!report){return;}await pasteboard.getSystemPasteboard().setData(pasteboard.createData(pasteboard.MIMETYPE_TEXT_PLAIN,report));if(life===this.receiptLife){this.receiptNotice=this.m('copied');}}
  private m(key:string):string{return MigrationText.text(this.language,key);}
  private closeMigration():void{this.migrationLife++;this.migrationService?.clear();this.migrationService=undefined;this.migrationIdentity=undefined;this.migrationPassword='';this.migrationOTP='';this.migrationCode='';this.migrationHistory=[];this.migrationQualified=[];this.migrationBusy=false;if(this.migrationTimer>=0){clearInterval(this.migrationTimer);this.migrationTimer=-1;}}
  private async openMigration(fresh:boolean=true):Promise<void>{
    this.closeMigration();this.page='migration';if(fresh){this.migrationCandidates=this.rows.filter((row:Message)=>!row.deleted&&!row.contentUnavailable&&!row.forwardUnavailable&&!['pending','frozen'].includes(row.state??'')&&Boolean(row.contentVersion)&&['text','image','audio','video','file'].includes(row.type));this.migrationSelected=[];this.migrationRows=[];this.migrationShown=20;}
    const life=this.migrationLife;this.migrationNotice='';
    try{
      if(this.busy||this.demo||!this.token||!this.auth?.currentFamily()){throw new Error('MIGRATION_STORE_NOT_READY');}
      const control=await this.syncData.syncControl(this.syncIdentity());if(!control||!control.cursor||control.restriction){throw new Error('MIGRATION_STORE_NOT_READY');}
      const family=this.auth?.currentFamily()??'';const owner=this.authOwner;const connection=this.authConnectionIdentity();
      const i:MigrationIdentity={profile:this.profile,deployment:this.deploymentId,user:this.userId,device:this.deviceId,family:family,epoch:this.restoreEpoch,namespace:this.namespace(),store:control.physical,guard:control.guard};
      const current=():boolean=>this.page==='migration'&&life===this.migrationLife&&this.foregroundVisible&&this.appVisible&&!this.demo&&this.token.length>0&&this.namespace()===i.namespace&&(this.auth?.currentFamily()??'')===family&&this.authOwner===owner&&this.authConnectionIdentity()===connection;
      if(!current()){return;}const files=new MigrationFiles(getContext(this) as common.UIAbilityContext,i,current);const store=new MigrationStore(this.store,i,current,files);await store.open();if(!current()){return;}const api=this.api();const receiptFiles=new ReceiptFiles(getContext(this) as common.UIAbilityContext,i,current);const receiptStore=new ReceiptStore(this.store,i,current,receiptFiles,MigrationNative.codec());await receiptStore.open();
      const ports:MigrationPorts={identity:i,current:current,uuid:()=>util.generateRandomUUID(true),store:store,files:files,codec:MigrationNative.codec(),request:async(path:string,method:string,body:string,physical:string)=>JSON.stringify(await api.migration<Object>(path,method as http.RequestMethod,body,physical,current)),source:async(path:string)=>{if(!current()){MigrationRules.fail();}const raw=JSON.stringify(await api.request<Object>(path));if(!current()){MigrationRules.fail();}return raw;},binary:(path:string,physical:string,limit:number)=>api.migrationBytes(path,physical,limit,current),qualifiedSources:()=>receiptStore.migrationSources(),qualifiedSource:(raw:string)=>receiptStore.migrationSource(raw),receiptIntent:(id:string)=>receiptStore.intent(id),receiptRequest:async(path:string,method:string,body:string,physical:string)=>JSON.stringify(await api.receipt<Object>(path,method as http.RequestMethod,body,physical,current))};
      const service=new MigrationService(ports);this.migrationIdentity=i;this.migrationService=service;await service.capability();await this.reloadMigration(service);
      if(current()){this.migrationTimer=setInterval(()=>{if(!this.migrationBusy&&current()){this.reloadMigration(service).catch((_error:Error)=>{if(current()){this.migrationHistory=[];this.migrationNotice=this.m('error');}});}},5000);}
    }catch(_){if(life===this.migrationLife){this.migrationNotice=this.m('connect');}}
  }
  private async reloadMigration(service:MigrationService):Promise<void>{const rows=await service.rows();const qualified=await service.qualifiedSources();await this.drainSyncCleanup();const history=await service.history();if(this.migrationService===service){this.migrationRows=rows;this.migrationHistory=history;this.migrationQualified=qualified.filter((raw:string)=>!this.migrationCandidates.some((item:Message)=>item.id===SyncJSON.parse(raw).get('id').string()));}}
  private migrationAction(action:(service:MigrationService,password:string,otp:string,code:string)=>Promise<void>):void{
    const service=this.migrationService;const life=this.migrationLife;if(!service||this.migrationBusy){return;}const password=this.migrationPassword;const otp=this.migrationOTP;const code=this.migrationCode;this.migrationPassword='';this.migrationOTP='';this.migrationCode='';this.migrationBusy=true;this.migrationNotice='';
    action(service,password,otp,code).then(async()=>{if(life===this.migrationLife&&this.migrationService===service){await this.reloadMigration(service);if(!this.migrationNotice){this.migrationNotice=this.m('success');}}}).catch((_error:Error)=>{if(life===this.migrationLife){this.migrationNotice=this.m('error');}}).finally(()=>{if(life===this.migrationLife){this.migrationBusy=false;}});
  }
  private migrationSelection():string{const rows:Object[]=[];for(const row of this.migrationCandidates.filter((item:Message)=>this.migrationSelected.includes(item.id))){const value:Record<string,Object>={};value['id']=row.id;value['conversationId']=row.conversationId;value['contentVersion']=row.contentVersion??'';value['type']=row.type;value['objectMode']=row.type==='text'?'none':this.migrationFull&&!row.forwarded&&Boolean(row.attachmentId)?'full':'metadata_only';if(row.forwarded){value['forwarded']=true;}else if(row.attachmentId){value['attachmentId']=row.attachmentId;}rows.push(value);}for(const raw of this.migrationQualified){const m=SyncJSON.parse(raw);if(!this.migrationSelected.includes(m.get('id').string())){continue;}const value:Record<string,Object>={};const mode=m.get('type').string()==='text'?'none':this.migrationFull&&m.has('sourceObjectReceiptId')?'full':'metadata_only';for(const key of ['id','conversationId','contentVersion','type','attachmentId','sourceReceiptId','sourceObjectReceiptId']){if(m.has(key)&&(key!=='sourceObjectReceiptId'||mode==='full')){value[key]=m.get(key).value as Object;}}value['objectMode']=mode;rows.push(value);}return JSON.stringify(rows);}
  private migrationField(raw:string,key:string):string{const value=SyncJSON.parse(raw).get(key);return typeof value.value==='string'?value.string():'';}
  private migrationCanSave(raw:string):boolean{const row=SyncJSON.parse(raw);return row.get('objectMode').value==='full'&&row.get('contentUnavailable').value!==true&&!row.get('forwarded').value;}
  private async copyMigrationPair(id:string):Promise<void>{const service=this.migrationService;const life=this.migrationLife;if(!service){return;}try{const proof=service.sharePair(id);await pasteboard.getSystemPasteboard().setData(pasteboard.createData(pasteboard.MIMETYPE_TEXT_PLAIN,proof));if(life===this.migrationLife){this.migrationNotice=this.m('copied');}}catch(_){if(life===this.migrationLife){this.migrationNotice=this.m('error');}}}
  private async saveMigrationObject(id:string):Promise<void>{
    const original=this.migrationIdentity;if(!original){return;}const context=getContext(this) as common.UIAbilityContext;const options=new picker.DocumentSaveOptions();options.newFileNames=[MigrationRules.id(id)+'.bin'];
    try{const uris=await new picker.DocumentViewPicker(context).save(options);if(!uris.length){return;}if(uris.length!==1||!uris[0].startsWith('file://')){MigrationRules.fail();}if(!this.migrationService){await this.openMigration(false);}const service=this.migrationService;const current=this.migrationIdentity;if(!service||!current||!MigrationStore.samePrincipal(original,current)){MigrationRules.fail();}
      const info=SyncJSON.parse(await service.objectInfo(id));const namespace=info.get('namespace').string();const files=new MigrationFiles(context,{profile:current.profile,deployment:current.deployment,user:current.user,device:current.device,family:current.family,epoch:current.epoch,namespace:namespace,store:current.store,guard:current.guard},()=>this.migrationService===service&&this.foregroundVisible&&this.appVisible);const bytes=new Uint8Array(files.object(info.get('path').string()));
      try{if(await MigrationNative.codec().hash(bytes)!==info.get('sha256').string()||this.migrationService!==service){MigrationRules.fail();}const file=fs.openSync(uris[0],fs.OpenMode.WRITE_ONLY|fs.OpenMode.TRUNC);try{let offset=0;while(offset<bytes.length){if(this.migrationService!==service){MigrationRules.fail();}const n=fs.writeSync(file.fd,bytes.buffer.slice(bytes.byteOffset+offset,bytes.byteOffset+bytes.length));if(n<=0){MigrationRules.fail();}offset+=n;}fs.fsyncSync(file.fd);}finally{fs.closeSync(file);}this.migrationNotice=this.m('success');}finally{bytes.fill(0);}
    }catch(_){this.migrationNotice=this.m('error');}
  }
  private t(key: string): string { return Strings.text(this.language, key); }
  private s(key: string): string { return SelfText.text(this.language, key); }
  private e(key: string): string { return ExportText.text(this.language, key); }
  private foreground(): string { return this.dark ? '#EAF0EB' : '#18251F'; }
  private surface(): string { return this.dark ? '#121615' : '#FAF9F5'; }
  private bubble(): string { return this.dark ? '#253A31' : '#DCEDE4'; }
  private namespace(): string { return Isolation.namespace(this.profile, this.deploymentId, this.userId, this.demo, this.restoreEpoch, this.deviceId, this.sourceContextId); }
  private operationNamespace(): string { return Isolation.namespace(this.profile, this.deploymentId, this.userId, this.demo, this.restoreEpoch, 'account-operations'); }
  private api(): Api { if (this.passwordPaused() || this.deletionPaused()) { throw new Error('ACCOUNT_SECURITY_RESULT_UNKNOWN'); } return new Api(this.base, this.token, this.caPath, this.auth?.hooks()); }
  private p(key: string): string { return PasswordText.text(this.language, key); }
  private clearPasswordInput(): void { this.passwordOld = ''; this.passwordNew = ''; this.passwordConfirm = ''; this.passwordOTP = ''; }
  private passwordPaused(): boolean { const family = this.auth?.currentFamily() ?? ''; return Boolean(family) && this.passwordRows.some((row: PasswordIntent) => row.family === family && PasswordRules.blocked(row)); }
  private passwordRepository(): PasswordStore {
    const identity = this.authConnectionIdentity();
    return new PasswordStore(this.store, this.profile, () => identity === this.authConnectionIdentity(), (row: PasswordIntent) => {
      this.auth?.passwordPause(row.family, PasswordRules.blocked(row));
      const index = this.passwordRows.findIndex((item: PasswordIntent) => item.id === row.id);
      this.passwordRows = index < 0 ? [row, ...this.passwordRows] : this.passwordRows.map((item: PasswordIntent) => item.id === row.id ? row : item);
      if (this.passwordPaused()) { this.stopSync(); this.clearSyncRAM(); this.rows = []; this.conversations = []; }
    });
  }
  private async loadPasswordRows(): Promise<void> { const repository = this.passwordRepository(); await repository.open(); this.passwordRows = await repository.list(); }
  private openPassword(): void { this.clearPasswordInput(); this.passwordShown = 20; this.passwordNotice = ''; this.page = 'password'; this.run(async () => this.loadPasswordRows()); }
  private submitPassword(id: string, retry: boolean): void {
    if (this.passwordBusy) { return; }
    let proof: PasswordProof;
    try { proof = PasswordRules.normalized(this.passwordOld, this.passwordNew, this.passwordConfirm, this.passwordOTP); }
    catch (_) { this.passwordNotice = this.p('invalid'); return; }
    this.clearPasswordInput(); this.passwordBusy = true; this.passwordNotice = '';
    const life = this.passwordLife; const identity = this.authConnectionIdentity(); const family = this.auth?.currentFamily() ?? ''; const authOwner = this.authOwner;
    const current = (): boolean => this.foregroundVisible && this.appVisible && this.page === 'password' && life === this.passwordLife && identity === this.authConnectionIdentity() && authOwner === this.authOwner && family === (this.auth?.currentFamily() ?? '');
    const ports: PasswordPorts = {
      profile: this.profile, origin: this.base, deployment: this.deploymentId, epoch: this.restoreEpoch, current: current,
      owner: (): PasswordOwner | undefined => {
        const pair = this.auth?.passwordOwner(); if (!pair || this.demo) { return undefined; }
        return { user: pair.userId, username: pair.username, device: pair.deviceId, family: pair.familyId, namespace: Isolation.namespace(pair.profile, pair.deployment, pair.userId, false, pair.epoch, pair.deviceId, pair.sourceContextId), token: pair.token };
      },
      uuid: () => util.generateRandomUUID(true), repository: this.passwordRepository(),
      prepare: async () => { await this.api().request<AuthContext>('/auth/context'); },
      bootstrap: async () => { await this.verifySavedCA(); return new Api(this.base, '', this.caPath).request<Bootstrap>('/bootstrap'); },
      request: async (route: string, body: string, token: string) => {
        if (!current()) { throw new Error('PASSWORD_CHANGE_CONTEXT_CHANGED'); }
        return JSON.stringify(await new Api(this.base, token, this.caPath).request<Object>(route, 'POST', body));
      },
      stop: async (row: PasswordIntent) => {
        if (current() && this.auth?.currentFamily() === row.family) { await this.auth.clear(this.auth.stamp()); }
      }
    };
    const service = new PasswordChange(ports);
    service.perform(id, proof, retry).then(async () => { if (identity === this.authConnectionIdentity() && life === this.passwordLife) { await this.loadPasswordRows(); } })
      .catch((error: Error) => { if (current()) { this.passwordNotice = this.p(error.message === 'PASSWORD_CHANGE_UNAVAILABLE' ? 'unavailable' : error.message === 'PASSWORD_CHANGE_ORIGINAL_PENDING' ? 'blocked' : 'error'); } })
      .finally(() => { proof.currentPassword = ''; proof.newPassword = ''; proof.otp = ''; if (life === this.passwordLife) { this.passwordBusy = false; } });
  }
  private d(key: string): string { return DeletionText.text(this.language, key); }
  private clearDeletionInput(): void { this.deletionPassword = ''; this.deletionRecovery = ''; this.deletionOTP = ''; }
  private closeDeletion(): void { this.clearDeletionInput(); this.deletionLife++; this.deletionBusy = false; }
  private deletionPaused(): boolean { const family = this.auth?.currentFamily() ?? ''; return Boolean(family) && this.deletionRows.some((row: DeletionIntent) => row.meta.family === family && DeletionRules.blocked(row)); }
  private deletionRepository(): DeletionStore {
    const identity = this.authConnectionIdentity();
    return new DeletionStore(this.store, this.profile, () => identity === this.authConnectionIdentity(), (row: DeletionIntent) => {
      this.auth?.deletionPause(row.meta.family, DeletionRules.blocked(row));
      const at = this.deletionRows.findIndex((item: DeletionIntent) => item.meta.id === row.meta.id);
      this.deletionRows = at < 0 ? [row, ...this.deletionRows] : this.deletionRows.map((item: DeletionIntent) => item.meta.id === row.meta.id ? row : item);
      if (this.deletionPaused()) { this.stopSync(); this.clearSyncRAM(); this.rows = []; this.conversations = []; }
    });
  }
  private async eraseDeletionLocal(row: DeletionIntent): Promise<void> {
    await this.deletionRepository().eraseLocal(row, async (plan: DeletionLocalPlan) => DeletionFiles.clear(getContext(this) as common.UIAbilityContext, plan));
  }
  private async loadDeletionRows(): Promise<void> { const repository = this.deletionRepository(); await repository.open(); this.deletionRows = await repository.list(); }
  private openDeletion(): void { this.clearDeletionInput(); this.deletionShown = 20; this.deletionNotice = ''; this.page = 'deletion'; this.run(async () => this.loadDeletionRows()); }
  private deletionVersion(row: DeletionIntent): string { return row.result ? DeletionRules.result(row.result, row).requestVersion : ''; }
  private deletionUntil(row: DeletionIntent): string { return row.result ? DeletionRules.result(row.result, row).cancelUntil ?? '' : ''; }
  private submitDeletion(id: string, mode: string, version: string = ''): void {
    if (this.deletionBusy || !this.foregroundVisible || !this.appVisible) { return; }
    let password = this.deletionPassword; let recovery = this.deletionRecovery; let otp = this.deletionOTP;
    this.clearDeletionInput(); this.deletionBusy = true; this.deletionNotice = '';
    const life = this.deletionLife; const identity = this.authConnectionIdentity(); const family = this.auth?.currentFamily() ?? ''; const authOwner = this.authOwner;
    const current = (): boolean => this.foregroundVisible && this.appVisible && this.page === 'deletion' && life === this.deletionLife && identity === this.authConnectionIdentity() && authOwner === this.authOwner && family === (this.auth?.currentFamily() ?? '');
    const ports: DeletionPorts = {
      profile: this.profile, origin: this.base, deployment: this.deploymentId, epoch: this.restoreEpoch, current: current,
      owner: (): PasswordOwner | undefined => { const pair = this.auth?.passwordOwner(); if (!pair || this.demo) { return undefined; } return { user: pair.userId, username: pair.username, device: pair.deviceId, family: pair.familyId, namespace: Isolation.namespace(pair.profile, pair.deployment, pair.userId, false, pair.epoch, pair.deviceId, pair.sourceContextId), token: pair.token }; },
      uuid: () => util.generateRandomUUID(true), repository: this.deletionRepository(),
      prepare: async () => { await this.api().request<AuthContext>('/auth/context'); },
      bootstrap: async () => { await this.verifySavedCA(); return new Api(this.base, '', this.caPath).request<Bootstrap>('/bootstrap'); },
      request: async (route: string, body: string, token: string) => { if (!current()) { throw new Error('DELETION_CONTEXT_CHANGED'); } return JSON.stringify(await new Api(this.base, token, this.caPath).request<Object>(route, 'POST', body)); },
      stop: async (row: DeletionIntent) => { if (!current()) { return; } if (row.state === 'completed') { await this.eraseDeletionLocal(row); } if (this.auth?.currentFamily() === row.meta.family) { await this.auth.clear(this.auth.stamp()); } }
    };
    const service = new Deletion(ports);
    const action = mode === 'query' || mode === 'cancel' ? service.rights(id, password, recovery, otp, mode === 'cancel', version) : service.request(id, password, otp, mode === 'retry');
    action.then(async () => { if (identity === this.authConnectionIdentity() && life === this.deletionLife) { await this.loadDeletionRows(); } })
      .catch((error: Error) => { if (current()) { this.deletionNotice = this.d(error.message === 'DELETION_UNAVAILABLE' ? 'unavailable' : error.message === 'DELETION_ORIGINAL_PENDING' ? 'blocked' : 'error'); } })
      .finally(() => { password = ''; recovery = ''; otp = ''; if (life === this.deletionLife) { this.deletionBusy = false; } });
  }
  private g(key: string): string { return GroupText.text(this.language, key); }
  private groupManager(): boolean { return Boolean(this.groupSnapshot) && GroupRules.manager(this.groupSnapshot); }
  private groupOwner(): boolean { return Boolean(this.groupSnapshot) && SyncJSON.parse(this.groupSnapshot).get('role').string() === 'owner' && !SyncJSON.parse(this.groupSnapshot).get('readOnly').bool(); }
  private groupRecover(): boolean { return Boolean(this.groupSnapshot) && SyncJSON.parse(this.groupSnapshot).get('canRecover').value === true; }
  private groupField(raw: string, key: string): string { const value = SyncJSON.parse(raw).get(key); return value.value === null ? '' : typeof value.value === 'string' ? value.string() : value.raw(); }
  private closeGroupTools(): void { this.groupLife++; this.groupService?.clear(); this.groupService = undefined; this.groupCode = ''; this.groupOTP = ''; this.groupBusy = false; }
  private openGroupTools(id: string): void {
    this.closeGroupTools(); this.groupControlId = id; this.groupSnapshot = ''; this.groupInvites = []; this.groupRequests = []; this.groupOperations = []; this.members = []; this.groupMembersNext = ''; this.groupInvitesNext = ''; this.groupRequestsNext = ''; this.groupShown = 20; this.groupNotice = ''; this.page = 'groupTools';
    const life = this.groupLife; const identity = this.authConnectionIdentity(); const family = this.auth?.currentFamily() ?? ''; const owner = this.authOwner;
    const current = (): boolean => this.page === 'groupTools' && this.foregroundVisible && this.appVisible && life === this.groupLife && identity === this.authConnectionIdentity() && family === (this.auth?.currentFamily() ?? '') && owner === this.authOwner;
    const environment: GroupEnvironment = { profile: this.profile, deployment: this.deploymentId, user: this.userId, epoch: this.restoreEpoch, namespace: this.namespace(), family: family };
    const repository = new GroupStore(this.store, environment, current);
    const ports: GroupPorts = { environment: environment, current: current, uuid: () => util.generateRandomUUID(true), repository: repository,
      codec: { encode: (text: string) => new util.Base64Helper().encodeToStringSync(new util.TextEncoder().encodeInto(text)), decode: (text: string) => util.TextDecoder.create('utf-8', { fatal: true }).decodeToString(new util.Base64Helper().decodeSync(text)) },
      request: async (route: string, method: string, body: string) => { if (!current()) { throw new Error('GROUP_CONTEXT_CHANGED'); } return JSON.stringify(await this.api().request<Object>(route, method as http.RequestMethod, body)); }
    };
    const service = new GroupTools(ports); this.groupService = service;
    this.groupBusy = true;
    repository.open().then(async () => { if (!current()) { return; } this.groupOperations = await service.list(); if (id) { await this.loadGroupControl(service); } })
      .catch((_error: Error) => { if (current()) { this.groupNotice = this.g('error'); } })
      .finally(() => { if (life === this.groupLife) { this.groupBusy = false; } });
  }
  private async loadGroupControl(service: GroupTools): Promise<void> {
    const life = this.groupLife; const snapshot = await service.load(this.groupControlId); if (life !== this.groupLife) { return; } this.groupSnapshot = snapshot;
    const policy = SyncJSON.parse(snapshot).get('policy'); this.groupMuteAll = policy.get('muteAll').bool(); this.groupSlow = String(policy.get('slowSeconds').integer(3600)); this.groupCapacity = String(policy.get('maxMembers').integer(10000));
    await this.loadGroupPage(service, 'members', true);
    if (this.groupManager()) { await this.loadGroupPage(service, 'invites', true); await this.loadGroupPage(service, 'join-requests', true); }
  }
  private async loadGroupPage(service: GroupTools, kind: string, first: boolean): Promise<void> {
    const life = this.groupLife; const cursor = first ? '' : kind === 'members' ? this.groupMembersNext : kind === 'invites' ? this.groupInvitesNext : this.groupRequestsNext;
    if (!first && !cursor) { return; } const page = await service.page(this.groupControlId, kind, cursor); if (life !== this.groupLife) { return; }
    if (kind === 'members') { const incoming = page.items.map((raw: string) => JSON.parse(raw) as GroupMember); const old = first ? [] : this.members; this.members = old.filter((item: GroupMember) => !incoming.some((row: GroupMember) => row.id === item.id)).concat(incoming); this.groupMembersNext = page.next; }
    else if (kind === 'invites') { const old = first ? [] : this.groupInvites; this.groupInvites = old.filter((raw: string) => !page.items.some((row: string) => this.groupField(row, 'id') === this.groupField(raw, 'id'))).concat(page.items); this.groupInvitesNext = page.next; }
    else { const old = first ? [] : this.groupRequests; this.groupRequests = old.filter((raw: string) => !page.items.some((row: string) => this.groupField(row, 'id') === this.groupField(raw, 'id'))).concat(page.items); this.groupRequestsNext = page.next; }
  }
  private groupMore(kind: string): void {
    const service = this.groupService; if (!service || this.groupBusy) { return; } const life = this.groupLife; this.groupBusy = true;
    this.loadGroupPage(service, kind, false).catch((_error: Error) => { if (life === this.groupLife) { this.groupNotice = this.g('error'); } }).finally(() => { if (life === this.groupLife) { this.groupBusy = false; } });
  }
  private stageGroup(action: string, target?: GroupMember, fields: string = '{}', label: string = ''): void {
    if (!this.groupService || this.groupBusy) { return; } const row = this.groupService.prepare(action, target, fields); const life = this.groupLife;
    AlertDialog.show({ title: this.g('confirm'), message: (label || this.g(action)) + (target ? ' · ' + target.displayName : ''), primaryButton: { value: this.g('confirm'), action: () => { if (life === this.groupLife) { this.performGroup(row, true); } } }, secondaryButton: { value: this.t('cancel'), action: () => {} } });
  }
  private joinGroupCode(): void { if (!this.groupService || this.groupBusy) { return; } try { this.performGroup(this.groupService.joining(this.groupCode), true); } catch (_) { this.groupNotice = this.g('error'); } }
  private performGroup(row: GroupIntent, retry: boolean): void {
    const service = this.groupService; if (!service || this.groupBusy) { return; } let code = this.groupCode; let otp = this.groupOTP; const life = this.groupLife; this.groupCode = ''; this.groupOTP = ''; this.groupBusy = true; this.groupNotice = '';
    service.perform(row, retry, code, otp).then(async (result: string | undefined) => {
      if (life !== this.groupLife) { return; } const operations = await service.list(); if (life !== this.groupLife) { return; } this.groupOperations = operations; this.groupNotice = this.g(result && row.kind === 'join' ? this.groupJoinLabel(result) : result ? 'applied' : 'unknown');
      if (result) { const conversations = (await this.api().request<ConversationList>('/conversations')).items; if (life !== this.groupLife) { return; } this.conversations = conversations; if (this.conversation) { this.conversation = this.conversations.find((item: Conversation) => item.id === this.conversation?.id); } }
    }).catch(async (_error: Error) => { if (life === this.groupLife) { this.groupNotice = this.g('error'); try { const operations = await service.list(); if (life === this.groupLife) { this.groupOperations = operations; } } catch (_) {} } })
      .finally(() => { code = ''; otp = ''; if (life === this.groupLife) { this.groupBusy = false; } });
  }
  private groupJoinLabel(raw: string): string { const state = SyncJSON.parse(raw).get('state').string(); return state === 'pending' ? 'joinPending' : state === 'approved' ? 'joinApproved' : state === 'rejected' ? 'joinRejected' : 'applied'; }
  private copyGroupCode(id: string): void {
    const service = this.groupService; if (!service) { return; } const life = this.groupLife;
    try { const code = service.share(id); pasteboard.getSystemPasteboard().setData(pasteboard.createData(pasteboard.MIMETYPE_TEXT_PLAIN, code)).then(() => { if (life === this.groupLife && service === this.groupService) { this.groupNotice = this.g('copied'); } }).catch((_error: Error) => { if (life === this.groupLife && service === this.groupService) { this.groupNotice = this.g('error'); } }); }
    catch (_) { if (life === this.groupLife) { this.groupNotice = this.g('reloadRequired'); } }
  }
  private async loadGroupMembers(more: boolean): Promise<void> {
    const conversation = this.conversation; if (!conversation) { return; } const cursor = more ? this.groupMembersNext : ''; if (more && !cursor) { return; }
    const identity = this.authConnectionIdentity(); const page = this.demo ? { items: [], nextCursor: '' } as MemberList : await this.api().request<MemberList>('/conversations/' + conversation.id + '/members?limit=50' + (cursor ? '&after=' + cursor : ''));
    if (identity !== this.authConnectionIdentity() || this.conversation?.id !== conversation.id) { return; } const next = page.nextCursor ?? ''; if (next && (!SyncRules.uuid(next) || next === cursor)) { throw new Error('GROUP_INVALID_CURSOR'); }
    const previous = more ? this.members : []; this.members = previous.filter((old: GroupMember) => !page.items.some((item: GroupMember) => item.id === old.id)).concat(page.items); this.groupMembersNext = next;
  }
  private exportPageChanged(): void {
    if (this.page !== 'receipts') { this.closeReceipt(); }
    if (this.page !== 'migration') { this.closeMigration(); }
    if (this.page !== 'exports') { this.closeExport(); }
    if (this.page !== 'password') { this.clearPasswordInput(); this.passwordLife++; this.passwordBusy = false; }
    if (this.page !== 'deletion') { this.closeDeletion(); }
    if (this.page !== 'groupTools') { this.closeGroupTools(); }
  }
  private closeExport(): void {
    this.exportLife++; this.exportTransfer++; this.exportService = undefined; this.exportBusy = false; this.exportDownloading = false;
    this.exportFiles?.dispose(); this.exportFiles = undefined; this.exportRows = []; this.exportViews = {}; this.exportSupported = false; this.exportNotice = '';
  }
  private stopExportDownload(): void { this.exportTransfer++; this.exportFiles?.dispose(); this.exportFiles = undefined; this.exportDownloading = false; this.exportNotice = this.e('exportDownloadStopped'); }
  private async openExports(): Promise<void> {
    this.closeExport(); this.page = 'exports'; this.conversation = undefined;
    if (this.demo || !this.token || !this.auth?.currentFamily()) { this.exportNotice = this.e('exportConnect'); return; }
    await this.runExport(async () => {
      const life = this.exportLife; const namespace = this.namespace(); const stamp = this.auth?.stamp() ?? ''; const family = this.auth?.currentFamily() ?? '';
      const current = (): boolean => this.page === 'exports' && life === this.exportLife && namespace === this.namespace() && stamp === (this.auth?.stamp() ?? '') && !this.demo && this.token.length > 0;
      const physical = await this.store.value('__sync_store__', 'physical'); if (!current() || !SyncRules.uuid(physical)) { throw new Error('EXPORT_STORE_CHANGED'); }
      const api = this.api(); const codec = NativeServices.syncCodec();
      const repository = new ExportStore(this.store, JSON.stringify([namespace, family]), this.restoreEpoch, physical, codec, current); await repository.open();
      const ports: ExportPorts = {
        deployment: this.deploymentId, epoch: this.restoreEpoch, physical: physical, repository: repository, codec: codec, uuid: () => util.generateRandomUUID(true), current: current,
        request: async (route: string, method: string, body: string) => JSON.stringify(await api.request<Object>(route, method as http.RequestMethod, body)),
        download: async (job: string, ticket: string) => {
          const transfer = ++this.exportTransfer; const stillCurrent = (): boolean => current() && transfer === this.exportTransfer && this.foregroundVisible && this.appVisible;
          const files = new ExportFiles(getContext(this) as common.UIAbilityContext, stillCurrent, namespace); this.exportFiles = files;
          return api.dataExport(job, ticket, files, stillCurrent);
        }
      };
      this.exportService = new ExportService(ports); await this.reloadExports();
    });
  }
  private async runExport(action: () => Promise<void>): Promise<void> {
    if (this.exportBusy) { return; } const life = this.exportLife; const namespace = this.namespace(); const stamp = this.auth?.stamp() ?? ''; this.exportBusy = true; this.exportNotice = '';
    try { await action(); }
    catch (error) {
      if (life === this.exportLife && this.page === 'exports' && namespace === this.namespace() && stamp === (this.auth?.stamp() ?? '')) {
        const code = error instanceof ApiFailure ? error.code : (error as Error).message;
        if (this.exportNotice !== this.e('exportDownloadStopped')) { this.exportNotice = this.e(code === 'EXPORT_FILE_LIMIT' ? 'exportLimit' : code === 'EXPORT_CAPABILITY_REQUIRED' ? 'exportRestricted' : code === 'EXPORT_RATE_LIMITED' ? 'exportLimited' : code === 'EXPORT_STORE_CHANGED' ? 'exportStoreNotReady' : code === 'EXPORT_FILE_INVALID' || code === 'EXPORT_FILE_INCOMPLETE' ? 'exportFileInvalid' : code === 'EXPORT_SAVE_FAILED' ? 'exportSaveFailed' : 'exportNotConfirmed'); }
      }
    } finally {
      if (life === this.exportLife && namespace === this.namespace() && stamp === (this.auth?.stamp() ?? '')) {
        try { if (this.exportService) { this.exportRows = await this.exportService.list(); } } catch (_) {}
        if (life === this.exportLife) { this.exportBusy = false; }
      }
    }
  }
  private async reloadExports(): Promise<void> {
    const service = this.exportService; if (!service) { throw new Error('EXPORT_STORE_CHANGED'); }
    this.exportRows = await service.list(); this.exportSupported = await service.capability(false); service.check();
    const views: Record<string, ExportView> = {};
    for (const row of this.exportRows.slice(0, this.exportVisibleCount)) { try { views[row.id] = await service.status(row.id); } catch (_) { service.check(); } }
    service.check(); this.exportViews = views;
  }
  private async exportAction(id: string, action: string): Promise<void> {
    const service = this.exportService; if (!service) { throw new Error('EXPORT_STORE_CHANGED'); }
    const view = action === 'create' ? await service.create() : action === 'cancel' ? await service.cancel(id) : await service.resume(id, action === 'retry'); service.check();
    this.exportRows = await service.list();
    await this.reloadExports();
  }
  private confirmExportCancel(row: ExportIntent): void {
    const service = this.exportService;
    AlertDialog.show({ message: this.e('exportCancelConfirm'), primaryButton: { value: this.e('exportKeep'), action: () => {} }, secondaryButton: { value: this.e('exportConfirmCancel'), action: () => { if (service && this.exportService === service) { this.runExport(async () => this.exportAction(row.id, 'cancel')); } } } });
  }
  private async downloadExport(row: ExportIntent): Promise<void> {
    const service = this.exportService; if (!service) { throw new Error('EXPORT_STORE_CHANGED'); } this.exportDownloading = true;
    try {
      const prepared = await service.delivery(row.id); const files = this.exportFiles; service.check(); if (!files) { throw new Error('EXPORT_CONTEXT_CHANGED'); }
      const saved = await files.save(getContext(this) as common.UIAbilityContext, prepared, async () => service.authorizeSave(row.id, prepared)); service.check();
      this.exportNotice = this.e(saved ? 'exportSaved' : 'exportSaveCancelled');
    } finally { if (this.exportService === service) { this.exportFiles?.dispose(); this.exportFiles = undefined; this.exportDownloading = false; } }
  }
  private syncIdentity(): SyncIdentity { return { namespace: this.namespace(), profile: this.profile, deployment: this.deploymentId, user: this.userId, source: this.sourceContextId, epoch: this.restoreEpoch, device: this.deviceId }; }
  private ensureSync(): SyncEngine {
    const identity = this.syncIdentity(); const auth = this.auth?.stamp() ?? ''; const life = this.syncLife;
    const key = identity.namespace + '|' + auth + '|' + life;
    if (this.synchronizer && this.syncKey === key) { return this.synchronizer; }
    this.synchronizer?.cancel(); this.syncKey = key;
    const api = this.api();
    const ports: SyncEnginePorts = {
      store: this.syncData, codec: NativeServices.syncCodec(), uuid: () => util.generateRandomUUID(true),
      current: () => this.namespace() === identity.namespace && (this.auth?.stamp() ?? '') === auth && this.syncLife === life && this.token.length > 0 && !this.demo && this.foregroundVisible && this.appVisible,
      request: async (path: string, method: string, body: string) => JSON.stringify(await api.request<Object>(path, method as http.RequestMethod, body)),
      cleanup: () => this.drainSyncCleanup(),
      committed: (messages:string[]) => this.offerReceiptAutomatic(messages),
      historyChanged: () => {this.closeMigration();this.migrationHistory=[];this.migrationQualified=[];}
    };
    this.synchronizer = new SyncEngine(identity, ports); return this.synchronizer;
  }
  private ensureSelf(): SelfState {
    const identity = this.syncIdentity(); const auth = this.auth?.stamp() ?? ''; const life = this.syncLife;
    const key = identity.namespace + '|' + auth + '|' + life;
    if (this.selfService && this.selfKey === key) { return this.selfService; }
    const api = this.api(); const ports: SelfPorts = {
      identity: identity, store: this.store, sync: this.syncData, codec: NativeServices.syncCodec(), uuid: () => util.generateRandomUUID(true),
      current: () => identity.namespace === this.namespace() && (this.auth?.stamp() ?? '') === auth && life === this.syncLife && this.token.length > 0 && !this.demo && this.foregroundVisible && this.appVisible,
      request: async (route: string, method: string, body: string) => JSON.stringify(await api.request<Object>(route, method as http.RequestMethod, body)),
      resync: async () => { await this.ensureSync().run(); }
    };
    this.selfKey = key; this.selfService = new SelfState(ports); return this.selfService;
  }
  private async loadSelfMetadata(): Promise<void> {
    const identity = this.syncIdentity(); const control = await this.syncData.syncControl(identity);
    if (identity.namespace !== this.namespace()) { return; }
    this.selfSupported = control?.schema === '2';
    if (!this.selfSupported) { this.selfViews = {}; return; }
    const views = JSON.parse('{}') as Record<string, SelfView>;
    for (const raw of await this.syncData.objects(identity, 'conversation_view')) { const value = SyncJSON.parse(raw); if (value.has('removed') && value.get('removed').bool()) { continue; } const conversation = value.get('conversationId').string(); views[conversation] = SelfRules.view(raw, identity, conversation); }
    const fonts = await this.syncData.objects(identity, 'preferences');
    const drafts: SelfDraftItem[] = [];
    for (const raw of await this.syncData.objects(identity, 'draft')) {
      const value = SyncJSON.parse(raw); if (value.has('removed') && value.get('removed').bool()) { continue; }
      const conversation = value.get('conversationId').string(); const draft = SelfRules.draft(raw, identity, conversation);
      const item: SelfDraftItem = { conversation: conversation, title: this.conversations.find((row: Conversation) => row.id === conversation)?.title ?? conversation.slice(0, 8), writable: draft.writable }; drafts.push(item);
    }
    if (identity.namespace !== this.namespace()) { return; }
    this.selfViews = views;
    this.selfDraftEntries = drafts;
    if (fonts.length) {
      const value = SelfRules.preference(fonts[0], identity); const key = identity.namespace + '|' + value.version + '|' + value.fontPreset;
      this.cloudFont = value.fontPreset;
      if (this.cloudFontKey !== key) { this.cloudFontKey = key; this.fontScale = value.fontPreset === 'small' ? 0.9 : value.fontPreset === 'large' ? 1.2 : 1; }
    }
  }
  private orderSelfConversations(rows: Conversation[]): Conversation[] {
    const visible = rows.filter((row: Conversation) => { const view = this.selfViews[row.id]; return this.selfAll || !view || (!view.hidden && !view.archived); });
    return visible.sort((a: Conversation, b: Conversation) => {
      const first = this.selfViews[a.id]?.pinnedRank ?? null; const second = this.selfViews[b.id]?.pinnedRank ?? null;
      if (first !== null && second === null) { return -1; } if (first === null && second !== null) { return 1; }
      return first !== null && second !== null && first !== second ? first - second : a.id.localeCompare(b.id);
    });
  }
  private selfMuted(conversation: string): boolean { const until = this.selfViews[conversation]?.mutedUntil; return !!until && Date.parse(until) > Date.now(); }
  private async openSelfState(): Promise<void> {
    if (this.demo || !this.selfSupported) { return; }
    const namespace = this.namespace(); const conversation = this.conversation;
    const service = this.ensureSelf(); this.page = 'self'; this.selfDraft = undefined; this.selfRead = undefined; this.selfView = undefined;
    if (conversation) {
      const view = await service.view(conversation.id); const draft = await service.draft(conversation.id);
      if (namespace !== this.namespace() || this.page !== 'self' || this.conversation?.id !== conversation.id) { return; }
      this.selfView = view; this.selfDraft = draft;
      if (conversation.state !== 'local_only') { const read = await service.read(conversation.id); if (namespace === this.namespace() && this.page === 'self' && this.conversation?.id === conversation.id) { this.selfRead = read; } }
    }
    const operations = await service.operations();
    if (namespace === this.namespace() && this.page === 'self') { this.selfOperations = operations.filter((row: SelfOperation) => row.state !== 'completed').map((row: SelfOperation) => { const item: SelfOperationItem = { id: row.id, kind: row.kind, state: row.state, reason: row.reason ?? '' }; return item; }); }
  }
  private async finishSelf(result: SelfResult): Promise<void> { this.info = SelfText.state(this.language, result.status); await this.reload(); if (this.page === 'self') { await this.openSelfState(); } }
  private async openOwnDraft(item: SelfDraftItem): Promise<void> {
    const current = this.conversations.find((row: Conversation) => row.id === item.conversation);
    if (current) { await this.open(current); }
    else {
      const local: Conversation = { id: item.conversation, kind: 'personal-metadata', title: this.s('draft') + ' · ' + item.title, memberIds: [], cursor: '0', readOnly: true, state: 'local_only' };
      this.conversation = local; this.rows = []; this.observedVisible.clear(); this.cloudReplyVersion = ''; this.draftRevision++;
      const raw = await this.store.value(this.namespace(), 'draft:' + local.id); this.draft = raw ? (JSON.parse(raw) as DraftState).text : ''; this.replyId = ''; this.editId = '';
    }
    await this.openSelfState();
  }
  private confirmCloudClear(): void {
    const conversation = this.conversation; const draft = this.selfDraft; if (!conversation || !draft) { return; }
    const namespace = this.namespace();
    AlertDialog.show({ title: this.s('clear'), message: this.s('help'), autoCancel: false,
      primaryButton: { value: this.t('cancel'), action: () => {} },
      secondaryButton: { value: this.t('continue'), action: () => this.run(async () => { if (namespace !== this.namespace() || this.conversation?.id !== conversation.id) { return; } await this.finishSelf(await this.ensureSelf().clearDraft(conversation.id, draft.version)); }) }
    });
  }
  private async changeSelfView(key: string): Promise<void> {
    const conversation = this.conversation; const current = this.selfView; if (!conversation || !current) { return; }
    const changes = JSON.parse('{}') as Record<string, Object | null>;
    if (key === 'hidden') { changes[key] = !current.hidden; }
    else if (key === 'archived') { changes[key] = !current.archived; }
    else if (key === 'manualUnread') { changes[key] = !current.manualUnread; }
    else if (key === 'pinnedRank') { changes[key] = current.pinnedRank === null ? 0 : null; }
    else if (key === 'mutedUntil') { changes[key] = current.mutedUntil && Date.parse(current.mutedUntil) > Date.now() ? null : new Date(Date.now() + 3600000).toISOString(); }
    await this.finishSelf(await this.ensureSelf().changeView(conversation.id, changes, current.version, current.manualUnreadGeneration));
  }
  private async saveCloudDraft(reply: boolean): Promise<void> {
    const conversation = this.conversation; if (!conversation) { return; }
    if (reply && (!this.replyId || !this.cloudReplyVersion)) { throw new Error('SELF_REPLY_RESELECT_REQUIRED'); }
    if (!this.selfDraft) { return; }
    await this.finishSelf(await this.ensureSelf().saveDraft(conversation.id, this.draft, reply ? this.replyId : '', reply ? this.cloudReplyVersion : '', this.selfDraft.version));
  }
  private async adoptCloudDraft(recovery: boolean): Promise<void> {
    const conversation = this.conversation; const shown = this.selfDraft; if (!conversation || !shown) { return; }
    await this.draftTail; const namespace = this.namespace(); const revision = this.draftRevision; const expected = await this.store.value(namespace, 'draft:' + conversation.id);
    const latest = await this.ensureSelf().draft(conversation.id);
    if (namespace !== this.namespace() || this.conversation?.id !== conversation.id || this.draftRevision !== revision) { throw new Error('SELF_LOCAL_INPUT_CHANGED'); }
    if (latest.version !== shown.version || (recovery ? latest.recovery?.operationId !== shown.recovery?.operationId || latest.recovery?.ownedText !== shown.recovery?.ownedText : latest.ownedText !== shown.ownedText)) { this.selfDraft = latest; throw new Error('SELF_DRAFT_REVIEW_REQUIRED'); }
    const replacement = recovery ? latest.recovery?.ownedText : latest.contentState === 'empty' ? '' : latest.ownedText;
    if (replacement === undefined || replacement === null) { throw new Error('SELF_DRAFT_UNAVAILABLE'); }
    await this.ensureSelf().adopt(conversation.id, expected, replacement, () => namespace === this.namespace() && this.conversation?.id === conversation.id && this.draftRevision === revision);
    if (namespace === this.namespace() && this.conversation?.id === conversation.id && this.draftRevision === revision) { this.draftRevision++; this.draft = replacement; this.replyId = ''; this.editId = ''; this.editVersion = ''; this.cloudReplyVersion = ''; this.mentionedUserIds = []; this.mentionAll = false; }
  }
  private observeVisibility(row: Message, visible: boolean, ratio: number): void {
    const conversation = this.conversation;
    if (!visible || ratio < 0.5 || !conversation || conversation.state === 'local_only' || !this.foregroundVisible || !this.appVisible || !this.selfSupported || row.deleted || row.contentUnavailable || row.forwardUnavailable || row.state !== 'received' || row.senderId === this.userId) { this.observedVisible.delete(row.id); return; }
    this.observedVisible.set(row.id, conversation.membershipGeneration ?? '');
  }
  private async observeDisplayed(): Promise<void> {
    const conversation = this.conversation; if (!conversation || conversation.state === 'local_only') { return; }
    const generation = conversation.membershipGeneration ?? '';
    const ids = this.rows.filter((row: Message) => this.observedVisible.get(row.id) === generation && !row.deleted && !row.contentUnavailable && !row.forwardUnavailable && row.state === 'received').slice(0, 200).map((row: Message) => row.id);
    if (!ids.length) { return; } await this.finishSelf(await this.ensureSelf().observe(conversation.id, ids, generation));
  }
  private clearSyncRAM(): void {
    this.forwardViews.clear();
    this.rows = this.rows.filter((row: Message) => row.state === 'pending' || row.state === 'frozen');
    this.lookupRows = []; this.lookupCursor = ''; this.lookupMore = false;
    this.forwardSource = undefined; this.announcement = undefined; this.announcementCandidate = undefined;
    this.selfDraft = undefined; this.selfRead = undefined; this.observedVisible.clear();
  }
  private async drainSyncCleanup(): Promise<void> {
    for(const item of await ReceiptCleanup.plans(this.store)){ReceiptFiles.eraseOwned(getContext(this) as common.UIAbilityContext,item[0],[item[1]]);await ReceiptCleanup.done(this.store,item[0],item[1]);}
    for (const item of await MigrationStore.cleanups(this.store)) { MigrationFiles.eraseOwned(getContext(this) as common.UIAbilityContext, item[0], [item[1]]); await MigrationStore.cleaned(this.store,item[0],item[1]); }
    const work = await this.syncData.cleanups();
    if (work.length) { this.clearSyncRAM(); }
    for (const item of work) {
      if (item.key === '__notification__') { await NativeServices.clearSyncNotification(); }
      else { NativeServices.removeSyncCache(getContext(this) as common.UIAbilityContext, item.path); }
      await this.syncData.cleaned(item);
    }
  }
  private async showLocalHistory(): Promise<void> {
    const identity = this.syncIdentity();
    const rows = (await this.syncData.retainedConversations(identity)).map((raw: string) => JSON.parse(raw) as Conversation);
    if (identity.namespace !== this.namespace()) { return; }
    this.conversations = rows;
    this.localEntries = true; this.conversation = undefined; this.rows = []; this.page = 'chats';
  }
  private authConnectionIdentity(): string { return [this.base, this.caPath, this.profile, this.deploymentId, this.restoreEpoch].join('|'); }
  private ensureAuth(): AuthCoordinator {
    const identity = this.authConnectionIdentity();
    if (this.auth && this.authIdentity === identity) { return this.auth; }
    const owner = util.generateRandomUUID(true);
    this.authOwner = owner; this.authIdentity = identity;
    const connection: AuthConnection = { base: this.base, caPath: this.caPath, profile: this.profile, deployment: this.deploymentId, epoch: this.restoreEpoch };
    const raw = new Api(connection.base, '', connection.caPath);
    const ports: AuthPorts = {
      current: () => this.authOwner === owner && this.authConnectionIdentity() === identity,
      read: (scope: string, key: string) => this.store.value(scope, key),
      write: (scope: string, values: AuthStateEntry[]) => this.store.saveAuthState(scope, values),
      commit: (scope: string, oldNamespaces: string[], namespace: string, values: AuthStateEntry[]) => this.store.commitAuthBinding(scope, oldNamespaces, namespace, values),
      freeze: (namespace: string, reason: string) => this.store.freeze(namespace, reason),
      load: (alias: string) => this.secure.load(alias),
      save: (alias: string, value: string) => this.secure.save(alias, value),
      remove: (alias: string) => this.secure.remove(alias),
      bootstrap: async () => { await this.verifySavedCA(); return raw.request<Bootstrap>('/bootstrap'); },
      send: async (path: string, body: string) => {
        if (this.authOwner !== owner || this.authConnectionIdentity() !== identity) { throw new Error('SESSION_SCOPE_CHANGED'); }
        return raw.request<AuthResult>(path, 'POST', body);
      },
      context: (token: string) => new Api(connection.base, token, connection.caPath).request<AuthContext>('/auth/context'),
      logout: async (token: string) => { await new Api(connection.base, token, connection.caPath).request<Object>('/auth/logout', 'POST', '{}'); },
      nonce: () => util.generateRandomUUID(true),
      publish: (pair: CredentialPair | undefined, user: User | undefined) => {
        if (this.authOwner !== owner || this.authConnectionIdentity() !== identity) { return; }
        if (!pair || !user) { this.resetSessionView(); return; }
        if(this.receiptAutoIdentity&&this.receiptAutoIdentity.family!==pair.familyId){this.receiptAutomatic?.stop();this.receiptAutomatic=undefined;this.receiptAutoIdentity=undefined;this.receiptAutoEnabled=false;}
        this.token = pair.token; this.userId = pair.userId; this.deviceId = pair.deviceId; this.sourceContextId = pair.sourceContextId;
        this.sessionAlias = 'JobsIM|' + this.namespace(); this.me = user; this.username = pair.username; this.demo = false;
      },
      features: (bootstrap: Bootstrap) => { if (this.authOwner === owner) { this.applyFeatures(bootstrap); } }
    };
    this.auth = new AuthCoordinator(connection, ports);
    return this.auth;
  }
  private async run(action: () => Promise<void>): Promise<void> {
    if (this.busy) { return; }
    const actionNamespace = this.namespace(); const actionToken = this.token;
    this.info = ''; this.busy = true;
    try { await action(); this.offline = false; }
    catch (error) {
      if ((error as Error).message === 'SESSION_SCOPE_CHANGED' && actionNamespace !== this.namespace()) { return; }
      this.offline = true; this.info = (error as Error).message;
      if (!this.demo && actionNamespace === this.namespace() && this.userId && ((error instanceof ApiFailure && (error.status === 0 || error.status >= 500 || ['NOT_A_MEMBER', 'HISTORY_NOT_ALLOWED'].includes(error.code))) || this.info.startsWith('SYNC_'))) {
        await this.syncData.syncRestrict(this.syncIdentity(), 'SYNC_UNAVAILABLE', false);
        this.syncRestricted = true; this.clearSyncRAM(); this.info = SyncText.text(this.language, 'restricted');
      }
      if (!this.demo) {
        for (const conversation of this.conversations) {
          for (const row of await this.store.list(this.namespace(), conversation.id, false)) { if (row.forwarded) { await this.clearForwardSidecar(row.id); } }
          await this.store.maskForwards(this.namespace(), conversation.id);
        }
        this.rows = this.rows.map((row: Message) => row.forwarded && row.state !== 'pending' && row.state !== 'frozen' ? Rules.unavailableForward(row) : row);
        this.lookupRows = this.lookupRows.map((row: Message) => row.forwarded ? Rules.unavailableForward(row) : row);
      }
      if (!this.auth && this.info.includes('HTTP 401') && actionNamespace === this.namespace() && actionToken === this.token) { await this.clearSession(); }
      if (!this.auth && this.info.includes('RESTORE_EPOCH_CHANGED') && actionNamespace === this.namespace()) { await this.store.freeze(this.namespace()); await this.clearSession(); }
    }
    finally { this.busy = false; }
  }
  private async saveSetting(key: string, value: string): Promise<void> {
    await this.prefs?.put(key, value); await this.prefs?.flush();
  }
  private applyTheme(value: string): void {
    this.theme = value;
    const context = getContext(this) as common.UIAbilityContext;
    const mode = value === 'dark' ? ConfigurationConstant.ColorMode.COLOR_MODE_DARK : value === 'light' ? ConfigurationConstant.ColorMode.COLOR_MODE_LIGHT : ConfigurationConstant.ColorMode.COLOR_MODE_NOT_SET;
    context.getApplicationContext().setColorMode(mode);
    this.dark = value === 'dark' || (value === 'system' && context.config.colorMode === ConfigurationConstant.ColorMode.COLOR_MODE_DARK);
  }
  private async connect(): Promise<void> {
    const base = Isolation.endpoint(this.connectionAddress);
    if (this.expectedId.length === 0) { throw new Error(this.t('expected')); }
    let caPath = '';
    if (this.caPem.length > 0) {
      const encoded = this.caPem.replace(/-----BEGIN CERTIFICATE-----|-----END CERTIFICATE-----|\s/g, '');
      const der = new util.Base64Helper().decodeSync(encoded);
      const digest = cryptoFramework.createMd('SHA256');
      await digest.update({ data: der });
      const result = await digest.digest();
      const actual = Array.from(result.data).map((value: number) => value.toString(16).padStart(2, '0')).join('');
      if (actual !== this.caFingerprint.replace(/:/g, '').toLowerCase()) { throw new Error('CA fingerprint mismatch'); }
      const context = getContext(this) as common.UIAbilityContext;
      caPath = context.filesDir + '/instance-' + util.generateRandomUUID(true) + '.pem';
      const file = fs.openSync(caPath, fs.OpenMode.CREATE | fs.OpenMode.WRITE_ONLY | fs.OpenMode.TRUNC);
      try { fs.writeSync(file.fd, this.caPem); fs.fsyncSync(file.fd); } finally { fs.closeSync(file); }
    }
    const result = await new Api(base, '', caPath).request<Bootstrap>('/bootstrap');
    if (result.deploymentId !== this.expectedId || result.protocolVersion !== '1') { throw new Error('Instance identity or protocol mismatch'); }
    const changed = this.deploymentId !== result.deploymentId || this.base !== base || this.caFingerprint !== (await this.prefs?.get('caFingerprint', '') as string);
    if (this.userId.length > 0) { await this.store.freeze(this.namespace()); }
    if (this.token.length > 0) { try { await this.api().request<Object>('/auth/logout', 'POST', '{}'); } catch (_) {} }
    await this.clearSession();
    if (changed) { this.profile = util.generateRandomUUID(true); }
    if (!/^[1-9][0-9]*$/.test(result.restoreEpoch)) { throw new Error('Invalid restore epoch'); }
    if (this.deploymentId === result.deploymentId && this.restoreEpoch !== result.restoreEpoch) { await this.store.freeze(this.namespace()); }
    this.restoreEpoch = result.restoreEpoch;
    this.base = base; this.deploymentId = result.deploymentId; this.caPath = caPath; this.applyFeatures(result);
    this.token = ''; this.userId = ''; this.conversations = []; this.rows = []; this.conversation = undefined; this.demo = false;
    await this.saveSetting('base', base); await this.saveSetting('deploymentId', this.deploymentId); await this.saveSetting('profile', this.profile); await this.saveSetting('restoreEpoch', this.restoreEpoch); await this.saveSetting('caPath', caPath); await this.saveSetting('caFingerprint', this.caFingerprint);
    this.info = result.name + ' · ' + result.deploymentId; this.page = 'chats';
  }
  private async verifyEpoch(): Promise<void> {
    if (this.demo || this.deploymentId.length === 0) { return; }
    const result = await this.api().request<Bootstrap>('/bootstrap');
    if (result.deploymentId !== this.deploymentId) { throw new Error('Deployment identity changed'); }
    if (!/^[1-9][0-9]*$/.test(result.restoreEpoch)) { throw new Error('Invalid restore epoch'); }
    this.applyFeatures(result);
    if (result.restoreEpoch !== this.restoreEpoch) {
      await this.store.freeze(this.namespace()); await this.clearSession();
      this.restoreEpoch = result.restoreEpoch; await this.saveSetting('restoreEpoch', this.restoreEpoch);
      throw new Error('RESTORE_EPOCH_CHANGED: sign in again; old outbox frozen');
    }
  }
  private async authenticate(explicitNew: boolean = false): Promise<void> {
    if (this.deploymentId.length === 0) { this.page = 'connect'; return; }
    await this.verifyEpoch(); await this.verifySavedCA();
    const auth = this.ensureAuth();
    const requestKey = 'authRequest:' + this.mode + ':' + this.username.toLowerCase();
    let requestId = explicitNew ? '' : await this.prefs?.get(requestKey, '') as string;
    if (!requestId) { requestId = util.generateRandomUUID(true); await this.saveSetting(requestKey, requestId); }
    if (this.mode === 'recover') {
      await this.api().request<Object>('/auth/recover', 'POST', Rules.authBody('recover', this.username, this.password, this.code, requestId));
      await auth.recovered(this.username);
      await this.saveSetting(requestKey, ''); this.password = ''; this.code = ''; this.mode = 'login'; return;
    }
    try { await auth.authenticate(this.mode, this.username, this.password, this.mode === 'login' ? this.code : '', requestId, explicitNew); }
    catch (error) {
      if (error instanceof ApiFailure && ['INVALID_CREDENTIALS', 'MFA_REQUIRED', 'MFA_INVALID', 'INVALID_USERNAME', 'INVALID_PASSWORD', 'USERNAME_TAKEN', 'USERNAME_RESERVED', 'RESERVED_USERNAME'].includes(error.code) && !(await auth.pending(this.username))) { await this.saveSetting(requestKey, ''); }
      throw error as Error;
    }
    await this.saveSetting(requestKey, ''); this.password = ''; this.code = ''; await this.reload();
  }
  private async preview(): Promise<void> {
    await this.clearSession(); this.demo = true; this.token = ''; this.userId = 'preview'; this.deviceId = 'preview'; this.conversation = undefined;
    this.conversations = [{ id: 'welcome', kind: 'direct', title: 'JobsIM', memberIds: [], cursor: '0' }, { id: 'studio', kind: 'group', title: 'Studio', memberIds: [], cursor: '0' }];
    this.people = [{ id: 'alex', username: 'alex', displayName: 'Alex' }];
    const rows = await this.store.list(this.namespace(), 'welcome', true);
    if (rows.length === 0) {
      await this.store.put(this.namespace(), { id: 'hello', conversationId: 'welcome', senderId: 'guide', clientMsgId: '', type: 'text', text: this.t('demoNote'), createdAt: new Date().toISOString(), version: '1', sequence: '0', deleted: false, pinned: false, favorite: false, reactions: [], state: 'demo' }, true);
    }
  }
  private async reload(): Promise<void> {
    if (this.demo || !this.token) { return; }
    const namespace = this.namespace();
    await this.verifyEpoch();
    await this.ensureSync().run();
    if (namespace !== this.namespace()) { return; }
    this.forwardViews.clear(); this.lookupRows = this.lookupRows.map((row: Message) => row.forwarded ? Rules.unavailableForward(row) : row);
    this.syncRestricted = false;
    await this.loadSelfMetadata();
    if (this.localEntries) {
      const selected = this.conversation;
      this.conversations = (await this.syncData.retainedConversations(this.syncIdentity())).map((raw: string) => JSON.parse(raw) as Conversation);
      if (selected?.state === 'local_only') { this.rows = await this.syncData.retained(this.syncIdentity(), selected.id); }
      return;
    }
    this.conversations = this.orderSelfConversations((await this.syncData.objects(this.syncIdentity(), 'conversation')).map((raw: string) => JSON.parse(raw) as Conversation));
    if (this.conversation) {
      const current = this.conversations.find((row: Conversation) => row.id === this.conversation?.id);
      if (current) { this.conversation = current; this.rows = await this.store.list(namespace, current.id, false); }
      else { this.conversation = undefined; this.rows = []; this.page = 'chats'; }
    }
    // Peer display enrichment is freshly authorized; it is not the authority for message installation.
    const contacts = await this.api().request<ContactsResult>('/contacts');
    if (namespace !== this.namespace()) { return; }
    this.people = contacts.items; this.requests = contacts.requests.filter((row: ContactRequest) => Rules.incoming(row, this.userId));
    const counts: Record<string, number> = {}; const mentions: Record<string, number> = {};
    for (const row of this.conversations) { counts[row.id] = await this.syncData.unread(this.syncIdentity(), row.id); mentions[row.id] = await this.store.mentionCount(namespace, row.id, false); }
    const newlyObserved = Object.keys(counts).some((id: string) => counts[id] > (this.unread[id] ?? 0) && !this.selfMuted(id));
    this.unread = counts; this.mentionUnread = mentions;
    if (newlyObserved && this.notificationsEnabled && namespace === this.namespace()) { await NativeServices.notify(getContext(this) as common.UIAbilityContext, NativeServices.notificationText(this.language)); }
    if (this.conversation) { await this.refreshForwardProjections(this.conversation); this.rows = await this.visibleRows(this.conversation.id); }
    if (this.page === 'lookup') { await this.refreshLookupForwards(); }
    this.me = await this.api().request<User>('/me');
    if ((this.me.avatarThumbnailURL ?? '') !== this.ownAvatarRef) { await this.loadOwnAvatar(); }
    this.avatarPending = (await this.store.value(this.operationNamespace(), 'avatar')).length > 0;
  }
  private async open(conversation: Conversation): Promise<void> {
    this.lookupFocusId = '';
    const changing = this.conversation?.id !== conversation.id;
    this.conversation = conversation;
    if (changing) { this.observedVisible.clear(); this.cloudReplyVersion = ''; this.draftRevision++; }
    if (!this.demo && conversation.state === 'local_only') { this.rows = await this.syncData.retained(this.syncIdentity(), conversation.id); this.historyMore = false; this.draft = ''; return; }
    const namespace = this.namespace();
    if (changing) {
      await this.draftTail;
      const stored = await this.store.value(namespace, 'draft:' + conversation.id, this.demo);
      const draft: DraftState = stored.length > 0 ? JSON.parse(stored) as DraftState : { text: '', replyId: '' };
      this.draft = draft.text; this.replyId = draft.replyId; this.editId = draft.editId ?? ''; this.editVersion = draft.editVersion ?? '';
      this.mentionedUserIds = draft.mentionedUserIds ?? []; this.mentionAll = draft.mentionAll ?? false;
      this.announcement = undefined; this.announcementCandidate = undefined; this.announcementPending = false; this.announcementRejected = false;
    }
    if (!this.demo) {
      for (const cached of await this.store.list(namespace, conversation.id, false)) { if (cached.forwarded) { await this.clearForwardSidecar(cached.id); } }
      await this.store.maskForwards(namespace, conversation.id);
    }
    this.rows = await this.store.list(namespace, conversation.id, this.demo);
    await this.store.setValue(namespace, 'unread:' + conversation.id, '0', this.demo);
    await this.store.readMentions(namespace, conversation.id, this.demo);
    if (!this.demo) { await this.syncData.readLocal(this.syncIdentity(), conversation.id); }
    const mentions = JSON.parse(JSON.stringify(this.mentionUnread)) as Record<string, number>; mentions[conversation.id] = 0; this.mentionUnread = mentions;
    const counts = JSON.parse(JSON.stringify(this.unread)) as Record<string, number>; counts[conversation.id] = 0; this.unread = counts;
    if (!this.demo) { await this.syncConversation(conversation); }
  }
  private async syncConversation(conversation: Conversation): Promise<void> {
    const namespace = this.namespace();
    if (conversation.state === 'local_only') { this.rows = await this.syncData.retained(this.syncIdentity(), conversation.id); this.historyMore = false; return; }
    await this.ensureSync().run();
    if (namespace !== this.namespace()) { return; }
    this.forwardViews.clear();
    this.syncRestricted = false;
    if (this.conversation?.id === conversation.id) { await this.syncData.readLocal(this.syncIdentity(), conversation.id); }
    this.historyMore = this.synchronizer?.hasMore ?? false;
    await this.refreshForwardProjections(conversation);
    if (this.conversation?.id === conversation.id) { this.rows = await this.visibleRows(conversation.id); }
  }
  private async visibleRows(conversation: string): Promise<Message[]> {
    const rows = await this.store.list(this.namespace(), conversation, this.demo);
    return rows.map((row: Message) => { const projection = this.forwardViews.get(row.id); return projection && projection.version === row.version && projection.contentVersion === row.contentVersion && !row.deleted ? projection : row; });
  }
  private async loadRemainingHistory(): Promise<void> {
    const conversation = this.conversation;
    const namespace = this.namespace();
    if (!conversation || this.demo) { return; }
    do {
      await this.syncConversation(conversation);
    } while (this.historyMore && this.conversation?.id === conversation.id && namespace === this.namespace());
  }
  private async submit(row: Message): Promise<void> {
    if (this.demo || this.conversation?.state === 'local_only' || row.state !== 'pending' || row.restoreEpoch !== this.restoreEpoch) { return; }
    const namespace = this.namespace();
    const original = (await this.store.list(namespace, row.conversationId, false)).find((value: Message) => value.id === row.id && value.clientMsgId === row.clientMsgId);
    if (!original || original.state !== 'pending' || namespace !== this.namespace()) { return; }
    row = original;
    const bootstrap = await this.api().request<Bootstrap>('/bootstrap');
    if (namespace !== this.namespace() || bootstrap.deploymentId !== this.deploymentId || bootstrap.restoreEpoch !== this.restoreEpoch) { return; }
    await this.syncData.ready(this.syncIdentity());
    let result: Message | undefined = undefined;
    try {
      if (row.sendAttempted) {
        try { if (!await this.ensureSync().resolve(row.clientMsgId)) { throw new Error('SYNC_ORIGINAL_BODY_CHANGED'); } await this.ensureSync().run(); if (this.conversation) { this.rows = await this.visibleRows(this.conversation.id); } return; }
        catch (error) { if (!String(error).includes('HTTP 404')) { throw error as Error; } }
      }
      if (!result) {
        if (!row.sendAdmission) { row.state = 'frozen'; row.freezeReason = 'SEND_ADMISSION_CHANGED'; await this.store.put(namespace, row, false); throw new Error(this.t('admissionChanged')); }
        if (!Rules.admissionSupported(bootstrap, this.deploymentId)) { throw new Error(this.t('admissionUnsupported')); }
        if (row.type === 'forward' && bootstrap.features?.forwarding !== true) { throw new Error(this.t('featureUnavailable')); }
        if (row.mentionAll === true && !Rules.roleAdmissionSupported(bootstrap, this.deploymentId)) { throw new Error(this.t('featureUnavailable')); }
        if ((row.mentionAll === true || (row.mentionedUserIds?.length ?? 0) > 0) && bootstrap.features?.mentions !== true) { throw new Error(this.t('featureUnavailable')); }
        row.sendAttempted = true; await this.store.put(namespace, row, false);
        if (namespace !== this.namespace()) { return; }
        result = await this.api().request<Message>('/conversations/' + row.conversationId + '/messages', http.RequestMethod.POST, Rules.outgoingBody(row));
      }
    } catch (error) {
      if (Rules.permanentMentionRejection(String(error)) || Rules.permanentForwardRejection(String(error))) {
        row.state = 'frozen'; row.freezeReason = String(error);
        await this.store.put(namespace, row, false);
        if (namespace === this.namespace() && this.conversation?.id === row.conversationId) { this.rows = await this.store.list(namespace, row.conversationId, false); }
        throw new Error(this.t('admissionChanged'));
      }
      throw new Error(String(error));
    }
    if (namespace !== this.namespace()) { return; }
    if (result.senderId !== row.senderId || result.clientMsgId !== row.clientMsgId || result.conversationId !== row.conversationId) { throw new Error('Original result scope mismatch'); }
    if (!await this.ensureSync().resolve(row.clientMsgId)) { throw new Error('SYNC_ORIGINAL_BODY_CHANGED'); }
    await this.ensureSync().run();
    if (this.conversation) { this.rows = await this.store.list(namespace, this.conversation.id, false); }
  }
  private async selectForward(row: Message): Promise<void> {
    if (this.demo) { this.forwardSource = row; this.page = 'forward'; return; }
    const namespace = this.namespace();
    const identity = this.syncIdentity(); const guard = await this.syncData.ready(identity); const life = this.syncLife;
    await this.verifyEpoch();
    if (!this.forwardingSupported) { throw new Error(this.t('featureUnavailable')); }
    const source = await this.api().request<Message>('/messages/' + encodeURIComponent(row.id));
    if (namespace !== this.namespace() || life !== this.syncLife || !await this.syncData.canShow(identity, guard, row.id)) { return; }
    SyncRules.message(JSON.stringify(source));
    if (source.id !== row.id || source.conversationId !== row.conversationId) { throw new Error('SYNC_ROUTE_MISMATCH'); }
    if (Rules.sourceContentVersion(source) !== Rules.sourceContentVersion(row)) {
      await this.store.put(namespace, source, false);
      this.rows = this.rows.map((value: Message) => value.id === source.id ? source : value);
      throw new Error(this.t('forwardSourceUpdated'));
    }
    if (!source.forwardAdmission || !source.contentVersion) { throw new Error(this.t('featureUnavailable')); }
    this.forwardSource = source; this.page = 'forward';
  }
  private async forwardTo(target: Conversation): Promise<void> {
    const source = this.forwardSource;
    if (!source) { return; }
    const namespace = this.namespace();
    const current = this.demo ? target : await this.api().request<Conversation>('/conversations/' + encodeURIComponent(target.id));
    if (namespace !== this.namespace()) { return; }
    const existing = await this.store.list(namespace, current.id, this.demo);
    if (existing.some((row: Message) => row.type === 'forward' && row.forwardOfId === source.id && row.state === 'pending')) { throw new Error(this.t('pending')); }
    const id = util.generateRandomUUID(true);
    const row: Message = this.demo ? JSON.parse(JSON.stringify(source)) as Message : Rules.forwardIntent(source, current, this.userId, this.restoreEpoch, id, new Date().toISOString());
    if (this.demo) { row.id = id; row.clientMsgId = id; row.conversationId = current.id; row.senderId = this.userId; row.forwarded = true; row.pinned = false; row.favorite = false; row.reactions = []; row.state = 'demo'; }
    await this.store.put(namespace, row, this.demo);
    this.page = 'chats'; this.forwardSource = undefined;
    await this.open(current);
    if (!this.demo) { await this.submit(row); }
  }
  private async refreshForwardProjections(conversation: Conversation): Promise<void> {
    const namespace = this.namespace();
    const identity = this.syncIdentity(); const guard = await this.syncData.ready(identity); const life = this.syncLife;
    const rows = (await this.store.list(namespace, conversation.id, false)).filter((row: Message) => row.forwarded && row.state !== 'pending' && row.state !== 'frozen' && !row.deleted);
    const key = 'forward-review:' + conversation.id;
    const start = rows.length > 0 ? Number(await this.store.value(namespace, key) || '0') % rows.length : 0;
    const batch = rows.slice(start).concat(rows.slice(0, start)).slice(0, 20);
    for (const cached of batch) {
      let projection: Message;
      try {
        projection = await this.api().request<Message>('/messages/' + encodeURIComponent(cached.id));
        if (projection.id !== cached.id || projection.conversationId !== conversation.id) { throw new Error('Forward scope mismatch'); }
      } catch (error) {
        projection = Rules.unavailableForward(cached);
        if (String(error).includes('HTTP 401') || String(error).includes('RESTORE_EPOCH_CHANGED')) { throw error as Error; }
      }
      if (namespace !== this.namespace() || life !== this.syncLife || !await this.syncData.canShow(identity, guard, cached.id)) { this.forwardViews.delete(cached.id); continue; }
      SyncRules.message(JSON.stringify(projection));
      if (projection.forwardUnavailable || projection.deleted || projection.contentUnavailable) { await this.clearForwardSidecar(cached.id); }
      await this.store.put(namespace, projection, false);
      this.forwardViews.set(projection.id, projection);
      this.lookupRows = this.lookupRows.map((row: Message) => row.id === projection.id ? projection : row);
    }
    await this.store.setValue(namespace, key, String(start + batch.length));
  }
  private async refreshLookupForwards(): Promise<void> {
    const namespace = this.namespace();
    const identity = this.syncIdentity(); const guard = await this.syncData.ready(identity); const life = this.syncLife;
    const forwards = this.lookupRows.filter((row: Message) => row.forwarded && !row.deleted);
    const start = forwards.length > 0 ? Number(await this.store.value(namespace, 'lookup-forward-review') || '0') % forwards.length : 0;
    for (const cached of forwards.slice(start).concat(forwards.slice(0, start)).slice(0, 20)) {
      let projection: Message;
      try { projection = await this.api().request<Message>('/messages/' + encodeURIComponent(cached.id)); }
      catch (error) { projection = Rules.unavailableForward(cached); }
      if (namespace !== this.namespace() || this.page !== 'lookup' || life !== this.syncLife || !await this.syncData.canShow(identity, guard, cached.id)) { return; }
      if (projection.id !== cached.id || projection.conversationId !== cached.conversationId) { projection = Rules.unavailableForward(cached); }
      SyncRules.message(JSON.stringify(projection));
      this.lookupRows = this.lookupRows.map((row: Message) => row.id === cached.id ? projection : row);
    }
    await this.store.setValue(namespace, 'lookup-forward-review', String(start + 20));
  }
  private async clearForwardSidecar(id: string): Promise<void> {
    const key = 'forward-sidecar:' + id;
    const path = await this.store.value(this.namespace(), key);
    const prefix = (getContext(this) as common.UIAbilityContext).cacheDir + '/attachment-';
    if (path.startsWith(prefix) && !path.includes('..')) { try { fs.unlinkSync(path); } catch (_) {} }
    await this.store.setValue(this.namespace(), key, '');
  }
  private async send(type: string = 'text', attachmentId: string = '', name: string = '', original?: Conversation): Promise<void> {
    const conversation = original ?? this.conversation;
    const namespace = this.namespace();
    if (!conversation || !Rules.writable(conversation) || (this.draft.length === 0 && attachmentId.length === 0)) { return; }
    if (this.editId.length > 0) {
      await this.api().request<Message>('/messages/' + this.editId, 'PATCH', JSON.stringify({ text: this.draft, expectedVersion: this.editVersion }));
      this.editId = ''; this.editVersion = ''; this.draft = ''; this.mentionedUserIds = []; this.mentionAll = false; this.persistDraft(); await this.syncConversation(conversation); return;
    }
    await this.verifyEpoch();
    if (namespace !== this.namespace() || this.conversation?.id !== conversation.id) { return; }
    if (!this.demo && !conversation.sendAdmission) { throw new Error(this.t('admissionUnsupported')); }
    if (!this.demo && (this.mentionAll || this.mentionedUserIds.length > 0) && !this.mentionsSupported) { throw new Error(this.t('featureUnavailable')); }
    const id = util.generateRandomUUID(true);
    const row: Message = { id: id, conversationId: conversation.id, senderId: this.userId, clientMsgId: id, type: type, text: name.length > 0 ? name : this.draft, replyToId: this.replyId, attachmentId: attachmentId, createdAt: new Date().toISOString(), version: '1', sequence: '0', deleted: false, pinned: false, favorite: false, reactions: [], state: this.demo ? 'demo' : 'pending', restoreEpoch: this.restoreEpoch, sendAdmission: conversation.sendAdmission, mentionedUserIds: this.mentionedUserIds.length > 0 ? this.mentionedUserIds.slice() : undefined, mentionAll: this.mentionAll ? true : undefined };
    await this.store.put(this.namespace(), row, this.demo); this.rows = this.rows.concat([row]); this.draft = ''; this.replyId = ''; this.mentionedUserIds = []; this.mentionAll = false; this.persistDraft();
    if (!this.demo) { await this.submit(row); }
  }
  private async messageAction(row: Message, action: string): Promise<void> {
    if (this.conversation?.state === 'local_only') { return; }
    if (this.conversation && !Rules.writable(this.conversation) && action !== 'favorite') { return; }
    if (action === 'forward') { await this.selectForward(row); return; }
    if (action === 'reply') { this.replyId = row.id; this.cloudReplyVersion = row.contentVersion ?? ''; this.persistDraft(); return; }
    if (action === 'edit' && row.forwarded) { return; }
    if (action === 'edit') { this.editId = row.id; this.editVersion = row.version; this.draft = row.text; this.mentionedUserIds = []; this.mentionAll = false; this.persistDraft(); return; }
    if (action === 'retry') { await this.submit(row); return; }
    let copy = JSON.parse(JSON.stringify(row)) as Message;
    let method = http.RequestMethod.POST;
    let tail = '';
    if (action === 'pin') { copy.pinned = !copy.pinned; method = copy.pinned ? http.RequestMethod.PUT : http.RequestMethod.DELETE; tail = '/pin'; }
    if (action === 'favorite') { copy.favorite = !copy.favorite; method = copy.favorite ? http.RequestMethod.PUT : http.RequestMethod.DELETE; tail = '/favorite'; }
    if (action === 'retract') { copy.deleted = true; copy.text = ''; method = http.RequestMethod.DELETE; }
    if (action === 'react') {
      const removing = row.reactions.some((reaction: Reaction) => reaction.emoji === '👍' && reaction.userId === this.userId);
      copy.reactions = removing ? row.reactions.filter((reaction: Reaction) => !(reaction.emoji === '👍' && reaction.userId === this.userId)) : row.reactions.concat([{ emoji: '👍', userId: this.userId }]);
      tail = removing ? '/reactions/' + encodeURIComponent('👍') : '/reactions'; method = removing ? http.RequestMethod.DELETE : http.RequestMethod.POST;
    }
    const namespace = this.namespace();
    if (!this.demo) {
      const identity = this.syncIdentity(); const guard = await this.syncData.ready(identity); const life = this.syncLife;
      copy = await this.api().request<Message>('/messages/' + row.id + tail, method, JSON.stringify({ emoji: '👍' }));
      SyncRules.message(JSON.stringify(copy));
      if (copy.id !== row.id || copy.conversationId !== row.conversationId) { throw new Error('SYNC_ROUTE_MISMATCH'); }
      if (namespace !== this.namespace() || life !== this.syncLife || !await this.syncData.canShow(identity, guard, row.id)) { return; }
    }
    await this.store.put(this.namespace(), copy, this.demo);
    if (!this.demo) { await this.drainSyncCleanup(); }
    if (this.conversation) { this.rows = await this.store.list(this.namespace(), this.conversation.id, this.demo); }
  }
  private async attach(): Promise<void> {
    const conversation = this.conversation;
    const namespace = this.namespace();
    if (!conversation) { return; }
    if (this.conversation && !Rules.writable(this.conversation)) { return; }
    const context = getContext(this) as common.UIAbilityContext;
    const options = new picker.DocumentSelectOptions();
    options.maxSelectNumber = 1;
    const uris = await new picker.DocumentViewPicker(context).select(options);
    if (uris.length === 0) { return; }
    if (namespace !== this.namespace() || this.conversation?.id !== conversation.id) { return; }
    if (this.demo) { await this.send('file', 'demo-' + util.generateRandomUUID(true), 'attachment', conversation); return; }
    const result = await this.api().upload(uris[0]);
    if (namespace !== this.namespace() || this.conversation?.id !== conversation.id) { return; }
    const type = result.contentType.startsWith('image/') ? 'image' : result.contentType.startsWith('audio/') ? 'audio' : result.contentType.startsWith('video/') ? 'video' : 'file';
    await this.send(type, result.id, result.name, conversation);
  }
  private async searchPeople(): Promise<void> {
    if (!this.demo) { this.results = (await this.api().request<UserList>('/users?q=' + encodeURIComponent(this.query))).items; }
  }
  private async showLookup(kind: string, scoped: boolean = false): Promise<void> {
    this.lookupScope = scoped ? (this.conversation?.id ?? '') : '';
    this.lookupKind = kind; this.lookupRows = []; this.lookupCursor = ''; this.lookupMore = false;
    this.lookupQuery = ''; this.lookupSubmitted = '';
    this.conversation = undefined; this.page = 'lookup';
    if (kind === 'favorites') { await this.loadLookup(); }
  }
  private async loadLookup(append: boolean = false): Promise<void> {
    const namespace = this.namespace();
    const identity = this.demo ? undefined : this.syncIdentity(); const guard = identity ? await this.syncData.ready(identity) : ''; const life = this.syncLife;
    const kind = this.lookupKind;
    const query = append ? this.lookupSubmitted : this.lookupQuery.trim();
    if (kind === 'search' && query.length === 0) { return; }
    const key = 'lookup:' + kind + ':' + this.lookupScope + ':' + query;
    let page: LookupPage;
    try {
      if (this.demo) {
        const items: Message[] = [];
        for (const conversation of this.conversations) {
          const rows = await this.store.list(namespace, conversation.id, true);
          for (const row of rows) { if (!row.deleted && (kind === 'favorites' ? row.favorite : row.text.includes(query))) { items.push(row); } }
        }
        page = { items: items, cursor: '', hasMore: false };
      } else {
        page = await this.api().request<LookupPage>(Rules.lookupPath(kind, query, this.lookupScope, append ? this.lookupCursor : ''));
        if (!Array.isArray(page.items) || typeof page.cursor !== 'string' || typeof page.hasMore !== 'boolean') { throw new Error('Invalid lookup page'); }
      }
      if (namespace !== this.namespace() || kind !== this.lookupKind) { return; }
      if (identity) {
        const filtered: Message[] = [];
        for (const message of page.items) { SyncRules.message(JSON.stringify(message)); if (this.lookupScope && message.conversationId !== this.lookupScope) { throw new Error('SYNC_OBJECT_ID_MISMATCH'); } if (life !== this.syncLife) { return; } if (await this.syncData.canShow(identity, guard, message.id)) { filtered.push(message); } }
        page.items = filtered;
      }
      const combined = append ? this.lookupRows.concat(page.items) : page.items;
      const unique = new Map<string, Message>();
      combined.forEach((message: Message) => unique.set(message.id, message));
      this.lookupRows = Array.from(unique.values()); this.lookupCursor = page.cursor; this.lookupMore = page.hasMore; this.lookupSubmitted = query;
      if (this.demo) { await this.store.setValue(namespace, key, JSON.stringify({ items: this.lookupRows, cursor: this.lookupCursor, hasMore: this.lookupMore }), true); }
    } catch (error) {
      if (namespace !== this.namespace()) { return; }
      const reason = (error as Error).message;
      if (reason.includes('HTTP 401') || reason.includes('HTTP 403') || reason.includes('HTTP 404') || reason.includes('HTTP 409')) {
        this.lookupRows = []; this.lookupCursor = ''; this.lookupMore = false;
      } else if (!append && this.demo) {
        const cached = await this.store.value(namespace, key, this.demo);
        if (cached.length > 0) { const previous = JSON.parse(cached) as LookupPage; this.lookupRows = previous.items; this.lookupCursor = ''; this.lookupMore = false; this.lookupSubmitted = query; }
      }
      throw error as Error;
    }
  }
  private async openLookupSource(candidate: Message): Promise<void> {
    const namespace = this.namespace();
    const identity = this.demo ? undefined : this.syncIdentity(); const guard = identity ? await this.syncData.ready(identity) : ''; const life = this.syncLife;
    try {
      const conversation = this.demo ? this.conversations.find((item: Conversation) => item.id === candidate.conversationId) : await this.api().request<Conversation>('/conversations/' + encodeURIComponent(candidate.conversationId));
      const message = this.demo ? candidate : await this.api().request<Message>('/messages/' + encodeURIComponent(candidate.id));
      if (namespace !== this.namespace() || !conversation) { return; }
      if (conversation.id !== candidate.conversationId || message.id !== candidate.id || message.conversationId !== candidate.conversationId) { throw new Error('SYNC_OBJECT_ID_MISMATCH'); }
      if (identity) { SyncRules.message(JSON.stringify(message)); if (life !== this.syncLife || !await this.syncData.canShow(identity, guard, message.id)) { return; } }
      await this.store.put(namespace, message, this.demo);
      if (!this.conversations.some((item: Conversation) => item.id === conversation.id)) { this.conversations = this.conversations.concat([conversation]); }
      this.page = 'chats'; this.query = '';
      await this.open(conversation);
      if (namespace === this.namespace()) { this.lookupFocusId = message.id; }
    } catch (error) {
      if (namespace === this.namespace()) { this.lookupRows = this.lookupRows.filter((message: Message) => message.id !== candidate.id); }
      throw error as Error;
    }
  }
  private async create(retry: boolean = false): Promise<void> {
    let draft: ConversationDraft = { kind: this.newGroup ? 'group' : 'direct', title: this.newTitle, memberIds: this.memberIds.split(',').map((id: string) => id.trim()).filter((id: string) => id.length > 0) };
    if (this.demo) {
      const row: Conversation = { id: util.generateRandomUUID(true), kind: draft.kind, title: draft.title, memberIds: draft.memberIds, cursor: '0' };
      this.conversations = [row].concat(this.conversations); this.page = 'chats'; await this.open(row); return;
    }
    const previous = await this.store.value(this.operationNamespace(), 'operation:create-conversation');
    if (retry && previous.length > 0) { draft = JSON.parse((JSON.parse(previous) as PendingOperation).payload) as ConversationDraft; }
    const operation = await this.beginOperation('create-conversation', JSON.stringify(draft));
    const row = await this.api().request<Conversation>('/conversations', 'POST', JSON.stringify({ kind: draft.kind, title: draft.title, memberIds: draft.memberIds, requestId: operation.requestId }));
    await this.finishOperation('create-conversation');
    this.conversations = [row].concat(this.conversations); this.page = 'chats'; await this.open(row);
  }
  private async beginOperation(key: string, payload: string): Promise<PendingOperation> {
    const old = await this.store.value(this.operationNamespace(), 'operation:' + key);
    if (old.length > 0) {
      const pending = JSON.parse(old) as PendingOperation;
      if (pending.payload !== payload) { throw new Error(this.t('resolvePending')); }
      return pending;
    }
    const pending: PendingOperation = { requestId: util.generateRandomUUID(true), payload: payload };
    await this.store.setValue(this.operationNamespace(), 'operation:' + key, JSON.stringify(pending));
    return pending;
  }
  private async finishOperation(key: string): Promise<void> {
    await this.store.setValue(this.operationNamespace(), 'operation:' + key, '');
  }
  private confirmChange(label: string, action: () => Promise<void>, detail: string = ''): void {
    const namespace = this.namespace();
    AlertDialog.show({
      title: this.t(label), message: this.t('confirmChange') + (detail.length > 0 ? '\n' + detail : ''),
      primaryButton: { value: this.t('cancel'), action: () => {} },
      secondaryButton: { value: this.t('continue'), action: () => { if (namespace === this.namespace()) { this.run(action); } } }
    });
  }
  private async directConversation(person: User): Promise<void> {
    if (this.demo) { return; }
    let draft: ConversationDraft = { kind: 'direct', title: person.displayName, memberIds: [person.id] };
    const key = 'direct:' + person.id;
    const previous = await this.store.value(this.operationNamespace(), 'operation:' + key);
    if (previous.length > 0) { draft = JSON.parse((JSON.parse(previous) as PendingOperation).payload) as ConversationDraft; }
    const operation = await this.beginOperation(key, JSON.stringify(draft));
    const row = await this.api().request<Conversation>('/conversations', 'POST', JSON.stringify({ kind: draft.kind, title: draft.title, memberIds: draft.memberIds, requestId: operation.requestId }));
    await this.finishOperation(key); this.page = 'chats'; await this.open(row);
  }
  private async contactAction(peer: string, action: string): Promise<void> {
    if (this.demo) { this.info = this.t('demoNote'); return; }
    if (peer.length === 0 || peer === this.userId) { throw new Error(this.t('members')); }
    const key = action + ':' + peer;
    const operation = await this.beginOperation(key, JSON.stringify({ userId: peer }));
    if (action === 'add') {
      await this.api().request<Object>('/contacts/requests', 'POST', JSON.stringify({ userId: peer, requestId: operation.requestId }));
    } else if (action === 'reopen') {
      const result = await this.api().request<ReopenResult>('/contacts/' + encodeURIComponent(peer) + '/reopen', 'POST', JSON.stringify({ requestId: operation.requestId }));
      this.info = this.t(result.current ? 'gateOpened' : 'originalOnly');
    } else {
      const path = action === 'deleteContact' ? '/contacts/' : '/blocks/';
      await this.api().request<Object>(path + encodeURIComponent(peer), action === 'block' ? 'PUT' : 'DELETE');
    }
    await this.finishOperation(key);
    if (action === 'block') { this.blockedIds = this.blockedIds.filter((id: string) => id !== peer).concat([peer]); }
    if (action === 'unblock') { this.blockedIds = this.blockedIds.filter((id: string) => id !== peer); }
    await this.store.setValue(this.namespace(), 'blockedIds', JSON.stringify(this.blockedIds));
    this.results = [];
    await this.reload();
  }
  private async requestAction(request: ContactRequest, action: string): Promise<void> {
    if (!Rules.incoming(request, this.userId)) { return; }
    await this.api().request<Object>('/contacts/requests/' + encodeURIComponent(request.id) + '/' + action, 'POST', '{}');
    await this.reload();
  }
  private async groupSettings(): Promise<void> {
    if (!this.conversation || this.conversation.kind !== 'group') { return; }
    await this.loadGroupMembers(false);
    this.groupTitle = this.conversation.title;
    this.page = 'group';
  }
  private async groupAction(action: string, member?: GroupMember): Promise<void> {
    const conversation = this.conversation;
    if (!conversation || !Rules.writable(conversation) || this.demo) { return; }
    if (action === 'remove' || action === 'leave') {
      if (!member) { return; }
      const key = 'remove:' + conversation.id + ':' + member.id;
      const stored = await this.store.value(this.operationNamespace(), 'operation:' + key);
      const operation = stored.length > 0 ? JSON.parse(stored) as PendingOperation : await this.beginOperation(key, JSON.stringify(member));
      const original = JSON.parse(operation.payload) as GroupMember;
      await this.api().request<Object>(Rules.removalPath(conversation.id, original), 'DELETE');
      await this.finishOperation(key);
      if (member.id === this.userId) { this.conversation = undefined; this.rows = []; this.page = 'chats'; await this.reload(); return; }
    } else if (action === 'addMember') {
      await this.api().request<Object>('/conversations/' + conversation.id + '/members', 'POST', JSON.stringify({ userId: this.groupNewMember }));
      this.groupNewMember = '';
    } else {
      if (action !== 'rename') { throw new Error('GROUP_ORIGINAL_PERMISSION_REQUIRED'); }
      await this.api().request<Conversation>('/conversations/' + conversation.id, 'PATCH', JSON.stringify({ title: this.groupTitle }));
      this.groupNewOwner = '';
    }
    this.conversations = (await this.api().request<ConversationList>('/conversations')).items;
    this.conversation = this.conversations.find((row: Conversation) => row.id === conversation.id);
    await this.groupSettings();
  }
  private async verifySavedCA(): Promise<void> {
    if (this.caPath.length === 0) { return; }
    const pem = fs.readTextSync(this.caPath);
    const der = new util.Base64Helper().decodeSync(pem.replace(/-----BEGIN CERTIFICATE-----|-----END CERTIFICATE-----|\s/g, ''));
    const digest = cryptoFramework.createMd('SHA256'); await digest.update({ data: der });
    const actual = Array.from((await digest.digest()).data).map((value: number) => value.toString(16).padStart(2, '0')).join('');
    if (actual !== this.caFingerprint.replace(/:/g, '').toLowerCase()) { throw new Error('Trusted CA changed; reconnect and verify'); }
  }
  private async restoreSession(): Promise<void> {
    if (this.deploymentId.length === 0) { return; }
    await this.verifySavedCA();
    await this.loadPasswordRows();
    await this.loadDeletionRows();
    for (const row of this.deletionRows.filter((item: DeletionIntent) => item.state === 'completed')) { await this.eraseDeletionLocal(row); }
    const raw = await this.prefs?.get('session', '') as string;
    const legacy = raw ? JSON.parse(raw) as SessionPointer : undefined;
    await this.ensureAuth().restore(legacy);
    if (this.token.length > 0) { await this.reload(); }
  }
  private resetSessionView(): void {
    this.receiptAutomatic?.stop();this.receiptAutomatic=undefined;this.receiptAutoIdentity=undefined;this.receiptAutoEnabled=false;
    this.clearPasswordInput(); this.clearDeletionInput();
    this.closeExport();
    this.stopSync(); this.forwardViews.clear(); this.clearSyncRAM(); this.syncRestricted = false; this.localEntries = false;
    this.selfService = undefined; this.selfKey = ''; this.selfSupported = false; this.selfAll = false; this.selfViews = {}; this.selfView = undefined; this.selfOperations = []; this.selfDraftEntries = []; this.cloudFontKey = ''; this.cloudFont = 'standard'; this.cloudReplyVersion = ''; this.draftRevision++;
    this.sessionAlias = ''; this.sourceContextId = ''; this.token = ''; this.userId = ''; this.deviceId = ''; this.me = undefined;
    this.conversation = undefined; this.rows = []; this.people = []; this.requests = []; this.results = []; this.conversations = []; this.avatarPath = '';
    this.mentionedUserIds = []; this.mentionAll = false; this.mentionsSupported = false; this.announcementsSupported = false; this.roleAdmissionSupported = false; this.mentionUnread = {}; this.announcement = undefined; this.announcementCandidate = undefined; this.announcementPending = false; this.announcementRejected = false;
    this.members = []; this.deviceSessions = []; this.reports = []; this.blockedIds = []; this.unread = {}; this.codes = [];
    this.draft = ''; this.replyId = ''; this.editId = ''; this.query = ''; this.reportTarget = ''; this.reportReason = ''; this.managedPeer = '';
    this.avatarPending = false; this.historyMore = false; this.ownAvatarRef = '';
  }
  private async clearSession(): Promise<void> {
    const alias = this.sessionAlias;
    if (this.auth) { await this.auth.clear(); }
    else if (alias) { await this.secure.remove(alias); }
    this.resetSessionView(); await this.saveSetting('session', '');
  }
  private async logout(): Promise<void> {
    if (!this.demo && this.token.length > 0) { await this.ensureAuth().logout(); }
    else { await this.clearSession(); }
    this.demo = false; this.page = 'chats';
  }
  private async generateCodes(): Promise<void> {
    const operation = await this.beginOperation('recovery-codes', '{}');
    const result = await this.api().request<Codes>('/auth/recovery-codes', 'POST', JSON.stringify({ password: this.password, requestId: operation.requestId }));
    this.codes = result.codes ?? []; this.password = '';
    await this.finishOperation('recovery-codes');
    this.info = this.codes.length === 0 ? this.t('codesUnavailable') : this.t('codes');
  }
  private async revokeSession(device: DeviceSession): Promise<void> {
    await this.api().request<Object>('/auth/sessions/' + encodeURIComponent(device.deviceId), 'DELETE');
    if (device.deviceId === this.deviceId) { await this.store.freeze(this.namespace()); await this.clearSession(); this.page = 'chats'; }
    else { this.deviceSessions = (await this.api().request<SessionList>('/auth/sessions')).items; }
  }
  private resourceExtension(mime: string): string {
    const type = mime.split(';')[0].trim();
    if (type === 'application/pdf') { return '.pdf'; }
    if (type.startsWith('text/')) { return '.txt'; }
    if (type === 'image/jpeg') { return '.jpg'; }
    if (type === 'image/png') { return '.png'; }
    if (type === 'image/gif') { return '.gif'; }
    if (type === 'audio/mp4') { return '.m4a'; }
    if (type === 'audio/mpeg') { return '.mp3'; }
    if (type === 'video/mp4') { return '.mp4'; }
    return '.bin';
  }
  private async openAttachment(row: Message): Promise<void> {
    const resource = Rules.forwardPath(row);
    if (resource.length === 0 || this.demo || this.conversation?.state === 'local_only') { return; }
    await this.verifyEpoch();
    const namespace = this.namespace();
    const identity = this.syncIdentity(); const guard = await this.syncData.ready(identity); const life = this.syncLife;
    if (row.forwarded) { await this.clearForwardSidecar(row.id); }
    const result = await this.api().download(resource);
    if (namespace !== this.namespace() || life !== this.syncLife || !await this.syncData.canShow(identity, guard, row.id)) { return; }
    const context = getContext(this) as common.UIAbilityContext;
    const path = context.cacheDir + '/attachment-' + util.generateRandomUUID(true) + this.resourceExtension(result.contentType);
    const file = fs.openSync(path, fs.OpenMode.CREATE | fs.OpenMode.WRITE_ONLY | fs.OpenMode.TRUNC);
    try { fs.writeSync(file.fd, result.data); fs.fsyncSync(file.fd); } finally { fs.closeSync(file); }
    try { await this.syncData.rememberSidecar(identity, guard, row.id, path); if (life !== this.syncLife || !await this.syncData.canShow(identity, guard, row.id)) { throw new Error('SYNC_LOCAL_GUARD_CHANGED'); } await NativeServices.openFile(context, path, result.contentType, row.text); }
    catch (error) { fs.unlinkSync(path); throw error as Error; }
  }
  private async fileDigest(path: string): Promise<string> {
    const file = fs.openSync(path, fs.OpenMode.READ_ONLY);
    const bytes = new ArrayBuffer(fs.statSync(file.fd).size);
    try { if (fs.readSync(file.fd, bytes) !== bytes.byteLength) { throw new Error('Incomplete file'); } } finally { fs.closeSync(file); }
    const md = cryptoFramework.createMd('SHA256'); await md.update({ data: new Uint8Array(bytes) });
    return Array.from((await md.digest()).data).map((value: number) => value.toString(16).padStart(2, '0')).join('');
  }
  private async chooseAvatar(): Promise<void> {
    if ((await this.store.value(this.operationNamespace(), 'avatar')).length > 0) { await this.reconcileAvatar(); return; }
    this.me = await this.api().request<User>('/me');
    const context = getContext(this) as common.UIAbilityContext;
    const options = new picker.DocumentSelectOptions(); options.maxSelectNumber = 1;
    const selected = await new picker.DocumentViewPicker(context).select(options);
    if (selected.length === 0) { return; }
    const source = fs.openSync(selected[0], fs.OpenMode.READ_ONLY);
    const path = context.filesDir + '/avatar-original-' + util.generateRandomUUID(true);
    try {
      const size = fs.statSync(source.fd).size;
      if (size > 2 * 1024 * 1024 || size === 0) { throw new Error(this.t('avatarLimit')); }
      const bytes = new ArrayBuffer(size);
      if (fs.readSync(source.fd, bytes) !== size) { throw new Error('Incomplete original avatar'); }
      const target = fs.openSync(path, fs.OpenMode.CREATE | fs.OpenMode.WRITE_ONLY | fs.OpenMode.TRUNC);
      try { fs.writeSync(target.fd, bytes); fs.fsyncSync(target.fd); } finally { fs.closeSync(target); }
    } finally { fs.closeSync(source); }
    const pending: AvatarPending = { requestId: util.generateRandomUUID(true), path: path, expectedVersion: this.me.profileVersion ?? '1', sha256: await this.fileDigest(path) };
    await this.store.setValue(this.operationNamespace(), 'avatar', JSON.stringify(pending)); this.avatarPending = true;
    await this.reconcileAvatar();
  }
  private async reconcileAvatar(): Promise<void> {
    const raw = await this.store.value(this.operationNamespace(), 'avatar');
    if (raw.length === 0) { return; }
    const pending = JSON.parse(raw) as AvatarPending;
    if (await this.fileDigest(pending.path) !== pending.sha256) { throw new Error('Original avatar changed; cannot resend'); }
    let operation: AvatarOperation;
    try { operation = await this.api().request<AvatarOperation>('/me/avatar/operations/' + pending.requestId); }
    catch (error) {
      if (!(error as Error).message.includes('HTTP 404')) { throw error as Error; }
      operation = await this.api().avatar(pending.path, pending.requestId, pending.expectedVersion);
    }
    if (operation.status === 'prepared') {
      operation = await this.api().avatar(pending.path, pending.requestId, pending.expectedVersion);
    }
    if (operation.status === 'prepared') { this.info = this.t('pending'); return; }
    this.me = await this.api().request<User>('/me');
    this.info = this.t(operation.current ? 'avatarUpdated' : 'originalOnly');
    await this.store.setValue(this.operationNamespace(), 'avatar', ''); this.avatarPending = false;
    if (pending.path.startsWith((getContext(this) as common.UIAbilityContext).filesDir + '/avatar-original-')) { fs.unlinkSync(pending.path); }
    await this.loadOwnAvatar();
  }
  private async loadOwnAvatar(): Promise<void> {
    this.avatarPath = '';
    const url = this.me?.avatarThumbnailURL;
    if (!url || !url.startsWith('/api/avatars/')) { this.ownAvatarRef = ''; return; }
    const result = await this.api().download(url.substring(4));
    const path = (getContext(this) as common.UIAbilityContext).cacheDir + '/own-avatar-' + util.generateRandomUUID(true) + '.jpg';
    const file = fs.openSync(path, fs.OpenMode.CREATE | fs.OpenMode.WRITE_ONLY | fs.OpenMode.TRUNC);
    try { fs.writeSync(file.fd, result.data); } finally { fs.closeSync(file); }
    await this.store.setValue(this.namespace(), 'own-avatar-sidecar:' + path.slice(path.lastIndexOf('/') + 1), path);
    this.avatarPath = path; this.ownAvatarRef = url;
  }
  private async showReports(): Promise<void> {
    this.reports = (await this.api().request<ReportList>('/reports')).items;
    this.page = 'reports';
  }
  private async submitReport(retry: boolean = false): Promise<void> {
    let payload: ReportDraft = { targetType: this.reportType, targetId: this.reportTarget, reason: this.reportReason.trim() };
    const prior = await this.store.value(this.operationNamespace(), 'operation:report');
    if (retry && prior.length > 0) {
      payload = JSON.parse((JSON.parse(prior) as PendingOperation).payload) as ReportDraft;
      this.reportType = payload.targetType; this.reportTarget = payload.targetId; this.reportReason = payload.reason;
      this.confirmChange('retryOriginal', async () => this.submitReport(false), payload.targetType + ' · ' + payload.targetId + '\n' + payload.reason);
      return;
    }
    if (!['message', 'user', 'conversation'].includes(payload.targetType) || payload.targetId.length === 0 || payload.reason.length === 0 || Array.from(payload.reason).length > 1000) { throw new Error(this.t('reportReason')); }
    const operation = await this.beginOperation('report', JSON.stringify(payload));
    await this.api().request<Report>('/reports', 'POST', JSON.stringify({ targetType: payload.targetType, targetId: payload.targetId, reason: payload.reason, requestId: operation.requestId }));
    await this.finishOperation('report'); this.reportReason = ''; await this.showReports();
  }
  private applyFeatures(bootstrap: Bootstrap): void {
    this.roleAdmissionSupported = Rules.roleAdmissionSupported(bootstrap, this.deploymentId);
    this.forwardingSupported = bootstrap.features?.forwarding === true;
    this.mentionsSupported = bootstrap.features?.mentions === true;
    this.announcementsSupported = bootstrap.features?.announcements === true;
    if (!this.announcementsSupported) { this.announcement = undefined; this.announcementCandidate = undefined; }
  }
  private async refreshMentionProjections(conversation: Conversation): Promise<void> {
    const namespace = this.namespace();
    const key = 'mention-review:' + conversation.id;
    const after = await this.store.value(namespace, key);
    let ids = await this.store.unreadMentionIds(namespace, conversation.id, after);
    if (ids.length === 0 && after.length > 0) { ids = await this.store.unreadMentionIds(namespace, conversation.id, ''); }
    for (const id of ids) {
      try {
        const message = await this.api().request<Message>('/messages/' + encodeURIComponent(id));
        if (namespace !== this.namespace()) { return; }
        if (message.id !== id || message.conversationId !== conversation.id) { throw new Error('Mention source scope mismatch'); }
        await this.store.put(namespace, message, false);
        if (!Rules.mentionUnread(true, message, this.userId, false)) { await this.store.dismissMention(namespace, id); }
      } catch (error) {
        if (namespace !== this.namespace()) { return; }
        if (String(error).includes('HTTP 403') || String(error).includes('HTTP 404')) { await this.store.dismissMention(namespace, id); await this.store.remove(namespace, id, false); }
        else { throw error as Error; }
      }
      await this.store.setValue(namespace, key, id);
    }
  }
  private async selectMentions(): Promise<void> {
    const conversation = this.conversation;
    const namespace = this.namespace();
    if (!conversation || conversation.kind !== 'group' || this.demo || this.editId.length > 0) { return; }
    await this.verifyEpoch();
    if (!this.mentionsSupported) { throw new Error(this.t('featureUnavailable')); }
    const current = await this.api().request<Conversation>('/conversations/' + encodeURIComponent(conversation.id));
    const members = await this.api().request<MemberList>('/conversations/' + encodeURIComponent(conversation.id) + '/members');
    if (namespace !== this.namespace() || this.conversation?.id !== conversation.id) { return; }
    this.conversation = current; this.members = members.items; this.page = 'mentions';
  }
  private toggleMention(member: GroupMember): void {
    if (this.mentionedUserIds.includes(member.id)) { this.mentionedUserIds = this.mentionedUserIds.filter((id: string) => id !== member.id); }
    else {
      if (this.mentionedUserIds.length >= 20) { this.info = this.t('mentionLimit'); return; }
      this.mentionedUserIds = this.mentionedUserIds.concat([member.id]);
    }
    this.mentionAll = false; this.persistDraft();
  }
  private async reconcileMessage(row: Message): Promise<void> {
    const namespace = this.namespace();
    try {
      if (!await this.ensureSync().resolve(row.clientMsgId)) { throw new Error(this.t('originalNotFound')); }
      if (namespace !== this.namespace()) { return; }
      if (this.conversation?.id === row.conversationId) { this.rows = await this.visibleRows(row.conversationId); }
      this.info = this.t('originalMessageFound');
    } catch (error) {
      if (error instanceof ApiFailure && error.status === 404) { throw new Error(this.t('originalNotFound')); }
      throw error as Error;
    }
  }
  private async loadAnnouncement(): Promise<void> {
    const conversation = this.conversation;
    const namespace = this.namespace();
    this.announcement = undefined;
    if (!conversation || conversation.kind !== 'group' || this.demo || !this.announcementsSupported) { return; }
    const identity = this.syncIdentity(); const guard = await this.syncData.ready(identity); const life = this.syncLife;
    const raw = await this.store.value(this.operationNamespace(), 'announcement:' + conversation.id);
    this.announcementPending = raw.length > 0;
    this.announcementRejected = raw.length > 0 && (JSON.parse(raw) as AnnouncementIntent).rejected === true;
    const result = await this.api().request<Announcement>('/conversations/' + encodeURIComponent(conversation.id) + '/announcement');
    if (namespace !== this.namespace() || this.conversation?.id !== conversation.id) { return; }
    if (result.conversationId !== conversation.id || !/^(0|[1-9][0-9]*)$/.test(result.version)) { throw new Error('Invalid announcement'); }
    // Never keep a source body on an unavailable or expired response.
    if (result.state !== 'active') { result.message = undefined; }
    if (result.message) { SyncRules.message(JSON.stringify(result.message)); if (result.message.conversationId !== conversation.id || life !== this.syncLife || !await this.syncData.canShow(identity, guard, result.message.id)) { result.message = undefined; result.state = 'unavailable'; } }
    if (life !== this.syncLife || namespace !== this.namespace()) { return; }
    this.announcement = result;
  }
  private async showAnnouncement(): Promise<void> {
    this.announcementCandidate = undefined; this.page = 'announcement';
    await this.loadAnnouncement();
  }
  private async stageAnnouncement(row: Message): Promise<void> {
    const conversation = this.conversation;
    const namespace = this.namespace();
    if (!conversation || conversation.kind !== 'group' || this.demo) { return; }
    await this.verifyEpoch();
    if (!this.announcementsSupported) { throw new Error(this.t('featureUnavailable')); }
    const identity = this.syncIdentity(); const guard = await this.syncData.ready(identity); const life = this.syncLife;
    const current = await this.api().request<Conversation>('/conversations/' + encodeURIComponent(conversation.id));
    const source = await this.api().request<Message>('/messages/' + encodeURIComponent(row.id));
    if (namespace !== this.namespace() || this.conversation?.id !== conversation.id) { return; }
    SyncRules.message(JSON.stringify(source));
    if (source.id !== row.id || life !== this.syncLife || !await this.syncData.canShow(identity, guard, source.id)) { return; }
    if (source.conversationId !== conversation.id || current.ownerId !== this.userId || !Rules.writable(current)) { throw new Error(this.t('readOnly')); }
    Rules.sourceContentVersion(source);
    this.conversation = current; this.announcementCandidate = source; this.announcementExpiry = 'never'; this.page = 'announcement';
    await this.loadAnnouncement();
  }
  private confirmAnnouncement(clear: boolean): void {
    const conversation = this.conversation;
    const current = this.announcement;
    if (!conversation || !current || !current.canManage || this.demo || this.announcementPending || (!clear && !this.announcementCandidate)) { return; }
    const intent = Rules.announcementIntent(conversation, clear ? undefined : this.announcementCandidate, current.version, util.generateRandomUUID(true), clear ? '' : Rules.announcementExpiry(this.announcementExpiry, Date.now()));
    // The confirmation refers to this snapshot, even if polling finds a newer one.
    this.confirmChange(clear ? 'announcementClear' : 'announcementSet', async () => this.publishAnnouncement(intent));
  }
  private async publishAnnouncement(intent: AnnouncementIntent): Promise<void> {
    const conversation = this.conversation;
    const namespace = this.namespace();
    const operationNamespace = this.operationNamespace();
    if (!conversation || conversation.id !== intent.conversationId || this.demo || this.announcementPending) { return; }
    // Commit the immutable reference request before the first HTTP attempt.
    await this.store.setValue(operationNamespace, 'announcement:' + conversation.id, JSON.stringify(intent));
    if (namespace !== this.namespace()) { return; }
    this.announcementPending = true; this.announcementRejected = false;
    await this.reconcileAnnouncement();
  }
  private async reconcileAnnouncement(): Promise<void> {
    const conversation = this.conversation;
    const namespace = this.namespace();
    const operationNamespace = this.operationNamespace();
    if (!conversation || this.demo) { return; }
    const key = 'announcement:' + conversation.id;
    const raw = await this.store.value(operationNamespace, key);
    if (raw.length === 0) { return; }
    const intent = JSON.parse(raw) as AnnouncementIntent;
    const payload = JSON.parse(intent.body) as AnnouncementMutation;
    if (intent.conversationId !== conversation.id || !['PUT', 'DELETE'].includes(intent.method) || payload.requestId.length < 8) { throw new Error('Original announcement scope mismatch'); }
    await this.verifyEpoch();
    if (!this.announcementsSupported || namespace !== this.namespace()) { throw new Error(this.t('featureUnavailable')); }
    const path = '/conversations/' + encodeURIComponent(intent.conversationId) + '/announcement';
    let operation: AnnouncementOperation;
    try { operation = await this.api().request<AnnouncementOperation>(path + '/operations/' + encodeURIComponent(payload.requestId)); }
    catch (error) {
      if (!String(error).includes('HTTP 404')) { throw error as Error; }
      if (!this.roleAdmissionSupported) { throw new Error(this.t('featureUnavailable')); }
      intent.rejected = false; this.announcementRejected = false;
      await this.store.setValue(operationNamespace, key, JSON.stringify(intent));
      try { operation = await this.api().request<AnnouncementOperation>(path, intent.method === 'PUT' ? 'PUT' : 'DELETE', intent.body); }
      catch (writeError) {
        if (Rules.announcementRejected(String(writeError))) { intent.rejected = true; await this.store.setValue(operationNamespace, key, JSON.stringify(intent)); this.announcementRejected = true; }
        throw writeError as Error;
      }
    }
    if (namespace !== this.namespace()) { return; }
    if (operation.operationId !== payload.requestId || operation.status !== 'applied') { throw new Error('Original announcement incomplete'); }
    // Even current=true is an original fact; GET is the only current display.
    await this.loadAnnouncement();
    if (namespace !== this.namespace()) { return; }
    await this.store.setValue(operationNamespace, key, '');
    this.announcementPending = false; this.announcementRejected = false; this.announcementCandidate = undefined;
    this.info = this.t(operation.current ? 'announcementUpdated' : 'originalOnly');
  }
  private async discardRejectedAnnouncement(): Promise<void> {
    const conversation = this.conversation;
    if (!conversation || !this.announcementRejected) { return; }
    const raw = await this.store.value(this.operationNamespace(), 'announcement:' + conversation.id);
    if (raw.length === 0 || (JSON.parse(raw) as AnnouncementIntent).rejected !== true) { return; }
    await this.store.setValue(this.operationNamespace(), 'announcement:' + conversation.id, '');
    this.announcementPending = false; this.announcementRejected = false; this.announcementCandidate = undefined;
    await this.loadAnnouncement();
  }
  @Builder private sessionsView() {
    Column({ space: 12 }) {
      this.action('back', () => this.page = 'settings')
      Text(this.t('sessions')).fontSize(24 * this.fontScale).fontColor(this.foreground())
      if (this.deviceSessions.length === 0) { this.emptyWith(() => this.run(async () => { this.deviceSessions = (await this.api().request<SessionList>('/auth/sessions')).items; })) }
      ForEach(this.deviceSessions, (device: DeviceSession) => {
        Column({ space: 8 }) {
          Text(device.name + (device.deviceId === this.deviceId ? ' · ' + this.t('currentDevice') : '')).fontColor(this.foreground())
          Text(device.createdAt + ' → ' + device.expiresAt).fontColor(this.foreground())
          this.action('revoke', () => this.confirmChange('revoke', async () => this.revokeSession(device)))
        }.width('100%')
      }, (device: DeviceSession) => device.deviceId)
    }.padding(20)
  }
  @Builder private reportsView() {
    Column({ space: 12 }) {
      this.action('back', () => this.page = 'chats')
      Text(this.t('report')).fontSize(24 * this.fontScale).fontColor(this.foreground())
      Flex({ wrap: FlexWrap.Wrap }) { ForEach(['message', 'user', 'conversation'], (kind: string) => { Button(this.t(kind === 'user' ? 'contacts' : kind === 'conversation' ? 'chats' : 'message')).height(48).backgroundColor(this.reportType === kind ? '#287C66' : this.bubble()).fontColor(this.reportType === kind ? '#FFFFFF' : this.foreground()).onClick(() => { this.reportType = kind; this.reportTarget = ''; }) }) }
      TextInput({ placeholder: this.t('targetId'), text: this.reportTarget }).onChange((value: string) => this.reportTarget = value)
      TextArea({ placeholder: this.t('reportReason'), text: this.reportReason }).height(120).onChange((value: string) => this.reportReason = value)
      this.action('report', () => this.run(async () => this.submitReport()))
      this.action('retryOriginal', () => this.run(async () => this.submitReport(true)))
      this.action('myReports', () => this.run(async () => this.showReports()))
      if (this.reports.length === 0) { this.emptyWith(() => this.run(async () => this.showReports())) }
      ForEach(this.reports, (report: Report) => { Column({ space: 6 }) { Text(report.targetType + ' · ' + report.targetId).fontColor(this.foreground()); Text(report.reason).fontColor(this.foreground()); Text(report.state + ' · ' + report.createdAt).fontColor(this.foreground()) }.width('100%').padding(12).backgroundColor(this.bubble()) }, (report: Report) => report.id)
    }.padding(20).width('100%')
  }
  @Builder private action(label: string, handler: () => void) {
    Button(this.t(label)).type(ButtonType.Normal).borderRadius(10).height(48).fontSize(14 * this.fontScale).backgroundColor('#287C66').fontColor('#FFFFFF').enabled(!this.busy).onClick(handler)
  }
  @Builder private empty() {
    Column({ space: 12 }) { Text(this.t('empty')).fontSize(22 * this.fontScale).fontColor(this.foreground()); Text(this.t('emptyHelp')).fontColor(this.foreground()); this.action('refresh', () => this.run(async () => this.reload())) }.width('100%').padding(32)
  }
  @Builder private emptyWith(retry: () => void) {
    Column({ space: 12 }) {
      Text(this.t('empty')).fontSize(22 * this.fontScale).fontColor(this.foreground())
      Text(this.t('emptyHelp')).fontColor(this.foreground())
      this.action('refresh', retry)
    }.width('100%').padding(32)
  }
  @Builder private authentication() {
    Column({ space: 18 }) {
      Text(this.t('welcome')).fontSize(32 * this.fontScale).fontWeight(FontWeight.Bold).fontColor(this.foreground()).width('100%')
      Text(this.t('connectionHelp')).fontSize(16 * this.fontScale).fontColor(this.foreground()).width('100%')
      Flex({ wrap: FlexWrap.Wrap }) { ForEach(['login', 'register', 'recover'], (key: string) => { Button(this.t(key)).type(ButtonType.Normal).backgroundColor(this.mode === key ? '#287C66' : this.bubble()).fontColor(this.mode === key ? '#FFFFFF' : this.foreground()).height(48).margin({ right: 8, bottom: 8 }).onClick(() => { this.mode = key; this.code = ''; }) }) }
      TextInput({ placeholder: this.t('username'), text: this.username }).height(52).onChange((value: string) => this.username = value)
      TextInput({ placeholder: this.t('password'), text: this.password }).type(InputType.Password).height(52).onChange((value: string) => this.password = value)
      if (this.mode === 'login') { TextInput({ placeholder: this.t('otp'), text: this.code }).height(52).onChange((value: string) => this.code = value) }
      if (this.mode === 'recover') { TextInput({ placeholder: this.t('code'), text: this.code }).height(52).onChange((value: string) => this.code = value) }
      this.action('continue', () => this.run(async () => this.authenticate()))
      if (this.mode === 'login') { this.action('authVerifyAgain', () => this.confirmChange('authVerifyAgain', async () => this.authenticate(true), this.t('authVerifyExplanation'))) }
      Text(this.t('authUnknownHelp')).fontColor(this.foreground()).fontSize(14 * this.fontScale)
      this.action('connect', () => this.page = 'connect')
      this.action('demo', () => this.run(async () => this.preview()))
    }.padding(24).width('100%')
  }
  @Builder private connectionForm() {
    Column({ space: 16 }) {
      Text(this.t('connect')).fontSize(28 * this.fontScale).fontColor(this.foreground())
      Text(this.t('connectionHelp')).fontColor(this.foreground())
      TextInput({ placeholder: this.t('server'), text: this.connectionAddress }).height(52).onChange((value: string) => this.connectionAddress = value)
      TextInput({ placeholder: this.t('expected'), text: this.expectedId }).height(52).onChange((value: string) => this.expectedId = value)
      this.action('connect', () => this.run(async () => this.connect()))
      Text(this.t('trustHelp')).fontColor(this.foreground())
      TextArea({ placeholder: this.t('ca') + ' (PEM)', text: this.caPem }).height(100).onChange((value: string) => this.caPem = value)
      TextInput({ placeholder: this.t('fingerprint'), text: this.caFingerprint }).height(52).onChange((value: string) => this.caFingerprint = value)
    }.padding(24)
  }
  @Builder private settings() {
    Column({ space: 16 }) {
      Text(this.t('language')).fontSize(22 * this.fontScale).fontColor(this.foreground())
      ForEach(Strings.languages, (language: string, index: number) => { Button(Strings.names[index]).type(ButtonType.Normal).height(48).width('100%').backgroundColor(this.language === language ? '#287C66' : this.bubble()).fontColor(this.language === language ? '#FFFFFF' : this.foreground()).onClick(() => { this.language = language; this.saveSetting('language', language); }) })
      Text(this.t('theme')).fontSize(22 * this.fontScale).fontColor(this.foreground())
      Flex({ wrap: FlexWrap.Wrap }) { ForEach(['system', 'light', 'dark'], (theme: string) => { Button(this.t(theme)).height(48).margin(4).onClick(() => { this.applyTheme(theme); this.saveSetting('theme', theme); }) }) }
      Text(this.t('font')).fontSize(22 * this.fontScale).fontColor(this.foreground())
      Slider({ value: this.fontScale, min: 0.85, max: 1.3, step: 0.05 }).onChange((value: number) => { this.fontScale = value; this.saveSetting('fontScale', String(value)); })
      this.exportButton('exportTitle', () => { this.openExports(); })
      Button(this.r('title')).height(52).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).enabled(!this.demo&&Boolean(this.userId)&&!this.busy).onClick(()=>this.openReceipt())
      Button(this.m('title')).height(52).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).enabled(!this.demo&&Boolean(this.userId)&&!this.busy).onClick(()=>this.openMigration())
      Button(this.g('join')).height(52).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).enabled(!this.demo && Boolean(this.userId)).onClick(() => this.openGroupTools(''))
      Button(this.d('title')).height(52).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).onClick(() => this.openDeletion())
      Button(this.p('title')).height(52).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).onClick(() => this.openPassword())
      if (!this.demo && this.selfSupported) { this.selfFontControls(); this.selfButton('title', () => this.run(async () => this.openSelfState())) }
      if (this.token.length > 0) {
        Text(this.me?.displayName ?? this.username).fontColor(this.foreground())
        if (this.avatarPath.length > 0) { Image('file://' + this.avatarPath).width(72).height(72).borderRadius(36) }
        this.action('avatar', () => this.run(async () => this.chooseAvatar()))
        if (this.avatarPending) { this.action('reconcileAvatar', () => this.run(async () => this.reconcileAvatar())) }
        this.action('sessions', () => this.run(async () => { this.deviceSessions = (await this.api().request<SessionList>('/auth/sessions')).items; this.page = 'sessions'; }))
        this.action('myReports', () => this.run(async () => this.showReports()))
        this.action(this.notificationsEnabled ? 'disableNotifications' : 'enableNotifications', () => this.run(async () => {
          if (!this.notificationsEnabled) { await NativeServices.requestNotifications(getContext(this) as common.UIAbilityContext); }
          this.notificationsEnabled = !this.notificationsEnabled;
          await this.prefs?.put('notifications', this.notificationsEnabled); await this.prefs?.flush();
        }))
        TextInput({ placeholder: this.t('password'), text: this.password }).type(InputType.Password).onChange((value: string) => this.password = value)
        this.action('codes', () => this.run(async () => this.generateCodes()))
        ForEach(this.codes, (value: string) => { Text(value).fontColor(this.foreground()).copyOption(CopyOptions.LocalDevice) })
      }
      this.action('logout', () => this.run(async () => this.logout()))
    }.padding(24).width('100%')
  }
  @Builder private exportButton(key: string, action: () => void, enabled: boolean = true) {
    Button(this.e(key)).height(52).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).fontSize(16 * this.fontScale).enabled(enabled && !this.exportBusy).onClick(action)
  }
  @Builder private deletionPage() {
    Column({ space: 12 }) {
      Button(this.t('back')).height(48).onClick(() => { this.closeDeletion(); this.page = 'settings'; })
      Text(this.d('title')).fontSize(24 * this.fontScale).fontColor(this.foreground()).width('100%')
      Text(this.d('help')).fontColor(this.foreground()).width('100%')
      if (this.deletionNotice) { Text(this.deletionNotice).fontColor(this.foreground()).width('100%') }
      if (this.deletionBusy) { LoadingProgress().width(28).height(28) }
      TextInput({ placeholder: this.d('current'), text: this.deletionPassword }).type(InputType.Password).height(52).onChange((value: string) => this.deletionPassword = value)
      TextInput({ placeholder: this.d('recovery'), text: this.deletionRecovery }).type(InputType.Password).height(52).onChange((value: string) => this.deletionRecovery = value)
      TextInput({ placeholder: this.d('otp'), text: this.deletionOTP }).type(InputType.Password).height(52).onChange((value: string) => this.deletionOTP = value)
      Button(this.d('submit')).height(52).width('100%').enabled(!this.deletionBusy && !this.demo && Boolean(this.userId) && Boolean(this.deletionPassword) && !this.deletionRecovery).onClick(() => {
        AlertDialog.show({ title: this.d('title'), message: this.d('help'), primaryButton: { value: this.d('submit'), action: () => this.submitDeletion('', 'request') }, secondaryButton: { value: this.t('cancel'), action: () => {} } });
      })
      Text(this.d('requests')).fontColor(this.foreground()).fontSize(20 * this.fontScale)
      Button(this.t('refresh')).height(48).enabled(!this.deletionBusy).onClick(() => this.run(async () => this.loadDeletionRows()))
      if (!this.deletionRows.length) { this.emptyWith(() => this.run(async () => this.loadDeletionRows())) }
      ForEach(this.deletionRows.slice(0, this.deletionShown), (row: DeletionIntent) => {
        Column({ space: 8 }) {
          Text(row.meta.username + ' · ' + row.meta.created).fontColor(this.foreground())
          Text(this.d(row.state)).fontColor(this.foreground())
          if (this.deletionUntil(row)) { Text(this.d('deadline') + ' · ' + this.deletionUntil(row)).fontColor(this.foreground()) }
          if (DeletionRules.cancelFrozen(row)) { Text(this.d('cancelFrozen')).fontColor(this.foreground()) }
          else if (row.cancelState === 'unknown') { Text(this.d('cancelQueryFirst')).fontColor(this.foreground()) }
          if (row.result) {
            Button(this.d('proofQuery')).height(48).width('100%').enabled(!this.deletionBusy && Boolean(this.deletionPassword) !== Boolean(this.deletionRecovery)).onClick(() => this.submitDeletion(row.meta.id, 'query'))
            if (row.state === 'requested' && DeletionRules.cancelReady(row)) { Button(this.d('proofCancel')).height(48).width('100%').enabled(!this.deletionBusy && Boolean(this.deletionPassword) !== Boolean(this.deletionRecovery)).onClick(() => {
              const version = this.deletionVersion(row);
              AlertDialog.show({ title: this.d('title'), message: this.d('confirmCancel'), primaryButton: { value: this.d('proofCancel'), action: () => this.submitDeletion(row.meta.id, 'cancel', version) }, secondaryButton: { value: this.t('cancel'), action: () => {} } });
            }) }
          }
          Button(this.d('check')).height(48).width('100%').enabled(!this.deletionBusy && Boolean(this.deletionPassword) && !this.deletionRecovery).onClick(() => this.submitDeletion(row.meta.id, 'lookup'))
          if (['unsent', 'unknown', 'pending'].includes(row.state)) { Button(this.d('retry')).height(48).width('100%').enabled(!this.deletionBusy && Boolean(this.deletionPassword) && !this.deletionRecovery).onClick(() => this.submitDeletion(row.meta.id, 'retry')) }
        }.width('100%').padding(12).backgroundColor(this.bubble())
      }, (row: DeletionIntent) => row.meta.id)
      if (this.deletionRows.length > this.deletionShown) { Button(this.t('loadMore')).height(48).onClick(() => this.deletionShown += 20) }
    }.padding(20).width('100%')
  }
  @Builder private passwordPage() {
    Column({ space: 12 }) {
      Button(this.t('back')).height(48).onClick(() => { this.clearPasswordInput(); this.passwordLife++; this.passwordBusy = false; this.page = 'settings'; })
      Text(this.p('title')).fontSize(24 * this.fontScale).fontColor(this.foreground()).width('100%')
      Text(this.p('help')).fontColor(this.foreground()).width('100%')
      if (this.passwordNotice) { Text(this.passwordNotice).fontColor(this.foreground()).width('100%') }
      if (this.passwordBusy) { LoadingProgress().width(28).height(28) }
      TextInput({ placeholder: this.p('current'), text: this.passwordOld }).type(InputType.Password).height(52).onChange((value: string) => this.passwordOld = value)
      TextInput({ placeholder: this.p('new'), text: this.passwordNew }).type(InputType.Password).height(52).onChange((value: string) => this.passwordNew = value)
      TextInput({ placeholder: this.p('confirm'), text: this.passwordConfirm }).type(InputType.Password).height(52).onChange((value: string) => this.passwordConfirm = value)
      TextInput({ placeholder: this.p('otp'), text: this.passwordOTP }).type(InputType.Password).height(52).onChange((value: string) => this.passwordOTP = value)
      Button(this.p('submit')).height(52).width('100%').enabled(!this.passwordBusy && !this.demo && Boolean(this.userId)).onClick(() => {
        AlertDialog.show({ title: this.p('title'), message: this.p('help'), primaryButton: { value: this.p('submit'), action: () => this.submitPassword('', false) }, secondaryButton: { value: this.t('cancel'), action: () => {} } });
      })
      Text(this.p('requests')).fontColor(this.foreground()).fontSize(20 * this.fontScale)
      Button(this.t('refresh')).height(48).enabled(!this.passwordBusy).onClick(() => this.run(async () => this.loadPasswordRows()))
      if (!this.passwordRows.length) { this.emptyWith(() => this.run(async () => this.loadPasswordRows())) }
      ForEach(this.passwordRows.slice(0, this.passwordShown), (row: PasswordIntent) => {
        Column({ space: 8 }) {
          Text(row.username + ' · ' + row.created).fontColor(this.foreground())
          Text(this.p(row.state)).fontColor(this.foreground())
          if (PasswordRules.blocked(row)) { Text(this.p('blocked')).fontColor(this.foreground()) }
          if (row.state === 'changed') { Text(this.p('login')).fontColor(this.foreground()) }
          Button(this.p('check')).height(48).width('100%').enabled(!this.passwordBusy).onClick(() => this.submitPassword(row.id, false))
          if (['unsent', 'unknown', 'pending'].includes(row.state)) { Button(this.p('retry')).height(48).width('100%').enabled(!this.passwordBusy).onClick(() => this.submitPassword(row.id, true)) }
        }.width('100%').padding(12).backgroundColor(this.bubble())
      }, (row: PasswordIntent) => row.id)
      if (this.passwordRows.length > this.passwordShown) { Button(this.t('loadMore')).height(48).onClick(() => this.passwordShown += 20) }
    }.padding(20).width('100%')
  }
  private exportState(view: ExportView | undefined): string {
    if (!view) { return this.e('exportUnknown'); }
    if (view.cleanup === 'source_handoff_pending' || ['revoking', 'cleaning'].includes(view.cleanup)) { return this.e('exportCleanup'); }
    if (Date.parse(view.expires) <= Date.now()) { return this.e('exportExpired'); }
    switch (view.state) {
      case 'requested': return this.e('exportRequested');
      case 'building': return this.e('exportBuilding');
      case 'ready': return this.e('exportReady');
      case 'cancelled': return this.e('exportCancelled');
      case 'expired': return this.e('exportExpired');
      case 'failed': return this.e('exportFailed');
      default: return this.e('exportUnknown');
    }
  }
  @Builder private exportsPage() {
    Column({ space: 16 }) {
      Button(this.t('back')).height(48).onClick(() => { this.page = 'settings'; })
      Text(this.e('exportTitle')).fontSize(26 * this.fontScale).fontWeight(FontWeight.Bold).fontColor(this.foreground()).width('100%')
      Text(this.e('exportSummary')).fontSize(16 * this.fontScale).fontColor(this.foreground()).width('100%')
      Text(this.e('exportLimits')).fontSize(14 * this.fontScale).fontColor(this.foreground()).width('100%')
      if (this.exportNotice) { Text(this.exportNotice).fontColor(this.foreground()).width('100%') }
      if (this.exportBusy) { LoadingProgress().width(32).height(32).color('#287C66') }
      if (this.exportDownloading) { Button(this.e('exportStopDownload')).height(52).onClick(() => this.stopExportDownload()) }
      this.exportButton('exportCreate', () => this.runExport(async () => this.exportAction('', 'create')), this.exportSupported && !this.demo && this.token.length > 0)
      if (!this.exportSupported) { Text(this.e(this.demo || !this.token ? 'exportConnect' : 'exportRestricted')).fontColor(this.foreground()).width('100%') }
      Button(this.t('refresh')).height(48).enabled(!this.exportBusy).onClick(() => { if (this.exportService) { this.runExport(async () => this.reloadExports()); } else { this.openExports(); } })
      Text(this.e('exportLocalList')).fontSize(20 * this.fontScale).fontWeight(FontWeight.Medium).fontColor(this.foreground()).width('100%')
      Text(this.e('exportLocalOnly')).fontSize(14 * this.fontScale).fontColor(this.foreground()).width('100%')
      if (!this.exportRows.length) {
        Column({ space: 16 }) { Image($r('app.media.app_icon')).width(64).height(64); Text(this.e('exportEmpty')).fontColor(this.foreground()); Button(this.t('refresh')).height(48).enabled(!this.exportBusy).onClick(() => { if (this.exportService) { this.runExport(async () => this.reloadExports()); } else { this.openExports(); } }) }.padding(24).width('100%').backgroundColor(this.bubble()).borderRadius(16)
      }
      ForEach(this.exportRows.slice(0, this.exportVisibleCount), (row: ExportIntent) => {
        Column({ space: 12 }) {
          Text(this.exportState(this.exportViews[row.id])).fontSize(20 * this.fontScale).fontWeight(FontWeight.Medium).fontColor(this.foreground()).width('100%')
          Text(row.created + ' · ' + row.id.slice(0, 8)).fontSize(13 * this.fontScale).fontColor(this.foreground()).width('100%')
          if (this.exportViews[row.id]) { Text(this.e('exportDeadline') + ' ' + this.exportViews[row.id].expires).fontSize(13 * this.fontScale).fontColor(this.foreground()).width('100%') }
          this.exportButton('exportCheck', () => this.runExport(async () => this.exportAction(row.id, 'check')))
          if (!row.job) { Text(this.e('exportPendingHint')).fontColor(this.foreground()); this.exportButton('exportRetry', () => this.runExport(async () => this.exportAction(row.id, 'retry')), this.exportSupported) }
          if (row.job && (row.cancelBody && this.exportViews[row.id]?.cleanup !== 'cleared' || !['cancelled', 'expired', 'failed'].includes(this.exportViews[row.id]?.state ?? ''))) { this.exportButton(row.cancelBody ? 'exportRetryCancel' : 'exportCancel', () => this.confirmExportCancel(row)) }
          if (this.exportViews[row.id] && ExportRules.ready(this.exportViews[row.id]) && !row.cancelBody) { this.exportButton('exportDownload', () => this.runExport(async () => this.downloadExport(row)), this.exportSupported); Text(this.e('exportDownloadHint')).fontSize(13 * this.fontScale).fontColor(this.foreground()); Text(this.e('exportLimit')).fontSize(13 * this.fontScale).fontColor(this.foreground()) }
        }.padding(16).borderRadius(16).backgroundColor(this.bubble()).width('100%')
      }, (row: ExportIntent) => row.id)
      if (this.exportVisibleCount < this.exportRows.length) { this.exportButton('exportMore', () => this.runExport(async () => { this.exportVisibleCount += 20; await this.reloadExports(); })) }
    }.padding(20).width('100%')
  }
  @Builder private chats() {
    Column({ space: 12 }) {
      Row() { Text(this.t('chats')).fontSize(28 * this.fontScale).fontWeight(FontWeight.Bold).fontColor(this.foreground()).layoutWeight(1); this.action('newChat', () => this.page = 'new') }.width('100%')
      this.action('refresh', () => this.run(async () => { this.localEntries = false; await this.reload(); }))
      if (!this.demo && this.selfSupported) { this.selfButton('all', () => this.run(async () => { this.selfAll = !this.selfAll; await this.reload(); })) }
      if (this.syncRestricted) { Text(SyncText.text(this.language, 'restricted')).fontColor(this.foreground()).width('100%') }
      if (!this.demo && this.token.length > 0) { Button(SyncText.text(this.language, 'local')).height(48).onClick(() => this.run(async () => this.showLocalHistory())) }
      if (this.localEntries) { Text(SyncText.text(this.language, 'localOnly')).fontColor(this.foreground()).width('100%') }
      Flex({ wrap: FlexWrap.Wrap }) { this.action('savedMessages', () => this.run(async () => this.showLookup('favorites'))); this.action('searchMessages', () => this.run(async () => this.showLookup('search'))) }
      if (this.conversations.length === 0) { this.empty() }
      ForEach(this.conversations, (row: Conversation) => { Row({ space: 16 }) { Text(row.title.slice(0, 1)).fontSize(24).width(48).height(48).textAlign(TextAlign.Center).backgroundColor(this.bubble()).borderRadius(12); Column({ space: 6 }) { Text(row.title + (this.selfViews[row.id]?.manualUnread ? ' ●' : '') + ((this.unread[row.id] ?? 0) > 0 ? ' · ' + String(this.unread[row.id]) : '')).fontSize(18 * this.fontScale).fontColor(this.foreground()).width('100%'); Text(this.t(row.kind === 'group' ? 'group' : 'private') + (this.selfViews[row.id]?.manualUnread ? ' · ' + this.s('manual') : '') + ((this.mentionUnread[row.id] ?? 0) > 0 ? ' · ' + this.t('mentionUnread') + ' ' + String(this.mentionUnread[row.id]) : '')).fontSize(13 * this.fontScale).fontColor(this.foreground()).width('100%') }.layoutWeight(1) }.width('100%').padding({ top: 12, bottom: 12 }).onClick(() => this.run(async () => this.open(row))) }, (row: Conversation) => row.id)
    }.padding(20).width('100%')
  }
  @Builder private contacts() {
    Column({ space: 16 }) {
      this.action('refresh', () => this.run(async () => this.reload()))
      Text(this.t('requests')).fontSize(22 * this.fontScale).fontColor(this.foreground())
      if (this.requests.length === 0) { this.empty() }
      ForEach(this.requests, (request: ContactRequest) => {
        Column({ space: 8 }) {
          Text(request.requester?.displayName ?? this.t('unavailableUser')).fontColor(this.foreground())
          Text(request.requester?.username ?? request.requesterId).fontColor(this.foreground())
          Row({ space: 8 }) {
            if (request.requester) { this.action('accept', () => this.run(async () => this.requestAction(request, 'accept'))) }
            this.action('reject', () => this.run(async () => this.requestAction(request, 'reject')))
          }
        }.width('100%').padding(12).backgroundColor(this.bubble())
      }, (request: ContactRequest) => request.id)
      Text(this.t('contacts')).fontSize(22 * this.fontScale).fontColor(this.foreground())
      if (this.people.length === 0) { this.empty() }
      ForEach(this.people, (person: User) => {
        Column({ space: 8 }) {
          Text(person.displayName + ' · @' + person.username).fontColor(this.foreground())
          Flex({ wrap: FlexWrap.Wrap }) {
            this.action('newChat', () => this.run(async () => this.directConversation(person)))
            this.action('report', () => { this.reportType = 'user'; this.reportTarget = person.id; this.reportReason = ''; this.page = 'reports'; })
            this.action('deleteContact', () => this.confirmChange('deleteContact', async () => this.contactAction(person.id, 'deleteContact')))
            this.action('block', () => this.confirmChange('block', async () => this.contactAction(person.id, 'block')))
          }
        }.width('100%')
      }, (person: User) => person.id)
      TextInput({ placeholder: this.t('search'), text: this.query }).height(52).onChange((value: string) => this.query = value)
      this.action('search', () => this.run(async () => this.searchPeople()))
      if (this.query.length > 0 && this.results.length === 0) { this.emptyWith(() => this.run(async () => this.searchPeople())) }
      ForEach(this.results, (person: User) => {
        Row() {
          Text(person.displayName + ' · @' + person.username).layoutWeight(1).fontColor(this.foreground())
          this.action('add', () => this.run(async () => this.contactAction(person.id, 'add')))
        }
      }, (person: User) => person.id)
      Text(this.t('localBlocks')).fontColor(this.foreground())
      ForEach(this.blockedIds, (id: string) => {
        Row() { Text(id).layoutWeight(1).fontColor(this.foreground()); this.action('unblock', () => this.run(async () => this.contactAction(id, 'unblock'))) }
      })
      TextInput({ placeholder: this.t('peerId'), text: this.managedPeer }).onChange((value: string) => this.managedPeer = value)
      Flex({ wrap: FlexWrap.Wrap }) {
        this.action('unblock', () => this.run(async () => this.contactAction(this.managedPeer, 'unblock')))
        this.action('reopen', () => this.run(async () => this.contactAction(this.managedPeer, 'reopen')))
      }
      Text(this.t('reopenHelp')).fontColor(this.foreground())
    }.padding(20)
  }
  @Builder private receiptButton(key:string,handler:()=>void){Button(this.r(key)).height(48).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).enabled(!this.receiptBusy&&Boolean(this.receiptService)).onClick(handler)}
  @Builder private receiptPage(){
    Column({space:12}){
      Button(this.m('back')).height(48).onClick(()=>this.page='settings')
      Text(this.r('title')).fontSize(24*this.fontScale).fontColor(this.foreground()).width('100%')
      Text(this.r('help')).fontColor(this.foreground())
      if(this.receiptBusy){LoadingProgress().width(32).height(32)}
      if(this.receiptNotice){Text(this.receiptNotice).fontColor(this.foreground())}
      Text(this.r(this.receiptQualification())).fontColor(this.foreground())
      Button(this.m('reload')).height(48).width('100%').enabled(!this.receiptBusy).onClick(()=>{if(this.receiptService){this.receiptAction(async(service:ReceiptService)=>this.reloadReceipt(service));}else{this.openReceipt();}})
      this.receiptButton('verify',()=>this.receiptAction(async(service:ReceiptService)=>{const result=SyncJSON.parse(await service.measure(true));if(this.receiptService===service&&result.get('restartRequired').value===true){this.receiptNotice=this.r('restart');}}))
      if(!this.receiptReport){Text(this.r('notVerified')).fontColor(this.foreground())}else{this.receiptButton('report',()=>this.copyReceiptReport())}
      Row(){Checkbox().select(this.receiptService?this.receiptAutomaticFor(this.receiptService.principal()):false).enabled(!this.receiptBusy&&this.receiptQualified()).onChange((value:boolean)=>this.setReceiptAutomatic(value));Text(this.r('automatic')).fontColor(this.foreground()).layoutWeight(1)}.width('100%')
      Text(this.r('automaticHelp')).fontColor(this.foreground())
      Text(this.r('profiles')).fontSize(20*this.fontScale).fontColor(this.foreground())
      if(!this.receiptProfiles.length){this.emptyWith(()=>this.openReceipt())}
      ForEach(this.receiptProfiles,(raw:string)=>{Column({space:6}){Text(SyncJSON.parse(raw).get('id').string()).fontColor(this.foreground());this.receiptButton('declare',()=>this.receiptAction(async(service:ReceiptService)=>{const row=await service.declaration(SyncJSON.parse(raw).get('id').string());await service.resume(row.id,true);}));}.width('100%').padding(12).backgroundColor(this.bubble())},(raw:string)=>SyncJSON.parse(raw).get('id').string())
      Text(this.r('saveBody')).fontSize(20*this.fontScale).fontColor(this.foreground())
      if(!this.rows.length){this.emptyWith(()=>this.openReceipt())}
      ForEach(this.rows.filter((row:Message)=>!row.deleted&&!row.forwarded&&!['pending','frozen'].includes(row.state??'')).slice(0,this.receiptShown),(row:Message)=>{Column(){Text(row.text||this.m('metadata')).fontColor(this.foreground());Button(this.r('saveBody')).height(48).enabled(!this.receiptBusy&&this.receiptQualified()).onClick(()=>this.receiptAction(async(service:ReceiptService)=>{await service.content(row.id,row.contentVersion??'');}));}.width('100%')},(row:Message)=>row.id)
      Text(this.m('operations')).fontSize(20*this.fontScale).fontColor(this.foreground())
      if(!this.receiptRows.length){this.emptyWith(()=>this.openReceipt())}
      ForEach(this.receiptRows.slice(0,this.receiptShown),(row:ReceiptIntent)=>{Column({space:6}){Text(this.r(row.kind==='message_content'?'content':row.kind==='object_received'?'object':'storage')+' · '+this.r(row.state)).fontColor(this.foreground());if(row.message){Text(row.message).fontColor(this.foreground())}if(row.state!=='retired'){Button(this.m('check')).height(48).enabled(!this.receiptBusy).onClick(()=>this.receiptAction(async(service:ReceiptService)=>{await service.resume(row.id,false);}));if(row.state!=='recorded'){Button(this.m('retry')).height(48).enabled(!this.receiptBusy).onClick(()=>this.receiptAction(async(service:ReceiptService)=>{await service.resume(row.id,true);}));}}if(row.kind==='message_content'&&row.state==='recorded'){Button(this.r('saveObject')).height(48).enabled(!this.receiptBusy&&this.receiptQualified(true)).onClick(()=>this.receiptAction(async(service:ReceiptService)=>{await service.objectOriginal(row.id);}));}if(row.kind!=='storage'&&row.state!=='retired'){this.receiptButton('target',()=>this.receiptAction(async(service:ReceiptService)=>{const raw=await service.targets(row.id);if(this.receiptService===service){this.receiptTarget=raw;}}));}}.width('100%').padding(12).backgroundColor(this.bubble())},(row:ReceiptIntent)=>row.identity.namespace+row.id)
      if(this.receiptTarget){Text(this.r('contentRecorded')+' · '+this.receiptTargetText('contentRecorded')).fontColor(this.foreground());Text(this.r('objectRecorded')+' · '+this.receiptTargetText('objectRecorded')).fontColor(this.foreground());Text(this.r('rangeTime')+' · '+this.receiptTargetText('contentCompleteAt')).fontColor(this.foreground())}
      Text(this.m('archive')).fontSize(20*this.fontScale).fontColor(this.foreground())
      if(!this.receiptArchive.length){this.emptyWith(()=>this.openReceipt())}
      ForEach(this.receiptArchive.slice(0,this.receiptShown),(raw:string)=>{Column(){Text(SyncJSON.parse(raw).get('text').string()).fontColor(this.foreground()).width('100%');if(SyncJSON.parse(raw).has('sourceObjectReceiptId')){Button(this.m('original')).height(48).enabled(!this.receiptBusy).onClick(()=>this.saveReceiptOriginal(SyncJSON.parse(raw).get('sourceReceiptId').string()))}}.width('100%')},(raw:string)=>SyncJSON.parse(raw).get('id').string())
      if(this.rows.length>this.receiptShown||this.receiptRows.length>this.receiptShown||this.receiptArchive.length>this.receiptShown){Button(this.m('more')).height(48).onClick(()=>this.receiptShown+=20)}
    }.padding(20).width('100%')
  }
  @Builder private migrationButton(key:string,handler:()=>void){Button(this.m(key)).height(48).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).enabled(!this.migrationBusy&&Boolean(this.migrationService)).onClick(handler)}
  @Builder private migrationPage(){
    Column({space:12}){
      Button(this.m('back')).height(48).onClick(()=>this.page='settings')
      Text(this.m('title')).fontSize(24*this.fontScale).fontColor(this.foreground()).width('100%')
      Text(this.m('help')).fontColor(this.foreground())
      if(this.migrationBusy){LoadingProgress().width(32).height(32)}
      if(this.migrationNotice){Text(this.migrationNotice).fontColor(this.foreground())}
      Button(this.m('reload')).height(48).width('100%').enabled(!this.migrationBusy).onClick(()=>{if(this.migrationService){this.migrationAction(async(service:MigrationService,_p:string,_o:string,_c:string)=>this.reloadMigration(service));}else{this.openMigration(false);}})
      TextInput({placeholder:this.m('password'),text:this.migrationPassword}).type(InputType.Password).onChange((value:string)=>this.migrationPassword=value)
      TextInput({placeholder:this.m('otp'),text:this.migrationOTP}).type(InputType.Password).onChange((value:string)=>this.migrationOTP=value)
      this.migrationButton('pair',()=>this.migrationAction(async(service:MigrationService,p:string,o:string,_c:string)=>{const row=await service.stage('pairing');await service.pair(row.id,p,o,true);}))
      Text(this.m('source')).fontSize(20*this.fontScale).fontColor(this.foreground())
      TextInput({placeholder:this.m('code'),text:this.migrationCode}).type(InputType.Password).onChange((value:string)=>this.migrationCode=value)
      Toggle({type:ToggleType.Checkbox,isOn:this.migrationFull}).onChange((value:boolean)=>this.migrationFull=value)
      Text(this.m('full')).fontColor(this.foreground())
      if(!this.migrationCandidates.length&&!this.migrationQualified.length){this.emptyWith(()=>this.openMigration())}
      ForEach(this.migrationCandidates.slice(0,this.migrationShown),(row:Message)=>{Row(){Checkbox().select(this.migrationSelected.includes(row.id)).onChange((checked:boolean)=>this.migrationSelected=checked?this.migrationSelected.concat([row.id]):this.migrationSelected.filter((id:string)=>id!==row.id));Text(row.text||this.m('metadata')).fontColor(this.foreground()).layoutWeight(1)}.width('100%')},(row:Message)=>row.id)
      ForEach(this.migrationQualified.slice(0,this.migrationShown),(raw:string)=>{Row(){Checkbox().select(this.migrationSelected.includes(SyncJSON.parse(raw).get('id').string())).onChange((checked:boolean)=>{const id=SyncJSON.parse(raw).get('id').string();this.migrationSelected=checked?this.migrationSelected.concat([id]):this.migrationSelected.filter((item:string)=>item!==id);});Text(SyncJSON.parse(raw).get('text').string()).fontColor(this.foreground()).layoutWeight(1)}.width('100%')},(raw:string)=>SyncJSON.parse(raw).get('id').string())
      this.migrationButton('send',()=>{const selection=this.migrationSelection();this.migrationAction(async(service:MigrationService,p:string,o:string,c:string)=>{const row=await service.stage('create',selection);const result=await service.create(row.id,c,p,o,true);if(result&&this.migrationService===service){this.migrationID=result.id;}});})
      Text(this.m('target')).fontSize(20*this.fontScale).fontColor(this.foreground())
      TextInput({placeholder:this.m('package'),text:this.migrationID}).onChange((value:string)=>this.migrationID=value)
      this.migrationButton('install',()=>{const id=this.migrationID;this.migrationAction(async(service:MigrationService,p:string,o:string,_c:string)=>{if(!await service.install(id,p,o)&&this.migrationService===service){this.migrationNotice=this.m('local');}});})
      Text(this.m('operations')).fontSize(20*this.fontScale).fontColor(this.foreground())
      if(!this.migrationRows.length){this.emptyWith(()=>this.openMigration(false))}
      ForEach(this.migrationRows.slice(0,this.migrationShown),(row:MigrationIntent)=>{Column({space:6}){Text(this.m(row.kind)+' · '+this.m(row.state)).fontColor(this.foreground());Text(row.created).fontColor(this.foreground());if(row.migration){Text(row.migration).fontColor(this.foreground()).copyOption(CopyOptions.LocalDevice)}
        this.migrationButton('check',()=>this.migrationAction(async(service:MigrationService,_p:string,_o:string,_c:string)=>{await service.checkOriginal(row.id);}))
        if(row.kind==='pairing'){this.migrationButton('copy',()=>this.copyMigrationPair(row.id))}
        if(row.kind!=='target'&&['unsent','unknown'].includes(row.state)){this.migrationButton('retry',()=>this.migrationAction(async(service:MigrationService,p:string,o:string,c:string)=>{if(row.kind==='pairing'){await service.pair(row.id,p,o,true);}else{await service.create(row.id,c,p,o,true);}}))}
        if(row.migration&&row.state!=='retired'){this.migrationButton('cancel',()=>this.migrationAction(async(service:MigrationService,_p:string,_o:string,_c:string)=>{await service.cancel(row.migration);}))}
      }.width('100%').padding(12).backgroundColor(this.bubble())},(row:MigrationIntent)=>row.identity.namespace+row.id)
      Text(this.m('archive')).fontSize(20*this.fontScale).fontColor(this.foreground())
      if(!this.migrationHistory.length){this.emptyWith(()=>this.openMigration(false))}
      ForEach(this.migrationHistory.slice(0,this.migrationShown),(raw:string)=>{Column({space:6}){Text(this.migrationField(raw,'createdAt')).fontColor(this.foreground());Text(SyncJSON.parse(raw).get('contentUnavailable').value===true?this.m('unavailable'):this.migrationField(raw,'text')).fontColor(this.foreground()).copyOption(CopyOptions.LocalDevice);if(this.migrationCanSave(raw)){this.migrationButton('original',()=>this.saveMigrationObject(this.migrationField(raw,'id')))}else if(this.migrationField(raw,'type')!=='text'){Text(this.m('metadata')).fontColor(this.foreground())}}.width('100%').padding(12).backgroundColor(this.bubble())},(raw:string)=>this.migrationField(raw,'id'))
      if(this.migrationQualified.length>this.migrationShown||this.migrationRows.length>this.migrationShown||this.migrationCandidates.length>this.migrationShown||this.migrationHistory.length>this.migrationShown){Button(this.m('more')).height(48).onClick(()=>this.migrationShown+=20)}
    }.padding(20).width('100%')
  }
  @Builder private groupToolsPage() {
    Column({ space: 12 }) {
      Button(this.t('back')).height(48).onClick(() => { this.closeGroupTools(); this.page = this.groupControlId ? 'group' : 'settings'; })
      Text(this.g('title')).fontSize(24 * this.fontScale).fontColor(this.foreground()).width('100%')
      Text(this.g('help')).fontColor(this.foreground())
      Text(this.g('privileged')).fontColor(this.foreground()).width('100%')
      if (this.groupNotice) { Text(this.groupNotice).fontColor(this.foreground()).width('100%') }
      if (this.groupBusy) { LoadingProgress().width(28).height(28) }
      Button(this.t('refresh')).height(48).enabled(!this.groupBusy).onClick(() => this.openGroupTools(this.groupControlId))
      TextInput({ placeholder: this.g('code'), text: this.groupCode }).type(InputType.Password).height(52).onChange((value: string) => this.groupCode = value)
      Button(this.g('join')).height(48).width('100%').enabled(!this.groupBusy && Boolean(this.groupCode)).onClick(() => this.joinGroupCode())
      if (this.groupOwner()) {
        Text(this.g('settings')).fontSize(20 * this.fontScale).fontColor(this.foreground())
        Row({ space: 10 }) { Toggle({ type: ToggleType.Switch, isOn: this.groupMuteAll }).onChange((value: boolean) => this.groupMuteAll = value); Text(this.g('muteAll')).fontColor(this.foreground()) }.width('100%')
        TextInput({ placeholder: this.g('slow'), text: this.groupSlow }).type(InputType.Number).height(52).onChange((value: string) => this.groupSlow = value)
        TextInput({ placeholder: this.g('capacity'), text: this.groupCapacity }).type(InputType.Number).height(52).onChange((value: string) => this.groupCapacity = value)
        Button(this.g('save')).height(48).enabled(!this.groupBusy && /^[0-9]+$/.test(this.groupSlow) && Number(this.groupSlow) <= 3600 && /^[0-9]+$/.test(this.groupCapacity) && Number(this.groupCapacity) >= 2 && Number(this.groupCapacity) <= 10000).onClick(() => this.stageGroup('settings', undefined, JSON.stringify({ muteAll: this.groupMuteAll, slowSeconds: Number(this.groupSlow), maxMembers: Number(this.groupCapacity) }), this.g('save')))
        Button(this.g('dissolve')).height(48).enabled(!this.groupBusy).onClick(() => this.stageGroup('dissolve'))
      }
      if (this.groupRecover()) { TextInput({ placeholder: this.t('otp'), text: this.groupOTP }).type(InputType.Password).height(52).onChange((value: string) => this.groupOTP = value) }
      if (this.groupControlId) { Text(this.t('membersList')).fontSize(20 * this.fontScale).fontColor(this.foreground()) }
      if (this.groupControlId && !this.members.length) { this.emptyWith(() => this.openGroupTools(this.groupControlId)) }
      ForEach(this.members, (member: GroupMember) => {
        Column({ space: 8 }) {
          Text(member.displayName + ' · ' + member.username).fontColor(this.foreground())
          if (this.groupOwner() && member.id !== this.userId) {
            Button(this.t('transfer')).height(48).enabled(!this.groupBusy).onClick(() => this.stageGroup('transfer', member))
            Button(this.g(member.role === 'admin' ? 'makeMember' : 'makeAdmin')).height(48).enabled(!this.groupBusy).onClick(() => this.stageGroup('role', member, JSON.stringify({ role: member.role === 'admin' ? 'member' : 'admin' }), this.g(member.role === 'admin' ? 'makeMember' : 'makeAdmin')))
          }
          if (this.groupManager() && member.role !== 'owner' && (this.groupOwner() || member.role !== 'admin')) {
            Button(this.g('mute')).height(48).enabled(!this.groupBusy).onClick(() => this.stageGroup('mute', member, JSON.stringify({ mutedUntil: new Date(Date.now() + 3600000).toISOString() }), this.g('mute')))
            Button(this.g('unmute')).height(48).enabled(!this.groupBusy).onClick(() => this.stageGroup('mute', member, JSON.stringify({ mutedUntil: '' }), this.g('unmute')))
          }
          if (this.groupRecover() && member.state !== 'tombstoned' && member.state !== 'banned') { Button(this.g('recover')).height(48).enabled(!this.groupBusy && /^[0-9]{6}$/.test(this.groupOTP)).onClick(() => this.stageGroup('recover', member)) }
        }.width('100%').padding(12).backgroundColor(this.bubble())
      }, (member: GroupMember) => member.id + ':' + member.membershipGeneration)
      if (this.groupMembersNext) { Button(this.t('loadMore')).height(48).enabled(!this.groupBusy).onClick(() => this.groupMore('members')) }
      if (this.groupManager()) {
        Text(this.g('invites')).fontSize(20 * this.fontScale).fontColor(this.foreground())
        TextInput({ placeholder: this.g('uses'), text: this.groupUses }).type(InputType.Number).height(52).onChange((value: string) => this.groupUses = value)
        Row({ space: 10 }) { Toggle({ type: ToggleType.Switch, isOn: this.groupApproval }).onChange((value: boolean) => this.groupApproval = value); Text(this.g('approval')).fontColor(this.foreground()) }.width('100%')
        Text(this.g('expires')).fontColor(this.foreground())
        Button(this.g('invite')).height(48).enabled(!this.groupBusy && /^[0-9]+$/.test(this.groupUses) && Number(this.groupUses) >= 1 && Number(this.groupUses) <= 10000).onClick(() => this.stageGroup('invite', undefined, JSON.stringify({ maxUses: Number(this.groupUses), approvalRequired: this.groupApproval, expiresAt: new Date(Date.now() + 86400000).toISOString() })))
        if (!this.groupInvites.length) { this.emptyWith(() => this.openGroupTools(this.groupControlId)) }
        ForEach(this.groupInvites, (raw: string) => {
          Column({ space: 8 }) {
            Text(this.groupField(raw, 'expiresAt') + ' · ' + this.groupField(raw, 'uses') + '/' + this.groupField(raw, 'maxUses')).fontColor(this.foreground())
            if (!this.groupField(raw, 'revokedAt')) { Button(this.g('copy')).height(48).enabled(!this.groupBusy).onClick(() => this.copyGroupCode(this.groupField(raw, 'id'))); Button(this.g('revoke')).height(48).enabled(!this.groupBusy).onClick(() => this.stageGroup('revoke', undefined, JSON.stringify({ id: this.groupField(raw, 'id') }))) }
          }.width('100%').padding(12).backgroundColor(this.bubble())
        }, (raw: string) => this.groupField(raw, 'id'))
        if (this.groupInvitesNext) { Button(this.t('loadMore')).height(48).enabled(!this.groupBusy).onClick(() => this.groupMore('invites')) }
        Text(this.g('requests')).fontSize(20 * this.fontScale).fontColor(this.foreground())
        if (!this.groupRequests.length) { this.emptyWith(() => this.openGroupTools(this.groupControlId)) }
        ForEach(this.groupRequests, (raw: string) => {
          Column({ space: 8 }) {
            Text((this.groupField(raw, 'displayName') || this.groupField(raw, 'username') || this.t('unavailableUser')) + ' · ' + this.groupField(raw, 'username') + ' · ' + this.g(this.groupJoinLabel(raw))).fontColor(this.foreground())
            if (this.groupField(raw, 'state') === 'pending') {
              Button(this.g('approve')).height(48).enabled(!this.groupBusy).onClick(() => this.stageGroup('decide', undefined, JSON.stringify({ id: this.groupField(raw, 'id'), approve: true }), this.g('approve')))
              Button(this.g('reject')).height(48).enabled(!this.groupBusy).onClick(() => this.stageGroup('decide', undefined, JSON.stringify({ id: this.groupField(raw, 'id'), approve: false }), this.g('reject')))
            }
          }.width('100%').padding(12).backgroundColor(this.bubble())
        }, (raw: string) => this.groupField(raw, 'id'))
        if (this.groupRequestsNext) { Button(this.t('loadMore')).height(48).enabled(!this.groupBusy).onClick(() => this.groupMore('join-requests')) }
      }
      Text(this.g('operations')).fontSize(20 * this.fontScale).fontColor(this.foreground())
      if (!this.groupOperations.some((row: GroupIntent) => !this.groupControlId || row.conversation === this.groupControlId)) { this.emptyWith(() => this.openGroupTools(this.groupControlId)) }
      ForEach(this.groupOperations.filter((row: GroupIntent) => !this.groupControlId || row.conversation === this.groupControlId).slice(0, this.groupShown), (row: GroupIntent) => {
        Column({ space: 8 }) {
          Text(this.g(row.kind === 'governance' ? 'title' : row.kind === 'decide' ? 'requests' : row.kind) + ' · ' + this.g(row.kind === 'join' && row.state === 'applied' ? this.groupJoinLabel(row.result) : row.state)).fontColor(this.foreground())
          Text(row.created).fontColor(this.foreground())
          Button(this.g('check')).height(48).enabled(!this.groupBusy).onClick(() => this.performGroup(row, false))
          if (row.state !== 'applied') { Button(this.g('retry')).height(48).enabled(!this.groupBusy).onClick(() => this.performGroup(row, true)) }
        }.width('100%').padding(12).backgroundColor(this.bubble())
      }, (row: GroupIntent) => row.namespace + ':' + row.id)
      if (this.groupOperations.length > this.groupShown) { Button(this.t('loadMore')).height(48).onClick(() => this.groupShown += 20) }
    }.padding(20).width('100%')
  }
  @Builder private groupView() {
    Column({ space: 12 }) {
      Button(this.g('title')).height(52).width('100%').enabled(!this.demo).onClick(() => this.openGroupTools(this.conversation?.id ?? ''))
      this.action('back', () => this.page = 'chats')
      Text(this.conversation?.title ?? '').fontSize(24 * this.fontScale).fontColor(this.foreground())
      if (this.conversation && !Rules.writable(this.conversation)) { Text(this.t('readOnly')).fontColor(this.foreground()) }
      if (this.conversation?.state === 'local_only') { Text(SyncText.text(this.language, 'localOnly')).fontColor(this.foreground()).width('100%') }
      if (this.conversation && Rules.writable(this.conversation)) {
        TextInput({ placeholder: this.t('title'), text: this.groupTitle }).onChange((value: string) => this.groupTitle = value)
        this.action('rename', () => this.run(async () => this.groupAction('rename')))
        TextInput({ placeholder: this.t('peerId'), text: this.groupNewMember }).onChange((value: string) => this.groupNewMember = value)
        ForEach(this.people, (person: User) => { if (!this.members.some((member: GroupMember) => member.id === person.id)) { Button(this.t('addMember') + ' · ' + person.displayName).height(48).backgroundColor(this.bubble()).fontColor(this.foreground()).onClick(() => { this.groupNewMember = person.id; this.run(async () => this.groupAction('addMember')); }) } }, (person: User) => person.id)
        this.action('addMember', () => this.run(async () => this.groupAction('addMember')))
        if (this.conversation.ownerId === this.userId) {
          TextInput({ placeholder: this.t('newOwner'), text: this.groupNewOwner }).onChange((value: string) => this.groupNewOwner = value)
          this.action('transfer', () => this.openGroupTools(this.conversation?.id ?? ''))
        }
      }
      if (this.members.length === 0) { this.emptyWith(() => this.run(async () => this.groupSettings())) }
      ForEach(this.members, (member: GroupMember) => {
        Column({ space: 8 }) {
          Text((member.state === 'tombstoned' ? this.t('unavailableUser') : member.displayName) + ' · ' + member.role).fontColor(this.foreground())
          Text(member.id).fontSize(12 * this.fontScale).fontColor(this.foreground()).copyOption(CopyOptions.LocalDevice)
          if (this.conversation && Rules.writable(this.conversation)) {
            if (member.id === this.userId && this.conversation.ownerId !== this.userId) { this.action('leave', () => this.confirmChange('leave', async () => this.groupAction('leave', member))) }
            if (this.conversation.ownerId === this.userId && member.id !== this.userId) { this.action('transfer', () => this.openGroupTools(this.conversation?.id ?? '')); this.action('remove', () => this.confirmChange('remove', async () => this.groupAction('remove', member))) }
          }
        }
      }, (member: GroupMember) => member.id + ':' + member.membershipGeneration)
      if (this.groupMembersNext) { Button(this.t('loadMore')).height(48).onClick(() => this.run(async () => this.loadGroupMembers(true))) }
    }.padding(20).width('100%')
  }
  @Builder private createForm() {
    Column({ space: 16 }) {
      TextInput({ placeholder: this.t('title'), text: this.newTitle }).height(52).onChange((value: string) => this.newTitle = value)
      TextInput({ placeholder: this.t('members'), text: this.memberIds }).height(52).onChange((value: string) => this.memberIds = value)
      ForEach(this.people, (person: User) => {
        Button(person.displayName + (this.memberIds.split(',').includes(person.id) ? ' ✓' : '')).height(48).backgroundColor(this.memberIds.split(',').includes(person.id) ? '#287C66' : this.bubble()).fontColor(this.memberIds.split(',').includes(person.id) ? '#FFFFFF' : this.foreground()).onClick(() => {
          const selected = this.memberIds.split(',').filter((id: string) => id.length > 0 && id !== person.id);
          if (!this.memberIds.split(',').includes(person.id)) { selected.push(person.id); }
          this.memberIds = selected.join(',');
        })
      }, (person: User) => person.id)
      Row() { Text(this.t('group')).fontColor(this.foreground()).layoutWeight(1); Toggle({ type: ToggleType.Switch, isOn: this.newGroup }).onChange((value: boolean) => this.newGroup = value) }
      this.action('create', () => this.run(async () => this.create()))
      if (!this.demo) { this.action('retryOriginal', () => this.run(async () => this.create(true))) }
    }.padding(24)
  }
  @Builder private conversationView() {
    Column({ space: 12 }) {
      Row() { this.action('back', () => { this.conversation = undefined; this.rows = []; }); Text(this.conversation?.title ?? '').fontSize(24 * this.fontScale).fontColor(this.foreground()).layoutWeight(1); this.action('refresh', () => this.run(async () => this.reload())) }.width('100%')
      if (this.conversation && !this.demo) { this.action('report', () => { this.reportType = 'conversation'; this.reportTarget = this.conversation?.id ?? ''; this.reportReason = ''; this.page = 'reports'; }) }
      if (this.conversation?.kind === 'group') { this.action('groupSettings', () => this.run(async () => this.groupSettings())) }
      if (this.conversation?.kind === 'group' && this.announcementsSupported && !this.demo) { this.action('announcement', () => this.run(async () => this.showAnnouncement())) }
      Button(this.m('title')).height(48).enabled(!this.demo&&!this.busy).onClick(()=>this.openMigration())
      this.action('searchMessages', () => this.run(async () => this.showLookup('search', true)))
      if (this.lookupFocusId.length > 0) { this.action('showConversation', () => this.lookupFocusId = '') }
      if (this.conversation && !Rules.writable(this.conversation)) { Text(this.t('readOnly')).fontColor(this.foreground()) }
      TextInput({ placeholder: this.t('search'), text: this.query }).onChange((value: string) => this.query = value)
      if (this.historyMore) { this.action('loadHistory', () => this.run(async () => this.loadRemainingHistory())) }
      if (this.rows.length === 0) { this.empty() }
      if (!this.demo && this.selfSupported) { this.selfButton('title', () => this.run(async () => this.openSelfState())); if (this.conversation?.state !== 'local_only') { this.selfButton('observe', () => this.run(async () => this.observeDisplayed())) } }
      ForEach(this.rows.filter((row: Message) => this.lookupFocusId.length > 0 ? row.id === this.lookupFocusId : this.query.length === 0 || row.text.includes(this.query)), (row: Message) => {
        Column({ space: 8 }) {
          Text(row.senderId.slice(0, 12)).fontSize(12 * this.fontScale).fontColor('#287C66').width('100%')
          Text((row.contentUnavailable || row.forwardUnavailable) ? this.t('contentUnavailable') : (row.deleted ? this.t('retracted') : (row.type === 'forward' && row.state === 'pending' ? this.t('forwardPending') : row.text))).fontSize(17 * this.fontScale).fontColor(this.foreground()).width('100%').copyOption(CopyOptions.LocalDevice)
          if (!(row.contentUnavailable || row.forwardUnavailable) && row.type !== 'text') {
            Text(this.t('attachment') + ' · ' + row.type).fontColor(this.foreground())
            if (!this.demo && this.conversation?.state !== 'local_only' && !row.deleted && Rules.forwardPath(row).length > 0) { this.action('openAttachment', () => this.run(async () => this.openAttachment(row))) }
          }
          if (!this.demo && !row.deleted) { this.action('report', () => { this.reportType = 'message'; this.reportTarget = row.id; this.reportReason = ''; this.page = 'reports'; }) }
          if (!row.deleted && !(row.contentUnavailable || row.forwardUnavailable) && row.reactions.length > 0) { Text(row.reactions.map((reaction: Reaction) => reaction.emoji).join(' ')).fontColor(this.foreground()) }
          if (row.mentionedMe && !row.deleted && !(row.contentUnavailable || row.forwardUnavailable)) { Text(this.t('mentionedYou')).fontColor('#287C66') }
          if (row.pinned || row.favorite) { Text((row.pinned ? this.t('pin') : '') + ' ' + (row.favorite ? this.t('favorite') : '')).fontColor('#287C66') }
          if (this.conversation?.state !== 'local_only' && !row.deleted && !(row.contentUnavailable || row.forwardUnavailable)) { Flex({ wrap: FlexWrap.Wrap }) { ForEach(this.conversation && Rules.writable(this.conversation) ? ['reply', 'react', 'pin', 'favorite'] : ['favorite'], (action: string) => { Button(this.t(action)).height(48).type(ButtonType.Normal).backgroundColor(this.surface()).fontColor('#287C66').onClick(() => this.run(async () => this.messageAction(row, action))) }); if (row.senderId === this.userId && this.conversation && Rules.writable(this.conversation)) { ForEach(row.forwarded ? ['retract'] : ['edit', 'retract'], (action: string) => { Button(this.t(action)).height(48).type(ButtonType.Normal).backgroundColor(this.surface()).fontColor('#287C66').onClick(() => this.run(async () => this.messageAction(row, action))) }) } } }
          if (!this.demo && this.conversation?.kind === 'group' && this.conversation.ownerId === this.userId && Rules.writable(this.conversation) && this.announcementsSupported && !row.deleted && !(row.contentUnavailable || row.forwardUnavailable) && row.state !== 'pending' && row.state !== 'frozen') { this.action('announcementSet', () => this.run(async () => this.stageAnnouncement(row))) }
          if (this.conversation?.state !== 'local_only' && !row.deleted && !(row.contentUnavailable || row.forwardUnavailable) && row.state !== 'pending' && row.state !== 'frozen' && (this.demo || this.forwardingSupported)) { this.action('forward', () => this.run(async () => this.selectForward(row))) }
          if (row.forwarded) { Text(this.t('forwarded')).fontColor(this.foreground()) }
          if (row.forwardUnavailable && !row.deleted && row.senderId === this.userId) { this.action('retract', () => this.run(async () => this.messageAction(row, 'retract'))) }
          if (row.state === 'pending') { this.action('retry', () => this.run(async () => this.submit(row))) }
          if (!this.demo && (row.state === 'pending' || row.state === 'frozen')) {
            if (row.state === 'frozen') { Text(this.t('admissionChanged')).fontColor(this.foreground()) }
            this.action('checkResult', () => this.run(async () => this.reconcileMessage(row)))
          }
        }.padding(16).backgroundColor(row.senderId === this.userId ? this.bubble() : this.surface()).borderRadius(14).width('100%').onVisibleAreaChange([0.5], (visible: boolean, ratio: number) => this.observeVisibility(row, visible, ratio))
      }, (row: Message) => row.id)
      if (this.editId.length > 0) { Row() { Text(this.t('edit') + ' · ' + this.editId.slice(0, 8)).fontColor(this.foreground()); this.action('cancel', () => { this.editId = ''; this.editVersion = ''; this.persistDraft(); }) } }
      if (this.replyId.length > 0) { Text(this.t('reply') + ' · ' + this.replyId.slice(0, 8)).fontColor(this.foreground()).onClick(() => { this.replyId = ''; this.persistDraft(); }) }
      if (this.conversation && Rules.writable(this.conversation)) {
      if (this.conversation.kind === 'group' && this.mentionsSupported && !this.demo && this.editId.length === 0) {
        this.action('mentions', () => this.run(async () => this.selectMentions()))
      }
      if (this.mentionAll || this.mentionedUserIds.length > 0) { Row({ space: 8 }) { Text(this.mentionAll ? this.t('mentionAll') : this.t('mentions') + ' · ' + String(this.mentionedUserIds.length)).fontColor(this.foreground()); this.action('mentionClear', () => { this.mentionedUserIds = []; this.mentionAll = false; this.persistDraft(); }) } }
      TextInput({ placeholder: this.t('message'), text: this.draft }).height(56).onChange((value: string) => this.updateDraft(value))
      Row({ space: 12 }) { this.action('attachment', () => this.run(async () => this.attach())); this.action('send', () => this.run(async () => this.send())) }
      }
    }.padding(20).width('100%')
  }
  @Builder private mentionsView() {
    Column({ space: 12 }) {
      this.action('back', () => this.page = 'chats')
      Text(this.t('mentions')).fontSize(24 * this.fontScale).fontColor(this.foreground())
      Text(this.t('mentionHelp')).fontColor(this.foreground()).width('100%')
      if ((this.conversation?.ownerId === this.userId && Rules.writable(this.conversation) && this.roleAdmissionSupported) || this.mentionAll) {
        Row({ space: 12 }) { Text(this.t('mentionAll')).fontColor(this.foreground()).layoutWeight(1); Toggle({ type: ToggleType.Switch, isOn: this.mentionAll }).onChange((value: boolean) => { if (value && !this.roleAdmissionSupported) { return; } this.mentionAll = value; if (value) { this.mentionedUserIds = []; } this.persistDraft(); }) }.width('100%')
      }
      if (this.members.filter((member: GroupMember) => member.id !== this.userId && member.state !== 'tombstoned').length === 0) { this.emptyWith(() => this.run(async () => this.selectMentions())) }
      ForEach(this.members, (member: GroupMember) => {
        if (member.id !== this.userId && member.state !== 'tombstoned') {
          Button(member.displayName + (this.mentionedUserIds.includes(member.id) ? ' · ' + this.t('selected') : '')).type(ButtonType.Normal).width('100%').constraintSize({ minHeight: 48 }).padding(12).backgroundColor(this.mentionedUserIds.includes(member.id) ? '#287C66' : this.bubble()).fontColor(this.mentionedUserIds.includes(member.id) ? '#FFFFFF' : this.foreground()).enabled(!this.busy).onClick(() => this.toggleMention(member))
        }
      }, (member: GroupMember) => member.id + ':' + member.membershipGeneration)
      this.action('mentionClear', () => { this.mentionedUserIds = []; this.mentionAll = false; this.persistDraft(); })
      this.action('save', () => {
        const conversation = this.conversation;
        if (!conversation) { return; }
        try { Rules.selectedMentions(this.mentionedUserIds, this.mentionAll, this.members, conversation, this.userId); this.persistDraft(); this.page = 'chats'; }
        catch (_) { this.info = this.t('mentionLimit'); }
      })
    }.padding(20).width('100%')
  }
  @Builder private announcementView() {
    Column({ space: 12 }) {
      this.action('back', () => this.page = 'chats')
      Text(this.t('announcement')).fontSize(24 * this.fontScale).fontColor(this.foreground())
      this.action('refresh', () => this.run(async () => this.loadAnnouncement()))
      if (!this.announcement) { this.emptyWith(() => this.run(async () => this.loadAnnouncement())) }
      if (this.announcement) {
        if (this.announcement.state === 'active' && this.announcement.message) {
          Text(this.announcement.message.text).fontSize(17 * this.fontScale).fontColor(this.foreground()).width('100%')
          this.action('showConversation', () => this.run(async () => { const message = this.announcement?.message; if (message) { await this.openLookupSource(message); } }))
        } else {
          Text(this.t(this.announcement.state === 'expired' ? 'announcementExpired' : this.announcement.state === 'unavailable' ? 'announcementUnavailable' : 'announcementEmpty')).fontColor(this.foreground()).width('100%').padding(16)
        }
        if (this.announcement.expiresAt) { Text(this.t('announcementExpiry') + ' · ' + this.announcement.expiresAt).fontColor(this.foreground()) }
        if (this.announcementPending) {
          Text(this.t('announcementPending')).fontColor(this.foreground()).width('100%')
          this.action('announcementReconcile', () => this.run(async () => this.reconcileAnnouncement()))
          if (this.announcementRejected) { this.action('discardRejected', () => this.confirmChange('discardRejected', async () => this.discardRejectedAnnouncement())) }
        } else if (this.announcement.canManage) {
          if (this.announcementCandidate) {
            Text(this.t('announcementSource')).fontColor(this.foreground()).width('100%')
            Text(this.announcementCandidate.text).fontSize(17 * this.fontScale).fontColor(this.foreground()).width('100%')
            Text(this.t('announcementExpiry')).fontColor(this.foreground()).width('100%')
            Select([{ value: this.t('expiryNever') }, { value: this.t('expiryHour') }, { value: this.t('expiryDay') }, { value: this.t('expiryWeek') }]).selected(['never', 'hour', 'day', 'week'].indexOf(this.announcementExpiry)).value(this.t(this.announcementExpiry === 'hour' ? 'expiryHour' : this.announcementExpiry === 'day' ? 'expiryDay' : this.announcementExpiry === 'week' ? 'expiryWeek' : 'expiryNever')).onSelect((index: number) => this.announcementExpiry = ['never', 'hour', 'day', 'week'][index]).width('100%').height(48)
            this.action('announcementSet', () => this.confirmAnnouncement(false))
          } else { Text(this.t('announcementHelp')).fontColor(this.foreground()).width('100%') }
          if (this.announcement.state !== 'empty') { this.action('announcementClear', () => this.confirmAnnouncement(true)) }
        }
      }
    }.padding(20).width('100%')
  }
  @Builder private forwardView() {
    Column({ space: 12 }) {
      this.action('back', () => { this.page = 'chats'; this.forwardSource = undefined; })
      Text(this.t('forward')).fontSize(22 * this.fontScale).fontColor(this.foreground())
      if (this.conversations.length === 0) { this.emptyWith(() => this.run(async () => this.reload())) }
      ForEach(this.conversations, (conversation: Conversation) => { Button(conversation.title || conversation.id).height(52).type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).enabled(Rules.writable(conversation)).onClick(() => this.run(async () => this.forwardTo(conversation))) }, (conversation: Conversation) => conversation.id)
    }.padding(20).width('100%')
  }
  @Builder private lookupView() {
    Column({ space: 12 }) {
      Row() { this.action('back', () => this.page = 'chats'); Text(this.t(this.lookupKind === 'favorites' ? 'savedMessages' : 'searchMessages')).fontSize(22 * this.fontScale).fontColor(this.foreground()) }
      if (this.lookupKind === 'search') { TextInput({ placeholder: this.t('searchMessages'), text: this.lookupQuery }).onChange((value: string) => this.lookupQuery = value); this.action('search', () => this.run(async () => this.loadLookup())) }
      this.action('refresh', () => this.run(async () => this.loadLookup()))
      if (this.lookupRows.length === 0) { this.emptyWith(() => this.run(async () => this.loadLookup())) }
      ForEach(this.lookupRows, (message: Message) => { Button() { Column({ space: 6 }) { Text((message.contentUnavailable || message.forwardUnavailable) ? this.t('contentUnavailable') : message.deleted ? this.t('retracted') : message.text).fontColor(this.foreground()).fontSize(17 * this.fontScale); Text(message.createdAt).fontColor(this.foreground()).fontSize(12 * this.fontScale) }.width('100%').padding(12) }.type(ButtonType.Normal).backgroundColor(this.bubble()).onClick(() => this.run(async () => this.openLookupSource(message))) }, (message: Message) => message.id)
      if (this.lookupMore) { this.action('loadMore', () => this.run(async () => this.loadLookup(true))) }
    }.padding(20).width('100%')
  }
  @Builder private selfButton(key: string, action: () => void) {
    Button(this.s(key)).height(48).fontSize(16 * this.fontScale).type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).onClick(action)
  }
  @Builder private selfFontControls() {
    Text(this.s('font')).fontColor(this.foreground()).fontSize(20 * this.fontScale)
    Flex({ wrap: FlexWrap.Wrap }) {
      ForEach(['small', 'standard', 'large'], (preset: string) => {
        Button(this.s(preset)).height(48).fontSize(16 * this.fontScale).type(ButtonType.Normal).backgroundColor(this.cloudFont === preset ? '#287C66' : this.bubble()).fontColor(this.cloudFont === preset ? '#FFFFFF' : this.foreground()).onClick(() => this.run(async () => this.finishSelf(await this.ensureSelf().font(preset))))
      })
    }
  }
  @Builder private selfPage() {
    Column({ space: 14 }) {
      this.action('back', () => this.page = 'chats')
      Text(this.s('title')).fontColor(this.foreground()).fontSize(24 * this.fontScale)
      this.action('refresh', () => this.run(async () => { await this.reload(); await this.openSelfState(); }))
      this.selfFontControls()
      Text(this.s('help')).fontColor(this.foreground()).fontSize(14 * this.fontScale)
      if (this.conversation) {
        Text(this.conversation.title).fontColor(this.foreground()).fontSize(20 * this.fontScale)
        if (this.selfView) {
          Text(this.s('views')).fontColor(this.foreground())
          Flex({ wrap: FlexWrap.Wrap }) {
            this.selfButton('pin', () => this.run(async () => this.changeSelfView('pinnedRank')))
            this.selfButton('hidden', () => this.run(async () => this.changeSelfView('hidden')))
            this.selfButton('archived', () => this.run(async () => this.changeSelfView('archived')))
            this.selfButton('manual', () => this.run(async () => this.changeSelfView('manualUnread')))
            this.selfButton(this.selfView.mutedUntil && Date.parse(this.selfView.mutedUntil) > Date.now() ? 'unmute' : 'mute', () => this.run(async () => this.changeSelfView('mutedUntil')))
          }
          Text(this.s('hidden') + ' ' + (this.selfView.hidden ? '●' : '○') + ' · ' + this.s('archived') + ' ' + (this.selfView.archived ? '●' : '○') + ' · ' + this.s('manual') + ' ' + (this.selfView.manualUnread ? '●' : '○')).fontColor(this.foreground())
        }
        if (this.selfRead) { Text(this.s('read') + ' · ' + this.selfRead.unreadCount + ' · ' + this.selfRead.readPoint.epoch + ':' + this.selfRead.readPoint.sequence).fontColor(this.foreground()) }
        if (this.selfDraft) {
          Text(this.s('draft')).fontSize(20 * this.fontScale).fontColor(this.foreground())
          if (!this.selfDraft.writable) { Text(this.s('readonly')).fontColor(this.foreground()) }
          Text(this.selfDraft.contentState === 'unavailable' ? this.t('contentUnavailable') : (this.selfDraft.ownedText ?? '')).fontColor(this.foreground()).copyOption(CopyOptions.LocalDevice)
          if (this.selfDraft.reply) { Text(this.t('reply') + ' · ' + this.selfDraft.reply.messageId.slice(0, 8)).fontColor(this.foreground()) }
          if (this.selfDraft.replyState === 'unavailable') { Text(this.t('contentUnavailable')).fontColor(this.foreground()) }
          TextArea({ placeholder: this.t('message'), text: this.draft }).height(112).onChange((value: string) => this.updateDraft(value))
          if (this.selfDraft.writable) { this.selfButton('save', () => this.run(async () => this.saveCloudDraft(false))); if (this.replyId && this.cloudReplyVersion) { this.selfButton('saveReply', () => this.run(async () => this.saveCloudDraft(true))) } }
          this.selfButton('clear', () => this.confirmCloudClear())
          if (this.selfDraft.contentState !== 'unavailable') { this.selfButton('adopt', () => this.run(async () => this.adoptCloudDraft(false))) }
          if (this.selfDraft.recovery) { Text(this.selfDraft.recovery.ownedText).fontColor(this.foreground()); this.selfButton('recovery', () => this.run(async () => this.adoptCloudDraft(true))) }
        } else { this.emptyWith(() => this.run(async () => this.openSelfState())) }
      } else {
        Text(this.s('draft')).fontColor(this.foreground())
        if (!this.selfDraftEntries.length) { this.emptyWith(() => this.run(async () => { await this.reload(); await this.openSelfState(); })) }
        ForEach(this.selfDraftEntries, (item: SelfDraftItem) => { Button(item.title).height(48).width('100%').type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).onClick(() => this.run(async () => this.openOwnDraft(item))) }, (item: SelfDraftItem) => item.conversation)
      }
      Text(this.s('operations')).fontColor(this.foreground()).fontSize(20 * this.fontScale)
      if (!this.selfOperations.length) { this.emptyWith(() => this.run(async () => this.openSelfState())) }
      ForEach(this.selfOperations, (item: SelfOperationItem) => {
        Column({ space: 8 }) { Text(this.s(item.kind === 'font' ? 'font' : item.kind === 'view' ? 'views' : item.kind === 'read' ? 'read' : 'draft') + ' · ' + SelfText.state(this.language, item.state) + ' · ' + item.id.slice(0, 8)).fontColor(this.foreground()); if (item.reason) { Text(item.reason).fontColor(this.foreground()) } if (item.state === 'pending') { this.selfButton('continue', () => this.run(async () => this.finishSelf(await this.ensureSelf().perform(item.id, true)))) } }.width('100%').padding(12)
      }, (item: SelfOperationItem) => item.id)
    }.padding(20).width('100%')
  }
  build() {
    Column() {
      Row() { Text('JobsIM').fontSize(25).fontWeight(FontWeight.Bold).fontColor(this.foreground()).layoutWeight(1); Button(this.t('connect')).height(48).type(ButtonType.Normal).backgroundColor(this.bubble()).fontColor(this.foreground()).onClick(() => this.page = 'connect') }.padding({ left: 20, right: 20, top: 12, bottom: 12 }).width('100%')
      if (this.busy) { LoadingProgress().width(28).height(28).color('#287C66') }
      if (this.demo || this.offline) { Text(this.t(this.demo ? 'demoNote' : 'offline')).fontSize(13 * this.fontScale).fontColor(this.foreground()).padding(12).width('100%').backgroundColor(this.bubble()) }
      if (this.info.length > 0) { Text(this.info).fontSize(13 * this.fontScale).fontColor(this.foreground()).padding(8).width('100%') }
      Scroll() { Column() {
        if (this.page === 'connect') { this.connectionForm() }
        else if (this.page === 'settings') { this.settings() }
        else if (this.page === 'exports') { this.exportsPage() }
        else if (this.page === 'password') { this.passwordPage() }
        else if (this.page === 'deletion') { this.deletionPage() }
        else if (this.page === 'groupTools') { this.groupToolsPage() }
        else if (this.page === 'migration') { this.migrationPage() }
        else if(this.page==='receipts'){this.receiptPage()}
        else if (this.userId.length === 0) { this.authentication() }
        else if (this.page === 'sessions') { this.sessionsView() }
        else if (this.page === 'reports') { this.reportsView() }
        else if (this.page === 'group') { this.groupView() }
        else if (this.page === 'lookup') { this.lookupView() }
        else if (this.page === 'forward') { this.forwardView() }
        else if (this.page === 'mentions') { this.mentionsView() }
        else if (this.page === 'announcement') { this.announcementView() }
        else if (this.page === 'self') { this.selfPage() }
        else if (this.conversation) { this.conversationView() }
        else if (this.page === 'contacts') { this.contacts() }
        else if (this.page === 'new') { this.createForm() }
        else { this.chats() }
      }.width('100%') }.layoutWeight(1).width('100%')
      Row() { ForEach(['chats', 'contacts', 'settings'], (page: string) => { Button(this.t(page)).layoutWeight(1).height(56).type(ButtonType.Normal).backgroundColor(this.page === page ? this.bubble() : this.surface()).fontColor(this.foreground()).onClick(() => { this.page = page; this.conversation = undefined; }) }) }.width('100%')
    }.height('100%').width('100%').backgroundColor(this.surface()).padding({ top: 24, bottom: 16 })
  }
}

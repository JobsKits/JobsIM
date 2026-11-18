// Created by Jobs on 2026年11月16日，星期一.
import relationalStore from '@ohos.data.relationalStore';
import { LocalStore } from './Store';
import { SyncStore } from './SyncStore';
import { ApiFailure, Bootstrap } from './Models';
import { SyncIdentity, SyncCodec, SyncControl, SyncJSON, SyncRules } from './SyncProtocol';
import { SelfRules, SelfOperation, SelfResult, SelfPreferences, SelfView, SelfRead, SelfDraft, SelfReply } from './SelfProtocol';

export interface SelfPorts {
  store: LocalStore; sync: SyncStore; identity: SyncIdentity; codec: SyncCodec;
  uuid: () => string; current: () => boolean;
  request: (route: string, method: string, body: string) => Promise<string>;
  resync: () => Promise<void>;
}
export class SelfState {
  private ports: SelfPorts;
  constructor(ports: SelfPorts) { this.ports = ports; }
  private check(): void { if (!this.ports.current()) { throw new Error('SELF_CONTEXT_CHANGED'); } }
  private async hash(operation: SelfOperation): Promise<string> { return this.ports.codec.sha256(this.ports.codec.bytes(JSON.stringify([this.ports.identity.namespace, operation.kind, operation.conversation, operation.epoch, operation.method, operation.route, operation.body]))); }
  private validate(operation: SelfOperation): void {
    let route = '/conversations/' + encodeURIComponent(operation.conversation) + '/'; let method = '';
    if (operation.kind === 'font') { route = '/me/preferences'; method = 'PATCH'; if (operation.conversation) { throw new Error('SELF_ORIGINAL_INVALID'); } }
    else {
      if (!SyncRules.uuid(operation.conversation)) { throw new Error('SELF_ORIGINAL_INVALID'); }
      if (operation.kind === 'view') { route += 'view'; method = 'PATCH'; }
      else if (operation.kind === 'read') { route += 'read'; method = 'PUT'; }
      else if (operation.kind === 'draft' && ['PUT', 'DELETE'].includes(operation.method)) { route += 'draft'; method = operation.method; }
      else { throw new Error('SELF_ORIGINAL_INVALID'); }
    }
    const body = SyncJSON.parse(operation.body);
    if (!SyncRules.uuid(operation.id) || operation.route !== route || operation.method !== method || body.get('requestId').string() !== operation.id || body.get('revisionEpoch').string() !== operation.epoch) { throw new Error('SELF_ORIGINAL_INVALID'); }
  }
  private async transaction(action: (db: relationalStore.RdbStore) => Promise<void>): Promise<void> {
    await this.ports.store.syncAccess(async (db: relationalStore.RdbStore) => {
      db.beginTransaction(); try { await action(db); db.commit(); } catch (error) { db.rollBack(); throw error as Error; }
    });
  }
  async open(): Promise<void> { await this.transaction(async (db: relationalStore.RdbStore) => { await db.executeSql('CREATE TABLE IF NOT EXISTS self_operations(namespace TEXT,id TEXT,kind TEXT,conversation TEXT,state TEXT,payload TEXT,PRIMARY KEY(namespace,id))'); }); }
  private async one(db: relationalStore.RdbStore, sql: string, values: relationalStore.ValueType[]): Promise<string> {
    const result = await db.querySql(sql, values); try { return result.goToNextRow() ? result.getString(0) : ''; } finally { result.close(); }
  }
  private async guard(db: relationalStore.RdbStore, expected: string): Promise<void> {
    this.check();
    const raw = await this.one(db, "SELECT value FROM local_state WHERE namespace=? AND key='sync:control'", [this.ports.identity.namespace]);
    const control = JSON.parse(raw) as SyncControl;
    if (control.schema !== '2' || control.guard !== expected || control.restriction || !SyncRules.same(JSON.stringify(control.identity), JSON.stringify(this.ports.identity))) { throw new Error('SELF_LOCAL_GUARD_CHANGED'); }
  }
  private async require(draft: boolean): Promise<string> {
    this.check(); const raw = await this.ports.request('/bootstrap', 'GET', ''); this.check();
    const boot = JSON.parse(raw) as Bootstrap;
    if (boot.deploymentId !== this.ports.identity.deployment || boot.restoreEpoch !== this.ports.identity.epoch) { throw new Error('RESTORE_EPOCH_CHANGED'); }
    if (boot.features?.sync_self_state !== true || boot.features?.authoritative_sync !== true || boot.features?.sync_jetstream !== true || draft && boot.features?.cloud_drafts !== true) { throw new Error('SELF_CAPABILITY_REQUIRED'); }
    const control = await this.ports.sync.syncControl(this.ports.identity);
    if (!control || control.schema !== '2') { throw new Error('SELF_SCHEMA_REQUIRED'); }
    return this.ports.sync.ready(this.ports.identity);
  }
  private async request(route: string, method: string = 'GET', body: string = ''): Promise<string> {
    this.check(); const guard = await this.ports.sync.ready(this.ports.identity);
    const raw = await this.ports.request(route, method, body); this.check();
    if (await this.ports.sync.ready(this.ports.identity) !== guard) { throw new Error('SELF_LOCAL_GUARD_CHANGED'); } return raw;
  }
  async preferences(): Promise<SelfPreferences> { await this.require(false); return SelfRules.preference(await this.request('/me/preferences'), this.ports.identity); }
  async view(conversation: string): Promise<SelfView> { await this.require(false); return SelfRules.view(await this.request('/conversations/' + encodeURIComponent(conversation) + '/view'), this.ports.identity, conversation); }
  async read(conversation: string): Promise<SelfRead> { await this.require(false); return SelfRules.read(await this.request('/conversations/' + encodeURIComponent(conversation) + '/read'), this.ports.identity, conversation, false); }
  async draft(conversation: string): Promise<SelfDraft> { await this.require(true); return SelfRules.draft(await this.request('/conversations/' + encodeURIComponent(conversation) + '/draft'), this.ports.identity, conversation); }
  async operations(): Promise<SelfOperation[]> {
    return this.ports.store.syncAccess(async (db: relationalStore.RdbStore) => {
      const result = await db.querySql('SELECT payload FROM self_operations WHERE namespace=? ORDER BY id', [this.ports.identity.namespace]); const rows: SelfOperation[] = [];
      try { while (result.goToNextRow()) { const operation = JSON.parse(result.getString(0)) as SelfOperation; this.validate(operation); if (await this.hash(operation) !== operation.hash) { throw new Error('SELF_ORIGINAL_BODY_CHANGED'); } rows.push(operation); } } finally { result.close(); } return rows;
    });
  }
  private async save(operation: SelfOperation, guard: string): Promise<void> {
    this.validate(operation); operation.hash = await this.hash(operation);
    await this.transaction(async (db: relationalStore.RdbStore) => {
      await this.guard(db, guard);
      const existing = await this.one(db, "SELECT id FROM self_operations WHERE namespace=? AND kind=? AND conversation=? AND state='pending'", [this.ports.identity.namespace, operation.kind, operation.conversation]);
      if (existing) { throw new Error('SELF_OPERATION_PENDING'); }
      await db.executeSql('INSERT INTO self_operations VALUES(?,?,?,?,?,?)', [this.ports.identity.namespace, operation.id, operation.kind, operation.conversation, operation.state, JSON.stringify(operation)]);
    });
  }
  private async update(original: SelfOperation, next: SelfOperation, guard: string): Promise<void> {
    await this.transaction(async (db: relationalStore.RdbStore) => {
      await this.guard(db, guard); const raw = await this.one(db, 'SELECT payload FROM self_operations WHERE namespace=? AND id=?', [this.ports.identity.namespace, original.id]);
      if (!SyncRules.same(raw, JSON.stringify(original)) || original.hash !== next.hash || next.hash !== await this.hash(next)) { throw new Error('SELF_ORIGINAL_BODY_CHANGED'); }
      await db.executeSql('UPDATE self_operations SET state=?,payload=? WHERE namespace=? AND id=?', [next.state, JSON.stringify(next), this.ports.identity.namespace, next.id]); this.check();
    });
  }
  private async prepare(kind: string, conversation: string, method: string, route: string, fields: Record<string, Object | null>): Promise<SelfResult> {
    const guard = await this.require(kind === 'draft'); const id = this.ports.uuid();
    fields['requestId'] = id; fields['revisionEpoch'] = this.ports.identity.epoch;
    const operation: SelfOperation = { id: id, kind: kind, conversation: conversation, epoch: this.ports.identity.epoch, method: method, route: route, body: JSON.stringify(fields), hash: '', state: 'pending', result: '' };
    await this.save(operation, guard); return this.perform(id, true);
  }
  async perform(id: string, explicit: boolean): Promise<SelfResult> {
    const operation = (await this.operations()).find((row: SelfOperation) => row.id === id);
    if (!operation || operation.epoch !== this.ports.identity.epoch) { throw new Error('SELF_ORIGINAL_INVALID'); }
    if (!['pending', 'completed'].includes(operation.state)) { throw new Error('SELF_OPERATION_FROZEN'); }
    const guard = await this.require(operation.kind === 'draft');
    const route = operation.kind === 'font' ? '/me/preferences-results' : operation.route + '-results';
    const lookup = operation.kind === 'draft' ? JSON.stringify({ method: operation.method, body: JSON.parse(operation.body) as Object }) : operation.body;
    let raw = ''; let missing = false;
    try { raw = await this.request(route, 'POST', lookup); }
    catch (error) { if (error instanceof ApiFailure && error.status === 404) { missing = true; } else { throw error as Error; } }
    let state = missing ? '' : SelfRules.operationResult(raw, operation);
    if (operation.state === 'pending' && explicit && (missing || state === 'pending')) {
      try { raw = await this.request(operation.route, operation.method, operation.body); state = SelfRules.operationResult(raw, operation); }
      catch (error) {
        if (error instanceof ApiFailure && error.status >= 400 && error.status < 500 && error.status !== 429) { const refused = JSON.parse(JSON.stringify(operation)) as SelfOperation; refused.state = 'frozen'; refused.reason = error.code; await this.update(operation, refused, guard); }
        throw error as Error;
      }
    } else if (missing) { throw new Error('SELF_RESULT_NOT_FOUND'); }
    if (operation.state === 'completed' && state !== 'completed') { throw new Error('SELF_RESULT_REGRESSION'); }
    if (operation.kind === 'view') { SelfRules.view(SyncJSON.parse(raw).get('result').raw(), this.ports.identity, operation.conversation); }
    const next = JSON.parse(JSON.stringify(operation)) as SelfOperation; next.result = raw; next.state = state;
    await this.update(operation, next, guard);
    const result = SyncJSON.parse(raw); const fact: SelfResult = { requestId: operation.id, operationId: result.get('operationId').string(), status: result.get('status').string(), current: result.get('current').bool(), version: result.get('result').get('version').string() };
    await this.ports.resync(); this.check(); return fact;
  }
  async font(preset: string): Promise<SelfResult> {
    if (!['small', 'standard', 'large'].includes(preset)) { throw new Error('SELF_INVALID_FONT'); }
    const current = await this.preferences(); const body = JSON.parse('{}') as Record<string, Object | null>; body['fontPreset'] = preset; body['expectedVersion'] = current.version;
    return this.prepare('font', '', 'PATCH', '/me/preferences', body);
  }
  async changeView(conversation: string, changes: Record<string, Object | null>, expectedVersion: string = '', manualGeneration: string = ''): Promise<SelfResult> {
    const current = await this.view(conversation); const copy = JSON.parse(JSON.stringify(changes)) as Record<string, Object | null>;
    if (expectedVersion && current.version !== expectedVersion || Object.keys(copy).includes('manualUnread') && manualGeneration && current.manualUnreadGeneration !== manualGeneration) { throw new Error('SELF_VIEW_REVIEW_REQUIRED'); }
    if (Object.keys(copy).some((key: string) => !['pinnedRank', 'mutedUntil', 'archived', 'hidden', 'manualUnread', 'manualUnreadFromMessageId'].includes(key))) { throw new Error('SELF_INVALID_VIEW'); }
    for (const key of ['archived', 'hidden', 'manualUnread']) { if (Object.keys(copy).includes(key) && typeof copy[key] !== 'boolean') { throw new Error('SELF_INVALID_VIEW'); } }
    if (Object.keys(copy).includes('manualUnread')) { copy['expectedManualUnreadGeneration'] = current.manualUnreadGeneration; }
    const body = JSON.parse('{}') as Record<string, Object | null>; body['expectedVersion'] = current.version; body['changes'] = copy;
    return this.prepare('view', conversation, 'PATCH', '/conversations/' + encodeURIComponent(conversation) + '/view', body);
  }
  async observe(conversation: string, ids: string[], originalMembership: string): Promise<SelfResult> {
    if (SyncRules.counter(originalMembership) === '0' || ids.length > 200 || new Set<string>(ids).size !== ids.length || ids.some((id: string) => !SyncRules.uuid(id))) { throw new Error('SELF_INVALID_OBSERVATION'); }
    const current = SyncJSON.parse(await this.request('/conversations/' + encodeURIComponent(conversation)));
    if (current.get('id').string() !== conversation || current.get('membershipGeneration').string() !== originalMembership) { throw new Error('QUALIFICATION_CHANGED'); }
    const body = JSON.parse('{}') as Record<string, Object | null>; body['membershipGeneration'] = originalMembership; body['observedMessageIds'] = ids.slice();
    return this.prepare('read', conversation, 'PUT', '/conversations/' + encodeURIComponent(conversation) + '/read', body);
  }
  async saveDraft(conversation: string, text: string, sourceId: string = '', sourceVersion: string = '', expectedVersion: string = ''): Promise<SelfResult> {
    if (SelfRules.bytes(text) > 16384) { throw new Error('SELF_DRAFT_CAPACITY'); }
    const current = await this.draft(conversation);
    if (expectedVersion && current.version !== expectedVersion) { throw new Error('SELF_DRAFT_REVIEW_REQUIRED'); }
    if (!current.writable || !current.draftAdmission) { throw new Error('SELF_DRAFT_READ_ONLY'); }
    let reply: SelfReply | null = null;
    if (sourceId) {
      const raw = await this.request('/messages/' + encodeURIComponent(sourceId)); const message = SyncRules.message(raw); const value = SyncJSON.parse(raw);
      if (message.id !== sourceId || message.conversationId !== conversation || message.contentVersion !== sourceVersion || message.deleted || message.contentUnavailable || message.forwardUnavailable || !value.get('replyAdmission').string()) { throw new Error('SELF_REPLY_RESELECT_REQUIRED'); }
      reply = { messageId: sourceId, sourceVersion: sourceVersion, replyAdmission: value.get('replyAdmission').string() };
    }
    const body = JSON.parse('{}') as Record<string, Object | null>; body['expectedVersion'] = current.version; body['draftAdmission'] = current.draftAdmission; body['ownedText'] = text; body['reply'] = reply;
    return this.prepare('draft', conversation, 'PUT', '/conversations/' + encodeURIComponent(conversation) + '/draft', body);
  }
  async clearDraft(conversation: string, expectedVersion: string = ''): Promise<SelfResult> {
    const current = await this.draft(conversation); if (!current.draftAdmission) { throw new Error('SELF_DRAFT_UNAVAILABLE'); }
    if (expectedVersion && current.version !== expectedVersion) { throw new Error('SELF_DRAFT_REVIEW_REQUIRED'); }
    const body = JSON.parse('{}') as Record<string, Object | null>; body['expectedVersion'] = current.version; body['draftAdmission'] = current.draftAdmission;
    return this.prepare('draft', conversation, 'DELETE', '/conversations/' + encodeURIComponent(conversation) + '/draft', body);
  }
  async adopt(conversation: string, expectedLocal: string, replacement: string, current: () => boolean): Promise<void> {
    const guard = await this.require(true);
    await this.transaction(async (db: relationalStore.RdbStore) => {
      await this.guard(db, guard); if (!current()) { throw new Error('SELF_LOCAL_INPUT_CHANGED'); }
      const raw = await this.one(db, 'SELECT value FROM local_state WHERE namespace=? AND key=?', [this.ports.identity.namespace, 'draft:' + conversation]);
      if (raw !== expectedLocal) { throw new Error('SELF_LOCAL_INPUT_CHANGED'); }
      const draft = JSON.parse(raw || '{}') as Record<string, Object | null>; draft['text'] = replacement; draft['replyId'] = ''; draft['editId'] = ''; draft['editVersion'] = ''; draft['mentionedUserIds'] = []; draft['mentionAll'] = false;
      await db.executeSql('INSERT OR REPLACE INTO local_state VALUES(?,?,?)', [this.ports.identity.namespace, 'draft:' + conversation, JSON.stringify(draft)]);
      if (!current()) { throw new Error('SELF_LOCAL_INPUT_CHANGED'); }
    });
  }
}

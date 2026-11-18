// Created by Jobs on 2026年11月17日，星期二.
import { ApiFailure, GroupMember } from './Models';
import { SyncJSON, SyncRules } from './SyncProtocol';
import { SelfRules } from './SelfProtocol';

export interface GroupEnvironment { profile: string; deployment: string; user: string; epoch: string; namespace: string; family: string; }
export interface GroupIntent { id: string; conversation: string; namespace: string; epoch: string; kind: string; method: string; path: string; lookup: string; body: string; created: string; attempts: number; state: string; result: string; family?: string; }
export interface GroupInvitation { deploymentId: string; conversationId: string; inviteId: string; token: string; }
export interface GroupPage { items: string[]; next: string; }
export interface GroupCodec { encode: (text: string) => string; decode: (text: string) => string; }
export interface GroupRepository { list: () => Promise<GroupIntent[]>; insert: (row: GroupIntent) => Promise<void>; replace: (before: GroupIntent, after: GroupIntent) => Promise<void>; }
export interface GroupPorts { environment: GroupEnvironment; current: () => boolean; uuid: () => string; repository: GroupRepository; codec: GroupCodec; request: (route: string, method: string, body: string) => Promise<string>; }
export class GroupRules {
  static fail(code: string = 'GROUP_INVALID'): never { throw new Error(code); }
  static invitation(text: string, deployment: string, codec: GroupCodec): GroupInvitation {
    if (!text.startsWith('JobsIM-Invite-1:') || text.length > 4096) { GroupRules.fail(); }
    const encoded = text.slice('JobsIM-Invite-1:'.length); const raw = codec.decode(encoded); if (codec.encode(raw) !== encoded) { GroupRules.fail(); }
    const value = SyncJSON.parse(raw); const id = value.get('inviteId').string(); const token = value.get('token').string();
    if (value.keys().sort().join(',') !== 'conversationId,deploymentId,inviteId,token' || value.get('deploymentId').string() !== deployment ||
      !SyncRules.uuid(value.get('conversationId').string()) || !SyncRules.uuid(id) || !new RegExp('^' + id + '\\.[A-Za-z0-9_-]{43}$').test(token)) { GroupRules.fail(); }
    return { deploymentId: deployment, conversationId: value.get('conversationId').string(), inviteId: id, token: token };
  }
  static validate(row: GroupIntent): void {
    const keys = Object.keys(row).sort().join(',');
    if (!['attempts,body,conversation,created,epoch,id,kind,lookup,method,namespace,path,result,state','attempts,body,conversation,created,epoch,family,id,kind,lookup,method,namespace,path,result,state'].includes(keys) || row.family !== undefined && !SyncRules.uuid(row.family) ||
      !SyncRules.uuid(row.id) || !SyncRules.uuid(row.conversation) || !row.namespace || SyncRules.counter(row.epoch) === '0' || !Number.isSafeInteger(row.attempts) || row.attempts < 0 ||
      !['governance', 'invite', 'join', 'revoke', 'decide'].includes(row.kind) || !['POST', 'DELETE'].includes(row.method) || !['unsent', 'unknown', 'applied'].includes(row.state)) { GroupRules.fail(); }
    const base = '/conversations/' + row.conversation + '/';
    let method = 'POST'; let route = base + 'governance'; let lookup = route + '/operations/' + row.id;
    if (row.kind === 'invite' || row.kind === 'join') { route = base + (row.kind === 'invite' ? 'invites' : 'join-requests'); lookup = route + '/operations/' + row.id; }
    if (row.kind === 'revoke' || row.kind === 'decide') { const prefix = base + (row.kind === 'revoke' ? 'invites/' : 'join-requests/'); if (!row.path.startsWith(prefix) || !SyncRules.uuid(row.path.slice(prefix.length))) { GroupRules.fail(); } route = row.path; if (row.kind === 'revoke') { method = 'DELETE'; } }
    if (row.method !== method || row.path !== route || row.lookup !== lookup) { GroupRules.fail(); }
    const value = SyncJSON.parse(row.body);
    const allowed = ['requestId', 'version', 'membershipGeneration', 'roleGeneration', 'action', 'targetId', 'targetMembershipGeneration', 'targetRoleGeneration', 'role', 'mutedUntil', 'muteAll', 'slowSeconds', 'maxMembers', 'expiresAt', 'maxUses', 'approvalRequired', 'inviteId', 'approve'];
    if (value.get('requestId').string() !== row.id || value.keys().some((key: string) => !allowed.includes(key))) { GroupRules.fail(); }
    SelfRules.timestamp(row.created); if (row.result && SyncJSON.parse(row.result).has('token')) { GroupRules.fail(); }
  }
  static manager(snapshot: string): boolean { const value = SyncJSON.parse(snapshot); return ['owner', 'admin'].includes(value.get('role').string()) && !value.get('readOnly').bool(); }
}
export class GroupTools {
  private ports: GroupPorts; private snapshot: string = ''; private conversation: string = ''; private tokens: Map<string, string> = new Map();
  constructor(ports: GroupPorts) { this.ports = ports; }
  private check(): void { if (!this.ports.current()) { GroupRules.fail('GROUP_CONTEXT_CHANGED'); } }
  clear(): void { this.tokens.clear(); this.snapshot = ''; }
  async list(): Promise<GroupIntent[]> {
    const rows = await this.ports.repository.list(); this.check(); const e = this.ports.environment; const prefix = 'live|' + e.profile + '|' + e.deployment + '|' + e.user + '|';
    rows.forEach((row: GroupIntent) => { GroupRules.validate(row); if (!row.namespace.startsWith(prefix)) { GroupRules.fail(); } }); return rows;
  }
  async load(conversation: string): Promise<string> {
    if (!SyncRules.uuid(conversation)) { GroupRules.fail(); } this.check();
    const raw = await this.ports.request('/conversations/' + conversation + '/governance', 'GET', ''); this.check(); const value = SyncJSON.parse(raw);
    for (const key of ['version', 'membershipGeneration', 'roleGeneration']) { if (SyncRules.counter(value.get(key).string()) === '0') { GroupRules.fail(); } }
    value.get('readOnly').bool(); value.get('role').string(); this.conversation = conversation; this.snapshot = raw; if (!GroupRules.manager(raw)) { this.tokens.clear(); } return raw;
  }
  async page(conversation: string, kind: string, after: string = ''): Promise<GroupPage> {
    if (!SyncRules.uuid(conversation) || !['members', 'invites', 'join-requests'].includes(kind) || after && !SyncRules.uuid(after)) { GroupRules.fail(); }
    const raw = await this.ports.request('/conversations/' + conversation + '/' + kind + '?limit=50' + (after ? '&after=' + after : ''), 'GET', ''); this.check();
    const value = SyncJSON.parse(raw); const next = value.get('nextCursor').string(); const items = value.get('items').list();
    if (items.length > 50 || next && (!SyncRules.uuid(next) || next === after)) { GroupRules.fail(); }
    if (kind === 'invites' && !after) { this.tokens.clear(); }
    const output: string[] = [];
    for (const item of items) {
      const id = item.get('id').string(); if (!SyncRules.uuid(id)) { GroupRules.fail(); }
      if (kind === 'members') { if (SyncRules.counter(item.get('membershipGeneration').string()) === '0' || SyncRules.counter(item.get('roleGeneration').string()) === '0') { GroupRules.fail(); } }
      if (kind === 'invites') {
        this.tokens.set(id, item.get('token').string()); const safe: Record<string, Object | null> = {};
        for (const key of ['id', 'expiresAt', 'maxUses', 'uses', 'approvalRequired', 'revokedAt']) { if (item.has(key)) { safe[key] = item.get(key).value; } } output.push(JSON.stringify(safe));
      } else { output.push(item.raw()); }
    }
    return { items: output, next: next };
  }
  prepare(action: string, target?: GroupMember, fields: string = '{}'): GroupIntent {
    this.check(); if (!this.snapshot) { GroupRules.fail('GROUP_SNAPSHOT_REQUIRED'); }
    const value = SyncJSON.parse(this.snapshot); const id = this.ports.uuid(); const e = this.ports.environment;
    if (action === 'recover' ? value.get('canRecover').value !== true : !GroupRules.manager(this.snapshot)) { GroupRules.fail(); }
    if (['settings','role','transfer','dissolve'].includes(action) && value.get('role').string() !== 'owner' || ['role','mute','transfer','recover'].includes(action) && !target || ['role','mute','transfer'].includes(action) && target?.role === 'owner') { GroupRules.fail(); }
    const body: Record<string, Object | null> = {}; body['requestId'] = id; body['version'] = value.get('version').string(); body['membershipGeneration'] = value.get('membershipGeneration').string(); body['roleGeneration'] = value.get('roleGeneration').string();
    let kind = 'governance'; let method = 'POST'; let route = '/conversations/' + this.conversation + '/governance'; let lookup = route + '/operations/' + id;
    const extra = SyncJSON.parse(fields);
    if (action === 'invite') { kind = 'invite'; route = '/conversations/' + this.conversation + '/invites'; lookup = route + '/operations/' + id; }
    else if (action === 'revoke' || action === 'decide') {
      const targetId = extra.get('id').string(); if (!SyncRules.uuid(targetId)) { GroupRules.fail(); } kind = action;
      route = '/conversations/' + this.conversation + (action === 'revoke' ? '/invites/' : '/join-requests/') + targetId; method = action === 'revoke' ? 'DELETE' : 'POST';
    } else { if (!['settings', 'role', 'mute', 'transfer', 'dissolve', 'recover'].includes(action)) { GroupRules.fail(); } body['action'] = action; }
    for (const key of extra.keys()) { if (key !== 'id') { body[key] = extra.get(key).value; } }
    if (target) { if (!SyncRules.uuid(target.id) || SyncRules.counter(target.membershipGeneration) === '0' || SyncRules.counter(target.roleGeneration ?? '') === '0') { GroupRules.fail(); }
      body['targetId'] = target.id; body['targetMembershipGeneration'] = target.membershipGeneration; body['targetRoleGeneration'] = target.roleGeneration ?? ''; }
    const row: GroupIntent = { id: id, conversation: this.conversation, namespace: e.namespace, epoch: e.epoch, kind: kind, method: method, path: route, lookup: lookup, body: JSON.stringify(body), created: new Date().toISOString(), attempts: 0, state: 'unsent', result: '', family: e.family }; GroupRules.validate(row); return row;
  }
  joining(code: string): GroupIntent {
    const e = this.ports.environment; const invite = GroupRules.invitation(code.trim(), e.deployment, this.ports.codec); const id = this.ports.uuid(); const route = '/conversations/' + invite.conversationId + '/join-requests';
    const body: Record<string, Object> = {}; body['requestId'] = id; body['inviteId'] = invite.inviteId;
    const row: GroupIntent = { id: id, conversation: invite.conversationId, namespace: e.namespace, epoch: e.epoch, kind: 'join', method: 'POST', path: route, lookup: route + '/operations/' + id, body: JSON.stringify(body), created: new Date().toISOString(), attempts: 0, state: 'unsent', result: '', family: e.family }; GroupRules.validate(row); return row;
  }
  async perform(candidate: GroupIntent, retry: boolean, code: string = '', otp: string = ''): Promise<string | undefined> {
    this.check(); GroupRules.validate(candidate);
    const originals = await this.list(); let row = originals.find((item: GroupIntent) => item.id === candidate.id); this.check();
    if (!row && candidate.kind === 'join') { const invite = SyncJSON.parse(candidate.body).get('inviteId').string(); row = originals.find((item: GroupIntent) => item.kind === 'join' && item.conversation === candidate.conversation && SyncJSON.parse(item.body).get('inviteId').string() === invite && (item.state !== 'applied' || Boolean(item.result) && SyncJSON.parse(item.result).get('state').string() === 'pending')); }

    if (!row) { row = candidate; await this.ports.repository.insert(row); this.check(); }
    let result: SyncJSON | undefined;
    try { result = SyncJSON.parse(await this.ports.request(row.lookup, 'GET', '')); }
    catch (error) { this.check(); if (!(error instanceof ApiFailure) || error.status !== 404 || error.code !== 'GROUP_OPERATION_NOT_FOUND') { throw error as Error; } }
    this.check(); if (!result && !retry) { return undefined; }
    if (!result) {
      if (row.epoch !== this.ports.environment.epoch) { GroupRules.fail('GROUP_OLD_EPOCH_READ_ONLY'); }
      if (row.namespace !== this.ports.environment.namespace || !row.family || row.family !== this.ports.environment.family) { GroupRules.fail('GROUP_ORIGINAL_SESSION_REQUIRED'); }
      const parsed = SyncJSON.parse(row.body); const body: Record<string, Object | null> = {};
      for (const key of parsed.keys()) { body[key] = parsed.get(key).value; }
      if (row.kind === 'join') { const invite = GroupRules.invitation(code.trim(), this.ports.environment.deployment, this.ports.codec); if (invite.conversationId !== row.conversation || invite.inviteId !== parsed.get('inviteId').string()) { GroupRules.fail(); } body['token'] = invite.token; }
      if (parsed.has('action') && parsed.get('action').string() === 'recover') {
        if (!/^[0-9]{6}$/.test(otp)) { GroupRules.fail(); }
        const step: Record<string, Object | null> = {}; step['requestId'] = row.id; step['action'] = 'group.recover'; step['targetId'] = row.conversation; step['payload'] = parsed.value; step['otp'] = otp;
        const proof = SyncJSON.parse(await this.ports.request('/auth/step-up', 'POST', JSON.stringify(step))); this.check(); const authorization = proof.get('authorizationId').string(); if (!SyncRules.uuid(authorization)) { GroupRules.fail(); } body['stepUpId'] = authorization;
      }
      const before = row; row = JSON.parse(JSON.stringify(row)) as GroupIntent; row.attempts++; row.state = 'unknown'; await this.ports.repository.replace(before, row); this.check();
      result = SyncJSON.parse(await this.ports.request(row.path, row.method, JSON.stringify(body))); this.check();
    }
    if (result.get('requestId').string() !== row.id) { GroupRules.fail(); }
    for (const key of ['version','membershipGeneration']) { if (result.has(key) && result.get(key).value !== null && SyncRules.counter(result.get(key).string()) === '0') { GroupRules.fail(); } }
    if (row.kind === 'governance' && (result.get('status').string() !== 'applied' || result.get('action').string() !== SyncJSON.parse(row.body).get('action').string())) { GroupRules.fail(); }
    if (row.kind === 'revoke' || row.kind === 'decide') {
      const expected = row.kind === 'revoke' ? 'revoked' : SyncJSON.parse(row.body).get('approve').bool() ? 'approved' : 'rejected';
      if (result.get('status').string() !== 'applied' || result.get('action').string() !== (row.kind === 'revoke' ? 'invite_revoke' : 'join_decide') || result.get('targetId').string() !== row.path.slice(row.path.lastIndexOf('/')+1) || result.get('state').string() !== expected || row.kind === 'decide' && expected === 'approved' && SyncRules.counter(result.get('membershipGeneration').string()) === '0') { GroupRules.fail(); }
    }
    if (row.kind === 'join') { const state = result.get('state').string(); if (!['pending','approved','rejected'].includes(state) || !SyncRules.uuid(result.get('id').string()) || state === 'approved' && SyncRules.counter(result.get('membershipGeneration').string()) === '0') { GroupRules.fail(); } }
    if (result.has('token')) { const id = result.get('id').string(); const token = result.get('token').string(); if (row.kind !== 'invite' || !SyncRules.uuid(id) || !new RegExp('^' + id + '\\.[A-Za-z0-9_-]{43}$').test(token)) { GroupRules.fail(); } this.tokens.set(id, token); }
    const safe: Record<string, Object | null> = {};
    for (const key of ['requestId', 'status', 'action', 'version', 'id', 'expiresAt', 'maxUses', 'uses', 'approvalRequired', 'revokedAt', 'state', 'membershipGeneration', 'targetId']) { if (result.has(key)) { safe[key] = result.get(key).value; } }
    const after = JSON.parse(JSON.stringify(row)) as GroupIntent; after.state = 'applied'; after.result = JSON.stringify(safe); await this.ports.repository.replace(row, after); this.check(); return after.result;
  }
  share(id: string): string {
    const token = this.tokens.get(id); if (!token) { GroupRules.fail('GROUP_INVITE_RELOAD_REQUIRED'); }
    const value: GroupInvitation = { deploymentId: this.ports.environment.deployment, conversationId: this.conversation, inviteId: id, token: token };
    return 'JobsIM-Invite-1:' + this.ports.codec.encode(JSON.stringify(value));
  }
}

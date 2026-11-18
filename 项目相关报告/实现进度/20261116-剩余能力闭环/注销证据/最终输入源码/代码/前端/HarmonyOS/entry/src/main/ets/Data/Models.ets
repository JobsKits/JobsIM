export interface User { id: string; username: string; displayName: string; avatarURL?: string; avatarThumbnailURL?: string; profileVersion?: string; state?: string; }
export interface Conversation { id: string; kind: string; title: string; memberIds: string[]; cursor: string; ownerId?: string; state?: string; readOnly?: boolean; membershipGeneration?: string; roleGeneration?: string; sendAdmission?: string; }
export interface Message {
  id: string; conversationId: string; senderId: string; clientMsgId: string; type: string; text: string;
  replyToId?: string; attachmentId?: string; createdAt: string; version: string; contentVersion?: string; sequence: string;
  mentionedUserIds?: string[]; mentionAll?: boolean; mentionedMe?: boolean;
  forwarded?: boolean; forwardUnavailable?: boolean; attachmentURL?: string; forwardAdmission?: string;
  forwardOfId?: string; forwardSourceVersion?: string; sendAttempted?: boolean;
  deleted: boolean; contentUnavailable?: boolean; pinned: boolean; favorite: boolean; reactions: Reaction[]; state?: string; restoreEpoch?: string; sendAdmission?: string; freezeReason?: string;
}
export interface AdmissionFeatures { send_admission?: boolean; role_admission?: boolean; forwarding?: boolean; mentions?: boolean; announcements?: boolean; groups?: boolean; messages?: boolean; auth_refresh?: boolean; auth_retirement_chain?: boolean; authoritative_sync?: boolean; sync_jetstream?: boolean; sync_self_state?: boolean; cloud_drafts?: boolean; chat_export?: boolean; own_password_change?: boolean; own_account_deletion?: boolean; }
export interface Bootstrap { deploymentId: string; name: string; protocolVersion: string; restoreEpoch: string; features?: AdmissionFeatures; }
export interface AuthResult { token: string; user: User; deviceId: string; restoreEpoch: string; refreshToken?: string; familyId?: string; refreshGeneration?: string; accessExpiresAt?: string; idleExpiresAt?: string; absoluteExpiresAt?: string; sourceContextId?: string; }
export interface ConversationList { items: Conversation[]; }
export interface MessageList { items: Message[]; cursor: string; }
export interface LookupPage { items: Message[]; cursor: string; hasMore: boolean; }
export interface UserList { items: User[]; }
export interface Codes { codes?: string[]; batchId: string; }
export interface MediaResult { id: string; name: string; contentType: string; size: number; }
export class Isolation {
  static namespace(profile: string, deployment: string, user: string, demo: boolean, epoch: string = '1', device: string = '', source: string = ''): string {
    const values = [demo ? 'demo' : 'live', profile, deployment, user, epoch, device];
    if (!demo && source.length > 0) { values.push(source); }
    return values.join('|');
  }
  static endpoint(raw: string): string {
    const value: string = raw.trim().replace(/\/$/, '');
    if (!/^https:\/\/[^\s/?#@]+(?::[0-9]+)?(?:\/[^?#]*)?$/.test(value)) {
      throw new Error('HTTPS URL without credentials, query or fragment required');
    }
    return value;
  }
}

export interface ContactRequest { id: string; requesterId: string; recipientId: string; state: string; requester?: User; }
export interface ContactsResult { items: User[]; requests: ContactRequest[]; }
export interface GroupMember extends User { role: string; membershipGeneration: string; roleGeneration?: string; }
export interface MemberList { items: GroupMember[]; }
export interface ReopenResult { current: boolean; generation: string; }
export interface PendingOperation { requestId: string; payload: string; }
export class Rules {
  static lookupPath(kind: string, query: string, conversation: string, cursor: string): string {
    if (kind !== 'favorites' && kind !== 'search') { throw new Error('Invalid lookup kind'); }
    let path = '/' + kind + '?limit=50';
    if (kind === 'search') {
      if (query.trim().length === 0) { throw new Error('Search query required'); }
      path += '&q=' + encodeURIComponent(query);
      if (conversation.length > 0) { path += '&conversationId=' + encodeURIComponent(conversation); }
    }
    if (cursor.length > 0) { path += '&cursor=' + encodeURIComponent(cursor); }
    return path;
  }
  static admissionSupported(bootstrap: Bootstrap, deployment: string): boolean {
    return bootstrap.deploymentId === deployment && bootstrap.features?.send_admission === true;
  }
  static roleAdmissionSupported(bootstrap: Bootstrap, deployment: string): boolean {
    return bootstrap.deploymentId === deployment && bootstrap.features?.role_admission === true;
  }
  static permanentMentionRejection(reason: string): boolean {
    return ['SEND_ADMISSION_CHANGED', 'MENTION_ALL_FORBIDDEN', 'MENTION_FORBIDDEN', 'MENTION_MEMBER_REQUIRED', 'QUALIFICATION_CHANGED'].some((code: string) => reason.includes(code));
  }
  static outgoingBody(row: Message): string {
    if (row.type === 'forward') { return JSON.stringify({ clientMsgId: row.clientMsgId, type: 'forward', forwardOfId: row.forwardOfId, forwardSourceVersion: row.forwardSourceVersion, forwardAdmission: row.forwardAdmission, restoreEpoch: row.restoreEpoch, sendAdmission: row.sendAdmission }); }
    return JSON.stringify({ clientMsgId: row.clientMsgId, restoreEpoch: row.restoreEpoch, type: row.type, text: row.text, replyToId: row.replyToId, attachmentId: row.attachmentId, sendAdmission: row.sendAdmission, mentionedUserIds: row.mentionedUserIds, mentionAll: row.mentionAll });
  }
  static unavailableForward(row: Message): Message {
    const masked = JSON.parse(JSON.stringify(row)) as Message;
    masked.text = ''; masked.attachmentId = undefined; masked.attachmentURL = undefined; masked.forwardAdmission = undefined; masked.forwardUnavailable = true;
    return masked;
  }
  static forwardPath(row: Message): string {
    if (row.deleted || row.contentUnavailable || row.forwardUnavailable) { return ''; }
    const path = '/api/messages/' + encodeURIComponent(row.id) + '/attachment';
    return row.forwarded ? row.attachmentURL === path ? path.substring(4) : '' : row.attachmentId ? '/media/' + row.attachmentId : '';
  }
  static permanentForwardRejection(reason: string): boolean {
    return ['NOT_A_MEMBER', 'HISTORY_NOT_ALLOWED', 'INVALID_FORWARD', 'FORWARD_PROTOCOL_REQUIRED', 'FORWARD_DEPTH_LIMIT', 'FORWARD_ADMISSION_CHANGED', 'FORWARD_SOURCE_CHANGED', 'FORWARD_SOURCE_UNAVAILABLE'].some((code: string) => reason.includes(code));
  }
  static forwardIntent(source: Message, target: Conversation, account: string, epoch: string, id: string, now: string): Message {
    if (source.deleted || source.contentUnavailable || source.forwardUnavailable || !source.forwardAdmission || !/^[1-9][0-9]*$/.test(source.contentVersion ?? '') || !Rules.writable(target) || !target.sendAdmission) { throw new Error('Current forward proof required'); }
    return { id: id, clientMsgId: id, conversationId: target.id, senderId: account, type: 'forward', text: '', createdAt: now, version: '1', sequence: '0', deleted: false, pinned: false, favorite: false, reactions: [], state: 'pending', restoreEpoch: epoch, sendAdmission: target.sendAdmission, forwardOfId: source.id, forwardSourceVersion: source.contentVersion, forwardAdmission: source.forwardAdmission, forwarded: true };
  }
  static mentionUnread(previous: boolean | undefined, message: Message, account: string, active: boolean): boolean {
    if (active || message.mentionedMe !== true || message.senderId === account || message.deleted || message.contentUnavailable) { return false; }
    return previous === undefined ? true : previous;
  }
  static selectedMentions(ids: string[], all: boolean, members: GroupMember[], conversation: Conversation, account: string): string[] {
    if (conversation.kind !== 'group' || !Rules.writable(conversation)) { throw new Error('Invalid mention group'); }
    if (all && (ids.length > 0 || conversation.ownerId !== account)) { throw new Error('Invalid mention all'); }
    if (ids.length > 20 || new Set<string>(ids).size !== ids.length || ids.some((id: string) => !members.some((member: GroupMember) => member.id === id && member.state !== 'tombstoned'))) { throw new Error('Current mention members required'); }
    return ids.slice();
  }
  static sourceContentVersion(message: Message): string {
    const version = message.contentVersion ?? message.version;
    if (!/^[1-9][0-9]*$/.test(version) || message.deleted || message.contentUnavailable || message.forwardUnavailable) { throw new Error('Source unavailable'); }
    return version;
  }
  static announcementIntent(conversation: Conversation, source: Message | undefined, expectedVersion: string, requestId: string, expiresAt: string = ''): AnnouncementIntent {
    if (conversation.kind !== 'group' || !Rules.writable(conversation) || !/^[1-9][0-9]*$/.test(conversation.membershipGeneration ?? '') || !/^[1-9][0-9]*$/.test(conversation.roleGeneration ?? '') || !/^(0|[1-9][0-9]*)$/.test(expectedVersion) || requestId.length < 8) { throw new Error('Original announcement context required'); }
    if (source && source.conversationId !== conversation.id) { throw new Error('Same-group source required'); }
    const body: AnnouncementMutation = { expectedVersion: expectedVersion, membershipGeneration: conversation.membershipGeneration ?? '', requestId: requestId, roleGeneration: conversation.roleGeneration };
    if (source) {
      body.messageId = source.id; body.sourceVersion = Rules.sourceContentVersion(source);
      if (expiresAt.length > 0) { if (!Number.isFinite(Date.parse(expiresAt))) { throw new Error('Invalid expiry'); } body.expiresAt = expiresAt; }
    }
    return { conversationId: conversation.id, method: source ? 'PUT' : 'DELETE', body: JSON.stringify(body), rejected: false };
  }
  static announcementRejected(reason: string): boolean {
    return ['QUALIFICATION_CHANGED', 'ANNOUNCEMENT_VERSION_CONFLICT', 'MEMBERSHIP_GENERATION_CHANGED', 'ANNOUNCEMENT_SOURCE_CHANGED', 'ANNOUNCEMENT_FORBIDDEN', 'INVALID_EXPIRY', 'FEATURE_DISABLED'].some((code: string) => reason.includes(code));
  }
  static announcementExpiry(preset: string, now: number): string {
    const seconds = preset === 'hour' ? 3600 : preset === 'day' ? 86400 : preset === 'week' ? 604800 : 0;
    if (!['never', 'hour', 'day', 'week'].includes(preset) || !Number.isFinite(now)) { throw new Error('Invalid expiry preset'); }
    return seconds === 0 ? '' : new Date(now + seconds * 1000).toISOString();
  }
  static incoming(request: ContactRequest, account: string): boolean {
    return request.state === 'pending' && request.recipientId === account;
  }
  static writable(conversation: Conversation): boolean {
    return conversation.readOnly !== true && conversation.state !== 'owner_unavailable' && conversation.state !== 'tombstoned';
  }
  static cursorMatches(epoch: string, cursor: string): boolean {
    return /^[1-9][0-9]*$/.test(epoch) && new RegExp('^' + epoch + ':[0-9]+$').test(cursor);
  }
  static sessionMatches(pointer: SessionPointer, context: AuthContext): boolean {
    return context.user.id === pointer.account && context.deviceId === pointer.device && context.restoreEpoch === pointer.epoch;
  }
  static authBody(mode: string, username: string, password: string, code: string, requestId: string): string {
    if (mode === 'recover') { return JSON.stringify({ username: username, password: password, code: code, requestId: requestId }); }
    return JSON.stringify({ username: username, password: password, requestId: requestId, deviceName: 'JobsIM HarmonyOS', otp: mode === 'login' ? code : '' });
  }
  static removalPath(conversationId: string, member: GroupMember): string {
    if (!/^[1-9][0-9]*$/.test(member.membershipGeneration)) { throw new Error('Missing membership generation'); }
    return '/conversations/' + encodeURIComponent(conversationId) + '/members/' + encodeURIComponent(member.id) + '?membershipGeneration=' + encodeURIComponent(member.membershipGeneration);
  }
}

export interface DraftState { text: string; replyId: string; editId?: string; editVersion?: string; mentionedUserIds?: string[]; mentionAll?: boolean; }

export interface AuthContext { user: User; deviceId: string; restoreEpoch: string; }
export interface SessionPointer { alias: string; account: string; device: string; epoch: string; profile: string; deployment: string; sourceContextId?: string; familyId?: string; refreshGeneration?: string; }
export interface DeviceSession { deviceId: string; name: string; createdAt: string; expiresAt: string; }
export interface SessionList { items: DeviceSession[]; }
export interface DownloadResult { data: ArrayBuffer; contentType: string; }
export interface AvatarPending { requestId: string; path: string; expectedVersion: string; sha256: string; }
export interface AvatarOperation { user: User; operationId: string; status: string; current: boolean; }
export interface Report { id: string; reporterId: string; targetType: string; targetId: string; reason: string; state: string; version: string; createdAt: string; updatedAt: string; }
export interface ReportList { items: Report[]; includesChatBodies: boolean; }
export interface ReportDraft { targetType: string; targetId: string; reason: string; }

export interface Reaction { emoji: string; userId: string; }

export interface ConversationDraft { kind: string; title: string; memberIds: string[]; }

export interface Announcement { conversationId: string; version: string; state: string; messageId?: string; sourceVersion?: string; expiresAt?: string; updatedAt?: string; canManage: boolean; message?: Message; }
export interface AnnouncementMutation { messageId?: string; sourceVersion?: string; expiresAt?: string; expectedVersion: string; membershipGeneration: string; requestId: string; roleGeneration?: string; }
export interface AnnouncementIntent { conversationId: string; method: string; body: string; rejected?: boolean; }
export interface AnnouncementOperation { operationId: string; status: string; result: Announcement; current: boolean; }

export interface AuthConnection { base: string; caPath: string; profile: string; deployment: string; epoch: string; }
export interface CredentialPair {
  profile: string; deployment: string; userId: string; username: string; deviceId: string;
  epoch: string; sourceContextId: string; token: string; refreshToken: string; familyId: string;
  generation: string; accessExpiresAt: string; idleExpiresAt: string; absoluteExpiresAt: string; requestId: string;
}
export interface AuthBinding { pointer: SessionPointer; user: User; username: string; familyId: string; generation: string; requestId: string; state: string; }
export interface AuthRecord {
  requestId: string; purpose: string; username: string; deviceName: string; otpRequired: boolean; state: string;
  previousFamilyId?: string; previousRequestId?: string; previousRequestIds?: string[]; result?: AuthBinding;
}
export interface AuthStateEntry { key: string; value: string; }
export interface RefreshRecord { familyId: string; generation: string; requestId: string; state: string; username: string; }
export interface RefreshPayload { refreshToken: string; requestId: string; familyId: string; refreshGeneration: string; }
export interface InitialAuthPayload {
  username: string; password: string; requestId: string; deviceName: string; otp?: string;
  previousFamilyId?: string; previousRequestId?: string; previousRequestIds?: string[];
}
export interface AuthHooks {
  stamp: () => string;
  before: (stamp: string) => Promise<string>;
  refresh: (token: string, stamp: string) => Promise<string>;
  rejected: (token: string, stamp: string) => Promise<void>;
}
export class AuthRules {
  static decimal(value: string): boolean { return /^[1-9][0-9]*$/.test(value); }
  static next(value: string): string {
    if (!AuthRules.decimal(value)) { throw new Error('INVALID_AUTH_GENERATION'); }
    const digits = value.split('');
    for (let index = digits.length - 1; index >= 0; index--) {
      const digit = Number(digits[index]);
      if (digit < 9) { digits[index] = String(digit + 1); return digits.join(''); }
      digits[index] = '0';
    }
    return '1' + digits.join('');
  }
  static time(value: string): number {
    if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/.test(value)) { throw new Error('INVALID_AUTH_EXPIRY'); }
    const result = Date.parse(value.replace(/\.(\d{3})\d+Z$/, '.$1Z'));
    if (!Number.isFinite(result)) { throw new Error('INVALID_AUTH_EXPIRY'); }
    return result;
  }
  static validate(pair: CredentialPair): void {
    const uuid = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
    if (!pair.profile || !uuid.test(pair.deployment) || !uuid.test(pair.userId) || !uuid.test(pair.deviceId) ||
      !uuid.test(pair.familyId) || !/^[a-z0-9_]{3,32}$/.test(pair.username) || pair.sourceContextId !== 'personal' ||
      !AuthRules.decimal(pair.epoch) || !AuthRules.decimal(pair.generation) || !pair.token || !pair.refreshToken ||
      pair.token === pair.refreshToken || pair.requestId.length < 8 || pair.requestId.length > 128) { throw new Error('INVALID_AUTH_PAIR'); }
    if (AuthRules.time(pair.accessExpiresAt) > AuthRules.time(pair.idleExpiresAt) ||
      AuthRules.time(pair.idleExpiresAt) > AuthRules.time(pair.absoluteExpiresAt)) { throw new Error('INVALID_AUTH_PAIR'); }
  }
  static pair(result: AuthResult, connection: AuthConnection, requestId: string): CredentialPair {
    const pair: CredentialPair = { profile: connection.profile, deployment: connection.deployment,
      userId: result.user.id, username: result.user.username, deviceId: result.deviceId, epoch: result.restoreEpoch,
      sourceContextId: result.sourceContextId ?? '', token: result.token, refreshToken: result.refreshToken ?? '',
      familyId: result.familyId ?? '', generation: result.refreshGeneration ?? '', accessExpiresAt: result.accessExpiresAt ?? '',
      idleExpiresAt: result.idleExpiresAt ?? '', absoluteExpiresAt: result.absoluteExpiresAt ?? '', requestId: requestId };
    AuthRules.validate(pair);
    if (pair.epoch !== connection.epoch) { throw new Error('RESTORE_EPOCH_CHANGED'); }
    return pair;
  }
  static rotate(next: CredentialPair, previous: CredentialPair): void {
    AuthRules.validate(next);
    if (next.profile !== previous.profile || next.deployment !== previous.deployment || next.userId !== previous.userId ||
      next.username !== previous.username || next.deviceId !== previous.deviceId || next.epoch !== previous.epoch ||
      next.sourceContextId !== previous.sourceContextId || next.familyId !== previous.familyId ||
      next.generation !== AuthRules.next(previous.generation) || next.token === previous.token || next.refreshToken === previous.refreshToken ||
      AuthRules.time(next.absoluteExpiresAt) !== AuthRules.time(previous.absoluteExpiresAt)) { throw new Error('INVALID_REFRESH_PAIR'); }
  }
  static namespace(pair: CredentialPair): string {
    return Isolation.namespace(pair.profile, pair.deployment, pair.userId, false, pair.epoch, pair.deviceId, pair.sourceContextId);
  }
  static pointer(pair: CredentialPair): SessionPointer {
    return { alias: 'JobsIM|' + AuthRules.namespace(pair), profile: pair.profile, deployment: pair.deployment, account: pair.userId,
      device: pair.deviceId, epoch: pair.epoch, sourceContextId: pair.sourceContextId, familyId: pair.familyId, refreshGeneration: pair.generation };
  }
  static binding(pair: CredentialPair, user: User): AuthBinding {
    if (pair.userId !== user.id || pair.username !== user.username) { throw new Error('INVALID_AUTH_USER'); }
    return { pointer: AuthRules.pointer(pair), user: user, username: pair.username, familyId: pair.familyId,
      generation: pair.generation, requestId: pair.requestId, state: 'active' };
  }
  static matches(pair: CredentialPair, binding: AuthBinding): boolean {
    const p = binding.pointer;
    return pair.profile === p.profile && pair.deployment === p.deployment && pair.userId === p.account &&
      pair.deviceId === p.device && pair.epoch === p.epoch && pair.sourceContextId === p.sourceContextId &&
      pair.familyId === binding.familyId && pair.generation === binding.generation && pair.requestId === binding.requestId &&
      p.alias === AuthRules.pointer(pair).alias && binding.username === pair.username;
  }
  static encode(pair: CredentialPair): string {
    AuthRules.validate(pair);
    // One compact SECRET, never separate AT/RT items. The native adapter checks UTF-8 <1KiB.
    return JSON.stringify([pair.profile, pair.deployment, pair.userId, pair.username, pair.deviceId, pair.epoch,
      pair.sourceContextId, pair.token, pair.refreshToken, pair.familyId, pair.generation,
      pair.accessExpiresAt, pair.idleExpiresAt, pair.absoluteExpiresAt, pair.requestId]);
  }
  static decode(value: string): CredentialPair {
    let values: string[];
    try { values = JSON.parse(value) as string[]; } catch (_error) { throw new Error('INVALID_AUTH_PAIR'); }
    if (!Array.isArray(values) || values.length !== 15 || values.some((part: string) => typeof part !== 'string')) { throw new Error('INVALID_AUTH_PAIR'); }
    const pair: CredentialPair = { profile: values[0], deployment: values[1], userId: values[2], username: values[3], deviceId: values[4], epoch: values[5],
      sourceContextId: values[6], token: values[7], refreshToken: values[8], familyId: values[9], generation: values[10],
      accessExpiresAt: values[11], idleExpiresAt: values[12], absoluteExpiresAt: values[13], requestId: values[14] };
    AuthRules.validate(pair); return pair;
  }
  static initialBody(record: AuthRecord, password: string, otp: string): string {
    const body: InitialAuthPayload = { username: record.username, password: password, requestId: record.requestId, deviceName: record.deviceName };
    if (record.otpRequired) { body.otp = otp; }
    if (record.previousFamilyId) { body.previousFamilyId = record.previousFamilyId; }
    if (record.previousRequestId) { body.previousRequestId = record.previousRequestId; }
    if (record.previousRequestIds) { body.previousRequestIds = record.previousRequestIds.slice(); }
    return JSON.stringify(body);
  }
}
export class ApiFailure extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string) { super('HTTP ' + status + ' ' + code); this.status = status; this.code = code; }
}
export interface AuthPorts {
  current: () => boolean;
  read: (scope: string, key: string) => Promise<string>;
  write: (scope: string, values: AuthStateEntry[]) => Promise<void>;
  commit: (scope: string, oldNamespaces: string[], newNamespace: string, values: AuthStateEntry[]) => Promise<void>;
  freeze: (namespace: string, reason: string) => Promise<void>;
  load: (alias: string) => Promise<string>;
  save: (alias: string, value: string) => Promise<void>;
  remove: (alias: string) => Promise<void>;
  bootstrap: () => Promise<Bootstrap>;
  send: (path: string, body: string) => Promise<AuthResult>;
  context: (token: string) => Promise<AuthContext>;
  logout: (token: string) => Promise<void>;
  nonce: () => string;
  publish: (pair: CredentialPair | undefined, user: User | undefined) => void;
  features: (bootstrap: Bootstrap) => void;
}

// Created by Jobs on 2026年11月17日，星期二.
import { ApiFailure, Bootstrap } from './Models';
import { SyncJSON, SyncRules } from './SyncProtocol';
import { SelfRules } from './SelfProtocol';
import { PasswordIntent, PasswordOwner, PasswordRules } from './PasswordChange';

export interface DeletionIntent { meta: PasswordIntent; attempts: number; state: string; result: string; cancelId: string; cancelState: string; }
export interface DeletionResult { deletionId: string; requestId: string; status: string; requestVersion: string; restoreEpoch: string; cancelUntil: string | null; cancelRequestId: string; requiresNewLogin: boolean; }
export interface DeletionRepository { list: () => Promise<DeletionIntent[]>; insert: (row: DeletionIntent) => Promise<void>; replace: (before: DeletionIntent, after: DeletionIntent) => Promise<void>; }
export interface DeletionPorts { profile: string; origin: string; deployment: string; epoch: string; current: () => boolean; owner: () => PasswordOwner | undefined; uuid: () => string; repository: DeletionRepository; request: (route: string, body: string, token: string) => Promise<string>; bootstrap: () => Promise<Bootstrap>; stop: (row: DeletionIntent) => Promise<void>; prepare?: () => Promise<void>; }
interface DeletionBody { requestId: string; username: string; deviceId: string; restoreEpoch: string; password: string; otp?: string; }
interface DeletionProofBody { username: string; deletionId: string; requestVersion: string; purpose: string; password?: string; recoveryCode?: string; otp?: string; }
interface DeletionActionBody { proof: string; requestId?: string; }
export class DeletionRules {
  static fail(code: string = 'DELETION_INVALID'): never { throw new Error(code); }
  static password(value: string): string { value = value.normalize('NFC'); if (!value || SelfRules.bytes(value) > 512) { DeletionRules.fail(); } return value; }
  static definiteRejection(status: number, code: string): boolean { return status === 400 && code === 'INVALID_ACCOUNT_DELETION' || status === 403 && code === 'DATA_RIGHTS_REJECTED' || status === 429 && code === 'RATE_LIMITED'; }
  static blocked(row: DeletionIntent): boolean { return row.attempts > 0 && !['unsent', 'rejected'].includes(row.state); }
  static validate(row: DeletionIntent): void {
    PasswordRules.validate(row.meta);
    if (Object.keys(row).sort().join(',') !== 'attempts,cancelId,cancelState,meta,result,state' || row.meta.attempts !== 0 || row.meta.state !== 'unsent' || row.meta.result ||
      !Number.isSafeInteger(row.attempts) || row.attempts < 0 || !['unsent', 'unknown', 'pending', 'requested', 'cancel_pending', 'processing', 'completed', 'cancelled', 'retired', 'rejected'].includes(row.state) ||
      !['', 'prepared', 'unknown', 'cancel_pending', 'cancelled'].includes(row.cancelState) || row.cancelId && !SyncRules.uuid(row.cancelId)) { DeletionRules.fail(); }
    if (row.result) { const stored = SyncJSON.parse(row.result); if (stored.keys().sort().join(',') !== 'cancelRequestId,cancelUntil,deletionId,requestId,requestVersion,requiresNewLogin,restoreEpoch,status' || stored.get('status').string() !== row.state) { DeletionRules.fail(); } DeletionRules.result(row.result, row); }
  }
  static result(raw: string, row: DeletionIntent): DeletionResult {
    const value = SyncJSON.parse(raw); const status = value.get('status').string(); const version = SyncRules.counter(value.get('requestVersion').string());
    const id = value.get('deletionId').string(); const cancel = value.get('cancelRequestId').string(); const until = value.get('cancelUntil').value === null ? null : value.get('cancelUntil').string();
    if (row.result) { const previous = SyncJSON.parse(row.result); if (previous.get('deletionId').string() !== id || SyncRules.compare(version, previous.get('requestVersion').string()) < 0) { DeletionRules.fail(); } }
    if (!SyncRules.uuid(id) || version === '0' || value.get('requestId').string() !== row.meta.id || value.get('restoreEpoch').string() !== row.meta.epoch || !value.get('requiresNewLogin').bool() ||
      !['pending', 'requested', 'cancel_pending', 'processing', 'completed', 'cancelled', 'retired'].includes(status) || cancel && SelfRules.bytes(cancel) < 8 || SelfRules.bytes(cancel) > 128) { DeletionRules.fail(); }
    if (!value.has('cancelUntil') || !until && !['pending', 'retired'].includes(status)) { DeletionRules.fail(); } if (until) { SelfRules.timestamp(until); }
    return { deletionId: id, requestId: row.meta.id, status: status, requestVersion: version, restoreEpoch: row.meta.epoch, cancelUntil: until, cancelRequestId: cancel, requiresNewLogin: true };
  }
}
export class Deletion {
  private ports: DeletionPorts;
  constructor(ports: DeletionPorts) { this.ports = ports; }
  private check(): void { if (!this.ports.current()) { DeletionRules.fail('DELETION_CONTEXT_CHANGED'); } }
  private async original(id: string): Promise<DeletionIntent> {
    const row = (await this.ports.repository.list()).find((item: DeletionIntent) => item.meta.id === id); this.check();
    if (!row) { DeletionRules.fail(); } DeletionRules.validate(row);
    if (row.meta.profile !== this.ports.profile || row.meta.origin !== this.ports.origin || row.meta.deployment !== this.ports.deployment) { DeletionRules.fail(); } return row;
  }
  private async capability(row: DeletionIntent): Promise<boolean> {
    const value = await this.ports.bootstrap(); this.check(); if (value.deploymentId !== row.meta.deployment) { DeletionRules.fail(); }
    return value.restoreEpoch === row.meta.epoch && value.features?.own_account_deletion === true;
  }
  private async publish(row: DeletionIntent, result: DeletionResult): Promise<DeletionResult> {
    this.check(); const after = JSON.parse(JSON.stringify(row)) as DeletionIntent; after.state = result.status; after.result = JSON.stringify(result);
    if (after.cancelId && after.cancelId === result.cancelRequestId && ['cancel_pending', 'cancelled'].includes(result.status)) { after.cancelState = result.status; }
    await this.ports.repository.replace(row, after); this.check(); if (DeletionRules.blocked(after)) { await this.ports.stop(after); } return result;
  }
  async request(id: string, password: string, otp: string, retry: boolean): Promise<DeletionResult | undefined> {
    password = DeletionRules.password(password); if (otp && !/^[0-9]{6}$/.test(otp)) { DeletionRules.fail(); } this.check();
    let row: DeletionIntent;
    if (!id) {
      if (this.ports.prepare) { await this.ports.prepare(); this.check(); }
      const owner = this.ports.owner(); if (!owner) { DeletionRules.fail('DELETION_LOGIN_REQUIRED'); }
      const meta: PasswordIntent = { id: this.ports.uuid(), profile: this.ports.profile, origin: this.ports.origin, deployment: this.ports.deployment, epoch: this.ports.epoch,
        user: owner.user, username: owner.username, device: owner.device, family: owner.family, namespace: owner.namespace, created: new Date().toISOString(), attempts: 0, state: 'unsent', result: '' };
      row = { meta: meta, attempts: 0, state: 'unsent', result: '', cancelId: '', cancelState: '' }; DeletionRules.validate(row);
      if (!await this.capability(row)) { DeletionRules.fail('DELETION_UNAVAILABLE'); } this.check(); if (this.ports.owner()?.family !== owner.family) { DeletionRules.fail(); }
      await this.ports.repository.insert(row);
    } else { row = await this.original(id); }
    const available = await this.capability(row);
    const body: DeletionBody = { requestId: row.meta.id, username: row.meta.username, deviceId: row.meta.device, restoreEpoch: row.meta.epoch, password: password };
    let result: DeletionResult | undefined; let unseen = false;
    try { result = DeletionRules.result(await this.ports.request('/auth/deletion-results', JSON.stringify(body), ''), row); }
    catch (error) { this.check(); if (error instanceof ApiFailure && error.status === 404 && error.code === 'ACCOUNT_DELETION_NOT_FOUND') { unseen = true; } else { throw error as Error; } }
    this.check(); if (!result && id && !retry) { return undefined; }
    if (!result || result.status === 'pending' && retry) {
      if (!available) { DeletionRules.fail('DELETION_UNAVAILABLE'); }
      const owner = this.ports.owner(); const token = unseen && owner?.family === row.meta.family && owner.device === row.meta.device && owner.user === row.meta.user && owner.namespace === row.meta.namespace ? owner.token : '';
      if (unseen && !token) { DeletionRules.fail('DELETION_LOGIN_REQUIRED'); }
      const before = row; row = JSON.parse(JSON.stringify(row)) as DeletionIntent; row.attempts++; row.state = 'unknown';
      await this.ports.repository.replace(before, row); this.check(); if (token && otp) { body.otp = otp; }
      try { result = DeletionRules.result(await this.ports.request('/auth/deletion-requests', JSON.stringify(body), token), row); this.check(); }
      catch (error) { this.check(); if (!id && before.attempts === 0 && error instanceof ApiFailure && DeletionRules.definiteRejection(error.status, error.code)) { const rejected = JSON.parse(JSON.stringify(row)) as DeletionIntent; rejected.state = 'rejected'; await this.ports.repository.replace(row, rejected); } throw error as Error; }
    }
    if (!result) { DeletionRules.fail(); } return this.publish(row, result);
  }
  async rights(id: string, password: string, recovery: string, otp: string, cancel: boolean, expectedVersion: string = ''): Promise<DeletionResult> {
    this.check(); let row = await this.original(id); const original = DeletionRules.result(row.result, row);
    if (Boolean(password) === Boolean(recovery) || SelfRules.bytes(recovery) > 128 || cancel && expectedVersion !== original.requestVersion) { DeletionRules.fail(); }
    if (cancel && !row.cancelId) { const before = row; row = JSON.parse(JSON.stringify(row)) as DeletionIntent; row.cancelId = this.ports.uuid(); row.cancelState = 'prepared'; await this.ports.repository.replace(before, row); this.check(); }
    const purpose = cancel ? 'cancel' : 'query'; const body: DeletionProofBody = { username: row.meta.username, deletionId: original.deletionId, requestVersion: original.requestVersion, purpose: purpose };
    if (password) { body.password = DeletionRules.password(password); } else { body.recoveryCode = recovery; } if (otp) { body.otp = otp; }
    const proof = SyncJSON.parse(await this.ports.request('/auth/data-rights/proofs', JSON.stringify(body), '')); this.check();
    if (proof.get('purpose').string() !== purpose || proof.get('deletionId').string() !== original.deletionId || proof.get('restoreEpoch').string() !== this.ports.epoch ||
      cancel && proof.get('requestVersion').string() !== original.requestVersion || SyncRules.counter(proof.get('requestVersion').string()) === '0' || Date.parse(proof.get('expiresAt').string()) <= Date.now()) { DeletionRules.fail(); }
    SelfRules.timestamp(proof.get('expiresAt').string());
    const token = proof.get('proof').string(); if (token.length < 32 || token.length > 128) { DeletionRules.fail(); }
    const action: DeletionActionBody = { proof: token };
    if (cancel) { action.requestId = row.cancelId; const before = row; row = JSON.parse(JSON.stringify(row)) as DeletionIntent; row.cancelState = 'unknown'; await this.ports.repository.replace(before, row); this.check(); }
    const result = DeletionRules.result(await this.ports.request('/auth/data-rights/' + (cancel ? 'cancel' : 'query'), JSON.stringify(action), ''), row); this.check(); return this.publish(row, result);
  }
}

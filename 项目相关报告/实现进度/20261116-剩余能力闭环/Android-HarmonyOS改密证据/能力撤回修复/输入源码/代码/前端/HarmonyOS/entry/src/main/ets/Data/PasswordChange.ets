// Created by Jobs on 2026年11月16日，星期一.
import { ApiFailure, Bootstrap } from './Models';
import { SyncJSON, SyncRules } from './SyncProtocol';
import { SelfRules } from './SelfProtocol';

export interface PasswordOwner { user: string; username: string; device: string; family: string; namespace: string; token: string; }
export interface PasswordIntent { id: string; profile: string; origin: string; deployment: string; epoch: string; user: string; username: string; device: string; family: string; namespace: string; created: string; attempts: number; state: string; result: string; }
export interface PasswordResult { requestId: string; status: string; restoreEpoch: string; credentialGeneration: string; securityVersion: string; current: boolean; requiresNewLogin: boolean; }
export interface PasswordRepository { list: () => Promise<PasswordIntent[]>; insert: (row: PasswordIntent) => Promise<void>; replace: (before: PasswordIntent, after: PasswordIntent) => Promise<void>; }
export interface PasswordProof { currentPassword: string; newPassword: string; otp: string; }
export interface PasswordBody { requestId: string; username: string; deviceId: string; restoreEpoch: string; currentPassword: string; newPassword: string; otp?: string; }
export interface PasswordPorts { profile: string; origin: string; deployment: string; epoch: string; current: () => boolean; owner: () => PasswordOwner | undefined; uuid: () => string; repository: PasswordRepository; request: (route: string, body: string, token: string) => Promise<string>; bootstrap: () => Promise<Bootstrap>; stop: (row: PasswordIntent) => Promise<void>; prepare?: () => Promise<void>; }
export class PasswordRules {
  static fail(code: string = 'PASSWORD_CHANGE_INVALID'): never { throw new Error(code); }
  static normalized(old: string, next: string, confirm: string, otp: string): PasswordProof {
    old = old.normalize('NFC'); next = next.normalize('NFC');
    if (!old || old === next || next !== confirm.normalize('NFC') || Array.from(next).length < 15 || Array.from(next).length > 128 ||
      SelfRules.bytes(old) > 512 || SelfRules.bytes(next) > 512 || otp && !/^[0-9]{6}$/.test(otp)) { PasswordRules.fail('PASSWORD_CHANGE_INPUT_INVALID'); }
    return { currentPassword: old, newPassword: next, otp: otp };
  }
  static blocked(row: PasswordIntent): boolean { return row.state === 'changed' || row.attempts > 0 && ['unknown', 'pending'].includes(row.state); }
  static validate(row: PasswordIntent): void {
    if (Object.keys(row).sort().join(',') !== 'attempts,created,deployment,device,epoch,family,id,namespace,origin,profile,result,state,user,username' ||
      ![row.id, row.deployment, row.user, row.device, row.family].every((value: string) => SyncRules.uuid(value)) ||
      !row.profile || !row.namespace || !/^[a-z0-9_]{3,32}$/.test(row.username) || SyncRules.counter(row.epoch) === '0' ||
      !Number.isSafeInteger(row.attempts) || row.attempts < 0 || !['unsent', 'unknown', 'pending', 'changed', 'retired', 'rejected'].includes(row.state)) { PasswordRules.fail(); }
    SelfRules.timestamp(row.created); if (row.result) { PasswordRules.result(row.result, row); }
  }
  static result(raw: string, row: PasswordIntent): PasswordResult {
    const value = SyncJSON.parse(raw); const status = value.get('status').string(); const current = value.get('current').bool(); const login = value.get('requiresNewLogin').bool();
    const credential = SyncRules.counter(value.get('credentialGeneration').string()); const security = SyncRules.counter(value.get('securityVersion').string());
    if (value.get('requestId').string() !== row.id || value.get('restoreEpoch').string() !== row.epoch || credential === '0' || security === '0' ||
      !['pending', 'changed', 'retired'].includes(status) || current && status !== 'changed' || status === 'changed' && !login) { PasswordRules.fail(); }
    return { requestId: row.id, status: status, restoreEpoch: row.epoch, credentialGeneration: credential, securityVersion: security, current: current, requiresNewLogin: login };
  }
}
export class PasswordChange {
  private ports: PasswordPorts;
  constructor(ports: PasswordPorts) { this.ports = ports; }
  private check(): void { if (!this.ports.current()) { PasswordRules.fail('PASSWORD_CHANGE_CONTEXT_CHANGED'); } }
  private async capability(row: PasswordIntent, required: boolean): Promise<boolean> {
    this.check(); const boot = await this.ports.bootstrap(); this.check();
    if (boot.deploymentId !== row.deployment) { PasswordRules.fail('PASSWORD_CHANGE_CONTEXT_CHANGED'); }
    const enabled = boot.restoreEpoch === row.epoch && boot.features?.own_password_change === true;
    if (required && !enabled) { PasswordRules.fail('PASSWORD_CHANGE_UNAVAILABLE'); } return enabled;
  }
  async perform(id: string, proof: PasswordProof, retry: boolean): Promise<PasswordResult | undefined> {
    proof = PasswordRules.normalized(proof.currentPassword, proof.newPassword, proof.newPassword, proof.otp);
    this.check(); let rows = await this.ports.repository.list(); this.check();
    let row: PasswordIntent;
    if (!id) {
      if (this.ports.prepare) { await this.ports.prepare(); this.check(); }
      const owner = this.ports.owner(); if (!owner) { PasswordRules.fail('PASSWORD_CHANGE_LOGIN_REQUIRED'); }
      row = { id: this.ports.uuid(), profile: this.ports.profile, origin: this.ports.origin, deployment: this.ports.deployment, epoch: this.ports.epoch,
        user: owner.user, username: owner.username, device: owner.device, family: owner.family, namespace: owner.namespace, created: new Date().toISOString(), attempts: 0, state: 'unsent', result: '' };
      PasswordRules.validate(row); await this.capability(row, true); this.check();
      if (this.ports.owner()?.family !== owner.family) { PasswordRules.fail('PASSWORD_CHANGE_CONTEXT_CHANGED'); }
      await this.ports.repository.insert(row);
    } else {
      const found = rows.find((item: PasswordIntent) => item.id === id); if (!found) { PasswordRules.fail(); } row = found;
    }
    PasswordRules.validate(row);
    if (row.profile !== this.ports.profile || row.origin !== this.ports.origin || row.deployment !== this.ports.deployment) { PasswordRules.fail(); }
    const available = await this.capability(row, !id);
    const body: PasswordBody = { requestId: row.id, username: row.username, deviceId: row.device, restoreEpoch: row.epoch, currentPassword: proof.currentPassword, newPassword: proof.newPassword };
    let result: PasswordResult | undefined; let unseen = false;
    try { result = PasswordRules.result(await this.ports.request('/auth/password-change-results', JSON.stringify(body), ''), row); }
    catch (error) { this.check(); if (error instanceof ApiFailure && error.status === 404 && error.code === 'PASSWORD_CHANGE_OPERATION_NOT_FOUND') { unseen = true; } else { throw error as Error; } }
    this.check(); if (!result && id && !retry) { return undefined; }
    if (!result || result.status === 'pending' && retry) {
      if (!available) { PasswordRules.fail('PASSWORD_CHANGE_UNAVAILABLE'); }
      const owner = this.ports.owner(); const token = unseen && owner?.family === row.family && owner.device === row.device && owner.user === row.user && owner.namespace === row.namespace ? owner.token : '';
      if (unseen && !token) { PasswordRules.fail('PASSWORD_CHANGE_LOGIN_REQUIRED'); }
      const before = row; row = JSON.parse(JSON.stringify(row)) as PasswordIntent; row.attempts++; row.state = 'unknown';
      await this.ports.repository.replace(before, row); this.check();
      if (token && proof.otp) { body.otp = proof.otp; }
      try { result = PasswordRules.result(await this.ports.request('/auth/password-change', JSON.stringify(body), token), row); this.check(); }
      catch (error) {
        this.check();
        if (!id && before.attempts === 0 && error instanceof ApiFailure && [400, 401, 403, 429].includes(error.status)) {
          const rejected = JSON.parse(JSON.stringify(row)) as PasswordIntent; rejected.state = 'rejected'; await this.ports.repository.replace(row, rejected);
        }
        throw error as Error;
      }
    }
    if (!result) { PasswordRules.fail(); }
    const after = JSON.parse(JSON.stringify(row)) as PasswordIntent; after.state = result.status; after.result = JSON.stringify(result);
    await this.ports.repository.replace(row, after); this.check();
    if (result.requiresNewLogin) { await this.ports.stop(after); } return result;
  }
}

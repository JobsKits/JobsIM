// Created by Jobs on 2026年11月16日，星期一.
import { authSnapshot, authSnapshotCurrent, blockAuth, clearPublicAuth, type AuthSnapshot } from "./AuthSession";
import { ApiError, withAuthLifecycle } from "./api";
import { normalizePasswordChangeProof, passwordChangeFailure, passwordChangeResult, type PasswordChangeIdentity, type PasswordChangeIntent, type PasswordChangeProof, type PasswordChangeResult } from "./PasswordChangeProtocol";
import { createPasswordChangeIntent, passwordChangeIntents, updatePasswordChangeIntent, type PasswordChangeStoreContext } from "./PasswordChangeStore";

export type PasswordChangeContext = PasswordChangeStoreContext & {
  identity?: PasswordChangeIdentity; signal?: AbortSignal; fetch?: typeof fetch;
  onPause?: (row: PasswordChangeIntent, result?: PasswordChangeResult) => void;
};
export class PasswordChangeActions {
  constructor(readonly context: PasswordChangeContext) {}
  rows() { return passwordChangeIntents(this.context); }
  private check(snapshot: AuthSnapshot) {
    this.context.signal?.throwIfAborted();
    if (!this.context.isCurrent() || !authSnapshotCurrent(snapshot)) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
  }
  private originalFamily(row: PasswordChangeIdentity, snapshot: AuthSnapshot) {
    const a = authSnapshot().auth;
    return authSnapshotCurrent(snapshot) && !!a && a.familyId === row.familyId && a.userId === row.userId && a.username === row.username && a.deviceId === row.deviceId && a.restoreEpoch === row.epoch && a.deploymentId === row.scopeId && a.sourceContextId === "personal";
  }
  private async request(path: string, snapshot: AuthSnapshot, body?: object, family?: string, csrf?: string): Promise<unknown> {
    this.check(snapshot);
    const signal = AbortSignal.any([AbortSignal.timeout(15000), ...(this.context.signal ? [this.context.signal] : [])]);
    const headers: Record<string, string> = { "X-JobsIM-Client": "web" };
    if (body) headers["Content-Type"] = "application/json";
    if (family) headers["X-JobsIM-Auth-Family"] = family;
    if (csrf) headers["X-CSRF-Token"] = csrf;
    // History has no Cookie/family/CSRF and cannot clear a later browser login.
    const response = await (this.context.fetch ?? fetch)("/api" + path, { method: body ? "POST" : "GET", headers, credentials: family ? "include" : "omit", redirect: "error", cache: "no-store", body: body ? JSON.stringify(body) : undefined, signal });
    this.check(snapshot);
    const reader = response.body?.getReader();
    if (!reader) passwordChangeFailure("PASSWORD_CHANGE_RESULT_UNKNOWN");
    const parts: Uint8Array[] = []; let size = 0;
    try {
      while (true) { const piece = await reader.read(); this.check(snapshot); if (piece.done) break; size += piece.value.byteLength; if (size > 32768) passwordChangeFailure("PASSWORD_CHANGE_RESULT_UNKNOWN"); parts.push(piece.value); }
    } catch (error) { await reader.cancel().catch(() => {}); throw error; }
    const bytes = new Uint8Array(size); let offset = 0; for (const part of parts) { bytes.set(part, offset); offset += part.byteLength; }
    let value: any;
    try { value = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes)); }
    catch { passwordChangeFailure("PASSWORD_CHANGE_RESULT_UNKNOWN"); }
    if (!response.ok) throw new ApiError(response.status, /^[A-Z0-9_]{1,80}$/.test(value?.error?.code) ? value.error.code : "PASSWORD_CHANGE_RESULT_UNKNOWN", "Password-change original responsibility retained");
    return value;
  }
  private async capability(snapshot: AuthSnapshot, epoch?: string) {
    const value = await this.request("/bootstrap", snapshot) as { deploymentId?: string; restoreEpoch?: string; features?: Record<string, boolean> };
    if (value?.deploymentId !== this.context.scopeId) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
    return value.features?.own_password_change === true && (!epoch || value.restoreEpoch === epoch);
  }
  private body(row: PasswordChangeIntent, proof: PasswordChangeProof, includeOTP = false) {
    return { requestId: row.requestId, username: row.username, deviceId: row.deviceId, restoreEpoch: row.epoch, currentPassword: proof.currentPassword, newPassword: proof.newPassword, ...(includeOTP && proof.otp ? { otp: proof.otp } : {}) };
  }
  private pause(row: PasswordChangeIntent, snapshot: AuthSnapshot, result?: PasswordChangeResult) {
    if (!this.originalFamily(row, snapshot)) return;
    this.context.onPause?.(row, result);
    if (result?.requiresNewLogin) {
      if (this.originalFamily(row, snapshot)) clearPublicAuth(snapshot);
    } else blockAuth("PASSWORD_CHANGE_RESULT_UNKNOWN");
  }
  private async apply(row: PasswordChangeIntent, result: PasswordChangeResult, snapshot: AuthSnapshot) {
    const saved = await updatePasswordChangeIntent(this.context, row, { result, retireNamespace: this.originalFamily(row, snapshot) });
    if (result.requiresNewLogin || result.status === "pending") this.pause(saved, snapshot, result);
    return saved;
  }
  private async locked<T>(action: (snapshot: AuthSnapshot) => Promise<T>) {
    const snapshot = authSnapshot(); this.check(snapshot);
    return withAuthLifecycle(() => {
      this.check(snapshot);
      if (!navigator.locks) passwordChangeFailure("PASSWORD_CHANGE_LOCK_UNAVAILABLE");
      return navigator.locks.request("jobsim-auth-refresh", { mode: "exclusive", signal: this.context.signal }, () => { this.check(snapshot); return action(snapshot); });
    });
  }
  async create(rawProof: PasswordChangeProof) {
    const proof = normalizePasswordChangeProof(rawProof);
    return this.locked(async snapshot => {
      const identity = this.context.identity;
      if (!identity || !this.originalFamily(identity, snapshot)) passwordChangeFailure("PASSWORD_CHANGE_ORIGINAL_FAMILY_REQUIRED");
      if (!await this.capability(snapshot, identity.epoch)) passwordChangeFailure("PASSWORD_CHANGE_UNAVAILABLE");
      this.check(snapshot);
      const row = await createPasswordChangeIntent(this.context, identity);
      return this.perform(row, proof, snapshot, true, false);
    });
  }
  async checkOriginal(id: string, rawProof: PasswordChangeProof, retry = false) {
    const proof = normalizePasswordChangeProof(rawProof);
    return this.locked(async snapshot => {
      const row = (await this.rows()).find(value => value.requestId === id);
      if (!row) passwordChangeFailure("PASSWORD_CHANGE_STORE_INVALID");
      return this.perform(row, proof, snapshot, false, retry);
    });
  }
  private async perform(original: PasswordChangeIntent, proof: PasswordChangeProof, snapshot: AuthSnapshot, fresh: boolean, retry: boolean): Promise<PasswordChangeIntent> {
    const available = await this.capability(snapshot, original.epoch);
    let row = original, found: PasswordChangeResult | undefined;
    try { found = passwordChangeResult(await this.request("/auth/password-change-results", snapshot, this.body(row, proof)), row); }
    catch (error) {
      if (!(error instanceof ApiError && error.status === 404 && error.code === "PASSWORD_CHANGE_OPERATION_NOT_FOUND")) throw error;
    }
    if (found && (found.status !== "pending" || !retry)) return this.apply(row, found, snapshot);
    if (!found && !fresh && !retry) return row;
    // An already prepared operation may continue anonymously; a different live family never authorizes it.
    if (snapshot.auth && !this.originalFamily(row, snapshot)) passwordChangeFailure("PASSWORD_CHANGE_ORIGINAL_FAMILY_REQUIRED");
    let csrf: string | undefined, family: string | undefined;
    if (!found) {
      if (!available || !this.originalFamily(row, snapshot)) passwordChangeFailure("PASSWORD_CHANGE_ORIGINAL_FAMILY_REQUIRED");
      const challenge = await this.request("/auth/csrf", snapshot, undefined, row.familyId) as { csrfToken?: string };
      if (typeof challenge?.csrfToken !== "string" || !challenge.csrfToken) passwordChangeFailure();
      if (!await this.capability(snapshot, row.epoch) || !this.originalFamily(row, snapshot)) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
      csrf = challenge.csrfToken; family = row.familyId;
    }
    const first = row.attempts === 0;
    row = await updatePasswordChangeIntent(this.context, row, { state: "unknown", attempt: true });
    this.check(snapshot);
    try {
      const result = passwordChangeResult(await this.request("/auth/password-change", snapshot, this.body(row, proof, !!family), family, csrf), row);
      return await this.apply(row, result, snapshot);
    } catch (error) {
      if (first && fresh && error instanceof ApiError && [400, 401, 403, 429].includes(error.status)) await updatePasswordChangeIntent(this.context, row, { state: "rejected" });
      else this.pause(row, snapshot);
      throw error;
    }
  }
}

// Created by Jobs on 2026年11月17日，星期二.
import { authSnapshot, authSnapshotCurrent, blockAuth, clearPublicAuth, type AuthSnapshot } from "./AuthSession";
import { ApiError, withAuthLifecycle } from "./api";
import { dataRightsProof, deletionCredential, deletionFailure, deletionResult, type DeletionCase, type DeletionCredential, type DeletionIdentity, type DeletionResult } from "./AccountDeletionProtocol";
import { createDeletionCase, deletionCases, importDeletionCase, updateDeletionCase, type DeletionStoreContext } from "./AccountDeletionStore";

export type AccountDeletionContext = DeletionStoreContext & { identity?: DeletionIdentity; signal?: AbortSignal; fetch?: typeof fetch; onPause?: (row: DeletionCase) => void };
export class AccountDeletionActions {
  constructor(readonly context: AccountDeletionContext) {}
  rows() { return deletionCases(this.context); }
  import(username: string, id: string, version: string) { return importDeletionCase(this.context, username, id, version); }
  private check(snapshot: AuthSnapshot) { this.context.signal?.throwIfAborted(); if (!this.context.isCurrent() || !authSnapshotCurrent(snapshot)) deletionFailure("ACCOUNT_DELETION_CONTEXT_CHANGED"); }
  private sameFamily(row: DeletionCase, snapshot: AuthSnapshot) {
    const identity = row.original?.identity, auth = authSnapshot().auth;
    return !!identity && !!auth && authSnapshotCurrent(snapshot) && identity.familyId === auth.familyId && identity.userId === auth.userId && identity.username === auth.username && identity.deviceId === auth.deviceId && identity.epoch === auth.restoreEpoch && identity.scopeId === auth.deploymentId && auth.sourceContextId === "personal";
  }
  private async request(path: string, snapshot: AuthSnapshot, body?: object, family?: string, csrf?: string): Promise<unknown> {
    this.check(snapshot);
    const headers: Record<string, string> = { "X-JobsIM-Client": "web", ...(body ? { "Content-Type": "application/json" } : {}), ...(family ? { "X-JobsIM-Auth-Family": family } : {}), ...(csrf ? { "X-CSRF-Token": csrf } : {}) };
    const signal = AbortSignal.any([AbortSignal.timeout(15000), ...(this.context.signal ? [this.context.signal] : [])]);
    // Data-rights proofs are purpose-bound RAM values. Anonymous requests never
    // carry another account's Cookie or accept a historical Set-Cookie response.
    const response = await (this.context.fetch ?? fetch)("/api" + path, { method: body ? "POST" : "GET", headers, credentials: family ? "include" : "omit", redirect: "error", cache: "no-store", signal, body: body ? JSON.stringify(body) : undefined });
    this.check(snapshot);
    if (response.headers.get("Content-Type")?.split(";", 1)[0].trim().toLowerCase() !== "application/json") { await response.body?.cancel().catch(() => {}); deletionFailure("ACCOUNT_DELETION_RESULT_UNKNOWN"); }
    const reader = response.body?.getReader(); if (!reader) deletionFailure("ACCOUNT_DELETION_RESULT_UNKNOWN");
    const parts: Uint8Array[] = []; let size = 0;
    try { while (true) { const part = await reader.read(); this.check(snapshot); if (part.done) break; size += part.value.byteLength; if (size > 32768) deletionFailure("ACCOUNT_DELETION_RESULT_UNKNOWN"); parts.push(part.value); } }
    catch (error) { await reader.cancel().catch(() => {}); throw error; }
    const bytes = new Uint8Array(size); let offset = 0; for (const part of parts) { bytes.set(part, offset); offset += part.byteLength; }
    let value: any;
    try { value = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes)); } catch { deletionFailure("ACCOUNT_DELETION_RESULT_UNKNOWN"); }
    if (!response.ok) throw new ApiError(response.status, /^[A-Z0-9_]{1,80}$/.test(value?.error?.code) ? value.error.code : "ACCOUNT_DELETION_RESULT_UNKNOWN", "Original account-deletion responsibility retained");
    return value;
  }
  private async capability(snapshot: AuthSnapshot, epoch?: string) {
    const result = await this.request("/bootstrap", snapshot) as { deploymentId?: string; restoreEpoch?: string; features?: Record<string, boolean> };
    if (result.deploymentId !== this.context.scopeId) deletionFailure("ACCOUNT_DELETION_CONTEXT_CHANGED");
    return result.features?.own_account_deletion === true && (!epoch || result.restoreEpoch === epoch);
  }
  private locked<T>(action: (snapshot: AuthSnapshot) => Promise<T>) {
    const snapshot = authSnapshot(); this.check(snapshot);
    return withAuthLifecycle(() => { this.check(snapshot); if (!navigator.locks) deletionFailure("ACCOUNT_DELETION_LOCK_UNAVAILABLE"); return navigator.locks.request("jobsim-auth-refresh", { mode: "exclusive", signal: this.context.signal }, () => { this.check(snapshot); return action(snapshot); }); });
  }
  private async row(id: string) { const row = (await this.rows()).find(value => value.caseId === id); if (!row) deletionFailure("ACCOUNT_DELETION_STORE_INVALID"); return row; }
  private originalBody(row: DeletionCase, proof: DeletionCredential, otp = false) {
    if (!row.original || !proof.password) deletionFailure("ACCOUNT_DELETION_PROOF_INVALID");
    return { requestId: row.original.requestId, username: row.username, deviceId: row.original.identity.deviceId, restoreEpoch: row.original.identity.epoch, password: proof.password, ...(otp && proof.otp ? { otp: proof.otp } : {}) };
  }
  private pause(row: DeletionCase, snapshot: AuthSnapshot, known: boolean) {
    if (!this.sameFamily(row, snapshot)) return;
    if (known) clearPublicAuth(snapshot); else blockAuth("ACCOUNT_DELETION_RESULT_UNKNOWN");
    this.context.onPause?.(row);
  }
  private async apply(row: DeletionCase, result: DeletionResult, snapshot: AuthSnapshot) {
    const saved = await updateDeletionCase(this.context, row, { result, retireNamespace: this.sameFamily(row, snapshot) });
    this.pause(saved, snapshot, true); return saved;
  }
  async create(raw: DeletionCredential) {
    const proof = deletionCredential(raw, true);
    return this.locked(async snapshot => {
      const identity = this.context.identity;
      if (!identity) deletionFailure("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED");
      const probe = { original: { identity } } as DeletionCase;
      if (!this.sameFamily(probe, snapshot) || !await this.capability(snapshot, identity.epoch)) deletionFailure("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED");
      const row = await createDeletionCase(this.context, identity);
      return this.performOriginal(row, proof, snapshot, true, false);
    });
  }
  checkOriginal(id: string, raw: DeletionCredential, retry = false) {
    const proof = deletionCredential(raw, true);
    return this.locked(async snapshot => this.performOriginal(await this.row(id), proof, snapshot, false, retry));
  }
  private async performOriginal(original: DeletionCase, proof: DeletionCredential, snapshot: AuthSnapshot, fresh: boolean, retry: boolean) {
    if (!original.original) deletionFailure("ACCOUNT_DELETION_ORIGINAL_REQUIRED");
    let row = original, found: DeletionResult | undefined;
    try { found = deletionResult(await this.request("/auth/deletion-results", snapshot, this.originalBody(row, proof)), row); }
    catch (error) { if (!(error instanceof ApiError && error.status === 404 && error.code === "ACCOUNT_DELETION_NOT_FOUND")) throw error; }
    if (found && (found.status !== "pending" || !retry)) return this.apply(row, found, snapshot);
    if (!found && !fresh && !retry) return row;
    if (snapshot.auth && !this.sameFamily(row, snapshot)) deletionFailure("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED");
    let family: string | undefined, csrf: string | undefined;
    if (!found) {
      if (!this.sameFamily(row, snapshot) || !await this.capability(snapshot, row.original!.identity.epoch)) deletionFailure("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED");
      const challenge = await this.request("/auth/csrf", snapshot, undefined, row.original!.identity.familyId) as { csrfToken?: string };
      if (typeof challenge?.csrfToken !== "string" || !challenge.csrfToken || !this.sameFamily(row, snapshot) || !await this.capability(snapshot, row.original!.identity.epoch)) deletionFailure("ACCOUNT_DELETION_CONTEXT_CHANGED");
      family = row.original!.identity.familyId; csrf = challenge.csrfToken;
    }
    const first = row.original!.attempts === 0;
    row = await updateDeletionCase(this.context, row, { attempt: true }); this.check(snapshot);
    try { return await this.apply(row, deletionResult(await this.request("/auth/deletion-requests", snapshot, this.originalBody(row, proof, !!family), family, csrf), row), snapshot); }
    catch (error) {
      if (first && fresh && error instanceof ApiError && (error.status === 400 && error.code === "INVALID_ACCOUNT_DELETION" || error.status === 403 && error.code === "DATA_RIGHTS_REJECTED" || error.status === 429 && error.code === "RATE_LIMITED")) await updateDeletionCase(this.context, row, { originalState: "rejected" });
      else this.pause(row, snapshot, false);
      throw error;
    }
  }
  private async issue(row: DeletionCase, raw: DeletionCredential, purpose: "query" | "cancel", snapshot: AuthSnapshot) {
    const credential = deletionCredential(raw);
    if (!row.observed) deletionFailure("ACCOUNT_DELETION_ORIGINAL_REQUIRED");
    return dataRightsProof(await this.request("/auth/data-rights/proofs", snapshot, { username: row.username, deletionId: row.observed.deletionId, requestVersion: row.observed.requestVersion, purpose, ...credential }), row, purpose);
  }
  query(id: string, raw: DeletionCredential) {
    return this.locked(async snapshot => {
      const row = await this.row(id), proof = await this.issue(row, raw, "query", snapshot);
      return this.apply(row, deletionResult(await this.request("/auth/data-rights/query", snapshot, { proof: proof.proof }), row), snapshot);
    });
  }
  cancel(id: string, raw: DeletionCredential) {
    return this.locked(async snapshot => {
      let row = await this.row(id);
      if (snapshot.auth && !this.sameFamily(row, snapshot)) deletionFailure("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED");
      if (row.result?.status !== "requested" || !row.result.cancelUntil || Date.now() >= Date.parse(row.result.cancelUntil) || row.cancellation && row.cancellation.state !== "unsent") deletionFailure("ACCOUNT_DELETION_CHECK_CANCEL_FIRST");
      row = await updateDeletionCase(this.context, row, { prepareCancel: true });
      const proof = await this.issue(row, raw, "cancel", snapshot);
      if (Date.now() >= Date.parse(proof.expiresAt)) deletionFailure("ACCOUNT_DELETION_PROOF_INVALID");
      row = await updateDeletionCase(this.context, row, { cancelUnknown: true }); this.check(snapshot);
      const result = deletionResult(await this.request("/auth/data-rights/cancel", snapshot, { proof: proof.proof, requestId: row.cancellation!.requestId }), row);
      if (result.cancelRequestId !== row.cancellation!.requestId || !["cancel_pending", "cancelled"].includes(result.status)) deletionFailure();
      return this.apply(row, result, snapshot);
    });
  }
}

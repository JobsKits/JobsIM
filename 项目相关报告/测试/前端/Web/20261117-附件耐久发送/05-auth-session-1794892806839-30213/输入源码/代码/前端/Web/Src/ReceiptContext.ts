// Created by Jobs on 2026年11月17日，星期二.
import { authSnapshot } from "./AuthSession";
import { api, ApiError } from "./api";
import { storageDatabase } from "./StorageDatabase";
import { guardCompletedErasure } from "./AccountDeletionCleanup";
import { receiptSyncStore } from "./SyncStore";
import { syncCounter, syncID, syncRecord } from "./SyncProtocol";
import type { PasswordChangeIdentity } from "./PasswordChangeProtocol";

export const receiptProtocol = "qualified-receipts-v1";
export const browserReceiptCapability = Object.freeze({ qualification: "transport_only", localDurabilityVerified: false,
  qualifiedAck: false, contentReceiptEnabled: false, objectReceiptEnabled: false,
  afterReceiptEnabled: false, afterObjectReceiptEnabled: false, defaultRetentionMode: "cloud_history" });
const readinessKeys = ["transportConfigured", "publishConnected", "consumeConnected", "messageDurable", "journalDurable", "journalIndependent", "epochMatches"] as const;
export type ReceiptReadiness = Record<typeof readinessKeys[number], boolean> & { ready: boolean; blockers: string[] };
export type ReceiptQualification = { qualificationId: string; storeGeneration: string; restoreEpoch: string; profileId: string;
  buildDigest: string; verificationDigest: string; storageContract: string; qualified: boolean; objectQualified: boolean };
export type ReceiptContext = { qualification: "transport_only" | "qualified_content" | "qualified_content_and_objects";
  storage: ReceiptQualification; readiness: ReceiptReadiness; afterReceiptEnabled: boolean; afterObjectReceiptEnabled: boolean };
export type ReceiptDeclaration = { requestId: string; storeGeneration: string; revisionEpoch: string; profileId: string; buildDigest: string; verificationDigest: string };
export type ReceiptOriginal = { body: ReceiptDeclaration; createdAt: string; state: "unknown" | "found" | "not_found" };
export type ReceiptBinding = { identity: PasswordChangeIdentity; isCurrent: () => boolean; request?: typeof api; signal?: AbortSignal };
export type ReceiptTarget = { messageId: string; qualificationId: string; contentVersion: string; deliveryGeneration: string; manifestDigest: string;
  originalTarget: boolean; contentTarget: boolean; objectTarget: boolean; contentRecorded: boolean; objectRecorded: boolean;
  current: boolean; messageCurrent: boolean; receiptScope: string; contentCompleteAt: string | null; objectCompleteAt: string | null };
function refuse(): never { throw Error("RECEIPT_CONTEXT_INVALID"); }
const digest = (value: unknown) => { if (typeof value !== "string" || !/^[0-9a-f]{64}$/.test(value)) refuse(); return value; };
const positive = (value: unknown) => { const result = syncCounter(value); if (result === "0") refuse(); return result; };
const flag = (value: unknown) => { if (typeof value !== "boolean") refuse(); return value; };
const exact = (value: Record<string, unknown>, keys: readonly string[]) => { if (Object.keys(value).length !== keys.length || Object.keys(value).some(key => !keys.includes(key))) refuse(); };
export function receiptDeclaration(raw: unknown): ReceiptDeclaration {
  const value = syncRecord(raw); exact(value, ["requestId", "storeGeneration", "revisionEpoch", "profileId", "buildDigest", "verificationDigest"]);
  if (typeof value.requestId !== "string" || new TextEncoder().encode(value.requestId).length < 8 || new TextEncoder().encode(value.requestId).length > 128 || value.requestId.trim() !== value.requestId || /[\r\n\0]/.test(value.requestId) || typeof value.profileId !== "string" || !/^[a-z0-9._-]{8,128}$/.test(value.profileId)) refuse();
  return { requestId: value.requestId, storeGeneration: syncID(value.storeGeneration), revisionEpoch: positive(value.revisionEpoch),
    profileId: value.profileId, buildDigest: digest(value.buildDigest), verificationDigest: digest(value.verificationDigest) };
}
function qualification(raw: unknown, store: string, epoch: string): ReceiptQualification {
  const value = syncRecord(raw); exact(value, ["qualificationId", "storeGeneration", "restoreEpoch", "profileId", "buildDigest", "verificationDigest", "storageContract", "qualified", "objectQualified"]);
  if (value.storeGeneration !== store || value.restoreEpoch !== epoch) refuse();
  for (const key of ["qualificationId", "profileId", "buildDigest", "verificationDigest", "storageContract"]) if (typeof value[key] !== "string") refuse();
  const qualified = flag(value.qualified), objectQualified = flag(value.objectQualified);
  if (objectQualified && !qualified) refuse();
  if (qualified) {
    syncID(value.qualificationId); digest(value.buildDigest); digest(value.verificationDigest);
    if (!value.profileId || !["native_sqlite_full_v1", "native_sqlite_full_file_fsync_v1"].includes(String(value.storageContract)) || objectQualified && value.storageContract !== "native_sqlite_full_file_fsync_v1") refuse();
  }
  return { qualificationId: value.qualificationId as string, storeGeneration: store, restoreEpoch: epoch, profileId: value.profileId as string,
    buildDigest: value.buildDigest as string, verificationDigest: value.verificationDigest as string, storageContract: value.storageContract as string, qualified, objectQualified };
}
export function parseReceiptContext(raw: unknown, identity: PasswordChangeIdentity, store: string, schema: string): ReceiptContext {
  const value = syncRecord(raw);
  if (value.protocol !== receiptProtocol || value.scopeId !== identity.scopeId || value.sourceContextId !== "personal" || value.principalId !== identity.userId || value.deviceId !== identity.deviceId || value.storeGeneration !== store || value.restoreEpoch !== identity.epoch || value.schemaVersion !== schema || value.defaultRetentionMode !== "cloud_history" || value.deletionEligible !== false || value.targetRangeComplete !== false) refuse();
  const storage = qualification(value.storage, store, identity.epoch), ready = syncRecord(value.readiness);
  exact(ready, [...readinessKeys, "ready", "blockers"]);
  for (const key of readinessKeys) flag(ready[key]);
  flag(ready.ready);
  if (!Array.isArray(ready.blockers) || ready.blockers.length > 32 || ready.blockers.some(item => typeof item !== "string" || !/^[a-z0-9_]{1,80}$/.test(item)) || new Set(ready.blockers).size !== ready.blockers.length || ready.ready !== readinessKeys.every(key => ready[key] === true) || ready.ready && ready.blockers.length) refuse();
  const expected = storage.objectQualified ? "qualified_content_and_objects" : storage.qualified ? "qualified_content" : "transport_only";
  if (value.qualification !== expected || value.qualifiedAck !== storage.qualified || value.contentReceiptEnabled !== storage.qualified || value.objectReceiptEnabled !== storage.objectQualified || value.afterReceiptEnabled !== (storage.qualified && ready.ready) || value.afterObjectReceiptEnabled !== (storage.objectQualified && ready.ready)) refuse();
  return { qualification: expected, storage, readiness: ready as ReceiptReadiness,
    afterReceiptEnabled: value.afterReceiptEnabled as boolean, afterObjectReceiptEnabled: value.afterObjectReceiptEnabled as boolean };
}
function parseOriginalResult(raw: unknown, body: ReceiptDeclaration) {
  const value = syncRecord(raw), original = syncRecord(value.result);
  exact(value, ["result", "current", "storage", "deletionEligible", "afterReceiptEnabled"]);
  exact(original, ["qualificationId", "storeGeneration", "restoreEpoch", "profileId", "buildDigest", "verificationDigest", "storageContract"]);
  if (value.deletionEligible !== false || value.afterReceiptEnabled !== false || original.storeGeneration !== body.storeGeneration || original.restoreEpoch !== body.revisionEpoch || original.profileId !== body.profileId || original.buildDigest !== body.buildDigest || original.verificationDigest !== body.verificationDigest || !["native_sqlite_full_v1", "native_sqlite_full_file_fsync_v1"].includes(String(original.storageContract))) refuse();
  syncID(original.qualificationId); flag(value.current);
  const current = qualification(value.storage, body.storeGeneration, body.revisionEpoch);
  if (value.current !== (current.qualified && current.qualificationId === original.qualificationId)) refuse();
  return { current: value.current, qualificationId: original.qualificationId as string };
}
function parseTarget(raw: unknown, id: string, epoch: string, qualificationID?: string): ReceiptTarget {
  const value = syncRecord(raw);
  if (value.protocol !== receiptProtocol || value.messageId !== id || value.restoreEpoch !== epoch || value.deletionEligible !== false || !["all_target_devices", "per_user_any_device"].includes(String(value.receiptScope))) refuse();
  if (typeof value.qualificationId !== "string" || qualificationID !== undefined && value.qualificationId !== qualificationID) refuse();
  if (value.qualificationId) syncID(value.qualificationId);
  for (const key of ["originalTarget", "contentTarget", "objectTarget", "contentRecorded", "objectRecorded", "current", "messageCurrent"]) flag(value[key]);
  if ((value.contentTarget || value.objectTarget) && (!value.originalTarget || !value.current) || value.objectTarget && !value.contentTarget) refuse();
  for (const key of ["contentCompleteAt", "objectCompleteAt"]) if (value[key] !== null && (typeof value[key] !== "string" || !Number.isFinite(Date.parse(value[key] as string)))) refuse();
  return { messageId: id, qualificationId: value.qualificationId, contentVersion: positive(value.contentVersion), deliveryGeneration: positive(value.deliveryGeneration), manifestDigest: digest(value.manifestDigest),
    originalTarget: value.originalTarget as boolean, contentTarget: value.contentTarget as boolean, objectTarget: value.objectTarget as boolean,
    contentRecorded: value.contentRecorded as boolean, objectRecorded: value.objectRecorded as boolean, current: value.current as boolean, messageCurrent: value.messageCurrent as boolean,
    receiptScope: value.receiptScope as string, contentCompleteAt: value.contentCompleteAt as string | null, objectCompleteAt: value.objectCompleteAt as string | null };
}
export class ReceiptContextActions {
  constructor(readonly context: ReceiptBinding) {}
  private check = () => {
    const i = this.context.identity, auth = authSnapshot().auth;
    if (!this.context.isCurrent() || this.context.signal?.aborted || !auth || auth.familyId !== i.familyId || auth.userId !== i.userId || auth.deviceId !== i.deviceId || auth.restoreEpoch !== i.epoch || auth.deploymentId !== i.scopeId) refuse();
    return true;
  };
  private async physical() {
    this.check(); const i = this.context.identity;
    const value = await receiptSyncStore(i.namespace, { scopeId: i.scopeId, principalId: i.userId, sourceContextId: "personal", deviceId: i.deviceId, restoreEpoch: i.epoch }, this.check);
    this.check(); return value;
  }
  private async request(path: string, store: string, options?: RequestInit) {
    this.check(); if ((await this.physical()).storeGeneration !== store) refuse();
    const headers = new Headers(options?.headers); headers.set("X-JobsIM-Store-Generation", store);
    const value = await (this.context.request ?? api)<unknown>(path, { ...options, headers, signal: this.context.signal });
    this.check(); if ((await this.physical()).storeGeneration !== store) refuse(); return value;
  }
  async load() {
    const physical = await this.physical();
    return parseReceiptContext(await this.request("/sync/receipt-context?schemaVersion=" + physical.schemaVersion, physical.storeGeneration), this.context.identity, physical.storeGeneration, physical.schemaVersion);
  }
  private async originals<T>(operation: (rows: ReceiptOriginal[]) => T): Promise<T> {
    this.check(); const db = await storageDatabase; this.check();
    return new Promise((resolve, reject) => {
      const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces");
      guardCompletedErasure(store, this.context.identity.namespace);
      const key = "receipt-queries|" + this.context.identity.namespace, read = store.get(key); let result: T, cause: unknown;
      read.onsuccess = () => { try {
        this.check(); const rows: ReceiptOriginal[] = read.result ?? [];
        if (!Array.isArray(rows) || rows.length > 1000 || new Set(rows.map(row => row.body?.requestId)).size !== rows.length) refuse();
        for (const row of rows) { exact(syncRecord(row), ["body", "createdAt", "state"]); receiptDeclaration(row.body); if (!["unknown", "found", "not_found"].includes(row.state) || !Number.isFinite(Date.parse(row.createdAt))) refuse(); }
        result = operation(rows); this.check(); store.put(rows, key);
      } catch (error) { cause = error; try { tx.abort(); } catch {} } };
      tx.oncomplete = () => { try { this.check(); resolve(result); } catch (error) { reject(error); } };
      tx.onerror = tx.onabort = () => reject(cause ?? tx.error ?? Error("RECEIPT_STORAGE_FAILED"));
    });
  }
  rows() { return this.originals(rows => structuredClone(rows).reverse()); }
  async checkOriginal(raw: unknown) {
    const body = receiptDeclaration(raw), physical = await this.physical();
    if (body.storeGeneration !== physical.storeGeneration || body.revisionEpoch !== this.context.identity.epoch) refuse();
    await this.originals(rows => {
      const prior = rows.find(row => row.body.requestId === body.requestId);
      if (prior && JSON.stringify(receiptDeclaration(prior.body)) !== JSON.stringify(body) || !prior && rows.length >= 1000) refuse();
      if (!prior) rows.push({ body, createdAt: new Date().toISOString(), state: "unknown" });
    });
    let result: ReturnType<typeof parseOriginalResult> | undefined;
    try { result = parseOriginalResult(await this.request("/sync/storage-results", body.storeGeneration,
      { method: "POST", body: JSON.stringify(body) }), body); }
    catch (error) { if (!(error instanceof ApiError && error.status === 404 && error.code === "STORAGE_DECLARATION_NOT_FOUND")) throw error; }
    await this.originals(rows => { const row = rows.find(item => item.body.requestId === body.requestId); if (!row || JSON.stringify(receiptDeclaration(row.body)) !== JSON.stringify(body)) refuse(); row.state = result ? "found" : "not_found"; });
    return result;
  }
  async target(id: string, qualificationID?: string) {
    const message = syncID(id), physical = await this.physical();
    const suffix = qualificationID === undefined ? "" : "&qualificationId=" + syncID(qualificationID);
    return parseTarget(await this.request("/sync/receipt-targets/" + message + "?schemaVersion=" + physical.schemaVersion + suffix, physical.storeGeneration), message, this.context.identity.epoch, qualificationID);
  }
}

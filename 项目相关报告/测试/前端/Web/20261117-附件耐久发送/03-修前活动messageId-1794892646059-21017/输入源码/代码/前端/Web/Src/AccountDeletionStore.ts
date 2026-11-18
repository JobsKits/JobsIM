// Created by Jobs on 2026年11月17日，星期二.
import { storageDatabase } from "./StorageDatabase";
import { deletionFailure, deletionResult, deletionUsername, validateDeletionCase, type DeletionCase, type DeletionIdentity, type DeletionResult } from "./AccountDeletionProtocol";
import { passwordChangeIdentity } from "./PasswordChangeProtocol";
import { syncCounter, syncID } from "./SyncProtocol";
import { eraseCompletedPersonalRecords } from "./AccountDeletionCleanup";

export type DeletionStoreContext = { scopeId: string; isCurrent: () => boolean };
async function access<T>(context: DeletionStoreContext, apply: (rows: DeletionCase[], store: IDBObjectStore) => T): Promise<T> {
  if (!context.isCurrent()) deletionFailure("ACCOUNT_DELETION_CONTEXT_CHANGED");
  const db = await storageDatabase;
  if (!context.isCurrent()) deletionFailure("ACCOUNT_DELETION_CONTEXT_CHANGED");
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), key = "account-deletions|" + location.origin + "|" + context.scopeId, get = store.get(key);
    let result: T, cause: unknown;
    get.onsuccess = () => {
      try {
        if (!context.isCurrent()) deletionFailure("ACCOUNT_DELETION_CONTEXT_CHANGED");
        const rows: DeletionCase[] = get.result ?? [];
        if (!Array.isArray(rows) || rows.length > 10000 || new Set(rows.map(row => row.caseId)).size !== rows.length) deletionFailure("ACCOUNT_DELETION_STORE_INVALID");
        rows.forEach(row => validateDeletionCase(row, context.scopeId)); result = apply(rows, store);
        rows.forEach(row => validateDeletionCase(row, context.scopeId));
        if (!context.isCurrent()) deletionFailure("ACCOUNT_DELETION_CONTEXT_CHANGED");
        store.put(rows, key);
      } catch (error) { cause = error; tx.abort(); }
    };
    tx.oncomplete = () => resolve(result!); tx.onerror = tx.onabort = () => reject(cause ?? tx.error ?? Error("ACCOUNT_DELETION_STORE_FAILED"));
  });
}
export const deletionCases = (context: DeletionStoreContext) => access(context, rows => structuredClone(rows).reverse());
export function createDeletionCase(context: DeletionStoreContext, identity: DeletionIdentity) {
  passwordChangeIdentity(identity, context.scopeId);
  return access(context, rows => {
    if (rows.length >= 10000 || rows.some(row => row.original?.identity.familyId === identity.familyId && row.original.state !== "rejected")) deletionFailure("ACCOUNT_DELETION_ORIGINAL_PENDING");
    const clean = { scopeId: identity.scopeId, userId: identity.userId, username: identity.username, deviceId: identity.deviceId, epoch: identity.epoch, familyId: identity.familyId, namespace: identity.namespace };
    const row: DeletionCase = { caseId: crypto.randomUUID(), scopeId: context.scopeId, username: identity.username, createdAt: new Date().toISOString(), original: { identity: clean, requestId: crypto.randomUUID(), attempts: 0, state: "unsent" } };
    rows.push(row); return structuredClone(row);
  });
}
export function importDeletionCase(context: DeletionStoreContext, username: string, deletionId: string, requestVersion: string) {
  username = deletionUsername(username); syncID(deletionId); if (syncCounter(requestVersion) === "0") deletionFailure("ACCOUNT_DELETION_PROOF_INVALID");
  return access(context, rows => {
    const known = rows.find(row => row.username === username && row.observed?.deletionId === deletionId);
    if (known) {
      if (BigInt(requestVersion) > BigInt(known.observed!.requestVersion)) { known.observed!.requestVersion = requestVersion; delete known.result; }
      return structuredClone(known);
    }
    if (rows.length >= 10000) deletionFailure("ACCOUNT_DELETION_STORE_INVALID");
    const row: DeletionCase = { caseId: crypto.randomUUID(), scopeId: context.scopeId, username, createdAt: new Date().toISOString(), observed: { deletionId, requestVersion } };
    rows.push(row); return structuredClone(row);
  });
}
export function updateDeletionCase(context: DeletionStoreContext, original: DeletionCase, update: { result?: DeletionResult; attempt?: boolean; originalState?: NonNullable<DeletionCase["original"]>["state"]; prepareCancel?: boolean; cancelUnknown?: boolean; retireNamespace?: boolean }) {
  return access(context, (rows, store) => {
    const row = rows.find(value => value.caseId === original.caseId);
    if (!row || row.username !== original.username || row.createdAt !== original.createdAt || JSON.stringify(row.original?.identity) !== JSON.stringify(original.original?.identity) || row.original?.requestId !== original.original?.requestId) deletionFailure("ACCOUNT_DELETION_CONTEXT_CHANGED");
    if (update.attempt && row.original) { row.original.attempts++; row.original.state = "unknown"; }
    if (update.originalState && row.original) row.original.state = update.originalState;
    if (update.result) {
      const result = deletionResult(update.result,row);
      row.result = result; row.observed = { deletionId: result.deletionId, requestVersion: result.requestVersion };
      if (row.original) row.original.state = "known";
      if (row.cancellation) row.cancellation.state = result.cancelRequestId === row.cancellation.requestId ? "confirmed" : result.cancelRequestId || result.status !== "requested" || result.requestVersion !== row.cancellation.requestVersion ? "closed" : "unsent";
      if (result.status === "completed" && row.original)
        eraseCompletedPersonalRecords(store, row.original.identity, { deletionId: result.deletionId, requestVersion: result.requestVersion }, context.isCurrent);
    }
    if (update.prepareCancel) {
      if (!row.observed || row.result?.status !== "requested" || row.cancellation && row.cancellation.state !== "unsent") deletionFailure("ACCOUNT_DELETION_CHECK_CANCEL_FIRST");
      row.cancellation ??= { requestId: crypto.randomUUID(), requestVersion: row.observed.requestVersion, state: "unsent" };
    }
    if (update.cancelUnknown) { if (!row.cancellation) deletionFailure("ACCOUNT_DELETION_STORE_INVALID"); row.cancellation.state = "unknown"; }
    if (update.retireNamespace && row.original && row.result?.requiresNewLogin) store.put({ reason: "ACCOUNT_DELETION_REQUIRES_LOGIN" }, "auth-frozen|" + row.original.identity.namespace);
    return structuredClone(row);
  });
}
export async function deletionFamilyBlocked(scope?: string, family?: string) {
  if (!scope || !family) return false;
  const rows = await deletionCases({ scopeId: scope, isCurrent: () => true });
  return rows.some(row => row.original?.identity.familyId === family && (row.original.attempts > 0 && row.original.state !== "rejected" || row.result?.requiresNewLogin));
}

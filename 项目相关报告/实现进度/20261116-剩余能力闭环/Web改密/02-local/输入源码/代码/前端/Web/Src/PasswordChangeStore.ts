// Created by Jobs on 2026年11月16日，星期一.
import { storageDatabase } from "./StorageDatabase";
import { passwordChangeFailure, passwordChangeIdentity, validatePasswordChangeIntent, type PasswordChangeIdentity, type PasswordChangeIntent, type PasswordChangeResult } from "./PasswordChangeProtocol";

export type PasswordChangeStoreContext = { scopeId: string; isCurrent: () => boolean };
async function access<T>(context: PasswordChangeStoreContext, change: (rows: PasswordChangeIntent[], store: IDBObjectStore) => T): Promise<T> {
  if (!context.isCurrent()) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
  const db = await storageDatabase;
  if (!context.isCurrent()) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces");
    const key = "password-changes|" + location.origin + "|" + context.scopeId, request = store.get(key);
    let result: T, cause: unknown;
    request.onsuccess = () => {
      try {
        if (!context.isCurrent()) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
        const rows: PasswordChangeIntent[] = request.result ?? [];
        if (!Array.isArray(rows) || rows.length > 10000 || new Set(rows.map(row => row.requestId)).size !== rows.length) passwordChangeFailure("PASSWORD_CHANGE_STORE_INVALID");
        rows.forEach(row => validatePasswordChangeIntent(row, context.scopeId));
        result = change(rows, store);
        if (!context.isCurrent()) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
        store.put(rows, key);
      } catch (error) { cause = error; tx.abort(); }
    };
    tx.oncomplete = () => resolve(result!);
    tx.onerror = tx.onabort = () => reject(cause ?? tx.error ?? Error("PASSWORD_CHANGE_STORE_FAILED"));
  });
}
export function passwordChangeIntents(context: PasswordChangeStoreContext) { return access(context, rows => structuredClone(rows).reverse()); }
export function createPasswordChangeIntent(context: PasswordChangeStoreContext, identity: PasswordChangeIdentity) {
  passwordChangeIdentity(identity, context.scopeId);
  return access(context, rows => {
    if (rows.some(row => row.familyId === identity.familyId && ["unsent", "unknown", "pending"].includes(row.state))) passwordChangeFailure("PASSWORD_CHANGE_ORIGINAL_PENDING");
    const row: PasswordChangeIntent = { ...identity, requestId: crypto.randomUUID(), createdAt: new Date().toISOString(), attempts: 0, state: "unsent" };
    rows.push(row); return structuredClone(row);
  });
}
export function updatePasswordChangeIntent(context: PasswordChangeStoreContext, original: PasswordChangeIntent, update: { state?: PasswordChangeIntent["state"]; attempt?: boolean; result?: PasswordChangeResult; retireNamespace?: boolean }) {
  return access(context, (rows, store) => {
    const row = rows.find(value => value.requestId === original.requestId);
    if (!row || ["scopeId", "userId", "username", "deviceId", "epoch", "familyId", "namespace", "createdAt"].some(key => row[key as keyof PasswordChangeIntent] !== original[key as keyof PasswordChangeIntent])) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
    if (update.attempt) row.attempts++;
    if (update.result) { row.result = update.result; row.state = update.result.status; }
    else if (update.state) row.state = update.state;
    validatePasswordChangeIntent(row, context.scopeId);
    if (update.retireNamespace && row.result?.requiresNewLogin) store.put({ reason: "PASSWORD_CHANGE_REQUIRES_LOGIN" }, "auth-frozen|" + row.namespace);
    return structuredClone(row);
  });
}
// A crash after the original responsibility commits must not reactivate the old BFF family.
export async function passwordChangeFamilyBlocked(scope: string | undefined, family: string | undefined): Promise<boolean> {
  if (!scope || !family) return false;
  const rows = await passwordChangeIntents({ scopeId: scope, isCurrent: () => true });
  return rows.some(row => row.familyId === family && (row.attempts > 0 && ["unknown", "pending"].includes(row.state) || row.result?.requiresNewLogin));
}

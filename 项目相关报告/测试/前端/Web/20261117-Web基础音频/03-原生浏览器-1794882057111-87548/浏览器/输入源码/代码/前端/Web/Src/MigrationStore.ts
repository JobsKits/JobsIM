// Created by Jobs on 2026年11月17日，星期二.
import { storageDatabase } from "./StorageDatabase";
import { passwordChangeIdentity } from "./PasswordChangeProtocol";
import { migrationDigest, migrationFailure, migrationPositive, migrationTime, type MigrationIdentity, type MigrationSelection } from "./MigrationProtocol";
import { syncID } from "./SyncProtocol";

export type MigrationIntent = {
  kind: "pairing" | "create" | "target"; identity: MigrationIdentity; requestId: string; createdAt: string;
  attempts: number; state: "unsent" | "unknown" | "ready" | "installed" | "blocked";
  selection: MigrationSelection[]; sourceDigest?: string;
  target?: { pairingId: string; deviceId: string; storeGeneration: string }; migrationId?: string;
};
export type MigrationStoreContext = { identity: MigrationIdentity; guardGeneration: string; isCurrent: () => boolean };
const identityKeys = ["scopeId", "userId", "username", "deviceId", "epoch", "familyId", "namespace", "storeGeneration"];
export function validateMigrationIntent(row: MigrationIntent, context: MigrationStoreContext) {
  if (!row || Object.keys(row).some(key => !["kind", "identity", "requestId", "createdAt", "attempts", "state", "selection", "sourceDigest", "target", "migrationId"].includes(key)) || !["pairing", "create", "target"].includes(row.kind) || !["unsent", "unknown", "ready", "installed", "blocked"].includes(row.state) || !Number.isSafeInteger(row.attempts) || row.attempts < 0) migrationFailure("MIGRATION_STORE_INVALID");
  passwordChangeIdentity(row.identity, context.identity.scopeId); syncID(row.identity.storeGeneration);
  if (Object.keys(row.identity).some(key => !identityKeys.includes(key)) || row.identity.userId !== context.identity.userId || row.identity.username !== context.identity.username) migrationFailure("MIGRATION_CONTEXT_CHANGED");
  syncID(row.requestId); migrationTime(row.createdAt);
  if (!Array.isArray(row.selection) || row.selection.length > 256 || row.kind !== "create" && row.selection.length || row.kind === "create" && !row.selection.length || new Set(row.selection.map(item => item.id)).size !== row.selection.length) migrationFailure("MIGRATION_STORE_INVALID");
  for (const item of row.selection) {
    if (Object.keys(item).some(key => !["id", "conversationId", "contentVersion", "type", "attachmentId", "forwarded", "objectMode"].includes(key)) || item.forwarded !== undefined && item.forwarded !== true) migrationFailure("MIGRATION_STORE_INVALID");
    syncID(item.id); syncID(item.conversationId); migrationPositive(item.contentVersion);
    if (!["text", "image", "audio", "video", "file"].includes(item.type) || !["none", "metadata_only", "full"].includes(item.objectMode) || (item.type === "text" ? item.objectMode !== "none" || item.attachmentId !== undefined : item.objectMode === "none" || (item.forwarded ? item.objectMode !== "metadata_only" || !!item.attachmentId : !item.attachmentId))) migrationFailure();
    if (item.attachmentId) syncID(item.attachmentId);
  }
  if (row.selection.filter(item => item.objectMode === "full").length > 64) migrationFailure("MIGRATION_SOURCE_LIMIT");
  if (row.sourceDigest) migrationDigest(row.sourceDigest);
  if (row.migrationId) syncID(row.migrationId);
  if (row.target) {
    if (Object.keys(row.target).some(key => !["pairingId", "deviceId", "storeGeneration"].includes(key))) migrationFailure("MIGRATION_STORE_INVALID");
    for (const value of Object.values(row.target)) syncID(value);
  }
}
async function access<T>(context: MigrationStoreContext, operation: (rows: MigrationIntent[], store: IDBObjectStore) => T) {
  if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED");
  const db = await storageDatabase; if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED");
  return new Promise<T>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces");
    const key = "migration-intents|" + location.origin + "|" + context.identity.scopeId + "|" + context.identity.userId;
    const read = store.get(key); let value: T, cause: unknown;
    read.onsuccess = () => {
      try {
        if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED");
        const rows: MigrationIntent[] = read.result ?? [];
        if (!Array.isArray(rows) || rows.length > 1000 || new Set(rows.map(row => row.requestId)).size !== rows.length) migrationFailure("MIGRATION_STORE_INVALID");
        rows.forEach(row => validateMigrationIntent(row, context)); value = operation(rows, store); rows.forEach(row => validateMigrationIntent(row, context));
        if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED"); store.put(rows, key);
      } catch (error) { cause = error; tx.abort(); }
    };
    tx.oncomplete = () => context.isCurrent() ? resolve(value!) : reject(Error("MIGRATION_CONTEXT_CHANGED")); tx.onerror = tx.onabort = () => reject(cause ?? tx.error ?? Error("MIGRATION_STORAGE_FAILED"));
  });
}
export const migrationIntents = (context: MigrationStoreContext) => access(context, rows => structuredClone(rows).reverse());
export function stageMigration(context: MigrationStoreContext, kind: MigrationIntent["kind"], selection: MigrationSelection[] = []) {
  return access(context, rows => {
    if (rows.length >= 1000) migrationFailure("MIGRATION_STORE_INVALID");
    const identity = Object.fromEntries(identityKeys.map(key => [key, context.identity[key as keyof MigrationIdentity]])) as MigrationIdentity;
    const row: MigrationIntent = { kind, identity, requestId: crypto.randomUUID(), createdAt: new Date().toISOString(), attempts: 0, state: "unsent", selection: structuredClone(selection) };
    rows.push(row); return structuredClone(row);
  });
}
export function updateMigration(context: MigrationStoreContext, original: MigrationIntent, change: { attempt?: boolean; state?: MigrationIntent["state"]; sourceDigest?: string; target?: MigrationIntent["target"]; migrationId?: string }) {
  return access(context, rows => {
    const row = rows.find(item => item.requestId === original.requestId);
    if (!row || JSON.stringify(row.identity) !== JSON.stringify(original.identity) || JSON.stringify(row.selection) !== JSON.stringify(original.selection) || row.kind !== original.kind) migrationFailure("MIGRATION_ORIGINAL_CHANGED");
    if (change.sourceDigest && row.sourceDigest && row.sourceDigest !== change.sourceDigest || change.target && row.target && JSON.stringify(row.target) !== JSON.stringify(change.target) || change.migrationId && row.migrationId && change.migrationId !== row.migrationId) migrationFailure("MIGRATION_ORIGINAL_CHANGED");
    if (change.attempt) { row.attempts++; row.state = "unknown"; }
    if (change.state) row.state = change.state;
    if (change.sourceDigest) row.sourceDigest = change.sourceDigest;
    if (change.target) row.target = structuredClone(change.target);
    if (change.migrationId) row.migrationId = change.migrationId;
    return structuredClone(row);
  });
}

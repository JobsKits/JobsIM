// Created by Jobs on 2026年11月17日，星期二.
import { storageDatabase } from "./StorageDatabase";
import { knownGlobalIDs, readGlobalFacts } from "./GlobalMessages";
import { migrationFailure, migrationTime } from "./MigrationProtocol";
import type { MigrationContext } from "./MigrationActions";
import type { MigrationHistory, MigrationHistoryMessage, MigrationView } from "./MigrationArchive";
import type { DurableSyncState } from "./SyncStore";
import type { Space } from "./types";

export type MigrationHistoryEntry = { message: MigrationHistoryMessage; membershipGeneration: string; migrationId: string; object?: { objectId: string; messageId: string; bytes: Blob; contentType: string } };
type History = { schema: "1"; namespace: string; storeGeneration: string; entries: MigrationHistoryEntry[]; imports: { id: string; manifestDigest: string; installedAt: string }[] };
const key = (context: MigrationContext) => "migration-history|" + context.identity.namespace;
const stageKey = (context: MigrationContext, id: string) => "migration-stage|" + context.identity.namespace + "|" + id;
type Stage = { schema: "1"; namespace: string; storeGeneration: string; id: string; manifestDigest: string; capsuleSHA256: string; expiresAt: string; messageIds: string[]; capsule: Blob };
const validCurrent = (context: MigrationContext, state: DurableSyncState, active: unknown, fence: unknown) => {
  const i = context.identity;
  if (!context.isCurrent() || context.signal?.aborted || fence || active !== i.namespace || !state || state.restriction || state.storeGeneration !== i.storeGeneration || state.identity.scopeId !== i.scopeId || state.identity.principalId !== i.userId || state.identity.deviceId !== i.deviceId || state.identity.restoreEpoch !== i.epoch || state.identity.sourceContextId !== "personal") migrationFailure("MIGRATION_CONTEXT_CHANGED");
};
function existing(value: unknown, context: MigrationContext): History {
  if (value === undefined) return { schema: "1", namespace: context.identity.namespace, storeGeneration: context.identity.storeGeneration, entries: [], imports: [] };
  const row = value as History;
  if (!row || Object.keys(row).some(field => !["schema", "namespace", "storeGeneration", "entries", "imports"].includes(field)) || row.schema !== "1" || row.namespace !== context.identity.namespace || row.storeGeneration !== context.identity.storeGeneration || !Array.isArray(row.entries) || row.entries.length > 100000 || !Array.isArray(row.imports) || new Set(row.entries.map(item => item.message.id)).size !== row.entries.length) migrationFailure("MIGRATION_HISTORY_INVALID");
  return row;
}
const sameOrigin = (a: MigrationHistoryMessage, b: MigrationHistoryMessage) => a.id === b.id && a.conversationId === b.conversationId && a.senderId === b.senderId && a.createdAt === b.createdAt && a.creationEpoch === b.creationEpoch && a.creationSequence === b.creationSequence;
const sameContent = (a: MigrationHistoryMessage, b: MigrationHistoryMessage) => { const { objectMode: _a, ...left } = a, { objectMode: _b, ...right } = b; return JSON.stringify(left) === JSON.stringify(right); };
function visible(entry: MigrationHistoryEntry, state: DurableSyncState, space: Space, global: Set<string>) {
  if (global.has(entry.message.id)) return false;
  const conversation = space.conversations.find(item => item.id === entry.message.conversationId);
  if (!conversation || conversation.membershipGeneration !== entry.membershipGeneration) return false;
  const ref = state.projections["conversation|conversation|" + conversation.id];
  return !!ref && ref.state === "available";
}
export async function stageMigrationCapsule(context: MigrationContext, view: MigrationView, capsule: Uint8Array<ArrayBuffer>): Promise<void> {
  if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED"); const db = await storageDatabase;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), i = context.identity;
    const facts = readGlobalFacts(store, i.namespace), active = store.get("auth-active|" + i.namespace.split("|").slice(0, 4).join("|")), fence = store.get("auth-frozen|" + i.namespace), state = store.get("sync-state|" + i.namespace), read = store.get(stageKey(context, view.id)); let cause: unknown;
    read.onsuccess = () => { try {
      validCurrent(context, state.result, active.result, fence.result);
      if (view.manifest.targetDeviceId !== i.deviceId || view.manifest.targetStoreGeneration !== i.storeGeneration || Date.parse(view.expiresAt) <= Date.now() || capsule.length !== Number(view.sizeBytes) || view.manifest.messages.some(item => knownGlobalIDs(i.namespace, facts?.result).has(item.messageId))) migrationFailure("MIGRATION_LOCAL_FLOOR_CHANGED");
      const old = read.result as Stage | undefined; if (old && (old.manifestDigest !== view.manifestDigest || old.capsuleSHA256 !== view.capsuleSHA256 || old.storeGeneration !== i.storeGeneration)) migrationFailure("MIGRATION_ORIGINAL_CHANGED");
      store.put({ schema: "1", namespace: i.namespace, storeGeneration: i.storeGeneration, id: view.id, manifestDigest: view.manifestDigest, capsuleSHA256: view.capsuleSHA256, expiresAt: view.expiresAt, messageIds: view.manifest.messages.map(item => item.messageId), capsule: new Blob([capsule], { type: "application/octet-stream" }) } satisfies Stage, stageKey(context, view.id));
    } catch (error) { cause = error; tx.abort(); } };
    tx.oncomplete = () => resolve(); tx.onabort = tx.onerror = () => reject(cause ?? tx.error ?? Error("MIGRATION_STORAGE_FAILED"));
  });
}
export async function stagedMigrationCapsule(context: MigrationContext, view: MigrationView): Promise<Uint8Array<ArrayBuffer> | undefined> {
  if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED"); const db = await storageDatabase;
  const blob = await new Promise<Blob | undefined>((resolve, reject) => {
    const tx = db.transaction("spaces"), store = tx.objectStore("spaces"), i = context.identity;
    const facts = readGlobalFacts(store, i.namespace), active = store.get("auth-active|" + i.namespace.split("|").slice(0, 4).join("|")), fence = store.get("auth-frozen|" + i.namespace), state = store.get("sync-state|" + i.namespace), read = store.get(stageKey(context, view.id));
    tx.oncomplete = () => { try {
      validCurrent(context, state.result, active.result, fence.result); const stage = read.result as Stage | undefined;
      if (!stage) { resolve(undefined); return; }
      if (stage.schema !== "1" || stage.namespace !== i.namespace || stage.storeGeneration !== i.storeGeneration || stage.id !== view.id || stage.manifestDigest !== view.manifestDigest || stage.capsuleSHA256 !== view.capsuleSHA256 || stage.expiresAt !== view.expiresAt || !(stage.capsule instanceof Blob) || stage.capsule.size !== Number(view.sizeBytes) || stage.messageIds.some(id => knownGlobalIDs(i.namespace, facts?.result).has(id))) migrationFailure("MIGRATION_STAGE_INVALID");
      resolve(stage.capsule);
    } catch (error) { reject(error); } }; tx.onerror = tx.onabort = () => reject(tx.error ?? Error("MIGRATION_STORAGE_FAILED"));
  });
  const bytes = blob ? new Uint8Array(await blob.arrayBuffer()) : undefined; if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED"); return bytes;
}
export async function discardMigrationStage(context: MigrationContext, id: string): Promise<void> {
  if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED"); const db = await storageDatabase;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), read = store.get(stageKey(context, id)); let cause: unknown;
    read.onsuccess = () => { try { if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED"); const row = read.result as Stage | undefined; if (row && (row.namespace !== context.identity.namespace || row.storeGeneration !== context.identity.storeGeneration || row.id !== id)) migrationFailure("MIGRATION_STAGE_INVALID"); store.delete(stageKey(context, id)); } catch (error) { cause = error; tx.abort(); } };
    tx.oncomplete = () => resolve(); tx.onabort = tx.onerror = () => reject(cause ?? tx.error ?? Error("MIGRATION_STORAGE_FAILED"));
  });
}
export async function migrationHistory(context: MigrationContext): Promise<MigrationHistoryEntry[]> {
  if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED"); const db = await storageDatabase;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces"), store = tx.objectStore("spaces"), i = context.identity;
    const facts = readGlobalFacts(store, i.namespace), active = store.get("auth-active|" + i.namespace.split("|").slice(0, 4).join("|")), fence = store.get("auth-frozen|" + i.namespace), state = store.get("sync-state|" + i.namespace), space = store.get(i.namespace), read = store.get(key(context));
    tx.oncomplete = () => { try { validCurrent(context, state.result, active.result, fence.result); const row = existing(read.result, context); resolve(row.entries.filter(entry => visible(entry, state.result, space.result, knownGlobalIDs(i.namespace, facts?.result)))); } catch (error) { reject(error); } };
    tx.onerror = tx.onabort = () => reject(tx.error ?? Error("MIGRATION_STORAGE_FAILED"));
  });
}
export async function publishMigration(context: MigrationContext, view: MigrationView, history: MigrationHistory, guardGeneration: string, installBefore: string): Promise<void> {
  const deadline = Date.parse(migrationTime(installBefore)); if (deadline <= Date.now() || deadline > Date.now() + 5500 || deadline > Date.parse(view.expiresAt)) migrationFailure("MIGRATION_INSTALL_EXPIRED");
  if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED");
  const db = await storageDatabase; if (!context.isCurrent()) migrationFailure("MIGRATION_CONTEXT_CHANGED");
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), i = context.identity;
    const facts = readGlobalFacts(store, i.namespace), active = store.get("auth-active|" + i.namespace.split("|").slice(0, 4).join("|")), fence = store.get("auth-frozen|" + i.namespace), state = store.get("sync-state|" + i.namespace), space = store.get(i.namespace), read = store.get(key(context));
    let cause: unknown;
    read.onsuccess = () => {
      try {
        validCurrent(context, state.result, active.result, fence.result);
        if (state.result.guardGeneration !== guardGeneration || Date.now() >= deadline || view.manifest.targetDeviceId !== i.deviceId || view.manifest.targetStoreGeneration !== i.storeGeneration) migrationFailure("MIGRATION_LOCAL_FLOOR_CHANGED");
        const globals = knownGlobalIDs(i.namespace, facts?.result), row = existing(read.result, context), entries = new Map(row.entries.filter(entry => !globals.has(entry.message.id)).map(entry => [entry.message.id, entry]));
        for (const message of history.messages) {
          const ref = view.manifest.messages.find(item => item.messageId === message.id); if (!ref) migrationFailure();
          const incoming: MigrationHistoryEntry = { message, membershipGeneration: ref.membershipGeneration, migrationId: view.id };
          if (!visible(incoming, state.result, space.result, globals)) migrationFailure("MIGRATION_LOCAL_FLOOR_CHANGED");
          const prior = entries.get(message.id);
          if (prior) {
            if (!sameOrigin(prior.message, message)) migrationFailure("MIGRATION_HISTORY_CONFLICT");
            if (BigInt(prior.message.contentVersion) > BigInt(message.contentVersion)) continue;
            if (prior.message.contentVersion === message.contentVersion) {
              if (!sameContent(prior.message, message)) migrationFailure("MIGRATION_HISTORY_CONFLICT");
              if (prior.object && !history.objects.some(object => object.messageId === message.id)) continue;
            }
          }
          const object = history.objects.find(item => item.messageId === message.id);
          if (object) incoming.object = { objectId: object.objectId, messageId: object.messageId, bytes: new Blob([object.bytes], { type: object.contentType }), contentType: object.contentType };
          entries.set(message.id, incoming);
        }
        row.entries = [...entries.values()];
        const previous = row.imports.find(item => item.id === view.id);
        if (previous && previous.manifestDigest !== view.manifestDigest) migrationFailure("MIGRATION_HISTORY_CONFLICT");
        if (!previous) row.imports.push({ id: view.id, manifestDigest: view.manifestDigest, installedAt: new Date().toISOString() });
        if (Date.now() >= deadline || !context.isCurrent()) migrationFailure("MIGRATION_INSTALL_EXPIRED");
        store.put(row, key(context));
        store.delete(stageKey(context, view.id));
      } catch (error) { cause = error; tx.abort(); }
    };
    tx.oncomplete = () => resolve(); tx.onabort = tx.onerror = () => reject(cause ?? tx.error ?? Error("MIGRATION_STORAGE_FAILED"));
  });
}

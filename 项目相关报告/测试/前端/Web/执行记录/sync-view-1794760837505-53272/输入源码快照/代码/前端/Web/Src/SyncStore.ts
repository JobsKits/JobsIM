import { persistableSpace, storageDatabase } from "./storage";
import { messagePayload } from "./forward";
import { emptySpace, type Message, type Space } from "./types";
import {
  SyncProtocolError, syncCounter, syncID, syncRefKey,
  type SyncBinding, type SyncManifest, type SyncPage, type SyncProjection,
} from "./SyncProtocol";

export type SyncIdentity = Omit<SyncBinding, "storeGeneration" | "streamId">;
export type ProjectionStamp = Omit<SyncProjection, "object">;
export type SnapshotIntent = {
  requestId: string;
  storeGeneration: string;
  schemaVersion: "1";
  pageSize: 100;
};
export type DurableSyncState = {
  schema: "1";
  identity: SyncIdentity;
  storeGeneration: string;
  registrationRequestId: string;
  guardGeneration: string;
  streamId?: string;
  snapshotIntent?: SnapshotIntent;
  pendingManifest?: SyncManifest;
  snapshotId?: string;
  cursor?: string;
  position: string;
  permissionGeneration?: string;
  classificationWatermark?: string;
  restriction?: string;
  projections: Record<string, ProjectionStamp>;
};
export type SyncStage = {
  manifest: SyncManifest;
  pages: Record<string, {
    page: number;
    pageDigest: string;
    referenceEnvelope: string;
    refs: SyncPage["refs"];
    projections: ProjectionStamp[];
  }>;
  obsolete?: string;
  installed?: boolean;
};
export type SyncMaterializer = (
  latest: Space,
  projections: SyncProjection[],
  mode: { replaceCloud: boolean },
) => Space;
export type OutboxProof = {
  clientMsgId: string;
  conversationId: string;
  originalBody: unknown;
};

const stateKey = (namespace: string) => "sync-state|" + namespace;
const stageKey = (namespace: string, id: string) => "sync-stage|" + namespace + "|" + id;
const retainedKey = (namespace: string) => "sync-retained|" + namespace;
const queueKey = (namespace: string) => "outbox|" + namespace;
const identityFields = ["scopeId", "principalId", "sourceContextId", "deviceId", "restoreEpoch"] as const;
function fail(code = "SYNC_CONTEXT_CHANGED"): never {
  throw new SyncProtocolError(code);
}

function assertIdentity(namespace: string, identity: SyncIdentity) {
  const parts = namespace.split("|");
  if (parts.length !== 6 || !parts[0] || parts[1] !== identity.scopeId || parts[2] !== identity.principalId || parts[3] !== "personal" || parts[4] !== identity.restoreEpoch || parts[5] !== identity.deviceId || identity.sourceContextId !== "personal") fail();
  for (const key of ["scopeId", "principalId", "deviceId"] as const) syncID(identity[key]);
  if (syncCounter(identity.restoreEpoch) === "0") fail();
}
function checkState(value: unknown, identity: SyncIdentity): DurableSyncState | undefined {
  if (value === undefined) return undefined;
  const state = value as DurableSyncState;
  if (state?.schema !== "1" || !state.identity || identityFields.some(key => state.identity[key] !== identity[key]) || !state.projections || typeof state.projections !== "object") fail("SYNC_STORE_INVALID");
  syncID(state.storeGeneration);
  syncID(state.registrationRequestId);
  syncID(state.guardGeneration);
  syncCounter(state.position);
  if (state.streamId !== undefined) syncID(state.streamId);
  return state;
}
function stamp(projection: SyncProjection): ProjectionStamp {
  const { object: _body, ...metadata } = projection;
  return metadata;
}

function erasedMessage(message: Message): Message {
  return {
    id: message.id, conversationId: message.conversationId, senderId: message.senderId,
    clientMsgId: message.clientMsgId, type: message.type, createdAt: message.createdAt,
    updatedAt: message.updatedAt, version: message.version, contentVersion: message.contentVersion,
    sequence: message.sequence, text: "", deleted: true, reactions: [], pinned: false, favorite: false,
  };
}
function eraseGlobalMessages(space: Space, ids: Set<string>): Space {
  return { ...space, messages: Object.fromEntries(Object.entries(space.messages).map(([id, rows]) => [
    id, rows.map(message => !message.pending && ids.has(message.id) ? erasedMessage(message) : message),
  ])) };
}
function retainCloud(prior: unknown, latest: Space): Space {
  const older = prior && typeof prior === "object" && "messages" in prior ? prior as Space : emptySpace();
  const rows = new Map(Object.values(older.messages).flat().filter(row => !row.pending).map(row => [row.id, row]));
  for (const row of Object.values(latest.messages).flat()) {
    if (row.pending) continue;
    const previous = rows.get(row.id);
    if (previous && !previous.deleted && !row.deleted && row.contentUnavailable) continue;
    if (previous && BigInt(previous.version) > BigInt(row.version)) continue;
    rows.set(row.id, row);
  }
  const messages: Record<string, Message[]> = {};
  for (const row of rows.values()) (messages[row.conversationId] ??= []).push(row);
  const conversations = new Map([...older.conversations, ...latest.conversations].map(row => [row.id, row]));
  return persistableSpace({ ...emptySpace(), conversations: [...conversations.values()], messages, profiles: { ...older.profiles, ...latest.profiles } });
}
function purgeGlobalHolders(store: IDBObjectStore, namespace: string, ids: Set<string>, isCurrent: () => boolean) {
  if (!ids.size) return;
  const prefix = namespace.split("|").slice(0, 4).join("|") + "|";
  for (const holderPrefix of [prefix, "sync-retained|" + prefix]) {
    const cursor = store.openCursor(IDBKeyRange.bound(holderPrefix, holderPrefix + "\uffff"));
    cursor.onsuccess = () => {
      if (!isCurrent()) { store.transaction.abort(); return; }
      const row = cursor.result;
      if (!row) return;
      try {
        // Outbox records have a different prefix and remain immutable.
        if (row.key !== namespace && row.value?.messages && typeof row.value.messages === "object") {
          row.update(persistableSpace({ ...eraseGlobalMessages(row.value as Space, ids), syncRevision: crypto.randomUUID() }));
        }
        row.continue();
      } catch { store.transaction.abort(); }
    };
  }
}

// No network or asynchronous mapper work may run inside this transaction.
async function update<T>(
  namespace: string,
  identity: SyncIdentity,
  isCurrent: () => boolean,
  operation: (store: IDBObjectStore, state: DurableSyncState | undefined, latest: Space, stage?: unknown, retained?: unknown) => T,
  snapshotId?: string,
): Promise<T> {
  assertIdentity(namespace, identity);
  if (!isCurrent()) fail();
  const db = await storageDatabase;
  if (!isCurrent()) fail();
  return new Promise<T>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces");
    const active = store.get("auth-active|" + namespace.split("|").slice(0, 4).join("|"));
    const fence = store.get("auth-frozen|" + namespace);
    const metadata = store.get(stateKey(namespace));
    const staged = snapshotId ? store.get(stageKey(namespace, snapshotId)) : undefined;
    const retained = store.get(retainedKey(namespace));
    const view = store.get(namespace);
    const queue = store.get(queueKey(namespace));
    let result: T;
    let failure: unknown;
    queue.onsuccess = () => {
      try {
        if (!isCurrent() || fence.result || active.result !== namespace) fail();
        const latest: Space = view.result ?? emptySpace();
        result = operation(store, checkState(metadata.result, identity), { ...latest, outbox: queue.result ?? latest.outbox }, staged?.result, retained.result);
        if (!isCurrent()) fail();
      } catch (error) { failure = error; tx.abort(); }
    };
    tx.oncomplete = () => isCurrent() ? resolve(result) : reject(new SyncProtocolError("SYNC_CONTEXT_CHANGED"));
    tx.onerror = () => reject(failure ?? tx.error ?? new SyncProtocolError("SYNC_STORAGE_FAILED"));
    tx.onabort = () => reject(failure ?? tx.error ?? new SyncProtocolError("SYNC_STORAGE_FAILED"));
  });
}

export function ensureSyncState(namespace: string, identity: SyncIdentity, isCurrent: () => boolean) {
  return update(namespace, identity, isCurrent, (store, current) => {
    if (current) return current;
    const state: DurableSyncState = {
      schema: "1", identity: { ...identity }, storeGeneration: crypto.randomUUID(),
      registrationRequestId: crypto.randomUUID(), position: "0", projections: {},
      guardGeneration: crypto.randomUUID(),
    };
    store.put(state, stateKey(namespace));
    return state;
  });
}

export function bindSyncStream(namespace: string, identity: SyncIdentity, expected: DurableSyncState, streamId: string, isCurrent: () => boolean) {
  syncID(streamId);
  return update(namespace, identity, isCurrent, (store, state) => {
    if (!state || state.storeGeneration !== expected.storeGeneration || state.registrationRequestId !== expected.registrationRequestId || state.streamId && state.streamId !== streamId) fail();
    const next = { ...state, streamId };
    store.put(next, stateKey(namespace));
    return next;
  });
}

export function beginSyncSnapshot(namespace: string, identity: SyncIdentity, isCurrent: () => boolean) {
  return update(namespace, identity, isCurrent, (store, state) => {
    if (!state?.streamId) fail();
    if (state.snapshotIntent) return state;
    const next: DurableSyncState = { ...state, snapshotIntent: {
      requestId: crypto.randomUUID(), storeGeneration: state.storeGeneration, schemaVersion: "1", pageSize: 100,
    } };
    store.put(next, stateKey(namespace));
    return next;
  });
}

function checkManifestState(state: DurableSyncState | undefined, manifest: SyncManifest) {
  if (!state || state.storeGeneration !== manifest.storeGeneration || state.streamId !== manifest.streamId) fail();
  return state;
}

export function stageSyncManifest(namespace: string, identity: SyncIdentity, manifest: SyncManifest, requestId: string, isCurrent: () => boolean) {
  return update(namespace, identity, isCurrent, (store, current, _latest, priorStage) => {
    const state = checkManifestState(current, manifest);
    if (state.snapshotIntent?.requestId !== requestId) fail();
    const old = priorStage as SyncStage | undefined;
    if (old && (old.obsolete || old.manifest.boundaryToken !== manifest.boundaryToken || JSON.stringify(old.manifest.pageManifest) !== JSON.stringify(manifest.pageManifest))) fail("SYNC_STAGE_CHANGED");
    // Retain old evidence on a failed restart, but require fresh page markers at commit.
    store.put({ manifest, pages: old?.pages ?? {} } satisfies SyncStage, stageKey(namespace, manifest.snapshotId));
    const next = { ...state, pendingManifest: manifest };
    store.put(next, stateKey(namespace));
    return next;
  }, manifest.snapshotId);
}

export function stageSyncPage(namespace: string, identity: SyncIdentity, manifest: SyncManifest, page: SyncPage, referenceEnvelope: string, isCurrent: () => boolean) {
  return update(namespace, identity, isCurrent, (store, state) => {
    checkManifestState(state, manifest);
    if (state?.pendingManifest?.snapshotId !== manifest.snapshotId) fail();
    const request = store.get(stageKey(namespace, manifest.snapshotId));
    request.onsuccess = () => {
      try {
        if (!isCurrent()) fail();
        const stage = request.result as SyncStage | undefined;
        if (!stage || stage.manifest.snapshotId !== manifest.snapshotId || stage.obsolete) fail();
        stage.pages[String(page.page)] = {
          page: page.page, pageDigest: page.pageDigest, referenceEnvelope,
          refs: page.refs, projections: page.items.map(stamp),
        };
        store.put(stage, stageKey(namespace, manifest.snapshotId));
      } catch { store.transaction.abort(); }
    };
  });
}
function chooseProjections(previous: Record<string, ProjectionStamp>, incoming: SyncProjection[]) {
  const next = { ...previous }, selected: SyncProjection[] = [];
  for (const item of incoming) {
    const key = syncRefKey(item.ref), prior = next[key];
    if (prior?.currentRevisionEpoch && item.currentRevisionEpoch) {
      const epoch = BigInt(item.currentRevisionEpoch), oldEpoch = BigInt(prior.currentRevisionEpoch);
      const revision = BigInt(item.currentRevision ?? "0"), oldRevision = BigInt(prior.currentRevision ?? "0");
      if (epoch < oldEpoch || epoch === oldEpoch && revision < oldRevision) continue;
      if (epoch === oldEpoch && revision === oldRevision && prior.state === "global" && item.state !== "global") continue;
    }
    const metadata = stamp(item);
    if (!metadata.currentRevision && prior) {
      metadata.currentRevision = prior.currentRevision;
      metadata.currentRevisionEpoch = prior.currentRevisionEpoch;
    }
    next[key] = metadata;
    selected.push(item);
  }
  return { next, selected };
}

export function commitSyncProjection(options: {
  namespace: string;
  identity: SyncIdentity;
  binding: SyncBinding;
  manifest?: SyncManifest;
  expectedCursor?: string;
  expectedGuardGeneration: string;
  cursor: string;
  position: string;
  permissionGeneration: string;
  classificationWatermark: string;
  projections: SyncProjection[];
  outboxProofs?: readonly OutboxProof[];
  verifiedPages?: { value: SyncPage; referenceEnvelope: string }[];
  materialize: SyncMaterializer;
  isCurrent: () => boolean;
}) {
  return update(options.namespace, options.identity, options.isCurrent, (store, current, latest, storedStage, retained) => {
    if (!current || current.storeGeneration !== options.binding.storeGeneration || current.streamId !== options.binding.streamId || !options.manifest && (current.cursor !== options.expectedCursor || current.restriction)) fail();
    if (current.guardGeneration !== options.expectedGuardGeneration) fail("SYNC_LOCAL_RESTRICTION_CHANGED");
    if (options.manifest && current.pendingManifest?.snapshotId !== options.manifest.snapshotId) fail();
    if (current.classificationWatermark && BigInt(options.classificationWatermark) < BigInt(current.classificationWatermark)) fail("SYNC_REVISION_REGRESSION");
    const stage = storedStage as SyncStage | undefined;
    if (options.manifest) {
      if (!stage || stage.obsolete || stage.manifest.snapshotId !== options.manifest.snapshotId || options.verifiedPages?.length !== options.manifest.pageManifest.length || Object.keys(stage.pages).length !== options.manifest.pageManifest.length) fail("SYNC_STAGE_INCOMPLETE");
      for (const [index, expected] of options.manifest.pageManifest.entries()) {
        const saved = stage.pages[String(index)], verified = options.verifiedPages[index];
        if (!saved || saved.page !== index || saved.pageDigest !== expected.digest || saved.refs.length !== expected.count || saved.projections.length !== expected.count || saved.referenceEnvelope !== verified.referenceEnvelope || JSON.stringify(saved.refs) !== JSON.stringify(verified.value.refs)) fail("SYNC_STAGE_INCOMPLETE");
      }
    }
    const selected = chooseProjections(current.projections, options.projections);
    if (options.manifest && selected.selected.length !== options.projections.length) fail("SYNC_REVISION_REGRESSION");
    const outbox = latest.outbox.filter(message => !options.outboxProofs?.some(proof =>
      message.senderId === options.identity.principalId && message.clientMsgId === proof.clientMsgId && message.conversationId === proof.conversationId &&
      JSON.stringify(messagePayload(message)) === JSON.stringify(proof.originalBody),
    ));
    const mapped = options.materialize({ ...latest, outbox }, selected.selected, { replaceCloud: !!options.manifest });
    const globalIDs = new Set(selected.selected.filter(item => item.ref.kind === "message" && item.state === "global").map(item => item.ref.id));
    const space: Space = eraseGlobalMessages({
      ...mapped, drafts: latest.drafts, mentionDrafts: latest.mentionDrafts,
      syncRevision: crypto.randomUUID(), restoreEpoch: options.identity.restoreEpoch,
    }, globalIDs);
    // The mapper may freeze lost-scope intents. Keep its exact original queue.
    const next: DurableSyncState = {
      ...current, projections: selected.next, cursor: options.cursor, position: options.position,
      permissionGeneration: options.permissionGeneration, classificationWatermark: options.classificationWatermark,
      snapshotId: options.manifest?.snapshotId ?? current.snapshotId, restriction: undefined,
      snapshotIntent: undefined, pendingManifest: undefined,
      guardGeneration: crypto.randomUUID(),
    };
    if (options.manifest) store.put(retainCloud(retained, latest), retainedKey(options.namespace));
    store.put(persistableSpace(space), options.namespace);
    store.put(space.outbox, queueKey(options.namespace));
    store.put(next, stateKey(options.namespace));
    purgeGlobalHolders(store, options.namespace, globalIDs, options.isCurrent);
    if (options.manifest && stage) store.put({ ...stage, installed: true }, stageKey(options.namespace, options.manifest.snapshotId));
    return { space, position: next.position, snapshotId: next.snapshotId ?? "" };
  }, options.manifest?.snapshotId);
}

export function restrictSyncProjection(namespace: string, identity: SyncIdentity, reason: string, isCurrent: () => boolean, obsoleteSnapshot = false) {
  return update(namespace, identity, isCurrent, (store, state, latest, _stage, retainedHolder) => {
    if (!state) return;
    const retained: Record<string, Message[]> = {};
    for (const message of latest.outbox) {
      (retained[message.conversationId] ??= []).push(message);
    }
    store.put(retainCloud(retainedHolder, latest), retainedKey(namespace));
    store.put({ ...latest, conversations: [], messages: retained, cursors: {}, profiles: {}, unread: {}, mentionUnread: {}, syncRevision: crypto.randomUUID() }, namespace);
    const next: DurableSyncState = { ...state, restriction: reason, guardGeneration: crypto.randomUUID() };
    if (obsoleteSnapshot) {
      const manifest = state.pendingManifest;
      if (manifest) {
        const read = store.get(stageKey(namespace, manifest.snapshotId));
        read.onsuccess = () => {
          if (!isCurrent()) { store.transaction.abort(); return; }
          const stage = read.result as SyncStage | undefined;
          if (stage) store.put({ ...stage, obsolete: reason }, stageKey(namespace, manifest.snapshotId));
        };
      }
      next.snapshotIntent = undefined;
      next.pendingManifest = undefined;
    }
    store.put(next, stateKey(namespace));
  });
}

export async function hasDurableSyncResponsibility(namespace: string): Promise<boolean> {
  const db = await storageDatabase;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces"), store = tx.objectStore("spaces");
    const state = store.getKey(stateKey(namespace));
    const retained = store.getKey(retainedKey(namespace));
    const prefix = "sync-stage|" + namespace + "|";
    const stage = store.openKeyCursor(IDBKeyRange.bound(prefix, prefix + "\uffff"));
    let found = false;
    stage.onsuccess = () => { found = !!stage.result; };
    tx.oncomplete = () => resolve(state.result !== undefined || retained.result !== undefined || found);
    tx.onerror = () => reject(tx.error ?? new SyncProtocolError("SYNC_STORAGE_FAILED"));
    tx.onabort = () => reject(tx.error ?? new SyncProtocolError("SYNC_STORAGE_FAILED"));
  });
}

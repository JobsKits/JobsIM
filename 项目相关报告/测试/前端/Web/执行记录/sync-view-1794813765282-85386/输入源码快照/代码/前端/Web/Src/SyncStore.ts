import { persistableSpace, storageDatabase } from "./storage";
import { messagePayload } from "./forward";
import { emptySpace, type Message, type Space } from "./types";
import { applyGlobalMessages, globalMessagePlaceholder as erasedMessage, knownGlobalIDs, readGlobalFacts, writeVerifiedGlobals } from "./GlobalMessages";
import {
  SyncProtocolError, syncCounter, syncID, syncRefKey,
  type SyncBinding, type SyncManifest, type SyncPage, type SyncProjection, type SyncSchema, type SyncChange,
} from "./SyncProtocol";
import { emptyPersonalState, parsePersonalState, selfStateKind, type PersonalState } from "./SelfStateProtocol";

export type SyncIdentity = Omit<SyncBinding, "storeGeneration" | "streamId">;
export type ProjectionStamp = Omit<SyncProjection, "object">;
export type SnapshotIntent = {
  requestId: string;
  storeGeneration: string;
  schemaVersion: SyncSchema;
  pageSize: 100;
};
export type DurableSyncState = {
  schema: "1";
  protocolSchema?: SyncSchema;
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
  origins?: Record<string, Omit<Extract<SyncChange, { kind: "reference" }>, "projection" | "kind">>;
  eventPositions?: Record<string, string>;
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
const personalKey = (namespace: string) => "sync-personal|" + namespace;
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
  if (state.protocolSchema !== undefined && state.protocolSchema !== "1" && state.protocolSchema !== "2") fail("SYNC_STORE_INVALID");
  if (state.streamId !== undefined) syncID(state.streamId);
  return state;
}
function stamp(projection: SyncProjection): ProjectionStamp {
  const { object: _body, ...metadata } = projection;
  return metadata;
}

function retainCloud(prior: unknown, latest: Space): Space {
  const older = prior && typeof prior === "object" && "messages" in prior ? prior as Space : emptySpace();
  const rows = new Map(Object.values(older.messages).flat().filter(row => !row.pending).map(row => [row.id, row.deleted ? erasedMessage(row) : row]));
  for (const row of Object.values(latest.messages).flat()) {
    if (row.pending) continue;
    const previous = rows.get(row.id);
    if (previous?.deleted && !row.deleted) continue;
    if (previous && !previous.deleted && !row.deleted && row.contentUnavailable) continue;
    if (previous && BigInt(previous.version) > BigInt(row.version)) continue;
    rows.set(row.id, row.deleted ? erasedMessage(row) : row);
  }
  const messages: Record<string, Message[]> = {};
  for (const row of rows.values()) (messages[row.conversationId] ??= []).push(row);
  const conversations = new Map([...older.conversations, ...latest.conversations].map(row => [row.id, row]));
  return persistableSpace({ ...emptySpace(), conversations: [...conversations.values()], messages, profiles: { ...older.profiles, ...latest.profiles } });
}
// No network or asynchronous mapper work may run inside this transaction.
async function update<T>(
  namespace: string,
  identity: SyncIdentity,
  isCurrent: () => boolean,
  operation: (store: IDBObjectStore, state: DurableSyncState | undefined, latest: Space, stage: unknown, retained: unknown, globalIDs: Set<string>, personal: PersonalState) => T,
  snapshotId?: string,
): Promise<T> {
  assertIdentity(namespace, identity);
  if (!isCurrent()) fail();
  const db = await storageDatabase;
  if (!isCurrent()) fail();
  return new Promise<T>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces");
    const facts = readGlobalFacts(store, namespace);
    const active = store.get("auth-active|" + namespace.split("|").slice(0, 4).join("|"));
    const fence = store.get("auth-frozen|" + namespace);
    const metadata = store.get(stateKey(namespace));
    const staged = snapshotId ? store.get(stageKey(namespace, snapshotId)) : undefined;
    const retained = store.get(retainedKey(namespace));
    const view = store.get(namespace);
    const personal = store.get(personalKey(namespace));
    const queue = store.get(queueKey(namespace));
    let result: T;
    let failure: unknown;
    queue.onsuccess = () => {
      try {
        if (!isCurrent() || fence.result || active.result !== namespace) fail();
        const latest: Space = view.result ?? emptySpace();
        const self = personal.result ?? emptyPersonalState();
        if (!self || !self.records || typeof self.records !== "object" || Array.isArray(self.records)) fail("SYNC_STORE_INVALID");
        result = operation(store, checkState(metadata.result, identity), { ...latest, outbox: queue.result ?? latest.outbox }, staged?.result, retained.result, knownGlobalIDs(namespace, facts?.result), self);
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

export function selectSyncSchema(namespace: string, identity: SyncIdentity, expected: DurableSyncState, schema: SyncSchema, isCurrent: () => boolean) {
  return update(namespace, identity, isCurrent, (store, state, latest, _stage, retained, globalIDs) => {
    if (!state || state.guardGeneration !== expected.guardGeneration) fail("SYNC_LOCAL_RESTRICTION_CHANGED");
    const previous = state.protocolSchema ?? "1";
    if (previous === schema) return state;
    if (previous !== "1" || schema !== "2") fail("SYNC_UNSUPPORTED_SCHEMA");
    store.put(state, "sync-schema-archive|" + namespace + "|" + state.registrationRequestId);
    store.put(applyGlobalMessages(retainCloud(retained, latest), globalIDs), retainedKey(namespace));
    const next: DurableSyncState = {
      schema: "1", protocolSchema: "2", identity: { ...state.identity }, storeGeneration: state.storeGeneration,
      registrationRequestId: crypto.randomUUID(), position: "0", projections: {}, guardGeneration: crypto.randomUUID(),
      classificationWatermark: state.classificationWatermark, restriction: "SYNC_BASELINE_REQUIRED",
    };
    store.put(persistableSpace({ ...latest, conversations: [], messages: {}, profiles: {}, cursors: {}, unread: {}, mentionUnread: {} }), namespace);
    store.put(next, stateKey(namespace));
    return next;
  });
}

export function loadPersonalState(namespace: string, identity: SyncIdentity, isCurrent: () => boolean) {
  return update(namespace, identity, isCurrent, (_store, state, _latest, _stage, _retained, _global, personal) => {
    if (!state || state.protocolSchema !== "2" || state.restriction) fail("SYNC_SELF_STATE_UNAVAILABLE");
    return structuredClone(personal);
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
      requestId: crypto.randomUUID(), storeGeneration: state.storeGeneration, schemaVersion: state.protocolSchema ?? "1", pageSize: 100,
    } };
    store.put(next, stateKey(namespace));
    return next;
  });
}

function checkManifestState(state: DurableSyncState | undefined, manifest: SyncManifest) {
  if (!state || state.storeGeneration !== manifest.storeGeneration || state.streamId !== manifest.streamId || (state.protocolSchema ?? "1") !== manifest.schemaVersion) fail();
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
    if (prior?.state === "global" && item.state !== "global") continue;
    if (prior?.currentRevisionEpoch && item.currentRevisionEpoch) {
      const epoch = BigInt(item.currentRevisionEpoch), oldEpoch = BigInt(prior.currentRevisionEpoch);
      const revision = BigInt(item.currentRevision ?? "0"), oldRevision = BigInt(prior.currentRevision ?? "0");
      if (epoch < oldEpoch || epoch === oldEpoch && revision < oldRevision) continue;
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
  events?: readonly SyncChange[];
  outboxProofs?: readonly OutboxProof[];
  verifiedPages?: { value: SyncPage; referenceEnvelope: string }[];
  materialize: SyncMaterializer;
  isCurrent: () => boolean;
}) {
  return update(options.namespace, options.identity, options.isCurrent, (store, current, latest, storedStage, retained, knownGlobals, personal) => {
    if (!current || current.storeGeneration !== options.binding.storeGeneration || current.streamId !== options.binding.streamId || !options.manifest && (current.cursor !== options.expectedCursor || current.restriction)) fail();
    if (current.guardGeneration !== options.expectedGuardGeneration) fail("SYNC_LOCAL_RESTRICTION_CHANGED");
    if (BigInt(syncCounter(options.position)) < BigInt(current.position)) fail("SYNC_REVISION_REGRESSION");
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
    const selected = chooseProjections(current.projections, options.projections.filter(item =>
      item.ref.kind !== "message" || item.state === "global" || !knownGlobals.has(item.ref.id),
    ));
    if (options.manifest && selected.selected.length !== options.projections.length) fail("SYNC_REVISION_REGRESSION");
    const self: PersonalState = { records: { ...personal.records } };
    for (const item of selected.selected) {
      if (!selfStateKind(item.ref.kind)) continue;
      if (current.protocolSchema !== "2") fail("SYNC_UNSUPPORTED_SCHEMA");
      const row = parsePersonalState(item.object, item.ref);
      if (row.scopeId !== options.identity.scopeId || row.userId !== options.identity.principalId || row.revisionEpoch !== options.identity.restoreEpoch || row.sourceContextId !== options.identity.sourceContextId) fail("SYNC_PERSONAL_IDENTITY_MISMATCH");
      const key = item.ref.kind + "|" + item.ref.id;
      if (row.removed === true) delete self.records[key];
      else if (item.state === "available") self.records[key] = row;
      else if (item.state === "unknown" || item.state === "redacted") fail("SYNC_PROJECTION_UNKNOWN");
    }
    const outbox = latest.outbox.filter(message => !options.outboxProofs?.some(proof =>
      message.senderId === options.identity.principalId && message.clientMsgId === proof.clientMsgId && message.conversationId === proof.conversationId &&
      JSON.stringify(messagePayload(message)) === JSON.stringify(proof.originalBody),
    ));
    const origins = { ...current.origins }, eventPositions = { ...current.eventPositions };
    for (const event of options.events ?? []) {
      if (event.kind !== "reference") continue;
      const { projection: _projection, kind: _kind, ...fact } = event;
      const old = origins[event.originEventId];
      const orderedRef = (value: typeof event.ref) => JSON.stringify(Object.entries(value).sort(([left], [right]) => left.localeCompare(right)));
      if (old && (old.position !== fact.position || old.originEventId !== fact.originEventId || old.eventDigest !== fact.eventDigest || orderedRef(old.ref) !== orderedRef(fact.ref))) fail("SYNC_ORIGIN_CONFLICT");
      const at = eventPositions[event.position];
      if (at && at !== event.originEventId) fail("SYNC_POSITION_CONFLICT");
      origins[event.originEventId] = old ?? fact;
      eventPositions[event.position] = event.originEventId;
    }
    const mapped = options.materialize({ ...latest, outbox }, selected.selected, { replaceCloud: !!options.manifest });
    const globalIDs = new Set(selected.selected.filter(item => item.ref.kind === "message" && item.state === "global").map(item => item.ref.id));
    const allGlobals = new Set([...knownGlobals, ...globalIDs]);
    const space: Space = applyGlobalMessages({
      ...mapped, drafts: latest.drafts, mentionDrafts: latest.mentionDrafts,
      syncRevision: crypto.randomUUID(), restoreEpoch: options.identity.restoreEpoch,
    }, allGlobals, false);
    // The mapper may freeze lost-scope intents. Keep its exact original queue.
    const next: DurableSyncState = {
      ...current, projections: selected.next, origins, eventPositions, cursor: options.cursor, position: options.position,
      permissionGeneration: options.permissionGeneration, classificationWatermark: options.classificationWatermark,
      snapshotId: options.manifest?.snapshotId ?? current.snapshotId, restriction: undefined,
      snapshotIntent: undefined, pendingManifest: undefined,
      guardGeneration: crypto.randomUUID(),
    };
    if (options.manifest) store.put(applyGlobalMessages(retainCloud(retained, latest), allGlobals), retainedKey(options.namespace));
    store.put(persistableSpace(space), options.namespace);
    store.put(space.outbox, queueKey(options.namespace));
    store.put(next, stateKey(options.namespace));
    if (current.protocolSchema === "2") store.put(self, personalKey(options.namespace));
    writeVerifiedGlobals(store, options.namespace, globalIDs, knownGlobals, options.isCurrent);
    if (options.manifest && stage) store.put({ ...stage, installed: true }, stageKey(options.namespace, options.manifest.snapshotId));
    return { space, position: next.position, snapshotId: next.snapshotId ?? "" };
  }, options.manifest?.snapshotId);
}

export function restrictSyncProjection(namespace: string, identity: SyncIdentity, reason: string, isCurrent: () => boolean, obsoleteSnapshot = false) {
  return update(namespace, identity, isCurrent, (store, state, latest, _stage, retainedHolder, globalIDs) => {
    if (!state) return;
    const retained: Record<string, Message[]> = {};
    for (const message of latest.outbox) {
      (retained[message.conversationId] ??= []).push(message);
    }
    store.put(applyGlobalMessages(retainCloud(retainedHolder, latest), globalIDs), retainedKey(namespace));
    store.put(applyGlobalMessages({ ...latest, conversations: [], messages: retained, cursors: {}, profiles: {}, unread: {}, mentionUnread: {}, syncRevision: crypto.randomUUID() }, globalIDs), namespace);
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

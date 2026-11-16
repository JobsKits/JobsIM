import type { Message, Space } from "./types";
import { loadSpace } from "./storage";
import {
  SyncProtocolError, checkSyncBinding, checkSyncValidation, parseSyncChanges,
  parseSyncManifest, parseSyncPage, syncCounter, syncID, syncRecord, syncRefKey,
  type SyncBinding, type SyncPage, type SyncProjection,
} from "./SyncProtocol";
import {
  beginSyncSnapshot, bindSyncStream, commitSyncProjection, ensureSyncState,
  restrictSyncProjection, stageSyncManifest, stageSyncPage,
  type DurableSyncState, type OutboxProof, type SyncIdentity, type SyncMaterializer,
} from "./SyncStore";
export type { OutboxProof } from "./SyncStore";

export type DurableSyncOptions = {
  namespace: string;
  binding: SyncIdentity;
  request: <T>(path: string, options?: RequestInit, timeout?: number) => Promise<T>;
  isCurrent: () => boolean;
  materialize: SyncMaterializer;
  reconcileOutbox?: (candidates: readonly Message[]) => Promise<OutboxProof[]>;
  onRestriction?: (reason: string) => void;
};
export type DurableSyncResult = { space: Space; position: string; snapshotId: string };

const running = new Map<string, { promise: Promise<DurableSyncResult>; isCurrent: () => boolean }>();
const jsonBody = (body: unknown): RequestInit => ({ method: "POST", body: JSON.stringify(body) });
const codeOf = (error: unknown) => error !== null && typeof error === "object" && "code" in error && typeof error.code === "string" ? error.code : "SYNC_UNAVAILABLE";
function assertCurrent(options: DurableSyncOptions) {
  if (!options.isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
}
async function reconcile(options: DurableSyncOptions): Promise<OutboxProof[]> {
  if (!options.reconcileOutbox) return [];
  assertCurrent(options);
  const candidates = (await loadSpace(options.namespace)).outbox;
  assertCurrent(options);
  const proofs = await options.reconcileOutbox(candidates);
  assertCurrent(options);
  return proofs;
}
const obsoleteCodes = new Set([
  "SNAPSHOT_EXPIRED", "SNAPSHOT_INVALIDATED", "SYNC_SNAPSHOT_EXPIRED",
  "SYNC_SNAPSHOT_INVALIDATED", "SYNC_PAGE_NOT_INSTALLABLE", "SYNC_PERMISSION_CHANGED",
]);

async function registered(options: DurableSyncOptions): Promise<DurableSyncState> {
  const state = await ensureSyncState(options.namespace, options.binding, options.isCurrent);
  assertCurrent(options);
  const reply = syncRecord(await options.request<unknown>("/sync/device", {
    method: "PUT", body: JSON.stringify({ requestId: state.registrationRequestId, storeGeneration: state.storeGeneration }),
  }));
  assertCurrent(options);
  for (const [key, value] of Object.entries(options.binding)) {
    if (reply[key] !== value) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
  }
  if (reply.storeGeneration !== state.storeGeneration || reply.qualification !== "transport_only" || reply.qualifiedAck !== false) throw new SyncProtocolError("SYNC_INVALID_RESPONSE");
  return bindSyncStream(options.namespace, options.binding, state, syncID(reply.streamId), options.isCurrent);
}

async function run(options: DurableSyncOptions): Promise<DurableSyncResult> {
  assertCurrent(options);
  const bootstrap = syncRecord(await options.request<unknown>("/bootstrap"));
  assertCurrent(options);
  if (bootstrap.deploymentId !== options.binding.scopeId || bootstrap.restoreEpoch !== options.binding.restoreEpoch) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
  const features = syncRecord(bootstrap.features);
  if (features.authoritative_sync !== true) throw new SyncProtocolError("SYNC_CAPABILITY_REQUIRED");
  let state = await registered(options);
  const binding: SyncBinding = { ...options.binding, storeGeneration: state.storeGeneration, streamId: state.streamId! };
  const context = syncRecord(await options.request<unknown>("/sync/context"));
  assertCurrent(options);
  checkSyncBinding(context, binding);
  if (context.qualification !== "transport_only" || context.qualifiedAck !== false || context.afterReceiptEnabled !== false) throw new SyncProtocolError("SYNC_INVALID_RESPONSE");
  syncCounter(context.head);
  if (typeof context.permissionGeneration !== "string" || !/^[0-9a-f]{64}$/.test(context.permissionGeneration)) throw new SyncProtocolError("SYNC_INVALID_RESPONSE");

  const baseline = !state.cursor || !!state.restriction || state.permissionGeneration !== context.permissionGeneration;
  if (state.permissionGeneration && state.permissionGeneration !== context.permissionGeneration) {
    await restrictSyncProjection(options.namespace, options.binding, "SYNC_PERMISSION_CHANGED", options.isCurrent, true);
    options.onRestriction?.("SYNC_PERMISSION_CHANGED");
    state = await ensureSyncState(options.namespace, options.binding, options.isCurrent);
  }

  if (!baseline) {
    const changes = await options.request<unknown>("/sync/changes?cursor=" + encodeURIComponent(state.cursor!) + "&limit=500");
    assertCurrent(options);
    const delta = await parseSyncChanges(changes, state.position, binding.restoreEpoch, state.permissionGeneration!);
    assertCurrent(options);
    const outboxProofs = await reconcile(options);
    return commitSyncProjection({
      namespace: options.namespace, identity: options.binding, binding, expectedCursor: state.cursor,
      expectedGuardGeneration: state.guardGeneration,
      cursor: delta.cursor, position: delta.position, permissionGeneration: delta.permissionGeneration,
      classificationWatermark: state.classificationWatermark ?? "0",
      projections: delta.items.flatMap(item => item.kind === "reference" ? [item.projection] : []),
      outboxProofs,
      materialize: options.materialize, isCurrent: options.isCurrent,
    });
  }

  state = await beginSyncSnapshot(options.namespace, options.binding, options.isCurrent);
  const intent = state.snapshotIntent!;
  const rawManifest = await options.request<unknown>("/sync/snapshots", jsonBody(intent));
  assertCurrent(options);
  const manifest = parseSyncManifest(rawManifest, binding);
  state = await stageSyncManifest(options.namespace, options.binding, manifest, intent.requestId, options.isCurrent);
  const projections: SyncProjection[] = [], seen = new Set<string>();
  const verifiedPages: { value: SyncPage; referenceEnvelope: string }[] = [];
  for (let page = 0; page < manifest.pageManifest.length; page++) {
    const raw = await options.request<unknown>("/sync/snapshots/" + manifest.snapshotId + "/pages/" + page);
    assertCurrent(options);
    const value = await parseSyncPage(raw, manifest, page);
    assertCurrent(options);
    for (const ref of value.refs) {
      const key = syncRefKey(ref);
      if (seen.has(key)) throw new SyncProtocolError("SYNC_DUPLICATE_REFERENCE");
      seen.add(key);
    }
    const referenceEnvelope = syncRecord(raw).referenceEnvelope as string;
    await stageSyncPage(options.namespace, options.binding, manifest, value, referenceEnvelope, options.isCurrent);
    verifiedPages.push({ value, referenceEnvelope });
    projections.push(...value.items);
  }
  const validation = await options.request<unknown>("/sync/snapshots/" + manifest.snapshotId + "/validate", jsonBody({ pageDigests: manifest.pageManifest.map(page => page.digest) }));
  assertCurrent(options);
  const classificationWatermark = checkSyncValidation(validation, manifest);
  // A final validation is not permission to display before the first current delta.
  const rawChanges = await options.request<unknown>("/sync/changes?cursor=" + encodeURIComponent(manifest.boundaryToken) + "&limit=500");
  assertCurrent(options);
  const delta = await parseSyncChanges(rawChanges, manifest.boundary, manifest.restoreEpoch, manifest.permissionGeneration);
  assertCurrent(options);
  projections.push(...delta.items.flatMap(item => item.kind === "reference" ? [item.projection] : []));
  const outboxProofs = await reconcile(options);
  return commitSyncProjection({
    namespace: options.namespace, identity: options.binding, binding, manifest,
    expectedGuardGeneration: state.guardGeneration,
    cursor: delta.cursor, position: delta.position, permissionGeneration: delta.permissionGeneration,
    classificationWatermark, projections, verifiedPages, outboxProofs, materialize: options.materialize, isCurrent: options.isCurrent,
  });
}

export function runDurableSync(options: DurableSyncOptions): Promise<DurableSyncResult> {
  const existing = running.get(options.namespace);
  if (existing?.isCurrent()) return existing.promise;
  const promise = (async () => {
    if (!navigator.locks) throw new SyncProtocolError("SYNC_LOCK_UNAVAILABLE");
    return navigator.locks.request("jobsim-sync|" + options.namespace, async () => {
      assertCurrent(options);
      try { return await run(options); }
      catch (error) {
        const code = codeOf(error);
        if (options.isCurrent()) {
          try { await restrictSyncProjection(options.namespace, options.binding, code, options.isCurrent, obsoleteCodes.has(code)); }
          catch { /* Keep the original protocol failure; the UI must still hide cloud content. */ }
          options.onRestriction?.(code);
        }
        throw error;
      }
    });
  })();
  running.set(options.namespace, { promise, isCurrent: options.isCurrent });
  void promise.finally(() => { if (running.get(options.namespace)?.promise === promise) running.delete(options.namespace); }).catch(() => {});
  return promise;
}

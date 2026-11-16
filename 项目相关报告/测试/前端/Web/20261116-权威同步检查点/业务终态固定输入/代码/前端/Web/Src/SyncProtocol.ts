export class SyncProtocolError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = "SyncProtocolError";
  }
}

export type SyncBinding = {
  scopeId: string;
  principalId: string;
  sourceContextId: "personal";
  deviceId: string;
  storeGeneration: string;
  restoreEpoch: string;
  streamId: string;
};
export type SyncRef = {
  owner: string;
  kind: string;
  id: string;
  revision: string;
  revisionEpoch: string;
  state: "active" | "removed";
  subjectUserId?: string;
  conversationId?: string;
  targetUserId?: string;
};
export type SyncProjection = {
  ref: SyncRef;
  state: "available" | "global" | "unavailable" | "redacted" | "unknown";
  currentRevision?: string;
  currentRevisionEpoch?: string;
  revisionMatch: boolean;
  bodyDigest?: string;
  object?: unknown;
};
export type SyncManifest = SyncBinding & {
  snapshotId: string;
  schemaVersion: "1";
  boundary: string;
  boundaryToken: string;
  ownerWatermarks: Record<string, string>;
  permissionGeneration: string;
  classificationWatermark: string;
  coverage: Record<string, unknown>;
  pageManifest: { page: number; count: number; digest: string }[];
  expiresAt: string;
};
export type SyncPage = {
  page: number;
  pageDigest: string;
  refs: SyncRef[];
  items: SyncProjection[];
};
export type SyncChange =
  | { kind: "reference"; position: string; originEventId: string; ref: SyncRef; projection: SyncProjection }
  | { kind: "redacted"; from: string; to: string; rangeDigest: string };
export type SyncChanges = {
  items: SyncChange[];
  cursor: string;
  position: string;
  head: string;
  hasMore: boolean;
  restoreEpoch: string;
  permissionGeneration: string;
  snapshotRequired: boolean;
};

type RecordValue = Record<string, unknown>;
function refuse(code = "SYNC_INVALID_RESPONSE"): never {
  throw new SyncProtocolError(code);
}
export const syncRecord = (value: unknown): RecordValue =>
  value !== null && typeof value === "object" && !Array.isArray(value) ? value as RecordValue : refuse();
const text = (value: unknown, max = 4096): string =>
  typeof value === "string" && value.length > 0 && value.length <= max ? value : refuse();
export const syncCounter = (value: unknown): string => {
  const result = text(value, 20);
  if (!/^(0|[1-9][0-9]*)$/.test(result) || BigInt(result) > 9223372036854775807n) refuse();
  return result;
};
export const syncID = (value: unknown): string => {
  const result = text(value, 36);
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(result)) refuse();
  return result;
};
const digest = (value: unknown): string => {
  const result = text(value, 64);
  if (!/^[0-9a-f]{64}$/.test(result)) refuse();
  return result;
};
const flag = (value: unknown): boolean => typeof value === "boolean" ? value : refuse();
const stable = (value: unknown): string => {
  if (Array.isArray(value)) return "[" + value.map(stable).join(",") + "]";
  if (value !== null && typeof value === "object") {
    const record = value as RecordValue;
    return "{" + Object.keys(record).sort().map(key => JSON.stringify(key) + ":" + stable(record[key])).join(",") + "}";
  }
  return JSON.stringify(value);
};
const same = (left: unknown, right: unknown) => stable(left) === stable(right);
export const syncRefKey = (ref: SyncRef) => [ref.owner, ref.kind, ref.id].join("|");

export function parseSyncRef(value: unknown): SyncRef {
  const row = syncRecord(value);
  const kinds: Record<string, string[]> = {
    auth: ["account", "devices"],
    directory: ["profile", "contacts", "relationship", "contact_request"],
    conversation: ["conversation", "announcement"],
    message: ["message"],
  };
  const owner = text(row.owner, 32), kind = text(row.kind, 32);
  if (!Object.hasOwn(kinds, owner) || !kinds[owner].includes(kind) || !["active", "removed"].includes(String(row.state))) refuse();
  const result: SyncRef = { owner, kind, id: syncID(row.id), revision: syncCounter(row.revision), revisionEpoch: syncCounter(row.revisionEpoch), state: row.state as SyncRef["state"] };
  if (result.revisionEpoch === "0") refuse();
  for (const key of ["subjectUserId", "conversationId", "targetUserId"] as const) {
    if (row[key] !== undefined) result[key] = syncID(row[key]);
  }
  if (!same(row, result)) refuse();
  return result;
}

export function checkSyncBinding(value: unknown, binding: SyncBinding) {
  const row = syncRecord(value);
  for (const key of Object.keys(binding) as (keyof SyncBinding)[]) {
    if (row[key] !== binding[key]) refuse("SYNC_CONTEXT_CHANGED");
  }
}

export function parseSyncManifest(value: unknown, binding: SyncBinding): SyncManifest {
  const row = syncRecord(value);
  checkSyncBinding(row, binding);
  if (row.schemaVersion !== "1" || !Array.isArray(row.pageManifest) || row.pageManifest.length < 1 || row.pageManifest.length > 200000) refuse();
  const pages = row.pageManifest.map((value, index) => {
    const page = syncRecord(value);
    if (page.page !== index || !Number.isInteger(page.count) || Number(page.count) < 0 || Number(page.count) > 100) refuse();
    return { page: index, count: Number(page.count), digest: digest(page.digest) };
  });
  if (pages.reduce((total, page) => total + page.count, 0) > 200000) refuse();
  const ownerWatermarks: Record<string, string> = {};
  const watermarks = syncRecord(row.ownerWatermarks);
  for (const owner of ["auth", "directory", "conversation", "message"]) ownerWatermarks[owner] = syncCounter(watermarks[owner]);
  const expiresAt = text(row.expiresAt, 64);
  if (!Number.isFinite(Date.parse(expiresAt)) || Date.parse(expiresAt) <= Date.now()) refuse("SYNC_SNAPSHOT_EXPIRED");
  return { ...binding, snapshotId: syncID(row.snapshotId), schemaVersion: "1", boundary: syncCounter(row.boundary), boundaryToken: text(row.boundaryToken), ownerWatermarks, permissionGeneration: digest(row.permissionGeneration), classificationWatermark: syncCounter(row.classificationWatermark), coverage: syncRecord(row.coverage), pageManifest: pages, expiresAt };
}

// Hash the owner's original bytes: Go and JavaScript JSON escaping differ.
async function openEnvelope(encoded: unknown, expected: unknown): Promise<unknown> {
  const value = text(encoded, 16 * 1024 * 1024), hash = digest(expected);
  if (!/^[A-Za-z0-9_-]+$/.test(value) || value.length % 4 === 1) refuse();
  let bytes: Uint8Array<ArrayBuffer>;
  try {
    bytes = Uint8Array.from(atob(value.replace(/-/g, "+").replace(/_/g, "/")), ch => ch.charCodeAt(0));
  } catch { return refuse(); }
  const actual = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map(byte => byte.toString(16).padStart(2, "0")).join("");
  if (actual !== hash) refuse("SYNC_DIGEST_MISMATCH");
  try { return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes)); }
  catch { return refuse(); }
}

async function parseProjection(value: unknown, expectedRef: SyncRef): Promise<SyncProjection> {
  const row = syncRecord(value), ref = parseSyncRef(row.ref);
  if (!same(ref, expectedRef) || !["available", "global", "unavailable", "redacted", "unknown"].includes(String(row.state))) refuse();
  const result: SyncProjection = { ref, state: row.state as SyncProjection["state"], revisionMatch: flag(row.revisionMatch) };
  if (row.currentRevision !== undefined) result.currentRevision = syncCounter(row.currentRevision);
  if (row.currentRevisionEpoch !== undefined) result.currentRevisionEpoch = syncCounter(row.currentRevisionEpoch);
  if (result.currentRevisionEpoch === "0") refuse();
  if (["available", "global", "unavailable"].includes(result.state) && (result.currentRevision === undefined || result.currentRevisionEpoch === undefined)) refuse();
  if (result.currentRevision !== undefined && result.currentRevisionEpoch !== undefined) {
    if (result.revisionMatch !== (result.currentRevision === ref.revision && result.currentRevisionEpoch === ref.revisionEpoch)) refuse("SYNC_REVISION_MISMATCH");
  } else if (result.revisionMatch) refuse("SYNC_REVISION_MISMATCH");
  if (row.object !== undefined) {
    result.bodyDigest = digest(row.bodyDigest);
    result.object = await openEnvelope(row.bodyEnvelope, result.bodyDigest);
    if (!same(result.object, row.object) || !["available", "global", "unavailable"].includes(result.state)) refuse();
  } else if (result.state === "available") refuse();
  return result;
}

export async function parseSyncPage(value: unknown, manifest: SyncManifest, page: number): Promise<SyncPage> {
  const row = syncRecord(value), expected = manifest.pageManifest[page];
  if (!expected || row.snapshotId !== manifest.snapshotId || row.page !== page || row.restoreEpoch !== manifest.restoreEpoch || row.permissionGeneration !== manifest.permissionGeneration) refuse("SYNC_CONTEXT_CHANGED");
  if (row.installable !== true || row.requiresFinalValidation !== true || row.requiresAtomicInstall !== true || row.qualifiedAck !== false || row.digestScope !== "immutable_references_only") refuse("SYNC_PAGE_NOT_INSTALLABLE");
  if (row.pageDigest !== expected.digest || !Array.isArray(row.items)) refuse("SYNC_DIGEST_MISMATCH");
  const original = await openEnvelope(row.referenceEnvelope, expected.digest);
  if (!Array.isArray(original) || original.length !== expected.count || !same(original, row.refs) || row.items.length !== expected.count) refuse();
  const refs = original.map(parseSyncRef), keys = new Set(refs.map(syncRefKey));
  if (keys.size !== refs.length) refuse("SYNC_DUPLICATE_REFERENCE");
  const items = await Promise.all(row.items.map((item, index) => parseProjection(item, refs[index])));
  if (items.some(item => !item.revisionMatch || item.currentRevisionEpoch !== manifest.restoreEpoch || item.state === "redacted" || item.state === "unknown")) refuse("SYNC_PAGE_NOT_INSTALLABLE");
  return { page, pageDigest: expected.digest, refs, items };
}

export function checkSyncValidation(value: unknown, manifest: SyncManifest) {
  const row = syncRecord(value);
  if (row.canInstall !== true || row.boundaryToken !== manifest.boundaryToken || row.restoreEpoch !== manifest.restoreEpoch || row.permissionGeneration !== manifest.permissionGeneration || row.requiresLocalAtomicInstall !== true || row.qualifiedAck !== false) refuse("SYNC_SNAPSHOT_INVALIDATED");
  const classificationWatermark = syncCounter(row.classificationWatermark);
  if (BigInt(classificationWatermark) < BigInt(manifest.classificationWatermark)) refuse("SYNC_SNAPSHOT_INVALIDATED");
  if (Date.parse(manifest.expiresAt) <= Date.now()) refuse("SYNC_SNAPSHOT_EXPIRED");
  return classificationWatermark;
}

export async function parseSyncChanges(value: unknown, previous: string, epoch: string, permission: string): Promise<SyncChanges> {
  const row = syncRecord(value), position = syncCounter(row.position), head = syncCounter(row.head);
  if (row.restoreEpoch !== epoch) refuse("RESTORE_EPOCH_CHANGED");
  const generation = digest(row.permissionGeneration), required = flag(row.snapshotRequired);
  if (generation !== permission || required) refuse("SYNC_PERMISSION_CHANGED");
  if (row.qualifiedAck !== false || !Array.isArray(row.items) || row.items.length > 500) refuse();
  const items: SyncChange[] = [], origins = new Set<string>();
  let next = BigInt(syncCounter(previous)) + 1n;
  for (const value of row.items) {
    const event = syncRecord(value);
    if (event.kind === "redacted") {
      const from = syncCounter(event.from), to = syncCounter(event.to);
      if (BigInt(from) !== next || BigInt(to) < next) refuse("SYNC_CONTINUITY_GAP");
      items.push({ kind: "redacted", from, to, rangeDigest: digest(event.rangeDigest) });
      next = BigInt(to) + 1n;
    } else if (event.kind === "reference") {
      const at = syncCounter(event.position), originEventId = syncID(event.originEventId), ref = parseSyncRef(event.ref);
      if (BigInt(at) !== next || origins.has(originEventId)) refuse("SYNC_CONTINUITY_GAP");
      const original = await openEnvelope(event.eventEnvelope, event.eventDigest);
      if (!same(original, { position: at, originEventId, ref })) refuse("SYNC_DIGEST_MISMATCH");
      origins.add(originEventId);
      const projection = await parseProjection(event.projection, ref);
      if (ref.revisionEpoch !== epoch || (projection.currentRevisionEpoch !== undefined && projection.currentRevisionEpoch !== epoch)) refuse("RESTORE_EPOCH_CHANGED");
      if (projection.state === "unknown") refuse("SYNC_PROJECTION_UNKNOWN");
      if (projection.state === "redacted") refuse("SYNC_INVALID_REDACTION");
      items.push({ kind: "reference", position: at, originEventId, ref, projection });
      next++;
    } else refuse();
  }
  const hasMore = flag(row.hasMore);
  if (BigInt(position) !== next - 1n || BigInt(head) < BigInt(position) || hasMore !== (BigInt(position) < BigInt(head))) refuse("SYNC_CONTINUITY_GAP");
  return { items, cursor: text(row.cursor), position, head, hasMore, restoreEpoch: epoch, permissionGeneration: generation, snapshotRequired: false };
}

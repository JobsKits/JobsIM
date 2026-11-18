// Created by Jobs on 2026年11月17日，星期二.
import { syncID, syncRecord } from "./SyncProtocol";
import { migrationBase64, migrationDecode, migrationDigest, migrationFailure, migrationHash, migrationLimit, migrationPositive, migrationTime, type MigrationIdentity } from "./MigrationProtocol";

export const migrationFormat = "jobsim-personal-migration-v1";
type MessageReference = { messageId: string; conversationId: string; contentVersion: string; membershipGeneration: string; classification: "available" | "cloud_only"; primaryDigest: string; objectId?: string; objectMode: "none" | "metadata_only" | "full"; deadline?: string };
type ObjectReference = { objectId: string; messageId: string; sizeBytes: string; sha256: string; contentType: string; classification: "available" | "cloud_only"; deadline?: string };
export type MigrationManifest = { format: typeof migrationFormat; id: string; scopeId: string; userId: string; sourceContextId: "personal"; sourceDeviceId: string; sourceStoreGeneration: string; targetDeviceId: string; targetStoreGeneration: string; restoreEpoch: string; securityVersion: string; sourceGeneration: string; messages: MessageReference[]; objects: ObjectReference[]; expiresAt: string };
export type MigrationView = { id: string; requestId: string; status: "building" | "ready" | "installed_reported" | "revoking" | "retired"; manifest: MigrationManifest; manifestDigest: string; capsuleSHA256: string; sizeBytes: string; expiresAt: string };
export type MigrationHistoryMessage = { id: string; conversationId: string; senderId: string; type: string; text: string; replyToId?: string; attachmentId?: string; createdAt: string; contentVersion: string; creationEpoch: string; creationSequence: string; forwarded?: boolean; objectMode: "none" | "metadata_only" | "full" };
export type MigrationHistory = { messages: MigrationHistoryMessage[]; objects: { objectId: string; messageId: string; bytes: Uint8Array<ArrayBuffer>; contentType: string }[] };
const exact = (raw: Record<string, unknown>, keys: string[]) => { if (Object.keys(raw).some(key => !keys.includes(key))) migrationFailure(); };
const stable = (value: unknown): string => value && typeof value === "object" ? Array.isArray(value) ? "[" + value.map(stable).join(",") + "]" : "{" + Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, v]) => JSON.stringify(key) + ":" + stable(v)).join(",") + "}" : JSON.stringify(value);
const ownerJSON = (value: unknown) => JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, ch => "\\u" + ch.charCodeAt(0).toString(16).padStart(4, "0"));
const classification = (value: unknown) => { if (value !== "available" && value !== "cloud_only") migrationFailure(); return value as "available" | "cloud_only"; };
const mode = (value: unknown) => { if (!["none", "metadata_only", "full"].includes(String(value))) migrationFailure(); return value as MessageReference["objectMode"]; };
function manifest(value: unknown, identity: MigrationIdentity): MigrationManifest {
  const row = syncRecord(value);
  exact(row, ["format", "id", "scopeId", "userId", "sourceContextId", "sourceDeviceId", "sourceStoreGeneration", "targetDeviceId", "targetStoreGeneration", "restoreEpoch", "securityVersion", "sourceGeneration", "messages", "objects", "expiresAt"]);
  if (row.format !== migrationFormat || row.sourceContextId !== "personal" || row.scopeId !== identity.scopeId || row.userId !== identity.userId || row.restoreEpoch !== identity.epoch) migrationFailure("MIGRATION_CONTEXT_CHANGED");
  for (const key of ["id", "sourceDeviceId", "sourceStoreGeneration", "targetDeviceId", "targetStoreGeneration"]) syncID(row[key]);
  migrationPositive(row.securityVersion); migrationPositive(row.sourceGeneration); migrationTime(row.expiresAt);
  if (row.objects === null) row.objects = [];
  if (!Array.isArray(row.messages) || !row.messages.length || row.messages.length > 256 || !Array.isArray(row.objects) || row.objects.length > 64) migrationFailure();
  const messages: MessageReference[] = row.messages.map(raw => {
    const item = syncRecord(raw); exact(item, ["messageId", "conversationId", "contentVersion", "membershipGeneration", "classification", "primaryDigest", "objectId", "objectMode", "deadline"]);
    const result: MessageReference = { messageId: syncID(item.messageId), conversationId: syncID(item.conversationId), contentVersion: migrationPositive(item.contentVersion), membershipGeneration: migrationPositive(item.membershipGeneration), classification: classification(item.classification), primaryDigest: migrationDigest(item.primaryDigest), objectMode: mode(item.objectMode) };
    if (item.objectId !== undefined) result.objectId = syncID(item.objectId);
    if (item.deadline !== undefined) result.deadline = migrationTime(item.deadline);
    if (result.objectMode === "none" && result.objectId !== undefined || result.objectMode === "full" && result.objectId === undefined) migrationFailure(); return result;
  });
  const objects: ObjectReference[] = row.objects.map(raw => {
    const item = syncRecord(raw); exact(item, ["objectId", "messageId", "sizeBytes", "sha256", "contentType", "classification", "deadline"]);
    if (typeof item.sizeBytes !== "string" || !/^(0|[1-9][0-9]*)$/.test(item.sizeBytes) || BigInt(item.sizeBytes) > 64n * 1024n * 1024n || typeof item.contentType !== "string" || item.contentType.length > 256 || /[\r\n]/.test(item.contentType)) migrationFailure();
    const result: ObjectReference = { objectId: syncID(item.objectId), messageId: syncID(item.messageId), sizeBytes: item.sizeBytes, sha256: migrationDigest(item.sha256), contentType: item.contentType, classification: classification(item.classification) };
    if (item.deadline !== undefined) result.deadline = migrationTime(item.deadline);
    if (!messages.some(parent => parent.messageId === result.messageId && parent.objectId === result.objectId && parent.objectMode === "full")) migrationFailure(); return result;
  });
  if (new Set(messages.map(item => item.messageId)).size !== messages.length || new Set(objects.map(item => item.objectId)).size !== objects.length || messages.filter(item => item.objectMode === "full").length !== objects.length) migrationFailure();
  return { ...row, messages, objects } as MigrationManifest;
}
export async function migrationView(raw: unknown, identity: MigrationIdentity, original?: { requestId: string; sourceStoreGeneration: string }): Promise<MigrationView> {
  const row = syncRecord(raw), bytes = migrationDecode(row.manifestEnvelope, 1024 * 1024, true), digest = migrationDigest(row.manifestDigest);
  if (await migrationHash(bytes) !== digest) migrationFailure("MIGRATION_MANIFEST_CHANGED");
  let decoded: unknown; try { decoded = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes)); } catch { return migrationFailure(); }
  if (stable(decoded) !== stable(row.manifest)) migrationFailure("MIGRATION_MANIFEST_CHANGED");
  const value = manifest(decoded, identity);
  if (row.id !== value.id || typeof row.requestId !== "string" || row.requestId.length < 8 || row.requestId.length > 128 || !["building", "ready", "installed_reported", "revoking", "retired"].includes(String(row.status)) || row.sourcePreserved !== true || row.qualifiedAck !== false || row.targetQualified !== false || row.expiresAt !== value.expiresAt) migrationFailure();
  if (original && (row.requestId !== original.requestId || value.sourceDeviceId !== identity.deviceId || value.sourceStoreGeneration !== original.sourceStoreGeneration)) migrationFailure("MIGRATION_ORIGINAL_CHANGED");
  if (typeof row.sizeBytes !== "string" || !/^(0|[1-9][0-9]*)$/.test(row.sizeBytes) || BigInt(row.sizeBytes) > BigInt(migrationLimit + 4096)) migrationFailure();
  if (["ready", "installed_reported"].includes(String(row.status))) migrationDigest(row.capsuleSHA256);
  else if (row.capsuleSHA256 !== "") migrationDigest(row.capsuleSHA256);
  return { id: value.id, requestId: row.requestId, status: row.status as MigrationView["status"], manifest: value, manifestDigest: digest, capsuleSHA256: row.capsuleSHA256 as string, sizeBytes: row.sizeBytes, expiresAt: value.expiresAt };
}
export function migrationAAD(view: MigrationView): Uint8Array<ArrayBuffer> {
  const m = view.manifest; return new TextEncoder().encode(["JobsIM migration v1", m.scopeId, m.id, m.userId, m.sourceDeviceId, m.sourceStoreGeneration, m.targetDeviceId, m.targetStoreGeneration, m.restoreEpoch, m.securityVersion, m.sourceGeneration, view.manifestDigest].join("\0"));
}
export async function openMigration(view: MigrationView, capsule: Uint8Array<ArrayBuffer>, rawKey: unknown, identity: MigrationIdentity): Promise<MigrationHistory> {
  const m = view.manifest;
  if (m.targetDeviceId !== identity.deviceId || m.targetStoreGeneration !== identity.storeGeneration || !["ready", "installed_reported"].includes(view.status) || Date.parse(view.expiresAt) <= Date.now()) migrationFailure("MIGRATION_TARGET_CHANGED");
  if (capsule.length !== Number(view.sizeBytes) || capsule.length < 36 || await migrationHash(capsule) !== view.capsuleSHA256 || new TextDecoder().decode(capsule.subarray(0, 8)) !== "JIMMIGR1") migrationFailure("MIGRATION_CAPSULE_INVALID");
  const k = syncRecord(rawKey);
  if (k.id !== view.id || k.manifestDigest !== view.manifestDigest || k.algorithm !== "AES-256-GCM" || k.targetQualified !== false || k.expiresAt !== view.expiresAt) migrationFailure();
  const bytes = migrationDecode(k.key, 32, true), aad = migrationDecode(k.aad, 8192, true), expectedAAD = migrationAAD(view);
  if (bytes.length !== 32 || migrationBase64(aad, true) !== migrationBase64(expectedAAD, true)) migrationFailure("MIGRATION_CONTEXT_CHANGED");
  let plain: ArrayBuffer;
  try { const key = await crypto.subtle.importKey("raw", bytes, "AES-GCM", false, ["decrypt"]); plain = await crypto.subtle.decrypt({ name: "AES-GCM", iv: capsule.subarray(8, 20), additionalData: aad, tagLength: 128 }, key, capsule.subarray(20)); }
  catch { return migrationFailure("MIGRATION_CAPSULE_INVALID"); }
  finally { bytes.fill(0); }
  try {
    if (plain.byteLength > migrationLimit) migrationFailure();
    const raw = syncRecord(JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(plain))); exact(raw, ["format", "manifestDigest", "messages", "objects"]);
    if (raw.objects === null) raw.objects = [];
    if (raw.format !== migrationFormat || raw.manifestDigest !== view.manifestDigest || !Array.isArray(raw.messages) || raw.messages.length !== m.messages.length || !Array.isArray(raw.objects) || raw.objects.length !== m.objects.length) migrationFailure();
    const messages: MigrationHistoryMessage[] = [];
    for (const [index, value] of raw.messages.entries()) {
      const row = syncRecord(value), ref = m.messages[index]; exact(row, ["id", "conversationId", "senderId", "type", "text", "replyToId", "attachmentId", "createdAt", "contentVersion", "creationEpoch", "creationSequence", "forwarded", "objectMode"]);
      if (row.id !== ref.messageId || row.conversationId !== ref.conversationId || row.contentVersion !== ref.contentVersion || row.attachmentId !== ref.objectId || row.objectMode !== ref.objectMode || row.forwarded !== undefined && row.forwarded !== true || row.forwarded && (ref.objectId || ref.objectMode === "full") || typeof row.text !== "string" || new TextEncoder().encode(row.text).length > 16384) migrationFailure();
      syncID(row.senderId); migrationTime(row.createdAt); migrationPositive(row.creationEpoch); migrationPositive(row.creationSequence);
      if (!["text", "image", "audio", "video", "file"].includes(String(row.type)) || (row.type === "text") !== (ref.objectMode === "none")) migrationFailure();
      const history: MigrationHistoryMessage = { id: ref.messageId, conversationId: ref.conversationId, senderId: row.senderId as string, type: row.type as string, text: row.text, ...(row.replyToId ? { replyToId: syncID(row.replyToId) } : {}), ...(ref.objectId ? { attachmentId: ref.objectId } : {}), createdAt: row.createdAt as string, contentVersion: ref.contentVersion, creationEpoch: row.creationEpoch as string, creationSequence: row.creationSequence as string, ...(row.forwarded ? { forwarded: true } : {}), objectMode: ref.objectMode };
      if (await migrationHash(new TextEncoder().encode(ownerJSON(history))) !== ref.primaryDigest) migrationFailure("MIGRATION_SOURCE_CHANGED"); messages.push(history);
    }
    const objects: MigrationHistory["objects"] = [];
    for (const [index, value] of raw.objects.entries()) {
      const row = syncRecord(value), ref = m.objects[index]; exact(row, ["objectId", "messageId", "bytes"]);
      const bytes = migrationDecode(row.bytes, 64 * 1024 * 1024);
      if (row.objectId !== ref.objectId || row.messageId !== ref.messageId || bytes.length !== Number(ref.sizeBytes) || await migrationHash(bytes) !== ref.sha256) migrationFailure("MIGRATION_OBJECT_INVALID");
      objects.push({ objectId: ref.objectId, messageId: ref.messageId, bytes, contentType: ref.contentType });
    }
    return { messages, objects };
  } finally { new Uint8Array(plain).fill(0); }
}

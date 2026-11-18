// Created by Jobs on 2026年11月17日，星期二.
import type { Message } from "./types";

export const attachmentScopeLimit = 32;
export const attachmentScopeBytes = 64 * 1024 * 1024;
export const attachmentAudioBytes = 8 * 1024 * 1024;
export type AttachmentIdentity = { namespace: string; userId: string; deviceId: string; familyId: string; restoreEpoch: string };
export type UploadManifest = { requestId: string; userId: string; deviceId: string; familyId: string; restoreEpoch: string; name: string; size: string; sha256: string };
export type UploadReceipt = { id: string; name: string; contentType: string; size: number; sha256: string };
export type AttachmentSendState = "prepared" | "unknown" | "uploaded" | "enqueued" | "accepted" | "cancelled" | "retired";
export type AttachmentSendIntent = {
  requestId: string; clientMsgId: string; identity: AttachmentIdentity; manifest: UploadManifest; message: Message;
  replyContentVersion?: string; blob?: Blob; state: AttachmentSendState; revision: string; createdAt: number; updatedAt: number;
  receipt?: UploadReceipt; preparedId?: string; messageId?: string; failure?: string;
};
export type AttachmentSendHolder = { schema: "1"; entries: AttachmentSendIntent[] };
export const attachmentSendPrefix = "attachment-sends|";
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const counter = /^(0|[1-9][0-9]{0,18})$/;
const count = (value: unknown, positive = false): value is string => typeof value === "string" && counter.test(value) && BigInt(value) <= 9223372036854775807n && (!positive || value !== "0");
function fail(): never { throw Error("ATTACHMENT_INTENT_INVALID"); }
export const attachmentScope = (namespace: string) => namespace.split("|").slice(0, 4).join("|");
export function attachmentIdentity(value: AttachmentIdentity): AttachmentIdentity {
  const p = value.namespace.split("|");
  if (p.length !== 6 || p[3] !== "personal" || !uuid.test(p[1]) || p[2] !== value.userId || p[4] !== value.restoreEpoch || p[5] !== value.deviceId || !uuid.test(value.userId) || !uuid.test(value.deviceId) || !uuid.test(value.familyId) || !count(value.restoreEpoch, true)) fail();
  try { const origin = new URL(p[0]); if (origin.origin !== p[0] || !["http:", "https:"].includes(origin.protocol)) fail(); } catch { fail(); }
  return { namespace: value.namespace, userId: value.userId, deviceId: value.deviceId, familyId: value.familyId, restoreEpoch: value.restoreEpoch };
}
export function uploadManifest(value: UploadManifest): UploadManifest {
  if (!value || !uuid.test(value.requestId) || !uuid.test(value.userId) || !uuid.test(value.deviceId) || !uuid.test(value.familyId) || !count(value.restoreEpoch, true)
    || typeof value.name !== "string" || !value.name || value.name === "." || value.name === ".." || /[\u0000-\u001f\u007f/\\]/.test(value.name) || new TextEncoder().encode(value.name).length > 255
    || !count(value.size, true) || BigInt(value.size) > BigInt(attachmentScopeBytes) || !/^[0-9a-f]{64}$/.test(value.sha256)) fail();
  return { requestId: value.requestId, userId: value.userId, deviceId: value.deviceId, familyId: value.familyId, restoreEpoch: value.restoreEpoch, name: value.name, size: value.size, sha256: value.sha256 };
}
export function attachmentIntent(value: AttachmentSendIntent): AttachmentSendIntent {
  if (!value || typeof value !== "object") fail();
  const identity = attachmentIdentity(value.identity), manifest = uploadManifest(value.manifest), m = value.message;
  if (value.requestId !== manifest.requestId || manifest.userId !== identity.userId || manifest.deviceId !== identity.deviceId || manifest.familyId !== identity.familyId || manifest.restoreEpoch !== identity.restoreEpoch
    || !m || !uuid.test(m.id) || !uuid.test(m.conversationId) || !uuid.test(m.clientMsgId) || m.clientMsgId !== value.clientMsgId || m.senderId !== identity.userId || m.restoreEpoch !== identity.restoreEpoch || !["image", "audio", "video", "file"].includes(m.type)
    || m.text !== manifest.name || m.pending !== true || m.deleted !== false || typeof m.sendAdmission !== "string" || !m.sendAdmission || m.sendAdmission.length > 16384
    || m.replyToId !== undefined && !uuid.test(m.replyToId) || value.replyContentVersion !== undefined && !count(value.replyContentVersion, true)
    || !!m.replyToId !== (value.replyContentVersion !== undefined)
    || m.originFamilyId !== undefined && m.originFamilyId !== identity.familyId || m.attachmentRequestId !== undefined && m.attachmentRequestId !== value.requestId
    || !["prepared", "unknown", "uploaded", "enqueued", "accepted", "cancelled", "retired"].includes(value.state) || !count(value.revision, true)
    || !Number.isSafeInteger(value.createdAt) || !Number.isSafeInteger(value.updatedAt) || value.createdAt < 0 || value.updatedAt < value.createdAt
    || !Number.isFinite(Date.parse(m.createdAt)) || !Number.isFinite(Date.parse(m.updatedAt))
    || value.preparedId !== undefined && !uuid.test(value.preparedId)
    || value.messageId !== undefined && !uuid.test(value.messageId) || value.blob !== undefined && (!(value.blob instanceof Blob) || value.blob.size !== Number(manifest.size))
    || m.type === "audio" && Number(manifest.size) > attachmentAudioBytes) fail();
  const message: Message = { id: m.id, conversationId: m.conversationId, senderId: m.senderId, clientMsgId: m.clientMsgId, type: m.type, text: m.text, createdAt: m.createdAt, updatedAt: m.updatedAt, version: "1", sequence: "0", deleted: false, reactions: [], pinned: false, favorite: false, pending: true, restoreEpoch: identity.restoreEpoch, sendAdmission: m.sendAdmission };
  message.originFamilyId = identity.familyId; message.attachmentRequestId = value.requestId;
  if (m.replyToId) message.replyToId = m.replyToId;
  if (m.mentionAll === true) message.mentionAll = true;
  if (m.mentionedUserIds !== undefined) { if (!Array.isArray(m.mentionedUserIds) || m.mentionedUserIds.length > 2000 || new Set(m.mentionedUserIds).size !== m.mentionedUserIds.length || m.mentionedUserIds.some(id => !uuid.test(id))) fail(); message.mentionedUserIds = [...m.mentionedUserIds]; }
  let receipt: UploadReceipt | undefined;
  if (value.receipt) {
    const r = value.receipt;
    if (!uuid.test(r.id) || value.preparedId && r.id !== value.preparedId || r.name !== manifest.name || r.sha256 !== manifest.sha256 || r.size !== Number(manifest.size) || typeof r.contentType !== "string" || !/^[a-z0-9.+-]+\/[a-z0-9.+-]+$/i.test(r.contentType) || m.type !== "file" && !r.contentType.startsWith(m.type + "/")) fail();
    receipt = { id: r.id, name: r.name, contentType: r.contentType, size: r.size, sha256: r.sha256 }; message.attachmentId = r.id;
  }
  if (["uploaded", "enqueued"].includes(value.state) && !receipt || ["prepared", "unknown", "uploaded", "enqueued"].includes(value.state) && !value.blob) fail();
  if (value.messageId !== undefined && (value.state !== "accepted" || value.blob !== undefined)) fail();
  if (m.attachmentId !== undefined && m.attachmentId !== receipt?.id) fail();
  return { requestId: value.requestId, clientMsgId: value.clientMsgId, identity, manifest, message, replyContentVersion: value.replyContentVersion, blob: value.blob, state: value.state, revision: value.revision, createdAt: value.createdAt, updatedAt: value.updatedAt, receipt, preparedId: value.preparedId, messageId: value.messageId, failure: typeof value.failure === "string" ? value.failure.slice(0, 80) : undefined };
}
export function attachmentHolder(value: unknown): AttachmentSendHolder {
  if (value === undefined) return { schema: "1", entries: [] };
  const holder = value as AttachmentSendHolder;
  if (holder?.schema !== "1" || !Array.isArray(holder.entries) || holder.entries.length > attachmentScopeLimit) fail();
  const entries = holder.entries.map(attachmentIntent), ids = new Set(entries.map(entry => entry.requestId));
  if (ids.size !== entries.length) fail();
  return { schema: "1", entries };
}
export function attachmentHolderForKey(key: string, value: unknown): AttachmentSendHolder {
  if (!key.startsWith(attachmentSendPrefix)) fail();
  const parts = key.substring(attachmentSendPrefix.length).split("|");
  if (parts.length === 8 && parts[6] === "archive" && count(parts[7], true)) parts.splice(6);
  const namespace = parts.join("|"), holder = attachmentHolder(value);
  if (holder.entries.some(entry => entry.identity.namespace !== namespace)) fail();
  return holder;
}
export function prepareAttachmentHolders(originals: readonly { key: string; holder: AttachmentSendHolder }[], candidate: AttachmentSendIntent): { key: string; holder: AttachmentSendHolder }[] {
  const entry = attachmentIntent(candidate), holders = originals.map(value => ({ key: value.key, holder: attachmentHolderForKey(value.key, value.holder) }));
  if (holders.some(value => attachmentScope(value.key.substring(attachmentSendPrefix.length)) !== attachmentScope(entry.identity.namespace))) fail();
  let count = holders.reduce((sum, value) => sum + value.holder.entries.length, 0);
  const bytes = holders.reduce((sum, value) => sum + value.holder.entries.reduce((sum, item) => sum + (item.blob?.size ?? 0), 0), 0);
  const terminal = holders.flatMap(value => value.holder.entries.filter(item => !item.blob && ["accepted", "cancelled", "retired"].includes(item.state)).map(item => ({ owner: value, item }))).sort((a, b) => a.item.updatedAt - b.item.updatedAt);
  while (count >= attachmentScopeLimit && terminal.length) { const oldest = terminal.shift()!; oldest.owner.holder.entries = oldest.owner.holder.entries.filter(item => item.requestId !== oldest.item.requestId); count--; }
  if (count >= attachmentScopeLimit || bytes + (entry.blob?.size ?? 0) > attachmentScopeBytes) throw Error("ATTACHMENT_QUOTA");
  if (holders.some(value => value.holder.entries.some(item => item.requestId === entry.requestId || item.clientMsgId === entry.clientMsgId))) throw Error("ATTACHMENT_ID_REUSED");
  const key = attachmentSendPrefix + entry.identity.namespace;
  let owner = holders.find(value => value.key === key); if (!owner) { owner = { key, holder: { schema: "1", entries: [] } }; holders.push(owner); }
  owner.holder.entries.push(entry); return holders;
}
export function uploadResult(value: unknown, entry: AttachmentSendIntent): { prepared: true; id: string } | { prepared: false; receipt: UploadReceipt } {
  const r = value as Record<string, unknown>;
  if (r?.protocol !== "stable-upload-v1" || r.requestId !== entry.requestId || typeof r.id !== "string" || !uuid.test(r.id)) fail();
  if (r.uploadState === "prepared") return { prepared: true, id: r.id };
  if (r.uploadState !== "committed" || r.url !== "/api/media/" + r.id) fail();
  const safe = attachmentIntent({ ...entry, receipt: r as unknown as UploadReceipt, state: "uploaded" });
  return { prepared: false, receipt: safe.receipt! };
}
export function terminalAttachment(entry: AttachmentSendIntent, state: "accepted" | "cancelled" | "retired", messageId = entry.messageId): AttachmentSendIntent {
  return { ...entry, state, messageId, blob: undefined, revision: String(BigInt(entry.revision) + 1n), updatedAt: Math.max(Date.now(), entry.updatedAt) };
}
export type ConfirmedAttachmentMessage = { messageId: string; conversationId: string; userId: string };
export function pruneGlobalAttachmentHolder(value: unknown, globals: ReadonlySet<string>, confirmedByClient: ReadonlyMap<string, ConfirmedAttachmentMessage>): AttachmentSendHolder {
  const holder = attachmentHolder(value);
  return { schema: "1", entries: holder.entries.filter(entry => {
    const confirmed = confirmedByClient.get(entry.message.conversationId + "|" + entry.clientMsgId);
    const id = entry.messageId ?? (confirmed?.conversationId === entry.message.conversationId && confirmed.userId === entry.identity.userId && uuid.test(confirmed.messageId) ? confirmed.messageId : undefined);
    return !id || !globals.has(id);
  }) };
}

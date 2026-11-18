// Created by Jobs on 2026年11月17日，星期二.
import { syncCounter, syncID, syncRecord, SyncProtocolError } from "./SyncProtocol";
import type { Message } from "./types";
import type { PasswordChangeIdentity } from "./PasswordChangeProtocol";

export const migrationLimit = 128 * 1024 * 1024;
export type MigrationIdentity = PasswordChangeIdentity & { storeGeneration: string };
export type MigrationSelection = { id: string; conversationId: string; contentVersion: string; type: string; attachmentId?: string; objectMode: "none" | "metadata_only" | "full" };
export type MigrationMessage = { bodyEnvelope: string; sourceReceiptId?: string; objectMode: MigrationSelection["objectMode"] };
export type MigrationObject = { objectId: string; messageId: string; sourceObjectReceiptId?: string; bytes: string };
export type MigrationSourceData = { messages: MigrationMessage[]; objects: MigrationObject[] };
export function migrationFailure(code = "MIGRATION_PROTOCOL_INVALID"): never { throw new SyncProtocolError(code); }
export const migrationPositive = (value: unknown) => { const v = syncCounter(value); if (v === "0") migrationFailure(); return v; };
export const migrationTime = (value: unknown) => { if (typeof value !== "string" || !Number.isFinite(Date.parse(value))) migrationFailure(); return value as string; };
export const migrationDigest = (value: unknown) => { if (typeof value !== "string" || !/^[0-9a-f]{64}$/.test(value)) migrationFailure(); return value as string; };
export function migrationBase64(bytes: Uint8Array, url = false): string {
  let binary = ""; for (let at = 0; at < bytes.length; at += 16384) binary += String.fromCharCode(...bytes.subarray(at, at + 16384));
  const encoded = btoa(binary); return url ? encoded.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "") : encoded;
}
export function migrationDecode(value: unknown, max: number, url = false): Uint8Array<ArrayBuffer> {
  if (typeof value !== "string" || value.length > Math.ceil(max / 3) * 4 || !(url ? /^[A-Za-z0-9_-]*$/ : /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/).test(value)) migrationFailure();
  let bytes: Uint8Array<ArrayBuffer>;
  try { bytes = Uint8Array.from(atob(url ? value.replace(/-/g, "+").replace(/_/g, "/") : value), ch => ch.charCodeAt(0)); } catch { return migrationFailure(); }
  if (bytes.length > max || migrationBase64(bytes, url) !== value) migrationFailure(); return bytes;
}
export async function migrationHash(value: BufferSource): Promise<string> { return [...new Uint8Array(await crypto.subtle.digest("SHA-256", value))].map(byte => byte.toString(16).padStart(2, "0")).join(""); }
export function migrationSelect(message: Message, full: boolean): MigrationSelection {
  if (message.pending || message.frozen || message.deleted || message.contentUnavailable || message.forwarded || message.forwardUnavailable || !["text", "image", "audio", "video", "file"].includes(message.type)) migrationFailure("MIGRATION_SOURCE_UNAVAILABLE");
  const id = syncID(message.id), conversationId = syncID(message.conversationId), contentVersion = migrationPositive(message.contentVersion);
  const attachmentId = message.attachmentId ? syncID(message.attachmentId) : undefined;
  if (message.type === "text" ? !!attachmentId : !attachmentId) migrationFailure();
  return { id, conversationId, contentVersion, type: message.type, ...(attachmentId ? { attachmentId } : {}), objectMode: attachmentId ? full ? "full" : "metadata_only" : "none" };
}
export function migrationEnvelope(raw: unknown, selected: MigrationSelection): MigrationMessage {
  const message = syncRecord(raw);
  if (message.id !== selected.id || message.conversationId !== selected.conversationId || message.contentVersion !== selected.contentVersion || message.type !== selected.type || message.attachmentId !== selected.attachmentId || message.deleted !== false || message.contentUnavailable || message.forwarded || message.forwardUnavailable || message.pending || message.frozen) migrationFailure("MIGRATION_SOURCE_CHANGED");
  syncID(message.senderId); migrationTime(message.createdAt); migrationPositive(message.creationEpoch); migrationPositive(message.creationSequence);
  if (typeof message.text !== "string" || new TextEncoder().encode(message.text).length > 16384) migrationFailure();
  // Freeze only immutable primary fields, so a later reaction cannot change the original request.
  const body = { id: message.id, conversationId: message.conversationId, senderId: message.senderId, type: message.type, text: message.text,
    ...(message.replyToId ? { replyToId: syncID(message.replyToId) } : {}), ...(selected.attachmentId ? { attachmentId: selected.attachmentId } : {}),
    createdAt: message.createdAt, contentVersion: message.contentVersion, creationEpoch: message.creationEpoch, creationSequence: message.creationSequence,
    ...(message.forwarded === true ? { forwarded: true } : {}),
  };
  const bytes = new TextEncoder().encode(JSON.stringify(body)); if (bytes.length > 96 * 1024) migrationFailure();
  return { bodyEnvelope: migrationBase64(bytes, true), objectMode: selected.objectMode };
}
export function migrationSourceBody(data: MigrationSourceData): string {
  if (!data.messages.length || data.messages.length > 256 || data.objects.length > 64) migrationFailure("MIGRATION_SOURCE_LIMIT");
  const body = JSON.stringify(data); if (new TextEncoder().encode(body).length > migrationLimit - 16384) migrationFailure("MIGRATION_SOURCE_LIMIT"); return body;
}

// Created by Jobs on 2026年11月17日，星期二.
import { storageDatabase } from "./StorageDatabase";
import { messagePayload } from "./forward";
import { attachmentHolderForKey, attachmentIdentity, attachmentIntent, attachmentPublicationKey, attachmentScope, attachmentSendPrefix, prepareAttachmentHolders, terminalAttachment, type AttachmentIdentity, type AttachmentSendHolder, type AttachmentSendIntent, type UploadReceipt } from "./AttachmentSendProtocol";
import type { Message } from "./types";
import { guardCompletedErasure } from "./AccountDeletionCleanup";
import { knownGlobalIDs, loadVerifiedGlobals, observeVerifiedGlobals, readGlobalFacts } from "./GlobalMessages";
import { attachmentErasureFloor, attachmentErasureKey, attachmentErasureMatches, attachmentErasureRepairKey, mergeAttachmentErasureFloor } from "./AttachmentErasureProtocol";

export type AttachmentStoreContext = { identity: AttachmentIdentity; isCurrent: () => boolean };
export type AttachmentSendRecord = AttachmentSendIntent & { holderKey: string };
const prefix = attachmentSendPrefix;
const keyFor = (identity: AttachmentIdentity) => prefix + identity.namespace;
const sameIdentity = (a: AttachmentIdentity, b: AttachmentIdentity) => JSON.stringify(attachmentIdentity(a)) === JSON.stringify(attachmentIdentity(b));
const sameOriginalMessage = (a: Message, b: Message) => a.senderId === b.senderId && a.conversationId === b.conversationId && a.originFamilyId === b.originFamilyId && a.attachmentRequestId === b.attachmentRequestId && JSON.stringify(messagePayload(a)) === JSON.stringify(messagePayload(b));
const fail = (): never => { throw Error("ATTACHMENT_CONTEXT_CHANGED"); };
function abortPending(tx: IDBTransaction) {
  try { tx.abort(); }
  catch (error) {
    // A request failure may already have aborted the transaction; keep its original rejection.
    if (!(error instanceof DOMException) || error.name !== "InvalidStateError") throw error;
  }
}
function guards(store: IDBObjectStore, context: AttachmentStoreContext, original?: AttachmentSendIntent) {
  guardCompletedErasure(store, context.identity.namespace);
  const active = store.get("auth-active|" + attachmentScope(context.identity.namespace)), fence = store.get("auth-frozen|" + context.identity.namespace);
  const scope = attachmentScope(context.identity.namespace), erasure = store.get(attachmentErasureKey(scope)), repair = store.get(attachmentErasureRepairKey(scope));
  return () => {
    if (!context.isCurrent() || active.result !== context.identity.namespace || fence.result) fail();
    if (repair.result !== undefined) throw Error("ATTACHMENT_ERASURE_REPAIR_REQUIRED");
    const floor = attachmentErasureFloor(erasure.result, scope);
    if (original && attachmentErasureMatches(floor, original.identity.userId, original.message.conversationId, original.clientMsgId)) throw Error("ATTACHMENT_ORIGINAL_ERASED");
  };
}
function finish<T>(tx: IDBTransaction, value: () => T, isCurrent: () => boolean, failure: () => unknown): Promise<T> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => failure() !== undefined ? reject(failure()) : isCurrent() ? resolve(value()) : reject(Error("ATTACHMENT_CONTEXT_CHANGED"));
    tx.onerror = () => reject(failure() ?? tx.error ?? Error("ATTACHMENT_STORAGE_FAILED"));
    tx.onabort = () => reject(failure() ?? tx.error ?? Error("ATTACHMENT_STORAGE_FAILED"));
  });
}
export async function attachmentIntents(context: AttachmentStoreContext): Promise<AttachmentSendRecord[]> {
  attachmentIdentity(context.identity); if (!context.isCurrent()) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces"), store = tx.objectStore("spaces"), cursor = store.openCursor(), entries: AttachmentSendRecord[] = [];
  let failure: unknown;
  cursor.onsuccess = () => {
    try {
      if (!context.isCurrent()) fail(); const row = cursor.result; if (!row) return;
      if (typeof row.key === "string" && row.key.startsWith(prefix) && attachmentScope(row.key.substring(prefix.length)) === attachmentScope(context.identity.namespace)) entries.push(...attachmentHolderForKey(row.key, row.value).entries.map(entry => ({ ...entry, holderKey: String(row.key) })));
      row.continue();
    } catch (error) { failure = error; abortPending(tx); }
  };
  return finish(tx, () => entries.sort((a, b) => b.createdAt - a.createdAt), context.isCurrent, () => failure);
}
export async function cancelAttachmentLocally(context: AttachmentStoreContext, original: AttachmentSendRecord): Promise<void> {
  attachmentIdentity(context.identity); attachmentIntent(original);
  if (!context.isCurrent() || attachmentScope(original.identity.namespace) !== attachmentScope(context.identity.namespace)) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), check = guards(store, context);
  const queueKey = "outbox|" + original.identity.namespace, queue = store.get(queueKey), read = store.get(original.holderKey); let failure: unknown;
  read.onsuccess = () => {
    try {
      check(); const holder = attachmentHolderForKey(original.holderKey, read.result), index = holder.entries.findIndex(entry => entry.requestId === original.requestId), current = holder.entries[index];
      if (!current || current.revision !== original.revision || !sameIdentity(current.identity, original.identity) || ["accepted", "cancelled"].includes(current.state)) throw Error("ATTACHMENT_INTENT_CHANGED");
      const rows = queue.result ?? [];
      if (!Array.isArray(rows)) throw Error("ATTACHMENT_OUTBOX_INVALID");
      const candidates: Message[] = rows.filter((item: Message) => item.clientMsgId === current.clientMsgId);
      if (candidates.length > 1 || candidates.some(item => !sameOriginalMessage(item, current.message))) throw Error("ATTACHMENT_INTENT_CHANGED");
      if (current.state === "enqueued" && !candidates.length) throw Error("ATTACHMENT_CHECK_ORIGINAL_FIRST");
      if (candidates.length) store.put(rows.map((item: Message) => item.clientMsgId === current.clientMsgId ? { ...item, frozen: true, freezeReason: item.freezeReason ?? "ATTACHMENT_CANCELLED" } : item), queueKey);
      holder.entries[index] = attachmentIntent(terminalAttachment(current, "cancelled")); check(); store.put(holder, original.holderKey);
    } catch (error) { failure = error; abortPending(tx); }
  };
  return finish(tx, () => undefined, context.isCurrent, () => failure);
}
export async function attachmentCleanupPending(context: AttachmentStoreContext): Promise<boolean> {
  attachmentIdentity(context.identity); if (!context.isCurrent()) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces"), request = tx.objectStore("spaces").getAllKeys();
  return finish(tx, () => request.result.some(key => typeof key === "string" && (key.startsWith("attachment-cleanup-pending|") && attachmentScope(key.substring("attachment-cleanup-pending|".length)) === attachmentScope(context.identity.namespace) || key === attachmentErasureRepairKey(attachmentScope(context.identity.namespace)))), context.isCurrent, () => undefined);
}
export async function retryAttachmentCleanup(context: AttachmentStoreContext): Promise<void> {
  if (await attachmentCleanupPending(context)) {
    const ids = await loadVerifiedGlobals(context.identity.namespace);
    if (!context.isCurrent()) fail();
    if (ids.length) await observeVerifiedGlobals(context.identity.namespace, ids, context.isCurrent);
  }
}
export async function prepareAttachmentIntent(context: AttachmentStoreContext, candidate: AttachmentSendIntent): Promise<AttachmentSendIntent> {
  const entry = attachmentIntent(candidate); if (!context.isCurrent() || !sameIdentity(context.identity, entry.identity)) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), check = guards(store, context, entry), cursor = store.openCursor();
  const holders: { key: string; holder: AttachmentSendHolder }[] = []; let failure: unknown;
  cursor.onsuccess = () => {
    try {
      check(); const row = cursor.result;
      if (row) {
        if (typeof row.key === "string" && row.key.startsWith(prefix) && attachmentScope(row.key.substring(prefix.length)) === attachmentScope(entry.identity.namespace)) holders.push({ key: row.key, holder: attachmentHolderForKey(row.key, row.value) });
        row.continue(); return;
      }
      const prepared = prepareAttachmentHolders(holders, entry); check(); for (const value of prepared) store.put(value.holder, value.key);
    } catch (error) { failure = error; abortPending(tx); }
  };
  return finish(tx, () => entry, context.isCurrent, () => failure);
}
export async function updateAttachmentIntent(context: AttachmentStoreContext, original: AttachmentSendIntent, patch: { state?: AttachmentSendIntent["state"]; receipt?: UploadReceipt; preparedId?: string; failure?: string }): Promise<AttachmentSendIntent> {
  if (!context.isCurrent() || !sameIdentity(context.identity, original.identity)) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), check = guards(store, context, original), read = store.get(keyFor(original.identity));
  let failure: unknown, saved = original;
  read.onsuccess = () => {
    try {
      check(); const holder = attachmentHolderForKey(keyFor(original.identity), read.result), index = holder.entries.findIndex(item => item.requestId === original.requestId), current = holder.entries[index];
      if (!current || current.revision !== original.revision || !sameIdentity(current.identity, context.identity) || ["accepted", "cancelled", "retired"].includes(current.state)) throw Error("ATTACHMENT_INTENT_CHANGED");
      saved = attachmentIntent({ ...current, state: patch.state ?? current.state, receipt: patch.receipt ?? current.receipt, preparedId: patch.preparedId ?? current.preparedId, failure: patch.failure, revision: String(BigInt(current.revision) + 1n), updatedAt: Math.max(Date.now(), current.updatedAt) });
      holder.entries[index] = saved; check(); store.put(holder, keyFor(original.identity));
    } catch (error) { failure = error; abortPending(tx); }
  };
  return finish(tx, () => saved, context.isCurrent, () => failure);
}
export function linkAttachmentEnqueue(context: AttachmentStoreContext, original: AttachmentSendIntent, message: Message): (store: IDBObjectStore) => void {
  return store => {
    const check = guards(store, context, original), queue = store.get("outbox|" + original.identity.namespace), read = store.get(keyFor(original.identity));
    read.onsuccess = () => {
      try {
        check(); const holder = attachmentHolderForKey(keyFor(original.identity), read.result), index = holder.entries.findIndex(item => item.requestId === original.requestId), current = holder.entries[index];
        if (!current || current.revision !== original.revision || !sameIdentity(current.identity, context.identity) || current.state !== "uploaded" || !current.receipt || !sameOriginalMessage(current.message, message)) throw Error("ATTACHMENT_INTENT_CHANGED");
        if (queue.result !== undefined && !Array.isArray(queue.result)) throw Error("ATTACHMENT_OUTBOX_INVALID");
        const existing = (queue.result as Message[] | undefined)?.filter(item => item.clientMsgId === current.clientMsgId) ?? [];
        if (existing.length > 1 || existing.some(item => item.frozen || !sameOriginalMessage(item, message))) throw Error("ATTACHMENT_INTENT_CHANGED");
        holder.entries[index] = attachmentIntent({ ...current, state: "enqueued", revision: String(BigInt(current.revision) + 1n), updatedAt: Math.max(Date.now(), current.updatedAt) });
        check(); store.put(holder, keyFor(original.identity));
      } catch { abortPending(store.transaction); }
    };
  };
}
async function assertAttachmentPublication(context: AttachmentStoreContext, message: Message): Promise<void> {
  if (!context.isCurrent()) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces"), store = tx.objectStore("spaces"), active = store.get("auth-active|" + attachmentScope(context.identity.namespace)), fence = store.get("auth-frozen|" + context.identity.namespace), erasure = store.get(attachmentErasureKey(attachmentScope(context.identity.namespace))), repair = store.get(attachmentErasureRepairKey(attachmentScope(context.identity.namespace))), read = store.get(keyFor(context.identity)), queue = store.get("outbox|" + context.identity.namespace);
  let failure: unknown;
  queue.onsuccess = () => {
    try {
      const entry = attachmentHolderForKey(keyFor(context.identity), read.result).entries.find(value => value.requestId === message.attachmentRequestId);
      if (queue.result !== undefined && !Array.isArray(queue.result)) throw Error("ATTACHMENT_OUTBOX_INVALID");
      const candidates = (queue.result as Message[] | undefined)?.filter(value => value.clientMsgId === message.clientMsgId) ?? [], original = candidates[0];
      const floor = attachmentErasureFloor(erasure.result, attachmentScope(context.identity.namespace));
      if (repair.result !== undefined) throw Error("ATTACHMENT_ERASURE_REPAIR_REQUIRED");
      if (!context.isCurrent() || active.result !== context.identity.namespace || fence.result || !entry || entry.state !== "enqueued" || !sameIdentity(entry.identity, context.identity) || candidates.length !== 1 || !original || original.frozen || original.originFamilyId !== context.identity.familyId || !sameOriginalMessage(original, entry.message) || !sameOriginalMessage(message, entry.message)) throw Error("ATTACHMENT_INTENT_CHANGED");
      if (attachmentErasureMatches(floor, entry.identity.userId, entry.message.conversationId, entry.clientMsgId)) throw Error("ATTACHMENT_ORIGINAL_ERASED");
    } catch (error) { failure = error; abortPending(tx); }
  };
  return finish(tx, () => undefined, context.isCurrent, () => failure);
}
export async function withAttachmentPublication<T>(context: AttachmentStoreContext, message: Message, publish: () => Promise<T>): Promise<T> {
  if (!navigator.locks) throw Error("ATTACHMENT_LOCK_UNAVAILABLE");
  return navigator.locks.request(attachmentPublicationKey(context.identity.namespace, message.conversationId, message.clientMsgId), async () => {
    await assertAttachmentPublication(context, message);
    if (!context.isCurrent()) fail();
    return publish();
  });
}
export async function acceptAttachmentMessage(context: AttachmentStoreContext, message: Message): Promise<void> {
  if (!context.isCurrent()) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), check = guards(store, context), facts = readGlobalFacts(store, context.identity.namespace), erasure = store.get(attachmentErasureKey(attachmentScope(context.identity.namespace))), read = store.get(keyFor(context.identity));
  let failure: unknown;
  read.onsuccess = () => {
    try {
      check(); const holder = attachmentHolderForKey(keyFor(context.identity), read.result), index = holder.entries.findIndex(item => item.clientMsgId === message.clientMsgId), current = holder.entries[index];
      if (!current) return;
      if (!sameIdentity(current.identity, context.identity) || !["enqueued", "cancelled"].includes(current.state) || message.senderId !== current.identity.userId || message.conversationId !== current.message.conversationId || !message.id || message.pending || !message.deleted && message.attachmentId !== current.receipt?.id) throw Error("ATTACHMENT_MESSAGE_BINDING_INVALID");
      if (knownGlobalIDs(context.identity.namespace, facts?.result).has(message.id)) {
        store.put(mergeAttachmentErasureFloor(erasure.result, attachmentScope(context.identity.namespace), [{ userId: current.identity.userId, conversationId: current.message.conversationId, clientMsgId: current.clientMsgId, serverId: message.id }]), attachmentErasureKey(attachmentScope(context.identity.namespace)));
        holder.entries.splice(index, 1);
      }
      else holder.entries[index] = attachmentIntent(terminalAttachment(current, "accepted", message.id));
      check(); store.put(holder, keyFor(context.identity));
    } catch (error) { failure = error; abortPending(tx); }
  };
  return finish(tx, () => undefined, context.isCurrent, () => failure);
}

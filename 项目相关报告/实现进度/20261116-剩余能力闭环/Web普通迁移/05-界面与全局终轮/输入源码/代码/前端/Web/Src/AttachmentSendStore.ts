// Created by Jobs on 2026年11月17日，星期二.
import { storageDatabase } from "./StorageDatabase";
import { messagePayload } from "./forward";
import { attachmentHolder, attachmentIdentity, attachmentIntent, attachmentScope, attachmentScopeBytes, attachmentScopeLimit, terminalAttachment, type AttachmentIdentity, type AttachmentSendHolder, type AttachmentSendIntent, type UploadReceipt } from "./AttachmentSendProtocol";
import type { Message } from "./types";
import { guardCompletedErasure } from "./AccountDeletionCleanup";

export type AttachmentStoreContext = { identity: AttachmentIdentity; isCurrent: () => boolean };
const prefix = "attachment-sends|";
const keyFor = (identity: AttachmentIdentity) => prefix + identity.namespace;
const sameIdentity = (a: AttachmentIdentity, b: AttachmentIdentity) => JSON.stringify(attachmentIdentity(a)) === JSON.stringify(attachmentIdentity(b));
const fail = (): never => { throw Error("ATTACHMENT_CONTEXT_CHANGED"); };
function guards(store: IDBObjectStore, context: AttachmentStoreContext) {
  guardCompletedErasure(store, context.identity.namespace);
  const active = store.get("auth-active|" + attachmentScope(context.identity.namespace)), fence = store.get("auth-frozen|" + context.identity.namespace);
  return () => { if (!context.isCurrent() || active.result !== context.identity.namespace || fence.result) fail(); };
}
function finish<T>(tx: IDBTransaction, value: () => T, isCurrent: () => boolean, failure: () => unknown): Promise<T> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => isCurrent() ? resolve(value()) : reject(Error("ATTACHMENT_CONTEXT_CHANGED"));
    tx.onerror = () => reject(failure() ?? tx.error ?? Error("ATTACHMENT_STORAGE_FAILED"));
    tx.onabort = () => reject(failure() ?? tx.error ?? Error("ATTACHMENT_STORAGE_FAILED"));
  });
}
export async function attachmentIntents(context: AttachmentStoreContext): Promise<AttachmentSendIntent[]> {
  attachmentIdentity(context.identity); if (!context.isCurrent()) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces"), store = tx.objectStore("spaces"), cursor = store.openCursor(), entries: AttachmentSendIntent[] = [];
  let failure: unknown;
  cursor.onsuccess = () => {
    try {
      if (!context.isCurrent()) fail(); const row = cursor.result; if (!row) return;
      if (typeof row.key === "string" && row.key.startsWith(prefix) && attachmentScope(row.key.substring(prefix.length)) === attachmentScope(context.identity.namespace)) entries.push(...attachmentHolder(row.value).entries);
      row.continue();
    } catch (error) { failure = error; tx.abort(); }
  };
  return finish(tx, () => entries.sort((a, b) => b.createdAt - a.createdAt), context.isCurrent, () => failure);
}
export async function prepareAttachmentIntent(context: AttachmentStoreContext, candidate: AttachmentSendIntent): Promise<AttachmentSendIntent> {
  const entry = attachmentIntent(candidate); if (!context.isCurrent() || !sameIdentity(context.identity, entry.identity)) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), check = guards(store, context), cursor = store.openCursor();
  const holders: { key: string; holder: AttachmentSendHolder }[] = []; let failure: unknown;
  cursor.onsuccess = () => {
    try {
      check(); const row = cursor.result;
      if (row) {
        if (typeof row.key === "string" && row.key.startsWith(prefix) && attachmentScope(row.key.substring(prefix.length)) === attachmentScope(entry.identity.namespace)) holders.push({ key: row.key, holder: attachmentHolder(row.value) });
        row.continue(); return;
      }
      let count = holders.reduce((sum, value) => sum + value.holder.entries.length, 0), bytes = holders.reduce((sum, value) => sum + value.holder.entries.reduce((sum, item) => sum + (item.blob?.size ?? 0), 0), 0);
      const terminal = holders.flatMap(value => value.holder.entries.filter(item => !item.blob && ["accepted", "cancelled", "retired"].includes(item.state)).map(item => ({ owner: value, item }))).sort((a, b) => a.item.updatedAt - b.item.updatedAt);
      while (count >= attachmentScopeLimit && terminal.length) { const oldest = terminal.shift()!; oldest.owner.holder.entries = oldest.owner.holder.entries.filter(item => item.requestId !== oldest.item.requestId); count--; }
      if (count >= attachmentScopeLimit || bytes + (entry.blob?.size ?? 0) > attachmentScopeBytes) throw Error("ATTACHMENT_QUOTA");
      if (holders.some(value => value.holder.entries.some(item => item.requestId === entry.requestId || item.clientMsgId === entry.clientMsgId))) throw Error("ATTACHMENT_ID_REUSED");
      let owner = holders.find(value => value.key === keyFor(entry.identity)); if (!owner) { owner = { key: keyFor(entry.identity), holder: { schema: "1", entries: [] } }; holders.push(owner); }
      owner.holder.entries.push(entry); check(); for (const value of holders) store.put(value.holder, value.key);
    } catch (error) { failure = error; tx.abort(); }
  };
  return finish(tx, () => entry, context.isCurrent, () => failure);
}
export async function updateAttachmentIntent(context: AttachmentStoreContext, original: AttachmentSendIntent, patch: { state?: AttachmentSendIntent["state"]; receipt?: UploadReceipt; preparedId?: string; failure?: string; cancel?: boolean }): Promise<AttachmentSendIntent> {
  if (!context.isCurrent() || !sameIdentity(context.identity, original.identity)) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), check = guards(store, context), read = store.get(keyFor(original.identity));
  let failure: unknown, saved = original;
  read.onsuccess = () => {
    try {
      check(); const holder = attachmentHolder(read.result), index = holder.entries.findIndex(item => item.requestId === original.requestId), current = holder.entries[index];
      if (!current || current.revision !== original.revision || !sameIdentity(current.identity, context.identity) || ["accepted", "cancelled"].includes(current.state) || current.state === "retired" && !patch.cancel) throw Error("ATTACHMENT_INTENT_CHANGED");
      if (patch.cancel && current.state === "enqueued") throw Error("ATTACHMENT_ALREADY_ENQUEUED");
      saved = attachmentIntent(patch.cancel ? terminalAttachment(current, "cancelled") : { ...current, state: patch.state ?? current.state, receipt: patch.receipt ?? current.receipt, preparedId: patch.preparedId ?? current.preparedId, failure: patch.failure, revision: String(BigInt(current.revision) + 1n), updatedAt: Math.max(Date.now(), current.updatedAt) });
      holder.entries[index] = saved; check(); store.put(holder, keyFor(original.identity));
    } catch (error) { failure = error; tx.abort(); }
  };
  return finish(tx, () => saved, context.isCurrent, () => failure);
}
export function linkAttachmentEnqueue(context: AttachmentStoreContext, original: AttachmentSendIntent, message: Message): (store: IDBObjectStore) => void {
  return store => {
    const check = guards(store, context), queue = store.get("outbox|" + original.identity.namespace), read = store.get(keyFor(original.identity));
    read.onsuccess = () => {
      try {
        check(); const holder = attachmentHolder(read.result), index = holder.entries.findIndex(item => item.requestId === original.requestId), current = holder.entries[index];
        if (!current || current.revision !== original.revision || !sameIdentity(current.identity, context.identity) || current.state !== "uploaded" || !current.receipt || JSON.stringify(messagePayload(current.message)) !== JSON.stringify(messagePayload(message))) throw Error("ATTACHMENT_INTENT_CHANGED");
        const existing = (queue.result as Message[] | undefined)?.find(item => item.clientMsgId === current.clientMsgId);
        if (existing && (existing.frozen || JSON.stringify(messagePayload(existing)) !== JSON.stringify(messagePayload(message)))) throw Error("ATTACHMENT_INTENT_CHANGED");
        holder.entries[index] = attachmentIntent({ ...current, state: "enqueued", revision: String(BigInt(current.revision) + 1n), updatedAt: Math.max(Date.now(), current.updatedAt) });
        check(); store.put(holder, keyFor(original.identity));
      } catch { store.transaction.abort(); }
    };
  };
}
export async function acceptAttachmentMessage(context: AttachmentStoreContext, message: Message): Promise<void> {
  if (!context.isCurrent()) fail();
  const db = await storageDatabase; if (!context.isCurrent()) fail();
  const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), check = guards(store, context), read = store.get(keyFor(context.identity));
  let failure: unknown;
  read.onsuccess = () => {
    try {
      check(); const holder = attachmentHolder(read.result), index = holder.entries.findIndex(item => item.clientMsgId === message.clientMsgId), current = holder.entries[index];
      if (!current) return;
      if (current.state !== "enqueued" || message.senderId !== current.identity.userId || message.conversationId !== current.message.conversationId || !message.id || message.pending || !message.deleted && message.attachmentId !== current.receipt?.id) throw Error("ATTACHMENT_MESSAGE_BINDING_INVALID");
      holder.entries[index] = attachmentIntent(terminalAttachment(current, "accepted", message.id)); check(); store.put(holder, keyFor(context.identity));
    } catch (error) { failure = error; tx.abort(); }
  };
  return finish(tx, () => undefined, context.isCurrent, () => failure);
}

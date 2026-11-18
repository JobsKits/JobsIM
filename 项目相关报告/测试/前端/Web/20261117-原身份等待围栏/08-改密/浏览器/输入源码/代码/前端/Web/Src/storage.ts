import { messagePayload, unavailableForward } from "./forward";
import { emptySpace, type Space, type Message } from "./types";
import { applyGlobalMessages, knownGlobalIDs, readGlobalFacts } from "./GlobalMessages";
import { storageDatabase } from "./StorageDatabase";
export { storageDatabase } from "./StorageDatabase";
const dbPromise = storageDatabase;
export function persistableSpace(value: Space): Space {
  const stripReplyProof = (message: Message) => {
    const { replyAdmission: _proof, ...safe } = message as Message & { replyAdmission?: unknown };
    return safe;
  };
  return applyGlobalMessages({ ...value,
    messages: Object.fromEntries(Object.entries(value.messages).map(([id, rows]) => [id, rows.map(stripReplyProof)])),
    outbox: value.outbox.map(stripReplyProof),
  }, new Set());
}
const queueKey = (key: string) => "outbox|" + key;
const authFenceKey = (key: string) => "auth-frozen|" + key;
const activeScopeKey = (key: string) => {
  const parts = key.split("|");
  return [5, 6].includes(parts.length) ? "auth-active|" + parts.slice(0, 4).join("|") : "";
};

export async function isolateAuthNamespace(scope: string, current: string) {
  const db = await dbPromise;
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces"), keys = store.getAllKeys();
    keys.onsuccess = () => {
      const namespaces = new Set(keys.result.filter((key): key is string => typeof key === "string")
        .map(key => key.startsWith("outbox|") ? key.substring(7) : key)
        .filter(key => key !== current && key.startsWith(scope + "|") && [5, 6].includes(key.split("|").length)));
      for (const key of namespaces) {
        store.put("AUTH_SESSION_CHANGED", authFenceKey(key));
        const read = store.get(queueKey(key));
        const freeze = (rows: Message[]) => store.put(rows.map(message => ({ ...message, frozen: true, freezeReason: message.freezeReason ?? "AUTH_SESSION_CHANGED" })), queueKey(key));
        read.onsuccess = () => {
          if (read.result !== undefined) freeze(read.result);
          else {
            const legacy = store.get(key);
            legacy.onsuccess = () => freeze(legacy.result?.outbox ?? []);
          }
        };
      }
      store.put(current, "auth-active|" + scope);
    };
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

export async function loadSpace(key: string): Promise<Space> {
  const db = await dbPromise;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces");
    const store = tx.objectStore("spaces");
    const facts = readGlobalFacts(store, key);
    const view = store.get(key);
    const queue = store.get(queueKey(key));
    tx.oncomplete = () => {
      const space: Space = view.result ?? emptySpace();
      const outbox: Message[] = queue.result ?? space.outbox;
      const pending = new Set(
        outbox.filter((m) => !m.frozen).map((m) => m.clientMsgId),
      );
      const messages = Object.fromEntries(
        Object.entries(space.messages).map(([id, rows]) => [
          id,
          rows
            .filter((m) => !m.pending || pending.has(m.clientMsgId))
            .map((m) =>
              m.forwarded && !m.pending ? unavailableForward(m) : m,
            ),
        ]),
      );
      for (const message of outbox) {
        if (message.frozen) continue;
        const rows = messages[message.conversationId] ?? [];
        if (!rows.some((m) => m.pending && m.senderId === message.senderId && m.conversationId === message.conversationId && m.clientMsgId === message.clientMsgId))
          messages[message.conversationId] = [...rows, message];
      }
      try { resolve(applyGlobalMessages({ ...space, messages, outbox }, knownGlobalIDs(key, facts?.result))); }
      catch (error) { reject(error); }
    };
    tx.onerror = () => reject(tx.error);
  });
}

export async function freezeConversation(
  key: string,
  conversationId: string,
  value: Space,
) {
  const db = await dbPromise;
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }),
      store = tx.objectStore("spaces");
    const facts = readGlobalFacts(store, key);
    const read = store.get(queueKey(key));
    read.onsuccess = () => {
      const queue: Message[] = (read.result ?? value.outbox).map(
        (m: Message) =>
          m.conversationId === conversationId ? { ...m, frozen: true } : m,
      );
      store.put(queue, queueKey(key));
      store.put(applyGlobalMessages({ ...value, outbox: queue }, knownGlobalIDs(key, facts?.result)), key);
    };
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

export async function freezeNamespace(
  key: string,
  value: Space,
  previousEpoch: string,
  reason = "RESTORE_EPOCH_CHANGED",
): Promise<Space> {
  const db = await dbPromise;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces");
    const facts = readGlobalFacts(store, key);
    const read = store.get(queueKey(key));
    let result: Space;
    read.onsuccess = () => {
      const queue: Message[] = (read.result ?? value.outbox).map(
        (message: Message) => ({
          ...message,
          restoreEpoch: message.restoreEpoch ?? previousEpoch,
          frozen: true,
          freezeReason: reason,
        }),
      );
      result = applyGlobalMessages({ ...value, outbox: queue }, knownGlobalIDs(key, facts?.result));
      store.put(queue, queueKey(key));
      store.put(persistableSpace(result), key);
    };
    tx.oncomplete = () => resolve(result);
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

async function transaction(
  key: string,
  value: Space,
  mode: "view" | "replace" | "enqueue",
  message?: Message,
) {
  const db = await dbPromise;
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces");
    const facts = readGlobalFacts(store, key);
    const scope = activeScopeKey(key);
    const active = scope ? store.get(scope) : undefined;
    const fence = store.get(authFenceKey(key));
    const guarded = (rows: Message[]) => {
      const reason = fence.result || (active?.result && active.result !== key ? "AUTH_SESSION_CHANGED" : "");
      if (!reason) return rows;
      store.put(reason, authFenceKey(key));
      return rows.map(row => ({ ...row, frozen: true, freezeReason: row.freezeReason ?? reason }));
    };
    const view = store.get(key);
    view.onsuccess = () => {
      const current: Space | undefined = view.result;
      // A delayed React save may update drafts, but cannot roll back a committed sync projection.
      const next = current?.syncRevision && current.syncRevision !== value.syncRevision
        ? { ...current, drafts: value.drafts, mentionDrafts: value.mentionDrafts }
        : value;
      store.put(persistableSpace(applyGlobalMessages(next, knownGlobalIDs(key, facts?.result))), key);
    };
    if (mode === "replace") fence.onsuccess = () => store.put(guarded(value.outbox), queueKey(key));
    if (mode === "enqueue" && message) {
      const read = store.get(queueKey(key));
      read.onsuccess = () => {
        const existing: Message[] = read.result ?? value.outbox;
        store.put(
          guarded([
            ...existing.filter((m) => m.clientMsgId !== message.clientMsgId),
            message,
          ]),
          queueKey(key),
        );
      };
    }
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

export const saveSpace = (key: string, value: Space) =>
  transaction(key, value, "replace");
// View snapshots cannot erase a queue committed while React is rendering.
export const saveView = (key: string, value: Space) =>
  transaction(key, value, "view");
export const queueMessage = (key: string, value: Space, message: Message) =>
  transaction(key, value, "enqueue", message);

export async function acknowledgeOriginalOutbox(
  key: string,
  principalId: string,
  proofs: readonly { clientMsgId: string; conversationId: string; originalBody: unknown }[],
  isCurrent: () => boolean,
): Promise<string[]> {
  if (!proofs.length) return [];
  const db = await dbPromise;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces");
    const scope = activeScopeKey(key), active = scope ? store.get(scope) : undefined;
    const fence = store.get(authFenceKey(key)), view = store.get(key), read = store.get(queueKey(key));
    const removed: string[] = [];
    let failure: unknown;
    read.onsuccess = () => {
      try {
        if (!isCurrent() || key.split("|")[2] !== principalId || fence.result || (key.split("|").length === 6 ? active?.result !== key : active?.result && active.result !== key)) throw new DOMException("Authentication context changed", "AbortError");
        const queue: Message[] = read.result ?? view.result?.outbox ?? [];
        const next = queue.filter(message => {
          const confirmed = message.senderId === principalId && proofs.some(proof => message.clientMsgId === proof.clientMsgId && message.conversationId === proof.conversationId && JSON.stringify(messagePayload(message)) === JSON.stringify(proof.originalBody));
          if (confirmed) removed.push(message.clientMsgId);
          return !confirmed;
        });
        store.put(next, queueKey(key));
      } catch (error) { failure = error; tx.abort(); }
    };
    tx.oncomplete = () => isCurrent() ? resolve(removed) : reject(new DOMException("Authentication context changed", "AbortError"));
    tx.onerror = () => reject(failure ?? tx.error);
    tx.onabort = () => reject(failure ?? tx.error ?? new DOMException("Original Outbox changed", "AbortError"));
  });
}

export async function prepareOutboxAttempt(
  key: string,
  clientMsgId: string,
): Promise<Message | null> {
  const db = await dbPromise;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces");
    const scope = activeScopeKey(key);
    const active = scope ? store.get(scope) : undefined;
    const fence = store.get(authFenceKey(key));
    const read = store.get(queueKey(key));
    let original: Message | null = null;
    const prepare = (queue: Message[]) => {
      const reason = fence.result || (active?.result && active.result !== key ? "AUTH_SESSION_CHANGED" : "");
      if (reason) {
        store.put(reason, authFenceKey(key));
        store.put(queue.map(message => ({ ...message, frozen: true, freezeReason: message.freezeReason ?? reason })), queueKey(key));
        return;
      }
      const found = queue.find(
        (message) => message.clientMsgId === clientMsgId,
      );
      if (found && !found.frozen) {
        original = found;
        store.put(
          queue.map((message) =>
            message.clientMsgId === clientMsgId
              ? { ...message, sendAttempted: true }
              : message,
          ),
          queueKey(key),
        );
      }
    };
    read.onsuccess = () => {
      if (read.result !== undefined) prepare(read.result);
      else {
        const legacy = store.get(key);
        legacy.onsuccess = () => prepare(legacy.result?.outbox ?? []);
      }
    };
    tx.oncomplete = () => resolve(original);
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

export async function acknowledgeOutbox(key: string, ids: string[]) {
  if (!ids.length) return;
  const db = await dbPromise;
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces");
    const read = store.get(queueKey(key));
    read.onsuccess = () => {
      const filter = (rows: Message[]) =>
        rows.filter((m) => !ids.includes(m.clientMsgId));
      if (read.result !== undefined)
        store.put(filter(read.result), queueKey(key));
      else {
        const legacy = store.get(key);
        legacy.onsuccess = () =>
          store.put(filter(legacy.result?.outbox ?? []), queueKey(key));
      }
    };
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

export async function freezeOutboxMessage(
  key: string,
  clientMsgId: string,
  reason = "SEND_ADMISSION_CHANGED",
) {
  const db = await dbPromise;
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces");
    const read = store.get(queueKey(key));
    read.onsuccess = () => {
      const freeze = (rows: Message[]) =>
        rows.map((message) =>
          message.clientMsgId === clientMsgId
            ? {
                ...message,
                frozen: true,
                freezeReason: reason,
              }
            : message,
        );
      if (read.result !== undefined)
        store.put(freeze(read.result), queueKey(key));
      else {
        const legacy = store.get(key);
        legacy.onsuccess = () =>
          store.put(freeze(legacy.result?.outbox ?? []), queueKey(key));
      }
    };
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

export async function saveIntent<T>(key: string, purpose: string, value: T) {
  const db = await dbPromise;
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    tx.objectStore("spaces").put(value, "operation|" + key + "|" + purpose);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}
export async function loadIntent<T>(
  key: string,
  purpose: string,
): Promise<T | undefined> {
  const db = await dbPromise;
  return new Promise((resolve, reject) => {
    const read = db
      .transaction("spaces")
      .objectStore("spaces")
      .get("operation|" + key + "|" + purpose);
    read.onsuccess = () => resolve(read.result);
    read.onerror = () => reject(read.error);
  });
}
export async function clearIntent(
  key: string,
  purpose: string,
  requestId?: string,
) {
  const db = await dbPromise;
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
    const store = tx.objectStore("spaces"),
      record = "operation|" + key + "|" + purpose;
    if (requestId) {
      const read = store.get(record);
      read.onsuccess = () => {
        if (read.result?.requestId === requestId) store.delete(record);
      };
    } else store.delete(record);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

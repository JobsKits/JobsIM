import { storageDatabase } from "./StorageDatabase";
import { unavailableForward } from "./forward";
import { SyncProtocolError } from "./SyncProtocol";
import type { Message, Space } from "./types";

type Namespace = { value: string; scope: string; modern: boolean };
type GlobalFacts = { schema: "1"; scope: string; generation: string; ids: string[] };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const counter = /^[1-9][0-9]{0,18}$/;
const epoch = (value: string) => counter.test(value) && BigInt(value) <= 9223372036854775807n;
const factsKey = (scope: string) => "global-facts|" + scope;

function personalNamespace(value: string, archives = false): Namespace | undefined {
  let parts = value.split("|");
  if (archives && [7, 8].includes(parts.length) && parts.at(-2) === "archive" && epoch(parts.at(-1)!))
    parts = parts.slice(0, -2);
  if (![5, 6].includes(parts.length) || parts[3] !== "personal" || !uuid.test(parts[1]) || !uuid.test(parts[2])) return;
  try {
    const origin = new URL(parts[0]);
    if (!["http:", "https:"].includes(origin.protocol) || origin.origin !== parts[0]) return;
  } catch { return; }
  if (parts.length === 6 ? !epoch(parts[4]) || !uuid.test(parts[5]) : !uuid.test(parts[4])) return;
  return { value: parts.join("|"), scope: parts.slice(0, 4).join("|"), modern: parts.length === 6 };
}

export function readGlobalFacts(store: IDBObjectStore, namespace: string): IDBRequest | undefined {
  const parsed = personalNamespace(namespace, true);
  return parsed ? store.get(factsKey(parsed.scope)) : undefined;
}

export function knownGlobalIDs(namespace: string, value: unknown): Set<string> {
  const parsed = personalNamespace(namespace, true);
  if (!parsed || value === undefined) return new Set();
  const facts = value as GlobalFacts;
  if (facts?.schema !== "1" || facts.scope !== parsed.scope || !uuid.test(facts.generation) ||
      !Array.isArray(facts.ids) || facts.ids.some(id => typeof id !== "string" || !uuid.test(id)) || new Set(facts.ids).size !== facts.ids.length)
    throw new SyncProtocolError("GLOBAL_FACTS_INVALID");
  return new Set(facts.ids);
}

export function globalMessagePlaceholder(message: Message): Message {
  return {
    id: message.id, conversationId: message.conversationId, senderId: message.senderId,
    clientMsgId: message.clientMsgId, type: message.type, createdAt: message.createdAt,
    updatedAt: message.updatedAt, version: message.version, contentVersion: message.contentVersion,
    sequence: message.sequence, text: "", deleted: true, reactions: [], pinned: false, favorite: false,
  };
}

export function applyGlobalMessages(value: Space, ids: ReadonlySet<string>, hideForwards = true): Space {
  const filter = (counts: Space["unread"]) => counts && Object.fromEntries(Object.entries(counts).map(([id, rows]) => [id, rows.filter(row => !ids.has(row))]));
  return {
    ...value,
    messages: Object.fromEntries(Object.entries(value.messages).map(([id, rows]) => [id, rows.map(message => {
      if (message.pending) return message;
      if (ids.has(message.id)) return globalMessagePlaceholder(message);
      if (!hideForwards || !message.forwarded) return message;
      return { ...unavailableForward(message), forwardOfId: undefined, forwardSourceVersion: undefined };
    })])),
    unread: filter(value.unread), mentionUnread: filter(value.mentionUnread),
  };
}

export function applyKnownGlobals(value: Space, ids: Iterable<string>): Space {
  return applyGlobalMessages(value, new Set(ids), false);
}

export async function loadVerifiedGlobals(namespace: string): Promise<string[]> {
  if (!personalNamespace(namespace)) throw new SyncProtocolError("GLOBAL_CONTEXT_INVALID");
  const db = await storageDatabase;
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces"), read = readGlobalFacts(tx.objectStore("spaces"), namespace)!;
    tx.oncomplete = () => {
      try { resolve([...knownGlobalIDs(namespace, read.result)]); }
      catch (error) { reject(error); }
    };
    tx.onerror = () => reject(tx.error ?? new SyncProtocolError("GLOBAL_STORAGE_FAILED"));
    tx.onabort = () => reject(tx.error ?? new SyncProtocolError("GLOBAL_STORAGE_FAILED"));
  });
}

function holder(key: IDBValidKey): { namespace: Namespace; kind: "view" | "state" | "stage" | "migration" } | undefined {
  if (typeof key !== "string") return;
  for (const [prefix, kind] of [["sync-retained|", "view"], ["sync-state|", "state"], ["sync-stage|", "stage"], ["migration-history|", "migration"]] as const) {
    if (!key.startsWith(prefix)) continue;
    const raw = key.substring(prefix.length);
    const namespace = personalNamespace(kind === "stage" ? raw.substring(0, raw.lastIndexOf("|")) : raw, kind === "view");
    return namespace ? { namespace, kind } : undefined;
  }
  const namespace = personalNamespace(key, true);
  return namespace ? { namespace, kind: "view" } : undefined;
}

function projectionMetadata(value: unknown, ids: ReadonlySet<string>): unknown {
  if (!value || typeof value !== "object") return value;
  const item = value as Record<string, unknown>, ref = item.ref as { kind?: string; id?: string } | undefined;
  const body = item.object as { forwarded?: boolean } | undefined;
  if (!(ref?.kind === "message" && ref.id && ids.has(ref.id)) && !body?.forwarded) return value;
  const { object: _body, bodyEnvelope: _envelope, ...metadata } = item;
  return ref?.kind === "message" && ref.id && ids.has(ref.id) ? { ...metadata, state: "global" } : metadata;
}

// Called inside the caller's existing strict spaces transaction, after complete DTO validation.
export function writeVerifiedGlobals(
  store: IDBObjectStore,
  namespace: string,
  ids: ReadonlySet<string>,
  previouslyKnown: ReadonlySet<string>,
  isCurrent: () => boolean,
): void {
  if (!ids.size) return;
  const parsed = personalNamespace(namespace);
  if (!parsed || [...ids].some(id => !uuid.test(id))) throw new SyncProtocolError("GLOBAL_CONTEXT_INVALID");
  if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
  const all = new Set([...previouslyKnown, ...ids]);
  const advanced = [...ids].some(id => !previouslyKnown.has(id));
  if (advanced) store.put({ schema: "1", scope: parsed.scope, generation: crypto.randomUUID(), ids: [...all] } satisfies GlobalFacts, factsKey(parsed.scope));
  const cursor = store.openCursor();
  cursor.onsuccess = () => {
    try {
      if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
      const row = cursor.result;
      if (!row) return;
      const identified = holder(row.key);
      if (identified?.namespace.scope === parsed.scope && row.value && typeof row.value === "object") {
        if (identified.kind === "view" && row.value.messages && typeof row.value.messages === "object") {
          row.update(applyGlobalMessages(row.value as Space, all));
        } else if (identified.kind === "state" && row.value.schema === "1" && row.value.projections) {
          row.update({ ...row.value,
            ...(advanced ? { guardGeneration: crypto.randomUUID() } : {}),
            projections: Object.fromEntries(Object.entries(row.value.projections).map(([key, value]) => [key, projectionMetadata(value, all)])),
          });
        } else if (identified.kind === "stage" && row.value.pages) {
          row.update({ ...row.value, pages: Object.fromEntries(Object.entries(row.value.pages).map(([key, value]) => {
            const page = value as { projections?: unknown[] };
            return [key, { ...page, projections: page.projections?.map(value => projectionMetadata(value, all)) }];
          })) });
        } else if (identified.kind === "migration" && row.value.schema === "1" && Array.isArray(row.value.entries)) {
          row.update({ ...row.value, entries: row.value.entries.filter((entry: { message?: { id?: string } }) => !entry.message?.id || !all.has(entry.message.id)) });
        }
      }
      row.continue();
    } catch { store.transaction.abort(); }
  };
}

export async function observeVerifiedGlobals(namespace: string, ids: readonly string[], isCurrent: () => boolean): Promise<{ scope: string; ids: string[] }> {
  const parsed = personalNamespace(namespace);
  if (!parsed || ids.some(id => !uuid.test(id))) throw new SyncProtocolError("GLOBAL_CONTEXT_INVALID");
  if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
  const accepted = [...new Set(ids)];
  if (!accepted.length) return { scope: parsed.scope, ids: [] };
  const db = await storageDatabase;
  if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces");
    const active = store.get("auth-active|" + parsed.scope), fence = store.get("auth-frozen|" + namespace);
    const facts = readGlobalFacts(store, namespace)!;
    let failure: unknown;
    facts.onsuccess = () => {
      try {
        if (!isCurrent() || fence.result || (parsed.modern ? active.result !== namespace : active.result && active.result !== namespace))
          throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
        writeVerifiedGlobals(store, namespace, new Set(accepted), knownGlobalIDs(namespace, facts.result), isCurrent);
      } catch (error) { failure = error; tx.abort(); }
    };
    tx.oncomplete = () => isCurrent() ? resolve({ scope: parsed.scope, ids: accepted }) : reject(new SyncProtocolError("SYNC_CONTEXT_CHANGED"));
    tx.onerror = () => reject(failure ?? tx.error ?? new SyncProtocolError("GLOBAL_STORAGE_FAILED"));
    tx.onabort = () => reject(failure ?? tx.error ?? new SyncProtocolError("GLOBAL_STORAGE_FAILED"));
  });
}

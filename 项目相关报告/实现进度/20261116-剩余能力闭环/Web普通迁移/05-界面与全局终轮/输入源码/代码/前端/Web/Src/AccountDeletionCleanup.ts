// Created by Jobs on 2026年11月17日，星期二.
import type { PasswordChangeIdentity } from "./PasswordChangeProtocol";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const positive = (value: unknown): value is string => typeof value === "string" && /^[1-9][0-9]{0,18}$/.test(value) && BigInt(value) <= 9223372036854775807n;
const invalid = (): never => { throw Error("ACCOUNT_ERASURE_CONTEXT_INVALID"); };
type PersonalNamespace = { value: string; scope: string };

function personalNamespace(value: string): PersonalNamespace | undefined {
  let parts = value.split("|");
  if ([7, 8].includes(parts.length) && parts.at(-2) === "archive" && positive(parts.at(-1)!)) parts = parts.slice(0, -2);
  if (![5, 6].includes(parts.length) || parts[3] !== "personal" || !uuid.test(parts[1]) || !uuid.test(parts[2])) return;
  if (parts.length === 6 ? !positive(parts[4]) || !uuid.test(parts[5]) : !uuid.test(parts[4])) return;
  try {
    const origin = new URL(parts[0]);
    if (!["http:", "https:"].includes(origin.protocol) || origin.origin !== parts[0]) return;
  } catch { return; }
  return { value: parts.join("|"), scope: parts.slice(0, 4).join("|") };
}

export function completedErasureKeyForNamespace(namespace: string): string | undefined {
  const parsed = personalNamespace(namespace);
  return parsed ? "account-erasure|" + parsed.scope : undefined;
}

// Queue this read before any writes. Aborting the whole transaction also rolls
// back work already scheduled by a stale React callback or a linked Outbox hook.
export function guardCompletedErasure(store: IDBObjectStore, namespace: string): void {
  const key = completedErasureKeyForNamespace(namespace);
  if (!key) return;
  const read = store.get(key);
  read.onsuccess = () => { if (read.result !== undefined) store.transaction.abort(); };
}

export function completedErasureScope(identity: PasswordChangeIdentity, expectedOrigin: string): string {
  if (!identity || !uuid.test(identity.scopeId) || !uuid.test(identity.userId) || !uuid.test(identity.deviceId) || !uuid.test(identity.familyId) || !positive(identity.epoch)) invalid();
  const wanted = [expectedOrigin, identity.scopeId, identity.userId, "personal", identity.epoch, identity.deviceId].join("|");
  const parsed = personalNamespace(wanted);
  if (!parsed || identity.namespace !== wanted) invalid();
  return parsed.scope;
}

const exactNamespacePrefixes = [
  "outbox|", "sync-state|", "sync-retained|", "sync-personal|", "self-state-operations|",
  "data-exports|", "playback-checkpoints|", "migration-history|", "attachment-sends|", "attachment-cleanup-pending|",
] as const;
const stagedNamespacePrefixes = ["sync-stage|", "sync-schema-archive|", "migration-stage|"] as const;

// Keys, rather than mutable record values or usernames, establish deletion ownership.
export function completedErasureKey(key: unknown, scope: string): { namespace: string; fence: boolean } | undefined {
  if (typeof key !== "string") return;
  const direct = personalNamespace(key);
  if (direct?.scope === scope) return { namespace: direct.value, fence: false };
  if (key.startsWith("auth-frozen|")) {
    const parsed = personalNamespace(key.slice("auth-frozen|".length));
    return parsed?.scope === scope ? { namespace: parsed.value, fence: true } : undefined;
  }
  for (const prefix of exactNamespacePrefixes) {
    if (!key.startsWith(prefix)) continue;
    const parsed = personalNamespace(key.slice(prefix.length));
    return parsed?.scope === scope ? { namespace: parsed.value, fence: false } : undefined;
  }
  for (const prefix of stagedNamespacePrefixes) {
    if (!key.startsWith(prefix)) continue;
    const raw = key.slice(prefix.length), end = raw.lastIndexOf("|");
    if (end < 0 || !uuid.test(raw.slice(end + 1))) return;
    const parsed = personalNamespace(raw.slice(0, end));
    return parsed?.scope === scope ? { namespace: parsed.value, fence: false } : undefined;
  }
  if (key.startsWith("operation|")) {
    const raw = key.slice("operation|".length).split("|");
    for (const count of [6, 5]) {
      if (raw.length <= count || !raw.slice(count).join("|")) continue;
      const parsed = personalNamespace(raw.slice(0, count).join("|"));
      if (parsed?.scope === scope) return { namespace: parsed.value, fence: false };
    }
  }
}

export function eraseCompletedPersonalRecords(
  store: IDBObjectStore,
  identity: PasswordChangeIdentity,
  proof: { deletionId: string; requestVersion: string },
  isCurrent: () => boolean,
): void {
  const scope = completedErasureScope(identity, location.origin);
  if (!uuid.test(proof.deletionId) || !positive(proof.requestVersion) || !isCurrent()) invalid();
  const account = scope.split("|").slice(0, 3).join("|");
  const perUserKeys = new Set(["group-operations|" + account, "migration-intents|" + account]);
  const passwordKey = "password-changes|" + [location.origin, identity.scopeId].join("|");
  const fence = (namespace: string) => store.put("ACCOUNT_DELETION_COMPLETED", "auth-frozen|" + namespace);
  fence(identity.namespace);
  store.delete("auth-active|" + scope);
  const cursor = store.openCursor();
  cursor.onsuccess = () => {
    try {
      if (!isCurrent()) invalid();
      const row = cursor.result;
      if (!row) {
        store.put({ schema: "1", deletionId: proof.deletionId, requestVersion: proof.requestVersion, status: "completed" }, "account-erasure|" + scope);
        return;
      }
      const owned = completedErasureKey(row.key, scope);
      if (owned) {
        fence(owned.namespace);
        if (!owned.fence) row.delete();
      } else if (typeof row.key === "string" && perUserKeys.has(row.key)) {
        row.delete();
      } else if (row.key === passwordKey && Array.isArray(row.value)) {
        // This bucket is shared by users of one instance; preserve everyone else.
        row.update(row.value.filter((value: { userId?: string; scopeId?: string } | null) =>
          !value || value.userId !== identity.userId || value.scopeId !== identity.scopeId));
      }
      row.continue();
    } catch { store.transaction.abort(); }
  };
}

import { applyKnownGlobals, globalMessagePlaceholder, loadVerifiedGlobals, observeVerifiedGlobals } from "./GlobalMessages";
import { validOriginalMessageResult } from "./SyncView";
import { unavailableForward } from "./forward";
import type { PublicAuth } from "./AuthSession";
import type { Message } from "./types";
import { SyncProtocolError } from "./SyncProtocol";

export type GlobalMessageNotice = { scope: string; ids: string[]; storageFailed?: boolean };
const known = new Map<string, Set<string>>();
const pending = new Map<string, Set<string>>();
const listeners = new Set<(notice: GlobalMessageNotice) => void>();
let revision = 0;
let legacyNamespace = "";

export const messageFactRevision = () => revision;
export const bindMessageNamespace = (namespace: string) => { legacyNamespace = namespace; };
function responseNamespace(auth: PublicAuth | null): string {
  return auth?.deploymentId && auth.userId && auth.deviceId && auth.sourceContextId === "personal"
    ? [location.origin, auth.deploymentId, auth.userId, "personal", auth.restoreEpoch, auth.deviceId].join("|")
    : legacyNamespace;
}
export async function refreshPrivateMediaFacts(auth: PublicAuth | null, isCurrent: () => boolean): Promise<void> {
  const namespace = responseNamespace(auth);
  if (namespace) await refreshMessageFacts(namespace, isCurrent);
}
export function subscribeGlobalMessages(listener: (notice: GlobalMessageNotice) => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

function publish(namespace: string, ids: readonly string[]): Set<string> {
  const scope = namespace.split("|").slice(0, 4).join("|");
  const all = known.get(scope) ?? new Set<string>();
  const added = ids.filter(id => !all.has(id));
  for (const id of added) all.add(id);
  known.set(scope, all);
  if (added.length) {
    revision++;
    for (const listener of listeners) listener({ scope, ids: [...all] });
  }
  return all;
}

export async function refreshMessageFacts(namespace: string, isCurrent: () => boolean): Promise<Set<string>> {
  const scope = namespace.split("|").slice(0, 4).join("|");
  const outstanding = pending.get(scope);
  if (outstanding?.size) await persistGlobals(namespace, [...outstanding], isCurrent);
  const ids = await loadVerifiedGlobals(namespace);
  if (!isCurrent()) throw Error("SYNC_CONTEXT_CHANGED");
  return new Set(publish(namespace, ids));
}

async function persistGlobals(namespace: string, ids: string[], isCurrent: () => boolean): Promise<void> {
  if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
  const scope = namespace.split("|").slice(0, 4).join("|");
  publish(namespace, ids);
  try {
    await observeVerifiedGlobals(namespace, ids, isCurrent);
    if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
    const outstanding = pending.get(scope);
    for (const id of ids) outstanding?.delete(id);
    if (outstanding && !outstanding.size) {
      pending.delete(scope);
      for (const listener of listeners) listener({ scope, ids: [...(known.get(scope) ?? [])], storageFailed: false });
    }
  } catch (error) {
    if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
    pending.set(scope, new Set([...(pending.get(scope) ?? []), ...ids]));
    for (const listener of listeners) listener({ scope, ids: [...(known.get(scope) ?? [])], storageFailed: true });
    throw new SyncProtocolError("GLOBAL_STORAGE_FAILED");
  }
}

// Restrict observation to authenticated business DTOs. Sync envelopes take their own validation path.
export async function projectMessageResponse(
  path: string, data: unknown, auth: PublicAuth | null, startedAt: number, isCurrent: () => boolean,
  options: RequestInit = {},
): Promise<unknown> {
  const route = path.split("?")[0];
  const uuid = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
  const directRoute = new RegExp("^/messages/(" + uuid + ")(?:/[a-z-]+)?$").exec(route);
  const conversationRoute = new RegExp("^/conversations/(" + uuid + ")/(?:messages|message-results)$").exec(route);
  const direct = !!directRoute, conversation = !!conversationRoute;
  const list = route === "/search" || route === "/favorites";
  const announcementRoute = new RegExp("^/conversations/(" + uuid + ")/announcement$").exec(route);
  const announcement = !!announcementRoute;
  if ((!direct && !conversation && !list && !announcement) || !data || typeof data !== "object") return data;
  const namespace = responseNamespace(auth);
  if (!namespace) return data;
  const row = data as Record<string, unknown>;
  const items = Array.isArray(row.items) && (conversation || list) ? row.items : undefined;
  const nested = announcement && row.message && typeof row.message === "object" ? row.message : undefined;
  const candidates = items ?? (nested ? [nested] : direct || conversation ? [data] : []);
  const body = typeof options.body === "string" ? JSON.parse(options.body) as Record<string, unknown> : undefined;
  const ownWrite = conversation && (options.method ?? "GET").toUpperCase() === "POST";
  const filteredConversation = list ? new URLSearchParams(path.split("?")[1] ?? "").get("conversationId") : undefined;
  const globals = candidates.filter((value): value is Message => {
    if (!value || typeof value !== "object") return false;
    const message = value as Message;
    if (message.deleted !== true || !validOriginalMessageResult(message, message.senderId, message.conversationId, message.clientMsgId)) return false;
    if (directRoute && message.id !== directRoute[1] || conversationRoute && message.conversationId !== conversationRoute[1]
      || filteredConversation && message.conversationId !== filteredConversation
      || announcementRoute && (message.conversationId !== announcementRoute[1] || row.messageId !== message.id)
      || ownWrite && (message.senderId !== (auth?.userId ?? namespace.split("|")[2]) || !body || message.clientMsgId !== body.clientMsgId))
      throw new SyncProtocolError("GLOBAL_RESPONSE_BINDING_INVALID");
    return true;
  }).map(message => message.id);
  if (globals.length) await persistGlobals(namespace, globals, isCurrent);
  const ids = await refreshMessageFacts(namespace, isCurrent);
  const project = (value: unknown): unknown => {
    if (!value || typeof value !== "object") return value;
    const message = value as Message;
    if (ids.has(message.id)) return globalMessagePlaceholder(message);
    return message.forwarded && startedAt !== revision ? unavailableForward(message) : value;
  };
  if (items) return { ...row, items: items.map(project) };
  if (nested) return { ...row, message: project(nested) };
  return direct || conversation ? project(data) : data;
}

export { applyKnownGlobals };

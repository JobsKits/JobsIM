import { SyncProtocolError, syncCounter, syncID, syncRecord, type SyncProjection } from "./SyncProtocol";
import { unavailableForward } from "./forward";
import type { Conversation, Message, Space, User } from "./types";

const string = (value: unknown, fallback = "") => typeof value === "string" ? value : fallback;

function conversation(value: unknown, expected: string): Conversation {
  const row = syncRecord(value), id = syncID(row.id);
  if (id !== expected || !["direct", "group", "self"].includes(String(row.kind)) || !Array.isArray(row.memberIds)) throw Error("SYNC_INVALID_CONVERSATION");
  return {
    id, kind: row.kind as Conversation["kind"], title: string(row.title),
    ownerId: row.ownerId ? syncID(row.ownerId) : "", memberIds: row.memberIds.map(syncID),
    updatedAt: string(row.updatedAt), cursor: string(row.cursor, "0"),
    state: string(row.state), readOnly: row.readOnly === true,
    membershipGeneration: row.membershipGeneration === undefined ? undefined : syncCounter(row.membershipGeneration),
    roleGeneration: row.roleGeneration === undefined ? undefined : syncCounter(row.roleGeneration),
    sendAdmission: typeof row.sendAdmission === "string" ? row.sendAdmission : undefined,
  };
}

function unavailable(message: Message, global: boolean): Message {
  return {
    ...unavailableForward(message), text: "", deleted: global,
    contentUnavailable: !global, attachmentId: undefined, attachmentName: undefined,
    attachmentURL: undefined, forwardAdmission: undefined, forwardSourceVersion: undefined,
    replyToId: undefined, mentionedUserIds: undefined, mentionAll: false,
    reactions: [], pinned: false, favorite: false,
  };
}

function message(projection: SyncProjection, previous?: Message): Message | null {
  if (previous?.deleted) return unavailable(previous, true);
  if (projection.state === "unknown" || projection.state === "redacted") return previous ? unavailable(previous, false) : null;
  const row = syncRecord(projection.object), id = syncID(row.id);
  if (id !== projection.ref.id) throw Error("SYNC_INVALID_MESSAGE");
  const global = projection.state === "global";
  const conversationId = row.conversationId ?? projection.ref.conversationId ?? previous?.conversationId;
  if (!conversationId && global) return null;
  const value: Message = {
    id, conversationId: syncID(conversationId), senderId: row.senderId ? syncID(row.senderId) : previous?.senderId ?? "",
    clientMsgId: row.clientMsgId ? syncID(row.clientMsgId) : previous?.clientMsgId ?? id,
    type: string(row.type, "text"), text: string(row.text),
    createdAt: string(row.createdAt, previous?.createdAt ?? ""), updatedAt: string(row.updatedAt),
    version: syncCounter(row.version ?? previous?.version ?? projection.currentRevision),
    contentVersion: row.contentVersion === undefined ? undefined : syncCounter(row.contentVersion),
    sequence: syncCounter(row.sequence ?? previous?.sequence ?? "0"),
    deleted: row.deleted === true, contentUnavailable: row.contentUnavailable === true,
    replyToId: row.replyToId ? syncID(row.replyToId) : undefined,
    attachmentId: row.attachmentId ? syncID(row.attachmentId) : undefined,
    attachmentName: typeof row.attachmentName === "string" ? row.attachmentName : undefined,
    attachmentURL: typeof row.attachmentURL === "string" ? row.attachmentURL : undefined,
    reactions: Array.isArray(row.reactions) ? row.reactions.map(value => {
      const reaction = syncRecord(value);
      return { emoji: string(reaction.emoji), userId: reaction.userId ? syncID(reaction.userId) : undefined, count: typeof reaction.count === "number" && Number.isSafeInteger(reaction.count) && reaction.count > 0 ? reaction.count : undefined };
    }) : [],
    pinned: row.pinned === true, favorite: row.favorite === true,
    mentionedUserIds: Array.isArray(row.mentionedUserIds) ? row.mentionedUserIds.map(syncID) : undefined,
    mentionAll: row.mentionAll === true, mentionedMe: row.mentionedMe === true,
    forwarded: row.forwarded === true, forwardUnavailable: row.forwardUnavailable === true,
    forwardAdmission: typeof row.forwardAdmission === "string" ? row.forwardAdmission : undefined,
    forwardOfId: row.forwardOfId ? syncID(row.forwardOfId) : undefined,
    forwardSourceVersion: row.forwardSourceVersion === undefined ? undefined : syncCounter(row.forwardSourceVersion),
  };
  if (projection.ref.conversationId && value.conversationId !== projection.ref.conversationId) throw Error("SYNC_INVALID_MESSAGE");
  return global || projection.state === "unavailable" || value.deleted || value.contentUnavailable || value.forwardUnavailable ? unavailable(value, global || value.deleted) : value;
}

export function materializeSyncSpace(previous: Space, projections: SyncProjection[], options: { replaceCloud: boolean; currentUserId?: string; readingConversationId?: string }): Space {
  const conversations = new Map((options.replaceCloud ? [] : previous.conversations).map(row => [row.id, row]));
  const oldMessages = new Map(Object.values(previous.messages).flat().filter(row => !row.pending).map(row => [row.id, row]));
  const messages = new Map(options.replaceCloud ? [] : [...oldMessages]);
  const profiles: Record<string, User> = { ...previous.profiles };
  for (const projection of projections) {
    if (projection.ref.kind === "conversation") {
      if (projection.state === "available") conversations.set(projection.ref.id, conversation(projection.object, projection.ref.id));
      else conversations.delete(projection.ref.id);
    } else if (projection.ref.kind === "message") {
      const next = message(projection, oldMessages.get(projection.ref.id));
      if (next) messages.set(next.id, next);
    } else if (projection.ref.kind === "account" && projection.state === "available") {
      const row = syncRecord(projection.object), id = syncID(row.id);
      if (id !== projection.ref.id) throw Error("SYNC_INVALID_ACCOUNT");
      profiles[id] = { id, username: string(row.username), displayName: string(row.displayName), admin: row.admin === true };
    }
  }
  const allowed = new Set(conversations.keys()), grouped: Record<string, Message[]> = {};
  for (const row of messages.values()) {
    if (!allowed.has(row.conversationId)) continue;
    (grouped[row.conversationId] ??= []).push(row);
  }
  const outbox = previous.outbox.map(row => allowed.has(row.conversationId) || row.frozen ? row : { ...row, frozen: true, freezeReason: row.freezeReason ?? "SYNC_SCOPE_UNAVAILABLE" });
  for (const row of outbox) {
    if (row.frozen) continue;
    const rows = grouped[row.conversationId] ??= [];
    if (!rows.some(message => message.pending && message.senderId === row.senderId && message.clientMsgId === row.clientMsgId)) rows.push(row);
  }
  for (const rows of Object.values(grouped)) rows.sort((left, right) => {
    if (!!left.pending !== !!right.pending) return left.pending ? 1 : -1;
    const l = BigInt(left.sequence), r = BigInt(right.sequence);
    return l === r ? left.createdAt.localeCompare(right.createdAt) || left.id.localeCompare(right.id) : l < r ? -1 : 1;
  });
  const visibleIDs = new Set(Object.values(grouped).flat().filter(row => !row.deleted && !row.contentUnavailable).map(row => row.id));
  const unread: Record<string, string[]> = Object.fromEntries(Object.entries(previous.unread ?? {}).filter(([id]) => allowed.has(id)).map(([id, rows]) => [id, rows.filter(id => visibleIDs.has(id))]));
  const mentionUnread: Record<string, string[]> = Object.fromEntries(Object.entries(previous.mentionUnread ?? {}).filter(([id]) => allowed.has(id)).map(([id, rows]) => [id, rows.filter(id => visibleIDs.has(id))]));
  if (!options.replaceCloud && options.currentUserId) {
    for (const row of messages.values()) {
      if (!allowed.has(row.conversationId) || row.deleted || row.contentUnavailable || row.pending || row.senderId === options.currentUserId || row.conversationId === options.readingConversationId) continue;
      if (!oldMessages.has(row.id)) unread[row.conversationId] = [...new Set([...(unread[row.conversationId] ?? []), row.id])];
      if (row.mentionedMe && unread[row.conversationId]?.includes(row.id)) mentionUnread[row.conversationId] = [...new Set([...(mentionUnread[row.conversationId] ?? []), row.id])];
    }
  }
  if (options.readingConversationId) {
    unread[options.readingConversationId] = [];
    mentionUnread[options.readingConversationId] = [];
  }
  return { ...previous, conversations: [...conversations.values()].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt) || a.id.localeCompare(b.id)), messages: grouped, outbox, unread, mentionUnread, profiles };
}

export async function refreshSyncForwardViews(
  space: Space,
  targets: Message[],
  request: <T>(path: string) => Promise<T>,
  isCurrent: () => boolean,
): Promise<Space> {
  const messages = Object.fromEntries(Object.entries(space.messages).map(([id, rows]) => [id, [...rows]]));
  for (let offset = 0; offset < targets.length; offset += 4) {
    await Promise.all(targets.slice(offset, offset + 4).map(async cached => {
      let fresh: Message;
      try {
        fresh = await request<Message>("/messages/" + encodeURIComponent(cached.id));
        if (fresh.id !== cached.id || fresh.conversationId !== cached.conversationId || fresh.forwarded !== true || BigInt(syncCounter(fresh.version)) < BigInt(cached.version)) throw new SyncProtocolError("SYNC_INVALID_FORWARD");
        if (fresh.deleted || fresh.forwardUnavailable || fresh.contentUnavailable) fresh = unavailableForward(fresh);
      } catch (error) {
        const status = error && typeof error === "object" && "status" in error ? error.status : 0;
        if (status !== 403 && status !== 404) throw error;
        fresh = unavailableForward(cached);
      }
      if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
      messages[cached.conversationId] = (messages[cached.conversationId] ?? []).map(row => row.id === cached.id ? fresh : row);
    }));
  }
  return { ...space, messages };
}

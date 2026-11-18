import { SyncProtocolError, syncCounter, syncID, syncRecord, type SyncRef } from "./SyncProtocol";

export type SelfStateKind = "preferences" | "conversation_view" | "read_state" | "read_observation" | "draft";
export type SelfStateRow = Record<string, unknown> & {
  scopeId: string; sourceContextId: "personal"; userId: string; revisionEpoch: string;
  id?: string; conversationId?: string; version?: string;
};
export type PersonalState = { records: Record<string, SelfStateRow> };
export const emptyPersonalState = (): PersonalState => ({ records: {} });
export const selfStateKind = (kind: string): kind is SelfStateKind =>
  ["preferences", "conversation_view", "read_state", "read_observation", "draft"].includes(kind);

const reject = (code = "SYNC_INVALID_PERSONAL_STATE"): never => { throw new SyncProtocolError(code); };
const positive = (value: unknown) => { const counter = syncCounter(value); if (counter === "0") reject(); return counter; };
const boolean = (value: unknown) => typeof value === "boolean" ? value : reject();
const allowed = (row: Record<string, unknown>, fields: string[]) => {
  if (Object.keys(row).some(key => !fields.includes(key))) reject();
};

// Owner revision orders sync pointers; an object's own version is a different
// counter and must never be compared to that owner-wide clock.
export function parsePersonalState(value: unknown, ref: SyncRef): SelfStateRow {
  const row = syncRecord(value);
  if (!selfStateKind(ref.kind)) reject();
  const scopeId = syncID(row.scopeId), userId = syncID(row.userId);
  const revisionEpoch = positive(row.revisionEpoch);
  if (row.sourceContextId !== "personal" || ref.subjectUserId !== userId) reject("SYNC_PERSONAL_IDENTITY_MISMATCH");
  if (ref.kind === "preferences") {
    if (ref.id !== userId || ref.owner !== "directory") reject();
  } else if (syncID(row.id) !== ref.id || syncID(row.conversationId) !== ref.conversationId || ref.owner !== "sync") reject();
  const base = ["id", "scopeId", "sourceContextId", "userId", "conversationId", "revisionEpoch", "version"];
  if (row.removed === true) {
    allowed(row, [...base, "removed"]);
    if (ref.state !== "removed") reject();
    return { ...row, scopeId, userId, sourceContextId: "personal", revisionEpoch };
  }
  syncCounter(row.version);
  switch (ref.kind) {
    case "preferences":
      allowed(row, [...base, "fontPreset", "explicit"]);
      if (!["small", "standard", "large"].includes(String(row.fontPreset))) reject();
      boolean(row.explicit);
      break;
    case "conversation_view":
      allowed(row, [...base, "pinnedRank", "mutedUntil", "archived", "hidden", "manualUnread", "manualUnreadGeneration", "manualUnreadMarkedAt", "manualUnreadFrom"]);
      for (const field of ["archived", "hidden", "manualUnread"]) boolean(row[field]);
      syncCounter(row.manualUnreadGeneration);
      if (row.pinnedRank !== null && (!Number.isInteger(row.pinnedRank) || Number(row.pinnedRank) < 0 || Number(row.pinnedRank) > 999999)) reject();
      for (const field of ["mutedUntil", "manualUnreadMarkedAt"]) if (row[field] !== null && (typeof row[field] !== "string" || !Number.isFinite(Date.parse(row[field] as string)))) reject();
      if (row.manualUnreadFrom !== null) {
        const point = syncRecord(row.manualUnreadFrom); positive(point.epoch); positive(point.sequence);
      }
      break;
    case "read_state": {
      allowed(row, [...base, "membershipGeneration", "readPoint", "countStatus", "continuityStatus", "writable"]);
      positive(row.membershipGeneration); boolean(row.writable);
      const point = syncRecord(row.readPoint), epoch = syncCounter(point.epoch), sequence = syncCounter(point.sequence);
      if ((epoch === "0") !== (sequence === "0") || row.countStatus !== "not_computed" || typeof row.continuityStatus !== "string") reject();
      break;
    }
    case "read_observation":
      allowed(row, [...base, "messageId", "membershipGeneration", "observedInEpoch", "observedAt", "orderKnown", "eligibleAtObservation", "currentlyApplicable", "countStatus", "creationEpoch", "creationSequence"]);
      syncID(row.messageId); positive(row.membershipGeneration); positive(row.observedInEpoch);
      boolean(row.eligibleAtObservation); boolean(row.currentlyApplicable);
      if (row.countStatus !== "not_computed" || typeof row.observedAt !== "string" || !Number.isFinite(Date.parse(row.observedAt))) reject();
      if (boolean(row.orderKnown)) { positive(row.creationEpoch); positive(row.creationSequence); }
      else if (row.creationEpoch !== "unknown" || row.creationSequence !== "unknown") reject();
      break;
    case "draft":
      allowed(row, [...base, "contentState", "ownedText", "replyState", "reply", "writable", "pendingOperationId"]);
      if (!["empty", "owned", "unavailable"].includes(String(row.contentState)) || !["none", "available", "unavailable"].includes(String(row.replyState))) reject();
      boolean(row.writable);
      if (row.ownedText !== null && (typeof row.ownedText !== "string" || new TextEncoder().encode(row.ownedText).length > 16384)) reject();
      if (row.reply !== null) {
        const reply = syncRecord(row.reply); allowed(reply, ["messageId", "sourceVersion"]);
        if (row.replyState !== "available") reject();
        syncID(reply.messageId); positive(reply.sourceVersion);
      }
      if (row.pendingOperationId !== undefined) syncID(row.pendingOperationId);
      break;
  }
  return { ...row, scopeId, userId, sourceContextId: "personal", revisionEpoch };
}

export function personalStateRecord(state: PersonalState, kind: SelfStateKind, conversation?: string): SelfStateRow | undefined {
  return Object.entries(state.records).find(([key, row]) => key.startsWith(kind + "|") && (conversation === undefined || row.conversationId === conversation))?.[1];
}

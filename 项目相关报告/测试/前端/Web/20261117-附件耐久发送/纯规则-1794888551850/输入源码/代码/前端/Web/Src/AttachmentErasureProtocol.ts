// Created by Jobs on 2026年11月17日，星期二.
export type AttachmentErasureFact = { userId: string; conversationId: string; clientMsgId: string; serverId: string };
export type AttachmentErasureFloor = { schema: "1"; scope: string; entries: AttachmentErasureFact[] };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
function invalid(): never { throw Error("ATTACHMENT_ERASURE_INVALID"); }
export const attachmentErasureKey = (scope: string) => "attachment-erasure|" + scope;
export const attachmentErasureRepairKey = (scope: string) => "attachment-erasure-repair|" + scope;
function validateScope(scope: string): string {
  const p = scope.split("|");
  if (p.length !== 4 || p[3] !== "personal" || !uuid.test(p[1]) || !uuid.test(p[2])) invalid();
  try { const origin = new URL(p[0]); if (origin.origin !== p[0] || !["http:", "https:"].includes(origin.protocol)) invalid(); } catch { invalid(); }
  return p[2];
}
function fact(value: AttachmentErasureFact, userId: string): AttachmentErasureFact {
  if (!value || value.userId !== userId || !uuid.test(value.conversationId) || !uuid.test(value.clientMsgId) || !uuid.test(value.serverId) || Object.keys(value).some(key => !["userId", "conversationId", "clientMsgId", "serverId"].includes(key))) invalid();
  return { userId, conversationId: value.conversationId, clientMsgId: value.clientMsgId, serverId: value.serverId };
}
const binding = (value: AttachmentErasureFact) => value.conversationId + "|" + value.clientMsgId;
export function attachmentErasureFloor(value: unknown, scope: string): AttachmentErasureFloor {
  const user = validateScope(scope);
  if (value === undefined) return { schema: "1", scope, entries: [] };
  const floor = value as AttachmentErasureFloor;
  if (floor?.schema !== "1" || floor.scope !== scope || !Array.isArray(floor.entries) || Object.keys(floor).some(key => !["schema", "scope", "entries"].includes(key))) invalid();
  const entries = floor.entries.map(entry => fact(entry, user));
  if (new Set(entries.map(binding)).size !== entries.length) invalid();
  return { schema: "1", scope, entries };
}
export function mergeAttachmentErasureFloor(value: unknown, scope: string, facts: readonly AttachmentErasureFact[]): AttachmentErasureFloor {
  const floor = attachmentErasureFloor(value, scope), user = validateScope(scope), known = new Map(floor.entries.map(entry => [binding(entry), entry]));
  for (const raw of facts) {
    const entry = fact(raw, user), previous = known.get(binding(entry));
    if (previous && previous.serverId !== entry.serverId) invalid();
    if (!previous) { floor.entries.push(entry); known.set(binding(entry), entry); }
  }
  return floor;
}
export function attachmentErasureMatches(floor: AttachmentErasureFloor, userId: string, conversationId: string, clientMsgId: string): boolean {
  return floor.entries.some(entry => entry.userId === userId && entry.conversationId === conversationId && entry.clientMsgId === clientMsgId);
}

// Created by Jobs on 2026年11月17日，星期二.
import { syncCounter, syncID, syncRecord, SyncProtocolError } from "./SyncProtocol";
import type { User } from "./types";

export type GroupGovernance = { version: string; membershipGeneration: string; roleGeneration: string; role: "owner" | "admin" | "member"; state: string; readOnly: boolean; canRecover: boolean; policy: { version: string; muteAll: boolean; slowSeconds: number; maxMembers: number }; mutedUntil: string | null };
export type GroupMutation = { requestId: string; version: string; membershipGeneration: string; roleGeneration: string; action: "settings" | "mute" | "role" | "transfer" | "dissolve" | "recover"; targetId?: string; targetMembershipGeneration?: string; targetRoleGeneration?: string; role?: "admin" | "member"; mutedUntil?: string; muteAll?: boolean; slowSeconds?: number; maxMembers?: number };
export type GroupInvite = { id: string; requestId?: string; expiresAt: string; maxUses: number; uses: number; approvalRequired: boolean; revokedAt: string | null; token?: string };
export type GroupJoin = { id: string; userId?: string; username?: string; displayName?: string; accountState?: string; requestId?: string; state: "pending" | "approved" | "rejected"; createdAt?: string; membershipGeneration?: string | null };
export type GroupShare = { deploymentId: string; conversationId: string; inviteId: string; token: string };
export function groupFailure(code = "GROUP_PROTOCOL_INVALID"): never { throw new SyncProtocolError(code); }
const counter = (value: unknown) => { const text = syncCounter(value); if (text === "0") groupFailure(); return text; };
const bounded = (value: unknown, min: number, max: number): value is number => typeof value === "number" && Number.isInteger(value) && value >= min && value <= max;
const date = (value: unknown): value is string => typeof value === "string" && Number.isFinite(Date.parse(value));
export function parseGroupGovernance(raw: unknown): GroupGovernance {
  const row = syncRecord(raw), policy = syncRecord(row.policy);
  if (!["owner", "admin", "member"].includes(String(row.role)) || typeof row.state !== "string" || typeof row.readOnly !== "boolean" || typeof policy.muteAll !== "boolean" || !bounded(policy.slowSeconds, 0, 3600) || !bounded(policy.maxMembers, 2, 10000) || row.mutedUntil !== null && !date(row.mutedUntil)) groupFailure();
  return { version: counter(row.version), membershipGeneration: counter(row.membershipGeneration), roleGeneration: counter(row.roleGeneration), role: row.role as GroupGovernance["role"], state: row.state, readOnly: row.readOnly, canRecover: row.canRecover === true, policy: { version: counter(policy.version), muteAll: policy.muteAll, slowSeconds: policy.slowSeconds, maxMembers: policy.maxMembers }, mutedUntil: row.mutedUntil as string | null };
}
export function groupBase(snapshot: GroupGovernance) { return { version: snapshot.version, membershipGeneration: snapshot.membershipGeneration, roleGeneration: snapshot.roleGeneration }; }
export function groupTarget(target: User) { return { targetId: syncID(target.id), targetMembershipGeneration: counter(target.membershipGeneration), targetRoleGeneration: counter(target.roleGeneration) }; }
export function validateGroupMutation(value: GroupMutation) {
  if (!value || Object.keys(value).some(key => !["requestId", "version", "membershipGeneration", "roleGeneration", "action", "targetId", "targetMembershipGeneration", "targetRoleGeneration", "role", "mutedUntil", "muteAll", "slowSeconds", "maxMembers"].includes(key))) groupFailure();
  syncID(value.requestId); counter(value.version); counter(value.membershipGeneration); counter(value.roleGeneration);
  if (!["settings", "mute", "role", "transfer", "dissolve", "recover"].includes(value.action)) groupFailure();
  if (["mute", "role", "transfer"].includes(value.action)) { syncID(value.targetId); counter(value.targetMembershipGeneration); counter(value.targetRoleGeneration); }
  if (value.action === "recover") { syncID(value.targetId); counter(value.targetMembershipGeneration); }
  if (value.action === "settings" && (typeof value.muteAll !== "boolean" || !bounded(value.slowSeconds, 0, 3600) || !bounded(value.maxMembers, 2, 10000))) groupFailure();
  if (value.action === "role" && !["admin", "member"].includes(String(value.role)) || value.action === "mute" && value.mutedUntil !== "" && !date(value.mutedUntil)) groupFailure();
}
export function parseGroupInvite(raw: unknown, token = true): GroupInvite {
  const row = syncRecord(raw);
  if (!date(row.expiresAt) || !bounded(row.maxUses, 1, 10000) || !bounded(row.uses ?? 0, 0, 10000) || typeof row.approvalRequired !== "boolean" || row.revokedAt != null && !date(row.revokedAt) || row.token !== undefined && (typeof row.token !== "string" || row.token.length > 256 || !row.token.startsWith(String(row.id) + "."))) groupFailure();
  return { id: syncID(row.id), ...(row.requestId !== undefined ? { requestId: syncID(row.requestId) } : {}), expiresAt: row.expiresAt, maxUses: row.maxUses, uses: (row.uses ?? 0) as number, approvalRequired: row.approvalRequired, revokedAt: row.revokedAt as string | null ?? null, ...(token && row.token ? { token: row.token } : {}) };
}
export function parseGroupJoin(raw: unknown): GroupJoin {
  const row = syncRecord(raw);
  if (!["pending", "approved", "rejected"].includes(String(row.state)) || row.createdAt !== undefined && !date(row.createdAt)) groupFailure();
  for(const key of ["username","displayName","accountState"])if(row[key]!==undefined&&typeof row[key]!=="string")groupFailure();
  return { id: syncID(row.id), state: row.state as GroupJoin["state"], ...(row.userId !== undefined ? { userId: syncID(row.userId) } : {}), ...(row.requestId !== undefined ? { requestId: syncID(row.requestId) } : {}), ...(row.createdAt !== undefined ? { createdAt: row.createdAt as string } : {}), ...(row.membershipGeneration !== undefined ? { membershipGeneration: row.membershipGeneration === null ? null : counter(row.membershipGeneration) } : {}),...(typeof row.username==="string"?{username:row.username}:{}),...(typeof row.displayName==="string"?{displayName:row.displayName}:{}),...(typeof row.accountState==="string"?{accountState:row.accountState}:{}) };
}
export function parseGroupPage<T>(raw: unknown, parse: (raw: unknown) => T, after = ""): { items: T[]; nextCursor: string } {
  const row = syncRecord(raw);
  if (!Array.isArray(row.items) || row.items.length > 100 || typeof row.nextCursor !== "string" || row.nextCursor && row.nextCursor === after) groupFailure("GROUP_PAGE_INVALID");
  if (row.nextCursor) syncID(row.nextCursor);
  const ids = row.items.map(item => syncID(syncRecord(item).id));
  if (new Set(ids).size !== ids.length || ids.some(id => after && id <= after) || row.nextCursor && (!ids.length || row.nextCursor !== ids.at(-1))) groupFailure("GROUP_PAGE_INVALID");
  return { items: row.items.map(parse), nextCursor: row.nextCursor };
}
export function shareGroupInvite(deploymentId: string, conversationId: string, invite: GroupInvite) {
  const share: GroupShare = { deploymentId: syncID(deploymentId), conversationId: syncID(conversationId), inviteId: syncID(invite.id), token: invite.token ?? "" };
  if (!share.token || share.token.length > 256) groupFailure("GROUP_INVITE_REQUIRED");
  return "JobsIM-Invite-1:" + btoa(String.fromCharCode(...new TextEncoder().encode(JSON.stringify(share))));
}
export function parseGroupShare(text: string, deploymentId: string): GroupShare {
  if (typeof text !== "string" || text.length > 4096 || !text.startsWith("JobsIM-Invite-1:")) groupFailure("GROUP_INVITE_REQUIRED");
  const encoded = text.slice("JobsIM-Invite-1:".length);
  let decoded: string, row: Record<string, unknown>;
  try { decoded = atob(encoded); if (btoa(decoded) !== encoded) groupFailure(); row = syncRecord(JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(Uint8Array.from(decoded, c => c.charCodeAt(0))))); } catch { groupFailure("GROUP_INVITE_REQUIRED"); }
  if (Object.keys(row!).some(key => !["deploymentId", "conversationId", "inviteId", "token"].includes(key)) || row!.deploymentId !== deploymentId || typeof row!.token !== "string" || row!.token.length > 256 || !row!.token.startsWith(String(row!.inviteId) + ".")) groupFailure("GROUP_INVITE_REQUIRED");
  return { deploymentId: syncID(row!.deploymentId), conversationId: syncID(row!.conversationId), inviteId: syncID(row!.inviteId), token: row!.token as string };
}

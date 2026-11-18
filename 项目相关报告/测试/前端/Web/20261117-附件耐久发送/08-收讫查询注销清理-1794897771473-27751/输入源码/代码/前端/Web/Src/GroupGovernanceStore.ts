// Created by Jobs on 2026年11月17日，星期二.
import { storageDatabase } from "./StorageDatabase";
import { guardCompletedErasure } from "./AccountDeletionCleanup";
import { passwordChangeIdentity, type PasswordChangeIdentity } from "./PasswordChangeProtocol";
import { groupFailure, validateGroupMutation, type GroupMutation } from "./GroupGovernanceProtocol";
import { syncCounter, syncID, syncRecord } from "./SyncProtocol";

export type GroupOperation = { identity: PasswordChangeIdentity; conversationId: string; requestId: string; kind: "governance" | "invite" | "join" | "revoke" | "decide"; originalBody: string; targetId?: string; createdAt: string; state: "unsent" | "unknown" | "applied" | "blocked"; attempts: number; result?: Record<string, unknown> };
export type GroupStoreContext = { identity: PasswordChangeIdentity; isCurrent: () => boolean };
export function validateGroupOperation(row: GroupOperation, context: GroupStoreContext) {
  if (!row || Object.keys(row).some(key => !["identity", "conversationId", "requestId", "kind", "originalBody", "targetId", "createdAt", "state", "attempts", "result"].includes(key)) || typeof row.originalBody !== "string" || row.originalBody.length > 16384 || typeof row.createdAt !== "string" || !Number.isFinite(Date.parse(row.createdAt)) || !["unsent", "unknown", "applied", "blocked"].includes(row.state) || !Number.isSafeInteger(row.attempts) || row.attempts < 0) groupFailure("GROUP_STORE_INVALID");
  passwordChangeIdentity(row.identity, context.identity.scopeId);
  if (row.identity.userId !== context.identity.userId || row.identity.username !== context.identity.username || Object.keys(row.identity).some(key => !["scopeId", "userId", "username", "deviceId", "epoch", "familyId", "namespace"].includes(key))) groupFailure("GROUP_CONTEXT_CHANGED");
  syncID(row.conversationId); syncID(row.requestId); if (row.targetId) syncID(row.targetId);
  const body = syncRecord(JSON.parse(row.originalBody));
  if (body.requestId !== row.requestId) groupFailure("GROUP_STORE_INVALID");
  const fields: Record<GroupOperation["kind"], string[]> = { governance: ["requestId", "version", "membershipGeneration", "roleGeneration", "action", "targetId", "targetMembershipGeneration", "targetRoleGeneration", "role", "mutedUntil", "muteAll", "slowSeconds", "maxMembers"], invite: ["requestId", "version", "membershipGeneration", "roleGeneration", "expiresAt", "maxUses", "approvalRequired"], join: ["requestId", "inviteId"], revoke: ["requestId", "version", "membershipGeneration", "roleGeneration"], decide: ["requestId", "version", "membershipGeneration", "roleGeneration", "approve"] };
  if (!fields[row.kind] || Object.keys(body).some(key => !fields[row.kind].includes(key))) groupFailure("GROUP_STORE_INVALID");
  if (row.kind === "governance") validateGroupMutation(body as GroupMutation);
  if (row.kind === "join") syncID(body.inviteId);
  else for (const key of ["version", "membershipGeneration", "roleGeneration"]) if (syncCounter(body[key]) === "0") groupFailure("GROUP_STORE_INVALID");
  if (row.kind === "invite" && (typeof body.expiresAt !== "string" || !Number.isFinite(Date.parse(body.expiresAt)) || !Number.isInteger(body.maxUses) || Number(body.maxUses) < 1 || Number(body.maxUses) > 10000 || typeof body.approvalRequired !== "boolean")) groupFailure("GROUP_STORE_INVALID");
  if (row.kind === "decide" && (typeof body.approve !== "boolean" || !row.targetId) || row.kind === "revoke" && !row.targetId) groupFailure("GROUP_STORE_INVALID");
  if (row.result && (Object.keys(row.result).some(key => !["status", "requestId", "action", "version", "targetId", "state", "membershipGeneration", "id", "expiresAt", "maxUses", "uses", "approvalRequired", "revokedAt"].includes(key)) || "token" in row.result)) groupFailure("GROUP_STORE_INVALID");
}
async function access<T>(context: GroupStoreContext, apply: (rows: GroupOperation[]) => T): Promise<T> {
  if (!context.isCurrent()) groupFailure("GROUP_CONTEXT_CHANGED");
  const db = await storageDatabase;
  if (!context.isCurrent()) groupFailure("GROUP_CONTEXT_CHANGED");
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), key = "group-operations|" + location.origin + "|" + context.identity.scopeId + "|" + context.identity.userId;
    guardCompletedErasure(store, context.identity.namespace);
    const get = store.get(key);
    let result: T, cause: unknown;
    get.onsuccess = () => {
      try {
        if (!context.isCurrent()) groupFailure("GROUP_CONTEXT_CHANGED");
        const rows: GroupOperation[] = get.result ?? [];
        if (!Array.isArray(rows) || rows.length > 10000 || new Set(rows.map(row => row.requestId)).size !== rows.length) groupFailure("GROUP_STORE_INVALID");
        rows.forEach(row => validateGroupOperation(row, context)); result = apply(rows); rows.forEach(row => validateGroupOperation(row, context));
        if (!context.isCurrent()) groupFailure("GROUP_CONTEXT_CHANGED"); store.put(rows, key);
      } catch (error) { cause = error; tx.abort(); }
    };
    tx.oncomplete = () => resolve(result!); tx.onabort = tx.onerror = () => reject(cause ?? tx.error ?? Error("GROUP_STORE_FAILED"));
  });
}
export const groupOperations = (context: GroupStoreContext) => access(context, rows => structuredClone(rows).reverse());
export function stageGroupOperation(context: GroupStoreContext, value: Omit<GroupOperation, "identity" | "createdAt" | "state" | "attempts">) {
  return access(context, rows => {
    if (rows.length >= 10000) groupFailure("GROUP_STORE_INVALID");
    const { scopeId, userId, username, deviceId, epoch, familyId, namespace } = context.identity;
    const row: GroupOperation = { ...value, identity: { scopeId, userId, username, deviceId, epoch, familyId, namespace }, createdAt: new Date().toISOString(), state: "unsent", attempts: 0 };
    rows.push(row); return structuredClone(row);
  });
}
export function updateGroupOperation(context: GroupStoreContext, original: GroupOperation, update: { state?: GroupOperation["state"]; attempt?: boolean; result?: Record<string, unknown> }) {
  return access(context, rows => {
    const row = rows.find(value => value.requestId === original.requestId);
    if (!row || row.originalBody !== original.originalBody || row.conversationId !== original.conversationId || row.kind !== original.kind || row.targetId !== original.targetId || JSON.stringify(row.identity) !== JSON.stringify(original.identity)) groupFailure("GROUP_CONTEXT_CHANGED");
    if (update.attempt) { row.attempts++; row.state = "unknown"; }
    if (update.state) row.state = update.state;
    if (update.result) row.result = update.result;
    return structuredClone(row);
  });
}

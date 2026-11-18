// Created by Jobs on 2026年11月16日，星期一.
import { syncCounter, syncID, SyncProtocolError } from "./SyncProtocol";

export type PasswordChangeIdentity = { scopeId: string; userId: string; username: string; deviceId: string; epoch: string; familyId: string; namespace: string };
export type PasswordChangeResult = { requestId: string; status: "pending" | "changed" | "retired"; restoreEpoch: string; credentialGeneration: string; securityVersion: string; current: boolean; requiresNewLogin: boolean };
export type PasswordChangeIntent = PasswordChangeIdentity & { requestId: string; createdAt: string; attempts: number; state: "unsent" | "unknown" | "pending" | "changed" | "retired" | "rejected"; result?: PasswordChangeResult };
export type PasswordChangeProof = { currentPassword: string; newPassword: string; otp?: string };
export function passwordChangeFailure(code = "PASSWORD_CHANGE_PROTOCOL_INVALID"): never { throw new SyncProtocolError(code); }
export function passwordChangeIdentity(identity: PasswordChangeIdentity, scope: string) {
  if (!identity || identity.scopeId !== scope || !/^[a-z0-9_]{3,32}$/.test(identity.username)) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
  for (const value of [identity.scopeId, identity.userId, identity.deviceId, identity.familyId]) syncID(value);
  if (syncCounter(identity.epoch) === "0" || identity.namespace !== [location.origin, scope, identity.userId, "personal", identity.epoch, identity.deviceId].join("|")) passwordChangeFailure("PASSWORD_CHANGE_CONTEXT_CHANGED");
}
export function passwordChangeResult(value: unknown, row: PasswordChangeIntent): PasswordChangeResult {
  const v = value as PasswordChangeResult;
  if (!v || v.requestId !== row.requestId || v.restoreEpoch !== row.epoch || !["pending", "changed", "retired"].includes(v.status) || typeof v.current !== "boolean" || typeof v.requiresNewLogin !== "boolean" || v.status === "changed" && !v.requiresNewLogin || v.current && v.status !== "changed") passwordChangeFailure();
  if (syncCounter(v.credentialGeneration) === "0" || syncCounter(v.securityVersion) === "0") passwordChangeFailure();
  return { requestId: v.requestId, status: v.status, restoreEpoch: v.restoreEpoch, credentialGeneration: v.credentialGeneration, securityVersion: v.securityVersion, current: v.current, requiresNewLogin: v.requiresNewLogin };
}
export function validatePasswordChangeIntent(row: PasswordChangeIntent, scope: string) {
  const keys = ["scopeId", "userId", "username", "deviceId", "epoch", "familyId", "namespace", "requestId", "createdAt", "attempts", "state", "result"];
  if (!row || Object.keys(row).some(key => !keys.includes(key))) passwordChangeFailure("PASSWORD_CHANGE_STORE_INVALID");
  passwordChangeIdentity(row, scope); syncID(row.requestId);
  if (typeof row.createdAt !== "string" || !Number.isFinite(Date.parse(row.createdAt)) || !Number.isSafeInteger(row.attempts) || row.attempts < 0 || !["unsent", "unknown", "pending", "changed", "retired", "rejected"].includes(row.state)) passwordChangeFailure("PASSWORD_CHANGE_STORE_INVALID");
  if (row.result) {
    const clean = passwordChangeResult(row.result, row);
    if (Object.keys(row.result).some(key => !(key in clean)) || row.state !== clean.status && row.state !== "unknown") passwordChangeFailure("PASSWORD_CHANGE_STORE_INVALID");
  }
}
export function normalizePasswordChangeProof(proof: PasswordChangeProof): PasswordChangeProof {
  const currentPassword = proof.currentPassword.normalize("NFC"), newPassword = proof.newPassword.normalize("NFC"), encoder = new TextEncoder();
  if (!currentPassword || encoder.encode(currentPassword).length > 512 || [...newPassword].length < 15 || [...newPassword].length > 128 || encoder.encode(newPassword).length > 512 || currentPassword === newPassword || proof.otp && !/^\d{6}$/.test(proof.otp)) passwordChangeFailure("PASSWORD_CHANGE_PROOF_INVALID");
  return { currentPassword, newPassword, ...(proof.otp ? { otp: proof.otp } : {}) };
}

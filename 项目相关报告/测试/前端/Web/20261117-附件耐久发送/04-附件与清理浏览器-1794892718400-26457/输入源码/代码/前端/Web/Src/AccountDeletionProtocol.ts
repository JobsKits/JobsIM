// Created by Jobs on 2026年11月17日，星期二.
import { syncCounter, syncID, SyncProtocolError } from "./SyncProtocol";
import { passwordChangeIdentity, type PasswordChangeIdentity } from "./PasswordChangeProtocol";

export type DeletionIdentity = PasswordChangeIdentity;
export type DeletionResult = { deletionId: string; requestId: string; status: "pending" | "requested" | "cancel_pending" | "processing" | "completed" | "cancelled" | "retired"; requestVersion: string; restoreEpoch: string; cancelUntil: string | null; cancelRequestId: string; requiresNewLogin: true };
export type DeletionCase = {
  caseId: string; scopeId: string; username: string; createdAt: string;
  original?: { identity: DeletionIdentity; requestId: string; attempts: number; state: "unsent" | "unknown" | "rejected" | "known" };
  observed?: { deletionId: string; requestVersion: string };
  result?: DeletionResult;
  cancellation?: { requestId: string; requestVersion: string; state: "unsent" | "unknown" | "confirmed" | "closed" };
};
export type DeletionCredential = { password?: string; recoveryCode?: string; otp?: string };
export type DataRightsProof = { proof: string; deletionId: string; requestVersion: string; purpose: "query" | "cancel"; expiresAt: string; restoreEpoch: string };
export function deletionFailure(code = "ACCOUNT_DELETION_PROTOCOL_INVALID"): never { throw new SyncProtocolError(code); }
export const deletionNonce = (value: unknown): value is string => typeof value === "string" && value.length >= 8 && value.length <= 128 && !/[\r\n\0]/.test(value);
export function deletionUsername(value: string) { const name = value.trim().toLowerCase(); if (!/^[a-z0-9_]{3,32}$/.test(name)) deletionFailure("ACCOUNT_DELETION_PROOF_INVALID"); return name; }
export function deletionCredential(raw: DeletionCredential, original = false): DeletionCredential {
  const password = raw.password?.normalize("NFC"), recoveryCode = raw.recoveryCode, otp = raw.otp;
  if (!!password === !!recoveryCode || original && !password || password && new TextEncoder().encode(password).length > 512 || recoveryCode && recoveryCode.length > 128 || otp && !/^\d{6}$/.test(otp)) deletionFailure("ACCOUNT_DELETION_PROOF_INVALID");
  return { ...(password ? { password } : { recoveryCode }), ...(otp ? { otp } : {}) };
}
export function deletionResult(raw: unknown, row: DeletionCase): DeletionResult {
  const value = raw as DeletionResult;
  if (!value || !deletionNonce(value.requestId) || !["pending", "requested", "cancel_pending", "processing", "completed", "cancelled", "retired"].includes(value.status) || value.requiresNewLogin !== true || value.cancelUntil !== null && (typeof value.cancelUntil !== "string" || !Number.isFinite(Date.parse(value.cancelUntil))) || typeof value.cancelRequestId !== "string" || value.cancelRequestId !== "" && !deletionNonce(value.cancelRequestId)) deletionFailure();
  syncID(value.deletionId);
  if (syncCounter(value.requestVersion) === "0" || syncCounter(value.restoreEpoch) === "0" || row.original && (value.requestId !== row.original.requestId || value.restoreEpoch !== row.original.identity.epoch) || row.observed && (value.deletionId !== row.observed.deletionId || BigInt(value.requestVersion) < BigInt(row.observed.requestVersion))) deletionFailure();
  if (value.status === "requested" && value.cancelUntil === null) deletionFailure();
  return { deletionId: value.deletionId, requestId: value.requestId, status: value.status, requestVersion: value.requestVersion, restoreEpoch: value.restoreEpoch, cancelUntil: value.cancelUntil, cancelRequestId: value.cancelRequestId, requiresNewLogin: true };
}
export function dataRightsProof(raw: unknown, row: DeletionCase, purpose: DataRightsProof["purpose"]): DataRightsProof {
  const value = raw as DataRightsProof, now = Date.now();
  if (!row.observed || !value || value.deletionId !== row.observed.deletionId || syncCounter(value.requestVersion) === "0" || purpose === "cancel" && value.requestVersion !== row.observed.requestVersion || purpose === "query" && BigInt(value.requestVersion) < BigInt(row.observed.requestVersion) || value.purpose !== purpose || typeof value.proof !== "string" || value.proof.length < 32 || value.proof.length > 128 || typeof value.expiresAt !== "string" || !Number.isFinite(Date.parse(value.expiresAt)) || Date.parse(value.expiresAt) <= now || Date.parse(value.expiresAt) > now + 310000 || syncCounter(value.restoreEpoch) === "0") deletionFailure("ACCOUNT_DELETION_PROOF_INVALID");
  return { proof: value.proof, deletionId: value.deletionId, requestVersion: value.requestVersion, purpose, expiresAt: value.expiresAt, restoreEpoch: value.restoreEpoch };
}
export function validateDeletionCase(row: DeletionCase, scope: string) {
  const keys = ["caseId", "scopeId", "username", "createdAt", "original", "observed", "result", "cancellation"];
  if (!row || Object.keys(row).some(key => !keys.includes(key)) || row.scopeId !== scope || deletionUsername(row.username) !== row.username || typeof row.createdAt !== "string" || !Number.isFinite(Date.parse(row.createdAt))) deletionFailure("ACCOUNT_DELETION_STORE_INVALID");
  syncID(row.scopeId); syncID(row.caseId);
  if (row.original) {
    const original = row.original;
    if (Object.keys(original).some(key => !["identity", "requestId", "attempts", "state"].includes(key)) || !deletionNonce(original.requestId) || !Number.isSafeInteger(original.attempts) || original.attempts < 0 || !["unsent", "unknown", "rejected", "known"].includes(original.state)) deletionFailure("ACCOUNT_DELETION_STORE_INVALID");
    passwordChangeIdentity(original.identity, scope);
    if (original.identity.username !== row.username || Object.keys(original.identity).some(key => !["scopeId", "userId", "username", "deviceId", "epoch", "familyId", "namespace"].includes(key))) deletionFailure("ACCOUNT_DELETION_STORE_INVALID");
  }
  if (!row.original && !row.observed) deletionFailure("ACCOUNT_DELETION_STORE_INVALID");
  if (row.observed) { syncID(row.observed.deletionId); if (syncCounter(row.observed.requestVersion) === "0" || Object.keys(row.observed).some(key => !["deletionId", "requestVersion"].includes(key))) deletionFailure("ACCOUNT_DELETION_STORE_INVALID"); }
  if (row.result) { const clean = deletionResult(row.result, row); if (!row.observed || Object.keys(row.result).some(key => !(key in clean))) deletionFailure("ACCOUNT_DELETION_STORE_INVALID"); }
  if (row.cancellation && (!row.observed || !deletionNonce(row.cancellation.requestId) || syncCounter(row.cancellation.requestVersion) === "0" || !["unsent", "unknown", "confirmed", "closed"].includes(row.cancellation.state) || Object.keys(row.cancellation).some(key => !["requestId", "requestVersion", "state"].includes(key)))) deletionFailure("ACCOUNT_DELETION_STORE_INVALID");
}

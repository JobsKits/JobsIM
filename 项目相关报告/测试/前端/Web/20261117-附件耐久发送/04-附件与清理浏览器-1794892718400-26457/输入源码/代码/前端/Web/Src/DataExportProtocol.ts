import { syncCounter, syncID, SyncProtocolError } from "./SyncProtocol";

export type ExportIdentity = { scopeId: string; userId: string; deviceId: string; epoch: string };
export type ExportState = "requested" | "building" | "ready" | "cancelled" | "expired" | "failed";
export type ExportView = {
  job: { id: string; artifactId: string; restoreEpoch: string; buildGeneration: string; state: ExportState; expiresAt: string; cleanupState: string; failureCode?: string };
  format: "zip"; scope: "personal_current"; capturedAt?: string; coverage: string[]; exclusions: string[];
};
export type ExportIntent = {
  requestId: string; originalBody: string; bodySHA256: string; createdAt: string; jobId?: string;
  cancelBody?: string; cancelSHA256?: string;
};
export function exportFailure(code = "DATA_EXPORT_PROTOCOL_INVALID"): never {
  throw new SyncProtocolError(code);
}
export function exportTime(value: unknown): string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}T.*(?:Z|[+-]\d\d:\d\d)$/.test(value) || !Number.isFinite(Date.parse(value))) exportFailure();
  return value as string;
}
export function exportView(value: unknown, identity: ExportIdentity, expectedJob?: string): ExportView {
  const v = value as ExportView;
  if (!v || v.format !== "zip" || v.scope !== "personal_current" || !v.job) exportFailure();
  const j = v.job;
  syncID(j.id); syncID(j.artifactId); syncCounter(j.restoreEpoch); syncCounter(j.buildGeneration);
  if (j.restoreEpoch === "0" || j.buildGeneration === "0" || j.restoreEpoch !== identity.epoch || expectedJob && j.id !== expectedJob || !["requested", "building", "ready", "cancelled", "expired", "failed"].includes(j.state) || typeof j.cleanupState !== "string" || !/^[a-z_]{1,48}$/.test(j.cleanupState)) exportFailure();
  exportTime(j.expiresAt); if (v.capturedAt !== undefined) exportTime(v.capturedAt);
  for (const list of [v.coverage, v.exclusions]) if (!Array.isArray(list) || list.length > 64 || list.some(item => typeof item !== "string" || !/^[a-z_]{1,100}$/.test(item))) exportFailure();
  if (j.failureCode !== undefined && !/^[A-Z0-9_]{1,64}$/.test(j.failureCode)) exportFailure();
  // Only declared metadata is returned. Unknown response fields are never stored.
  return { format: "zip", scope: "personal_current", job: { id: j.id, artifactId: j.artifactId, restoreEpoch: j.restoreEpoch, buildGeneration: j.buildGeneration, state: j.state, expiresAt: j.expiresAt, cleanupState: j.cleanupState, ...(j.failureCode ? { failureCode: j.failureCode } : {}) }, capturedAt: v.capturedAt, coverage: [...v.coverage], exclusions: [...v.exclusions] };
}
export async function exportDigest(raw: string): Promise<string> {
  return [...new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)))].map(v => v.toString(16).padStart(2, "0")).join("");
}
export function validateExportIntent(row: ExportIntent, identity: ExportIdentity): ExportIntent {
  if (!row || Object.keys(row).some(key => !["requestId", "originalBody", "bodySHA256", "createdAt", "jobId", "cancelBody", "cancelSHA256"].includes(key))) exportFailure("DATA_EXPORT_STORE_INVALID");
  syncID(row.requestId); exportTime(row.createdAt);
  if (typeof row.originalBody !== "string" || row.originalBody.length > 1024 || !/^[a-f0-9]{64}$/.test(row.bodySHA256)) exportFailure("DATA_EXPORT_STORE_INVALID");
  const body = JSON.parse(row.originalBody);
  if (Object.keys(body).sort().join() !== "requestId,restoreEpoch" || body.requestId !== row.requestId || body.restoreEpoch !== identity.epoch) exportFailure("DATA_EXPORT_STORE_INVALID");
  if (row.jobId) syncID(row.jobId);
  if ((row.cancelBody === undefined) !== (row.cancelSHA256 === undefined)) exportFailure("DATA_EXPORT_STORE_INVALID");
  if (row.cancelBody !== undefined) {
    if (row.cancelBody.length > 512 || !/^[a-f0-9]{64}$/.test(row.cancelSHA256!)) exportFailure("DATA_EXPORT_STORE_INVALID");
    const cancel = JSON.parse(row.cancelBody);
    if (Object.keys(cancel).join() !== "requestId") exportFailure("DATA_EXPORT_STORE_INVALID");
    syncID(cancel.requestId);
  }
  return row;
}

import { storageDatabase } from "./StorageDatabase";
import { exportDigest, exportFailure, validateExportIntent, type ExportIdentity, type ExportIntent } from "./DataExportProtocol";

export type ExportStoreContext = { namespace: string; identity: ExportIdentity; isCurrent: () => boolean };
function binding(context: ExportStoreContext) {
  const i = context.identity;
  if (context.namespace !== [location.origin, i.scopeId, i.userId, "personal", i.epoch, i.deviceId].join("|")) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
}
async function access<T>(context: ExportStoreContext, change: (rows: ExportIntent[]) => T): Promise<T> {
  binding(context); if (!context.isCurrent()) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
  const db = await storageDatabase;
  if (!context.isCurrent()) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
  return new Promise<T>((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces");
    const active = store.get("auth-active|" + context.namespace.split("|").slice(0, 4).join("|"));
    const frozen = store.get("auth-frozen|" + context.namespace);
    const request = store.get("data-exports|" + context.namespace);
    let value: T, cause: unknown;
    request.onsuccess = () => {
      try {
        if (!context.isCurrent() || frozen.result || active.result !== context.namespace) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
        const rows: ExportIntent[] = request.result ?? [];
        if (!Array.isArray(rows) || rows.length > 10000 || new Set(rows.map(row => row.requestId)).size !== rows.length) exportFailure("DATA_EXPORT_STORE_INVALID");
        rows.forEach(row => validateExportIntent(row, context.identity));
        value = change(rows);
        if (!context.isCurrent()) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
        store.put(rows, "data-exports|" + context.namespace);
      } catch (error) { cause = error; tx.abort(); }
    };
    tx.oncomplete = () => resolve(value!);
    tx.onerror = () => reject(cause ?? tx.error);
    tx.onabort = () => reject(cause ?? tx.error ?? Error("DATA_EXPORT_STORE_FAILED"));
  });
}
export async function exportIntents(context: ExportStoreContext) {
  return access(context, rows => structuredClone(rows).sort((a, b) => b.createdAt.localeCompare(a.createdAt)));
}
export async function addExportIntent(context: ExportStoreContext): Promise<ExportIntent> {
  const requestId = crypto.randomUUID(), originalBody = JSON.stringify({ requestId, restoreEpoch: context.identity.epoch });
  const row = { requestId, originalBody, bodySHA256: await exportDigest(originalBody), createdAt: new Date().toISOString() };
  return access(context, rows => {
    if (rows.some(item => !item.jobId)) exportFailure("DATA_EXPORT_ORIGINAL_PENDING");
    rows.push(row); return structuredClone(row);
  });
}
export async function originalExportIntent(context: ExportStoreContext, id: string): Promise<ExportIntent> {
  const row = (await exportIntents(context)).find(item => item.requestId === id);
  if (!row || await exportDigest(row.originalBody) !== row.bodySHA256 || row.cancelBody && await exportDigest(row.cancelBody) !== row.cancelSHA256) exportFailure("DATA_EXPORT_STORE_INVALID");
  if (!context.isCurrent()) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
  return row!;
}
export async function bindExportJob(context: ExportStoreContext, original: ExportIntent, jobId: string) {
  return access(context, rows => {
    const row = rows.find(item => item.requestId === original.requestId);
    if (!row || row.originalBody !== original.originalBody || row.bodySHA256 !== original.bodySHA256 || row.jobId && row.jobId !== jobId) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
    row.jobId = jobId;
  });
}
export async function cancelExportIntent(context: ExportStoreContext, original: ExportIntent): Promise<ExportIntent> {
  if (original.cancelBody) return original;
  const cancelBody = JSON.stringify({ requestId: crypto.randomUUID() }), cancelSHA256 = await exportDigest(cancelBody);
  return access(context, rows => {
    const row = rows.find(item => item.requestId === original.requestId);
    if (!row || row.originalBody !== original.originalBody || row.jobId !== original.jobId || !row.jobId) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
    if (!row.cancelBody) { row.cancelBody = cancelBody; row.cancelSHA256 = cancelSHA256; }
    return structuredClone(row);
  });
}

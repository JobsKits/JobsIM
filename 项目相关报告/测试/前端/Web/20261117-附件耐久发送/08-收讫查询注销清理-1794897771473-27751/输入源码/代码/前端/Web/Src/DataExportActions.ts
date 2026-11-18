import { api, apiFile, ApiError, json } from "./api";
import { bindExportJob, cancelExportIntent, exportIntents, originalExportIntent, addExportIntent, type ExportStoreContext } from "./DataExportStore";
import { exportFailure, exportTime, exportView, type ExportView, type ExportIntent } from "./DataExportProtocol";

export type DataExportContext = ExportStoreContext & {
  request?: typeof api; file?: typeof apiFile; signal?: AbortSignal;
};
export class DataExportActions {
  constructor(readonly context: DataExportContext) {}
  private current() { if (!this.context.isCurrent()) exportFailure("DATA_EXPORT_CONTEXT_CHANGED"); }
  private async request<T>(path: string, options?: RequestInit): Promise<T> {
    this.current(); const value = await (this.context.request ?? api)<T>(path, { ...options, signal: this.context.signal }, 15000); this.current(); return value;
  }
  private async capability(required: boolean) {
    const b = await this.request<{ deploymentId: string; restoreEpoch: string; features: Record<string, boolean> }>("/bootstrap");
    if (b.deploymentId !== this.context.identity.scopeId || b.restoreEpoch !== this.context.identity.epoch) exportFailure("DATA_EXPORT_CONTEXT_CHANGED");
    if (required && b.features?.chat_export !== true) exportFailure("DATA_EXPORT_UNAVAILABLE");
  }
  private async locked<T>(action: () => Promise<T>): Promise<T> {
    this.current(); if (!navigator.locks) exportFailure("DATA_EXPORT_LOCK_UNAVAILABLE");
    return navigator.locks.request("jobsim-data-export|" + this.context.namespace, { mode: "exclusive", signal: this.context.signal }, async () => { this.current(); return action(); });
  }
  rows() { return exportIntents(this.context); }
  private async lookup(row: ExportIntent): Promise<ExportView | undefined> {
    try {
      const value = exportView(await this.request("/me/data-export-results", { method: "POST", body: row.originalBody }), this.context.identity, row.jobId);
      await bindExportJob(this.context, row, value.job.id); this.current(); return value;
    } catch (error) {
      if (error instanceof ApiError && error.status === 404 && error.code === "EXPORT_OPERATION_NOT_FOUND") return undefined;
      throw error;
    }
  }
  async create(): Promise<ExportView | undefined> {
    return this.locked(async () => {
      await this.capability(true);
      const row = await addExportIntent(this.context);
      const existing = await this.lookup(row);
      if (existing) return existing;
      const value = exportView(await this.request("/me/data-exports", { method: "POST", body: row.originalBody }), this.context.identity);
      await bindExportJob(this.context, row, value.job.id); this.current(); return value;
    });
  }
  async check(id: string, retry = false): Promise<ExportView | undefined> {
    return this.locked(async () => {
      await this.capability(retry);
      const row = await originalExportIntent(this.context, id), existing = await this.lookup(row);
      if (existing || !retry || row.jobId) return existing;
      const value = exportView(await this.request("/me/data-exports", { method: "POST", body: row.originalBody }), this.context.identity);
      await bindExportJob(this.context, row, value.job.id); this.current(); return value;
    });
  }
  async status(id: string): Promise<ExportView | undefined> {
    const row = await originalExportIntent(this.context, id);
    if (!row.jobId) return this.check(id, false);
    return exportView(await this.request("/me/data-exports/" + row.jobId), this.context.identity, row.jobId);
  }
  async cancel(id: string): Promise<ExportView> {
    return this.locked(async () => {
      await this.capability(false);
      const original = await originalExportIntent(this.context, id);
      const row = await cancelExportIntent(this.context, original);
      return exportView(await this.request("/me/data-exports/" + row.jobId + "/cancel", { method: "POST", body: row.cancelBody }), this.context.identity, row.jobId);
    });
  }
  async download(id: string): Promise<{ blob: Blob; fileName: string }> {
    return this.locked(async () => {
      await this.capability(true);
      const row = await originalExportIntent(this.context, id);
      if (!row.jobId || row.cancelBody) exportFailure("DATA_EXPORT_UNAVAILABLE");
      const path = "/me/data-exports/" + row.jobId;
      const live = exportView(await this.request(path), this.context.identity, row.jobId);
      if (live.job.state !== "ready" || live.job.cleanupState !== "published" || Date.parse(live.job.expiresAt) <= Date.now()) exportFailure("DATA_EXPORT_UNAVAILABLE");
      const ticket = await this.request<{ downloadToken: string; expiresAt: string; format: string }>(path + "/download-ticket", json("POST", {}));
      const deadline = Date.parse(exportTime(ticket.expiresAt));
      if (ticket.format !== "zip" || typeof ticket.downloadToken !== "string" || !/^[A-Za-z0-9_-]{16,256}$/.test(ticket.downloadToken) || deadline <= Date.now() || deadline > Date.now() + 65000 || deadline > Date.parse(live.job.expiresAt)) exportFailure();
      this.current();
      const file = await (this.context.file ?? apiFile)(path + "/download", { headers: { "X-JobsIM-Download-Token": ticket.downloadToken }, signal: this.context.signal }, 120000);
      this.current(); return file;
    });
  }
}

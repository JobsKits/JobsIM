// Created by Jobs on 2026年11月16日，星期一.
import { ApiFailure, Bootstrap } from './Models';
import { SyncCodec, SyncRules } from './SyncProtocol';
import { ExportIntent, ExportView, ExportPrepared, ExportRules, ExportRepository } from './ExportProtocol';

export interface ExportPorts {
  deployment: string; epoch: string; physical: string; repository: ExportRepository; codec: SyncCodec;
  uuid: () => string; current: () => boolean;
  request: (path: string, method: string, body: string) => Promise<string>;
  download: (job: string, token: string) => Promise<ExportPrepared>;
}
export class ExportService {
  private ports: ExportPorts;
  constructor(ports: ExportPorts) { this.ports = ports; }
  check(): void { if (!this.ports.current()) { ExportRules.fail('EXPORT_CONTEXT_CHANGED'); } }
  private async call(path: string, method: string = 'GET', body: string = ''): Promise<string> { this.check(); const result = await this.ports.request(path, method, body); this.check(); return result; }
  async capability(write: boolean): Promise<boolean> {
    const raw = await this.call('/bootstrap'); const boot = JSON.parse(raw) as Bootstrap;
    if (boot.deploymentId !== this.ports.deployment || boot.restoreEpoch !== this.ports.epoch) { ExportRules.fail('RESTORE_EPOCH_CHANGED'); }
    if (write && boot.features?.chat_export !== true) { ExportRules.fail('EXPORT_CAPABILITY_REQUIRED'); }
    return boot.features?.chat_export === true;
  }
  async list(): Promise<ExportIntent[]> { this.check(); const rows = await this.ports.repository.list(); this.check(); return rows; }
  private async original(id: string): Promise<ExportIntent> {
    const row = (await this.list()).find((item: ExportIntent) => item.id === id); if (!row) { ExportRules.fail('EXPORT_ORIGINAL_CHANGED'); }
    ExportRules.original(row, this.ports.epoch, this.ports.physical);
    if (await this.ports.codec.sha256(this.ports.codec.bytes(row.body)) !== row.hash || row.cancelBody && await this.ports.codec.sha256(this.ports.codec.bytes(row.cancelBody)) !== row.cancelHash) { ExportRules.fail('EXPORT_ORIGINAL_CHANGED'); } this.check(); return row;
  }
  private clone(row: ExportIntent): ExportIntent { return JSON.parse(JSON.stringify(row)) as ExportIntent; }
  async create(): Promise<ExportView> {
    await this.capability(true); const id = this.ports.uuid(); if (!SyncRules.uuid(id)) { ExportRules.fail('EXPORT_ORIGINAL_CHANGED'); }
    const body = JSON.stringify({ requestId: id, restoreEpoch: this.ports.epoch });
    const row: ExportIntent = { id: id, body: body, hash: await this.ports.codec.sha256(this.ports.codec.bytes(body)), created: new Date().toISOString(), physical: this.ports.physical, job: '', cancelBody: '', cancelHash: '' };
    this.check(); await this.ports.repository.insert(row); this.check(); return this.resume(id, true);
  }
  async resume(id: string, explicit: boolean): Promise<ExportView> {
    const row = await this.original(id); await this.capability(false); let raw = '';
    try { raw = await this.call('/me/data-export-results', 'POST', row.body); }
    catch (error) {
      if (!(error instanceof ApiFailure) || error.status !== 404 || error.code !== 'EXPORT_OPERATION_NOT_FOUND') { throw error as Error; }
      if (!explicit || row.job) { ExportRules.fail('EXPORT_RESULT_NOT_FOUND'); }
      await this.capability(true); raw = await this.call('/me/data-exports', 'POST', row.body);
    }
    const view = ExportRules.view(raw, this.ports.epoch, row.job); const next = this.clone(row); next.job = view.id;
    await this.ports.repository.replace(row, next); this.check(); return view;
  }
  async status(id: string): Promise<ExportView> {
    await this.capability(false); const row = await this.original(id); if (!row.job) { return this.resume(id, false); }
    return ExportRules.view(await this.call('/me/data-exports/' + encodeURIComponent(row.job)), this.ports.epoch, row.job);
  }
  async cancel(id: string): Promise<ExportView> {
    await this.capability(false); let row = await this.original(id);
    if (!row.job) { await this.resume(id, false); row = await this.original(id); }
    if (!row.cancelBody) {
      const next = this.clone(row); const nonce = this.ports.uuid(); if (!SyncRules.uuid(nonce) || nonce === id) { ExportRules.fail('EXPORT_ORIGINAL_CHANGED'); }
      next.cancelBody = JSON.stringify({ requestId: nonce }); next.cancelHash = await this.ports.codec.sha256(this.ports.codec.bytes(next.cancelBody));
      await this.ports.repository.replace(row, next); row = next;
    }
    // Cancellation has no separate result route. Reuse its original durable nonce.
    const raw = await this.call('/me/data-exports/' + encodeURIComponent(row.job) + '/cancel', 'POST', row.cancelBody);
    return ExportRules.view(raw, this.ports.epoch, row.job);
  }
  async delivery(id: string): Promise<ExportPrepared> {
    await this.capability(true); const row = await this.original(id); const view = await this.status(id);
    if (row.cancelBody || !ExportRules.ready(view)) { ExportRules.fail('EXPORT_NOT_READY'); }
    const ticket = ExportRules.ticket(await this.call('/me/data-exports/' + encodeURIComponent(view.id) + '/download-ticket', 'POST', '{}'), view);
    this.check(); return this.ports.download(view.id, ticket.downloadToken);
  }
  async authorizeSave(id: string, prepared: ExportPrepared): Promise<void> {
    await this.capability(true); const row = await this.original(id); const view = await this.status(id);
    if (row.cancelBody || view.id !== prepared.job || !ExportRules.ready(view)) { ExportRules.fail('EXPORT_NOT_READY'); }
    ExportRules.ticket(await this.call('/me/data-exports/' + encodeURIComponent(view.id) + '/download-ticket', 'POST', '{}'), view); this.check();
  }
}

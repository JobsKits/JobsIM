// Created by Jobs on 2026年11月16日，星期一.
import relationalStore from '@ohos.data.relationalStore';
import { LocalStore } from './Store';
import { SyncCodec, SyncRules } from './SyncProtocol';
import { ExportIntent, ExportRules, ExportRepository } from './ExportProtocol';

export class ExportStore implements ExportRepository {
  private store: LocalStore; private scope: string; private epoch: string; private physical: string; private codec: SyncCodec; private current: () => boolean;
  constructor(store: LocalStore, scope: string, epoch: string, physical: string, codec: SyncCodec, current: () => boolean) { this.store = store; this.scope = scope; this.epoch = epoch; this.physical = physical; this.codec = codec; this.current = current; }
  private check(): void { if (!this.current()) { ExportRules.fail('EXPORT_CONTEXT_CHANGED'); } }
  private async guard(db: relationalStore.RdbStore): Promise<void> {
    this.check(); const result = await db.querySql("SELECT value FROM local_state WHERE namespace='__sync_store__' AND key='physical'");
    try { if (!result.goToNextRow() || result.getString(0) !== this.physical || !SyncRules.uuid(this.physical)) { ExportRules.fail('EXPORT_STORE_CHANGED'); } } finally { result.close(); }
  }
  private async checked(row: ExportIntent): Promise<void> { ExportRules.original(row, this.epoch, this.physical); if (await this.codec.sha256(this.codec.bytes(row.body)) !== row.hash || row.cancelBody && await this.codec.sha256(this.codec.bytes(row.cancelBody)) !== row.cancelHash) { ExportRules.fail('EXPORT_ORIGINAL_CHANGED'); } }
  async open(): Promise<void> { await this.store.syncAccess(async (db: relationalStore.RdbStore) => { await this.guard(db); await db.executeSql('CREATE TABLE IF NOT EXISTS export_intents(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))'); }); }
  async list(): Promise<ExportIntent[]> {
    return this.store.syncAccess(async (db: relationalStore.RdbStore) => {
      await this.guard(db); const result = await db.querySql('SELECT payload FROM export_intents WHERE namespace=? ORDER BY id DESC', [this.scope]); const rows: ExportIntent[] = [];
      try { while (result.goToNextRow()) { const row = JSON.parse(result.getString(0)) as ExportIntent; await this.checked(row); rows.push(row); } } finally { result.close(); }
      this.check(); return rows.sort((a: ExportIntent, b: ExportIntent) => b.created.localeCompare(a.created));
    });
  }
  async insert(row: ExportIntent): Promise<void> { await this.checked(row); await this.mutate(async (db: relationalStore.RdbStore) => { await db.executeSql('INSERT INTO export_intents VALUES(?,?,?)', [this.scope, row.id, JSON.stringify(row)]); }); }
  async replace(before: ExportIntent, after: ExportIntent): Promise<void> {
    await this.checked(after);
    if (before.id !== after.id || before.body !== after.body || before.hash !== after.hash || before.created !== after.created || before.physical !== after.physical || before.job && before.job !== after.job || before.cancelBody && (before.cancelBody !== after.cancelBody || before.cancelHash !== after.cancelHash)) { ExportRules.fail('EXPORT_ORIGINAL_CHANGED'); }
    await this.mutate(async (db: relationalStore.RdbStore) => {
      const result = await db.querySql('SELECT payload FROM export_intents WHERE namespace=? AND id=?', [this.scope, before.id]);
      try { if (!result.goToNextRow() || result.getString(0) !== JSON.stringify(before)) { ExportRules.fail('EXPORT_ORIGINAL_CHANGED'); } } finally { result.close(); }
      await db.executeSql('UPDATE export_intents SET payload=? WHERE namespace=? AND id=?', [JSON.stringify(after), this.scope, before.id]);
    });
  }
  private async mutate(action: (db: relationalStore.RdbStore) => Promise<void>): Promise<void> {
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => { db.beginTransaction(); try { await this.guard(db); await action(db); this.check(); db.commit(); } catch (error) { db.rollBack(); throw error as Error; } });
  }
}

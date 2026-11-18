// Created by Jobs on 2026年11月17日，星期二.
import relationalStore from '@ohos.data.relationalStore';
import { LocalStore } from './Store';
import { Isolation } from './Models';
import { DeletionIntent, DeletionRules, DeletionRepository } from './Deletion';

export interface DeletionLocalPlan { namespaces: string[]; files: string[]; }
interface DeletionPath { path?: string; }
export class DeletionStore implements DeletionRepository {
  private store: LocalStore; private profile: string; private current: () => boolean; private changed: (row: DeletionIntent) => void;
  constructor(store: LocalStore, profile: string, current: () => boolean, changed: (row: DeletionIntent) => void = (_row: DeletionIntent) => {}) { this.store = store; this.profile = profile; this.current = current; this.changed = changed; }
  private check(): void { if (!this.current()) { DeletionRules.fail('DELETION_CONTEXT_CHANGED'); } }
  async open(): Promise<void> { await this.store.syncAccess(async (db: relationalStore.RdbStore) => { this.check(); await db.executeSql('CREATE TABLE IF NOT EXISTS account_deletion_intents(profile TEXT,id TEXT,payload TEXT,PRIMARY KEY(profile,id))'); }); }
  private async read(db: relationalStore.RdbStore): Promise<DeletionIntent[]> {
    const query = await db.querySql('SELECT payload FROM account_deletion_intents WHERE profile=?', [this.profile]); const rows: DeletionIntent[] = [];
    try { while (query.goToNextRow()) { const row = JSON.parse(query.getString(0)) as DeletionIntent; DeletionRules.validate(row); if (row.meta.profile !== this.profile) { DeletionRules.fail(); } rows.push(row); } } finally { query.close(); }
    this.check(); return rows.sort((a: DeletionIntent, b: DeletionIntent) => b.meta.created.localeCompare(a.meta.created));
  }
  async list(): Promise<DeletionIntent[]> { return this.store.syncAccess(async (db: relationalStore.RdbStore) => this.read(db)); }
  async insert(row: DeletionIntent): Promise<void> {
    DeletionRules.validate(row);
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => { db.beginTransaction(); try {
      const rows = await this.read(db); if (rows.some((old: DeletionIntent) => old.meta.family === row.meta.family && !['rejected', 'retired', 'cancelled', 'completed'].includes(old.state))) { DeletionRules.fail('DELETION_ORIGINAL_PENDING'); }
      await db.executeSql('INSERT INTO account_deletion_intents VALUES(?,?,?)', [this.profile, row.meta.id, JSON.stringify(row)]); this.check(); db.commit();
    } catch (error) { db.rollBack(); throw error as Error; } });
  }
  async replace(before: DeletionIntent, after: DeletionIntent): Promise<void> {
    DeletionRules.validate(after);
    if (after.attempts < before.attempts || JSON.stringify(before.meta) !== JSON.stringify(after.meta) || before.cancelId && (before.cancelId !== after.cancelId || before.cancelVersion !== after.cancelVersion)) { DeletionRules.fail(); }
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => { db.beginTransaction(); try {
      const row = (await this.read(db)).find((item: DeletionIntent) => item.meta.id === before.meta.id); if (!row || JSON.stringify(row) !== JSON.stringify(before)) { DeletionRules.fail('DELETION_ORIGINAL_CHANGED'); }
      await db.executeSql('UPDATE account_deletion_intents SET payload=? WHERE profile=? AND id=?', [JSON.stringify(after), this.profile, before.meta.id]);
      const scope = Isolation.namespace(after.meta.profile, after.meta.deployment, '__auth__', false, after.meta.epoch, 'metadata', 'personal');
      await db.executeSql('INSERT OR REPLACE INTO local_state(namespace,key,value) VALUES(?,?,?)', [scope, 'account-deletion:' + after.meta.family, DeletionRules.blocked(after) ? after.meta.id : '']);
      this.check(); db.commit(); this.changed(after);
    } catch (error) { db.rollBack(); throw error as Error; } });
  }
  private async values(db: relationalStore.RdbStore, sql: string, values: string[]): Promise<string[]> {
    const rows = await db.querySql(sql, values); const result: string[] = [];
    try { while (rows.goToNextRow()) { result.push(rows.getString(0)); } } finally { rows.close(); } return result;
  }
  private tables(): string[] { return ['messages', 'local_state', 'mention_receipts', 'sync_stages', 'sync_refs', 'sync_objects', 'sync_events', 'sync_retained', 'sync_retained_conversations', 'sync_eligibility', 'sync_quarantine', 'sync_unread', 'sync_stream_archive', 'self_operations', 'sync_originals', 'sync_cleanup', 'group_action_intents']; }
  async eraseLocal(row: DeletionIntent, clearFiles: (plan: DeletionLocalPlan) => Promise<void>): Promise<void> {
    DeletionRules.validate(row); if (row.state !== 'completed' || row.meta.profile !== this.profile) { DeletionRules.fail(); }
    const prefix = 'live|' + row.meta.profile + '|' + row.meta.deployment + '|' + row.meta.user + '|';
    const plan = await this.store.syncAccess(async (db: relationalStore.RdbStore) => {
      const spaces = [row.meta.namespace]; const files: string[] = [];
      for (const table of this.tables()) {
        if (!(await this.values(db, 'SELECT name FROM sqlite_master WHERE type=? AND name=?', ['table', table])).length) { continue; }
        spaces.push(...await this.values(db, 'SELECT DISTINCT namespace FROM ' + table + ' WHERE substr(namespace,1,length(?))=?', [prefix, prefix]));
      }
      const rows = await db.querySql('SELECT key,value FROM local_state WHERE substr(namespace,1,length(?))=?', [prefix, prefix]);
      try { while (rows.goToNextRow()) {
        const key = rows.getString(0); const value = rows.getString(1);
        if (key.startsWith('media-sidecar:') || key.startsWith('forward-sidecar:') || key.startsWith('own-avatar-sidecar')) { if (value) { files.push(value); } }
        if (key === 'avatar' && value) { const pending = JSON.parse(value) as DeletionPath; if (pending.path) { files.push(pending.path); } }
      } } finally { rows.close(); }
      if ((await this.values(db, 'SELECT name FROM sqlite_master WHERE type=? AND name=?', ['table', 'sync_cleanup'])).length) { files.push(...await this.values(db, 'SELECT path FROM sync_cleanup WHERE substr(namespace,1,length(?))=?', [prefix, prefix])); }
      this.check(); const output: DeletionLocalPlan = { namespaces: Array.from(new Set(spaces)), files: Array.from(new Set(files)) }; return output;
    });
    if (plan.namespaces.some((space: string) => !space.startsWith(prefix))) { DeletionRules.fail(); }
    await clearFiles(plan); this.check();
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => { db.beginTransaction(); try {
      for (const table of this.tables()) {
        if (!(await this.values(db, 'SELECT name FROM sqlite_master WHERE type=? AND name=?', ['table', table])).length) { continue; }
        await db.executeSql('DELETE FROM ' + table + ' WHERE substr(namespace,1,length(?))=?', [prefix, prefix]);
      }
      this.check(); db.commit();
    } catch (error) { db.rollBack(); throw error as Error; } });
  }
}

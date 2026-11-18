// Created by Jobs on 2026年11月17日，星期二.
import relationalStore from '@ohos.data.relationalStore';
import { LocalStore } from './Store';
import { GroupEnvironment, GroupIntent, GroupRules, GroupRepository } from './GroupTools';

export class GroupStore implements GroupRepository {
  private store: LocalStore; private environment: GroupEnvironment; private current: () => boolean;
  constructor(store: LocalStore, environment: GroupEnvironment, current: () => boolean) { this.store = store; this.environment = environment; this.current = current; }
  private check(): void { if (!this.current()) { GroupRules.fail('GROUP_CONTEXT_CHANGED'); } }
  private prefix(): string { const e = this.environment; return 'live|' + e.profile + '|' + e.deployment + '|' + e.user + '|'; }
  async open(): Promise<void> { await this.store.syncAccess(async (db: relationalStore.RdbStore) => { await db.executeSql('CREATE TABLE IF NOT EXISTS group_action_intents(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))'); }); }
  private async read(db: relationalStore.RdbStore): Promise<GroupIntent[]> {
    const prefix = this.prefix(); const rows = await db.querySql('SELECT namespace,payload FROM group_action_intents WHERE substr(namespace,1,length(?))=?', [prefix, prefix]); const result: GroupIntent[] = [];
    try { while (rows.goToNextRow()) { const row = JSON.parse(rows.getString(1)) as GroupIntent; GroupRules.validate(row); if (row.namespace !== rows.getString(0)) { GroupRules.fail(); } result.push(row); } } finally { rows.close(); }
    this.check(); return result.sort((a: GroupIntent, b: GroupIntent) => b.created.localeCompare(a.created));
  }
  async list(): Promise<GroupIntent[]> { return this.store.syncAccess(async (db: relationalStore.RdbStore) => this.read(db)); }
  async insert(row: GroupIntent): Promise<void> {
    GroupRules.validate(row); if (!row.namespace.startsWith(this.prefix())) { GroupRules.fail(); }
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => { this.check(); await db.executeSql('INSERT INTO group_action_intents VALUES(?,?,?)', [row.namespace, row.id, JSON.stringify(row)]); });
  }
  async replace(before: GroupIntent, after: GroupIntent): Promise<void> {
    GroupRules.validate(after); const a = JSON.parse(JSON.stringify(before)) as GroupIntent; const b = JSON.parse(JSON.stringify(after)) as GroupIntent;
    a.attempts = b.attempts = 0; a.state = b.state = ''; a.result = b.result = ''; if (JSON.stringify(a) !== JSON.stringify(b)) { GroupRules.fail(); }
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => { db.beginTransaction(); try {
      const found = (await this.read(db)).find((row: GroupIntent) => row.id === before.id && row.namespace === before.namespace);
      if (!found || JSON.stringify(found) !== JSON.stringify(before)) { GroupRules.fail('GROUP_ORIGINAL_CHANGED'); }
      await db.executeSql('UPDATE group_action_intents SET payload=? WHERE namespace=? AND id=?', [JSON.stringify(after), before.namespace, before.id]); this.check(); db.commit();
    } catch (error) { db.rollBack(); throw error as Error; } });
  }
}

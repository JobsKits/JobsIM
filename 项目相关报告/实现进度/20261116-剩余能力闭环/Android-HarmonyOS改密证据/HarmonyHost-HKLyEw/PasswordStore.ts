// Created by Jobs on 2026年11月16日，星期一.
import relationalStore from '@ohos.data.relationalStore';
import { LocalStore } from './Store';
import { Isolation } from './Models';
import { PasswordIntent, PasswordRules, PasswordRepository } from './PasswordChange';

export class PasswordStore implements PasswordRepository {
  private store: LocalStore; private profile: string; private current: () => boolean; private changed: (row: PasswordIntent) => void;
  constructor(store: LocalStore, profile: string, current: () => boolean, changed: (row: PasswordIntent) => void = (_row: PasswordIntent) => {}) { this.store = store; this.profile = profile; this.current = current; this.changed = changed; }
  private check(): void { if (!this.current()) { PasswordRules.fail('PASSWORD_CHANGE_CONTEXT_CHANGED'); } }
  async open(): Promise<void> { await this.store.syncAccess(async (db: relationalStore.RdbStore) => { this.check(); await db.executeSql('CREATE TABLE IF NOT EXISTS password_change_intents(profile TEXT,id TEXT,payload TEXT,PRIMARY KEY(profile,id))'); }); }
  private async read(db: relationalStore.RdbStore): Promise<PasswordIntent[]> {
    const result = await db.querySql('SELECT payload FROM password_change_intents WHERE profile=?', [this.profile]); const rows: PasswordIntent[] = [];
    try { while (result.goToNextRow()) { const row = JSON.parse(result.getString(0)) as PasswordIntent; PasswordRules.validate(row); if (row.profile !== this.profile) { PasswordRules.fail(); } rows.push(row); } } finally { result.close(); }
    this.check(); return rows.sort((a: PasswordIntent, b: PasswordIntent) => b.created.localeCompare(a.created));
  }
  async list(): Promise<PasswordIntent[]> { return this.store.syncAccess(async (db: relationalStore.RdbStore) => this.read(db)); }
  async insert(row: PasswordIntent): Promise<void> {
    PasswordRules.validate(row);
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => { db.beginTransaction(); try {
      const rows = await this.read(db); if (rows.some((old: PasswordIntent) => old.family === row.family && ['unsent', 'unknown', 'pending'].includes(old.state))) { PasswordRules.fail('PASSWORD_CHANGE_ORIGINAL_PENDING'); }
      await db.executeSql('INSERT INTO password_change_intents VALUES(?,?,?)', [this.profile, row.id, JSON.stringify(row)]); this.check(); db.commit();
    } catch (error) { db.rollBack(); throw error as Error; } });
  }
  async replace(before: PasswordIntent, after: PasswordIntent): Promise<void> {
    PasswordRules.validate(after);
    const a = JSON.parse(JSON.stringify(before)) as PasswordIntent; const b = JSON.parse(JSON.stringify(after)) as PasswordIntent;
    a.attempts = b.attempts = 0; a.state = b.state = ''; a.result = b.result = '';
    if (JSON.stringify(a) !== JSON.stringify(b)) { PasswordRules.fail(); }
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => { db.beginTransaction(); try {
      const row = (await this.read(db)).find((item: PasswordIntent) => item.id === before.id); if (!row || JSON.stringify(row) !== JSON.stringify(before)) { PasswordRules.fail('PASSWORD_CHANGE_ORIGINAL_CHANGED'); }
      await db.executeSql('UPDATE password_change_intents SET payload=? WHERE profile=? AND id=?', [JSON.stringify(after), this.profile, before.id]);
      const scope = Isolation.namespace(after.profile, after.deployment, '__auth__', false, after.epoch, 'metadata', 'personal');
      const key = 'password-change:' + after.family;
      await db.executeSql('INSERT OR REPLACE INTO local_state(namespace,key,value) VALUES(?,?,?)', [scope, key, PasswordRules.blocked(after) ? after.id : '']);
      this.check(); db.commit(); this.changed(after);
    } catch (error) { db.rollBack(); throw error as Error; } });
  }
}

import relationalStore from '@ohos.data.relationalStore';
import common from '@ohos.app.ability.common';
import { Message, Rules, AuthStateEntry } from './Models';
export interface StoreSyncHooks {
  put: (db: relationalStore.RdbStore, namespace: string, row: Message) => Promise<Message | undefined>;
  remove: (db: relationalStore.RdbStore, namespace: string, id: string) => Promise<void>;
  freeze: (db: relationalStore.RdbStore, namespace: string, reason: string) => Promise<void>;
  cloud: (db: relationalStore.RdbStore, namespace: string) => Promise<boolean>;
}
export class LocalStore {
  private writeTail: Promise<void> = Promise.resolve();
  private live?: relationalStore.RdbStore;
  private demo?: relationalStore.RdbStore;
  private syncHooks?: StoreSyncHooks;
  setSyncHooks(hooks: StoreSyncHooks): void { this.syncHooks = hooks; }
  async syncAccess<T>(action: (db: relationalStore.RdbStore) => Promise<T>): Promise<T> {
    const work = this.writeTail.then(() => action(this.database(false)));
    this.writeTail = work.then(() => {}).catch((_error: Error) => {});
    return work;
  }
  async open(context: common.UIAbilityContext): Promise<void> {
    this.live = await relationalStore.getRdbStore(context, { name: 'JobsIM-live.db', securityLevel: relationalStore.SecurityLevel.S2 });
    this.demo = await relationalStore.getRdbStore(context, { name: 'JobsIM-demo.db', securityLevel: relationalStore.SecurityLevel.S2 });
    await this.prepare(this.live);
    await this.prepare(this.demo);
  }
  private async prepare(db: relationalStore.RdbStore): Promise<void> {
    await db.executeSql('CREATE TABLE IF NOT EXISTS messages (namespace TEXT NOT NULL, conversation TEXT NOT NULL, id TEXT NOT NULL, payload TEXT NOT NULL, state TEXT NOT NULL, created TEXT NOT NULL, PRIMARY KEY(namespace,id))');
    await db.executeSql('CREATE INDEX IF NOT EXISTS message_order ON messages(namespace,conversation,created)');
    await db.executeSql('CREATE TABLE IF NOT EXISTS local_state (namespace TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY(namespace,key))');
    await db.executeSql('CREATE TABLE IF NOT EXISTS mention_receipts (namespace TEXT NOT NULL, conversation TEXT NOT NULL, id TEXT NOT NULL, unread INTEGER NOT NULL, PRIMARY KEY(namespace,id))');
    await db.executeSql('INSERT OR IGNORE INTO mention_receipts(namespace,conversation,id,unread) SELECT namespace,conversation,id,0 FROM messages');
    await db.executeSql('PRAGMA synchronous=FULL');
  }
  private database(demo: boolean): relationalStore.RdbStore {
    const db = demo ? this.demo : this.live;
    if (!db) { throw new Error('Local database unavailable'); }
    return db;
  }
  private async serialized(action: () => Promise<void>): Promise<void> {
    const work = this.writeTail.then(action);
    this.writeTail = work.catch((_error: Error) => {});
    await work;
  }
  async put(namespace: string, message: Message, demo: boolean): Promise<void> {
    await this.serialized(async () => {
      const db = this.database(demo); db.beginTransaction();
      try { await this.putNow(namespace, message, demo); db.commit(); }
      catch (error) { db.rollBack(); throw error as Error; }
    });
  }
  private async putNow(namespace: string, message: Message, demo: boolean): Promise<void> {
    if (!demo && this.syncHooks) {
      const checked = await this.syncHooks.put(this.database(false), namespace, message);
      if (!checked) { return; } message = checked;
    }
    await this.database(demo).insert('messages', { namespace: namespace, conversation: message.conversationId, id: message.id, payload: JSON.stringify(message), state: message.state ?? 'received', created: message.createdAt }, relationalStore.ConflictResolution.ON_CONFLICT_REPLACE);
    if (message.state !== 'pending' && message.state !== 'frozen') { await this.database(demo).insert('mention_receipts', { namespace: namespace, conversation: message.conversationId, id: message.id, unread: 0 }, relationalStore.ConflictResolution.ON_CONFLICT_IGNORE); }
  }
  async list(namespace: string, conversation: string, demo: boolean): Promise<Message[]> {
    const work = this.writeTail.then(() => this.listNow(namespace, conversation, demo));
    this.writeTail = work.then(() => {}).catch((_error: Error) => {}); return work;
  }
  private async listNow(namespace: string, conversation: string, demo: boolean): Promise<Message[]> {
    const db = this.database(demo);
    const cloud = demo || !this.syncHooks || await this.syncHooks.cloud(db, namespace);
    const predicate = new relationalStore.RdbPredicates('messages').equalTo('namespace', namespace).equalTo('conversation', conversation).orderByAsc('created');
    const result = await db.query(predicate);
    const rows: Message[] = [];
    try { while (result.goToNextRow()) { const row = JSON.parse(result.getString(result.getColumnIndex('payload'))) as Message; if (cloud || row.state === 'pending' || row.state === 'frozen') { rows.push(row); } } }
    finally { result.close(); }
    return rows;
  }
  async freeze(namespace: string, reason: string = 'restore_epoch_changed'): Promise<void> {
    await this.serialized(async () => {
      const db = this.database(false);
      db.beginTransaction();
      try { await this.freezeNow(namespace, reason); db.commit(); }
      catch (error) { db.rollBack(); throw error as Error; }
    });
  }
  private async freezeNow(namespace: string, reason: string): Promise<void> {
    const db = this.database(false);
    const result = await db.query(new relationalStore.RdbPredicates('messages').equalTo('namespace', namespace).equalTo('state', 'pending'));
    const rows: Message[] = [];
    try { while (result.goToNextRow()) { rows.push(JSON.parse(result.getString(result.getColumnIndex('payload'))) as Message); } }
    finally { result.close(); }
    for (let index = 0; index < rows.length; index++) { rows[index].state = 'frozen'; rows[index].freezeReason = reason; await this.putNow(namespace, rows[index], false); }
    if (this.syncHooks) { await this.syncHooks.freeze(db, namespace, reason); }
  }
  async maskForwards(namespace: string, conversation: string): Promise<void> {
    const rows = await this.list(namespace, conversation, false);
    for (const row of rows) {
      if (row.forwarded && row.state !== 'pending' && row.state !== 'frozen') { await this.put(namespace, Rules.unavailableForward(row), false); }
    }
  }
  async remove(namespace: string, id: string, demo: boolean): Promise<void> {
    await this.serialized(async () => {
      const db = this.database(demo); db.beginTransaction();
      try {
        if (!demo && this.syncHooks) { await this.syncHooks.remove(db, namespace, id); }
        await db.delete(new relationalStore.RdbPredicates('messages').equalTo('namespace', namespace).equalTo('id', id)); db.commit();
      } catch (error) { db.rollBack(); throw error as Error; }
    });
  }
  async value(namespace: string, key: string, demo: boolean = false): Promise<string> {
    const result = await this.database(demo).query(new relationalStore.RdbPredicates('local_state').equalTo('namespace', namespace).equalTo('key', key));
    try { return result.goToFirstRow() ? result.getString(result.getColumnIndex('value')) : ''; } finally { result.close(); }
  }
  async setValue(namespace: string, key: string, value: string, demo: boolean = false): Promise<void> {
    await this.serialized(async () => this.setValueNow(namespace, key, value, demo));
  }
  private async setValueNow(namespace: string, key: string, value: string, demo: boolean = false): Promise<void> {
    await this.database(demo).insert('local_state', { namespace: namespace, key: key, value: value }, relationalStore.ConflictResolution.ON_CONFLICT_REPLACE);
  }

  async mergeHistory(namespace: string, conversation: string, messages: Message[], cursor: string, account: string, active: boolean): Promise<void> {
    await this.serialized(async () => this.mergeHistoryNow(namespace, conversation, messages, cursor, account, active));
  }
  private async mergeHistoryNow(namespace: string, conversation: string, messages: Message[], cursor: string, account: string, active: boolean): Promise<void> {
    const db = this.database(false);
    db.beginTransaction();
    try {
      for (const message of messages) {
        const prior = await db.query(new relationalStore.RdbPredicates('mention_receipts').equalTo('namespace', namespace).equalTo('id', message.id));
        let previous: boolean | undefined = undefined;
        try { if (prior.goToFirstRow()) { previous = prior.getLong(prior.getColumnIndex('unread')) === 1; } } finally { prior.close(); }
        await this.putNow(namespace, message, false);
        await db.insert('mention_receipts', { namespace: namespace, conversation: conversation, id: message.id, unread: Rules.mentionUnread(previous, message, account, active) ? 1 : 0 }, relationalStore.ConflictResolution.ON_CONFLICT_REPLACE);
      }
      if (active) { await db.update({ unread: 0 }, new relationalStore.RdbPredicates('mention_receipts').equalTo('namespace', namespace).equalTo('conversation', conversation)); }
      await this.setValueNow(namespace, 'cursor:' + conversation, cursor);
      db.commit();
    } catch (error) { db.rollBack(); throw error as Error; }
  }
  async readMentions(namespace: string, conversation: string, demo: boolean): Promise<void> {
    await this.serialized(async () => { await this.database(demo).update({ unread: 0 }, new relationalStore.RdbPredicates('mention_receipts').equalTo('namespace', namespace).equalTo('conversation', conversation)); });
  }
  async mentionCount(namespace: string, conversation: string, demo: boolean): Promise<number> {
    const result = await this.database(demo).query(new relationalStore.RdbPredicates('mention_receipts').equalTo('namespace', namespace).equalTo('conversation', conversation).equalTo('unread', 1));
    try { return result.rowCount; } finally { result.close(); }
  }
  async unreadMentionIds(namespace: string, conversation: string, after: string): Promise<string[]> {
    const predicate = new relationalStore.RdbPredicates('mention_receipts').equalTo('namespace', namespace).equalTo('conversation', conversation).equalTo('unread', 1).orderByAsc('id').limitAs(20);
    if (after.length > 0) { predicate.greaterThan('id', after); }
    const result = await this.database(false).query(predicate);
    const ids: string[] = [];
    try { while (result.goToNextRow()) { ids.push(result.getString(result.getColumnIndex('id'))); } } finally { result.close(); }
    return ids;
  }
  async dismissMention(namespace: string, id: string): Promise<void> {
    await this.serialized(async () => { await this.database(false).update({ unread: 0 }, new relationalStore.RdbPredicates('mention_receipts').equalTo('namespace', namespace).equalTo('id', id)); });
  }
  async revokeConversation(namespace: string, conversation: string): Promise<void> {
    if (this.syncHooks) {
      await this.serialized(async () => {
        const db = this.database(false); db.beginTransaction();
        try {
          const hooks = this.syncHooks;
          if (!hooks) { throw new Error('SYNC_CONFIGURATION_REQUIRED'); }
          await hooks.freeze(db, namespace, 'SYNC_SCOPE_RESTRICTED');
          const result = await db.query(new relationalStore.RdbPredicates('messages').equalTo('namespace', namespace).equalTo('conversation', conversation).equalTo('state', 'pending'));
          const rows: Message[] = [];
          try { while (result.goToNextRow()) { rows.push(JSON.parse(result.getString(result.getColumnIndex('payload'))) as Message); } } finally { result.close(); }
          for (const row of rows) { row.state = 'frozen'; row.freezeReason = 'SYNC_SCOPE_RESTRICTED'; await this.putNow(namespace, row, false); }
          await this.setValueNow(namespace, 'cursor:' + conversation, ''); await this.setValueNow(namespace, 'unread:' + conversation, '0'); db.commit();
        } catch (error) { db.rollBack(); throw error as Error; }
      }); return;
    }
    const rows = await this.list(namespace, conversation, false);
    for (let index = 0; index < rows.length; index++) {
      const row = rows[index];
      if (row.state === 'pending' || row.state === 'frozen') { row.state = 'frozen'; await this.put(namespace, row, false); }
      else { await this.remove(namespace, row.id, false); }
    }
    await this.setValue(namespace, 'cursor:' + conversation, '');
    await this.setValue(namespace, 'unread:' + conversation, '0');
    await this.readMentions(namespace, conversation, false);
  }

  async saveAuthState(scope: string, values: AuthStateEntry[]): Promise<void> {
    await this.serialized(async () => {
      const db = this.database(false);
      db.beginTransaction();
      try {
        for (const value of values) { await this.setValueNow(scope, value.key, value.value); }
        db.commit();
      } catch (error) { db.rollBack(); throw error as Error; }
    });
  }
  async commitAuthBinding(scope: string, oldNamespaces: string[], newNamespace: string, values: AuthStateEntry[]): Promise<void> {
    await this.serialized(async () => {
      const db = this.database(false);
      db.beginTransaction();
      try {
        for (const previous of oldNamespaces) {
          if (previous !== newNamespace) { await this.freezeNow(previous, 'authentication_device_changed'); }
        }
        for (const value of values) { await this.setValueNow(scope, value.key, value.value); }
        db.commit();
      } catch (error) { db.rollBack(); throw error as Error; }
    });
  }

}

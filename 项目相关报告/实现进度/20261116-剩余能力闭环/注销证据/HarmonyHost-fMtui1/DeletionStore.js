"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.DeletionStore = void 0;
const Models_1 = require("./Models");
const Deletion_1 = require("./Deletion");
class DeletionStore {
    constructor(store, profile, current, changed = (_row) => { }) { this.store = store; this.profile = profile; this.current = current; this.changed = changed; }
    check() { if (!this.current()) {
        Deletion_1.DeletionRules.fail('DELETION_CONTEXT_CHANGED');
    } }
    async open() { await this.store.syncAccess(async (db) => { this.check(); await db.executeSql('CREATE TABLE IF NOT EXISTS account_deletion_intents(profile TEXT,id TEXT,payload TEXT,PRIMARY KEY(profile,id))'); }); }
    async read(db) {
        const query = await db.querySql('SELECT payload FROM account_deletion_intents WHERE profile=?', [this.profile]);
        const rows = [];
        try {
            while (query.goToNextRow()) {
                const row = JSON.parse(query.getString(0));
                Deletion_1.DeletionRules.validate(row);
                if (row.meta.profile !== this.profile) {
                    Deletion_1.DeletionRules.fail();
                }
                rows.push(row);
            }
        }
        finally {
            query.close();
        }
        this.check();
        return rows.sort((a, b) => b.meta.created.localeCompare(a.meta.created));
    }
    async list() { return this.store.syncAccess(async (db) => this.read(db)); }
    async insert(row) {
        Deletion_1.DeletionRules.validate(row);
        await this.store.syncAccess(async (db) => {
            db.beginTransaction();
            try {
                const rows = await this.read(db);
                if (rows.some((old) => old.meta.family === row.meta.family && !['rejected', 'retired', 'cancelled', 'completed'].includes(old.state))) {
                    Deletion_1.DeletionRules.fail('DELETION_ORIGINAL_PENDING');
                }
                await db.executeSql('INSERT INTO account_deletion_intents VALUES(?,?,?)', [this.profile, row.meta.id, JSON.stringify(row)]);
                this.check();
                db.commit();
            }
            catch (error) {
                db.rollBack();
                throw error;
            }
        });
    }
    async replace(before, after) {
        Deletion_1.DeletionRules.validate(after);
        if (after.attempts < before.attempts || JSON.stringify(before.meta) !== JSON.stringify(after.meta) || before.cancelId && (before.cancelId !== after.cancelId || before.cancelVersion !== after.cancelVersion)) {
            Deletion_1.DeletionRules.fail();
        }
        await this.store.syncAccess(async (db) => {
            db.beginTransaction();
            try {
                const row = (await this.read(db)).find((item) => item.meta.id === before.meta.id);
                if (!row || JSON.stringify(row) !== JSON.stringify(before)) {
                    Deletion_1.DeletionRules.fail('DELETION_ORIGINAL_CHANGED');
                }
                await db.executeSql('UPDATE account_deletion_intents SET payload=? WHERE profile=? AND id=?', [JSON.stringify(after), this.profile, before.meta.id]);
                const scope = Models_1.Isolation.namespace(after.meta.profile, after.meta.deployment, '__auth__', false, after.meta.epoch, 'metadata', 'personal');
                await db.executeSql('INSERT OR REPLACE INTO local_state(namespace,key,value) VALUES(?,?,?)', [scope, 'account-deletion:' + after.meta.family, Deletion_1.DeletionRules.blocked(after) ? after.meta.id : '']);
                this.check();
                db.commit();
                this.changed(after);
            }
            catch (error) {
                db.rollBack();
                throw error;
            }
        });
    }
    async values(db, sql, values) {
        const rows = await db.querySql(sql, values);
        const result = [];
        try {
            while (rows.goToNextRow()) {
                result.push(rows.getString(0));
            }
        }
        finally {
            rows.close();
        }
        return result;
    }
    tables() { return ['messages', 'local_state', 'mention_receipts', 'sync_stages', 'sync_refs', 'sync_objects', 'sync_events', 'sync_retained', 'sync_retained_conversations', 'sync_eligibility', 'sync_quarantine', 'sync_unread', 'sync_stream_archive', 'self_operations', 'sync_originals', 'sync_cleanup', 'group_action_intents']; }
    async eraseLocal(row, clearFiles) {
        Deletion_1.DeletionRules.validate(row);
        if (row.state !== 'completed' || row.meta.profile !== this.profile) {
            Deletion_1.DeletionRules.fail();
        }
        const prefix = 'live|' + row.meta.profile + '|' + row.meta.deployment + '|' + row.meta.user + '|';
        const plan = await this.store.syncAccess(async (db) => {
            const spaces = [row.meta.namespace];
            const files = [];
            for (const table of this.tables()) {
                if (!(await this.values(db, 'SELECT name FROM sqlite_master WHERE type=? AND name=?', ['table', table])).length) {
                    continue;
                }
                spaces.push(...await this.values(db, 'SELECT DISTINCT namespace FROM ' + table + ' WHERE substr(namespace,1,length(?))=?', [prefix, prefix]));
            }
            const rows = await db.querySql('SELECT key,value FROM local_state WHERE substr(namespace,1,length(?))=?', [prefix, prefix]);
            try {
                while (rows.goToNextRow()) {
                    const key = rows.getString(0);
                    const value = rows.getString(1);
                    if (key.startsWith('media-sidecar:') || key.startsWith('forward-sidecar:') || key.startsWith('own-avatar-sidecar')) {
                        if (value) {
                            files.push(value);
                        }
                    }
                    if (key === 'avatar' && value) {
                        const pending = JSON.parse(value);
                        if (pending.path) {
                            files.push(pending.path);
                        }
                    }
                }
            }
            finally {
                rows.close();
            }
            if ((await this.values(db, 'SELECT name FROM sqlite_master WHERE type=? AND name=?', ['table', 'sync_cleanup'])).length) {
                files.push(...await this.values(db, 'SELECT path FROM sync_cleanup WHERE substr(namespace,1,length(?))=?', [prefix, prefix]));
            }
            this.check();
            const output = { namespaces: Array.from(new Set(spaces)), files: Array.from(new Set(files)) };
            return output;
        });
        if (plan.namespaces.some((space) => !space.startsWith(prefix))) {
            Deletion_1.DeletionRules.fail();
        }
        await clearFiles(plan);
        this.check();
        await this.store.syncAccess(async (db) => {
            db.beginTransaction();
            try {
                for (const table of this.tables()) {
                    if (!(await this.values(db, 'SELECT name FROM sqlite_master WHERE type=? AND name=?', ['table', table])).length) {
                        continue;
                    }
                    await db.executeSql('DELETE FROM ' + table + ' WHERE substr(namespace,1,length(?))=?', [prefix, prefix]);
                }
                this.check();
                db.commit();
            }
            catch (error) {
                db.rollBack();
                throw error;
            }
        });
    }
}
exports.DeletionStore = DeletionStore;

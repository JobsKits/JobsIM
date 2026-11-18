"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.GroupStore = void 0;
const GroupTools_1 = require("./GroupTools");
class GroupStore {
    constructor(store, environment, current) { this.store = store; this.environment = environment; this.current = current; }
    check() { if (!this.current()) {
        GroupTools_1.GroupRules.fail('GROUP_CONTEXT_CHANGED');
    } }
    prefix() { const e = this.environment; return 'live|' + e.profile + '|' + e.deployment + '|' + e.user + '|'; }
    async open() { await this.store.syncAccess(async (db) => { await db.executeSql('CREATE TABLE IF NOT EXISTS group_action_intents(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))'); }); }
    async read(db) {
        const prefix = this.prefix();
        const rows = await db.querySql('SELECT namespace,payload FROM group_action_intents WHERE substr(namespace,1,length(?))=?', [prefix, prefix]);
        const result = [];
        try {
            while (rows.goToNextRow()) {
                const row = JSON.parse(rows.getString(1));
                GroupTools_1.GroupRules.validate(row);
                if (row.namespace !== rows.getString(0)) {
                    GroupTools_1.GroupRules.fail();
                }
                result.push(row);
            }
        }
        finally {
            rows.close();
        }
        this.check();
        return result.sort((a, b) => b.created.localeCompare(a.created));
    }
    async list() { return this.store.syncAccess(async (db) => this.read(db)); }
    async insert(row) {
        GroupTools_1.GroupRules.validate(row);
        if (!row.namespace.startsWith(this.prefix())) {
            GroupTools_1.GroupRules.fail();
        }
        await this.store.syncAccess(async (db) => { this.check(); await db.executeSql('INSERT INTO group_action_intents VALUES(?,?,?)', [row.namespace, row.id, JSON.stringify(row)]); });
    }
    async replace(before, after) {
        GroupTools_1.GroupRules.validate(after);
        const a = JSON.parse(JSON.stringify(before));
        const b = JSON.parse(JSON.stringify(after));
        a.attempts = b.attempts = 0;
        a.state = b.state = '';
        a.result = b.result = '';
        if (JSON.stringify(a) !== JSON.stringify(b)) {
            GroupTools_1.GroupRules.fail();
        }
        await this.store.syncAccess(async (db) => {
            db.beginTransaction();
            try {
                const found = (await this.read(db)).find((row) => row.id === before.id && row.namespace === before.namespace);
                if (!found || JSON.stringify(found) !== JSON.stringify(before)) {
                    GroupTools_1.GroupRules.fail('GROUP_ORIGINAL_CHANGED');
                }
                await db.executeSql('UPDATE group_action_intents SET payload=? WHERE namespace=? AND id=?', [JSON.stringify(after), before.namespace, before.id]);
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
exports.GroupStore = GroupStore;

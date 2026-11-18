"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.PasswordStore = void 0;
const Models_1 = require("./Models");
const PasswordChange_1 = require("./PasswordChange");
class PasswordStore {
    constructor(store, profile, current, changed = (_row) => { }) { this.store = store; this.profile = profile; this.current = current; this.changed = changed; }
    check() { if (!this.current()) {
        PasswordChange_1.PasswordRules.fail('PASSWORD_CHANGE_CONTEXT_CHANGED');
    } }
    async open() { await this.store.syncAccess(async (db) => { this.check(); await db.executeSql('CREATE TABLE IF NOT EXISTS password_change_intents(profile TEXT,id TEXT,payload TEXT,PRIMARY KEY(profile,id))'); }); }
    async read(db) {
        const result = await db.querySql('SELECT payload FROM password_change_intents WHERE profile=?', [this.profile]);
        const rows = [];
        try {
            while (result.goToNextRow()) {
                const row = JSON.parse(result.getString(0));
                PasswordChange_1.PasswordRules.validate(row);
                if (row.profile !== this.profile) {
                    PasswordChange_1.PasswordRules.fail();
                }
                rows.push(row);
            }
        }
        finally {
            result.close();
        }
        this.check();
        return rows.sort((a, b) => b.created.localeCompare(a.created));
    }
    async list() { return this.store.syncAccess(async (db) => this.read(db)); }
    async insert(row) {
        PasswordChange_1.PasswordRules.validate(row);
        await this.store.syncAccess(async (db) => {
            db.beginTransaction();
            try {
                const rows = await this.read(db);
                if (rows.some((old) => old.family === row.family && ['unsent', 'unknown', 'pending'].includes(old.state))) {
                    PasswordChange_1.PasswordRules.fail('PASSWORD_CHANGE_ORIGINAL_PENDING');
                }
                await db.executeSql('INSERT INTO password_change_intents VALUES(?,?,?)', [this.profile, row.id, JSON.stringify(row)]);
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
        PasswordChange_1.PasswordRules.validate(after);
        const a = JSON.parse(JSON.stringify(before));
        const b = JSON.parse(JSON.stringify(after));
        a.attempts = b.attempts = 0;
        a.state = b.state = '';
        a.result = b.result = '';
        if (JSON.stringify(a) !== JSON.stringify(b)) {
            PasswordChange_1.PasswordRules.fail();
        }
        await this.store.syncAccess(async (db) => {
            db.beginTransaction();
            try {
                const row = (await this.read(db)).find((item) => item.id === before.id);
                if (!row || JSON.stringify(row) !== JSON.stringify(before)) {
                    PasswordChange_1.PasswordRules.fail('PASSWORD_CHANGE_ORIGINAL_CHANGED');
                }
                await db.executeSql('UPDATE password_change_intents SET payload=? WHERE profile=? AND id=?', [JSON.stringify(after), this.profile, before.id]);
                const scope = Models_1.Isolation.namespace(after.profile, after.deployment, '__auth__', false, after.epoch, 'metadata', 'personal');
                const key = 'password-change:' + after.family;
                await db.executeSql('INSERT OR REPLACE INTO local_state(namespace,key,value) VALUES(?,?,?)', [scope, key, PasswordChange_1.PasswordRules.blocked(after) ? after.id : '']);
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
}
exports.PasswordStore = PasswordStore;

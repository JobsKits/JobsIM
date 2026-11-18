"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.MigrationStore = void 0;
const SyncProtocol_1 = require("./SyncProtocol");
const MigrationProtocol_1 = require("./MigrationProtocol");
class MigrationStore {
    static async eraseGlobal(db, profile, deployment, user, id) {
        const exists = await db.querySql("SELECT name FROM sqlite_master WHERE type='table' AND name IN('migration_history','migration_objects','migration_cleanup')");
        let count = 0;
        try {
            while (exists.goToNextRow()) {
                count++;
            }
        }
        finally {
            exists.close();
        }
        if (count !== 3) {
            return;
        }
        const prefix = 'live|' + profile + '|' + deployment + '|' + user + '|';
        const rows = await db.querySql('SELECT namespace,id,payload FROM migration_history WHERE substr(namespace,1,length(?))=?', [prefix, prefix]);
        const erase = [];
        try {
            while (rows.goToNextRow()) {
                const raw = SyncProtocol_1.SyncJSON.parse(rows.getString(2));
                if (rows.getString(1) === id || raw.get('forwarded').value === true) {
                    erase.push([rows.getString(0), rows.getString(1)]);
                }
            }
        }
        finally {
            rows.close();
        }
        for (const item of erase) {
            const objects = await db.querySql('SELECT payload FROM migration_objects WHERE namespace=? AND message=?', item);
            const names = [];
            try {
                while (objects.goToNextRow()) {
                    names.push(SyncProtocol_1.SyncJSON.parse(objects.getString(0)).get('path').string());
                }
            }
            finally {
                objects.close();
            }
            for (const name of names) {
                await db.executeSql('INSERT OR IGNORE INTO migration_cleanup VALUES(?,?)', [item[0], name]);
            }
            await db.executeSql('DELETE FROM migration_objects WHERE namespace=? AND message=?', item);
            await db.executeSql('DELETE FROM migration_history WHERE namespace=? AND id=?', item);
        }
    }
    static async cleanups(local) { return local.syncAccess(async (db) => { const table = await db.querySql("SELECT name FROM sqlite_master WHERE type='table' AND name='migration_cleanup'"); let exists = false; try {
        exists = table.goToNextRow();
    }
    finally {
        table.close();
    } if (!exists) {
        return [];
    } const rows = await db.querySql('SELECT namespace,name FROM migration_cleanup'); const result = []; try {
        while (rows.goToNextRow()) {
            result.push([rows.getString(0), rows.getString(1)]);
        }
    }
    finally {
        rows.close();
    } return result; }); }
    static async cleaned(local, namespace, name) { await local.syncAccess(async (db) => { await db.executeSql('DELETE FROM migration_cleanup WHERE namespace=? AND name=?', [namespace, name]); }); }
    constructor(local, identity, current, files) { this.local = local; this.identity = identity; this.current = current; this.files = files; }
    bindLifecycle(active) { const original = this.current; this.current = () => original() && active(); }
    check() { if (!this.current()) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_CONTEXT_CHANGED');
    } }
    prefix() { const i = this.identity; return 'live|' + i.profile + '|' + i.deployment + '|' + i.user + '|'; }
    scope() { const i = this.identity; return JSON.stringify([i.profile, i.deployment, i.user, 'personal']); }
    static samePrincipal(a, b) { return a.profile === b.profile && a.deployment === b.deployment && a.user === b.user && a.device === b.device && a.family === b.family && a.epoch === b.epoch && a.namespace === b.namespace && a.store === b.store; }
    static validate(row) {
        const raw = SyncProtocol_1.SyncJSON.parse(JSON.stringify(row));
        MigrationProtocol_1.MigrationRules.exact(raw, ['id', 'kind', 'identity', 'created', 'selection', 'attempts', 'state', 'sourceDigest', 'target', 'migration', 'view']);
        MigrationProtocol_1.MigrationRules.id(row.id);
        MigrationProtocol_1.MigrationRules.identity(row.identity);
        MigrationProtocol_1.MigrationRules.exact(raw.get('identity'), ['profile', 'deployment', 'user', 'device', 'family', 'epoch', 'namespace', 'store', 'guard']);
        if (!['pairing', 'create', 'target'].includes(row.kind) || !['unsent', 'unknown', 'ready', 'installed', 'retired'].includes(row.state) || !Number.isSafeInteger(row.attempts) || row.attempts < 0 || !Number.isFinite(Date.parse(row.created))) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const selected = SyncProtocol_1.SyncJSON.parse(row.selection).list();
        if (selected.length > 256 || (row.kind === 'create') !== (selected.length > 0)) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const ids = [];
        let full = 0;
        for (const item of selected) {
            MigrationProtocol_1.MigrationRules.exact(item, ['id', 'conversationId', 'contentVersion', 'type', 'attachmentId', 'forwarded', 'objectMode', 'sourceReceiptId', 'sourceObjectReceiptId']);
            const id = MigrationProtocol_1.MigrationRules.id(item.get('id').string());
            if (ids.includes(id)) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            ids.push(id);
            MigrationProtocol_1.MigrationRules.id(item.get('conversationId').string());
            MigrationProtocol_1.MigrationRules.positive(item.get('contentVersion').string());
            const type = item.get('type').string();
            const mode = item.get('objectMode').string();
            if (!['text', 'image', 'audio', 'video', 'file'].includes(type) || !['none', 'metadata_only', 'full'].includes(mode) || (type === 'text') !== (mode === 'none')) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            if (item.has('attachmentId')) {
                MigrationProtocol_1.MigrationRules.id(item.get('attachmentId').string());
            }
            if (item.has('sourceReceiptId')) {
                MigrationProtocol_1.MigrationRules.id(item.get('sourceReceiptId').string());
            }
            if (item.has('sourceObjectReceiptId')) {
                MigrationProtocol_1.MigrationRules.id(item.get('sourceObjectReceiptId').string());
                if (!item.has('sourceReceiptId') || mode !== 'full') {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
            if (item.has('forwarded') && (item.get('forwarded').value !== true || item.has('attachmentId') || mode !== (type === 'text' ? 'none' : 'metadata_only'))) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            if (mode === 'full') {
                full++;
                if (!item.has('attachmentId')) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
        }
        if (full > 64) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        if (row.sourceDigest) {
            MigrationProtocol_1.MigrationRules.digest(row.sourceDigest);
        }
        if (row.migration) {
            MigrationProtocol_1.MigrationRules.id(row.migration);
        }
        if (row.kind === 'target' && !row.migration) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        if (row.target) {
            const t = SyncProtocol_1.SyncJSON.parse(row.target);
            MigrationProtocol_1.MigrationRules.exact(t, ['pairingId', 'targetDeviceId', 'targetStoreGeneration', 'restoreEpoch', 'securityVersion', 'sourceGeneration', 'expires']);
            for (const k of ['pairingId', 'targetDeviceId', 'targetStoreGeneration']) {
                MigrationProtocol_1.MigrationRules.id(t.get(k).string());
            }
            for (const k of ['restoreEpoch', 'securityVersion', 'sourceGeneration']) {
                MigrationProtocol_1.MigrationRules.positive(t.get(k).string());
            }
            t.get('expires').integer(Number.MAX_SAFE_INTEGER);
        }
        if (row.view) {
            const view = SyncProtocol_1.SyncJSON.parse(row.view);
            MigrationProtocol_1.MigrationRules.exact(view, ['id', 'requestId', 'status', 'manifest', 'manifestEnvelope', 'manifestDigest', 'capsuleSHA256', 'sizeBytes', 'expiresAt', 'sourcePreserved', 'qualifiedAck', 'targetQualified']);
            MigrationProtocol_1.MigrationRules.digest(view.get('manifestDigest').string());
        }
    }
    async open() { await this.local.syncAccess(async (db) => { await db.executeSql('CREATE TABLE IF NOT EXISTS migration_intents(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))'); await db.executeSql('CREATE TABLE IF NOT EXISTS migration_history(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))'); await db.executeSql('CREATE TABLE IF NOT EXISTS migration_objects(namespace TEXT,id TEXT,message TEXT,payload TEXT,PRIMARY KEY(namespace,id))'); await db.executeSql('CREATE TABLE IF NOT EXISTS migration_installs(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))'); await db.executeSql('CREATE TABLE IF NOT EXISTS migration_cleanup(namespace TEXT,name TEXT,PRIMARY KEY(namespace,name))'); }); }
    async values(db, sql, args) { const rows = await db.querySql(sql, args); const out = []; try {
        while (rows.goToNextRow()) {
            out.push(rows.getString(0));
        }
    }
    finally {
        rows.close();
    } return out; }
    async bindingNow(db) {
        this.check();
        const i = this.identity;
        const raw = (await this.values(db, "SELECT value FROM local_state WHERE namespace=? AND key='sync:control'", [i.namespace]))[0];
        const physical = (await this.values(db, "SELECT value FROM local_state WHERE namespace='__sync_store__' AND key='physical'", []))[0];
        if (!raw || physical !== i.store) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_STORE_NOT_READY');
        }
        const c = SyncProtocol_1.SyncJSON.parse(raw);
        const b = c.get('identity');
        if (c.get('physical').string() !== i.store || c.get('guard').string() !== i.guard || !c.get('cursor').string() || c.get('restriction').string() || b.get('namespace').string() !== i.namespace || b.get('profile').string() !== i.profile || b.get('deployment').string() !== i.deployment || b.get('user').string() !== i.user || b.get('source').string() !== 'personal' || b.get('device').string() !== i.device || b.get('epoch').string() !== i.epoch) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_STORE_NOT_READY');
        }
        this.check();
    }
    async binding() { await this.local.syncAccess(async (db) => this.bindingNow(db)); }
    async rowsNow(db) { const p = this.prefix(); const rows = await db.querySql('SELECT namespace,payload FROM migration_intents WHERE substr(namespace,1,length(?))=?', [p, p]); const out = []; try {
        while (rows.goToNextRow()) {
            const row = JSON.parse(rows.getString(1));
            MigrationStore.validate(row);
            if (row.identity.namespace !== rows.getString(0)) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            out.push(row);
        }
    }
    finally {
        rows.close();
    } this.check(); return out.sort((a, b) => b.created.localeCompare(a.created)); }
    async rows() { return this.local.syncAccess(async (db) => this.rowsNow(db)); }
    async stage(kind, selection, id, migration = '') {
        const row = { id: id, kind: kind, identity: this.identity, created: new Date().toISOString(), selection: selection, attempts: 0, state: 'unsent', sourceDigest: '', target: '', migration: migration, view: '' };
        MigrationStore.validate(row);
        await this.local.syncAccess(async (db) => { db.beginTransaction(); try {
            await this.bindingNow(db);
            await db.executeSql('INSERT INTO migration_intents VALUES(?,?,?)', [row.identity.namespace, row.id, JSON.stringify(row)]);
            this.check();
            db.commit();
        }
        catch (error) {
            db.rollBack();
            throw error;
        } });
        return row;
    }
    async replace(before, after) {
        MigrationStore.validate(after);
        if (before.id !== after.id || before.kind !== after.kind || JSON.stringify(before.identity) !== JSON.stringify(after.identity) || before.created !== after.created || before.selection !== after.selection || after.attempts < before.attempts || before.sourceDigest && before.sourceDigest !== after.sourceDigest || before.target && before.target !== after.target || before.migration && before.migration !== after.migration) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_ORIGINAL_CHANGED');
        }
        await this.local.syncAccess(async (db) => { db.beginTransaction(); try {
            const found = (await this.rowsNow(db)).find((item) => item.id === before.id);
            if (!found || JSON.stringify(found) !== JSON.stringify(before)) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_ORIGINAL_CHANGED');
            }
            await db.executeSql('UPDATE migration_intents SET payload=? WHERE namespace=? AND id=?', [JSON.stringify(after), before.identity.namespace, before.id]);
            this.check();
            db.commit();
        }
        catch (error) {
            db.rollBack();
            throw error;
        } });
        return after;
    }
    async installed(view) { return this.local.syncAccess(async (db) => { await this.bindingNow(db); const raw = (await this.values(db, 'SELECT payload FROM migration_installs WHERE namespace=? AND id=?', [this.identity.namespace, view.id]))[0]; if (!raw) {
        return false;
    } const value = SyncProtocol_1.SyncJSON.parse(raw); if (value.get('manifestDigest').string() !== view.digest || value.get('localCommitted').value !== true || value.get('qualifiedAck').value !== false) {
        MigrationProtocol_1.MigrationRules.fail();
    } return true; }); }
    async install(view, history, before) {
        this.files.disk();
        let paths = new Map();
        this.check();
        const m = SyncProtocol_1.SyncJSON.parse(view.manifest);
        const objects = m.get('objects').value === null ? [] : m.get('objects').list();
        const i = this.identity;
        await this.local.syncAccess(async (db) => {
            db.beginTransaction();
            try {
                await this.bindingNow(db);
                if (Date.parse(before) <= Date.now()) {
                    MigrationProtocol_1.MigrationRules.fail('MIGRATION_INSTALL_EXPIRED');
                }
                for (const raw of history.messages) {
                    const row = SyncProtocol_1.SyncJSON.parse(raw);
                    const id = row.get('id').string();
                    if ((await this.values(db, 'SELECT id FROM sync_terminal WHERE scope=? AND id=?', [this.scope(), id])).length) {
                        MigrationProtocol_1.MigrationRules.fail('MIGRATION_GLOBAL_SOURCE');
                    }
                    const live = (await this.values(db, "SELECT payload FROM messages WHERE namespace=? AND id=? AND state NOT IN('pending','frozen')", [i.namespace, id]))[0];
                    if (live) {
                        const old = SyncProtocol_1.SyncJSON.parse(live);
                        const ref = m.get('messages').list().find((item) => item.get('messageId').value === id);
                        if (old.get('deleted').value === true || old.get('contentUnavailable').value === true && (!ref || ref.get('classification').value !== 'cloud_only' || row.get('forwarded').value === true)) {
                            MigrationProtocol_1.MigrationRules.fail('MIGRATION_LOCAL_FLOOR');
                        }
                    }
                }
                paths = this.files.stage(view, history);
                this.check();
                for (const raw of history.messages) {
                    const row = SyncProtocol_1.SyncJSON.parse(raw);
                    const id = row.get('id').string();
                    if ((await this.values(db, 'SELECT id FROM sync_terminal WHERE scope=? AND id=?', [this.scope(), id])).length) {
                        MigrationProtocol_1.MigrationRules.fail('MIGRATION_GLOBAL_SOURCE');
                    }
                    const old = (await this.values(db, 'SELECT payload FROM migration_history WHERE namespace=? AND id=?', [i.namespace, id]))[0];
                    if (old) {
                        const seen = SyncProtocol_1.SyncJSON.parse(old);
                        if (seen.get('creationEpoch').string() !== row.get('creationEpoch').string() || seen.get('creationSequence').string() !== row.get('creationSequence').string()) {
                            MigrationProtocol_1.MigrationRules.fail();
                        }
                        if (SyncProtocol_1.SyncRules.compare(seen.get('contentVersion').string(), row.get('contentVersion').string()) > 0) {
                            continue;
                        }
                    }
                    const live = (await this.values(db, "SELECT payload FROM messages WHERE namespace=? AND id=? AND state NOT IN('pending','frozen')", [i.namespace, id]))[0];
                    if (live) {
                        const seen = SyncProtocol_1.SyncJSON.parse(live);
                        const reference = m.get('messages').list().find((item) => item.get('messageId').value === id);
                        if (seen.get('deleted').value === true || seen.get('contentUnavailable').value === true && (!reference || reference.get('classification').value !== 'cloud_only' || row.get('forwarded').value === true)) {
                            MigrationProtocol_1.MigrationRules.fail('MIGRATION_LOCAL_FLOOR');
                        }
                        if (seen.has('contentVersion') && seen.get('contentVersion').string() && SyncProtocol_1.SyncRules.compare(seen.get('contentVersion').string(), row.get('contentVersion').string()) > 0) {
                            continue;
                        }
                    }
                    const prior = await db.querySql('SELECT id,payload FROM migration_objects WHERE namespace=? AND message=?', [i.namespace, id]);
                    const older = [];
                    try {
                        while (prior.goToNextRow()) {
                            older.push([prior.getString(0), prior.getString(1)]);
                        }
                    }
                    finally {
                        prior.close();
                    }
                    for (const item of older) {
                        const value = SyncProtocol_1.SyncJSON.parse(item[1]);
                        if (paths.get(item[0]) !== value.get('path').string()) {
                            await db.executeSql('INSERT OR IGNORE INTO migration_cleanup VALUES(?,?)', [i.namespace, value.get('path').string()]);
                        }
                    }
                    await db.executeSql('DELETE FROM migration_objects WHERE namespace=? AND message=?', [i.namespace, id]);
                    await db.executeSql('INSERT OR REPLACE INTO migration_history VALUES(?,?,?)', [i.namespace, id, raw]);
                    for (const object of history.objects.filter((item) => item.message === id)) {
                        const ref = objects.find((item) => item.get('objectId').string() === object.id);
                        const name = paths.get(object.id);
                        if (!ref || !name) {
                            MigrationProtocol_1.MigrationRules.fail();
                        }
                        const payload = {};
                        payload['id'] = object.id;
                        payload['messageId'] = id;
                        payload['path'] = name;
                        payload['sha256'] = ref.get('sha256').string();
                        payload['contentType'] = object.mime;
                        payload['migrationId'] = view.id;
                        await db.executeSql('INSERT OR REPLACE INTO migration_objects VALUES(?,?,?,?)', [i.namespace, object.id, id, JSON.stringify(payload)]);
                    }
                }
                const receipt = {};
                receipt['id'] = view.id;
                receipt['manifestDigest'] = view.digest;
                receipt['localCommitted'] = true;
                receipt['qualifiedAck'] = false;
                await db.executeSql('INSERT OR REPLACE INTO migration_installs VALUES(?,?,?)', [i.namespace, view.id, JSON.stringify(receipt)]);
                this.check();
                if (Date.parse(before) <= Date.now()) {
                    MigrationProtocol_1.MigrationRules.fail('MIGRATION_INSTALL_EXPIRED');
                }
                db.commit();
            }
            catch (error) {
                db.rollBack();
                throw error;
            }
        });
    }
    async history() { return this.local.syncAccess(async (db) => { await this.bindingNow(db); const p = this.prefix(); const rows = await db.querySql('SELECT namespace,id,payload FROM migration_history WHERE substr(namespace,1,length(?))=?', [p, p]); const all = []; try {
        while (rows.goToNextRow()) {
            if (rows.getString(0).split('|')[4] === this.identity.epoch) {
                all.push([rows.getString(1), rows.getString(2)]);
            }
        }
    }
    finally {
        rows.close();
    } const out = []; for (const item of all) {
        if ((await this.values(db, 'SELECT id FROM sync_terminal WHERE scope=? AND id=?', [this.scope(), item[0]])).length) {
            continue;
        }
        const message = SyncProtocol_1.SyncJSON.parse(item[1]);
        if (message.get('deleted').value === true) {
            continue;
        }
        const current = (await this.values(db, "SELECT payload FROM messages WHERE namespace=? AND id=? AND state NOT IN('pending','frozen')", [this.identity.namespace, item[0]]))[0];
        if (current) {
            const live = SyncProtocol_1.SyncJSON.parse(current);
            if (live.get('deleted').value === true || live.has('contentVersion') && live.get('contentVersion').string() && SyncProtocol_1.SyncRules.compare(live.get('contentVersion').string(), message.get('contentVersion').string()) > 0) {
                continue;
            }
        }
        out.push(item[1]);
    } const unique = new Map(); for (const raw of out) {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        const id = value.get('id').string();
        const old = unique.get(id);
        if (old) {
            const before = SyncProtocol_1.SyncJSON.parse(old);
            if (before.get('creationEpoch').string() !== value.get('creationEpoch').string() || before.get('creationSequence').string() !== value.get('creationSequence').string()) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            if (SyncProtocol_1.SyncRules.compare(before.get('contentVersion').string(), value.get('contentVersion').string()) > 0) {
                continue;
            }
        }
        unique.set(id, raw);
    } this.check(); return Array.from(unique.values()).sort((a, b) => SyncProtocol_1.SyncJSON.parse(a).get('createdAt').string().localeCompare(SyncProtocol_1.SyncJSON.parse(b).get('createdAt').string())); }); }
    async objectInfo(id) { return this.local.syncAccess(async (db) => { await this.bindingNow(db); MigrationProtocol_1.MigrationRules.id(id); if ((await this.values(db, 'SELECT id FROM sync_terminal WHERE scope=? AND id=?', [this.scope(), id])).length) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_GLOBAL_SOURCE');
    } const p = this.prefix(); const rows = await db.querySql('SELECT namespace,payload FROM migration_objects WHERE substr(namespace,1,length(?))=? AND message=?', [p, p, id]); let result = ''; try {
        while (rows.goToNextRow()) {
            const ns = rows.getString(0);
            if (ns.split('|')[4] === this.identity.epoch) {
                const value = JSON.parse(rows.getString(1));
                value['namespace'] = ns;
                result = JSON.stringify(value);
            }
        }
    }
    finally {
        rows.close();
    } this.check(); if (!result) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_OBJECT_METADATA_ONLY');
    } return result; }); }
    async retireMessage(id) { await this.local.syncAccess(async (db) => { db.beginTransaction(); try {
        await this.bindingNow(db);
        MigrationProtocol_1.MigrationRules.id(id);
        const p = this.prefix();
        const rows = await db.querySql('SELECT namespace,id,payload FROM migration_objects WHERE substr(namespace,1,length(?))=? AND message=?', [p, p, id]);
        const files = [];
        try {
            while (rows.goToNextRow()) {
                files.push([rows.getString(0), SyncProtocol_1.SyncJSON.parse(rows.getString(2)).get('path').string()]);
            }
        }
        finally {
            rows.close();
        }
        for (const file of files) {
            await db.executeSql('INSERT OR IGNORE INTO migration_cleanup VALUES(?,?)', file);
        }
        await db.executeSql('DELETE FROM migration_objects WHERE substr(namespace,1,length(?))=? AND message=?', [p, p, id]);
        await db.executeSql('DELETE FROM migration_history WHERE substr(namespace,1,length(?))=? AND id=?', [p, p, id]);
        this.check();
        db.commit();
    }
    catch (error) {
        db.rollBack();
        throw error;
    } }); }
    async referenced() { return this.local.syncAccess(async (db) => { const p = this.prefix(); return (await this.values(db, 'SELECT payload FROM migration_objects WHERE substr(namespace,1,length(?))=?', [p, p])).map((raw) => SyncProtocol_1.SyncJSON.parse(raw).get('path').string()); }); }
    async discard(id) { MigrationProtocol_1.MigrationRules.id(id); await this.local.syncAccess(async (db) => { const references = await this.values(db, 'SELECT payload FROM migration_objects WHERE namespace=?', [this.identity.namespace]); this.files.discard(id, references.map((raw) => SyncProtocol_1.SyncJSON.parse(raw).get('path').string())); }); }
    async drain() { await this.local.syncAccess(async (db) => { const rows = await db.querySql('SELECT name FROM migration_cleanup WHERE namespace=?', [this.identity.namespace]); const names = []; try {
        while (rows.goToNextRow()) {
            names.push(rows.getString(0));
        }
    }
    finally {
        rows.close();
    } this.files.erase(names); for (const name of names) {
        await db.executeSql('DELETE FROM migration_cleanup WHERE namespace=? AND name=?', [this.identity.namespace, name]);
    } this.check(); }); }
}
exports.MigrationStore = MigrationStore;

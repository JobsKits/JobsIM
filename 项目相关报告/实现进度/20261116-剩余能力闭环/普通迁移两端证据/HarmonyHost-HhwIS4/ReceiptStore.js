"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ReceiptStore = void 0;
const _ohos_util_1 = require("@ohos.util");
const NativeStorageBudget_1 = require("./NativeStorageBudget");
const SyncProtocol_1 = require("./SyncProtocol");
const MigrationProtocol_1 = require("./MigrationProtocol");
const MigrationStore_1 = require("./MigrationStore");
const ReceiptProtocol_1 = require("./ReceiptProtocol");
class ReceiptStore {
    constructor(local, identity, current, files, codec) { this.local = local; this.identity = identity; this.current = current; this.files = files; this.codec = codec; }
    bindLifecycle(active) { const original = this.current; this.current = () => original() && active(); }
    check() { if (!this.current()) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_CONTEXT_CHANGED');
    } }
    prefix() { const i = this.identity; return 'live|' + i.profile + '|' + i.deployment + '|' + i.user + '|'; }
    async values(db, sql, args = []) { const rows = await db.querySql(sql, args); const result = []; try {
        while (rows.goToNextRow()) {
            result.push(rows.getString(0));
        }
    }
    finally {
        rows.close();
    } return result; }
    async open() { await this.local.syncAccess(async (db) => { db.beginTransaction(); try {
        for (const sql of ['CREATE TABLE IF NOT EXISTS receipt_intents(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))', 'CREATE TABLE IF NOT EXISTS receipt_bodies(namespace TEXT,id TEXT,message TEXT,payload TEXT,PRIMARY KEY(namespace,id))', 'CREATE TABLE IF NOT EXISTS receipt_objects(namespace TEXT,id TEXT,message TEXT,payload TEXT,PRIMARY KEY(namespace,id))', 'CREATE TABLE IF NOT EXISTS receipt_cleanup(namespace TEXT,name TEXT,PRIMARY KEY(namespace,name))', 'CREATE TABLE IF NOT EXISTS receipt_measurements(namespace TEXT,build TEXT,payload TEXT,PRIMARY KEY(namespace,build))', 'CREATE TABLE IF NOT EXISTS receipt_probes(namespace TEXT,id TEXT,kind TEXT,payload TEXT,PRIMARY KEY(namespace,id,kind))']) {
            await db.executeSql(sql);
        }
        this.check();
        db.commit();
    }
    catch (error) {
        db.rollBack();
        throw error;
    } }); }
    async full(db) { const rows = await db.querySql('PRAGMA synchronous'); try {
        if (!rows.goToNextRow() || rows.getLong(0) !== 2) {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_FULL_REQUIRED');
        }
    }
    finally {
        rows.close();
    } }
    async bound(db) {
        this.check();
        const i = this.identity;
        const raw = (await this.values(db, "SELECT value FROM local_state WHERE namespace=? AND key='sync:control'", [i.namespace]))[0];
        if (!raw) {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_STORE_NOT_READY');
        }
        const c = SyncProtocol_1.SyncJSON.parse(raw);
        const b = c.get('identity');
        if (c.get('physical').string() !== i.store || c.get('guard').string() !== i.guard || !c.get('cursor').string() || c.get('restriction').string() || b.get('namespace').string() !== i.namespace || b.get('profile').string() !== i.profile || b.get('deployment').string() !== i.deployment || b.get('user').string() !== i.user || b.get('device').string() !== i.device || b.get('epoch').string() !== i.epoch || b.get('source').string() !== 'personal' || (await this.values(db, "SELECT value FROM local_state WHERE namespace='__sync_store__' AND key='physical'"))[0] !== i.store) {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_STORE_NOT_READY');
        }
        return c;
    }
    async binding() { await this.local.syncAccess(async (db) => { await this.bound(db); }); }
    async control(key) { return this.local.syncAccess(async (db) => { const c = await this.bound(db); return key === 'schema' ? (c.has('schema') ? c.get('schema').string() : '1') : c.get('permission').string(); }); }
    scope() { const i = this.identity; return JSON.stringify([i.profile, i.deployment, i.user, 'personal']); }
    async floor(db, message, cloudOnly = false) { if ((await this.values(db, 'SELECT id FROM sync_terminal WHERE scope=? AND id=?', [this.scope(), message])).length) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_GLOBAL_SOURCE');
    } const live = (await this.values(db, "SELECT payload FROM messages WHERE namespace=? AND id=? AND state NOT IN('pending','frozen')", [this.identity.namespace, message]))[0]; if (live) {
        const value = SyncProtocol_1.SyncJSON.parse(live);
        if (value.get('deleted').value === true || value.get('contentUnavailable').value === true && !cloudOnly) {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_GLOBAL_SOURCE');
        }
    } }
    async rows() { return this.local.syncAccess(async (db) => { const prefix = this.prefix(); const rows = await db.querySql('SELECT namespace,payload FROM receipt_intents WHERE substr(namespace,1,length(?))=?', [prefix, prefix]); const result = []; try {
        while (rows.goToNextRow()) {
            const row = JSON.parse(rows.getString(1));
            ReceiptProtocol_1.ReceiptRules.validate(row, this.codec);
            if (row.identity.namespace !== rows.getString(0)) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            result.push(row);
        }
    }
    finally {
        rows.close();
    } this.check(); return result; }); }
    async stage(row, body = '') { this.files.disk(NativeStorageBudget_1.NativeStorageBudget.total([this.codec.bytes(body).length, this.codec.bytes(JSON.stringify(row)).length])); await this.local.syncAccess(async (db) => { db.beginTransaction(); try {
        await this.bound(db);
        await this.full(db);
        ReceiptProtocol_1.ReceiptRules.validate(row, this.codec);
        if (!MigrationStore_1.MigrationStore.samePrincipal(row.identity, this.identity)) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        if (row.kind !== 'storage') {
            await this.floor(db, row.message);
            if (row.kind === 'message_content') {
                if (await this.codec.hash(this.codec.bytes(body)) !== row.digest) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
                const data = {};
                data['bodyEnvelope'] = this.codec.encode(this.codec.bytes(body), true);
                data['bodyDigest'] = row.digest;
                await db.executeSql('INSERT INTO receipt_bodies VALUES(?,?,?,?)', [row.identity.namespace, row.id, row.message, JSON.stringify(data)]);
            }
            else {
                await this.files.verify(row.descriptor);
                await db.executeSql('INSERT INTO receipt_objects VALUES(?,?,?,?)', [row.identity.namespace, row.id, row.message, row.descriptor]);
            }
        }
        await db.executeSql('INSERT INTO receipt_intents VALUES(?,?,?)', [row.identity.namespace, row.id, JSON.stringify(row)]);
        this.check();
        db.commit();
    }
    catch (error) {
        db.rollBack();
        throw error;
    } }); }
    async stageObject(row, ref, bytes) { return this.local.syncAccess(async (db) => { db.beginTransaction(); try {
        await this.bound(db);
        await this.full(db);
        await this.floor(db, row.message);
        row.descriptor = await this.files.save(row.id, ref, bytes, this.codec);
        ReceiptProtocol_1.ReceiptRules.validate(row, this.codec);
        await db.executeSql('INSERT INTO receipt_objects VALUES(?,?,?,?)', [row.identity.namespace, row.id, row.message, row.descriptor]);
        await db.executeSql('INSERT INTO receipt_intents VALUES(?,?,?)', [row.identity.namespace, row.id, JSON.stringify(row)]);
        this.check();
        db.commit();
        return row;
    }
    catch (error) {
        db.rollBack();
        this.files.erase([row.id + '.bin']);
        throw error;
    } }); }
    async replace(before, after) { ReceiptProtocol_1.ReceiptRules.validate(after, this.codec); const copy = JSON.parse(JSON.stringify(before)); copy.state = after.state; copy.attempts = after.attempts; copy.result = after.result; if (JSON.stringify(copy) !== JSON.stringify(after) || after.attempts < before.attempts) {
        MigrationProtocol_1.MigrationRules.fail();
    } await this.local.syncAccess(async (db) => { db.beginTransaction(); try {
        this.check();
        await this.full(db);
        const current = (await this.values(db, 'SELECT payload FROM receipt_intents WHERE namespace=? AND id=?', [before.identity.namespace, before.id]))[0];
        if (current !== JSON.stringify(before)) {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_ORIGINAL_CHANGED');
        }
        await db.executeSql('UPDATE receipt_intents SET payload=? WHERE namespace=? AND id=?', [JSON.stringify(after), before.identity.namespace, before.id]);
        this.check();
        db.commit();
    }
    catch (error) {
        db.rollBack();
        throw error;
    } }); return after; }
    async body(row) { return this.local.syncAccess(async (db) => { this.check(); return (await this.values(db, 'SELECT payload FROM receipt_bodies WHERE namespace=? AND id=?', [row.identity.namespace, row.id]))[0] ?? ''; }); }
    async intent(id) { await this.binding(); const row = (await this.rows()).find((item) => item.id === id && item.identity.namespace === this.identity.namespace && item.identity.store === this.identity.store && item.state === 'recorded'); if (!row) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_LOCAL_PROVENANCE_REQUIRED');
    } return row; }
    async migrationSources() { await this.binding(); const rows = await this.rows(); const out = []; for (const row of rows.filter((item) => item.kind === 'message_content' && item.state === 'recorded' && item.identity.namespace === this.identity.namespace && item.identity.store === this.identity.store)) {
        const body = await this.body(row);
        if (!body) {
            continue;
        }
        const parsed = SyncProtocol_1.SyncJSON.parse(body);
        const raw = this.codec.text(MigrationProtocol_1.MigrationRules.decode(parsed.get('bodyEnvelope').string(), 2 * 1024 * 1024, true, this.codec));
        if (await this.codec.hash(this.codec.bytes(raw)) !== row.digest) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const m = SyncProtocol_1.SyncJSON.parse(raw);
        if (m.get('forwarded').value === true) {
            continue;
        }
        const candidate = {};
        candidate['id'] = row.message;
        candidate['conversationId'] = m.get('conversationId').string();
        candidate['contentVersion'] = row.content;
        candidate['type'] = m.get('type').string();
        candidate['text'] = m.get('text').string();
        candidate['sourceReceiptId'] = row.id;
        if (m.has('attachmentId') && m.get('attachmentId').string()) {
            candidate['attachmentId'] = m.get('attachmentId').string();
        }
        const object = rows.find((item) => item.kind === 'object_received' && item.state === 'recorded' && item.identity.namespace === this.identity.namespace && SyncProtocol_1.SyncJSON.parse(item.body).get('parentReceiptId').value === row.id);
        if (object) {
            candidate['sourceObjectReceiptId'] = object.id;
        }
        const protectedBody = await this.local.syncAccess(async (db) => { await this.bound(db); return (await this.values(db, 'SELECT id FROM sync_terminal WHERE scope=? AND id=?', [this.scope(), row.message])).length === 0; });
        if (protectedBody) {
            out.push(JSON.stringify(candidate));
        }
    } this.check(); const unique = new Map(); for (const raw of out) {
        const row = SyncProtocol_1.SyncJSON.parse(raw);
        const id = row.get('id').string();
        const before = unique.get(id);
        if (!before || SyncProtocol_1.SyncRules.compare(SyncProtocol_1.SyncJSON.parse(before).get('contentVersion').string(), row.get('contentVersion').string()) < 0) {
            unique.set(id, raw);
        }
    } return Array.from(unique.values()); }
    async migrationSource(raw) {
        const selected = SyncProtocol_1.SyncJSON.parse(raw);
        const row = await this.intent(selected.get('sourceReceiptId').string());
        if (row.kind !== 'message_content' || row.message !== selected.get('id').string() || row.content !== selected.get('contentVersion').string()) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        ReceiptProtocol_1.ReceiptRules.result(row.result, row);
        const cached = await this.body(row);
        if (!cached) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const body = SyncProtocol_1.SyncJSON.parse(cached);
        const bytes = MigrationProtocol_1.MigrationRules.decode(body.get('bodyEnvelope').string(), 2 * 1024 * 1024, true, this.codec);
        if (await this.codec.hash(bytes) !== row.digest) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const m = SyncProtocol_1.SyncJSON.parse(this.codec.text(bytes));
        if (['id', 'conversationId', 'contentVersion', 'type'].some((key) => m.get(key).value !== selected.get(key).value) || m.get('forwarded').value === true || m.get('attachmentId').value !== selected.get('attachmentId').value) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const envelope = {};
        envelope['bodyEnvelope'] = body.get('bodyEnvelope').string();
        envelope['sourceReceiptId'] = row.id;
        envelope['objectMode'] = selected.get('objectMode').string();
        let fileBytes;
        let objectInfo = '';
        if (selected.get('objectMode').value === 'full') {
            const object = await this.intent(selected.get('sourceObjectReceiptId').string());
            if (object.kind !== 'object_received' || SyncProtocol_1.SyncJSON.parse(object.body).get('parentReceiptId').value !== row.id) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            ReceiptProtocol_1.ReceiptRules.result(object.result, object);
            fileBytes = await this.files.read(object.descriptor);
            const descriptor = SyncProtocol_1.SyncJSON.parse(object.descriptor);
            if (await this.codec.hash(fileBytes) !== descriptor.get('sha256').string()) {
                fileBytes.fill(0);
                MigrationProtocol_1.MigrationRules.fail();
            }
            const metadata = {};
            metadata['objectId'] = descriptor.get('id').string();
            metadata['messageId'] = row.message;
            metadata['sourceObjectReceiptId'] = object.id;
            objectInfo = JSON.stringify(metadata);
        }
        try {
            await this.local.syncAccess(async (db) => { await this.bound(db); await this.floor(db, row.message, true); });
            this.check();
            return { envelope: JSON.stringify(envelope), objectInfo: objectInfo, bytes: fileBytes };
        }
        catch (error) {
            fileBytes?.fill(0);
            throw error;
        }
    }
    async measured(build) { const raw = await this.local.syncAccess(async (db) => { await this.bound(db); return (await this.values(db, 'SELECT payload FROM receipt_measurements WHERE namespace=? AND build=?', [this.identity.namespace, build]))[0]; }); if (raw) {
        await ReceiptProtocol_1.ReceiptRules.measurement(raw, this.identity, build, this.codec);
    } this.check(); return raw; }
    async measure(objects) {
        this.files.disk();
        const build = await this.files.buildDigest();
        const old = await this.measured(build);
        if (old && SyncProtocol_1.SyncJSON.parse(old).get('storageContract').string() === (objects ? ReceiptProtocol_1.ReceiptRules.objectContract : ReceiptProtocol_1.ReceiptRules.contentContract)) {
            return old;
        }
        return this.local.syncAccess(async (db) => {
            db.beginTransaction();
            try {
                await this.bound(db);
                await this.full(db);
                const existing = (await this.values(db, "SELECT payload FROM receipt_probes WHERE namespace=? AND kind='metadata'", [this.identity.namespace]))[0];
                if (existing) {
                    const meta = SyncProtocol_1.SyncJSON.parse(existing);
                    if (meta.get('buildDigest').string() !== build || meta.get('store').string() !== this.identity.store || meta.get('epoch').string() !== this.identity.epoch || meta.get('objects').bool() !== objects) {
                        MigrationProtocol_1.MigrationRules.fail('RECEIPT_PROBE_CHANGED');
                    }
                    if (meta.get('launch').string() === ReceiptStore.launch) {
                        db.commit();
                        return '{"ready":false,"restartRequired":true}';
                    }
                    const id = meta.get('id').string();
                    if ((await this.values(db, 'SELECT payload FROM receipt_probes WHERE namespace=? AND id=? AND kind=?', [this.identity.namespace, id, 'body']))[0] !== 'JobsIM durable body v1' || (await this.values(db, 'SELECT payload FROM receipt_probes WHERE namespace=? AND id=? AND kind=?', [this.identity.namespace, id, 'intent']))[0] !== 'JobsIM durable intent v1') {
                        MigrationProtocol_1.MigrationRules.fail('RECEIPT_REOPEN_FAILED');
                    }
                    if (objects) {
                        await this.files.verify(meta.get('descriptor').raw());
                    }
                    const report = {};
                    report['protocol'] = ReceiptProtocol_1.ReceiptRules.protocol;
                    const identity = {};
                    identity['profile'] = this.identity.profile;
                    identity['deployment'] = this.identity.deployment;
                    identity['user'] = this.identity.user;
                    identity['device'] = this.identity.device;
                    identity['epoch'] = this.identity.epoch;
                    identity['namespace'] = this.identity.namespace;
                    identity['store'] = this.identity.store;
                    report['identity'] = identity;
                    report['buildDigest'] = build;
                    report['storageContract'] = objects ? ReceiptProtocol_1.ReceiptRules.objectContract : ReceiptProtocol_1.ReceiptRules.contentContract;
                    report['writerFull'] = true;
                    report['reopenBody'] = true;
                    report['reopenIntent'] = true;
                    report['objectFsync'] = objects;
                    report['directoryFsync'] = objects;
                    report['verificationDigest'] = await this.codec.hash(this.codec.bytes(new SyncProtocol_1.SyncJSON(report).canonical()));
                    const raw = JSON.stringify(report);
                    await ReceiptProtocol_1.ReceiptRules.measurement(raw, this.identity, build, this.codec);
                    await db.executeSql('INSERT OR REPLACE INTO receipt_measurements VALUES(?,?,?)', [this.identity.namespace, build, raw]);
                    if (objects) {
                        await db.executeSql('INSERT OR IGNORE INTO receipt_cleanup VALUES(?,?)', [this.identity.namespace, id + '.bin']);
                    }
                    await db.executeSql('DELETE FROM receipt_probes WHERE namespace=?', [this.identity.namespace]);
                    this.check();
                    db.commit();
                    return raw;
                }
                const id = _ohos_util_1.default.generateRandomUUID(true);
                const meta = {};
                meta['id'] = id;
                meta['launch'] = ReceiptStore.launch;
                meta['buildDigest'] = build;
                meta['store'] = this.identity.store;
                meta['epoch'] = this.identity.epoch;
                meta['objects'] = objects;
                if (objects) {
                    const bytes = this.codec.bytes('JobsIM durable original v1');
                    const ref = {};
                    ref['objectId'] = id;
                    ref['sizeBytes'] = String(bytes.length);
                    ref['sha256'] = await this.codec.hash(bytes);
                    ref['contentType'] = 'application/octet-stream';
                    meta['descriptor'] = JSON.parse(await this.files.save(id, JSON.stringify(ref), bytes, this.codec));
                    bytes.fill(0);
                }
                for (const kind of ['body', 'intent']) {
                    await db.executeSql('INSERT INTO receipt_probes VALUES(?,?,?,?)', [this.identity.namespace, id, kind, 'JobsIM durable ' + kind + ' v1']);
                }
                await db.executeSql('INSERT INTO receipt_probes VALUES(?,?,?,?)', [this.identity.namespace, id, 'metadata', JSON.stringify(meta)]);
                this.check();
                db.commit();
                return '{"ready":false,"restartRequired":true}';
            }
            catch (error) {
                db.rollBack();
                throw error;
            }
        });
    }
}
exports.ReceiptStore = ReceiptStore;
ReceiptStore.launch = _ohos_util_1.default.generateRandomUUID(true);

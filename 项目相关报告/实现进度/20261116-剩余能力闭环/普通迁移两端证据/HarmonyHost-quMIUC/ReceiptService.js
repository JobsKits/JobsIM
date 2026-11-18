"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ReceiptService = void 0;
// Created by Jobs on 2026年11月17日，星期二.
const Models_1 = require("./Models");
const SyncProtocol_1 = require("./SyncProtocol");
const MigrationProtocol_1 = require("./MigrationProtocol");
const MigrationStore_1 = require("./MigrationStore");
const ReceiptProtocol_1 = require("./ReceiptProtocol");
class ReceiptService {
    constructor(ports) {
        this.running = false;
        this.stopped = false;
        this.ports = ports;
        ports.store.bindLifecycle?.(() => !this.stopped && ports.current());
    }
    clear() { this.stopped = true; }
    check() { if (this.stopped || !this.ports.current()) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_CONTEXT_CHANGED');
    } }
    async locked(action) { this.check(); if (this.running) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_BUSY');
    } this.running = true; try {
        return await action();
    }
    finally {
        this.running = false;
    } }
    copy(row) { return JSON.parse(JSON.stringify(row)); }
    original(row) { this.check(); if (!MigrationStore_1.MigrationStore.samePrincipal(row.identity, this.ports.identity)) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_ORIGINAL_FAMILY_REQUIRED');
    } }
    async request(path, method = 'GET', body = '') { this.check(); const raw = await this.ports.request(path, method, body, this.ports.identity.store); this.check(); return raw; }
    principal() { return this.ports.identity; }
    async objectBytes(parentId) { const raw = (await this.ports.store.migrationSources()).find((item) => SyncProtocol_1.SyncJSON.parse(item).get('sourceReceiptId').value === parentId); if (!raw) {
        MigrationProtocol_1.MigrationRules.fail();
    } const original = SyncProtocol_1.SyncJSON.parse(raw); const selected = {}; for (const key of ['id', 'conversationId', 'contentVersion', 'type', 'attachmentId', 'sourceReceiptId', 'sourceObjectReceiptId']) {
        if (original.has(key)) {
            selected[key] = original.get(key).value;
        }
    } selected['objectMode'] = 'full'; const data = await this.ports.store.migrationSource(JSON.stringify(selected)); const bytes = data.bytes; if (!bytes) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_LOCAL_OBJECT_MISSING');
    } try {
        this.check();
        return bytes;
    }
    catch (error) {
        bytes.fill(0);
        throw error;
    } }
    async archive() { this.check(); const rows = await this.ports.store.migrationSources(); this.check(); return rows; }
    async rows() { this.check(); const rows = await this.ports.store.rows(); this.check(); return rows; }
    async reconcile() { for (const row of (await this.rows()).filter((item) => ['pending', 'unknown'].includes(item.state) && item.identity.namespace === this.ports.identity.namespace && item.identity.store === this.ports.identity.store)) {
        try {
            await this.resume(row.id, false);
        }
        catch (_) {
            this.check();
        }
    } }
    async context() { const schema = await this.ports.store.control('schema'); return ReceiptProtocol_1.ReceiptRules.context(await this.request('/sync/receipt-context?schemaVersion=' + schema), this.ports.identity, schema).raw(); }
    async measurement() { const build = await this.ports.store.files.buildDigest(); this.check(); return this.ports.store.measured(build); }
    async measure(objects) { return this.locked(async () => { const raw = await this.ports.store.measure(objects); this.check(); return raw; }); }
    async measured() { const raw = await this.measurement(); if (!raw) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_LOCAL_VERIFICATION_REQUIRED');
    } return SyncProtocol_1.SyncJSON.parse(raw); }
    async profiles() { const local = await this.measured(); const raw = SyncProtocol_1.SyncJSON.parse(await this.request('/sync/receipt-profiles?schemaVersion=' + await this.ports.store.control('schema') + '&buildDigest=' + local.get('buildDigest').string())); if (raw.get('protocol').string() !== ReceiptProtocol_1.ReceiptRules.protocol || raw.get('qualifiedAck').value !== false || raw.get('declarationRequired').value !== true || raw.get('localDurabilityVerificationRequired').value !== true) {
        MigrationProtocol_1.MigrationRules.fail();
    } return raw.get('profiles').list().filter((row) => { MigrationProtocol_1.MigrationRules.exact(row, ['id', 'buildDigest', 'verificationDigest', 'storageContract']); return row.get('buildDigest').value === local.get('buildDigest').value && row.get('verificationDigest').value === local.get('verificationDigest').value && row.get('storageContract').value === local.get('storageContract').value; }).map((row) => row.raw()); }
    async qualified(objects = false) { await this.ports.store.binding(); const local = await this.measured(); const q = SyncProtocol_1.SyncJSON.parse(await this.context()).get('storage'); if (q.get('qualified').value !== true || objects && q.get('objectQualified').value !== true || ['buildDigest', 'verificationDigest', 'storageContract'].some((key) => q.get(key).value !== local.get(key).value)) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_CURRENT_QUALIFICATION_REQUIRED');
    } return q; }
    intent(id, kind, body, schema) { return { id: id, kind: kind, identity: this.ports.identity, schema: schema, body: body, message: '', content: '', digest: '', proof: '', descriptor: '', expires: '', state: 'pending', attempts: 0, result: '' }; }
    async declaration(profile) { return this.locked(async () => { if ((await this.rows()).some((row) => row.kind === 'storage' && ['pending', 'unknown'].includes(row.state) && MigrationStore_1.MigrationStore.samePrincipal(row.identity, this.ports.identity))) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_ORIGINAL_PENDING');
    } const local = await this.measured(); const selected = (await this.profiles()).map((raw) => SyncProtocol_1.SyncJSON.parse(raw)).find((item) => item.get('id').value === profile); if (!selected) {
        MigrationProtocol_1.MigrationRules.fail('RECEIPT_PROFILE_NOT_APPROVED');
    } const id = this.ports.uuid(); const b = {}; b['requestId'] = id; b['storeGeneration'] = this.ports.identity.store; b['revisionEpoch'] = this.ports.identity.epoch; b['profileId'] = profile; b['buildDigest'] = local.get('buildDigest').string(); b['verificationDigest'] = local.get('verificationDigest').string(); const row = this.intent(id, 'storage', JSON.stringify(b), await this.ports.store.control('schema')); await this.ports.store.stage(row); this.check(); return row; }); }
    route(row, result) { return row.kind === 'storage' ? result ? '/sync/storage-results' : '/sync/storage' : row.kind === 'message_content' ? result ? '/sync/receipt-results' : '/sync/receipts' : result ? '/sync/object-receipt-results' : '/sync/object-receipts'; }
    async lookup(row) { if (row.state === 'retired') {
        return row;
    } let raw; try {
        raw = await this.request(this.route(row, true), 'POST', row.body);
    }
    catch (error) {
        if (error instanceof Models_1.ApiFailure && error.status === 404 && error.code === (row.kind === 'storage' ? 'STORAGE_DECLARATION_NOT_FOUND' : row.kind === 'message_content' ? 'RECEIPT_OPERATION_NOT_FOUND' : 'OBJECT_RECEIPT_OPERATION_NOT_FOUND')) {
            return undefined;
        }
        throw error;
    } raw = ReceiptProtocol_1.ReceiptRules.result(raw, row); const next = this.copy(row); next.state = 'recorded'; next.result = raw; return this.ports.store.replace(row, next); }
    async resume(id, retry = false) { return this.locked(async () => { const row = (await this.rows()).find((item) => item.id === id); if (!row) {
        MigrationProtocol_1.MigrationRules.fail();
    } return this.resumeOriginal(row, retry); }); }
    async resumeOriginal(input, retry) {
        this.check();
        let row = input;
        if (row.state === 'retired') {
            return row;
        }
        const found = await this.lookup(row);
        if (found) {
            return found;
        }
        if (!retry) {
            return undefined;
        }
        this.original(row);
        await this.ports.store.binding();
        const body = SyncProtocol_1.SyncJSON.parse(row.body);
        if (row.kind === 'storage') {
            const local = await this.measured();
            if (body.get('buildDigest').value !== local.get('buildDigest').value || body.get('verificationDigest').value !== local.get('verificationDigest').value || !(await this.profiles()).some((raw) => SyncProtocol_1.SyncJSON.parse(raw).get('id').value === body.get('profileId').value)) {
                MigrationProtocol_1.MigrationRules.fail();
            }
        }
        else {
            const q = await this.qualified(row.kind === 'object_received');
            if (q.get('qualificationId').value !== body.get('qualificationId').value || Date.parse(row.expires) <= Date.now()) {
                MigrationProtocol_1.MigrationRules.fail('RECEIPT_PROOF_EXPIRED');
            }
            if (row.kind === 'object_received') {
                await this.ports.store.files.verify(row.descriptor);
            }
            else {
                const cached = await this.ports.store.body(row);
                if (!cached) {
                    MigrationProtocol_1.MigrationRules.fail('RECEIPT_LOCAL_BODY_MISSING');
                }
                const c = SyncProtocol_1.SyncJSON.parse(cached);
                const bytes = MigrationProtocol_1.MigrationRules.decode(c.get('bodyEnvelope').string(), 2 * 1024 * 1024, true, this.ports.codec);
                if (await this.ports.codec.hash(bytes) !== row.digest) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
        }
        const unknown = this.copy(row);
        unknown.state = 'unknown';
        unknown.attempts++;
        row = await this.ports.store.replace(row, unknown);
        this.original(row);
        await this.ports.store.binding();
        const raw = ReceiptProtocol_1.ReceiptRules.result(await this.request(this.route(row, false), row.kind === 'storage' ? 'PUT' : 'POST', row.body), row);
        const done = this.copy(row);
        done.state = 'recorded';
        done.result = raw;
        return this.ports.store.replace(row, done);
    }
    async proof(raw, prefix, qualification) { const r = SyncProtocol_1.SyncJSON.parse(raw); const p = ReceiptProtocol_1.ReceiptRules.proof(r.get('challenge').string(), prefix, this.ports.identity, qualification, this.ports.codec, await this.ports.store.control('permission')); const expiry = Date.parse(r.get('expiresAt').string()); if (expiry !== p.get('expires').integer(Number.MAX_SAFE_INTEGER) * 1000 || expiry <= Date.now() || expiry > Date.now() + 3660000 || r.get('objectComplete').value !== false || r.get('deletionEligible').value !== false) {
        MigrationProtocol_1.MigrationRules.fail();
    } const keys = ['messageId', 'conversationId', 'contentVersion', 'restoreEpoch', 'storeGeneration', 'qualificationId', 'bodyDigest']; const proofKeys = ['message', 'conversation', 'contentVersion', 'epoch', 'store', 'qualification', 'bodyDigest']; if (keys.some((key, index) => r.get(key).value !== p.get(proofKeys[index]).value)) {
        MigrationProtocol_1.MigrationRules.fail();
    } return p; }
    async content(message, expectedVersion = '', expectedPair = []) {
        return this.locked(async () => {
            MigrationProtocol_1.MigrationRules.id(message);
            const pending = (await this.rows()).find((row) => row.kind === 'message_content' && row.message === message && (!expectedVersion || row.content === expectedVersion) && row.identity.namespace === this.ports.identity.namespace && ['pending', 'unknown'].includes(row.state));
            if (pending) {
                return this.resumeOriginal(pending, false);
            }
            const q = await this.qualified();
            const request = {};
            request['messageId'] = message;
            request['schemaVersion'] = await this.ports.store.control('schema');
            const raw = await this.request('/sync/receipt-challenges', 'POST', JSON.stringify(request));
            const r = SyncProtocol_1.SyncJSON.parse(raw);
            const p = await this.proof(raw, 'rc1', q.get('qualificationId').string());
            if (r.get('requiresAtomicLocalCommit').value !== true || p.get('message').string() !== message) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            if (expectedVersion && p.get('contentVersion').value !== expectedVersion || expectedPair.length && JSON.stringify(ReceiptProtocol_1.ReceiptRules.pair(p)) !== JSON.stringify(expectedPair)) {
                MigrationProtocol_1.MigrationRules.fail('RECEIPT_ORIGINAL_CHANGED');
            }
            const bytes = MigrationProtocol_1.MigrationRules.decode(r.get('bodyEnvelope').string(), 2 * 1024 * 1024, true, this.ports.codec);
            if (await this.ports.codec.hash(bytes) !== r.get('bodyDigest').string()) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            const body = this.ports.codec.text(bytes);
            const m = SyncProtocol_1.SyncJSON.parse(body);
            for (const key of ['id', 'conversationId', 'senderId']) {
                MigrationProtocol_1.MigrationRules.id(m.get(key).string());
            }
            if (m.get('id').string() !== message || m.get('conversationId').value !== p.get('conversation').value || m.get('contentVersion').value !== p.get('contentVersion').value || m.get('deleted').value !== false || m.get('forwarded').value === true || m.get('contentUnavailable').value === true || m.get('forwardUnavailable').value === true || !['text', 'image', 'audio', 'video', 'file'].includes(m.get('type').string()) || m.has('attachmentURL') && m.get('attachmentURL').string()) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            if (m.get('type').string() === 'text') {
                if (m.has('attachmentId') && m.get('attachmentId').string()) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
            else {
                MigrationProtocol_1.MigrationRules.id(m.get('attachmentId').string());
            }
            MigrationProtocol_1.MigrationRules.positive(m.get('creationEpoch').string());
            MigrationProtocol_1.MigrationRules.positive(m.get('creationSequence').string());
            if (this.ports.codec.bytes(m.get('text').string()).length > 16384 || !Number.isFinite(Date.parse(m.get('createdAt').string()))) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            const recorded = (await this.rows()).find((row) => row.kind === 'message_content' && row.message === message && row.content === p.get('contentVersion').string() && row.state === 'recorded' && MigrationStore_1.MigrationStore.samePrincipal(row.identity, this.ports.identity) && SyncProtocol_1.SyncJSON.parse(row.body).get('qualificationId').value === q.get('qualificationId').value);
            if (recorded) {
                return recorded;
            }
            const id = this.ports.uuid();
            const b = {};
            b['requestId'] = id;
            b['storeGeneration'] = this.ports.identity.store;
            b['revisionEpoch'] = this.ports.identity.epoch;
            b['qualificationId'] = q.get('qualificationId').string();
            b['kind'] = 'message_content';
            b['challenge'] = r.get('challenge').string();
            b['bodyDigest'] = p.get('bodyDigest').string();
            const row = this.intent(id, 'message_content', JSON.stringify(b), await this.ports.store.control('schema'));
            row.message = message;
            row.content = p.get('contentVersion').string();
            row.digest = p.get('bodyDigest').string();
            row.proof = p.raw();
            row.expires = r.get('expiresAt').string();
            await this.ports.store.stage(row, body);
            return this.resumeOriginal(row, true);
        });
    }
    async objectOriginal(parentId) {
        return this.locked(async () => {
            const parent = (await this.rows()).find((row) => row.id === parentId);
            if (!parent) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            this.original(parent);
            if (parent.kind !== 'message_content' || parent.state !== 'recorded') {
                MigrationProtocol_1.MigrationRules.fail();
            }
            const pending = (await this.rows()).find((row) => row.kind === 'object_received' && row.identity.namespace === this.ports.identity.namespace && ['pending', 'unknown'].includes(row.state) && SyncProtocol_1.SyncJSON.parse(row.body).get('parentReceiptId').value === parentId);
            if (pending) {
                return this.resumeOriginal(pending, false);
            }
            const q = await this.qualified(true);
            if (SyncProtocol_1.SyncJSON.parse(parent.body).get('qualificationId').value !== q.get('qualificationId').value) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            const request = {};
            request['parentReceiptId'] = parentId;
            request['schemaVersion'] = await this.ports.store.control('schema');
            const raw = await this.request('/sync/object-receipt-challenges', 'POST', JSON.stringify(request));
            const r = SyncProtocol_1.SyncJSON.parse(raw);
            const p = await this.proof(raw, 'ro1', q.get('qualificationId').string());
            if (r.get('requiresDurableOriginalObject').value !== true || r.get('targetRangeComplete').value !== false || r.get('parentReceiptId').value !== parentId || p.get('parentReceiptId').value !== parentId || p.get('message').value !== parent.message || p.get('contentVersion').value !== parent.content || p.get('bodyDigest').value !== parent.digest || !ReceiptProtocol_1.ReceiptRules.samePair(p, SyncProtocol_1.SyncJSON.parse(parent.proof))) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            const ref = r.get('object');
            for (const key of ['objectId', 'sizeBytes']) {
                if (ref.get(key).value !== p.get(key).value) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
            if (ref.get('sha256').value !== p.get('objectDigest').value || Number(ref.get('sizeBytes').string()) > 64 * 1024 * 1024 || ref.get('contentType').string().length > 256 || /[\r\n]/.test(ref.get('contentType').string())) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            MigrationProtocol_1.MigrationRules.id(ref.get('objectId').string());
            MigrationProtocol_1.MigrationRules.digest(ref.get('sha256').string());
            SyncProtocol_1.SyncRules.counter(ref.get('sizeBytes').string());
            const size = Number(ref.get('sizeBytes').string());
            const id = this.ports.uuid();
            const bytes = await this.ports.binary('/media/' + ref.get('objectId').string(), this.ports.identity.store, Math.max(1, size));
            try {
                this.check();
                const b = {};
                b['requestId'] = id;
                b['storeGeneration'] = this.ports.identity.store;
                b['revisionEpoch'] = this.ports.identity.epoch;
                b['qualificationId'] = q.get('qualificationId').string();
                b['kind'] = 'object_received';
                b['parentReceiptId'] = parentId;
                b['challenge'] = r.get('challenge').string();
                b['objectId'] = ref.get('objectId').string();
                b['sizeBytes'] = ref.get('sizeBytes').string();
                b['objectDigest'] = ref.get('sha256').string();
                const row = this.intent(id, 'object_received', JSON.stringify(b), await this.ports.store.control('schema'));
                row.message = parent.message;
                row.content = parent.content;
                row.digest = parent.digest;
                row.proof = p.raw();
                row.expires = r.get('expiresAt').string();
                const committed = await this.ports.store.stageObject(row, ref.raw(), bytes);
                return await this.resumeOriginal(committed, true);
            }
            finally {
                bytes.fill(0);
            }
        });
    }
    async receiveIfTarget(message, version) { MigrationProtocol_1.MigrationRules.id(message); MigrationProtocol_1.MigrationRules.positive(version); const q = await this.qualified(); const raw = SyncProtocol_1.SyncJSON.parse(await this.request('/sync/receipt-targets/' + message + '?schemaVersion=' + await this.ports.store.control('schema') + '&qualificationId=' + q.get('qualificationId').string())); if (raw.get('protocol').value !== ReceiptProtocol_1.ReceiptRules.protocol || raw.get('messageId').value !== message || raw.get('qualificationId').value !== q.get('qualificationId').value || raw.get('restoreEpoch').value !== this.ports.identity.epoch || raw.get('contentVersion').value !== version || raw.get('deletionEligible').value !== false) {
        MigrationProtocol_1.MigrationRules.fail();
    } if (raw.get('messageCurrent').value !== true || raw.get('contentTarget').value !== true || raw.get('contentRecorded').value !== false) {
        return undefined;
    } const pair = ReceiptProtocol_1.ReceiptRules.pair(raw); if (!pair[0]) {
        MigrationProtocol_1.MigrationRules.fail();
    } return this.content(message, version, pair); }
    async targets(id) { const row = (await this.rows()).find((item) => item.id === id); if (!row || row.state === 'retired' || row.kind === 'storage') {
        MigrationProtocol_1.MigrationRules.fail();
    } return ReceiptProtocol_1.ReceiptRules.target(await this.request('/sync/receipt-targets/' + row.message + '?schemaVersion=' + row.schema + '&qualificationId=' + SyncProtocol_1.SyncJSON.parse(row.body).get('qualificationId').string()), row); }
}
exports.ReceiptService = ReceiptService;

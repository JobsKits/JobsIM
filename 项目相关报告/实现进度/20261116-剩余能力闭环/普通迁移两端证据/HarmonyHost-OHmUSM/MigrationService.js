"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.MigrationService = void 0;
// Created by Jobs on 2026年11月17日，星期二.
const Models_1 = require("./Models");
const SyncProtocol_1 = require("./SyncProtocol");
const Deletion_1 = require("./Deletion");
const MigrationProtocol_1 = require("./MigrationProtocol");
const ReceiptProtocol_1 = require("./ReceiptProtocol");
const MigrationStore_1 = require("./MigrationStore");
class MigrationService {
    constructor(ports) {
        this.running = false;
        this.stopped = false;
        this.proofs = new Map();
        this.ports = ports;
        ports.store.bindLifecycle?.(() => !this.stopped && ports.current());
    }
    clear() { this.stopped = true; this.proofs.clear(); }
    check() { if (this.stopped || !this.ports.current()) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_CONTEXT_CHANGED');
    } }
    original(row) { this.check(); if (!MigrationStore_1.MigrationStore.samePrincipal(row.identity, this.ports.identity)) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_ORIGINAL_SESSION_REQUIRED');
    } }
    async locked(action) { this.check(); if (this.running) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_BUSY');
    } this.running = true; try {
        return await action();
    }
    finally {
        this.running = false;
    } }
    async request(path, method = 'GET', body = '') { this.check(); const raw = await this.ports.request(path, method, body, this.ports.identity.store); this.check(); return raw; }
    async qualifiedSources() { this.check(); const rows = this.ports.qualifiedSources ? await this.ports.qualifiedSources() : []; this.check(); return rows; }
    async rows() { this.check(); const rows = await this.ports.store.rows(); this.check(); return rows; }
    async capability() {
        const c = SyncProtocol_1.SyncJSON.parse(await this.request('/sync/migration-capabilities'));
        if (c.get('enabled').value !== true || c.get('format').string() !== MigrationProtocol_1.MigrationRules.format || c.get('sourceContextId').string() !== 'personal' || c.get('qualifiedAck').value !== false || c.get('targetQualified').value !== false) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_UNAVAILABLE');
        }
        const keys = ['maxMessages', 'maxObjects', 'maxRequestBytes', 'maxObjectBytes', 'maxLifetimeSeconds', 'pairingLifetimeSeconds', 'installLifetimeSeconds'];
        const values = [256, 64, MigrationProtocol_1.MigrationRules.limit, 64 * 1024 * 1024, 600, 300, 5];
        for (let at = 0; at < keys.length; at++) {
            if (c.get(keys[at]).integer(MigrationProtocol_1.MigrationRules.limit) !== values[at]) {
                MigrationProtocol_1.MigrationRules.fail();
            }
        }
        await this.ports.store.binding();
        this.check();
    }
    async stage(kind, selection = '[]', migration = '') { await this.capability(); const row = await this.ports.store.stage(kind, selection, this.ports.uuid(), migration); this.check(); return row; }
    async save(before, after) { const saved = await this.ports.store.replace(before, after); this.check(); return saved; }
    copy(row) { return JSON.parse(JSON.stringify(row)); }
    proof(password, otp) { const body = {}; body['password'] = Deletion_1.DeletionRules.password(password); if (otp) {
        if (!/^[0-9]{6}$/.test(otp)) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        body['otp'] = otp;
    } return body; }
    pairing(token) {
        if (token.length > 8192) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const parts = token.split('.');
        const codec = this.ports.codec;
        if (parts.length !== 3 || parts[0] !== 'mp1' || MigrationProtocol_1.MigrationRules.decode(parts[2], 32, true, codec).length !== 32) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const p = SyncProtocol_1.SyncJSON.parse(codec.text(MigrationProtocol_1.MigrationRules.decode(parts[1], 8192, true, codec)));
        const i = this.ports.identity;
        if (p.get('scopeId').string() !== i.deployment || p.get('userId').string() !== i.user || p.get('restoreEpoch').string() !== i.epoch) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_TARGET_MISMATCH');
        }
        for (const key of ['pairingId', 'targetDeviceId', 'targetStoreGeneration']) {
            MigrationProtocol_1.MigrationRules.id(p.get(key).string());
        }
        for (const key of ['securityVersion', 'sourceGeneration']) {
            MigrationProtocol_1.MigrationRules.positive(p.get(key).string());
        }
        const expiry = p.get('expires').integer(Number.MAX_SAFE_INTEGER) * 1000;
        if (expiry <= Date.now() || expiry > Date.now() + 301000) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_PAIRING_EXPIRED');
        }
        const out = {};
        for (const key of ['pairingId', 'targetDeviceId', 'targetStoreGeneration', 'restoreEpoch', 'securityVersion', 'sourceGeneration', 'expires']) {
            out[key] = p.get(key).value;
        }
        return JSON.stringify(out);
    }
    async pairResult(row, raw) {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        const token = value.get('pairingProof').string();
        const target = this.pairing(token);
        const p = SyncProtocol_1.SyncJSON.parse(target);
        if (value.get('sourcePreserved').value !== true || value.get('pairingId').string() !== p.get('pairingId').string() || value.get('targetDeviceId').string() !== row.identity.device || value.get('targetStoreGeneration').string() !== row.identity.store || value.get('restoreEpoch').string() !== row.identity.epoch || Date.parse(value.get('expiresAt').string()) !== p.get('expires').integer(Number.MAX_SAFE_INTEGER) * 1000) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_ORIGINAL_CHANGED');
        }
        const next = this.copy(row);
        next.state = 'ready';
        next.target = target;
        await this.save(row, next);
        this.proofs.set(row.id, token);
    }
    publicView(raw) { const value = SyncProtocol_1.SyncJSON.parse(raw); const out = {}; for (const key of ['id', 'requestId', 'status', 'manifest', 'manifestEnvelope', 'manifestDigest', 'capsuleSHA256', 'sizeBytes', 'expiresAt', 'sourcePreserved', 'qualifiedAck', 'targetQualified']) {
        if (value.has(key)) {
            out[key] = value.get(key).value;
        }
    } return JSON.stringify(out); }
    async remember(row, raw) {
        const v = await MigrationProtocol_1.MigrationRules.view(raw, this.ports.identity, this.ports.codec, row.kind === 'create' ? row.id : '');
        const m = SyncProtocol_1.SyncJSON.parse(v.manifest);
        this.check();
        if (row.view && SyncProtocol_1.SyncJSON.parse(row.view).get('manifestDigest').string() !== v.digest) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_ORIGINAL_CHANGED');
        }
        if (row.kind === 'create') {
            const selected = SyncProtocol_1.SyncJSON.parse(row.selection).list();
            const refs = m.get('messages').list();
            if (selected.length !== refs.length) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            for (let at = 0; at < selected.length; at++) {
                const s = selected[at];
                const ref = refs[at];
                if (ref.get('messageId').string() !== s.get('id').string() || ref.get('conversationId').string() !== s.get('conversationId').string() || ref.get('contentVersion').string() !== s.get('contentVersion').string() || ref.get('objectId').value !== s.get('attachmentId').value || ref.get('objectMode').string() !== s.get('objectMode').string()) {
                    MigrationProtocol_1.MigrationRules.fail('MIGRATION_ORIGINAL_CHANGED');
                }
            }
            if (row.target) {
                const t = SyncProtocol_1.SyncJSON.parse(row.target);
                if (m.get('targetDeviceId').string() !== t.get('targetDeviceId').string() || m.get('targetStoreGeneration').string() !== t.get('targetStoreGeneration').string()) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
        }
        const next = this.copy(row);
        next.migration = v.id;
        next.view = this.publicView(raw);
        next.state = ['retired', 'revoking'].includes(v.state) ? 'retired' : v.state === 'building' ? 'unknown' : row.state === 'installed' ? 'installed' : 'ready';
        await this.save(row, next);
        if (['retired', 'revoking'].includes(v.state)) {
            await this.ports.store.discard(v.id);
        }
        return v;
    }
    async lookup(row) {
        if (row.kind === 'target') {
            const raw = await this.request('/sync/migrations/' + row.migration);
            await this.remember(row, raw);
            return raw;
        }
        const body = {};
        body['requestId'] = row.id;
        body[row.kind === 'pairing' ? 'storeGeneration' : 'sourceStoreGeneration'] = row.identity.store;
        const raw = await this.request(row.kind === 'pairing' ? '/sync/migration-pairing-results' : '/sync/migration-results', 'POST', JSON.stringify(body));
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        if (value.get('status').value === 'not_found') {
            if (value.keys().length !== 1) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            return undefined;
        }
        if (row.kind === 'pairing') {
            await this.pairResult(row, raw);
        }
        else {
            await this.remember(row, raw);
        }
        return raw;
    }
    async checkOriginal(id) { return this.locked(async () => { const row = (await this.rows()).find((item) => item.id === id); if (!row) {
        MigrationProtocol_1.MigrationRules.fail();
    } return this.lookup(row); }); }
    async pair(id, password, otp, retry) {
        return this.locked(async () => {
            let row = (await this.rows()).find((item) => item.id === id);
            if (!row || row.kind !== 'pairing') {
                MigrationProtocol_1.MigrationRules.fail();
            }
            if (await this.lookup(row)) {
                return this.proofs.get(row.id);
            }
            if (!retry) {
                return undefined;
            }
            this.original(row);
            await this.capability();
            this.ports.files.disk();
            if (row.attempts > 0 && Date.parse(row.created) + 300000 < Date.now()) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_PAIRING_EXPIRED');
            }
            const body = this.proof(password, otp);
            body['requestId'] = row.id;
            body['storeGeneration'] = row.identity.store;
            body['restoreEpoch'] = row.identity.epoch;
            const next = this.copy(row);
            next.attempts++;
            next.state = 'unknown';
            row = await this.save(row, next);
            this.original(row);
            await this.pairResult(row, await this.request('/sync/migration-pairings', 'POST', JSON.stringify(body)));
            return this.proofs.get(row.id);
        });
    }
    sharePair(id) { this.check(); const proof = this.proofs.get(id); if (!proof) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_PAIRING_RECHECK_REQUIRED');
    } this.pairing(proof); return proof; }
    async validateLocalSource(selected) {
        const intent = this.ports.receiptIntent;
        const request = this.ports.receiptRequest;
        if (!intent || !request) {
            MigrationProtocol_1.MigrationRules.fail('MIGRATION_LOCAL_PROVENANCE_REQUIRED');
        }
        const row = await intent(selected.get('sourceReceiptId').string());
        const p = SyncProtocol_1.SyncJSON.parse(row.proof);
        ReceiptProtocol_1.ReceiptRules.result(await request('/sync/receipt-results', 'POST', row.body, this.ports.identity.store), row);
        this.check();
        const pair = ReceiptProtocol_1.ReceiptRules.pair(p);
        if (pair[0]) {
            const target = await request('/sync/receipt-targets/' + row.message + '?schemaVersion=' + row.schema + '&qualificationId=' + SyncProtocol_1.SyncJSON.parse(row.body).get('qualificationId').string(), 'GET', '', this.ports.identity.store);
            ReceiptProtocol_1.ReceiptRules.target(target, row);
            if (SyncProtocol_1.SyncJSON.parse(target).get('messageCurrent').value !== true) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_MEMBERSHIP_CHANGED');
            }
        }
        else {
            const context = SyncProtocol_1.SyncJSON.parse(await this.ports.source('/sync/context?schemaVersion=' + row.schema));
            if (context.get('permissionGeneration').value !== p.get('permission').value) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_MEMBERSHIP_CHANGED');
            }
        }
        if (selected.get('objectMode').value === 'full') {
            const object = await intent(selected.get('sourceObjectReceiptId').string());
            ReceiptProtocol_1.ReceiptRules.result(await request('/sync/object-receipt-results', 'POST', object.body, this.ports.identity.store), object);
        }
        this.check();
    }
    async sourceData(original) {
        this.original(original);
        await this.ports.store.binding();
        this.ports.files.disk();
        const messages = [];
        const objects = [];
        let used = 0;
        const codec = this.ports.codec;
        for (const selected of SyncProtocol_1.SyncJSON.parse(original.selection).list()) {
            if (selected.has('sourceReceiptId')) {
                try {
                    const live = SyncProtocol_1.SyncJSON.parse(await this.ports.source('/messages/' + selected.get('id').string()));
                    this.check();
                    if (live.get('id').value !== selected.get('id').value || live.get('contentVersion').value !== selected.get('contentVersion').value || live.get('deleted').value === true || live.get('forwarded').value === true || live.get('contentUnavailable').value === true && live.get('deleteScope').value !== 'cloud_only') {
                        MigrationProtocol_1.MigrationRules.fail();
                    }
                }
                catch (error) {
                    if (!(error instanceof Models_1.ApiFailure) || error.status !== 409 || error.code !== 'RETENTION_CLOUD_EXPIRED') {
                        throw error;
                    }
                    this.check();
                }
                await this.validateLocalSource(selected);
                const local = this.ports.qualifiedSource;
                if (!local) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
                const data = await local(selected.raw());
                const envelope = JSON.parse(data.envelope);
                messages.push(envelope);
                used += codec.bytes(data.envelope).length;
                const bytes = data.bytes;
                if (bytes) {
                    try {
                        this.original(original);
                        const encoded = codec.encode(bytes, false);
                        used += encoded.length;
                        const metadata = JSON.parse(data.objectInfo);
                        metadata['bytes'] = encoded;
                        objects.push(metadata);
                    }
                    finally {
                        bytes.fill(0);
                    }
                }
                if (used > MigrationProtocol_1.MigrationRules.limit - 32768) {
                    MigrationProtocol_1.MigrationRules.fail('MIGRATION_SOURCE_LIMIT');
                }
                continue;
            }
            const raw = await this.ports.source('/messages/' + selected.get('id').string());
            this.original(original);
            const envelope = await MigrationProtocol_1.MigrationRules.envelope(raw, selected.raw(), codec);
            messages.push(JSON.parse(envelope));
            used += codec.bytes(envelope).length;
            if (selected.get('objectMode').string() === 'full') {
                const max = Math.min(64 * 1024 * 1024, Math.floor((MigrationProtocol_1.MigrationRules.limit - used - 32768) * 3 / 4));
                if (max < 1) {
                    MigrationProtocol_1.MigrationRules.fail('MIGRATION_SOURCE_LIMIT');
                }
                const bytes = await this.ports.binary('/media/' + selected.get('attachmentId').string(), this.ports.identity.store, max);
                try {
                    this.original(original);
                    const encoded = codec.encode(bytes, false);
                    used += encoded.length;
                    const object = {};
                    object['objectId'] = selected.get('attachmentId').string();
                    object['messageId'] = selected.get('id').string();
                    object['bytes'] = encoded;
                    objects.push(object);
                }
                finally {
                    bytes.fill(0);
                }
            }
            if (used > MigrationProtocol_1.MigrationRules.limit - 32768) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_SOURCE_LIMIT');
            }
        }
        const data = {};
        data['messages'] = messages;
        data['objects'] = objects;
        return JSON.stringify(data);
    }
    async create(id, targetProof, password, otp, retry) {
        return this.locked(async () => {
            let row = (await this.rows()).find((item) => item.id === id);
            if (!row || row.kind !== 'create') {
                MigrationProtocol_1.MigrationRules.fail();
            }
            const found = await this.lookup(row);
            if (found) {
                return MigrationProtocol_1.MigrationRules.view(found, this.ports.identity, this.ports.codec, row.id);
            }
            if (!retry) {
                return undefined;
            }
            this.original(row);
            await this.capability();
            const target = this.pairing(targetProof);
            if (SyncProtocol_1.SyncJSON.parse(target).get('targetDeviceId').string() === row.identity.device) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_TARGET_MISMATCH');
            }
            let next = this.copy(row);
            next.target = target;
            row = await this.save(row, next);
            const data = await this.sourceData(row);
            const digest = await this.ports.codec.hash(this.ports.codec.bytes(data));
            this.original(row);
            if (row.sourceDigest && row.sourceDigest !== digest) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_ORIGINAL_CHANGED');
            }
            next = this.copy(row);
            next.sourceDigest = digest;
            row = await this.save(row, next);
            const body = this.proof(password, otp);
            body['requestId'] = row.id;
            body['sourceContextId'] = 'personal';
            body['sourceStoreGeneration'] = row.identity.store;
            body['restoreEpoch'] = row.identity.epoch;
            body['targetPairingProof'] = targetProof;
            const source = new SyncProtocol_1.SyncJSON(JSON.parse(data));
            body['messages'] = source.get('messages').value;
            body['objects'] = source.get('objects').value;
            const encoded = JSON.stringify(body);
            if (this.ports.codec.bytes(encoded).length > MigrationProtocol_1.MigrationRules.limit) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_SOURCE_LIMIT');
            }
            next = this.copy(row);
            next.attempts++;
            next.state = 'unknown';
            row = await this.save(row, next);
            this.original(row);
            return this.remember(row, await this.request('/sync/migrations', 'POST', encoded));
        });
    }
    async status(id) { return MigrationProtocol_1.MigrationRules.view(await this.request('/sync/migrations/' + MigrationProtocol_1.MigrationRules.id(id)), this.ports.identity, this.ports.codec); }
    async install(id, password, otp) {
        return this.locked(async () => {
            await this.capability();
            const v = await this.status(id);
            const m = SyncProtocol_1.SyncJSON.parse(v.manifest);
            const i = this.ports.identity;
            if (m.get('targetDeviceId').string() !== i.device || m.get('targetStoreGeneration').string() !== i.store) {
                MigrationProtocol_1.MigrationRules.fail('MIGRATION_TARGET_CHANGED');
            }
            if (v.state === 'installed_reported') {
                if (!await this.ports.store.installed(v)) {
                    MigrationProtocol_1.MigrationRules.fail('MIGRATION_LOCAL_COMMIT_MISSING');
                }
                return true;
            }
            let row = (await this.rows()).find((item) => item.kind === 'target' && item.migration === id && MigrationStore_1.MigrationStore.samePrincipal(item.identity, i));
            if (!row) {
                row = await this.ports.store.stage('target', '[]', this.ports.uuid(), id);
            }
            this.original(row);
            const next = this.copy(row);
            next.attempts++;
            next.state = row.state === 'installed' ? 'installed' : 'unknown';
            const info = {};
            info['id'] = v.id;
            info['manifestDigest'] = v.digest;
            info['expiresAt'] = v.expires;
            next.view = JSON.stringify(info);
            row = await this.save(row, next);
            this.ports.files.disk(v.size);
            const cached = await this.ports.files.load(v, this.ports.codec);
            const bytes = cached ?? await this.ports.binary('/sync/migrations/' + id + '/capsule', i.store, v.size);
            let history;
            try {
                this.check();
                await this.ports.files.save(v, bytes, this.ports.codec);
                const body = this.proof(password, otp);
                body['manifestDigest'] = v.digest;
                const key = await this.request('/sync/migrations/' + id + '/key', 'POST', JSON.stringify(body));
                history = await MigrationProtocol_1.MigrationRules.open(v, bytes, key, i, this.ports.codec);
                this.check();
                const proof = {};
                proof['manifestDigest'] = v.digest;
                const validation = await this.request('/sync/migrations/' + id + '/validate', 'POST', JSON.stringify(proof));
                const until = MigrationProtocol_1.MigrationRules.validateInstall(validation, v, i, this.ports.codec);
                await this.ports.store.install(v, history, until);
                await this.ports.store.discard(id);
                this.check();
                const current = (await this.rows()).find((item) => item.id === row?.id);
                if (!current) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
                const done = this.copy(current);
                done.state = 'installed';
                await this.save(current, done);
                try {
                    proof['installToken'] = SyncProtocol_1.SyncJSON.parse(validation).get('installToken').string();
                    const reported = SyncProtocol_1.SyncJSON.parse(await this.request('/sync/migrations/' + id + '/installed', 'POST', JSON.stringify(proof)));
                    if (reported.get('id').string() !== id || reported.get('manifestDigest').string() !== v.digest || reported.get('status').string() !== 'installed_reported' || reported.get('sourcePreserved').value !== true || reported.get('qualifiedAck').value !== false || reported.get('targetQualified').value !== false || reported.get('serverVerifiedLocalDurability').value !== false) {
                        MigrationProtocol_1.MigrationRules.fail();
                    }
                    return true;
                }
                catch (_) {
                    this.check();
                    return false;
                }
            }
            catch (error) {
                try {
                    await this.ports.store.discard(id);
                }
                catch (_) { }
                throw error;
            }
            finally {
                bytes.fill(0);
                if (history) {
                    for (const object of history.objects) {
                        object.bytes.fill(0);
                    }
                }
            }
        });
    }
    async cancel(id) { return this.locked(async () => { const view = await this.status(id); const originals = (await this.rows()).filter((item) => item.migration === id && item.identity.namespace === this.ports.identity.namespace); if (!originals.length) {
        MigrationProtocol_1.MigrationRules.fail('MIGRATION_ORIGINAL_SESSION_REQUIRED');
    } for (const row of originals) {
        this.original(row);
    } const body = {}; body['manifestDigest'] = view.digest; const raw = await this.request('/sync/migrations/' + id + '/cancel', 'POST', JSON.stringify(body)); const result = await MigrationProtocol_1.MigrationRules.view(raw, this.ports.identity, this.ports.codec); for (const row of (await this.rows()).filter((item) => item.migration === id)) {
        await this.remember(row, raw);
    } if (['retired', 'revoking'].includes(result.state)) {
        await this.ports.store.discard(id);
    } return result; }); }
    async objectInfo(id) { const raw = (await this.history()).find((item) => SyncProtocol_1.SyncJSON.parse(item).get('id').string() === id); if (!raw || SyncProtocol_1.SyncJSON.parse(raw).get('forwarded').value === true || SyncProtocol_1.SyncJSON.parse(raw).get('contentUnavailable').value === true) {
        MigrationProtocol_1.MigrationRules.fail();
    } return this.ports.store.objectInfo(id); }
    async history() {
        const out = [];
        const initial = await this.ports.store.history();
        const observed = new Map();
        for (const raw of initial) {
            observed.set(SyncProtocol_1.SyncJSON.parse(raw).get('id').string(), SyncProtocol_1.SyncJSON.parse(raw).canonical());
        }
        for (const raw of initial) {
            const message = SyncProtocol_1.SyncJSON.parse(raw);
            if (message.get('forwarded').value !== true) {
                out.push(raw);
                continue;
            }
            try {
                const live = SyncProtocol_1.SyncJSON.parse(await this.ports.source('/messages/' + message.get('id').string()));
                this.check();
                const valid = live.get('deleted').value === false && live.get('forwarded').value === true && live.get('contentUnavailable').value !== true && live.get('forwardUnavailable').value !== true && ['id', 'conversationId', 'senderId', 'type', 'text', 'replyToId', 'contentVersion', 'creationEpoch', 'creationSequence', 'createdAt'].every((key) => live.get(key).value === message.get(key).value);
                if (valid) {
                    out.push(raw);
                }
                else {
                    await this.ports.store.retireMessage(message.get('id').string());
                }
            }
            catch (error) {
                this.check();
                if (error instanceof Models_1.ApiFailure && [403, 404].includes(error.status) && ['MESSAGE_NOT_FOUND', 'NOT_FOUND', 'HISTORY_NOT_ALLOWED', 'NOT_A_MEMBER', 'FORWARD_UNAVAILABLE', 'RETENTION_CLOUD_EXPIRED'].includes(error.code)) {
                    await this.ports.store.retireMessage(message.get('id').string());
                }
                else {
                    const hidden = JSON.parse(raw);
                    hidden['text'] = '';
                    hidden['forwardUnavailable'] = true;
                    hidden['contentUnavailable'] = true;
                    out.push(JSON.stringify(hidden));
                }
            }
        }
        this.check();
        const fresh = new Map();
        for (const raw of await this.ports.store.history()) {
            fresh.set(SyncProtocol_1.SyncJSON.parse(raw).get('id').string(), SyncProtocol_1.SyncJSON.parse(raw).canonical());
        }
        this.check();
        return out.filter((raw) => { const id = SyncProtocol_1.SyncJSON.parse(raw).get('id').string(); return observed.has(id) && fresh.get(id) === observed.get(id); });
    }
}
exports.MigrationService = MigrationService;

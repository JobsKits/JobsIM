"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ReceiptRules = void 0;
// Created by Jobs on 2026年11月17日，星期二.
const SyncProtocol_1 = require("./SyncProtocol");
const MigrationProtocol_1 = require("./MigrationProtocol");
class ReceiptRules {
    static pair(raw) { const generation = raw.has('deliveryGeneration') ? raw.get('deliveryGeneration').string() : ''; const digest = raw.has('manifestDigest') ? raw.get('manifestDigest').string() : ''; if (generation || digest) {
        MigrationProtocol_1.MigrationRules.positive(generation);
        MigrationProtocol_1.MigrationRules.digest(digest);
    } return [generation, digest]; }
    static samePair(a, b) { return JSON.stringify(ReceiptRules.pair(a)) === JSON.stringify(ReceiptRules.pair(b)); }
    static proof(token, prefix, i, qualification, codec, permission = '') {
        const parts = token.split('.');
        if (token.length > 8192 || parts.length !== 3 || parts[0] !== prefix || MigrationProtocol_1.MigrationRules.decode(parts[2], 32, true, codec).length !== 32) {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_PROOF_INVALID');
        }
        const p = SyncProtocol_1.SyncJSON.parse(codec.text(MigrationProtocol_1.MigrationRules.decode(parts[1], 8192, true, codec)));
        MigrationProtocol_1.MigrationRules.exact(p, ['scope', 'user', 'device', 'store', 'epoch', 'qualification', 'message', 'conversation', 'contentVersion', 'bodyDigest', 'permission', 'expires', 'deliveryGeneration', 'manifestDigest', 'parentReceiptId', 'objectId', 'sizeBytes', 'objectDigest']);
        if (p.get('scope').string() !== i.deployment || p.get('user').string() !== i.user || p.get('device').string() !== i.device || p.get('store').string() !== i.store || p.get('epoch').string() !== i.epoch || p.get('qualification').string() !== qualification) {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_CONTEXT_CHANGED');
        }
        for (const key of ['message', 'conversation', 'qualification']) {
            MigrationProtocol_1.MigrationRules.id(p.get(key).string());
        }
        MigrationProtocol_1.MigrationRules.positive(p.get('contentVersion').string());
        MigrationProtocol_1.MigrationRules.digest(p.get('bodyDigest').string());
        if (!p.get('permission').string() || permission && permission !== p.get('permission').string() || p.get('expires').integer(Number.MAX_SAFE_INTEGER) === 0) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        ReceiptRules.pair(p);
        if (prefix === 'ro1') {
            MigrationProtocol_1.MigrationRules.id(p.get('objectId').string());
            const parent = p.get('parentReceiptId').string();
            if (codec.bytes(parent).length < 8 || codec.bytes(parent).length > 128 || parent.trim() !== parent || /[\r\n\u0000]/.test(parent) || Number(SyncProtocol_1.SyncRules.counter(p.get('sizeBytes').string())) > 64 * 1024 * 1024) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            MigrationProtocol_1.MigrationRules.digest(p.get('objectDigest').string());
        }
        return p;
    }
    static context(raw, i, schema) {
        const r = SyncProtocol_1.SyncJSON.parse(raw);
        if (r.get('protocol').string() !== ReceiptRules.protocol || r.get('scopeId').string() !== i.deployment || r.get('principalId').string() !== i.user || r.get('deviceId').string() !== i.device || r.get('sourceContextId').string() !== 'personal' || r.get('storeGeneration').string() !== i.store || r.get('restoreEpoch').string() !== i.epoch || r.get('schemaVersion').string() !== schema || r.get('deletionEligible').value !== false || r.get('targetRangeComplete').value !== false || r.get('defaultRetentionMode').string() !== 'cloud_history') {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_CONTEXT_CHANGED');
        }
        const q = r.get('storage');
        const content = q.get('qualified').bool();
        const objects = q.get('objectQualified').bool();
        const ready = r.get('readiness').get('ready').bool();
        if (q.get('storeGeneration').string() !== i.store || q.get('restoreEpoch').string() !== i.epoch || objects && (!content || q.get('storageContract').string() !== ReceiptRules.objectContract) || r.get('qualifiedAck').bool() !== content || r.get('contentReceiptEnabled').bool() !== content || r.get('objectReceiptEnabled').bool() !== objects || r.get('afterReceiptEnabled').bool() !== (content && ready) || r.get('afterObjectReceiptEnabled').bool() !== (objects && ready) || r.get('qualification').string() !== (objects ? 'qualified_content_and_objects' : content ? 'qualified_content' : 'transport_only')) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        if (content) {
            MigrationProtocol_1.MigrationRules.id(q.get('qualificationId').string());
            MigrationProtocol_1.MigrationRules.digest(q.get('buildDigest').string());
            MigrationProtocol_1.MigrationRules.digest(q.get('verificationDigest').string());
            if (![ReceiptRules.contentContract, ReceiptRules.objectContract].includes(q.get('storageContract').string())) {
                MigrationProtocol_1.MigrationRules.fail();
            }
        }
        return r;
    }
    static async measurement(raw, i, build, codec) {
        const r = SyncProtocol_1.SyncJSON.parse(raw);
        MigrationProtocol_1.MigrationRules.exact(r, ['protocol', 'identity', 'buildDigest', 'storageContract', 'writerFull', 'reopenBody', 'reopenIntent', 'objectFsync', 'directoryFsync', 'verificationDigest']);
        const measured = r.get('identity');
        MigrationProtocol_1.MigrationRules.exact(measured, ['profile', 'deployment', 'user', 'device', 'epoch', 'namespace', 'store']);
        if (r.get('protocol').string() !== ReceiptRules.protocol || measured.get('profile').value !== i.profile || measured.get('deployment').value !== i.deployment || measured.get('user').value !== i.user || measured.get('device').value !== i.device || measured.get('epoch').value !== i.epoch || measured.get('namespace').value !== i.namespace || measured.get('store').value !== i.store || r.get('buildDigest').string() !== build || r.get('writerFull').value !== true || r.get('reopenBody').value !== true || r.get('reopenIntent').value !== true) {
            MigrationProtocol_1.MigrationRules.fail('RECEIPT_LOCAL_VERIFICATION_REQUIRED');
        }
        const contract = r.get('storageContract').string();
        if (![ReceiptRules.contentContract, ReceiptRules.objectContract].includes(contract) || contract === ReceiptRules.objectContract && (r.get('objectFsync').value !== true || r.get('directoryFsync').value !== true)) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const values = {};
        for (const key of r.keys().filter((item) => item !== 'verificationDigest')) {
            values[key] = r.get(key).value;
        }
        if (await codec.hash(codec.bytes(new SyncProtocol_1.SyncJSON(values).canonical())) !== r.get('verificationDigest').string()) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        return r;
    }
    static validate(row, codec) {
        const r = SyncProtocol_1.SyncJSON.parse(JSON.stringify(row));
        MigrationProtocol_1.MigrationRules.exact(r, ['id', 'kind', 'identity', 'schema', 'body', 'message', 'content', 'digest', 'proof', 'descriptor', 'expires', 'state', 'attempts', 'result']);
        MigrationProtocol_1.MigrationRules.id(row.id);
        MigrationProtocol_1.MigrationRules.identity(row.identity);
        MigrationProtocol_1.MigrationRules.exact(r.get('identity'), ['profile', 'deployment', 'user', 'device', 'family', 'epoch', 'namespace', 'store', 'guard']);
        if (!['storage', 'message_content', 'object_received'].includes(row.kind) || !['1', '2'].includes(row.schema) || !['pending', 'unknown', 'recorded', 'retired'].includes(row.state) || !Number.isSafeInteger(row.attempts) || row.attempts < 0) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        if (row.state === 'retired') {
            if (row.body || row.proof || row.descriptor) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            return;
        }
        const b = SyncProtocol_1.SyncJSON.parse(row.body);
        if (b.get('requestId').string() !== row.id || b.get('storeGeneration').string() !== row.identity.store || b.get('revisionEpoch').string() !== row.identity.epoch) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        if (row.kind === 'storage') {
            MigrationProtocol_1.MigrationRules.exact(b, ['requestId', 'storeGeneration', 'revisionEpoch', 'profileId', 'buildDigest', 'verificationDigest']);
            if (!/^[A-Za-z0-9_.-]{1,128}$/.test(b.get('profileId').string())) {
                MigrationProtocol_1.MigrationRules.fail();
            }
            MigrationProtocol_1.MigrationRules.digest(b.get('buildDigest').string());
            MigrationProtocol_1.MigrationRules.digest(b.get('verificationDigest').string());
            return;
        }
        MigrationProtocol_1.MigrationRules.exact(b, row.kind === 'message_content' ? ['requestId', 'storeGeneration', 'revisionEpoch', 'qualificationId', 'kind', 'challenge', 'bodyDigest'] : ['requestId', 'storeGeneration', 'revisionEpoch', 'qualificationId', 'kind', 'parentReceiptId', 'challenge', 'objectId', 'sizeBytes', 'objectDigest']);
        MigrationProtocol_1.MigrationRules.id(row.message);
        MigrationProtocol_1.MigrationRules.positive(row.content);
        MigrationProtocol_1.MigrationRules.digest(row.digest);
        const p = ReceiptRules.proof(b.get('challenge').string(), row.kind === 'message_content' ? 'rc1' : 'ro1', row.identity, b.get('qualificationId').string(), codec);
        if (b.get('kind').string() !== row.kind || p.canonical() !== SyncProtocol_1.SyncJSON.parse(row.proof).canonical() || p.get('message').string() !== row.message || p.get('contentVersion').string() !== row.content || p.get('bodyDigest').string() !== row.digest || Date.parse(row.expires) !== p.get('expires').integer(Number.MAX_SAFE_INTEGER) * 1000) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        if (row.kind === 'message_content') {
            if (b.get('bodyDigest').string() !== row.digest) {
                MigrationProtocol_1.MigrationRules.fail();
            }
        }
        else {
            for (const key of ['parentReceiptId', 'objectId', 'sizeBytes', 'objectDigest']) {
                if (b.get(key).value !== p.get(key).value) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
            const d = SyncProtocol_1.SyncJSON.parse(row.descriptor);
            MigrationProtocol_1.MigrationRules.exact(d, ['id', 'path', 'sha256', 'sizeBytes', 'contentType']);
            if (d.get('id').value !== b.get('objectId').value || d.get('sha256').value !== b.get('objectDigest').value || d.get('sizeBytes').value !== b.get('sizeBytes').value || !/^[-0-9a-f]{36}\.bin$/.test(d.get('path').string())) {
                MigrationProtocol_1.MigrationRules.fail();
            }
        }
    }
    static result(raw, row) {
        if (row.state === 'retired') {
            MigrationProtocol_1.MigrationRules.fail();
        }
        const r = SyncProtocol_1.SyncJSON.parse(raw);
        const b = SyncProtocol_1.SyncJSON.parse(row.body);
        if (row.kind === 'storage') {
            const result = r.get('result');
            for (const key of ['storeGeneration', 'profileId', 'buildDigest', 'verificationDigest']) {
                if (result.get(key).value !== b.get(key).value) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
            MigrationProtocol_1.MigrationRules.id(result.get('qualificationId').string());
            if (result.get('restoreEpoch').string() !== row.identity.epoch || r.get('deletionEligible').value !== false || r.get('afterReceiptEnabled').value !== false || typeof r.get('current').value !== 'boolean') {
                MigrationProtocol_1.MigrationRules.fail();
            }
            return raw;
        }
        const p = SyncProtocol_1.SyncJSON.parse(row.proof);
        const fields = ['operationId', 'status', 'receiptKind', 'messageId', 'conversationId', 'contentVersion', 'restoreEpoch', 'deviceId', 'storeGeneration', 'qualificationId', 'bodyDigest'];
        const expected = [row.id, 'recorded', row.kind, row.message, p.get('conversation').string(), row.content, row.identity.epoch, row.identity.device, row.identity.store, b.get('qualificationId').string(), row.digest];
        if (fields.some((key, index) => r.get(key).value !== expected[index]) || r.get('storageQualifiedAtCommit').value !== true || r.get('deletionEligible').value !== false || r.get('targetRangeComplete').value !== false || r.get('objectComplete').value !== (row.kind === 'object_received') || !ReceiptRules.samePair(r, p)) {
            MigrationProtocol_1.MigrationRules.fail();
        }
        if (row.kind === 'object_received') {
            for (const key of ['parentReceiptId', 'objectId', 'sizeBytes', 'objectDigest']) {
                if (r.get(key).value !== b.get(key).value) {
                    MigrationProtocol_1.MigrationRules.fail();
                }
            }
        }
        return raw;
    }
    static target(raw, row) { const r = SyncProtocol_1.SyncJSON.parse(raw); if (r.get('protocol').string() !== ReceiptRules.protocol || r.get('qualificationId').value !== SyncProtocol_1.SyncJSON.parse(row.body).get('qualificationId').value || r.get('messageId').string() !== row.message || r.get('contentVersion').string() !== row.content || r.get('restoreEpoch').string() !== row.identity.epoch || !ReceiptRules.samePair(r, SyncProtocol_1.SyncJSON.parse(row.proof)) || r.get('deletionEligible').value !== false) {
        MigrationProtocol_1.MigrationRules.fail();
    } for (const key of ['originalTarget', 'contentTarget', 'objectTarget', 'contentRecorded', 'objectRecorded', 'current', 'messageCurrent']) {
        r.get(key).bool();
    } for (const key of ['contentCompleteAt', 'objectCompleteAt']) {
        if (r.get(key).value !== null && !Number.isFinite(Date.parse(r.get(key).string()))) {
            MigrationProtocol_1.MigrationRules.fail();
        }
    } return raw; }
}
exports.ReceiptRules = ReceiptRules;
ReceiptRules.protocol = 'qualified-receipts-v1';
ReceiptRules.contentContract = 'native_sqlite_full_v1';
ReceiptRules.objectContract = 'native_sqlite_full_file_fsync_v1';

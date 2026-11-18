"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.MigrationRules = void 0;
// Created by Jobs on 2026年11月17日，星期二.
const SyncProtocol_1 = require("./SyncProtocol");
const SelfProtocol_1 = require("./SelfProtocol");
class MigrationRules {
    static fail(code = 'MIGRATION_INVALID') { throw new Error(code); }
    static positive(value) { const n = SyncProtocol_1.SyncRules.counter(value); if (n === '0') {
        MigrationRules.fail();
    } return n; }
    static digest(value) { if (!/^[0-9a-f]{64}$/.test(value)) {
        MigrationRules.fail();
    } return value; }
    static exact(row, keys) { if (row.keys().some((key) => !keys.includes(key))) {
        MigrationRules.fail();
    } }
    static id(value) { if (!SyncProtocol_1.SyncRules.uuid(value)) {
        MigrationRules.fail();
    } return value; }
    static identity(i) { for (const id of [i.deployment, i.user, i.device, i.family, i.store, i.guard]) {
        MigrationRules.id(id);
    } MigrationRules.positive(i.epoch); if (!i.profile || i.profile.includes('|') || i.namespace !== ['live', i.profile, i.deployment, i.user, i.epoch, i.device, 'personal'].join('|')) {
        MigrationRules.fail();
    } }
    static decode(value, max, url, codec) {
        if (value.length > Math.ceil(max / 3) * 4) {
            MigrationRules.fail();
        }
        const bytes = codec.decode(value, max, url);
        if (bytes.length > max || codec.encode(bytes, url) !== value) {
            MigrationRules.fail();
        }
        return bytes;
    }
    static async envelope(raw, selected, codec) {
        const row = SyncProtocol_1.SyncJSON.parse(raw);
        const seen = SyncProtocol_1.SyncJSON.parse(selected);
        for (const key of ['id', 'conversationId', 'contentVersion', 'type']) {
            if (row.get(key).string() !== seen.get(key).string()) {
                MigrationRules.fail('MIGRATION_SOURCE_CHANGED');
            }
        }
        if (row.get('deleted').value !== false || row.get('contentUnavailable').value === true || row.get('forwardUnavailable').value === true || row.get('pending').value === true || row.get('frozen').value === true || Boolean(row.get('forwarded').value) !== Boolean(seen.get('forwarded').value)) {
            MigrationRules.fail('MIGRATION_SOURCE_UNAVAILABLE');
        }
        const body = {};
        for (const key of ['id', 'conversationId', 'senderId']) {
            body[key] = MigrationRules.id(row.get(key).string());
        }
        const type = row.get('type').string();
        const text = row.get('text').string();
        const mode = seen.get('objectMode').string();
        if (!['text', 'image', 'audio', 'video', 'file'].includes(type) || SelfProtocol_1.SelfRules.bytes(text) > 16384 || !['none', 'metadata_only', 'full'].includes(mode) || (type === 'text') !== (mode === 'none')) {
            MigrationRules.fail();
        }
        body['type'] = type;
        body['text'] = text;
        if (row.has('replyToId')) {
            body['replyToId'] = MigrationRules.id(row.get('replyToId').string());
        }
        if (seen.has('attachmentId')) {
            const id = MigrationRules.id(seen.get('attachmentId').string());
            if (row.get('attachmentId').string() !== id || row.get('forwarded').value === true) {
                MigrationRules.fail();
            }
            body['attachmentId'] = id;
        }
        SelfProtocol_1.SelfRules.timestamp(row.get('createdAt').string());
        body['createdAt'] = row.get('createdAt').string();
        for (const key of ['contentVersion', 'creationEpoch', 'creationSequence']) {
            body[key] = MigrationRules.positive(row.get(key).string());
        }
        if (row.get('forwarded').value === true) {
            if (mode === 'full' || seen.has('attachmentId')) {
                MigrationRules.fail();
            }
            body['forwarded'] = true;
        }
        const output = {};
        output['bodyEnvelope'] = codec.encode(codec.bytes(JSON.stringify(body)), true);
        output['objectMode'] = mode;
        return JSON.stringify(output);
    }
    static async view(raw, i, codec, request = '') {
        MigrationRules.identity(i);
        const row = SyncProtocol_1.SyncJSON.parse(raw);
        const bytes = MigrationRules.decode(row.get('manifestEnvelope').string(), 1024 * 1024, true, codec);
        const digest = MigrationRules.digest(row.get('manifestDigest').string());
        if (await codec.hash(bytes) !== digest) {
            MigrationRules.fail('MIGRATION_MANIFEST_CHANGED');
        }
        const m = SyncProtocol_1.SyncJSON.parse(codec.text(bytes));
        if (m.canonical() !== row.get('manifest').canonical()) {
            MigrationRules.fail();
        }
        MigrationRules.exact(m, ['format', 'id', 'scopeId', 'userId', 'sourceContextId', 'sourceDeviceId', 'sourceStoreGeneration', 'targetDeviceId', 'targetStoreGeneration', 'restoreEpoch', 'securityVersion', 'sourceGeneration', 'messages', 'objects', 'expiresAt']);
        if (m.get('format').string() !== MigrationRules.format || m.get('scopeId').string() !== i.deployment || m.get('userId').string() !== i.user || m.get('sourceContextId').string() !== 'personal' || m.get('restoreEpoch').string() !== i.epoch) {
            MigrationRules.fail('MIGRATION_CONTEXT_CHANGED');
        }
        for (const key of ['id', 'sourceDeviceId', 'sourceStoreGeneration', 'targetDeviceId', 'targetStoreGeneration']) {
            MigrationRules.id(m.get(key).string());
        }
        if (m.get('sourceDeviceId').string() === m.get('targetDeviceId').string()) {
            MigrationRules.fail();
        }
        MigrationRules.positive(m.get('securityVersion').string());
        MigrationRules.positive(m.get('sourceGeneration').string());
        SelfProtocol_1.SelfRules.timestamp(m.get('expiresAt').string());
        const refs = m.get('messages').list();
        const objects = m.get('objects').value === null ? [] : m.get('objects').list();
        if (!refs.length || refs.length > 256 || objects.length > 64) {
            MigrationRules.fail();
        }
        const ids = [];
        const objectIds = [];
        let full = 0;
        for (const ref of refs) {
            MigrationRules.exact(ref, ['messageId', 'conversationId', 'contentVersion', 'membershipGeneration', 'classification', 'primaryDigest', 'objectId', 'objectMode', 'deadline']);
            const id = MigrationRules.id(ref.get('messageId').string());
            if (ids.includes(id)) {
                MigrationRules.fail();
            }
            ids.push(id);
            MigrationRules.id(ref.get('conversationId').string());
            MigrationRules.positive(ref.get('contentVersion').string());
            MigrationRules.positive(ref.get('membershipGeneration').string());
            MigrationRules.digest(ref.get('primaryDigest').string());
            const mode = ref.get('objectMode').string();
            if (!['available', 'cloud_only'].includes(ref.get('classification').string()) || !['none', 'metadata_only', 'full'].includes(mode) || mode === 'none' && ref.has('objectId') || mode === 'full' && !ref.has('objectId')) {
                MigrationRules.fail();
            }
            if (ref.has('objectId')) {
                MigrationRules.id(ref.get('objectId').string());
            }
            if (mode === 'full') {
                full++;
            }
            if (ref.has('deadline')) {
                SelfProtocol_1.SelfRules.timestamp(ref.get('deadline').string());
            }
        }
        for (const ref of objects) {
            MigrationRules.exact(ref, ['objectId', 'messageId', 'sizeBytes', 'sha256', 'contentType', 'classification', 'deadline']);
            const id = MigrationRules.id(ref.get('objectId').string());
            const parent = MigrationRules.id(ref.get('messageId').string());
            if (objectIds.includes(id)) {
                MigrationRules.fail();
            }
            objectIds.push(id);
            const size = SyncProtocol_1.SyncRules.counter(ref.get('sizeBytes').string());
            MigrationRules.digest(ref.get('sha256').string());
            const mime = ref.get('contentType').string();
            if (SyncProtocol_1.SyncRules.compare(size, String(64 * 1024 * 1024)) > 0 || mime.length > 256 || /[\r\n]/.test(mime) || !['available', 'cloud_only'].includes(ref.get('classification').string()) || !refs.some((item) => item.get('messageId').string() === parent && item.get('objectId').value === id && item.get('objectMode').string() === 'full')) {
                MigrationRules.fail();
            }
            if (ref.has('deadline')) {
                SelfProtocol_1.SelfRules.timestamp(ref.get('deadline').string());
            }
        }
        const state = row.get('status').string();
        const nonce = row.get('requestId').string();
        const size = SyncProtocol_1.SyncRules.counter(row.get('sizeBytes').string());
        const capsule = row.get('capsuleSHA256').string();
        if (full !== objects.length || row.get('id').string() !== m.get('id').string() || row.get('expiresAt').string() !== m.get('expiresAt').string() || row.get('sourcePreserved').value !== true || row.get('targetQualified').value !== false || row.get('qualifiedAck').value !== false || !['building', 'ready', 'installed_reported', 'revoking', 'retired'].includes(state) || nonce.length < 8 || nonce.length > 128 || SyncProtocol_1.SyncRules.compare(size, String(MigrationRules.limit + 4096)) > 0) {
            MigrationRules.fail();
        }
        if (request && (nonce !== request || m.get('sourceDeviceId').string() !== i.device || m.get('sourceStoreGeneration').string() !== i.store)) {
            MigrationRules.fail('MIGRATION_ORIGINAL_CHANGED');
        }
        if (['ready', 'installed_reported'].includes(state) || capsule) {
            MigrationRules.digest(capsule);
        }
        return { id: m.get('id').string(), request: nonce, state: state, manifest: m.raw(), digest: digest, capsuleDigest: capsule, size: Number(size), expires: m.get('expiresAt').string() };
    }
    static aad(v, codec) { const m = SyncProtocol_1.SyncJSON.parse(v.manifest); const fields = ['JobsIM migration v1']; for (const key of ['scopeId', 'id', 'userId', 'sourceDeviceId', 'sourceStoreGeneration', 'targetDeviceId', 'targetStoreGeneration', 'restoreEpoch', 'securityVersion', 'sourceGeneration']) {
        fields.push(m.get(key).string());
    } fields.push(v.digest); return codec.bytes(fields.join('\u0000')); }
    static space(text, start) { let at = start; while (at < text.length && ' \t\r\n'.includes(text[at])) {
        at++;
    } return at; }
    static stringEnd(text, start) { if (text[start] !== '"') {
        MigrationRules.fail();
    } let at = start + 1; while (at < text.length) {
        if (text[at] === '\\') {
            at += 2;
            continue;
        }
        if (text[at] === '"') {
            return at + 1;
        }
        at++;
    } return MigrationRules.fail(); }
    static valueEnd(text, start) {
        if (text[start] === '"') {
            return MigrationRules.stringEnd(text, start);
        }
        if (text[start] === '{' || text[start] === '[') {
            let depth = 0;
            let at = start;
            while (at < text.length) {
                const ch = text[at];
                if (ch === '"') {
                    at = MigrationRules.stringEnd(text, at);
                    continue;
                }
                if (ch === '{' || ch === '[') {
                    depth++;
                }
                if (ch === '}' || ch === ']') {
                    depth--;
                    if (depth === 0) {
                        return at + 1;
                    }
                }
                at++;
            }
            return MigrationRules.fail();
        }
        let at = start;
        while (at < text.length && !',]} \t\r\n'.includes(text[at])) {
            at++;
        }
        if (at === start) {
            MigrationRules.fail();
        }
        return at;
    }
    static rawMessages(text) {
        let at = MigrationRules.space(text, 0);
        if (text[at] !== '{') {
            MigrationRules.fail();
        }
        at++;
        const keys = [];
        const result = [];
        while (at < text.length) {
            at = MigrationRules.space(text, at);
            if (text[at] === '}') {
                break;
            }
            const end = MigrationRules.stringEnd(text, at);
            const key = text.slice(at, end);
            if (!['"format"', '"manifestDigest"', '"messages"', '"objects"'].includes(key) || keys.includes(key)) {
                MigrationRules.fail();
            }
            keys.push(key);
            at = MigrationRules.space(text, end);
            if (text[at] !== ':') {
                MigrationRules.fail();
            }
            const value = MigrationRules.space(text, at + 1);
            const finish = MigrationRules.valueEnd(text, value);
            if (key === '"messages"') {
                at = value;
                if (text[at] !== '[') {
                    MigrationRules.fail();
                }
                at = MigrationRules.space(text, at + 1);
                while (text[at] !== ']') {
                    if (text[at] !== '{') {
                        MigrationRules.fail();
                    }
                    const stop = MigrationRules.valueEnd(text, at);
                    result.push(text.slice(at, stop));
                    at = MigrationRules.space(text, stop);
                    if (text[at] === ']') {
                        break;
                    }
                    if (text[at] !== ',') {
                        MigrationRules.fail();
                    }
                    at = MigrationRules.space(text, at + 1);
                }
            }
            at = MigrationRules.space(text, finish);
            if (text[at] === '}') {
                break;
            }
            if (text[at] !== ',') {
                MigrationRules.fail();
            }
            at++;
        }
        if (keys.length !== 4 || text[at] !== '}' || MigrationRules.space(text, at + 1) !== text.length) {
            MigrationRules.fail();
        }
        return result;
    }
    static async open(v, capsule, rawKey, i, codec) {
        const m = SyncProtocol_1.SyncJSON.parse(v.manifest);
        if (m.get('targetDeviceId').string() !== i.device || m.get('targetStoreGeneration').string() !== i.store || !['ready', 'installed_reported'].includes(v.state) || Date.parse(v.expires) <= Date.now()) {
            MigrationRules.fail('MIGRATION_TARGET_CHANGED');
        }
        if (capsule.length !== v.size || capsule.length < 36 || await codec.hash(capsule) !== v.capsuleDigest || codec.text(capsule.slice(0, 8)) !== 'JIMMIGR1') {
            MigrationRules.fail('MIGRATION_CAPSULE_INVALID');
        }
        const k = SyncProtocol_1.SyncJSON.parse(rawKey);
        if (k.get('id').string() !== v.id || k.get('manifestDigest').string() !== v.digest || k.get('algorithm').string() !== 'AES-256-GCM' || k.get('targetQualified').value !== false || k.get('expiresAt').string() !== v.expires) {
            MigrationRules.fail();
        }
        let key = new Uint8Array(0);
        let plain;
        try {
            key = MigrationRules.decode(k.get('key').string(), 32, true, codec);
            const aad = MigrationRules.decode(k.get('aad').string(), 8192, true, codec);
            if (key.length !== 32 || codec.encode(aad, true) !== codec.encode(MigrationRules.aad(v, codec), true)) {
                MigrationRules.fail();
            }
            plain = await codec.decrypt(key, capsule.slice(8, 20), aad, capsule.slice(20));
        }
        finally {
            key.fill(0);
        }
        const objects = [];
        try {
            if (plain.length > MigrationRules.limit) {
                MigrationRules.fail();
            }
            const text = codec.text(plain);
            const data = new SyncProtocol_1.SyncJSON(JSON.parse(text));
            const originalMessages = MigrationRules.rawMessages(text);
            MigrationRules.exact(data, ['format', 'manifestDigest', 'messages', 'objects']);
            const rows = data.get('messages').list();
            const refs = m.get('messages').list();
            const files = m.get('objects').value === null ? [] : m.get('objects').list();
            const input = data.get('objects').value === null ? [] : data.get('objects').list();
            if (data.get('format').string() !== MigrationRules.format || data.get('manifestDigest').string() !== v.digest || rows.length !== refs.length || input.length !== files.length || originalMessages.length !== rows.length) {
                MigrationRules.fail();
            }
            const messages = [];
            for (let at = 0; at < rows.length; at++) {
                const row = rows[at];
                const ref = refs[at];
                MigrationRules.exact(row, ['id', 'conversationId', 'senderId', 'type', 'text', 'replyToId', 'attachmentId', 'createdAt', 'contentVersion', 'creationEpoch', 'creationSequence', 'forwarded', 'objectMode']);
                if (row.get('id').string() !== ref.get('messageId').string() || row.get('conversationId').string() !== ref.get('conversationId').string() || row.get('contentVersion').string() !== ref.get('contentVersion').string() || row.get('attachmentId').value !== ref.get('objectId').value || row.get('objectMode').string() !== ref.get('objectMode').string()) {
                    MigrationRules.fail();
                }
                MigrationRules.id(row.get('senderId').string());
                if (row.has('replyToId')) {
                    MigrationRules.id(row.get('replyToId').string());
                }
                MigrationRules.positive(row.get('creationEpoch').string());
                MigrationRules.positive(row.get('creationSequence').string());
                SelfProtocol_1.SelfRules.timestamp(row.get('createdAt').string());
                if (SelfProtocol_1.SelfRules.bytes(row.get('text').string()) > 16384 || !['text', 'image', 'audio', 'video', 'file'].includes(row.get('type').string()) || (row.get('type').string() === 'text') !== (row.get('objectMode').string() === 'none') || row.has('forwarded') && (row.get('forwarded').value !== true || row.has('attachmentId') || row.get('objectMode').string() === 'full') || await codec.hash(codec.bytes(originalMessages[at])) !== ref.get('primaryDigest').string()) {
                    MigrationRules.fail('MIGRATION_SOURCE_CHANGED');
                }
                messages.push(row.raw());
            }
            for (let at = 0; at < input.length; at++) {
                const row = input[at];
                const ref = files[at];
                MigrationRules.exact(row, ['objectId', 'messageId', 'bytes']);
                if (row.get('objectId').string() !== ref.get('objectId').string() || row.get('messageId').string() !== ref.get('messageId').string()) {
                    MigrationRules.fail();
                }
                const bytes = MigrationRules.decode(row.get('bytes').string(), 64 * 1024 * 1024, false, codec);
                if (String(bytes.length) !== ref.get('sizeBytes').string() || await codec.hash(bytes) !== ref.get('sha256').string()) {
                    bytes.fill(0);
                    MigrationRules.fail('MIGRATION_OBJECT_INVALID');
                }
                objects.push({ id: row.get('objectId').string(), message: row.get('messageId').string(), mime: ref.get('contentType').string(), bytes: bytes });
            }
            return { messages: messages, objects: objects };
        }
        catch (error) {
            for (const object of objects) {
                object.bytes.fill(0);
            }
            throw error;
        }
        finally {
            plain.fill(0);
        }
    }
    static validateInstall(raw, v, i, codec) {
        const row = SyncProtocol_1.SyncJSON.parse(raw);
        if (row.get('id').string() !== v.id || row.get('status').string() !== 'validated' || row.get('manifestDigest').string() !== v.digest || row.get('requiresAtomicLocalCommit').value !== true || row.get('targetQualified').value !== false || row.get('qualifiedAck').value !== false) {
            MigrationRules.fail();
        }
        const before = row.get('installBefore').string();
        SelfProtocol_1.SelfRules.timestamp(before);
        if (Date.parse(before) <= Date.now() || Date.parse(before) > Date.now() + 6000) {
            MigrationRules.fail();
        }
        const parts = row.get('installToken').string().split('.');
        if (parts.length !== 3 || parts[0] !== 'mi1' || MigrationRules.decode(parts[2], 32, true, codec).length !== 32) {
            MigrationRules.fail();
        }
        const p = SyncProtocol_1.SyncJSON.parse(codec.text(MigrationRules.decode(parts[1], 8192, true, codec)));
        if (p.get('id').string() !== v.id || p.get('manifestDigest').string() !== v.digest || p.get('userId').string() !== i.user || p.get('deviceId').string() !== i.device || p.get('storeGeneration').string() !== i.store || p.get('restoreEpoch').string() !== i.epoch || p.get('expires').integer(Number.MAX_SAFE_INTEGER) * 1000 !== Date.parse(before)) {
            MigrationRules.fail();
        }
        return before;
    }
}
exports.MigrationRules = MigrationRules;
MigrationRules.format = 'jobsim-personal-migration-v1';
MigrationRules.limit = 128 * 1024 * 1024;

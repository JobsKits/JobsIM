"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SyncProtocol = exports.SyncRules = exports.SyncJSON = void 0;
// Object is used only behind checked JSON accessors; counters never pass through Number.
class SyncJSON {
    constructor(value) { this.value = value; }
    static parse(text) {
        if (text.length === 0 || text.length > 16777216) {
            throw new Error('SYNC_INVALID_JSON');
        }
        return new SyncJSON(JSON.parse(text));
    }
    keys() {
        if (this.value === null || typeof this.value !== 'object' || Array.isArray(this.value)) {
            throw new Error('SYNC_INVALID_OBJECT');
        }
        return Object.keys(this.value);
    }
    has(key) { return this.keys().includes(key); }
    get(key) {
        if (!this.has(key)) {
            return new SyncJSON(null);
        }
        return new SyncJSON(this.value[key]);
    }
    string() { if (typeof this.value !== 'string') {
        throw new Error('SYNC_INVALID_STRING');
    } return this.value; }
    bool() { if (typeof this.value !== 'boolean') {
        throw new Error('SYNC_INVALID_BOOLEAN');
    } return this.value; }
    integer(max) {
        if (typeof this.value !== 'number' || !Number.isSafeInteger(this.value) || this.value < 0 || this.value > max) {
            throw new Error('SYNC_INVALID_INTEGER');
        }
        return this.value;
    }
    list() {
        if (!Array.isArray(this.value)) {
            throw new Error('SYNC_INVALID_ARRAY');
        }
        return this.value.map((value) => new SyncJSON(value));
    }
    raw() { return JSON.stringify(this.value); }
    canonical(depth = 0) {
        if (depth > 64) {
            throw new Error('SYNC_JSON_DEPTH');
        }
        if (this.value === null || typeof this.value !== 'object') {
            return this.raw();
        }
        if (Array.isArray(this.value)) {
            return '[' + this.list().map((value) => value.canonical(depth + 1)).join(',') + ']';
        }
        return '{' + this.keys().sort().map((key) => JSON.stringify(key) + ':' + this.get(key).canonical(depth + 1)).join(',') + '}';
    }
}
exports.SyncJSON = SyncJSON;
class SyncRules {
    static uuid(value) { return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value); }
    static counter(value) {
        if (!/^(0|[1-9][0-9]{0,18})$/.test(value) || (value.length === 19 && value > '9223372036854775807')) {
            throw new Error('SYNC_INVALID_COUNTER');
        }
        return value;
    }
    static compare(a, b) { SyncRules.counter(a); SyncRules.counter(b); return a.length === b.length ? a === b ? 0 : a < b ? -1 : 1 : a.length < b.length ? -1 : 1; }
    static next(value) {
        const chars = SyncRules.counter(value).split('');
        for (let i = chars.length - 1; i >= 0; i--) {
            if (chars[i] !== '9') {
                chars[i] = String(Number(chars[i]) + 1);
                return SyncRules.counter(chars.join(''));
            }
            chars[i] = '0';
        }
        return SyncRules.counter('1' + chars.join(''));
    }
    static same(a, b) { return SyncJSON.parse(a).canonical() === SyncJSON.parse(b).canonical(); }
    static scope(identity) { return JSON.stringify([identity.profile, identity.deployment, identity.user, identity.source]); }
    static identity(identity) {
        if (!identity.profile || !SyncRules.uuid(identity.deployment) || !SyncRules.uuid(identity.user) || !SyncRules.uuid(identity.device) || identity.source !== 'personal' || SyncRules.counter(identity.epoch) === '0' || !identity.namespace) {
            throw new Error('SYNC_IDENTITY_REQUIRED');
        }
    }
    static ref(value, epoch, schema = '1') {
        const allowed = ['owner', 'kind', 'id', 'revision', 'revisionEpoch', 'state', 'conversationId', 'subjectUserId', 'targetUserId'];
        if (value.keys().some((key) => !allowed.includes(key))) {
            throw new Error('SYNC_INVALID_REFERENCE');
        }
        const ref = JSON.parse(value.raw());
        const kinds = [['auth', 'account', 'devices'], ['directory', 'profile', 'contacts', 'relationship', 'contact_request'], ['conversation', 'conversation', 'announcement'], ['message', 'message']];
        if (schema === '2') {
            kinds[1].push('preferences');
            kinds.push(['sync', 'conversation_view', 'read_state', 'read_observation', 'draft']);
        }
        const owner = kinds.find((row) => row[0] === ref.owner);
        if (!owner || !owner.slice(1).includes(ref.kind) || !SyncRules.uuid(ref.id) || !['active', 'removed'].includes(ref.state) || ref.revisionEpoch !== epoch) {
            throw new Error('SYNC_INVALID_REFERENCE');
        }
        SyncRules.counter(ref.revision);
        SyncRules.counter(ref.revisionEpoch);
        for (const key of ['conversationId', 'subjectUserId', 'targetUserId']) {
            if (value.has(key) && !SyncRules.uuid(value.get(key).string())) {
                throw new Error('SYNC_INVALID_REFERENCE');
            }
        }
        return ref;
    }
    static message(raw, minimalGlobal = false) {
        const value = SyncJSON.parse(raw);
        const message = JSON.parse(raw);
        if (!SyncRules.uuid(value.get('id').string()) || SyncRules.counter(value.get('version').string()) === '0') {
            throw new Error('SYNC_INVALID_MESSAGE');
        }
        if (minimalGlobal) {
            if (!value.get('deleted').bool()) {
                throw new Error('SYNC_INVALID_GLOBAL');
            }
            return message;
        }
        for (const key of ['conversationId', 'senderId']) {
            if (!SyncRules.uuid(value.get(key).string())) {
                throw new Error('SYNC_INVALID_MESSAGE');
            }
        }
        const client = value.get('clientMsgId').string();
        // encodeURIComponent counts UTF-8 bytes without accepting lone surrogate replacements.
        const size = encodeURIComponent(client).replace(/%[0-9A-F]{2}|[^%]/g, 'x').length;
        if (size < 8 || size > 128 || SyncRules.counter(value.get('sequence').string()) === '0' || SyncRules.counter(value.get('contentVersion').string()) === '0') {
            throw new Error('SYNC_INVALID_MESSAGE');
        }
        value.get('deleted').bool();
        value.get('text').string();
        const kind = value.get('type').string();
        if (!['text', 'image', 'audio', 'video', 'file', 'forward'].includes(kind) || !Number.isFinite(Date.parse(value.get('createdAt').string()))) {
            throw new Error('SYNC_INVALID_MESSAGE');
        }
        if (kind === 'forward' && (!value.get('forwarded').bool())) {
            throw new Error('SYNC_INVALID_MESSAGE');
        }
        if (value.has('creationEpoch') || value.has('creationSequence')) {
            const e = value.get('creationEpoch').string();
            const s = value.get('creationSequence').string();
            if (e === 'unknown' || s === 'unknown') {
                if (e !== 'unknown' || s !== 'unknown') {
                    throw new Error('SYNC_INVALID_CREATION');
                }
            }
            else {
                SyncRules.counter(e);
                SyncRules.counter(s);
            }
        }
        return message;
    }
    static persist(message) {
        const fields = JSON.parse(JSON.stringify(message));
        fields['replyAdmission'] = null;
        fields['draftAdmission'] = null;
        const row = JSON.parse(JSON.stringify(fields));
        row.forwardAdmission = undefined;
        if (row.forwarded || row.type === 'forward') {
            row.text = '';
            row.attachmentId = undefined;
            row.attachmentURL = undefined;
            row.forwardUnavailable = true;
        }
        if (row.deleted || row.contentUnavailable) {
            row.text = '';
            row.attachmentId = undefined;
            row.attachmentURL = undefined;
            row.replyToId = undefined;
            row.reactions = [];
        }
        return row;
    }
}
exports.SyncRules = SyncRules;
class SyncProtocol {
    constructor(codec) { this.codec = codec; }
    async envelope(encoded, digest) {
        if (!/^[A-Za-z0-9_-]+$/.test(encoded) || !/^[0-9a-f]{64}$/.test(digest)) {
            throw new Error('SYNC_INVALID_ENVELOPE');
        }
        const bytes = this.codec.decode(encoded);
        if (bytes.length > 16777216 || this.codec.encode(bytes) !== encoded || await this.codec.sha256(bytes) !== digest) {
            throw new Error('SYNC_DIGEST_MISMATCH');
        }
        const text = this.codec.text(bytes);
        SyncJSON.parse(text);
        return text;
    }
    binding(value, control, stream) {
        const wanted = [['scopeId', control.identity.deployment], ['principalId', control.identity.user], ['sourceContextId', control.identity.source], ['deviceId', control.identity.device], ['storeGeneration', control.physical], ['restoreEpoch', control.identity.epoch]];
        for (const pair of wanted) {
            if (value.get(pair[0]).string() !== pair[1]) {
                throw new Error('SYNC_CONTEXT_CHANGED');
            }
        }
        if (!SyncRules.uuid(value.get('streamId').string()) || (stream && value.get('streamId').string() !== control.stream)) {
            throw new Error('SYNC_CONTEXT_CHANGED');
        }
    }
    manifest(raw, control) {
        const value = SyncJSON.parse(raw);
        this.binding(value, control, true);
        const schema = control.schema ?? '1';
        if (value.get('schemaVersion').string() !== schema || !SyncRules.uuid(value.get('snapshotId').string())) {
            throw new Error('SYNC_INVALID_MANIFEST');
        }
        const descriptors = value.get('pageManifest').list().map((page, index) => {
            const result = { page: page.get('page').integer(200000), count: page.get('count').integer(100), digest: page.get('digest').string() };
            if (result.page !== index || !/^[0-9a-f]{64}$/.test(result.digest)) {
                throw new Error('SYNC_INVALID_MANIFEST');
            }
            return result;
        });
        if (descriptors.reduce((sum, row) => sum + row.count, 0) > 200000) {
            throw new Error('SYNC_CAPACITY_EXCEEDED');
        }
        for (const owner of ['auth', 'directory', 'conversation', 'message']) {
            SyncRules.counter(value.get('ownerWatermarks').get(owner).string());
        }
        if (schema === '2') {
            SyncRules.counter(value.get('ownerWatermarks').get('sync').string());
        }
        if (value.get('ownerWatermarks').keys().sort().join(',') !== (schema === '2' ? 'auth,conversation,directory,message,sync' : 'auth,conversation,directory,message')) {
            throw new Error('SYNC_INVALID_MANIFEST');
        }
        const manifest = { raw: raw, schema: schema, id: value.get('snapshotId').string(), boundary: SyncRules.counter(value.get('boundary').string()), token: value.get('boundaryToken').string(), permission: value.get('permissionGeneration').string(), classification: SyncRules.counter(value.get('classificationWatermark').string()), expiresAt: value.get('expiresAt').string(), descriptors: descriptors };
        this.alive(manifest);
        if (!manifest.token || !/^[0-9a-f]{64}$/.test(manifest.permission) || SyncRules.compare(manifest.classification, value.get('ownerWatermarks').get('message').string()) < 0) {
            throw new Error('SYNC_INVALID_MANIFEST');
        }
        return manifest;
    }
    alive(manifest) { if (!Number.isFinite(Date.parse(manifest.expiresAt)) || Date.now() >= Date.parse(manifest.expiresAt)) {
        throw new Error('SYNC_SNAPSHOT_EXPIRED');
    } }
    async projection(value, ref, epoch) {
        if (!SyncRules.same(value.get('ref').raw(), JSON.stringify(ref))) {
            throw new Error('SYNC_REFERENCE_MISMATCH');
        }
        const state = value.get('state').string();
        if (!['available', 'global', 'unavailable'].includes(state) || !value.get('revisionMatch').bool()) {
            throw new Error('SYNC_PROJECTION_UNKNOWN');
        }
        const revision = SyncRules.counter(value.get('currentRevision').string());
        if (value.get('currentRevisionEpoch').string() !== epoch || revision !== ref.revision) {
            throw new Error('SYNC_REVISION_MISMATCH');
        }
        let object = '';
        if (value.has('object') && value.get('object').value !== null) {
            object = await this.envelope(value.get('bodyEnvelope').string(), value.get('bodyDigest').string());
            if (!SyncRules.same(object, value.get('object').raw())) {
                throw new Error('SYNC_BODY_MISMATCH');
            }
            if (ref.kind === 'message') {
                const row = SyncRules.message(object, state === 'global');
                if (row.id !== ref.id || (row.conversationId && ref.conversationId !== row.conversationId)) {
                    throw new Error('SYNC_OBJECT_ID_MISMATCH');
                }
            }
            else {
                const body = SyncJSON.parse(object);
                if (['account', 'profile', 'contact_request', 'conversation'].includes(ref.kind) && body.get('id').string() !== ref.id) {
                    throw new Error('SYNC_OBJECT_ID_MISMATCH');
                }
                if (ref.kind === 'announcement' && body.get('conversationId').string() !== ref.id) {
                    throw new Error('SYNC_OBJECT_ID_MISMATCH');
                }
            }
        }
        else if (state === 'available' || value.has('bodyEnvelope') || value.has('bodyDigest')) {
            throw new Error('SYNC_INVALID_PROJECTION');
        }
        return { ref: ref, state: state, revision: revision, epoch: epoch, object: object };
    }
    async page(raw, manifest, index, epoch) {
        this.alive(manifest);
        const value = SyncJSON.parse(raw);
        const descriptor = manifest.descriptors[index];
        if (!descriptor || value.get('snapshotId').string() !== manifest.id || value.get('page').integer(200000) !== index || value.get('pageDigest').string() !== descriptor.digest || value.get('permissionGeneration').string() !== manifest.permission || value.get('restoreEpoch').string() !== epoch || !value.get('installable').bool() || !value.get('requiresFinalValidation').bool() || !value.get('requiresAtomicInstall').bool() || value.get('qualifiedAck').bool()) {
            throw new Error('SYNC_INVALID_PAGE');
        }
        const reference = await this.envelope(value.get('referenceEnvelope').string(), descriptor.digest);
        if (!SyncRules.same(reference, value.get('refs').raw())) {
            throw new Error('SYNC_REFERENCE_MISMATCH');
        }
        const refs = SyncJSON.parse(reference).list();
        const items = value.get('items').list();
        if (refs.length !== descriptor.count || items.length !== refs.length) {
            throw new Error('SYNC_PAGE_INCOMPLETE');
        }
        const projections = [];
        for (let i = 0; i < refs.length; i++) {
            projections.push(await this.projection(items[i], SyncRules.ref(refs[i], epoch, manifest.schema), epoch));
        }
        return { index: index, stage: JSON.stringify({ index: index, digest: descriptor.digest, reference: reference, envelope: value.get('referenceEnvelope').string() }), projections: projections };
    }
    validation(raw, manifest, epoch) {
        this.alive(manifest);
        const value = SyncJSON.parse(raw);
        const watermark = SyncRules.counter(value.get('classificationWatermark').string());
        if (!value.get('canInstall').bool() || !value.get('requiresLocalAtomicInstall').bool() || value.get('qualifiedAck').bool() || value.get('boundaryToken').string() !== manifest.token || value.get('restoreEpoch').string() !== epoch || value.get('permissionGeneration').string() !== manifest.permission || SyncRules.compare(watermark, manifest.classification) < 0) {
            throw new Error('SYNC_FINAL_VALIDATION_FAILED');
        }
        return watermark;
    }
    async delta(raw, position, permission, epoch, schema = '1') {
        const value = SyncJSON.parse(raw);
        if (value.get('restoreEpoch').string() !== epoch || value.get('permissionGeneration').string() !== permission || value.get('snapshotRequired').bool() || value.get('qualifiedAck').bool()) {
            throw new Error('SYNC_PERMISSION_CHANGED');
        }
        let at = position;
        const projections = [];
        const events = [];
        const seen = new Set();
        for (const item of value.get('items').list()) {
            const kind = item.get('kind').string();
            if (kind === 'redacted') {
                const from = SyncRules.counter(item.get('from').string());
                const to = SyncRules.counter(item.get('to').string());
                if (from !== SyncRules.next(at) || SyncRules.compare(to, from) < 0 || !/^[0-9a-f]{64}$/.test(item.get('rangeDigest').string())) {
                    throw new Error('SYNC_POSITION_GAP');
                }
                at = to;
                events.push({ origin: 'range:' + from + ':' + to, fact: item.raw() });
                continue;
            }
            if (kind !== 'reference' || item.get('position').string() !== SyncRules.next(at)) {
                throw new Error('SYNC_POSITION_GAP');
            }
            const fact = await this.envelope(item.get('eventEnvelope').string(), item.get('eventDigest').string());
            const signed = SyncJSON.parse(fact);
            const origin = item.get('originEventId').string();
            const ref = SyncRules.ref(item.get('ref'), epoch, schema);
            if (!origin || seen.has(origin) || signed.get('originEventId').string() !== origin || signed.get('position').string() !== item.get('position').string() || !SyncRules.same(signed.get('ref').raw(), item.get('ref').raw())) {
                throw new Error('SYNC_ORIGIN_CONFLICT');
            }
            seen.add(origin);
            at = item.get('position').string();
            projections.push(await this.projection(item.get('projection'), ref, epoch));
            events.push({ origin: origin, fact: fact });
        }
        const end = SyncRules.counter(value.get('position').string());
        const head = SyncRules.counter(value.get('head').string());
        const more = value.get('hasMore').bool();
        if (end !== at || SyncRules.compare(head, at) < 0 || more !== (head !== at) || !value.get('cursor').string()) {
            throw new Error('SYNC_POSITION_GAP');
        }
        return { cursor: value.get('cursor').string(), position: at, head: head, more: more, projections: projections, events: events };
    }
}
exports.SyncProtocol = SyncProtocol;

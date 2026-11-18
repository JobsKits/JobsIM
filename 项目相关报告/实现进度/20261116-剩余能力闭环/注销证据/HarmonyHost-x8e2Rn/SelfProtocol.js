"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SelfRules = void 0;
// Created by Jobs on 2026年11月16日，星期一.
const SyncProtocol_1 = require("./SyncProtocol");
class SelfRules {
    static personal(ref) { return ref.kind === 'preferences' || ref.owner === 'sync'; }
    static bytes(text) { return encodeURIComponent(text).replace(/%[0-9A-F]{2}|[^%]/g, 'x').length; }
    static timestamp(text) {
        if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$/.test(text) || !Number.isFinite(Date.parse(text))) {
            throw new Error('SELF_INVALID_TIMESTAMP');
        }
    }
    static point(value) { SyncProtocol_1.SyncRules.counter(value.get('epoch').string()); SyncProtocol_1.SyncRules.counter(value.get('sequence').string()); }
    static scope(value, identity, conversation, ref) {
        if (value.get('scopeId').string() !== identity.deployment || value.get('sourceContextId').string() !== identity.source || value.get('userId').string() !== identity.user || value.get('revisionEpoch').string() !== identity.epoch || (conversation && value.get('conversationId').string() !== conversation)) {
            throw new Error('SELF_SCOPE_MISMATCH');
        }
        if (conversation && !SyncProtocol_1.SyncRules.uuid(value.get('id').string())) {
            throw new Error('SELF_INVALID_OBJECT');
        }
        if (ref && (ref.subjectUserId !== identity.user || (ref.kind === 'preferences' ? ref.id !== identity.user : value.get('id').string() !== ref.id))) {
            throw new Error('SELF_SCOPE_MISMATCH');
        }
        if (value.has('removed') && value.get('removed').bool()) {
            if (!ref || ref.state !== 'removed') {
                throw new Error('SELF_INVALID_OBJECT');
            }
            return;
        }
        // Owner revision and the object's business version are independent counters.
        SyncProtocol_1.SyncRules.counter(value.get('version').string());
    }
    static preference(raw, identity) {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        SelfRules.scope(value, identity, '');
        value.get('explicit').bool();
        if (!['small', 'standard', 'large'].includes(value.get('fontPreset').string())) {
            throw new Error('SELF_INVALID_FONT');
        }
        return JSON.parse(raw);
    }
    static view(raw, identity, conversation) {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        SelfRules.scope(value, identity, conversation);
        for (const key of ['archived', 'hidden', 'manualUnread']) {
            value.get(key).bool();
        }
        SyncProtocol_1.SyncRules.counter(value.get('manualUnreadGeneration').string());
        if (value.get('pinnedRank').value !== null) {
            value.get('pinnedRank').integer(999999);
        }
        for (const key of ['mutedUntil', 'manualUnreadMarkedAt']) {
            if (value.get(key).value !== null) {
                SelfRules.timestamp(value.get(key).string());
            }
        }
        if (value.get('manualUnreadFrom').value !== null) {
            SelfRules.point(value.get('manualUnreadFrom'));
        }
        return JSON.parse(raw);
    }
    static read(raw, identity, conversation, snapshot) {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        SelfRules.scope(value, identity, conversation);
        SyncProtocol_1.SyncRules.counter(value.get('membershipGeneration').string());
        SelfRules.point(value.get('readPoint'));
        value.get('writable').bool();
        value.get('continuityStatus').string();
        if (value.get('countStatus').string() !== (snapshot ? 'not_computed' : 'exact_current_source')) {
            throw new Error('SELF_INVALID_READ');
        }
        if (snapshot) {
            if (value.has('unreadCount') || value.has('countSourceWatermark')) {
                throw new Error('SELF_INVALID_READ');
            }
        }
        else {
            SyncProtocol_1.SyncRules.counter(value.get('unreadCount').string());
            SyncProtocol_1.SyncRules.counter(value.get('countSourceWatermark').string());
        }
        return JSON.parse(raw);
    }
    static draft(raw, identity, conversation) {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        SelfRules.scope(value, identity, conversation);
        const allowed = ['id', 'scopeId', 'sourceContextId', 'userId', 'conversationId', 'revisionEpoch', 'version', 'contentState', 'ownedText', 'replyState', 'reply', 'draftAdmission', 'writable', 'pendingOperationId', 'recovery'];
        if (value.keys().some((key) => !allowed.includes(key))) {
            throw new Error('SELF_INVALID_DRAFT');
        }
        const state = value.get('contentState').string();
        const reply = value.get('replyState').string();
        value.get('writable').bool();
        if (!['empty', 'owned', 'unavailable'].includes(state) || !['none', 'available', 'unavailable'].includes(reply) || !value.has('ownedText') || !value.has('reply')) {
            throw new Error('SELF_INVALID_DRAFT');
        }
        if (value.get('ownedText').value !== null) {
            const text = value.get('ownedText').string();
            if (SelfRules.bytes(text) > 16384 || state === 'unavailable' || state === 'empty' && text !== '') {
                throw new Error('SELF_INVALID_DRAFT');
            }
        }
        else if (state === 'owned') {
            throw new Error('SELF_INVALID_DRAFT');
        }
        if (value.get('reply').value !== null) {
            const source = value.get('reply');
            const keys = ['messageId', 'sourceVersion', 'replyAdmission'];
            if (reply !== 'available' || !SyncProtocol_1.SyncRules.uuid(source.get('messageId').string()) || SyncProtocol_1.SyncRules.counter(source.get('sourceVersion').string()) === '0' || source.keys().some((key) => !keys.includes(key))) {
                throw new Error('SELF_INVALID_REPLY');
            }
        }
        else if (reply === 'available') {
            throw new Error('SELF_INVALID_REPLY');
        }
        if (value.has('recovery') && value.get('recovery').value !== null) {
            const recovery = value.get('recovery');
            if (!SyncProtocol_1.SyncRules.uuid(recovery.get('operationId').string()) || SelfRules.bytes(recovery.get('ownedText').string()) > 16384 || recovery.keys().some((key) => !['operationId', 'ownedText'].includes(key))) {
                throw new Error('SELF_INVALID_DRAFT');
            }
        }
        return JSON.parse(raw);
    }
    static projection(projection, identity, schema) {
        if (!SelfRules.personal(projection.ref)) {
            return projection.object;
        }
        if (schema !== '2') {
            throw new Error('SYNC_SCHEMA_MISMATCH');
        }
        if (!projection.object) {
            if (projection.state === 'available') {
                throw new Error('SELF_INVALID_OBJECT');
            }
            return '';
        }
        const value = SyncProtocol_1.SyncJSON.parse(projection.object);
        SelfRules.scope(value, identity, projection.ref.conversationId ?? '', projection.ref);
        if (value.has('removed') && value.get('removed').bool()) {
            return projection.object;
        }
        if (projection.ref.kind === 'preferences') {
            SelfRules.preference(projection.object, identity);
        }
        else if (projection.ref.kind === 'conversation_view') {
            SelfRules.view(projection.object, identity, projection.ref.conversationId ?? '');
        }
        else if (projection.ref.kind === 'read_state') {
            SelfRules.read(projection.object, identity, projection.ref.conversationId ?? '', true);
        }
        else if (projection.ref.kind === 'draft') {
            const draft = SelfRules.draft(projection.object, identity, projection.ref.conversationId ?? '');
            draft.draftAdmission = undefined;
            if (draft.reply) {
                draft.reply.replyAdmission = undefined;
            }
            return JSON.stringify(draft);
        }
        else if (projection.ref.kind === 'read_observation') {
            if (!SyncProtocol_1.SyncRules.uuid(value.get('messageId').string()) || value.get('countStatus').string() !== 'not_computed') {
                throw new Error('SELF_INVALID_READ');
            }
            for (const key of ['eligibleAtObservation', 'currentlyApplicable']) {
                value.get(key).bool();
            }
            SyncProtocol_1.SyncRules.counter(value.get('membershipGeneration').string());
            SyncProtocol_1.SyncRules.counter(value.get('observedInEpoch').string());
            SelfRules.timestamp(value.get('observedAt').string());
            if (value.get('orderKnown').bool()) {
                SyncProtocol_1.SyncRules.counter(value.get('creationEpoch').string());
                SyncProtocol_1.SyncRules.counter(value.get('creationSequence').string());
            }
            else if (value.get('creationEpoch').string() !== 'unknown' || value.get('creationSequence').string() !== 'unknown') {
                throw new Error('SELF_INVALID_READ');
            }
        }
        else {
            throw new Error('SELF_INVALID_OBJECT');
        }
        return projection.object;
    }
    static operationResult(raw, operation) {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        const result = value.get('result');
        const id = value.get('operationId').string();
        const status = value.get('status').string();
        value.get('current').bool();
        if (['read', 'draft'].includes(operation.kind) ? !SyncProtocol_1.SyncRules.uuid(id) : id !== operation.id) {
            throw new Error('SELF_RESULT_INVALID');
        }
        if (result.get('revisionEpoch').string() !== operation.epoch) {
            throw new Error('RESTORE_EPOCH_CHANGED');
        }
        const version = SyncProtocol_1.SyncRules.counter(result.get('version').string());
        if (!['applied', 'prepared', 'retired'].includes(status) || (status !== 'applied' && operation.kind !== 'draft') || status === 'applied' && version === '0') {
            throw new Error('SELF_RESULT_INVALID');
        }
        if (operation.result && SyncProtocol_1.SyncJSON.parse(operation.result).get('operationId').string() !== id) {
            throw new Error('SELF_RESULT_IDENTITY_CHANGED');
        }
        const original = SyncProtocol_1.SyncJSON.parse(operation.body);
        if (operation.kind === 'font' && result.get('fontPreset').string() !== original.get('fontPreset').string()) {
            throw new Error('SELF_RESULT_INVALID');
        }
        if (operation.kind === 'view') {
            if (result.get('conversationId').string() !== operation.conversation) {
                throw new Error('SELF_RESULT_INVALID');
            }
            SyncProtocol_1.SyncRules.counter(result.get('manualUnreadGeneration').string());
        }
        if (operation.kind === 'read') {
            const observed = result.get('observedMessageIds').list().map((item) => item.string()).sort();
            const requested = original.get('observedMessageIds').list().map((item) => item.string()).sort();
            if (result.get('membershipGeneration').string() !== original.get('membershipGeneration').string() || result.get('countStatus').string() !== 'not_computed' || JSON.stringify(observed) !== JSON.stringify(requested)) {
                throw new Error('SELF_RESULT_INVALID');
            }
            SelfRules.point(result.get('readPoint'));
            result.get('hasMoreEligible').bool();
            result.get('hasUnorderedEligible').bool();
        }
        if (operation.kind === 'draft' && (!['empty', 'owned', 'unavailable'].includes(result.get('contentState').string()) || !['none', 'available', 'unavailable'].includes(result.get('replyState').string()))) {
            throw new Error('SELF_RESULT_INVALID');
        }
        return status === 'applied' ? 'completed' : status === 'retired' ? 'frozen' : 'pending';
    }
}
exports.SelfRules = SelfRules;

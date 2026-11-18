"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ApiFailure = exports.AuthRules = exports.Rules = exports.Isolation = void 0;
class Isolation {
    static namespace(profile, deployment, user, demo, epoch = '1', device = '', source = '') {
        const values = [demo ? 'demo' : 'live', profile, deployment, user, epoch, device];
        if (!demo && source.length > 0) {
            values.push(source);
        }
        return values.join('|');
    }
    static endpoint(raw) {
        const value = raw.trim().replace(/\/$/, '');
        if (!/^https:\/\/[^\s/?#@]+(?::[0-9]+)?(?:\/[^?#]*)?$/.test(value)) {
            throw new Error('HTTPS URL without credentials, query or fragment required');
        }
        return value;
    }
}
exports.Isolation = Isolation;
class Rules {
    static lookupPath(kind, query, conversation, cursor) {
        if (kind !== 'favorites' && kind !== 'search') {
            throw new Error('Invalid lookup kind');
        }
        let path = '/' + kind + '?limit=50';
        if (kind === 'search') {
            if (query.trim().length === 0) {
                throw new Error('Search query required');
            }
            path += '&q=' + encodeURIComponent(query);
            if (conversation.length > 0) {
                path += '&conversationId=' + encodeURIComponent(conversation);
            }
        }
        if (cursor.length > 0) {
            path += '&cursor=' + encodeURIComponent(cursor);
        }
        return path;
    }
    static admissionSupported(bootstrap, deployment) {
        return bootstrap.deploymentId === deployment && bootstrap.features?.send_admission === true;
    }
    static roleAdmissionSupported(bootstrap, deployment) {
        return bootstrap.deploymentId === deployment && bootstrap.features?.role_admission === true;
    }
    static permanentMentionRejection(reason) {
        return ['SEND_ADMISSION_CHANGED', 'MENTION_ALL_FORBIDDEN', 'MENTION_FORBIDDEN', 'MENTION_MEMBER_REQUIRED', 'QUALIFICATION_CHANGED'].some((code) => reason.includes(code));
    }
    static outgoingBody(row) {
        if (row.type === 'forward') {
            return JSON.stringify({ clientMsgId: row.clientMsgId, type: 'forward', forwardOfId: row.forwardOfId, forwardSourceVersion: row.forwardSourceVersion, forwardAdmission: row.forwardAdmission, restoreEpoch: row.restoreEpoch, sendAdmission: row.sendAdmission });
        }
        return JSON.stringify({ clientMsgId: row.clientMsgId, restoreEpoch: row.restoreEpoch, type: row.type, text: row.text, replyToId: row.replyToId, attachmentId: row.attachmentId, sendAdmission: row.sendAdmission, mentionedUserIds: row.mentionedUserIds, mentionAll: row.mentionAll });
    }
    static unavailableForward(row) {
        const masked = JSON.parse(JSON.stringify(row));
        masked.text = '';
        masked.attachmentId = undefined;
        masked.attachmentURL = undefined;
        masked.forwardAdmission = undefined;
        masked.forwardUnavailable = true;
        return masked;
    }
    static forwardPath(row) {
        if (row.deleted || row.contentUnavailable || row.forwardUnavailable) {
            return '';
        }
        const path = '/api/messages/' + encodeURIComponent(row.id) + '/attachment';
        return row.forwarded ? row.attachmentURL === path ? path.substring(4) : '' : row.attachmentId ? '/media/' + row.attachmentId : '';
    }
    static permanentForwardRejection(reason) {
        return ['NOT_A_MEMBER', 'HISTORY_NOT_ALLOWED', 'INVALID_FORWARD', 'FORWARD_PROTOCOL_REQUIRED', 'FORWARD_DEPTH_LIMIT', 'FORWARD_ADMISSION_CHANGED', 'FORWARD_SOURCE_CHANGED', 'FORWARD_SOURCE_UNAVAILABLE'].some((code) => reason.includes(code));
    }
    static forwardIntent(source, target, account, epoch, id, now) {
        if (source.deleted || source.contentUnavailable || source.forwardUnavailable || !source.forwardAdmission || !/^[1-9][0-9]*$/.test(source.contentVersion ?? '') || !Rules.writable(target) || !target.sendAdmission) {
            throw new Error('Current forward proof required');
        }
        return { id: id, clientMsgId: id, conversationId: target.id, senderId: account, type: 'forward', text: '', createdAt: now, version: '1', sequence: '0', deleted: false, pinned: false, favorite: false, reactions: [], state: 'pending', restoreEpoch: epoch, sendAdmission: target.sendAdmission, forwardOfId: source.id, forwardSourceVersion: source.contentVersion, forwardAdmission: source.forwardAdmission, forwarded: true };
    }
    static mentionUnread(previous, message, account, active) {
        if (active || message.mentionedMe !== true || message.senderId === account || message.deleted || message.contentUnavailable) {
            return false;
        }
        return previous === undefined ? true : previous;
    }
    static selectedMentions(ids, all, members, conversation, account) {
        if (conversation.kind !== 'group' || !Rules.writable(conversation)) {
            throw new Error('Invalid mention group');
        }
        if (all && (ids.length > 0 || conversation.ownerId !== account)) {
            throw new Error('Invalid mention all');
        }
        if (ids.length > 20 || new Set(ids).size !== ids.length || ids.some((id) => !members.some((member) => member.id === id && member.state !== 'tombstoned'))) {
            throw new Error('Current mention members required');
        }
        return ids.slice();
    }
    static sourceContentVersion(message) {
        const version = message.contentVersion ?? message.version;
        if (!/^[1-9][0-9]*$/.test(version) || message.deleted || message.contentUnavailable || message.forwardUnavailable) {
            throw new Error('Source unavailable');
        }
        return version;
    }
    static announcementIntent(conversation, source, expectedVersion, requestId, expiresAt = '') {
        if (conversation.kind !== 'group' || !Rules.writable(conversation) || !/^[1-9][0-9]*$/.test(conversation.membershipGeneration ?? '') || !/^[1-9][0-9]*$/.test(conversation.roleGeneration ?? '') || !/^(0|[1-9][0-9]*)$/.test(expectedVersion) || requestId.length < 8) {
            throw new Error('Original announcement context required');
        }
        if (source && source.conversationId !== conversation.id) {
            throw new Error('Same-group source required');
        }
        const body = { expectedVersion: expectedVersion, membershipGeneration: conversation.membershipGeneration ?? '', requestId: requestId, roleGeneration: conversation.roleGeneration };
        if (source) {
            body.messageId = source.id;
            body.sourceVersion = Rules.sourceContentVersion(source);
            if (expiresAt.length > 0) {
                if (!Number.isFinite(Date.parse(expiresAt))) {
                    throw new Error('Invalid expiry');
                }
                body.expiresAt = expiresAt;
            }
        }
        return { conversationId: conversation.id, method: source ? 'PUT' : 'DELETE', body: JSON.stringify(body), rejected: false };
    }
    static announcementRejected(reason) {
        return ['QUALIFICATION_CHANGED', 'ANNOUNCEMENT_VERSION_CONFLICT', 'MEMBERSHIP_GENERATION_CHANGED', 'ANNOUNCEMENT_SOURCE_CHANGED', 'ANNOUNCEMENT_FORBIDDEN', 'INVALID_EXPIRY', 'FEATURE_DISABLED'].some((code) => reason.includes(code));
    }
    static announcementExpiry(preset, now) {
        const seconds = preset === 'hour' ? 3600 : preset === 'day' ? 86400 : preset === 'week' ? 604800 : 0;
        if (!['never', 'hour', 'day', 'week'].includes(preset) || !Number.isFinite(now)) {
            throw new Error('Invalid expiry preset');
        }
        return seconds === 0 ? '' : new Date(now + seconds * 1000).toISOString();
    }
    static incoming(request, account) {
        return request.state === 'pending' && request.recipientId === account;
    }
    static writable(conversation) {
        return conversation.readOnly !== true && conversation.state !== 'owner_unavailable' && conversation.state !== 'tombstoned';
    }
    static cursorMatches(epoch, cursor) {
        return /^[1-9][0-9]*$/.test(epoch) && new RegExp('^' + epoch + ':[0-9]+$').test(cursor);
    }
    static sessionMatches(pointer, context) {
        return context.user.id === pointer.account && context.deviceId === pointer.device && context.restoreEpoch === pointer.epoch;
    }
    static authBody(mode, username, password, code, requestId) {
        if (mode === 'recover') {
            return JSON.stringify({ username: username, password: password, code: code, requestId: requestId });
        }
        return JSON.stringify({ username: username, password: password, requestId: requestId, deviceName: 'JobsIM HarmonyOS', otp: mode === 'login' ? code : '' });
    }
    static removalPath(conversationId, member) {
        if (!/^[1-9][0-9]*$/.test(member.membershipGeneration)) {
            throw new Error('Missing membership generation');
        }
        return '/conversations/' + encodeURIComponent(conversationId) + '/members/' + encodeURIComponent(member.id) + '?membershipGeneration=' + encodeURIComponent(member.membershipGeneration);
    }
}
exports.Rules = Rules;
class AuthRules {
    static decimal(value) { return /^[1-9][0-9]*$/.test(value); }
    static next(value) {
        if (!AuthRules.decimal(value)) {
            throw new Error('INVALID_AUTH_GENERATION');
        }
        const digits = value.split('');
        for (let index = digits.length - 1; index >= 0; index--) {
            const digit = Number(digits[index]);
            if (digit < 9) {
                digits[index] = String(digit + 1);
                return digits.join('');
            }
            digits[index] = '0';
        }
        return '1' + digits.join('');
    }
    static time(value) {
        if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/.test(value)) {
            throw new Error('INVALID_AUTH_EXPIRY');
        }
        const result = Date.parse(value.replace(/\.(\d{3})\d+Z$/, '.$1Z'));
        if (!Number.isFinite(result)) {
            throw new Error('INVALID_AUTH_EXPIRY');
        }
        return result;
    }
    static validate(pair) {
        const uuid = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
        if (!pair.profile || !uuid.test(pair.deployment) || !uuid.test(pair.userId) || !uuid.test(pair.deviceId) ||
            !uuid.test(pair.familyId) || !/^[a-z0-9_]{3,32}$/.test(pair.username) || pair.sourceContextId !== 'personal' ||
            !AuthRules.decimal(pair.epoch) || !AuthRules.decimal(pair.generation) || !pair.token || !pair.refreshToken ||
            pair.token === pair.refreshToken || pair.requestId.length < 8 || pair.requestId.length > 128) {
            throw new Error('INVALID_AUTH_PAIR');
        }
        if (AuthRules.time(pair.accessExpiresAt) > AuthRules.time(pair.idleExpiresAt) ||
            AuthRules.time(pair.idleExpiresAt) > AuthRules.time(pair.absoluteExpiresAt)) {
            throw new Error('INVALID_AUTH_PAIR');
        }
    }
    static pair(result, connection, requestId) {
        const pair = { profile: connection.profile, deployment: connection.deployment,
            userId: result.user.id, username: result.user.username, deviceId: result.deviceId, epoch: result.restoreEpoch,
            sourceContextId: result.sourceContextId ?? '', token: result.token, refreshToken: result.refreshToken ?? '',
            familyId: result.familyId ?? '', generation: result.refreshGeneration ?? '', accessExpiresAt: result.accessExpiresAt ?? '',
            idleExpiresAt: result.idleExpiresAt ?? '', absoluteExpiresAt: result.absoluteExpiresAt ?? '', requestId: requestId };
        AuthRules.validate(pair);
        if (pair.epoch !== connection.epoch) {
            throw new Error('RESTORE_EPOCH_CHANGED');
        }
        return pair;
    }
    static rotate(next, previous) {
        AuthRules.validate(next);
        if (next.profile !== previous.profile || next.deployment !== previous.deployment || next.userId !== previous.userId ||
            next.username !== previous.username || next.deviceId !== previous.deviceId || next.epoch !== previous.epoch ||
            next.sourceContextId !== previous.sourceContextId || next.familyId !== previous.familyId ||
            next.generation !== AuthRules.next(previous.generation) || next.token === previous.token || next.refreshToken === previous.refreshToken ||
            AuthRules.time(next.absoluteExpiresAt) !== AuthRules.time(previous.absoluteExpiresAt)) {
            throw new Error('INVALID_REFRESH_PAIR');
        }
    }
    static namespace(pair) {
        return Isolation.namespace(pair.profile, pair.deployment, pair.userId, false, pair.epoch, pair.deviceId, pair.sourceContextId);
    }
    static pointer(pair) {
        return { alias: 'JobsIM|' + AuthRules.namespace(pair), profile: pair.profile, deployment: pair.deployment, account: pair.userId,
            device: pair.deviceId, epoch: pair.epoch, sourceContextId: pair.sourceContextId, familyId: pair.familyId, refreshGeneration: pair.generation };
    }
    static binding(pair, user) {
        if (pair.userId !== user.id || pair.username !== user.username) {
            throw new Error('INVALID_AUTH_USER');
        }
        return { pointer: AuthRules.pointer(pair), user: user, username: pair.username, familyId: pair.familyId,
            generation: pair.generation, requestId: pair.requestId, state: 'active' };
    }
    static matches(pair, binding) {
        const p = binding.pointer;
        return pair.profile === p.profile && pair.deployment === p.deployment && pair.userId === p.account &&
            pair.deviceId === p.device && pair.epoch === p.epoch && pair.sourceContextId === p.sourceContextId &&
            pair.familyId === binding.familyId && pair.generation === binding.generation && pair.requestId === binding.requestId &&
            p.alias === AuthRules.pointer(pair).alias && binding.username === pair.username;
    }
    static encode(pair) {
        AuthRules.validate(pair);
        // One compact SECRET, never separate AT/RT items. The native adapter checks UTF-8 <1KiB.
        return JSON.stringify([pair.profile, pair.deployment, pair.userId, pair.username, pair.deviceId, pair.epoch,
            pair.sourceContextId, pair.token, pair.refreshToken, pair.familyId, pair.generation,
            pair.accessExpiresAt, pair.idleExpiresAt, pair.absoluteExpiresAt, pair.requestId]);
    }
    static decode(value) {
        let values;
        try {
            values = JSON.parse(value);
        }
        catch (_error) {
            throw new Error('INVALID_AUTH_PAIR');
        }
        if (!Array.isArray(values) || values.length !== 15 || values.some((part) => typeof part !== 'string')) {
            throw new Error('INVALID_AUTH_PAIR');
        }
        const pair = { profile: values[0], deployment: values[1], userId: values[2], username: values[3], deviceId: values[4], epoch: values[5],
            sourceContextId: values[6], token: values[7], refreshToken: values[8], familyId: values[9], generation: values[10],
            accessExpiresAt: values[11], idleExpiresAt: values[12], absoluteExpiresAt: values[13], requestId: values[14] };
        AuthRules.validate(pair);
        return pair;
    }
    static initialBody(record, password, otp) {
        const body = { username: record.username, password: password, requestId: record.requestId, deviceName: record.deviceName };
        if (record.otpRequired) {
            body.otp = otp;
        }
        if (record.previousFamilyId) {
            body.previousFamilyId = record.previousFamilyId;
        }
        if (record.previousRequestId) {
            body.previousRequestId = record.previousRequestId;
        }
        if (record.previousRequestIds) {
            body.previousRequestIds = record.previousRequestIds.slice();
        }
        return JSON.stringify(body);
    }
}
exports.AuthRules = AuthRules;
class ApiFailure extends Error {
    constructor(status, code) { super('HTTP ' + status + ' ' + code); this.status = status; this.code = code; }
}
exports.ApiFailure = ApiFailure;

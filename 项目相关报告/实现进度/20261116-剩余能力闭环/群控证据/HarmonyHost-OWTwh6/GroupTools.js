"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.GroupTools = exports.GroupRules = void 0;
// Created by Jobs on 2026年11月17日，星期二.
const Models_1 = require("./Models");
const SyncProtocol_1 = require("./SyncProtocol");
const SelfProtocol_1 = require("./SelfProtocol");
class GroupRules {
    static fail(code = 'GROUP_INVALID') { throw new Error(code); }
    static invitation(text, deployment, codec) {
        if (!text.startsWith('JobsIM-Invite-1:') || text.length > 4096) {
            GroupRules.fail();
        }
        const encoded = text.slice('JobsIM-Invite-1:'.length);
        const raw = codec.decode(encoded);
        if (codec.encode(raw) !== encoded) {
            GroupRules.fail();
        }
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        const id = value.get('inviteId').string();
        const token = value.get('token').string();
        if (value.keys().sort().join(',') !== 'conversationId,deploymentId,inviteId,token' || value.get('deploymentId').string() !== deployment ||
            !SyncProtocol_1.SyncRules.uuid(value.get('conversationId').string()) || !SyncProtocol_1.SyncRules.uuid(id) || !new RegExp('^' + id + '\\.[A-Za-z0-9_-]{43}$').test(token)) {
            GroupRules.fail();
        }
        return { deploymentId: deployment, conversationId: value.get('conversationId').string(), inviteId: id, token: token };
    }
    static validate(row) {
        if (Object.keys(row).sort().join(',') !== 'attempts,body,conversation,created,epoch,id,kind,lookup,method,namespace,path,result,state' ||
            !SyncProtocol_1.SyncRules.uuid(row.id) || !SyncProtocol_1.SyncRules.uuid(row.conversation) || !row.namespace || SyncProtocol_1.SyncRules.counter(row.epoch) === '0' || !Number.isSafeInteger(row.attempts) || row.attempts < 0 ||
            !['governance', 'invite', 'join', 'revoke', 'decide'].includes(row.kind) || !['POST', 'DELETE'].includes(row.method) || !['unsent', 'unknown', 'applied'].includes(row.state)) {
            GroupRules.fail();
        }
        const base = '/conversations/' + row.conversation + '/';
        let method = 'POST';
        let route = base + 'governance';
        let lookup = route + '/operations/' + row.id;
        if (row.kind === 'invite' || row.kind === 'join') {
            route = base + (row.kind === 'invite' ? 'invites' : 'join-requests');
            lookup = route + '/operations/' + row.id;
        }
        if (row.kind === 'revoke' || row.kind === 'decide') {
            const prefix = base + (row.kind === 'revoke' ? 'invites/' : 'join-requests/');
            if (!row.path.startsWith(prefix) || !SyncProtocol_1.SyncRules.uuid(row.path.slice(prefix.length))) {
                GroupRules.fail();
            }
            route = row.path;
            if (row.kind === 'revoke') {
                method = 'DELETE';
            }
        }
        if (row.method !== method || row.path !== route || row.lookup !== lookup) {
            GroupRules.fail();
        }
        const value = SyncProtocol_1.SyncJSON.parse(row.body);
        const allowed = ['requestId', 'version', 'membershipGeneration', 'roleGeneration', 'action', 'targetId', 'targetMembershipGeneration', 'targetRoleGeneration', 'role', 'mutedUntil', 'muteAll', 'slowSeconds', 'maxMembers', 'expiresAt', 'maxUses', 'approvalRequired', 'inviteId', 'approve'];
        if (value.get('requestId').string() !== row.id || value.keys().some((key) => !allowed.includes(key))) {
            GroupRules.fail();
        }
        SelfProtocol_1.SelfRules.timestamp(row.created);
        if (row.result && SyncProtocol_1.SyncJSON.parse(row.result).has('token')) {
            GroupRules.fail();
        }
    }
    static manager(snapshot) { const value = SyncProtocol_1.SyncJSON.parse(snapshot); return ['owner', 'admin'].includes(value.get('role').string()) && !value.get('readOnly').bool(); }
}
exports.GroupRules = GroupRules;
class GroupTools {
    constructor(ports) {
        this.snapshot = '';
        this.conversation = '';
        this.tokens = new Map();
        this.ports = ports;
    }
    check() { if (!this.ports.current()) {
        GroupRules.fail('GROUP_CONTEXT_CHANGED');
    } }
    clear() { this.tokens.clear(); this.snapshot = ''; }
    async list() {
        const rows = await this.ports.repository.list();
        this.check();
        const e = this.ports.environment;
        const prefix = 'live|' + e.profile + '|' + e.deployment + '|' + e.user + '|';
        rows.forEach((row) => { GroupRules.validate(row); if (!row.namespace.startsWith(prefix)) {
            GroupRules.fail();
        } });
        return rows;
    }
    async load(conversation) {
        if (!SyncProtocol_1.SyncRules.uuid(conversation)) {
            GroupRules.fail();
        }
        this.check();
        const raw = await this.ports.request('/conversations/' + conversation + '/governance', 'GET', '');
        this.check();
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        for (const key of ['version', 'membershipGeneration', 'roleGeneration']) {
            if (SyncProtocol_1.SyncRules.counter(value.get(key).string()) === '0') {
                GroupRules.fail();
            }
        }
        value.get('readOnly').bool();
        value.get('role').string();
        this.conversation = conversation;
        this.snapshot = raw;
        return raw;
    }
    async page(conversation, kind, after = '') {
        if (!SyncProtocol_1.SyncRules.uuid(conversation) || !['members', 'invites', 'join-requests'].includes(kind) || after && !SyncProtocol_1.SyncRules.uuid(after)) {
            GroupRules.fail();
        }
        const raw = await this.ports.request('/conversations/' + conversation + '/' + kind + '?limit=50' + (after ? '&after=' + after : ''), 'GET', '');
        this.check();
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        const next = value.get('nextCursor').string();
        const items = value.get('items').list();
        if (items.length > 50 || next && (!SyncProtocol_1.SyncRules.uuid(next) || next === after)) {
            GroupRules.fail();
        }
        if (kind === 'invites' && !after) {
            this.tokens.clear();
        }
        const output = [];
        for (const item of items) {
            const id = item.get('id').string();
            if (!SyncProtocol_1.SyncRules.uuid(id)) {
                GroupRules.fail();
            }
            if (kind === 'members') {
                if (SyncProtocol_1.SyncRules.counter(item.get('membershipGeneration').string()) === '0' || SyncProtocol_1.SyncRules.counter(item.get('roleGeneration').string()) === '0') {
                    GroupRules.fail();
                }
            }
            if (kind === 'invites') {
                this.tokens.set(id, item.get('token').string());
                const safe = {};
                for (const key of ['id', 'expiresAt', 'maxUses', 'uses', 'approvalRequired', 'revokedAt']) {
                    if (item.has(key)) {
                        safe[key] = item.get(key).value;
                    }
                }
                output.push(JSON.stringify(safe));
            }
            else {
                output.push(item.raw());
            }
        }
        return { items: output, next: next };
    }
    prepare(action, target, fields = '{}') {
        this.check();
        if (!this.snapshot) {
            GroupRules.fail('GROUP_SNAPSHOT_REQUIRED');
        }
        const value = SyncProtocol_1.SyncJSON.parse(this.snapshot);
        const id = this.ports.uuid();
        const e = this.ports.environment;
        if (action === 'recover' ? value.get('canRecover').value !== true : !GroupRules.manager(this.snapshot)) {
            GroupRules.fail();
        }
        if (['settings', 'role', 'transfer', 'dissolve'].includes(action) && value.get('role').string() !== 'owner' || ['role', 'mute', 'transfer', 'recover'].includes(action) && !target || ['role', 'mute', 'transfer'].includes(action) && target?.role === 'owner') {
            GroupRules.fail();
        }
        const body = {};
        body['requestId'] = id;
        body['version'] = value.get('version').string();
        body['membershipGeneration'] = value.get('membershipGeneration').string();
        body['roleGeneration'] = value.get('roleGeneration').string();
        let kind = 'governance';
        let method = 'POST';
        let route = '/conversations/' + this.conversation + '/governance';
        let lookup = route + '/operations/' + id;
        const extra = SyncProtocol_1.SyncJSON.parse(fields);
        if (action === 'invite') {
            kind = 'invite';
            route = '/conversations/' + this.conversation + '/invites';
            lookup = route + '/operations/' + id;
        }
        else if (action === 'revoke' || action === 'decide') {
            const targetId = extra.get('id').string();
            if (!SyncProtocol_1.SyncRules.uuid(targetId)) {
                GroupRules.fail();
            }
            kind = action;
            route = '/conversations/' + this.conversation + (action === 'revoke' ? '/invites/' : '/join-requests/') + targetId;
            method = action === 'revoke' ? 'DELETE' : 'POST';
        }
        else {
            if (!['settings', 'role', 'mute', 'transfer', 'dissolve', 'recover'].includes(action)) {
                GroupRules.fail();
            }
            body['action'] = action;
        }
        for (const key of extra.keys()) {
            if (key !== 'id') {
                body[key] = extra.get(key).value;
            }
        }
        if (target) {
            if (!SyncProtocol_1.SyncRules.uuid(target.id) || SyncProtocol_1.SyncRules.counter(target.membershipGeneration) === '0' || SyncProtocol_1.SyncRules.counter(target.roleGeneration ?? '') === '0') {
                GroupRules.fail();
            }
            body['targetId'] = target.id;
            body['targetMembershipGeneration'] = target.membershipGeneration;
            body['targetRoleGeneration'] = target.roleGeneration ?? '';
        }
        const row = { id: id, conversation: this.conversation, namespace: e.namespace, epoch: e.epoch, kind: kind, method: method, path: route, lookup: lookup, body: JSON.stringify(body), created: new Date().toISOString(), attempts: 0, state: 'unsent', result: '' };
        GroupRules.validate(row);
        return row;
    }
    joining(code) {
        const e = this.ports.environment;
        const invite = GroupRules.invitation(code.trim(), e.deployment, this.ports.codec);
        const id = this.ports.uuid();
        const route = '/conversations/' + invite.conversationId + '/join-requests';
        const body = {};
        body['requestId'] = id;
        body['inviteId'] = invite.inviteId;
        const row = { id: id, conversation: invite.conversationId, namespace: e.namespace, epoch: e.epoch, kind: 'join', method: 'POST', path: route, lookup: route + '/operations/' + id, body: JSON.stringify(body), created: new Date().toISOString(), attempts: 0, state: 'unsent', result: '' };
        GroupRules.validate(row);
        return row;
    }
    async perform(candidate, retry, code = '', otp = '') {
        this.check();
        GroupRules.validate(candidate);
        const originals = await this.list();
        let row = originals.find((item) => item.id === candidate.id);
        this.check();
        if (!row && candidate.kind === 'join') {
            const invite = SyncProtocol_1.SyncJSON.parse(candidate.body).get('inviteId').string();
            row = originals.find((item) => item.kind === 'join' && item.conversation === candidate.conversation && SyncProtocol_1.SyncJSON.parse(item.body).get('inviteId').string() === invite && (item.state !== 'applied' || Boolean(item.result) && SyncProtocol_1.SyncJSON.parse(item.result).get('state').string() === 'pending'));
        }
        if (!row) {
            row = candidate;
            await this.ports.repository.insert(row);
            this.check();
        }
        let result;
        try {
            result = SyncProtocol_1.SyncJSON.parse(await this.ports.request(row.lookup, 'GET', ''));
        }
        catch (error) {
            this.check();
            if (!(error instanceof Models_1.ApiFailure) || error.status !== 404 || error.code !== 'GROUP_OPERATION_NOT_FOUND') {
                throw error;
            }
        }
        this.check();
        if (!result && !retry) {
            return undefined;
        }
        if (!result) {
            if (row.epoch !== this.ports.environment.epoch) {
                GroupRules.fail('GROUP_OLD_EPOCH_READ_ONLY');
            }
            const parsed = SyncProtocol_1.SyncJSON.parse(row.body);
            const body = {};
            for (const key of parsed.keys()) {
                body[key] = parsed.get(key).value;
            }
            if (row.kind === 'join') {
                const invite = GroupRules.invitation(code.trim(), this.ports.environment.deployment, this.ports.codec);
                if (invite.conversationId !== row.conversation || invite.inviteId !== parsed.get('inviteId').string()) {
                    GroupRules.fail();
                }
                body['token'] = invite.token;
            }
            if (parsed.has('action') && parsed.get('action').string() === 'recover') {
                if (!/^[0-9]{6}$/.test(otp)) {
                    GroupRules.fail();
                }
                const step = {};
                step['requestId'] = row.id;
                step['action'] = 'group.recover';
                step['targetId'] = row.conversation;
                step['payload'] = parsed.value;
                step['otp'] = otp;
                const proof = SyncProtocol_1.SyncJSON.parse(await this.ports.request('/auth/step-up', 'POST', JSON.stringify(step)));
                this.check();
                const authorization = proof.get('authorizationId').string();
                if (!SyncProtocol_1.SyncRules.uuid(authorization)) {
                    GroupRules.fail();
                }
                body['stepUpId'] = authorization;
            }
            const before = row;
            row = JSON.parse(JSON.stringify(row));
            row.attempts++;
            row.state = 'unknown';
            await this.ports.repository.replace(before, row);
            this.check();
            result = SyncProtocol_1.SyncJSON.parse(await this.ports.request(row.path, row.method, JSON.stringify(body)));
            this.check();
        }
        if (result.get('requestId').string() !== row.id) {
            GroupRules.fail();
        }
        for (const key of ['version', 'membershipGeneration']) {
            if (result.has(key) && result.get(key).value !== null && SyncProtocol_1.SyncRules.counter(result.get(key).string()) === '0') {
                GroupRules.fail();
            }
        }
        if (row.kind === 'governance' && (result.get('status').string() !== 'applied' || result.get('action').string() !== SyncProtocol_1.SyncJSON.parse(row.body).get('action').string())) {
            GroupRules.fail();
        }
        if (row.kind === 'join') {
            const state = result.get('state').string();
            if (!['pending', 'approved', 'rejected'].includes(state) || !SyncProtocol_1.SyncRules.uuid(result.get('id').string()) || state === 'approved' && SyncProtocol_1.SyncRules.counter(result.get('membershipGeneration').string()) === '0') {
                GroupRules.fail();
            }
        }
        if (result.has('token')) {
            const id = result.get('id').string();
            const token = result.get('token').string();
            if (row.kind !== 'invite' || !SyncProtocol_1.SyncRules.uuid(id) || !new RegExp('^' + id + '\\.[A-Za-z0-9_-]{43}$').test(token)) {
                GroupRules.fail();
            }
            this.tokens.set(id, token);
        }
        const safe = {};
        for (const key of ['requestId', 'status', 'action', 'version', 'id', 'expiresAt', 'maxUses', 'uses', 'approvalRequired', 'revokedAt', 'state', 'membershipGeneration', 'targetId']) {
            if (result.has(key)) {
                safe[key] = result.get(key).value;
            }
        }
        const after = JSON.parse(JSON.stringify(row));
        after.state = 'applied';
        after.result = JSON.stringify(safe);
        await this.ports.repository.replace(row, after);
        this.check();
        return after.result;
    }
    share(id) {
        const token = this.tokens.get(id);
        if (!token) {
            GroupRules.fail('GROUP_INVITE_RELOAD_REQUIRED');
        }
        const value = { deploymentId: this.ports.environment.deployment, conversationId: this.conversation, inviteId: id, token: token };
        return 'JobsIM-Invite-1:' + this.ports.codec.encode(JSON.stringify(value));
    }
}
exports.GroupTools = GroupTools;

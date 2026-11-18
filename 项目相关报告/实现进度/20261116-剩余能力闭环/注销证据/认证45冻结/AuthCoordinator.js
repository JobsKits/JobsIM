"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.AuthCoordinator = void 0;
// Created by Jobs on 2026年11月15日，星期日.
// Platform-neutral coordinator; native adapters own RDB, RCP and one AssetStore SECRET.
const Models_1 = require("./Models");
class AuthCoordinator {
    constructor(connection, ports) {
        this.visible = false;
        this.closing = false;
        this.proofs = [];
        this.revision = 0;
        this.passwordFamily = '';
        this.deletionFamily = '';
        this.connection = connection;
        this.ports = ports;
    }
    scope() { return Models_1.Isolation.namespace(this.connection.profile, this.connection.deployment, '__auth__', false, this.connection.epoch, 'metadata', 'personal'); }
    protocolScope() { return Models_1.Isolation.namespace(this.connection.profile, this.connection.deployment, '__auth__', false, 'protocol', 'metadata', 'personal'); }
    stamp() {
        return (this.ports.current() ? '' : 'inactive|') + this.scope() + '|' + String(this.revision) + '|' + (this.pair ? Models_1.AuthRules.namespace(this.pair) + '|' + this.pair.familyId : 'signed-out') + (this.pair?.familyId === this.passwordFamily ? '|password-paused' : '') + (this.pair?.familyId === this.deletionFamily ? '|deletion-paused' : '');
    }
    currentFamily() { return this.pair?.familyId ?? ''; }
    passwordOwner() { return this.ports.current() ? this.pair : undefined; }
    passwordPause(family, paused) { if (this.pair?.familyId === family) {
        this.passwordFamily = paused ? family : '';
    } }
    deletionPause(family, paused) { if (this.pair?.familyId === family) {
        this.deletionFamily = paused ? family : '';
    } }
    async locked(action) {
        const work = AuthCoordinator.mutationTail.then(action);
        AuthCoordinator.mutationTail = work.catch((_error) => { });
        await work;
    }
    async getRecord(id) {
        const value = await this.ports.read(this.scope(), 'request:' + id);
        return value ? JSON.parse(value) : undefined;
    }
    async pending(username) {
        const id = await this.ports.read(this.scope(), 'pending:' + username.toLowerCase());
        if (!id) {
            return undefined;
        }
        const record = await this.getRecord(id);
        return record && !['completed', 'failed', 'recovered'].includes(record.state) ? record : undefined;
    }
    entry(key, value) { return { key: key, value: value }; }
    async verify() {
        if (!this.ports.current()) {
            throw new Error('SESSION_SCOPE_CHANGED');
        }
        const stamp = this.stamp();
        const result = await this.ports.bootstrap();
        if (!this.ports.current() || stamp !== this.stamp()) {
            throw new Error('SESSION_SCOPE_CHANGED');
        }
        if (result.deploymentId !== this.connection.deployment || result.protocolVersion !== '1') {
            throw new Error('DEPLOYMENT_CHANGED');
        }
        if (result.features?.auth_refresh !== true) {
            throw new Error('AUTH_REFRESH_UNSUPPORTED');
        }
        await this.ports.write(this.protocolScope(), [this.entry('auth-refresh-seen', 'true')]);
        if (result.restoreEpoch !== this.connection.epoch) {
            await this.locked(async () => {
                if (stamp !== this.stamp()) {
                    return;
                }
                if (this.pair) {
                    await this.ports.freeze(Models_1.AuthRules.namespace(this.pair), 'restore_epoch_changed');
                }
                await this.ports.write(this.scope(), [this.entry('restore-blocked', 'true')]);
                this.pair = undefined;
                this.user = undefined;
                this.visible = false;
                this.revision++;
                this.ports.publish(undefined, undefined);
            });
            throw new Error('RESTORE_EPOCH_CHANGED');
        }
        this.ports.features(result);
        return result;
    }
    async block(stamp, token = '') {
        await this.locked(async () => {
            if (!this.ports.current() || stamp !== this.stamp() || (token.length > 0 && this.pair?.token !== token)) {
                return;
            }
            this.pair = undefined;
            this.user = undefined;
            this.visible = false;
            this.revision++;
            this.ports.publish(undefined, undefined);
        });
    }
    async before(stamp) {
        if (!this.ports.current() || stamp !== this.stamp() || this.closing) {
            throw new Error('SESSION_SCOPE_CHANGED');
        }
        if (this.pair && (await this.ports.read(this.scope(), 'password-change:' + this.pair.familyId) || await this.ports.read(this.scope(), 'account-deletion:' + this.pair.familyId))) {
            throw new Error('PASSWORD_CHANGE_RESULT_UNKNOWN');
        }
        await this.verify();
        if (this.refreshTask) {
            await this.refreshTask;
        }
        if (stamp !== this.stamp() || !this.pair) {
            throw new Error('AUTH_REAUTH_REQUIRED');
        }
        const pair = this.pair;
        if (await this.ports.read(this.scope(), 'password-change:' + pair.familyId) || await this.ports.read(this.scope(), 'account-deletion:' + pair.familyId)) {
            throw new Error('PASSWORD_CHANGE_RESULT_UNKNOWN');
        }
        const refresh = await this.ports.read(this.scope(), 'refresh:' + pair.familyId);
        const initial = await this.pending(pair.username);
        if ((refresh && JSON.parse(refresh).state !== 'completed') || initial ||
            await this.ports.read(this.scope(), 'logout:' + pair.familyId)) {
            await this.block(stamp);
            throw new Error('AUTH_REAUTH_REQUIRED');
        }
        return pair.token;
    }
    hooks() {
        return { stamp: () => this.ports.current() ? this.stamp() : this.stamp() + '|superseded', before: (stamp) => this.before(stamp),
            refresh: (token, stamp) => this.refresh(token, stamp),
            rejected: (token, stamp) => this.rejected(token, stamp) };
    }
    async rejected(token, stamp) {
        if (stamp !== this.stamp() || this.pair?.token !== token) {
            return;
        }
        const pair = this.pair;
        if (!pair) {
            return;
        }
        await this.ports.write(this.scope(), [this.entry('refresh:' + pair.familyId, JSON.stringify({ familyId: pair.familyId, generation: pair.generation, requestId: pair.requestId, state: 'reauth_required', username: pair.username }))]);
        await this.block(stamp, token);
    }
    async bind(pair, user, record, refresh, expose) {
        const binding = Models_1.AuthRules.binding(pair, user);
        const previousText = await this.ports.read(this.scope(), 'binding:' + pair.username);
        const previous = previousText ? JSON.parse(previousText) : undefined;
        const oldNamespaces = [];
        if (record) {
            oldNamespaces.push(Models_1.Isolation.namespace(pair.profile, pair.deployment, pair.userId, false, pair.epoch, pair.deviceId));
            if (previous) {
                oldNamespaces.push(Models_1.Isolation.namespace(previous.pointer.profile, previous.pointer.deployment, previous.pointer.account, false, previous.pointer.epoch, previous.pointer.device, previous.pointer.sourceContextId ?? ''));
            }
            const legacy = await this.ports.read(this.scope(), 'legacy-pointer');
            if (legacy) {
                const pointer = JSON.parse(legacy);
                if (pointer.account === pair.userId) {
                    oldNamespaces.push(Models_1.Isolation.namespace(pointer.profile, pointer.deployment, pointer.account, false, pointer.epoch, pointer.device));
                }
            }
            record.result = binding;
            record.state = 'securing';
            await this.ports.write(this.scope(), [this.entry('request:' + record.requestId, JSON.stringify(record))]);
        }
        await this.ports.save(binding.pointer.alias, Models_1.AuthRules.encode(pair));
        if (record) {
            record.state = 'installing';
            await this.ports.write(this.scope(), [this.entry('request:' + record.requestId, JSON.stringify(record))]);
        }
        if (refresh) {
            refresh.state = 'installing';
            await this.ports.write(this.scope(), [this.entry('refresh:' + refresh.familyId, JSON.stringify(refresh))]);
        }
        const entries = [this.entry('active', JSON.stringify(binding)), this.entry('binding:' + pair.username, JSON.stringify(binding))];
        if (record) {
            record.state = 'completed';
            entries.push(this.entry('request:' + record.requestId, JSON.stringify(record)));
            if (record.previousFamilyId) {
                entries.push(this.entry('logout:' + record.previousFamilyId, ''));
                const oldRefresh = await this.ports.read(this.scope(), 'refresh:' + record.previousFamilyId);
                if (oldRefresh) {
                    const previousRefresh = JSON.parse(oldRefresh);
                    previousRefresh.state = 'completed';
                    entries.push(this.entry('refresh:' + record.previousFamilyId, JSON.stringify(previousRefresh)));
                }
            }
        }
        if (refresh) {
            refresh.state = 'completed';
            entries.push(this.entry('refresh:' + refresh.familyId, JSON.stringify(refresh)));
        }
        await this.ports.commit(this.scope(), oldNamespaces, Models_1.AuthRules.namespace(pair), entries);
        this.pair = pair;
        this.user = user;
        this.visible = expose;
        if (record) {
            this.revision++;
        }
        if (expose) {
            this.ports.publish(pair, user);
        }
        if (record && previous && previous.pointer.alias !== binding.pointer.alias) {
            try {
                await this.ports.remove(previous.pointer.alias);
            }
            catch (_error) { /* Previous pointer is no longer current; never roll back the secured new pair. */ }
        }
    }
    async restore(legacy) {
        await this.verify();
        if (await this.ports.read(this.scope(), 'restore-blocked')) {
            throw new Error('RESTORE_EPOCH_CHANGED');
        }
        if (legacy && !legacy.sourceContextId && legacy.profile === this.connection.profile && legacy.deployment === this.connection.deployment) {
            const expected = 'JobsIM|' + Models_1.Isolation.namespace(legacy.profile, legacy.deployment, legacy.account, false, legacy.epoch, legacy.device);
            if (legacy.alias !== expected) {
                throw new Error('SESSION_ALIAS_MISMATCH');
            }
            await this.ports.write(this.scope(), [this.entry('legacy-pointer', JSON.stringify(legacy))]);
            await this.ports.freeze(Models_1.Isolation.namespace(legacy.profile, legacy.deployment, legacy.account, false, legacy.epoch, legacy.device), 'authentication_protocol_changed');
            // A legacy AT is never read as a refresh credential.
        }
        const initialId = await this.ports.read(this.scope(), 'active-initial');
        const initial = initialId ? await this.getRecord(initialId) : undefined;
        let binding;
        if (initial && !['completed', 'failed', 'recovered'].includes(initial.state)) {
            if (!initial.result) {
                throw new Error('AUTH_REAUTH_REQUIRED');
            }
            const secret = await this.ports.load(initial.result.pointer.alias);
            if (!secret) {
                throw new Error('AUTH_REAUTH_REQUIRED');
            }
            const candidate = Models_1.AuthRules.decode(secret);
            if (!Models_1.AuthRules.matches(candidate, initial.result) || candidate.requestId !== initial.requestId) {
                throw new Error('AUTH_REAUTH_REQUIRED');
            }
            await this.locked(async () => this.bind(candidate, initial.result?.user, initial, undefined, false));
            binding = initial.result;
        }
        else {
            const value = await this.ports.read(this.scope(), 'active');
            binding = value ? JSON.parse(value) : undefined;
            if (!binding || binding.state !== 'active') {
                return;
            }
            const secret = await this.ports.load(binding.pointer.alias);
            if (!secret) {
                return;
            }
            const pair = Models_1.AuthRules.decode(secret);
            if (pair.profile !== this.connection.profile || pair.deployment !== this.connection.deployment || pair.epoch !== this.connection.epoch) {
                throw new Error('AUTH_REAUTH_REQUIRED');
            }
            const valueRefresh = await this.ports.read(this.scope(), 'refresh:' + pair.familyId);
            if (valueRefresh && JSON.parse(valueRefresh).state !== 'completed') {
                const refresh = JSON.parse(valueRefresh);
                if (pair.requestId !== refresh.requestId || pair.generation !== Models_1.AuthRules.next(refresh.generation) ||
                    pair.userId !== binding.pointer.account || pair.deviceId !== binding.pointer.device || pair.familyId !== binding.familyId) {
                    throw new Error('AUTH_REAUTH_REQUIRED');
                }
                await this.locked(async () => this.bind(pair, binding?.user, undefined, refresh, false));
                binding = Models_1.AuthRules.binding(pair, binding.user);
            }
            else {
                if (!Models_1.AuthRules.matches(pair, binding)) {
                    throw new Error('AUTH_REAUTH_REQUIRED');
                }
                this.pair = pair;
                this.user = binding.user;
            }
        }
        const pair = this.pair;
        if (!pair || !binding || await this.ports.read(this.scope(), 'logout:' + pair.familyId)) {
            throw new Error('AUTH_REAUTH_REQUIRED');
        }
        if (await this.ports.read(this.scope(), 'password-change:' + pair.familyId)) {
            this.passwordFamily = pair.familyId;
            return;
        }
        if (await this.ports.read(this.scope(), 'account-deletion:' + pair.familyId)) {
            this.deletionFamily = pair.familyId;
            return;
        }
        const contextStamp = this.stamp();
        let contextToken = '';
        let context;
        try {
            contextToken = await this.before(contextStamp);
            context = await this.ports.context(contextToken);
        }
        catch (error) {
            if (!this.ports.current() || contextStamp !== this.stamp()) {
                throw new Error('SESSION_SCOPE_CHANGED');
            }
            if (error instanceof Models_1.ApiFailure && error.status === 401 && error.code === 'ACCESS_EXPIRED') {
                context = await this.ports.context(await this.refresh(contextToken, contextStamp));
            }
            else {
                await this.block(contextStamp);
                throw error;
            }
        }
        if (!this.ports.current() || contextStamp !== this.stamp()) {
            throw new Error('SESSION_SCOPE_CHANGED');
        }
        if (!this.pair || context.user.id !== this.pair.userId || context.deviceId !== this.pair.deviceId || context.restoreEpoch !== this.pair.epoch) {
            await this.block(contextStamp);
            throw new Error('AUTH_SCOPE_MISMATCH');
        }
        this.user = context.user;
        this.visible = true;
        this.ports.publish(this.pair, this.user);
    }
    async refresh(token, stamp) {
        if (!this.ports.current() || stamp !== this.stamp() || !this.pair) {
            throw new Error('SESSION_SCOPE_CHANGED');
        }
        if (await this.ports.read(this.scope(), 'password-change:' + this.pair.familyId) || await this.ports.read(this.scope(), 'account-deletion:' + this.pair.familyId)) {
            throw new Error('PASSWORD_CHANGE_RESULT_UNKNOWN');
        }
        if (this.pair.token !== token) {
            return this.pair.token;
        }
        if (this.refreshTask) {
            return this.refreshTask;
        }
        await this.verify();
        if (!this.ports.current() || stamp !== this.stamp() || !this.pair) {
            throw new Error('SESSION_SCOPE_CHANGED');
        }
        if (this.pair.token !== token) {
            return this.pair.token;
        }
        if (this.refreshTask) {
            return this.refreshTask;
        }
        const original = this.pair;
        const scope = this.scope();
        const task = async () => {
            const marker = { familyId: original.familyId, generation: original.generation, requestId: this.ports.nonce(), state: 'pending', username: original.username };
            await this.locked(async () => {
                if (stamp !== this.stamp() || this.pair?.token !== token) {
                    throw new Error('SESSION_SCOPE_CHANGED');
                }
                const prior = await this.ports.read(scope, 'refresh:' + original.familyId);
                if (prior && JSON.parse(prior).state !== 'completed') {
                    throw new Error('AUTH_REAUTH_REQUIRED');
                }
                await this.ports.write(scope, [this.entry('refresh:' + original.familyId, JSON.stringify(marker))]);
            });
            try {
                const result = await this.ports.send('/auth/refresh', JSON.stringify({ refreshToken: original.refreshToken, requestId: marker.requestId, familyId: original.familyId, refreshGeneration: original.generation }));
                const pair = Models_1.AuthRules.pair(result, this.connection, marker.requestId);
                Models_1.AuthRules.rotate(pair, original);
                await this.locked(async () => {
                    if (stamp !== this.stamp() || this.pair?.token !== token) {
                        throw new Error('SESSION_SCOPE_CHANGED');
                    }
                    await this.bind(pair, result.user, undefined, marker, this.visible);
                });
                return pair.token;
            }
            catch (error) {
                marker.state = error instanceof Models_1.ApiFailure && ['REFRESH_REUSE_DETECTED', 'REFRESH_INVALID', 'REFRESH_EXPIRED', 'REFRESH_REAUTH_REQUIRED'].includes(error.code) ? 'reauth_required' : 'unknown';
                try {
                    await this.ports.write(scope, [this.entry('refresh:' + original.familyId, JSON.stringify(marker))]);
                }
                catch (_storage) { }
                await this.block(stamp, original.token);
                throw new Error('AUTH_REAUTH_REQUIRED');
            }
        };
        this.refreshTask = task();
        try {
            return await this.refreshTask;
        }
        finally {
            this.refreshTask = undefined;
        }
    }
    async chain(record) {
        const result = [];
        const visiting = [];
        const append = async (id) => {
            if (result.includes(id)) {
                return;
            }
            if (id.length < 8 || id.length > 128 || visiting.includes(id)) {
                throw new Error('AUTH_CHAIN_INCOMPLETE');
            }
            const item = await this.getRecord(id);
            if (!item) {
                throw new Error('AUTH_CHAIN_INCOMPLETE');
            }
            visiting.push(id);
            if (item.previousRequestIds) {
                for (const previous of item.previousRequestIds) {
                    await append(previous);
                }
            }
            if (item.previousRequestId) {
                await append(item.previousRequestId);
            }
            visiting.pop();
            if (result.length >= 64) {
                throw new Error('AUTH_CHAIN_LIMIT');
            }
            result.push(id);
        };
        await append(record.requestId);
        return result;
    }
    async authenticate(mode, username, password, otp, proposedId, explicitNew) {
        const bootstrap = await this.verify();
        const startStamp = this.stamp();
        const scope = this.scope();
        const old = await this.pending(username);
        let record;
        if (old && !explicitNew) {
            if (old.purpose !== mode && !(mode === 'login' && old.purpose === 'reauthenticate')) {
                throw new Error('ORIGINAL_AUTHENTICATION_REQUIRED');
            }
            record = old;
        }
        else {
            record = { requestId: proposedId, purpose: mode, username: username.toLowerCase(), deviceName: 'JobsIM HarmonyOS', otpRequired: mode === 'login' && otp.length > 0, state: 'pending' };
            if (old && explicitNew) {
                if (bootstrap.features?.auth_retirement_chain !== true) {
                    throw new Error('AUTH_RETIREMENT_CHAIN_UNSUPPORTED');
                }
                if (proposedId === old.requestId || await this.getRecord(proposedId)) {
                    throw new Error('AUTHENTICATION_ID_REUSE');
                }
                if (old.purpose === 'register' && old.state !== 'delivery_expired' && !old.result) {
                    throw new Error('ORIGINAL_REGISTRATION_REQUIRED');
                }
                record.purpose = 'reauthenticate';
                record.previousRequestId = old.requestId;
                record.previousRequestIds = await this.chain(old);
                if (old.result) {
                    record.previousFamilyId = old.result.familyId;
                }
            }
            else if (mode === 'login') {
                const prior = await this.ports.read(scope, 'binding:' + record.username);
                if (prior) {
                    const binding = JSON.parse(prior);
                    const refresh = await this.ports.read(scope, 'refresh:' + binding.familyId);
                    if (await this.ports.read(scope, 'logout:' + binding.familyId) || (refresh && JSON.parse(refresh).state !== 'completed')) {
                        record.purpose = 'reauthenticate';
                        record.previousFamilyId = binding.familyId;
                    }
                }
            }
            if (explicitNew && record.purpose !== 'reauthenticate') {
                throw new Error('PREVIOUS_AUTHENTICATION_REQUIRED');
            }
            await this.ports.write(scope, [this.entry('request:' + record.requestId, JSON.stringify(record)), this.entry('pending:' + record.username, record.requestId)]);
        }
        const proof = this.proofs.find((value) => value.requestId === record.requestId);
        const effectiveOtp = record.otpRequired ? otp : '';
        if (proof && (proof.password !== password || proof.otp !== effectiveOtp)) {
            throw new Error('ORIGINAL_AUTHENTICATION_PROOF_CHANGED');
        }
        if (!proof) {
            this.proofs.push({ requestId: record.requestId, password: password, otp: effectiveOtp });
        }
        await this.ports.write(scope, [this.entry('active-initial', record.requestId)]);
        try {
            if (!this.ports.current() || startStamp !== this.stamp()) {
                throw new Error('SESSION_SCOPE_CHANGED');
            }
            const result = await this.ports.send('/auth/' + record.purpose, Models_1.AuthRules.initialBody(record, password, effectiveOtp));
            const pair = Models_1.AuthRules.pair(result, this.connection, record.requestId);
            await this.locked(async () => {
                if (!this.ports.current() || scope !== this.scope() || await this.ports.read(scope, 'active-initial') !== record.requestId || await this.ports.read(scope, 'pending:' + record.username) !== record.requestId) {
                    throw new Error('SESSION_SCOPE_CHANGED');
                }
                await this.bind(pair, result.user, record, undefined, true);
            });
            this.proofs = this.proofs.filter((value) => value.requestId !== record.requestId);
        }
        catch (error) {
            const known = error instanceof Models_1.ApiFailure && ['INVALID_CREDENTIALS', 'MFA_REQUIRED', 'MFA_INVALID', 'INVALID_USERNAME', 'INVALID_PASSWORD', 'USERNAME_TAKEN', 'RESERVED_USERNAME', 'USERNAME_RESERVED'].includes(error.code) && !old;
            record.state = error instanceof Models_1.ApiFailure && error.code === 'DELIVERY_EXPIRED' ? 'delivery_expired' : known ? 'failed' : 'unknown';
            try {
                await this.ports.write(scope, [this.entry('request:' + record.requestId, JSON.stringify(record))]);
            }
            catch (_storage) { }
            if (!known) {
                await this.block(startStamp);
            }
            if (known) {
                this.proofs = this.proofs.filter((value) => value.requestId !== record.requestId);
            }
            throw error;
        }
    }
    async clear(stamp = this.stamp()) {
        await this.locked(async () => {
            if (!this.ports.current() || stamp !== this.stamp()) {
                return;
            }
            const pair = this.pair;
            await this.ports.write(this.scope(), [this.entry('active', '')]);
            this.pair = undefined;
            this.user = undefined;
            this.visible = false;
            this.revision++;
            this.ports.publish(undefined, undefined);
            if (pair) {
                await this.ports.remove(Models_1.AuthRules.pointer(pair).alias);
            }
        });
    }
    async recovered(username) {
        await this.locked(async () => {
            const pending = await this.pending(username);
            if (pending) {
                pending.state = 'recovered';
                await this.ports.write(this.scope(), [this.entry('request:' + pending.requestId, JSON.stringify(pending))]);
            }
        });
    }
    async logout() {
        const pair = this.pair;
        if (!pair) {
            await this.clear();
            return;
        }
        const scope = this.scope();
        const stamp = this.stamp();
        await this.locked(async () => {
            if (stamp !== this.stamp()) {
                throw new Error('SESSION_SCOPE_CHANGED');
            }
            await this.ports.write(scope, [this.entry('logout:' + pair.familyId, pair.familyId)]);
            this.closing = true;
        });
        let succeeded = false;
        try {
            try {
                await this.ports.logout(pair.token);
            }
            catch (error) {
                if (error instanceof Models_1.ApiFailure && error.code === 'ACCESS_EXPIRED') {
                    await this.ports.logout(await this.refresh(pair.token, stamp));
                }
                else {
                    throw error;
                }
            }
            succeeded = true;
        }
        finally {
            if (succeeded) {
                await this.ports.write(scope, [this.entry('logout:' + pair.familyId, '')]);
            }
            try {
                await this.clear(stamp);
            }
            finally {
                this.closing = false;
            }
        }
    }
}
exports.AuthCoordinator = AuthCoordinator;
AuthCoordinator.mutationTail = Promise.resolve();

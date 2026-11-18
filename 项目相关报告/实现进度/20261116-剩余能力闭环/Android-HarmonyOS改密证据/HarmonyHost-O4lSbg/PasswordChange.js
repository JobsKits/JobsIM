"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.PasswordChange = exports.PasswordRules = void 0;
// Created by Jobs on 2026年11月16日，星期一.
const Models_1 = require("./Models");
const SyncProtocol_1 = require("./SyncProtocol");
const SelfProtocol_1 = require("./SelfProtocol");
class PasswordRules {
    static fail(code = 'PASSWORD_CHANGE_INVALID') { throw new Error(code); }
    static normalized(old, next, confirm, otp) {
        old = old.normalize('NFC');
        next = next.normalize('NFC');
        if (!old || old === next || next !== confirm.normalize('NFC') || Array.from(next).length < 15 || Array.from(next).length > 128 ||
            SelfProtocol_1.SelfRules.bytes(old) > 512 || SelfProtocol_1.SelfRules.bytes(next) > 512 || otp && !/^[0-9]{6}$/.test(otp)) {
            PasswordRules.fail('PASSWORD_CHANGE_INPUT_INVALID');
        }
        return { currentPassword: old, newPassword: next, otp: otp };
    }
    static definiteRejection(status, code) { return status === 400 && code === 'INVALID_PASSWORD_CHANGE' || status === 403 && code === 'PASSWORD_CHANGE_REJECTED' || status === 429 && code === 'RATE_LIMITED'; }
    static blocked(row) { return row.state === 'changed' || row.attempts > 0 && ['unknown', 'pending'].includes(row.state); }
    static validate(row) {
        if (Object.keys(row).sort().join(',') !== 'attempts,created,deployment,device,epoch,family,id,namespace,origin,profile,result,state,user,username' ||
            ![row.id, row.deployment, row.user, row.device, row.family].every((value) => SyncProtocol_1.SyncRules.uuid(value)) ||
            !row.profile || !row.namespace || !/^[a-z0-9_]{3,32}$/.test(row.username) || SyncProtocol_1.SyncRules.counter(row.epoch) === '0' ||
            !Number.isSafeInteger(row.attempts) || row.attempts < 0 || !['unsent', 'unknown', 'pending', 'changed', 'retired', 'rejected'].includes(row.state)) {
            PasswordRules.fail();
        }
        SelfProtocol_1.SelfRules.timestamp(row.created);
        if (row.result) {
            PasswordRules.result(row.result, row);
        }
    }
    static result(raw, row) {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        const status = value.get('status').string();
        const current = value.get('current').bool();
        const login = value.get('requiresNewLogin').bool();
        const credential = SyncProtocol_1.SyncRules.counter(value.get('credentialGeneration').string());
        const security = SyncProtocol_1.SyncRules.counter(value.get('securityVersion').string());
        if (value.get('requestId').string() !== row.id || value.get('restoreEpoch').string() !== row.epoch || credential === '0' || security === '0' ||
            !['pending', 'changed', 'retired'].includes(status) || current && status !== 'changed' || status === 'changed' && !login) {
            PasswordRules.fail();
        }
        return { requestId: row.id, status: status, restoreEpoch: row.epoch, credentialGeneration: credential, securityVersion: security, current: current, requiresNewLogin: login };
    }
}
exports.PasswordRules = PasswordRules;
class PasswordChange {
    constructor(ports) { this.ports = ports; }
    check() { if (!this.ports.current()) {
        PasswordRules.fail('PASSWORD_CHANGE_CONTEXT_CHANGED');
    } }
    async capability(row, required) {
        this.check();
        const boot = await this.ports.bootstrap();
        this.check();
        if (boot.deploymentId !== row.deployment) {
            PasswordRules.fail('PASSWORD_CHANGE_CONTEXT_CHANGED');
        }
        const enabled = boot.restoreEpoch === row.epoch && boot.features?.own_password_change === true;
        if (required && !enabled) {
            PasswordRules.fail('PASSWORD_CHANGE_UNAVAILABLE');
        }
        return enabled;
    }
    async perform(id, proof, retry) {
        proof = PasswordRules.normalized(proof.currentPassword, proof.newPassword, proof.newPassword, proof.otp);
        this.check();
        let rows = await this.ports.repository.list();
        this.check();
        let row;
        if (!id) {
            if (this.ports.prepare) {
                await this.ports.prepare();
                this.check();
            }
            const owner = this.ports.owner();
            if (!owner) {
                PasswordRules.fail('PASSWORD_CHANGE_LOGIN_REQUIRED');
            }
            row = { id: this.ports.uuid(), profile: this.ports.profile, origin: this.ports.origin, deployment: this.ports.deployment, epoch: this.ports.epoch,
                user: owner.user, username: owner.username, device: owner.device, family: owner.family, namespace: owner.namespace, created: new Date().toISOString(), attempts: 0, state: 'unsent', result: '' };
            PasswordRules.validate(row);
            await this.capability(row, true);
            this.check();
            if (this.ports.owner()?.family !== owner.family) {
                PasswordRules.fail('PASSWORD_CHANGE_CONTEXT_CHANGED');
            }
            await this.ports.repository.insert(row);
        }
        else {
            const found = rows.find((item) => item.id === id);
            if (!found) {
                PasswordRules.fail();
            }
            row = found;
        }
        PasswordRules.validate(row);
        if (row.profile !== this.ports.profile || row.origin !== this.ports.origin || row.deployment !== this.ports.deployment) {
            PasswordRules.fail();
        }
        const available = await this.capability(row, !id);
        const body = { requestId: row.id, username: row.username, deviceId: row.device, restoreEpoch: row.epoch, currentPassword: proof.currentPassword, newPassword: proof.newPassword };
        let result;
        let unseen = false;
        try {
            result = PasswordRules.result(await this.ports.request('/auth/password-change-results', JSON.stringify(body), ''), row);
        }
        catch (error) {
            this.check();
            if (error instanceof Models_1.ApiFailure && error.status === 404 && error.code === 'PASSWORD_CHANGE_OPERATION_NOT_FOUND') {
                unseen = true;
            }
            else {
                throw error;
            }
        }
        this.check();
        if (!result && id && !retry) {
            return undefined;
        }
        if (!result || result.status === 'pending' && retry) {
            if (!available) {
                PasswordRules.fail('PASSWORD_CHANGE_UNAVAILABLE');
            }
            const owner = this.ports.owner();
            const token = unseen && owner?.family === row.family && owner.device === row.device && owner.user === row.user && owner.namespace === row.namespace ? owner.token : '';
            if (unseen && !token) {
                PasswordRules.fail('PASSWORD_CHANGE_LOGIN_REQUIRED');
            }
            const before = row;
            row = JSON.parse(JSON.stringify(row));
            row.attempts++;
            row.state = 'unknown';
            await this.ports.repository.replace(before, row);
            this.check();
            if (token && proof.otp) {
                body.otp = proof.otp;
            }
            try {
                result = PasswordRules.result(await this.ports.request('/auth/password-change', JSON.stringify(body), token), row);
                this.check();
            }
            catch (error) {
                this.check();
                if (!id && before.attempts === 0 && error instanceof Models_1.ApiFailure && PasswordRules.definiteRejection(error.status, error.code)) {
                    const rejected = JSON.parse(JSON.stringify(row));
                    rejected.state = 'rejected';
                    await this.ports.repository.replace(row, rejected);
                }
                throw error;
            }
        }
        if (!result) {
            PasswordRules.fail();
        }
        const after = JSON.parse(JSON.stringify(row));
        after.state = result.status;
        after.result = JSON.stringify(result);
        await this.ports.repository.replace(row, after);
        this.check();
        if (result.requiresNewLogin) {
            await this.ports.stop(after);
        }
        return result;
    }
}
exports.PasswordChange = PasswordChange;

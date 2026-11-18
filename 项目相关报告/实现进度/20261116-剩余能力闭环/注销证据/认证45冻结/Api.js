"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.Api = void 0;
const _kit_RemoteCommunicationKit_1 = require("@kit.RemoteCommunicationKit");
const _ohos_file_fs_1 = require("@ohos.file.fs");
const Models_1 = require("./Models");
const ExportProtocol_1 = require("./ExportProtocol");
class Api {
    constructor(base, token = '', caPath = '', hooks) {
        this.boundStamp = '';
        this.base = base;
        this.token = token;
        this.caPath = caPath;
        this.hooks = hooks;
        this.boundStamp = hooks ? hooks.stamp() : '';
    }
    session(timeout) {
        const security = this.caPath.length > 0 ? { remoteValidation: { filePath: this.caPath } } : { remoteValidation: 'system' };
        return _kit_RemoteCommunicationKit_1.rcp.createSession({ requestConfiguration: { security: security, transfer: { autoRedirect: false, timeout: { connectMs: 3500, transferMs: timeout } } } });
    }
    decode(response) {
        const value = response.toString() ?? '{}';
        if (value.length > 2097152) {
            throw new Error('Response limit');
        }
        if (response.statusCode < 200 || response.statusCode >= 300) {
            let code = '';
            try {
                code = JSON.parse(value).error.code;
            }
            catch (_) { }
            throw new Models_1.ApiFailure(response.statusCode, code || 'HTTP_ERROR');
        }
        try {
            return JSON.parse(value.length > 0 ? value : '{}');
        }
        catch (_error) {
            throw new Models_1.ApiFailure(0, 'INVALID_RESPONSE');
        }
    }
    check() {
        if (this.hooks && this.boundStamp !== this.hooks.stamp()) {
            throw new Error('SESSION_SCOPE_CHANGED');
        }
    }
    async authorized(operation) {
        const hooks = this.hooks;
        if (!hooks) {
            return operation(this.token);
        }
        const stamp = this.boundStamp;
        let token = await hooks.before(stamp);
        this.check();
        try {
            const result = await operation(token);
            this.check();
            return result;
        }
        catch (error) {
            this.check();
            if (!(error instanceof Models_1.ApiFailure) || error.status !== 401) {
                throw error;
            }
            if (error.code === 'ACCESS_EXPIRED') {
                token = await hooks.refresh(token, stamp);
            }
            else {
                const current = await hooks.before(stamp);
                if (current === token) {
                    await hooks.rejected(token, stamp);
                    throw error;
                }
                token = current;
            }
            this.check();
            try {
                const result = await operation(token);
                this.check();
                return result;
            }
            catch (second) {
                if (second instanceof Models_1.ApiFailure && second.status === 401) {
                    await hooks.rejected(token, stamp);
                }
                throw second;
            }
        }
    }
    async request(path, method = 'GET', body = '') {
        const publicPath = ['/bootstrap', '/auth/register', '/auth/login', '/auth/refresh', '/auth/reauthenticate', '/auth/recover'].includes(path);
        if (publicPath) {
            return this.requestRaw(path, method, body, '');
        }
        return this.authorized((token) => this.requestRaw(path, method, body, token));
    }
    async requestRaw(path, method, body, token) {
        const session = this.session(5000);
        try {
            const headers = { 'Accept': 'application/json', 'Content-Type': 'application/json' };
            if (token.length > 0) {
                headers['Authorization'] = 'Bearer ' + token;
            }
            const request = new _kit_RemoteCommunicationKit_1.rcp.Request(this.base + '/api' + path, method, headers, body.length > 0 ? body : undefined);
            return this.decode(await session.fetch(request));
        }
        catch (error) {
            if (error instanceof Models_1.ApiFailure) {
                throw error;
            }
            throw new Models_1.ApiFailure(0, 'TRANSPORT_UNKNOWN');
        }
        finally {
            session.close();
        }
    }
    async upload(uri) {
        return this.authorized((token) => this.uploadRaw(uri, token));
    }
    async uploadRaw(uri, token) {
        const file = _ohos_file_fs_1.default.openSync(uri, _ohos_file_fs_1.default.OpenMode.READ_ONLY);
        let buffer;
        try {
            const size = _ohos_file_fs_1.default.statSync(file.fd).size;
            if (size > 25 * 1024 * 1024) {
                throw new Error('Attachment limit: 25 MiB');
            }
            buffer = new ArrayBuffer(size);
            const read = _ohos_file_fs_1.default.readSync(file.fd, buffer);
            if (read !== size) {
                throw new Error('Incomplete attachment');
            }
        }
        finally {
            _ohos_file_fs_1.default.closeSync(file);
        }
        const session = this.session(30000);
        try {
            const fields = { file: { contentType: 'application/octet-stream', remoteFileName: 'attachment', contentOrPath: { content: buffer } } };
            const request = new _kit_RemoteCommunicationKit_1.rcp.Request(this.base + '/api/media', 'POST', { 'Authorization': 'Bearer ' + token }, new _kit_RemoteCommunicationKit_1.rcp.MultipartForm(fields));
            return this.decode(await session.fetch(request));
        }
        catch (error) {
            if (error instanceof Models_1.ApiFailure) {
                throw error;
            }
            throw new Models_1.ApiFailure(0, 'TRANSPORT_UNKNOWN');
        }
        finally {
            session.close();
        }
    }
    async download(path) {
        return this.authorized((token) => this.downloadRaw(path, token));
    }
    async downloadRaw(path, token) {
        if (!/^\/(media|avatars)\/[A-Za-z0-9_-]+$/.test(path) && !/^\/messages\/[A-Za-z0-9_-]+\/attachment$/.test(path)) {
            throw new Error('Invalid private resource path');
        }
        const session = this.session(30000);
        try {
            const response = await session.fetch(new _kit_RemoteCommunicationKit_1.rcp.Request(this.base + '/api' + path, 'GET', { Authorization: 'Bearer ' + token }));
            if (response.statusCode < 200 || response.statusCode >= 300) {
                this.decode(response);
            }
            const data = response.body;
            if (!data || data.byteLength > 25 * 1024 * 1024) {
                throw new Error('Attachment size limit');
            }
            const mime = response.headers['content-type'];
            return { data: data, contentType: typeof mime === 'string' ? mime : 'application/octet-stream' };
        }
        catch (error) {
            if (error instanceof Models_1.ApiFailure) {
                throw error;
            }
            throw new Models_1.ApiFailure(0, 'TRANSPORT_UNKNOWN');
        }
        finally {
            session.close();
        }
    }
    async dataExport(job, ticket, sink, current) {
        if (!/^[0-9a-f-]{36}$/.test(job) || !/^[A-Za-z0-9_-]{16,256}$/.test(ticket)) {
            ExportProtocol_1.ExportRules.fail('EXPORT_FILE_INVALID');
        }
        try {
            return await this.authorized(async (token) => {
                if (!current()) {
                    ExportProtocol_1.ExportRules.fail('EXPORT_CONTEXT_CHANGED');
                }
                sink.reset();
                const session = this.session(120000);
                let stopped = '';
                const timer = setInterval(() => { if (!current()) {
                    stopped = 'EXPORT_CONTEXT_CHANGED';
                    session.close();
                } }, 100);
                try {
                    const request = new _kit_RemoteCommunicationKit_1.rcp.Request(this.base + '/api/me/data-exports/' + job + '/download', 'GET', { 'Authorization': 'Bearer ' + token, 'X-JobsIM-Download-Token': ticket, 'Accept': 'application/zip', 'Cache-Control': 'no-store' });
                    request.destination = (bytes) => { try {
                        if (!current()) {
                            ExportProtocol_1.ExportRules.fail('EXPORT_CONTEXT_CHANGED');
                        }
                        sink.write(bytes);
                    }
                    catch (error) {
                        stopped = error.message;
                        session.close();
                        throw error;
                    } };
                    request.configuration = { tracing: { httpEventsHandler: { onHeaderReceive: (headers) => { const value = headers['content-length']; if (typeof value === 'string' && /^\d+$/.test(value) && Number(value) > ExportProtocol_1.ExportRules.limit) {
                                    stopped = 'EXPORT_FILE_LIMIT';
                                    session.close();
                                } } } } };
                    const response = await session.fetch(request);
                    if (stopped) {
                        throw new Error(stopped);
                    }
                    if (response.statusCode !== 200) {
                        let code = 'EXPORT_DOWNLOAD_FAILED';
                        try {
                            code = JSON.parse(sink.errorText()).error.code;
                        }
                        catch (_) { }
                        throw new Models_1.ApiFailure(response.statusCode, code);
                    }
                    const mime = response.headers['content-type'];
                    const disposition = response.headers['content-disposition'];
                    const length = response.headers['content-length'];
                    if (typeof mime !== 'string' || mime.split(';')[0].trim().toLowerCase() !== 'application/zip' || typeof disposition !== 'string' || length !== undefined && (typeof length !== 'string' || !/^\d+$/.test(length))) {
                        ExportProtocol_1.ExportRules.fail('EXPORT_FILE_INVALID');
                    }
                    if (!current()) {
                        ExportProtocol_1.ExportRules.fail('EXPORT_CONTEXT_CHANGED');
                    }
                    return await sink.finish(job, ExportProtocol_1.ExportRules.fileName(disposition), typeof length === 'string' ? Number(length) : -1);
                }
                catch (error) {
                    if (stopped) {
                        throw new Error(stopped);
                    }
                    throw error;
                }
                finally {
                    clearInterval(timer);
                    session.close();
                }
            });
        }
        catch (error) {
            sink.dispose();
            throw error;
        }
    }
    async avatar(path, requestId, expectedVersion) {
        return this.authorized((token) => this.avatarRaw(path, requestId, expectedVersion, token));
    }
    async avatarRaw(path, requestId, expectedVersion, token) {
        const file = _ohos_file_fs_1.default.openSync(path, _ohos_file_fs_1.default.OpenMode.READ_ONLY);
        let buffer;
        try {
            const size = _ohos_file_fs_1.default.statSync(file.fd).size;
            if (size > 2 * 1024 * 1024) {
                throw new Error('Avatar limit: 2 MiB');
            }
            buffer = new ArrayBuffer(size);
            if (_ohos_file_fs_1.default.readSync(file.fd, buffer) !== size) {
                throw new Error('Incomplete avatar');
            }
        }
        finally {
            _ohos_file_fs_1.default.closeSync(file);
        }
        const session = this.session(30000);
        try {
            const fields = { file: { remoteFileName: 'avatar', contentType: 'application/octet-stream', contentOrPath: { content: buffer } }, requestId: requestId, expectedVersion: expectedVersion };
            return this.decode(await session.fetch(new _kit_RemoteCommunicationKit_1.rcp.Request(this.base + '/api/me/avatar', 'POST', { Authorization: 'Bearer ' + token }, new _kit_RemoteCommunicationKit_1.rcp.MultipartForm(fields))));
        }
        catch (error) {
            if (error instanceof Models_1.ApiFailure) {
                throw error;
            }
            throw new Models_1.ApiFailure(0, 'TRANSPORT_UNKNOWN');
        }
        finally {
            session.close();
        }
    }
}
exports.Api = Api;

"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ExportRules = void 0;
// Created by Jobs on 2026年11月16日，星期一.
const SyncProtocol_1 = require("./SyncProtocol");
const SelfProtocol_1 = require("./SelfProtocol");
class ExportRules {
    static fail(code = 'EXPORT_INVALID_RESPONSE') { throw new Error(code); }
    static view(raw, epoch, expected = '') {
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        const job = value.get('job');
        if (value.get('format').string() !== 'zip' || value.get('scope').string() !== 'personal_current') {
            ExportRules.fail();
        }
        const id = job.get('id').string();
        const artifact = job.get('artifactId').string();
        if (!SyncProtocol_1.SyncRules.uuid(id) || !SyncProtocol_1.SyncRules.uuid(artifact) || expected && expected !== id) {
            ExportRules.fail();
        }
        const actualEpoch = SyncProtocol_1.SyncRules.counter(job.get('restoreEpoch').string());
        const generation = SyncProtocol_1.SyncRules.counter(job.get('buildGeneration').string());
        if (actualEpoch !== epoch || actualEpoch === '0' || generation === '0') {
            ExportRules.fail('RESTORE_EPOCH_CHANGED');
        }
        const state = job.get('state').string();
        const cleanup = job.get('cleanupState').string();
        if (!['requested', 'building', 'ready', 'cancelled', 'expired', 'failed'].includes(state) || !/^[a-z_]{1,48}$/.test(cleanup)) {
            ExportRules.fail();
        }
        const expires = job.get('expiresAt').string();
        SelfProtocol_1.SelfRules.timestamp(expires);
        for (const key of ['coverage', 'exclusions']) {
            const list = value.get(key).list();
            if (list.length > 64 || list.some((item) => !/^[a-z_]{1,100}$/.test(item.string()))) {
                ExportRules.fail();
            }
        }
        if (value.has('capturedAt')) {
            SelfProtocol_1.SelfRules.timestamp(value.get('capturedAt').string());
        }
        const failure = job.has('failureCode') ? job.get('failureCode').string() : '';
        if (failure && !/^[A-Z0-9_]{1,64}$/.test(failure)) {
            ExportRules.fail();
        }
        return { id: id, artifact: artifact, epoch: actualEpoch, generation: generation, state: state, expires: expires, cleanup: cleanup, failure: failure };
    }
    static ready(view) { return view.state === 'ready' && view.cleanup === 'published' && Date.parse(view.expires) > Date.now(); }
    static original(row, epoch, physical) {
        if (Object.keys(row).sort().join(',') !== 'body,cancelBody,cancelHash,created,hash,id,job,physical') {
            ExportRules.fail('EXPORT_ORIGINAL_CHANGED');
        }
        if (!SyncProtocol_1.SyncRules.uuid(row.id) || row.physical !== physical || !/^[a-f0-9]{64}$/.test(row.hash) || row.body.length > 1024 || row.job && !SyncProtocol_1.SyncRules.uuid(row.job)) {
            ExportRules.fail('EXPORT_ORIGINAL_CHANGED');
        }
        SelfProtocol_1.SelfRules.timestamp(row.created);
        const body = SyncProtocol_1.SyncJSON.parse(row.body);
        if (body.keys().sort().join(',') !== 'requestId,restoreEpoch' || body.get('requestId').string() !== row.id || body.get('restoreEpoch').string() !== epoch) {
            ExportRules.fail('EXPORT_ORIGINAL_CHANGED');
        }
        if (Boolean(row.cancelBody) !== Boolean(row.cancelHash)) {
            ExportRules.fail('EXPORT_ORIGINAL_CHANGED');
        }
        if (row.cancelBody) {
            const cancel = SyncProtocol_1.SyncJSON.parse(row.cancelBody);
            if (!row.job || cancel.keys().join(',') !== 'requestId' || !SyncProtocol_1.SyncRules.uuid(cancel.get('requestId').string()) || cancel.get('requestId').string() === row.id || !/^[a-f0-9]{64}$/.test(row.cancelHash)) {
                ExportRules.fail('EXPORT_ORIGINAL_CHANGED');
            }
        }
    }
    static ticket(raw, view) {
        if (!ExportRules.ready(view)) {
            ExportRules.fail('EXPORT_NOT_READY');
        }
        const value = SyncProtocol_1.SyncJSON.parse(raw);
        const token = value.get('downloadToken').string();
        const expires = value.get('expiresAt').string();
        SelfProtocol_1.SelfRules.timestamp(expires);
        if (value.get('format').string() !== 'zip' || !/^[A-Za-z0-9_-]{16,256}$/.test(token) || Date.parse(expires) <= Date.now() || Date.parse(expires) > Date.parse(view.expires)) {
            ExportRules.fail();
        }
        return { downloadToken: token, expiresAt: expires, format: 'zip' };
    }
    static fileName(header) {
        if (!/^attachment(?:\s*;|$)/i.test(header)) {
            ExportRules.fail('EXPORT_FILE_INVALID');
        }
        const values = {};
        const pattern = /;\s*(filename\*|filename)\s*=\s*("(?:[^"\\]|\\.)*"|[^;]*)/gi;
        let match = pattern.exec(header);
        while (match) {
            const key = match[1].toLowerCase();
            if (Object.keys(values).includes(key)) {
                ExportRules.fail('EXPORT_FILE_INVALID');
            }
            let name = match[2].trim();
            if (name.startsWith('"')) {
                name = name.slice(1, -1);
            }
            values[key] = name;
            match = pattern.exec(header);
        }
        let name = values['filename*'] ?? values['filename'] ?? '';
        if (Object.keys(values).includes('filename*')) {
            if (!/^UTF-8''/i.test(name)) {
                ExportRules.fail('EXPORT_FILE_INVALID');
            }
            try {
                name = decodeURIComponent(name.slice(7));
            }
            catch (_) {
                ExportRules.fail('EXPORT_FILE_INVALID');
            }
        }
        if (!name || name.length > 180 || name !== name.trim() || /[\u0000-\u001f\u007f-\u009f\u200b-\u200f\u202a-\u202e\u2060-\u206f/\\<>:"|?*]/.test(name) || !name.toLowerCase().endsWith('.zip')) {
            ExportRules.fail('EXPORT_FILE_INVALID');
        }
        return name;
    }
    // Read bounded file slices. A truncated HTTP200 must never become a saved ZIP.
    static zip(size, read) {
        if (size > ExportRules.limit) {
            ExportRules.fail('EXPORT_FILE_LIMIT');
        }
        if (size < 22 || !Number.isSafeInteger(size)) {
            ExportRules.fail('EXPORT_FILE_INVALID');
        }
        const start = Math.max(0, size - 65557);
        const tail = new DataView(read(start, size - start).buffer);
        let end = -1;
        for (let i = tail.byteLength - 22; i >= 0; i--) {
            if (tail.getUint32(i, true) === 0x06054b50 && i + 22 + tail.getUint16(i + 20, true) === tail.byteLength) {
                end = i;
                break;
            }
        }
        if (end < 0 || tail.getUint16(end + 4, true) !== 0 || tail.getUint16(end + 6, true) !== 0) {
            ExportRules.fail('EXPORT_FILE_INVALID');
        }
        let count = tail.getUint16(end + 10, true);
        let bytes = tail.getUint32(end + 12, true);
        let offset = tail.getUint32(end + 16, true);
        let bound = start + end;
        if (tail.getUint16(end + 8, true) !== count) {
            ExportRules.fail('EXPORT_FILE_INVALID');
        }
        if (count === 65535 || bytes === 0xffffffff || offset === 0xffffffff) {
            if (end < 20 || tail.getUint32(end - 20, true) !== 0x07064b50 || tail.getUint32(end - 16, true) !== 0 || tail.getUint32(end - 4, true) !== 1) {
                ExportRules.fail('EXPORT_FILE_INVALID');
            }
            const at = ExportRules.u64(tail, end - 12);
            if (at + 56 > bound - 20) {
                ExportRules.fail('EXPORT_FILE_INVALID');
            }
            const record = new DataView(read(at, 56).buffer);
            if (record.getUint32(0, true) !== 0x06064b50 || ExportRules.u64(record, 4) < 44 || at + 12 + ExportRules.u64(record, 4) !== bound - 20 || record.getUint32(16, true) || record.getUint32(20, true) || ExportRules.u64(record, 24) !== ExportRules.u64(record, 32)) {
                ExportRules.fail('EXPORT_FILE_INVALID');
            }
            count = ExportRules.u64(record, 32);
            bytes = ExportRules.u64(record, 40);
            offset = ExportRules.u64(record, 48);
            bound = at;
        }
        if (bytes > ExportRules.directoryLimit || count > Math.floor(ExportRules.directoryLimit / 46)) {
            ExportRules.fail('EXPORT_FILE_LIMIT');
        }
        if (count < 1 || bytes < count * 46 || offset + bytes > bound) {
            ExportRules.fail('EXPORT_FILE_INVALID');
        }
        const directory = new DataView(read(offset, bytes).buffer);
        let cursor = 0;
        for (let i = 0; i < count; i++) {
            if (cursor + 46 > bytes || directory.getUint32(cursor, true) !== 0x02014b50) {
                ExportRules.fail('EXPORT_FILE_INVALID');
            }
            const name = directory.getUint16(cursor + 28, true);
            if (!name || directory.getUint16(cursor + 34, true) || directory.getUint32(cursor + 42, true) + directory.getUint32(cursor + 20, true) + 30 > offset) {
                ExportRules.fail('EXPORT_FILE_INVALID');
            }
            cursor += 46 + name + directory.getUint16(cursor + 30, true) + directory.getUint16(cursor + 32, true);
        }
        if (cursor !== bytes) {
            ExportRules.fail('EXPORT_FILE_INVALID');
        }
    }
    static u64(data, offset) { const value = data.getUint32(offset, true) + data.getUint32(offset + 4, true) * 4294967296; if (!Number.isSafeInteger(value) || value > ExportRules.limit) {
        ExportRules.fail('EXPORT_FILE_LIMIT');
    } return value; }
}
exports.ExportRules = ExportRules;
ExportRules.limit = 1073741824;
ExportRules.directoryLimit = 33554432;

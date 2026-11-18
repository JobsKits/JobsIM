"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ReceiptCoordinator = void 0;
// Created by Jobs on 2026年11月17日，星期二.
const SyncProtocol_1 = require("./SyncProtocol");
class ReceiptCoordinator {
    constructor(ports) {
        this.pending = new Map();
        this.cursor = '';
        this.revision = 0;
        this.running = false;
        this.ports = ports;
    }
    stop() { this.revision++; this.service?.clear(); this.service = undefined; this.running = false; this.pending.clear(); this.cursor = ''; }
    offer(raws) {
        if (!this.ports.enabled()) {
            return;
        }
        for (const raw of raws) {
            try {
                const m = SyncProtocol_1.SyncJSON.parse(raw);
                if (m.get('deleted').value !== true && m.get('forwarded').value !== true && m.get('contentUnavailable').value !== true && SyncProtocol_1.SyncRules.uuid(m.get('id').string()) && m.get('contentVersion').string() !== '0') {
                    const row = { id: m.get('id').string(), version: SyncProtocol_1.SyncRules.counter(m.get('contentVersion').string()) };
                    this.pending.set(row.id, row);
                }
            }
            catch (_) { }
        }
        while (this.pending.size > 512) {
            const first = Array.from(this.pending.keys())[0];
            this.pending.delete(first);
        }
        if (this.running) {
            return;
        }
        const stamp = this.revision;
        this.running = true;
        this.drain(stamp).finally(() => { if (stamp === this.revision) {
            this.running = false;
        } });
    }
    async drain(stamp) { let service; const current = () => stamp === this.revision && this.ports.enabled(); try {
        if (!current()) {
            return;
        }
        service = await this.ports.factory(current);
        this.service = service;
        await service.reconcile();
        if (!current()) {
            return;
        }
        const page = await this.ports.cached(this.cursor, 8);
        if (!current()) {
            return;
        }
        this.cursor = page.after;
        for (const row of page.rows) {
            if (!this.pending.has(row.id)) {
                this.pending.set(row.id, row);
            }
        }
        const items = Array.from(this.pending.values()).slice(0, 8);
        for (const row of items) {
            if (!current()) {
                return;
            }
            try {
                await service.receiveIfTarget(row.id, row.version);
            }
            catch (_) {
                if (!current()) {
                    return;
                }
            }
            this.pending.delete(row.id);
        }
    }
    catch (_) { }
    finally {
        service?.clear();
        if (stamp === this.revision) {
            this.service = undefined;
        }
    } }
}
exports.ReceiptCoordinator = ReceiptCoordinator;

"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.NativeStorageBudget = void 0;
// Created by Jobs on 2026年11月17日，星期二.
class NativeStorageBudget {
    static total(parts) { let total = 0; for (const size of parts) {
        if (!Number.isSafeInteger(size) || size < 0 || size > NativeStorageBudget.maximumPending - total) {
            throw new Error('STORAGE_BUDGET_INVALID');
        }
        total += size;
    } return total; }
    static fits(available, required) { if (!Number.isSafeInteger(available) || available < 0 || !Number.isSafeInteger(required) || required < 0 || required > NativeStorageBudget.maximumPending) {
        throw new Error('STORAGE_BUDGET_INVALID');
    } return available >= NativeStorageBudget.reserve && required <= available - NativeStorageBudget.reserve; }
}
exports.NativeStorageBudget = NativeStorageBudget;
NativeStorageBudget.reserve = 64 * 1024 * 1024;
NativeStorageBudget.maximumPending = 128 * 1024 * 1024 + 4096;

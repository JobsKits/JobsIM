// Created by Jobs on 2026年11月17日，星期二.
import { authSnapshot } from "./AuthSession";
import { api, apiBoundedBytes } from "./api";
import { migrationSyncSource } from "./SyncStore";
import { migrationBase64, migrationDecode, migrationEnvelope, migrationFailure, migrationHash, migrationLimit, migrationPositive, migrationSelect, migrationSourceBody, migrationTime, type MigrationSelection, type MigrationSourceData } from "./MigrationProtocol";
import { migrationIntents, stageMigration, updateMigration, type MigrationIntent, type MigrationStoreContext } from "./MigrationStore";
import type { Message } from "./types";
import { syncID, syncRecord } from "./SyncProtocol";
import { migrationFormat, migrationView, openMigration, type MigrationView } from "./MigrationArchive";
import { discardMigrationStage, migrationHistory, publishMigration, stagedMigrationCapsule, stageMigrationCapsule } from "./MigrationHistoryStore";

export type MigrationContext = MigrationStoreContext & { request?: typeof api; binary?: typeof apiBoundedBytes; signal?: AbortSignal };
export type MigrationPairing = { pairingId: string; pairingProof: string; targetDeviceId: string; targetStoreGeneration: string; restoreEpoch: string; expiresAt: string };
function pairingProof(proof: string, context: MigrationContext) {
  const parts = proof.split("."); if (proof.length > 16384 || parts.length !== 3 || parts[0] !== "mp1") migrationFailure();
  const bytes = migrationDecode(parts[1], 8192, true); if (migrationDecode(parts[2], 32, true).length !== 32) migrationFailure();
  const p = syncRecord(JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes))), i = context.identity;
  if (Object.keys(p).some(key => !["pairingId", "scopeId", "userId", "targetDeviceId", "targetStoreGeneration", "restoreEpoch", "securityVersion", "sourceGeneration", "expires"].includes(key)) || p.scopeId !== i.scopeId || p.userId !== i.userId || p.restoreEpoch !== i.epoch || !Number.isSafeInteger(p.expires)) migrationFailure("MIGRATION_TARGET_CHANGED");
  syncID(p.pairingId); syncID(p.targetDeviceId); syncID(p.targetStoreGeneration); migrationPositive(p.securityVersion); migrationPositive(p.sourceGeneration);
  return p;
}
function passwordProof(password: string, otp: string) {
  const normalized = password.normalize("NFC"); if (!normalized || new TextEncoder().encode(normalized).length > 512 || otp && !/^[0-9]{6}$/.test(otp)) migrationFailure("MIGRATION_PROOF_INVALID");
  return { password: normalized, ...(otp ? { otp } : {}) };
}
export class MigrationActions {
  constructor(readonly context: MigrationContext) {}
  private check(row?: MigrationIntent, write = false) {
    const identity = this.context.identity, auth = authSnapshot().auth;
    if (!this.context.isCurrent() || this.context.signal?.aborted || !auth || auth.familyId !== identity.familyId || auth.userId !== identity.userId || auth.deviceId !== identity.deviceId || auth.restoreEpoch !== identity.epoch || auth.deploymentId !== identity.scopeId) migrationFailure("MIGRATION_CONTEXT_CHANGED");
    if (write && row && (row.identity.familyId !== identity.familyId || row.identity.namespace !== identity.namespace || row.identity.storeGeneration !== identity.storeGeneration)) migrationFailure("MIGRATION_ORIGINAL_SESSION_REQUIRED");
  }
  private binding() { const i = this.context.identity; return { scopeId: i.scopeId, principalId: i.userId, sourceContextId: "personal" as const, deviceId: i.deviceId, restoreEpoch: i.epoch }; }
  async physical() {
    this.check(); const source = await migrationSyncSource(this.context.identity.namespace, this.binding(), this.context.isCurrent); this.check();
    if (source.storeGeneration !== this.context.identity.storeGeneration) migrationFailure("MIGRATION_STORE_CHANGED"); return source;
  }
  async rows() { this.check(); const rows = await migrationIntents(this.context); this.check(); return rows; }
  async capability() {
    const raw = syncRecord(await this.request("/sync/migration-capabilities"));
    if (raw.enabled !== true || raw.format !== migrationFormat || raw.sourceContextId !== "personal" || raw.qualifiedAck !== false || raw.targetQualified !== false || raw.maxMessages !== 256 || raw.maxObjects !== 64 || raw.maxRequestBytes !== migrationLimit || raw.maxObjectBytes !== 64 * 1024 * 1024 || raw.maxLifetimeSeconds !== 600 || raw.pairingLifetimeSeconds !== 300 || raw.installLifetimeSeconds !== 5) migrationFailure("MIGRATION_UNAVAILABLE");
    return true;
  }
  private async locked<T>(operation: () => Promise<T>) {
    this.check(); if (!navigator.locks) migrationFailure("MIGRATION_LOCK_UNAVAILABLE");
    return navigator.locks.request("jobsim-migration|" + this.context.identity.namespace, { mode: "exclusive", signal: this.context.signal }, async () => { this.check(); return operation(); });
  }
  private async original(id: string) { const row = (await this.rows()).find(item => item.requestId === id); if (!row) migrationFailure("MIGRATION_ORIGINAL_MISSING"); return row; }
  private async remember(row: MigrationIntent, value: MigrationView) {
    if (row.kind === "create") {
      const refs = value.manifest.messages;
      if (refs.length !== row.selection.length || refs.some((ref, index) => ref.messageId !== row.selection[index].id || ref.contentVersion !== row.selection[index].contentVersion || ref.objectMode !== row.selection[index].objectMode || ref.objectId !== row.selection[index].attachmentId) || row.target && (value.manifest.targetDeviceId !== row.target.deviceId || value.manifest.targetStoreGeneration !== row.target.storeGeneration)) migrationFailure("MIGRATION_ORIGINAL_CHANGED");
    }
    await updateMigration(this.context, row, { migrationId: value.id, state: ["revoking", "retired"].includes(value.status) ? "blocked" : value.status === "building" ? "unknown" : row.state === "installed" ? "installed" : "ready" });
    if (["revoking", "retired"].includes(value.status)) await discardMigrationStage(this.context, value.id); return value;
  }
  async checkOriginal(id: string): Promise<MigrationPairing | MigrationView | undefined> { return this.locked(async () => this.lookup(await this.original(id))); }
  private async lookup(row: MigrationIntent): Promise<MigrationPairing | MigrationView | undefined> {
    if (row.kind === "target") { if (!row.migrationId) return undefined; return this.remember(row, await this.status(row.migrationId)); }
    const raw = syncRecord(await this.request(row.kind === "pairing" ? "/sync/migration-pairing-results" : "/sync/migration-results", { method: "POST", body: JSON.stringify({ requestId: row.requestId, [row.kind === "pairing" ? "storeGeneration" : "sourceStoreGeneration"]: row.identity.storeGeneration }) }));
    if (raw.status === "not_found" && Object.keys(raw).length === 1) return undefined;
    if (row.kind === "pairing") {
      const value = this.parsePairing(raw, row); await updateMigration(this.context, row, { state: "ready", target: { pairingId: value.pairingId, deviceId: value.targetDeviceId, storeGeneration: value.targetStoreGeneration } }); return value;
    }
    return this.remember(row, await migrationView(raw, row.identity, { requestId: row.requestId, sourceStoreGeneration: row.identity.storeGeneration }));
  }
  private parsePairing(raw: unknown, row: MigrationIntent): MigrationPairing {
    const v = syncRecord(raw); if (typeof v.pairingProof !== "string" || v.sourcePreserved !== true) migrationFailure();
    const p = pairingProof(v.pairingProof, this.context), expiresAt = migrationTime(v.expiresAt);
    if (v.pairingId !== p.pairingId || v.targetDeviceId !== p.targetDeviceId || v.targetStoreGeneration !== p.targetStoreGeneration || v.restoreEpoch !== p.restoreEpoch || v.targetDeviceId !== row.identity.deviceId || v.targetStoreGeneration !== row.identity.storeGeneration || Date.parse(expiresAt) !== Number(p.expires) * 1000 || row.target && row.target.pairingId !== v.pairingId) migrationFailure("MIGRATION_ORIGINAL_CHANGED");
    return { pairingId: v.pairingId as string, pairingProof: v.pairingProof, targetDeviceId: v.targetDeviceId as string, targetStoreGeneration: v.targetStoreGeneration as string, restoreEpoch: v.restoreEpoch as string, expiresAt };
  }
  async pair(password: string, otp = "", originalId?: string, retry = true): Promise<MigrationPairing | undefined> {
    return this.locked(async () => {
      let row = originalId ? await this.original(originalId) : await stageMigration(this.context, "pairing");
      if (row.kind !== "pairing") migrationFailure(); const found = await this.lookup(row); if (found) return found as MigrationPairing; if (!retry) return undefined;
      this.check(row, true); await this.capability(); await this.physical();
      const proof = passwordProof(password, otp); row = await updateMigration(this.context, row, { attempt: true }); this.check(row, true);
      const value = this.parsePairing(await this.request("/sync/migration-pairings", { method: "POST", body: JSON.stringify({ requestId: row.requestId, storeGeneration: row.identity.storeGeneration, restoreEpoch: row.identity.epoch, ...proof }) }), row);
      await updateMigration(this.context, row, { state: "ready", target: { pairingId: value.pairingId, deviceId: value.targetDeviceId, storeGeneration: value.targetStoreGeneration } }); return value;
    });
  }
  async create(id: string, targetPairingProof: string, password: string, otp = "", retry = true): Promise<MigrationView | undefined> {
    return this.locked(async () => {
      let row = await this.original(id); if (row.kind !== "create") migrationFailure(); const found = await this.lookup(row); if (found) return found as MigrationView; if (!retry) return undefined;
      this.check(row, true); if (row.state === "blocked") migrationFailure("MIGRATION_ORIGINAL_CHANGED"); await this.capability();
      const target = pairingProof(targetPairingProof, this.context);
      if (target.targetDeviceId === row.identity.deviceId || Number(target.expires) * 1000 <= Date.now() || Number(target.expires) * 1000 > Date.now() + 301000) migrationFailure("MIGRATION_PAIRING_EXPIRED");
      row = await updateMigration(this.context, row, { target: { pairingId: target.pairingId as string, deviceId: target.targetDeviceId as string, storeGeneration: target.targetStoreGeneration as string } });
      const source = await this.sourceData(row); row = source.row;
      const body = JSON.stringify({ requestId: row.requestId, sourceContextId: "personal", sourceStoreGeneration: row.identity.storeGeneration, restoreEpoch: row.identity.epoch, targetPairingProof, ...passwordProof(password, otp), ...source.data });
      if (new TextEncoder().encode(body).length > migrationLimit) migrationFailure("MIGRATION_SOURCE_LIMIT");
      row = await updateMigration(this.context, row, { attempt: true }); this.check(row, true);
      const view = await migrationView(await this.request("/sync/migrations", { method: "POST", body }), row.identity, { requestId: row.requestId, sourceStoreGeneration: row.identity.storeGeneration }); return this.remember(row, view);
    });
  }
  async status(id: string): Promise<MigrationView> { return migrationView(await this.request("/sync/migrations/" + syncID(id)), this.context.identity); }
  private target(view: MigrationView) { const i = this.context.identity; if (view.manifest.targetDeviceId !== i.deviceId || view.manifest.targetStoreGeneration !== i.storeGeneration || !["ready", "installed_reported"].includes(view.status)) migrationFailure("MIGRATION_TARGET_CHANGED"); if (Date.parse(view.expiresAt) <= Date.now()) migrationFailure("MIGRATION_EXPIRED"); }
  async observeTarget(id: string): Promise<MigrationView> {
    const view = await this.status(id); this.target(view); let row = (await this.rows()).find(item => item.kind === "target" && item.migrationId === view.id && item.identity.namespace === this.context.identity.namespace && item.identity.storeGeneration === this.context.identity.storeGeneration);
    if (!row) row = await stageMigration(this.context, "target"); await this.remember(row, view); return view;
  }
  async download(id: string): Promise<Uint8Array<ArrayBuffer>> {
    const view = await this.status(id); this.target(view);
    const bytes = await (this.context.binary ?? apiBoundedBytes)("/sync/migrations/" + view.id + "/capsule", Number(view.sizeBytes), { headers: { "X-JobsIM-Store-Generation": this.context.identity.storeGeneration }, signal: this.context.signal }, 60000); this.check();
    if (bytes.length !== Number(view.sizeBytes) || await migrationHash(bytes) !== view.capsuleSHA256) migrationFailure("MIGRATION_CAPSULE_INVALID"); return bytes;
  }
  history() { this.check(); return migrationHistory(this.context); }
  async install(id: string, password: string, otp = "", file?: File): Promise<{ view: MigrationView; reported: boolean }> {
    return this.locked(async () => {
      await this.capability(); const view = await this.observeTarget(id); this.target(view);
      let capsule: Uint8Array<ArrayBuffer>;
      if (file) { if (file.size !== Number(view.sizeBytes) || file.size > migrationLimit) migrationFailure("MIGRATION_CAPSULE_INVALID"); capsule = new Uint8Array(await file.arrayBuffer()); this.check(); }
      else capsule = await stagedMigrationCapsule(this.context, view) ?? await this.download(view.id);
      if (capsule.length !== Number(view.sizeBytes) || await migrationHash(capsule) !== view.capsuleSHA256) migrationFailure("MIGRATION_CAPSULE_INVALID");
      await stageMigrationCapsule(this.context, view, capsule); this.check();
      const key = await this.request("/sync/migrations/" + view.id + "/key", { method: "POST", body: JSON.stringify({ manifestDigest: view.manifestDigest, ...passwordProof(password, otp) }) });
      const history = await openMigration(view, capsule, key, this.context.identity); this.check();
      const physical = await this.physical(), validation = syncRecord(await this.request("/sync/migrations/" + view.id + "/validate", { method: "POST", body: JSON.stringify({ manifestDigest: view.manifestDigest }) }));
      if (validation.id !== view.id || validation.status !== "validated" || validation.manifestDigest !== view.manifestDigest || validation.requiresAtomicLocalCommit !== true || validation.targetQualified !== false || validation.qualifiedAck !== false || typeof validation.installToken !== "string") migrationFailure();
      const token = validation.installToken.split("."); if (token.length !== 3 || token[0] !== "mi1" || migrationDecode(token[2], 32, true).length !== 32) migrationFailure();
      const p = syncRecord(JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(migrationDecode(token[1], 8192, true)))), i = this.context.identity;
      if (p.id !== view.id || p.manifestDigest !== view.manifestDigest || p.userId !== i.userId || p.deviceId !== i.deviceId || p.storeGeneration !== i.storeGeneration || p.restoreEpoch !== i.epoch || !Number.isSafeInteger(p.expires) || Number(p.expires) * 1000 !== Date.parse(migrationTime(validation.installBefore))) migrationFailure("MIGRATION_TARGET_CHANGED");
      await publishMigration(this.context, view, history, physical.guardGeneration, validation.installBefore as string); this.check();
      const row = (await this.rows()).find(item => item.kind === "target" && item.migrationId === view.id && item.identity.storeGeneration === i.storeGeneration); if (row) await updateMigration(this.context, row, { state: "installed" });
      try {
        const result = syncRecord(await this.request("/sync/migrations/" + view.id + "/installed", { method: "POST", body: JSON.stringify({ manifestDigest: view.manifestDigest, installToken: validation.installToken }) }));
        if (result.id !== view.id || result.manifestDigest !== view.manifestDigest || result.status !== "installed_reported" || result.sourcePreserved !== true || result.targetQualified !== false || result.qualifiedAck !== false || result.serverVerifiedLocalDurability !== false) migrationFailure();
        return { view, reported: true };
      } catch { this.check(); return { view, reported: false }; }
    });
  }
  async cancel(id: string) { return this.locked(async () => {
    const view = await this.status(id), result = await migrationView(await this.request("/sync/migrations/" + view.id + "/cancel", { method: "POST", body: JSON.stringify({ manifestDigest: view.manifestDigest }) }), this.context.identity);
    if (["revoking", "retired"].includes(result.status)) await discardMigrationStage(this.context, view.id);
    for (const row of (await this.rows()).filter(item => item.migrationId === id)) await this.remember(row, result); return result;
  }); }
  async select(selection: MigrationSelection[]) {
    const source = await this.physical();
    for (const selected of selection) {
      const message = source.messages.find(row => row.id === selected.id);
      if (!message || JSON.stringify(migrationSelect(message, selected.objectMode === "full")) !== JSON.stringify(selected)) migrationFailure("MIGRATION_SOURCE_CHANGED");
    }
    this.check(); return stageMigration(this.context, "create", selection);
  }
  protected async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    this.check(); const headers = new Headers(options.headers); headers.set("X-JobsIM-Store-Generation", this.context.identity.storeGeneration);
    const result = await (this.context.request ?? api)<T>(path, { ...options, headers, signal: this.context.signal }, 30000); this.check(); return result;
  }
  async sourceData(original: MigrationIntent): Promise<{ row: MigrationIntent; data: MigrationSourceData }> {
    this.check(original, true); if (original.kind !== "create") migrationFailure(); await this.physical();
    const data: MigrationSourceData = { messages: [], objects: [] }; let encodedBytes = 0;
    for (const selected of original.selection) {
      this.check(original, true);
      const raw = await this.request<Message>("/messages/" + selected.id), envelope = migrationEnvelope(raw, selected);
      data.messages.push(envelope); encodedBytes += envelope.bodyEnvelope.length;
      if (selected.objectMode === "full") {
        const path = raw.forwarded ? "/messages/" + selected.id + "/attachment" : "/media/" + selected.attachmentId;
        const bytes = await (this.context.binary ?? apiBoundedBytes)(path, Math.min(64 * 1024 * 1024, Math.floor((migrationLimit - encodedBytes - 32768) * 3 / 4)), { headers: { "X-JobsIM-Store-Generation": original.identity.storeGeneration }, signal: this.context.signal }, 60000);
        this.check(original, true); const encoded = migrationBase64(bytes); encodedBytes += encoded.length;
        data.objects.push({ objectId: selected.attachmentId!, messageId: selected.id, bytes: encoded });
      }
      if (encodedBytes > migrationLimit - 32768) migrationFailure("MIGRATION_SOURCE_LIMIT");
    }
    const body = migrationSourceBody(data), digest = await migrationHash(new TextEncoder().encode(body)); this.check(original, true);
    const row = await updateMigration(this.context, original, { sourceDigest: digest }); this.check(original, true); return { row, data };
  }
}

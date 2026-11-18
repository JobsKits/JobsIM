// Created by Jobs on 2026年11月17日，星期二.
import { authSnapshot } from "./AuthSession";
import { api, apiBoundedBytes } from "./api";
import { migrationSyncSource } from "./SyncStore";
import { migrationBase64, migrationEnvelope, migrationFailure, migrationHash, migrationLimit, migrationSelect, migrationSourceBody, type MigrationSelection, type MigrationSourceData } from "./MigrationProtocol";
import { migrationIntents, stageMigration, updateMigration, type MigrationIntent, type MigrationStoreContext } from "./MigrationStore";
import type { Message } from "./types";

export type MigrationContext = MigrationStoreContext & { request?: typeof api; binary?: typeof apiBoundedBytes; signal?: AbortSignal };
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
  async rows() { this.check(); return migrationIntents(this.context); }
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

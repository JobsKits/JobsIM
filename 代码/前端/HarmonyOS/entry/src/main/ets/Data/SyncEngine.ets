// Created by Jobs on 2026年11月16日，星期一.
import { ApiFailure, Bootstrap } from './Models';
import { SyncIdentity, SyncControl, SyncManifest, SyncPage, SyncProjection, SyncDescriptor, SyncProof, SyncOriginal, SyncCommit, SyncCodec, SyncJSON, SyncProtocol, SyncRules } from './SyncProtocol';

export interface SyncStorePort {
  syncControl: (identity: SyncIdentity) => Promise<SyncControl | undefined>;
  syncBegin: (identity: SyncIdentity, uuid: () => string) => Promise<SyncControl>;
  syncSelectSchema: (expected: SyncControl, schema: string) => Promise<SyncControl>;
  syncUpdate: (expected: SyncControl, next: SyncControl, page?: SyncPage) => Promise<SyncControl>;
  syncRestrict: (identity: SyncIdentity, reason: string, reset: boolean) => Promise<void>;
  syncOriginals: (identity: SyncIdentity) => Promise<SyncOriginal[]>;
  syncInstall: (commit: SyncCommit, current: () => boolean) => Promise<void>;
  syncConfirm: (identity: SyncIdentity, guard: string, proof: SyncProof, current: () => boolean) => Promise<void>;
}
export interface SyncEnginePorts {
  store: SyncStorePort;
  codec: SyncCodec;
  uuid: () => string;
  current: () => boolean;
  request: (path: string, method: string, body: string) => Promise<string>;
  cleanup: () => Promise<void>;
  committed?: (messages:string[])=>void;
  historyChanged?: ()=>void;
}

export class SyncEngine {
  hasMore: boolean = false;
  private identity: SyncIdentity;
  private ports: SyncEnginePorts;
  private protocol: SyncProtocol;
  private work?: Promise<void>;
  private cancelled: boolean = false;
  constructor(identity: SyncIdentity, ports: SyncEnginePorts) {
    SyncRules.identity(identity); this.identity = identity; this.ports = ports; this.protocol = new SyncProtocol(ports.codec);
  }
  cancel(): void { this.cancelled = true; }
  private current(): boolean { return !this.cancelled && this.ports.current(); }
  private check(): void { if (!this.current()) { throw new Error('SYNC_CONTEXT_CHANGED'); } }
  private async request(path: string, method: string = 'GET', body: string = ''): Promise<string> {
    this.check(); const result = await this.ports.request(path, method, body); this.check(); return result;
  }
  async run(): Promise<void> {
    if (this.work) { return this.work; }
    const work = this.round(); this.work = work;
    try { await work; } finally { if (this.work === work) { this.work = undefined; } }
  }
  private async proofs(): Promise<SyncProof[]> {
    const result: SyncProof[] = [];
    for (const original of await this.ports.store.syncOriginals(this.identity)) {
      try {
        const raw = await this.request(original.route, 'POST', original.body);
        const row = SyncRules.message(raw);
        if (row.clientMsgId === original.id && row.conversationId === original.conversation && row.senderId === original.sender && row.senderId === this.identity.user) { result.push({ original: original, result: raw }); }
      } catch (error) {
        this.check();
        if (error instanceof ApiFailure && (error.status === 401 || ['RESTORE_EPOCH_CHANGED', 'STORE_GENERATION_CHANGED', 'DEPLOYMENT_CHANGED'].includes(error.code))) { throw error; }
        // Auxiliary unknown/bad 200 is zero proof, never a veto on an independent verified global.
      }
    }
    return result;
  }
  async resolve(id: string): Promise<boolean> {
    const control = await this.ports.store.syncControl(this.identity);
    if (!control) { throw new Error('SYNC_REGISTRATION_REQUIRED'); }
    const original = (await this.ports.store.syncOriginals(this.identity)).find((row: SyncOriginal) => row.id === id);
    if (!original) { return false; }
    const raw = await this.request(original.route, 'POST', original.body);
    const row = SyncRules.message(raw);
    if (row.clientMsgId !== original.id || row.conversationId !== original.conversation || row.senderId !== this.identity.user) { throw new Error('SYNC_OUTBOX_RESULT_INVALID'); }
    await this.ports.store.syncConfirm(this.identity, control.guard, { original: original, result: raw }, () => this.current());
    await this.ports.cleanup(); return true;
  }
  private async round(): Promise<void> {
    let responsible = false;
    try {
      this.check(); await this.ports.cleanup();
      const existing = await this.ports.store.syncControl(this.identity); responsible = existing !== undefined;
      const boot = JSON.parse(await this.request('/bootstrap')) as Bootstrap;
      if (boot.deploymentId !== this.identity.deployment || boot.restoreEpoch !== this.identity.epoch) { throw new Error('RESTORE_EPOCH_CHANGED'); }
      if (boot.features?.authoritative_sync !== true || boot.features?.sync_jetstream !== true) { throw new Error('SYNC_CAPABILITY_REQUIRED'); }
      let control = await this.ports.store.syncBegin(this.identity, this.ports.uuid); responsible = true;
      if (boot.features?.sync_self_state === true && control.schema !== '2') { control = await this.ports.store.syncSelectSchema(control, '2'); }
      if (control.schema === '2' && boot.features?.sync_self_state !== true) { throw new Error('SYNC_CAPABILITY_REQUIRED'); }
      const registrationBody: Record<string, string> = JSON.parse(JSON.stringify({ requestId: control.registration, storeGeneration: control.physical })) as Record<string, string>;
      if (control.schema === '2') { registrationBody['schemaVersion'] = '2'; }
      const registration = SyncJSON.parse(await this.request('/sync/device', 'PUT', JSON.stringify(registrationBody)));
      this.protocol.binding(registration, control, false);
      if (registration.get('qualification').string() !== 'transport_only' || registration.get('qualifiedAck').bool()) { throw new Error('SYNC_INVALID_REGISTRATION'); }
      if (control.schema === '2' && registration.get('schemaVersion').string() !== '2') { throw new Error('SYNC_SCHEMA_MISMATCH'); }
      if (control.stream && control.stream !== registration.get('streamId').string()) { throw new Error('SYNC_CONTEXT_CHANGED'); }
      let next = JSON.parse(JSON.stringify(control)) as SyncControl;
      next.stream = registration.get('streamId').string(); control = await this.ports.store.syncUpdate(control, next);
      const context = SyncJSON.parse(await this.request('/sync/context' + (control.schema === '2' ? '?schemaVersion=2' : '')));
      this.protocol.binding(context, control, true);
      if (context.get('qualification').string() !== 'transport_only' || context.get('qualifiedAck').bool() || context.get('afterReceiptEnabled').bool()) { throw new Error('SYNC_INVALID_CONTEXT'); }
      if (control.schema === '2' && context.get('coverage').get('schemaVersion').string() !== '2') { throw new Error('SYNC_SCHEMA_MISMATCH'); }
      SyncRules.counter(context.get('head').string());
      const permission = context.get('permissionGeneration').string();
      if (!/^[0-9a-f]{64}$/.test(permission)) { throw new Error('SYNC_INVALID_PERMISSION'); }
      if (control.permission && control.permission !== permission) {
        await this.ports.store.syncRestrict(this.identity, 'SYNC_PERMISSION_CHANGED', true);
        const updated = await this.ports.store.syncControl(this.identity);
        if (!updated) { throw new Error('SYNC_STORE_INVALID'); } control = updated;
      }
      let manifest: SyncManifest | undefined = undefined;
      const pages: SyncPage[] = [];
      let projections: SyncProjection[] = [];
      let classification = control.classification;
      let deltaRaw = '';
      if (control.cursor && !control.restriction && control.permission === permission) {
        deltaRaw = await this.request('/sync/changes?limit=500&cursor=' + encodeURIComponent(control.cursor) + (control.schema === '2' ? '&schemaVersion=2' : ''));
      } else {
        if (!control.snapshotRequest) {
          next = JSON.parse(JSON.stringify(control)) as SyncControl;
          next.snapshotRequest = JSON.stringify({ requestId: this.ports.uuid(), storeGeneration: control.physical, schemaVersion: control.schema ?? '1', pageSize: 100 });
          control = await this.ports.store.syncUpdate(control, next);
        }
        const raw = await this.request('/sync/snapshots', 'POST', control.snapshotRequest);
        manifest = this.protocol.manifest(raw, control);
        if (manifest.permission !== permission) { throw new Error('SYNC_PERMISSION_CHANGED'); }
        if (control.manifest && !SyncRules.same(control.manifest, raw)) { throw new Error('SYNC_MANIFEST_CHANGED'); }
        next = JSON.parse(JSON.stringify(control)) as SyncControl; next.manifest = raw;
        control = await this.ports.store.syncUpdate(control, next);
        const references = new Set<string>(); let bodyBytes = 0;
        for (let index = 0; index < manifest.descriptors.length; index++) {
          const page = await this.protocol.page(await this.request('/sync/snapshots/' + manifest.id + '/pages/' + index), manifest, index, this.identity.epoch);
          for (const projection of page.projections) {
            const key = projection.ref.owner + ':' + projection.ref.kind + ':' + projection.ref.id;
            if (references.has(key)) { throw new Error('SYNC_DUPLICATE_REFERENCE'); } references.add(key);
            bodyBytes += this.ports.codec.bytes(projection.object).length;
            if (bodyBytes > 67108864) { throw new Error('SYNC_LOCAL_CAPACITY'); }
          }
          pages.push(page); projections = projections.concat(page.projections);
          control = await this.ports.store.syncUpdate(control, control, page);
        }
        classification = this.protocol.validation(await this.request('/sync/snapshots/' + manifest.id + '/validate', 'POST', JSON.stringify({ pageDigests: manifest.descriptors.map((row: SyncDescriptor) => row.digest) })), manifest, this.identity.epoch);
        // First permission delta precedes publication in the local transaction.
        deltaRaw = await this.request('/sync/changes?limit=500&cursor=' + encodeURIComponent(manifest.token) + (control.schema === '2' ? '&schemaVersion=2' : ''));
      }
      const delta = await this.protocol.delta(deltaRaw, manifest ? manifest.boundary : control.position, manifest ? manifest.permission : permission, this.identity.epoch, control.schema ?? '1');
      projections = projections.concat(delta.projections);
      next = JSON.parse(JSON.stringify(control)) as SyncControl;
      next.cursor = delta.cursor; next.position = delta.position; next.permission = permission; next.classification = classification;
      next.restriction = ''; next.snapshotRequest = ''; next.manifest = ''; next.guard = this.ports.uuid();
      const proofs = await this.proofs(); this.check(); if (manifest) { this.protocol.alive(manifest); }
      await this.ports.store.syncInstall({ expected: control, next: next, manifest: manifest, pages: pages, projections: projections, events: delta.events, proofs: proofs }, () => this.current());
      this.check();this.hasMore = delta.more;
      const changed=pages.reduce((all:SyncProjection[],page:SyncPage)=>all.concat(page.projections),[] as SyncProjection[]).concat(projections);
      if(changed.some((row:SyncProjection)=>row.ref.kind==='message')){try{this.ports.historyChanged?.();}catch(_){}}
      await this.ports.cleanup();
      this.check();const installed:SyncProjection[]=pages.reduce((all:SyncProjection[],page:SyncPage)=>all.concat(page.projections),[] as SyncProjection[]).concat(projections);try{this.ports.committed?.(installed.filter((row:SyncProjection)=>row.ref.kind==='message'&&row.state==='available'&&Boolean(row.object)).map((row:SyncProjection)=>row.object));}catch(_){}

    } catch (error) {
      if (responsible && this.current()) {
        const reason = error instanceof ApiFailure ? error.code : (error as Error).message;
        await this.ports.store.syncRestrict(this.identity, reason, ['SNAPSHOT_EXPIRED', 'SNAPSHOT_INVALIDATED', 'SYNC_SNAPSHOT_EXPIRED', 'SYNC_PERMISSION_CHANGED'].includes(reason));
      }
      throw error as Error;
    }
  }
}

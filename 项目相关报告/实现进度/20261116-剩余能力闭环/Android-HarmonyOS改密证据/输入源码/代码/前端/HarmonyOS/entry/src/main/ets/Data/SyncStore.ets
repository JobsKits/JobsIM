// Created by Jobs on 2026年11月16日，星期一.
import relationalStore from '@ohos.data.relationalStore';
import { LocalStore } from './Store';
import { SyncStorePort } from './SyncEngine';
import { SelfRules } from './SelfProtocol';
import { Message, Rules } from './Models';
import { SyncCodec, SyncIdentity, SyncControl, SyncPage, SyncProjection, SyncOriginal, SyncProof, SyncCommit, SyncCleanup, SyncJSON, SyncRules } from './SyncProtocol';

export class SyncStore implements SyncStorePort {
  private store: LocalStore;
  private codec: SyncCodec;
  private uuid: () => string;
  constructor(store: LocalStore, codec: SyncCodec, uuid: () => string) {
    this.store = store; this.codec = codec; this.uuid = uuid;
    store.setSyncHooks({
      put: (db: relationalStore.RdbStore, ns: string, row: Message) => this.beforePut(db, ns, row),
      remove: async (db: relationalStore.RdbStore, ns: string, id: string) => { await db.executeSql("UPDATE sync_originals SET state='discarded' WHERE namespace=? AND id=? AND state!='confirmed'", [ns, id]); },
      freeze: (db: relationalStore.RdbStore, ns: string, reason: string) => this.restrictNow(db, ns, reason, false),
      cloud: async (db: relationalStore.RdbStore, ns: string) => { const control = await this.controlNow(db, ns); return control !== undefined && !control.restriction; }
    });
  }
  private nonce(): string { const result = this.uuid(); if (!SyncRules.uuid(result)) { throw new Error('SYNC_INVALID_UUID'); } return result; }
  private async hash(value: string): Promise<string> { return this.codec.sha256(this.codec.bytes(value)); }
  private async values(db: relationalStore.RdbStore, sql: string, args: relationalStore.ValueType[] = []): Promise<string[]> {
    const result = await db.querySql(sql, args); const rows: string[] = [];
    try { while (result.goToNextRow()) { rows.push(result.getString(0)); } } finally { result.close(); } return rows;
  }
  private async value(db: relationalStore.RdbStore, ns: string, key: string): Promise<string> { return (await this.values(db, 'SELECT value FROM local_state WHERE namespace=? AND key=?', [ns, key]))[0] ?? ''; }
  private async set(db: relationalStore.RdbStore, ns: string, key: string, value: string): Promise<void> { await db.executeSql('INSERT OR REPLACE INTO local_state VALUES(?,?,?)', [ns, key, value]); }
  private async transaction(action: (db: relationalStore.RdbStore) => Promise<void>): Promise<void> {
    await this.store.syncAccess(async (db: relationalStore.RdbStore) => {
      db.beginTransaction();
      try { await action(db); db.commit(); } catch (error) { db.rollBack(); throw error as Error; }
    });
  }
  async open(): Promise<void> {
    await this.transaction(async (db: relationalStore.RdbStore) => {
      for (const sql of [
        'CREATE TABLE IF NOT EXISTS sync_stages(namespace TEXT,snapshot TEXT,page INTEGER,payload TEXT,PRIMARY KEY(namespace,snapshot,page))',
        'CREATE TABLE IF NOT EXISTS sync_refs(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_objects(namespace TEXT,id TEXT,kind TEXT,payload TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_events(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_terminal(scope TEXT,id TEXT,PRIMARY KEY(scope,id))',
        'CREATE TABLE IF NOT EXISTS sync_retained(namespace TEXT,id TEXT,conversation TEXT,payload TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_retained_conversations(namespace TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_eligibility(namespace TEXT,id TEXT,version TEXT,hash TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_quarantine(namespace TEXT,key TEXT,payload TEXT,PRIMARY KEY(namespace,key))',
        'CREATE TABLE IF NOT EXISTS sync_unread(namespace TEXT,id TEXT,conversation TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_stream_archive(namespace TEXT,stream TEXT,kind TEXT,id TEXT,payload TEXT,PRIMARY KEY(namespace,stream,kind,id))',
        'CREATE TABLE IF NOT EXISTS self_operations(namespace TEXT,id TEXT,kind TEXT,conversation TEXT,state TEXT,payload TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_originals(namespace TEXT,id TEXT,conversation TEXT,sender TEXT,route TEXT,body TEXT,hash TEXT,state TEXT,serverid TEXT,PRIMARY KEY(namespace,id))',
        'CREATE TABLE IF NOT EXISTS sync_cleanup(namespace TEXT,id TEXT,key TEXT,path TEXT,PRIMARY KEY(namespace,id,key))'
      ]) { await db.executeSql(sql); }
      const physical = await this.value(db, '__sync_store__', 'physical');
      if (!physical) {
        if ((await this.values(db, "SELECT value FROM local_state WHERE key='sync:control' OR key LIKE 'registered:%'")).length > 0) { throw new Error('SYNC_STORE_INVALID'); }
        await this.set(db, '__sync_store__', 'physical', this.nonce());
      } else if (!SyncRules.uuid(physical)) { throw new Error('SYNC_STORE_INVALID'); }
      // Legacy lookup copies have no authority proof. Keep ordinary bytes isolated, never serve them.
      const legacy = await db.querySql("SELECT namespace,key,value FROM local_state WHERE key LIKE 'lookup:%'");
      const copies: string[][] = [];
      try { while (legacy.goToNextRow()) { copies.push([legacy.getString(0), legacy.getString(1), legacy.getString(2)]); } } finally { legacy.close(); }
      for (const copy of copies) {
        let raw = copy[2];
        try { const page = SyncJSON.parse(raw); if (page.has('items')) { const object = JSON.parse(raw) as Record<string, Object>; object['items'] = page.get('items').list().map((item: SyncJSON) => SyncRules.persist(SyncRules.message(item.raw()))); raw = JSON.stringify(object); } } catch (_) { /* Unknown legacy bytes remain quarantined, never promoted. */ }
        await db.executeSql('INSERT OR IGNORE INTO sync_quarantine VALUES(?,?,?)', [copy[0], copy[1], raw]);
        await db.executeSql('DELETE FROM local_state WHERE namespace=? AND key=?', [copy[0], copy[1]]);
      }
    });
  }
  private async controlNow(db: relationalStore.RdbStore, namespace: string): Promise<SyncControl | undefined> {
    const raw = await this.value(db, namespace, 'sync:control');
    if (!raw) {
      if (await this.value(db, '__sync_store__', 'registered:' + namespace)) { throw new Error('SYNC_STORE_INVALID'); } return undefined;
    }
    const result = JSON.parse(raw) as SyncControl; SyncRules.identity(result.identity);
    if (result.identity.namespace !== namespace || result.physical !== await this.value(db, '__sync_store__', 'physical') || !SyncRules.uuid(result.guard) || !SyncRules.uuid(result.registration) || (result.schema !== undefined && result.schema !== '1' && result.schema !== '2')) { throw new Error('SYNC_STORE_INVALID'); }
    SyncRules.counter(result.position); SyncRules.counter(result.classification); return result;
  }
  private async matched(db: relationalStore.RdbStore, identity: SyncIdentity): Promise<SyncControl | undefined> {
    const control = await this.controlNow(db, identity.namespace);
    if (control && !SyncRules.same(JSON.stringify(control.identity), JSON.stringify(identity))) { throw new Error('SYNC_CONTEXT_CHANGED'); } return control;
  }
  async syncControl(identity: SyncIdentity): Promise<SyncControl | undefined> { return this.store.syncAccess((db: relationalStore.RdbStore) => this.matched(db, identity)); }
  async syncBegin(identity: SyncIdentity, _uuid: () => string): Promise<SyncControl> {
    let control: SyncControl | undefined = undefined;
    await this.transaction(async (db: relationalStore.RdbStore) => {
      control = await this.matched(db, identity);
      if (!control) {
        SyncRules.identity(identity);
        control = { identity: identity, physical: await this.value(db, '__sync_store__', 'physical'), registration: this.nonce(), guard: this.nonce(), stream: '', position: '0', cursor: '', permission: '', classification: '0', restriction: 'SYNC_BASELINE_REQUIRED', snapshotRequest: '', manifest: '' };
        if (!SyncRules.uuid(control.physical)) { throw new Error('SYNC_STORE_INVALID'); }
        await this.archive(db, identity.namespace);
        await this.set(db, identity.namespace, 'sync:control', JSON.stringify(control));
        await this.set(db, '__sync_store__', 'registered:' + identity.namespace, '1');
      }
      for (const raw of await this.values(db, "SELECT payload FROM messages WHERE namespace=? AND state IN('pending','frozen')", [identity.namespace])) {
        const row = JSON.parse(raw) as Message;
        if (!(await this.values(db, 'SELECT id FROM sync_originals WHERE namespace=? AND id=?', [identity.namespace, row.clientMsgId])).length) { await this.originalNow(db, identity.namespace, row); }
      }
    });
    if (!control) { throw new Error('SYNC_STORE_INVALID'); } return control;
  }
  private async expected(db: relationalStore.RdbStore, control: SyncControl): Promise<void> {
    const actual = await this.matched(db, control.identity);
    if (!actual || !SyncRules.same(JSON.stringify(actual), JSON.stringify(control))) { throw new Error('SYNC_LOCAL_GUARD_CHANGED'); }
  }
  async syncSelectSchema(expected: SyncControl, schema: string): Promise<SyncControl> {
    if (schema !== '2') { throw new Error('SYNC_SCHEMA_DOWNGRADE_FORBIDDEN'); }
    if (expected.schema === '2') { return expected; }
    const next = JSON.parse(JSON.stringify(expected)) as SyncControl;
    await this.transaction(async (db: relationalStore.RdbStore) => {
      await this.expected(db, expected);
      const ns = expected.identity.namespace;
      await db.executeSql('INSERT INTO sync_stream_archive VALUES(?,?,?,?,?)', [ns, expected.stream, 'control', expected.schema ?? '1', JSON.stringify(expected)]);
      const entries: string[][] = [['sync_refs', 'id'], ['sync_events', 'id'], ['sync_objects', 'id'], ['sync_stages', "snapshot||':'||CAST(page AS TEXT)"]];
      for (const entry of entries) { await db.executeSql('INSERT INTO sync_stream_archive SELECT namespace,?,?,' + entry[1] + ',payload FROM ' + entry[0] + ' WHERE namespace=?', [expected.stream, entry[0], ns]); }
      await this.archive(db, ns);
      for (const table of ['sync_refs', 'sync_events', 'sync_objects', 'sync_stages']) { await db.executeSql('DELETE FROM ' + table + ' WHERE namespace=?', [ns]); }
      next.schema = '2'; next.registration = this.nonce(); next.guard = this.nonce(); next.stream = ''; next.position = '0'; next.cursor = ''; next.permission = ''; next.classification = '0'; next.restriction = 'SYNC_SCHEMA_BASELINE_REQUIRED'; next.snapshotRequest = ''; next.manifest = '';
      await this.set(db, ns, 'sync:control', JSON.stringify(next));
    }); return next;
  }
  async syncUpdate(expected: SyncControl, next: SyncControl, page?: SyncPage): Promise<SyncControl> {
    await this.transaction(async (db: relationalStore.RdbStore) => {
      await this.expected(db, expected);
      if ((expected.schema ?? '1') !== (next.schema ?? '1')) { throw new Error('SYNC_SCHEMA_TRANSITION_REQUIRED'); }
      if (!SyncRules.same(JSON.stringify(expected.identity), JSON.stringify(next.identity)) || expected.registration !== next.registration || expected.physical !== next.physical) { throw new Error('SYNC_CONTEXT_CHANGED'); }
      if (page) {
        const id = SyncJSON.parse(next.manifest).get('snapshotId').string();
        const prior = await this.values(db, 'SELECT payload FROM sync_stages WHERE namespace=? AND snapshot=? AND page=?', [next.identity.namespace, id, page.index]);
        if (prior.length && !SyncRules.same(prior[0], page.stage)) { throw new Error('SYNC_STAGE_CHANGED'); }
        await db.executeSql('INSERT OR REPLACE INTO sync_stages VALUES(?,?,?,?)', [next.identity.namespace, id, page.index, page.stage]);
      }
      await this.set(db, next.identity.namespace, 'sync:control', JSON.stringify(next));
    }); return next;
  }
  private async archive(db: relationalStore.RdbStore, ns: string): Promise<void> {
    for (const raw of await this.values(db, "SELECT payload FROM sync_objects WHERE namespace=? AND kind='conversation'", [ns])) {
      const row = JSON.parse(raw) as Record<string, Object | null>; row['sendAdmission'] = null; row['readOnly'] = true; row['state'] = 'local_only';
      await db.executeSql('INSERT OR REPLACE INTO sync_retained_conversations VALUES(?,?,?)', [ns, SyncJSON.parse(raw).get('id').string(), JSON.stringify(row)]);
    }
    for (const raw of await this.values(db, "SELECT payload FROM messages WHERE namespace=? AND state NOT IN('pending','frozen')", [ns])) {
      const row = JSON.parse(raw) as Message;
      if (!row.forwarded && row.type !== 'forward' && !row.deleted && !row.contentUnavailable) { await db.executeSql('INSERT OR REPLACE INTO sync_retained VALUES(?,?,?,?)', [ns, row.id, row.conversationId, raw]); }
    }
    await db.executeSql("DELETE FROM messages WHERE namespace=? AND state NOT IN('pending','frozen')", [ns]);
    await db.executeSql('DELETE FROM sync_objects WHERE namespace=?', [ns]);
  }
  private async restrictNow(db: relationalStore.RdbStore, ns: string, reason: string, reset: boolean): Promise<void> {
    const control = await this.controlNow(db, ns); if (!control) { return; }
    await this.archive(db, ns); control.restriction = reason; control.guard = this.nonce();
    if (reset) { control.snapshotRequest = ''; control.manifest = ''; }
    await this.set(db, ns, 'sync:control', JSON.stringify(control));
  }
  async syncRestrict(identity: SyncIdentity, reason: string, reset: boolean): Promise<void> { await this.transaction(async (db: relationalStore.RdbStore) => { await this.matched(db, identity); await this.restrictNow(db, identity.namespace, reason, reset); }); }
  private async originalHash(ns: string, original: SyncOriginal): Promise<string> { return this.hash(JSON.stringify([ns, original.sender, original.route, original.body])); }
  private async originalNow(db: relationalStore.RdbStore, ns: string, row: Message): Promise<void> {
    const original: SyncOriginal = { id: row.clientMsgId, conversation: row.conversationId, sender: row.senderId, route: '/conversations/' + encodeURIComponent(row.conversationId) + '/message-results', body: Rules.outgoingBody(row), hash: '' };
    original.hash = await this.originalHash(ns, original);
    const previous = await this.values(db, 'SELECT hash FROM sync_originals WHERE namespace=? AND id=?', [ns, original.id]);
    if (previous.length) { if (previous[0] !== original.hash) { throw new Error('SYNC_ORIGINAL_BODY_CHANGED'); } if (row.state === 'frozen') { await db.executeSql("UPDATE sync_originals SET state='frozen' WHERE namespace=? AND id=? AND state='pending'", [ns, original.id]); } return; }
    await db.executeSql('INSERT INTO sync_originals VALUES(?,?,?,?,?,?,?,?,?)', [ns, original.id, original.conversation, original.sender, original.route, original.body, original.hash, row.state ?? 'pending', '']);
  }
  async syncOriginals(identity: SyncIdentity): Promise<SyncOriginal[]> {
    return this.store.syncAccess(async (db: relationalStore.RdbStore) => {
      const result = await db.querySql("SELECT id,conversation,sender,route,body,hash FROM sync_originals WHERE namespace=? AND state IN('pending','frozen') ORDER BY id", [identity.namespace]); const rows: SyncOriginal[] = [];
      try { while (result.goToNextRow()) { rows.push({ id: result.getString(0), conversation: result.getString(1), sender: result.getString(2), route: result.getString(3), body: result.getString(4), hash: result.getString(5) }); } } finally { result.close(); }
      const valid: SyncOriginal[] = [];
      for (const row of rows) {
        try { if (await this.originalHash(identity.namespace, row) === row.hash && row.sender === identity.user && row.route === '/conversations/' + encodeURIComponent(row.conversation) + '/message-results' && SyncJSON.parse(row.body).get('clientMsgId').string() === row.id) { valid.push(row); } } catch (_) {}
      }
      // Corrupt auxiliary evidence is zero proof; retain its original queue bytes for explicit repair.
      return valid;
    });
  }
  private async terminal(db: relationalStore.RdbStore, scope: string, id: string): Promise<boolean> { return (await this.values(db, 'SELECT id FROM sync_terminal WHERE scope=? AND id=?', [scope, id])).length > 0; }
  private async global(db: relationalStore.RdbStore, identity: SyncIdentity, id: string): Promise<void> {
    const scope = SyncRules.scope(identity); await db.executeSql('INSERT OR IGNORE INTO sync_terminal VALUES(?,?)', [scope, id]);
    const holders: string[] = [identity.namespace];
    for (const raw of await this.values(db, "SELECT value FROM local_state WHERE key='sync:control'")) {
      const control = JSON.parse(raw) as SyncControl;
      if (SyncRules.scope(control.identity) === scope && !holders.includes(control.identity.namespace)) { holders.push(control.identity.namespace); }
    }
    for (const ns of holders) {
      const conversations = await this.values(db, 'SELECT conversation FROM messages WHERE namespace=? AND id=? UNION SELECT conversation FROM sync_retained WHERE namespace=? AND id=?', [ns, id, ns, id]);
      for (const table of ['sync_retained', 'sync_eligibility', 'mention_receipts', 'sync_unread']) { await db.executeSql('DELETE FROM ' + table + ' WHERE namespace=? AND id=?', [ns, id]); }
      const quarantine = await db.querySql('SELECT key,payload FROM sync_quarantine WHERE namespace=?', [ns]); const copies: string[][] = [];
      try { while (quarantine.goToNextRow()) { copies.push([quarantine.getString(0), quarantine.getString(1)]); } } finally { quarantine.close(); }
      for (const copy of copies) {
        if (!copy[1]) { continue; }
        try { const page = SyncJSON.parse(copy[1]); const object = JSON.parse(copy[1]) as Record<string, Object>;
          if (page.has('items')) { object['items'] = page.get('items').list().filter((item: SyncJSON) => item.get('id').string() !== id).map((item: SyncJSON) => item.value as Object); await db.executeSql('UPDATE sync_quarantine SET payload=? WHERE namespace=? AND key=?', [JSON.stringify(object), ns, copy[0]]); }
        } catch (_) { /* Unproven legacy structure is isolated from every display/export path. */ }
      }
      await db.executeSql("DELETE FROM messages WHERE namespace=? AND id=? AND state NOT IN('pending','frozen')", [ns, id]);
      await db.executeSql("UPDATE sync_originals SET body='' WHERE namespace=? AND state='confirmed' AND serverid=?", [ns, id]);
      for (const conversation of conversations) { await this.set(db, ns, 'unread:' + conversation, '0'); }
      for (const key of ['forward-sidecar:' + id, 'media-sidecar:' + id, '__notification__']) { await db.executeSql('INSERT OR IGNORE INTO sync_cleanup VALUES(?,?,?,?)', [ns, id, key, key === '__notification__' ? '' : await this.value(db, ns, key)]); }
    }
  }
  private async beforePut(db: relationalStore.RdbStore, ns: string, input: Message): Promise<Message | undefined> {
    if (input.state === 'pending' || input.state === 'frozen') { await this.originalNow(db, ns, input); return input; }
    SyncRules.message(JSON.stringify(input));
    const control = await this.controlNow(db, ns);
    if (control && (input.deleted || await this.terminal(db, SyncRules.scope(control.identity), input.id))) { await this.global(db, control.identity, input.id); control.guard = this.nonce(); await this.set(db, ns, 'sync:control', JSON.stringify(control)); return undefined; }
    const previous = await this.values(db, 'SELECT payload FROM messages WHERE namespace=? AND id=?', [ns, input.id]);
    if (previous.length) { const row = JSON.parse(previous[0]) as Message; if (row.deleted || SyncRules.compare(row.version, input.version) > 0) { return undefined; } }
    return SyncRules.persist(input);
  }
  private async apply(db: relationalStore.RdbStore, identity: SyncIdentity, projection: SyncProjection, baseline: boolean = false): Promise<void> {
    const control = await this.matched(db, identity);
    if (!control) { throw new Error('SYNC_STORE_INVALID'); }
    projection.object = SelfRules.projection(projection, identity, control.schema ?? '1');
    const ns = identity.namespace; const ref = projection.ref;
    if (ref.kind === 'message') {
      if (projection.state === 'global' || await this.terminal(db, SyncRules.scope(identity), ref.id)) { await this.global(db, identity, ref.id); return; }
      if (!projection.object || projection.state !== 'available') {
        for (const raw of await this.values(db, 'SELECT payload FROM messages WHERE namespace=? AND id=?', [ns, ref.id])) { const row = JSON.parse(raw) as Message; if (!row.forwarded && !row.deleted && row.state !== 'pending' && row.state !== 'frozen') { await db.executeSql('INSERT OR REPLACE INTO sync_retained VALUES(?,?,?,?)', [ns, row.id, row.conversationId, raw]); } }
        await db.executeSql("DELETE FROM messages WHERE namespace=? AND id=? AND state NOT IN('pending','frozen')", [ns, ref.id]); return;
      }
      const row = SyncRules.persist(SyncRules.message(projection.object));
      if (row.id !== ref.id || row.conversationId !== ref.conversationId) { throw new Error('SYNC_OBJECT_ID_MISMATCH'); }
      row.state = 'received'; const raw = JSON.stringify(row);
      const old = await this.values(db, 'SELECT payload FROM messages WHERE namespace=? AND id=?', [ns, row.id]);
      if (old.length && SyncRules.compare((JSON.parse(old[0]) as Message).version, row.version) > 0) { return; }
      await db.executeSql('INSERT OR REPLACE INTO messages VALUES(?,?,?,?,?,?)', [ns, row.conversationId, row.id, raw, 'received', row.createdAt]);
      const receipt = await this.values(db, 'SELECT CAST(unread AS TEXT) FROM mention_receipts WHERE namespace=? AND id=?', [ns, row.id]);
      await db.executeSql('INSERT OR REPLACE INTO mention_receipts VALUES(?,?,?,?)', [ns, row.conversationId, row.id, Rules.mentionUnread(receipt.length ? receipt[0] === '1' : undefined, row, identity.user, baseline) ? 1 : 0]);
      if (!baseline && !old.length && row.senderId !== identity.user && !row.deleted && !row.contentUnavailable) { await db.executeSql('INSERT OR IGNORE INTO sync_unread VALUES(?,?,?)', [ns, row.id, row.conversationId]); }
      if (!row.forwarded && !row.deleted && !row.contentUnavailable) { await db.executeSql('INSERT OR REPLACE INTO sync_eligibility VALUES(?,?,?,?)', [ns, row.id, row.version, await this.hash(raw)]); }
    } else {
      let raw = projection.object;
      if (ref.kind === 'announcement' && raw) { const object = JSON.parse(raw) as Record<string, Object | null>; object['message'] = null; raw = JSON.stringify(object); }
      if (projection.state === 'available' && raw) { await db.executeSql('INSERT OR REPLACE INTO sync_objects VALUES(?,?,?,?)', [ns, ref.owner + ':' + ref.kind + ':' + ref.id, ref.kind, raw]); }
      else { await db.executeSql('DELETE FROM sync_objects WHERE namespace=? AND id=?', [ns, ref.owner + ':' + ref.kind + ':' + ref.id]); }
    }
    await db.executeSql('INSERT OR REPLACE INTO sync_refs VALUES(?,?,?)', [ns, ref.owner + ':' + ref.kind + ':' + ref.id, JSON.stringify({ ref: ref, state: projection.state, revision: projection.revision, epoch: projection.epoch })]);
  }
  private async confirm(db: relationalStore.RdbStore, identity: SyncIdentity, proof: SyncProof): Promise<void> {
    const original = proof.original; const row = SyncRules.message(proof.result);
    if (row.senderId !== identity.user || original.sender !== identity.user || row.clientMsgId !== original.id || row.conversationId !== original.conversation || original.route !== '/conversations/' + encodeURIComponent(original.conversation) + '/message-results' || await this.originalHash(identity.namespace, original) !== original.hash) { throw new Error('SYNC_OUTBOX_RESULT_INVALID'); }
    const stored = await this.values(db, 'SELECT hash FROM sync_originals WHERE namespace=? AND id=?', [identity.namespace, original.id]);
    if (!stored.length || stored[0] !== original.hash) { throw new Error('SYNC_ORIGINAL_BODY_CHANGED'); }
    await db.executeSql("UPDATE sync_originals SET state='confirmed',serverid=? WHERE namespace=? AND id=? AND hash=?", [row.id, identity.namespace, original.id, original.hash]);
    await db.executeSql("DELETE FROM messages WHERE namespace=? AND id=? AND state IN('pending','frozen')", [identity.namespace, original.id]);
    if (row.deleted || await this.terminal(db, SyncRules.scope(identity), row.id)) { await this.global(db, identity, row.id); }
  }
  async syncConfirm(identity: SyncIdentity, guard: string, proof: SyncProof, current: () => boolean): Promise<void> {
    await this.transaction(async (db: relationalStore.RdbStore) => {
      const control = await this.matched(db, identity);
      if (!control || control.guard !== guard || !current()) { throw new Error('SYNC_LOCAL_GUARD_CHANGED'); }
      await this.confirm(db, identity, proof); control.guard = this.nonce(); await this.set(db, identity.namespace, 'sync:control', JSON.stringify(control));
      if (!current()) { throw new Error('SYNC_CONTEXT_CHANGED'); }
    });
  }
  async syncInstall(commit: SyncCommit, current: () => boolean): Promise<void> {
    await this.transaction(async (db: relationalStore.RdbStore) => {
      if (!current()) { throw new Error('SYNC_CONTEXT_CHANGED'); } await this.expected(db, commit.expected);
      const identity = commit.expected.identity;
      if (!SyncRules.same(JSON.stringify(commit.next.identity), JSON.stringify(identity)) || commit.next.physical !== commit.expected.physical || commit.next.registration !== commit.expected.registration || commit.next.stream !== commit.expected.stream || !SyncRules.uuid(commit.next.guard)) { throw new Error('SYNC_CONTEXT_CHANGED'); }
      if (SyncRules.compare(commit.next.position, commit.expected.position) < 0 || SyncRules.compare(commit.next.classification, commit.expected.classification) < 0) { throw new Error('SYNC_POSITION_REGRESSION'); }
      if (commit.manifest) {
        if (!SyncRules.same(commit.expected.manifest, commit.manifest.raw) || Date.now() >= Date.parse(commit.manifest.expiresAt)) { throw new Error('SYNC_SNAPSHOT_EXPIRED'); }
        const staged = await this.values(db, 'SELECT payload FROM sync_stages WHERE namespace=? AND snapshot=? ORDER BY page', [identity.namespace, commit.manifest.id]);
        if (staged.length !== commit.manifest.descriptors.length || staged.length !== commit.pages.length) { throw new Error('SYNC_STAGE_INCOMPLETE'); }
        for (let i = 0; i < staged.length; i++) { if (!SyncRules.same(staged[i], commit.pages[i].stage)) { throw new Error('SYNC_STAGE_CHANGED'); } }
        await this.archive(db, identity.namespace);
      }
      for (const projection of commit.projections.filter((row: SyncProjection) => row.state === 'global')) { await this.apply(db, identity, projection); }
      for (const projection of commit.projections.filter((row: SyncProjection) => row.state !== 'global')) { await this.apply(db, identity, projection, commit.manifest !== undefined); }
      for (const event of commit.events) {
        const prior = await this.values(db, 'SELECT payload FROM sync_events WHERE namespace=? AND id=?', [identity.namespace, event.origin]);
        if (prior.length && !SyncRules.same(prior[0], event.fact)) { throw new Error('SYNC_ORIGIN_CONFLICT'); }
        await db.executeSql('INSERT OR IGNORE INTO sync_events VALUES(?,?,?)', [identity.namespace, event.origin, event.fact]);
      }
      for (const proof of commit.proofs) { await this.confirm(db, identity, proof); }
      for (const raw of await this.values(db, "SELECT payload FROM messages WHERE namespace=? AND state='pending'", [identity.namespace])) {
        const row = JSON.parse(raw) as Message;
        const source = await this.values(db, "SELECT payload FROM sync_objects WHERE namespace=? AND id=?", [identity.namespace, 'conversation:conversation:' + row.conversationId]);
        const permitted = source.length > 0 && SyncJSON.parse(source[0]).get('sendAdmission').value === row.sendAdmission && SyncJSON.parse(source[0]).get('readOnly').value !== true;
        if (!permitted) {
          row.state = 'frozen'; row.freezeReason = 'SEND_ADMISSION_CHANGED';
          await db.executeSql("UPDATE messages SET state='frozen',payload=? WHERE namespace=? AND id=?", [JSON.stringify(row), identity.namespace, row.id]);
          await db.executeSql("UPDATE sync_originals SET state='frozen' WHERE namespace=? AND id=? AND state='pending'", [identity.namespace, row.clientMsgId]);
        }
      }
      await this.set(db, identity.namespace, 'sync:control', JSON.stringify(commit.next));
      await db.executeSql('DELETE FROM sync_stages WHERE namespace=?', [identity.namespace]);
      if (!current()) { throw new Error('SYNC_CONTEXT_CHANGED'); }
    });
  }
  async objects(identity: SyncIdentity, kind: string): Promise<string[]> {
    return this.store.syncAccess(async (db: relationalStore.RdbStore) => { const control = await this.matched(db, identity); if (!control || control.restriction) { throw new Error('SYNC_NOT_READY'); } return this.values(db, 'SELECT payload FROM sync_objects WHERE namespace=? AND kind=? ORDER BY id', [identity.namespace, kind]); });
  }
  async retainedConversations(identity: SyncIdentity): Promise<string[]> {
    return this.store.syncAccess(async (db: relationalStore.RdbStore) => {
      await this.matched(db, identity);
      return this.values(db, 'SELECT DISTINCT c.payload FROM sync_retained_conversations c JOIN sync_retained r ON r.namespace=c.namespace AND r.conversation=c.id JOIN sync_eligibility e ON e.namespace=r.namespace AND e.id=r.id LEFT JOIN sync_terminal t ON t.scope=? AND t.id=r.id WHERE c.namespace=? AND t.id IS NULL ORDER BY c.id', [SyncRules.scope(identity), identity.namespace]);
    });
  }
  async rememberSidecar(identity: SyncIdentity, guard: string, id: string, path: string): Promise<void> {
    await this.transaction(async (db: relationalStore.RdbStore) => {
      const control = await this.matched(db, identity);
      if (!control || control.guard !== guard || control.restriction || await this.terminal(db, SyncRules.scope(identity), id)) { throw new Error('SYNC_LOCAL_GUARD_CHANGED'); }
      await this.set(db, identity.namespace, 'media-sidecar:' + id, path);
    });
  }
  async readLocal(identity: SyncIdentity, conversation: string): Promise<void> { await this.transaction(async (db: relationalStore.RdbStore) => { await this.matched(db, identity); await db.executeSql('DELETE FROM sync_unread WHERE namespace=? AND conversation=?', [identity.namespace, conversation]); await db.executeSql('UPDATE mention_receipts SET unread=0 WHERE namespace=? AND conversation=?', [identity.namespace, conversation]); }); }
  async unread(identity: SyncIdentity, conversation: string): Promise<number> { return this.store.syncAccess(async (db: relationalStore.RdbStore) => { await this.matched(db, identity); return (await this.values(db, 'SELECT id FROM sync_unread WHERE namespace=? AND conversation=?', [identity.namespace, conversation])).length; }); }
  async retained(identity: SyncIdentity, conversation: string): Promise<Message[]> {
    return this.store.syncAccess(async (db: relationalStore.RdbStore) => {
      const control = await this.matched(db, identity);
      if (!control || ['authentication_device_changed', 'restore_epoch_changed', 'SYNC_AUTH_CONTEXT_CHANGED', 'RESTORE_EPOCH_CHANGED', 'SYNC_CONTEXT_CHANGED', 'STORE_GENERATION_CHANGED'].includes(control.restriction)) { throw new Error('SYNC_REVALIDATION_REQUIRED'); }
      const rows = await db.querySql('SELECT r.payload,e.version,e.hash FROM sync_retained r JOIN sync_eligibility e ON e.namespace=r.namespace AND e.id=r.id LEFT JOIN sync_terminal t ON t.scope=? AND t.id=r.id WHERE r.namespace=? AND r.conversation=? AND t.id IS NULL ORDER BY r.id', [SyncRules.scope(identity), identity.namespace, conversation]); const result: Message[] = [];
      try { while (rows.goToNextRow()) { const raw = rows.getString(0); const row = JSON.parse(raw) as Message; if (!row.forwarded && !row.deleted && !row.contentUnavailable && row.version === rows.getString(1) && await this.hash(raw) === rows.getString(2)) { result.push(row); } } } finally { rows.close(); } return result;
    });
  }
  async cleanups(): Promise<SyncCleanup[]> {
    return this.store.syncAccess(async (db: relationalStore.RdbStore) => { const rows = await db.querySql('SELECT namespace,key,path FROM sync_cleanup ORDER BY namespace,id,key'); const result: SyncCleanup[] = [];
      try { while (rows.goToNextRow()) { result.push({ namespace: rows.getString(0), key: rows.getString(1), path: rows.getString(2) }); } } finally { rows.close(); } return result;
    });
  }
  async cleaned(item: SyncCleanup): Promise<void> { await this.transaction(async (db: relationalStore.RdbStore) => { await db.executeSql('DELETE FROM sync_cleanup WHERE namespace=? AND key=? AND path=?', [item.namespace, item.key, item.path]); if (item.key !== '__notification__') { await this.set(db, item.namespace, item.key, ''); } }); }
  async ready(identity: SyncIdentity): Promise<string> { const control = await this.syncControl(identity); if (!control || control.restriction || !control.cursor) { throw new Error('SYNC_NOT_READY'); } return control.guard; }
  async canShow(identity: SyncIdentity, guard: string, id: string): Promise<boolean> { return this.store.syncAccess(async (db: relationalStore.RdbStore) => { const control = await this.matched(db, identity); return !!control && !control.restriction && control.guard === guard && !(await this.terminal(db, SyncRules.scope(identity), id)); }); }
}

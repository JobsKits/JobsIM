import { rcp } from '@kit.RemoteCommunicationKit';
import fs from '@ohos.file.fs';
import { MediaResult, DownloadResult, AvatarOperation, AuthHooks, ApiFailure } from './Models';
import { ExportRules, ExportPrepared } from './ExportProtocol';
import { ExportSink } from './ExportFiles';
interface ServerError { code: string; }
interface ErrorResponse { error: ServerError; }
export class Api {
  private base: string;
  private token: string;
  private caPath: string;
  private hooks?: AuthHooks;
  private boundStamp: string = '';
  constructor(base: string, token: string = '', caPath: string = '', hooks?: AuthHooks) {
    this.base = base;
    this.token = token;
    this.caPath = caPath;
    this.hooks = hooks; this.boundStamp = hooks ? hooks.stamp() : '';
  }
  private session(timeout: number): rcp.Session {
    const security: rcp.SecurityConfiguration = this.caPath.length > 0 ? { remoteValidation: { filePath: this.caPath } } : { remoteValidation: 'system' };
    return rcp.createSession({ requestConfiguration: { security: security, transfer: { autoRedirect: false, timeout: { connectMs: 3500, transferMs: timeout } } } });
  }
  private decode<T>(response: rcp.Response): T {
    const value = response.toString() ?? '{}';
    if (value.length > 2097152) { throw new Error('Response limit'); }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      let code = '';
      try { code = (JSON.parse(value) as ErrorResponse).error.code; } catch (_) {}
      throw new ApiFailure(response.statusCode, code || 'HTTP_ERROR');
    }
    try { return JSON.parse(value.length > 0 ? value : '{}') as T; }
    catch (_error) { throw new ApiFailure(0, 'INVALID_RESPONSE'); }
  }
  private check(): void {
    if (this.hooks && this.boundStamp !== this.hooks.stamp()) { throw new Error('SESSION_SCOPE_CHANGED'); }
  }
  private async authorized<T>(operation: (token: string) => Promise<T>): Promise<T> {
    const hooks = this.hooks;
    if (!hooks) { return operation(this.token); }
    const stamp = this.boundStamp;
    let token = await hooks.before(stamp); this.check();
    try {
      const result = await operation(token); this.check(); return result;
    } catch (error) {
      this.check();
      if (!(error instanceof ApiFailure) || error.status !== 401) { throw error as Error; }
      if (error.code === 'ACCESS_EXPIRED') { token = await hooks.refresh(token, stamp); }
      else {
        const current = await hooks.before(stamp);
        if (current === token) { await hooks.rejected(token, stamp); throw error as Error; }
        token = current;
      }
      this.check();
      try { const result = await operation(token); this.check(); return result; }
      catch (second) {
        if (second instanceof ApiFailure && second.status === 401) { await hooks.rejected(token, stamp); }
        throw second as Error;
      }
    }
  }
  async request<T>(path: string, method: rcp.HttpMethod = 'GET', body: string = ''): Promise<T> {
    const publicPath = ['/bootstrap', '/auth/register', '/auth/login', '/auth/refresh', '/auth/reauthenticate', '/auth/recover'].includes(path);
    if (publicPath) { return this.requestRaw<T>(path, method, body, ''); }
    return this.authorized<T>((token: string) => this.requestRaw<T>(path, method, body, token));
  }
  private async requestRaw<T>(path: string, method: rcp.HttpMethod, body: string, token: string): Promise<T> {
    const session = this.session(5000);
    try {
      const headers: rcp.RequestHeaders = { 'Accept': 'application/json', 'Content-Type': 'application/json' };
      if (token.length > 0) { headers['Authorization'] = 'Bearer ' + token; }
      const request = new rcp.Request(this.base + '/api' + path, method, headers, body.length > 0 ? body : undefined);
      return this.decode<T>(await session.fetch(request));
    } catch (error) { if (error instanceof ApiFailure) { throw error; } throw new ApiFailure(0, 'TRANSPORT_UNKNOWN'); } finally { session.close(); }
  }
  async migration<T>(path: string, method: rcp.HttpMethod, body: string, store: string, current: () => boolean = () => true): Promise<T> {
    if (!/^[0-9a-f-]{36}$/.test(store) || !/^\/sync\/(migration-capabilities|migration-pairings|migration-pairing-results|migration-results|migrations(?:\/[0-9a-f-]{36}(?:\/(?:key|validate|installed|cancel))?)?)$/.test(path) || body.length > 128*1024*1024) { throw new Error('MIGRATION_INVALID'); }
    return this.authorized<T>(async (token: string) => {
      const session = this.session(120000);const timer=setInterval(()=>{if(!current()){session.close();}},100);
      try { if(!current()){throw new Error('MIGRATION_CONTEXT_CHANGED');}const request = new rcp.Request(this.base + '/api' + path,method,{ 'Authorization':'Bearer '+token,'X-JobsIM-Store-Generation':store,'Content-Type':'application/json','Accept':'application/json','Cache-Control':'no-store' },body || undefined); return this.decode<T>(await session.fetch(request)); }
      catch (error) { if (error instanceof ApiFailure) { throw error; } throw new ApiFailure(0,'MIGRATION_RESULT_UNKNOWN'); } finally { clearInterval(timer);session.close(); }
    });
  }
  async receipt<T>(path: string, method: rcp.HttpMethod, body: string, store: string, current: () => boolean = () => true): Promise<T> {
    if (!/^[0-9a-f-]{36}$/.test(store) || !/^\/sync\/(receipt-context(?:\?schemaVersion=[12])?|receipt-profiles\?schemaVersion=[12]&buildDigest=[0-9a-f]{64}|receipt-targets\/[0-9a-f-]{36}(?:\?schemaVersion=[12](?:&qualificationId=[0-9a-f-]{36})?)?|storage(?:-results)?|receipt-challenges|receipts|receipt-results|object-receipt-challenges|object-receipts|object-receipt-results)$/.test(path) || body.length > 32*1024) { throw new Error('RECEIPT_INVALID'); }
    return this.authorized<T>(async (token: string) => {
      const session = this.session(120000);const timer=setInterval(()=>{if(!current()){session.close();}},100);
      try { if(!current()){throw new Error('RECEIPT_CONTEXT_CHANGED');}const request = new rcp.Request(this.base + '/api' + path,method,{ 'Authorization':'Bearer '+token,'X-JobsIM-Store-Generation':store,'Content-Type':'application/json','Accept':'application/json','Cache-Control':'no-store' },body || undefined); return this.decode<T>(await session.fetch(request)); }
      catch (error) { if (error instanceof ApiFailure) { throw error; } throw new ApiFailure(0,'RECEIPT_RESULT_UNKNOWN'); } finally { clearInterval(timer);session.close(); }
    });
  }
  async migrationBytes(path: string, store: string, limit: number, current: () => boolean = () => true): Promise<Uint8Array> {
    if (!/^[0-9a-f-]{36}$/.test(store) || !/^\/(sync\/migrations\/[0-9a-f-]{36}\/capsule|media\/[0-9a-f-]{36}|messages\/[0-9a-f-]{36}\/attachment)$/.test(path) || !Number.isSafeInteger(limit) || limit < 1 || limit > 128*1024*1024+4096) { throw new Error('MIGRATION_INVALID'); }
    return this.authorized<Uint8Array>(async (token: string) => {
      const session = this.session(120000);const timer=setInterval(()=>{if(!current()){session.close();}},100);
      try {
        if(!current()){throw new Error('MIGRATION_CONTEXT_CHANGED');}const request = new rcp.Request(this.base + '/api' + path,'GET',{ 'Authorization':'Bearer '+token,'X-JobsIM-Store-Generation':store,'Cache-Control':'no-store' });
        request.configuration = { tracing: { httpEventsHandler: { onHeaderReceive: (headers: rcp.ResponseHeaders) => { const length=headers['content-length'];if(typeof length==='string' && /^\d+$/.test(length) && Number(length)>limit){session.close();throw new Error('MIGRATION_FILE_LIMIT');} } } } };
        const response=await session.fetch(request);if(response.statusCode!==200){this.decode<Object>(response);}const bytes=response.body??(response.headers['content-length']==='0'?new ArrayBuffer(0):undefined);if(!bytes || bytes.byteLength>limit){throw new Error('MIGRATION_FILE_LIMIT');}const length=response.headers['content-length'];if(length!==undefined&&(typeof length!=='string'||!/^\d+$/.test(length)||Number(length)!==bytes.byteLength)){throw new Error('MIGRATION_FILE_INCOMPLETE');}if(!current()){new Uint8Array(bytes).fill(0);throw new Error('MIGRATION_CONTEXT_CHANGED');}return new Uint8Array(bytes);
      }catch(error){if(error instanceof ApiFailure){throw error;}throw new ApiFailure(0,'MIGRATION_RESULT_UNKNOWN');}finally{clearInterval(timer);session.close();}
    });
  }
  async upload(uri: string): Promise<MediaResult> {
    return this.authorized<MediaResult>((token: string) => this.uploadRaw(uri, token));
  }
  private async uploadRaw(uri: string, token: string): Promise<MediaResult> {
    const file = fs.openSync(uri, fs.OpenMode.READ_ONLY);
    let buffer: ArrayBuffer;
    try {
      const size = fs.statSync(file.fd).size;
      if (size > 25 * 1024 * 1024) { throw new Error('Attachment limit: 25 MiB'); }
      buffer = new ArrayBuffer(size);
      const read = fs.readSync(file.fd, buffer);
      if (read !== size) { throw new Error('Incomplete attachment'); }
    } finally { fs.closeSync(file); }
    const session = this.session(30000);
    try {
      const fields: rcp.MultipartFormFields = { file: { contentType: 'application/octet-stream', remoteFileName: 'attachment', contentOrPath: { content: buffer } } };
      const request = new rcp.Request(this.base + '/api/media', 'POST', { 'Authorization': 'Bearer ' + token }, new rcp.MultipartForm(fields));
      return this.decode<MediaResult>(await session.fetch(request));
    } catch (error) { if (error instanceof ApiFailure) { throw error; } throw new ApiFailure(0, 'TRANSPORT_UNKNOWN'); } finally { session.close(); }
  }
  async download(path: string): Promise<DownloadResult> {
    return this.authorized<DownloadResult>((token: string) => this.downloadRaw(path, token));
  }
  private async downloadRaw(path: string, token: string): Promise<DownloadResult> {
    if (!/^\/(media|avatars)\/[A-Za-z0-9_-]+$/.test(path) && !/^\/messages\/[A-Za-z0-9_-]+\/attachment$/.test(path)) { throw new Error('Invalid private resource path'); }
    const session = this.session(30000);
    try {
      const response = await session.fetch(new rcp.Request(this.base + '/api' + path, 'GET', { Authorization: 'Bearer ' + token }));
      if (response.statusCode < 200 || response.statusCode >= 300) { this.decode<Object>(response); }
      const data = response.body;
      if (!data || data.byteLength > 25 * 1024 * 1024) { throw new Error('Attachment size limit'); }
      const mime = response.headers['content-type'];
      return { data: data, contentType: typeof mime === 'string' ? mime : 'application/octet-stream' };
    } catch (error) { if (error instanceof ApiFailure) { throw error; } throw new ApiFailure(0, 'TRANSPORT_UNKNOWN'); } finally { session.close(); }
  }
  async dataExport(job: string, ticket: string, sink: ExportSink, current: () => boolean): Promise<ExportPrepared> {
    if (!/^[0-9a-f-]{36}$/.test(job) || !/^[A-Za-z0-9_-]{16,256}$/.test(ticket)) { ExportRules.fail('EXPORT_FILE_INVALID'); }
    try {
      return await this.authorized<ExportPrepared>(async (token: string) => {
        if (!current()) { ExportRules.fail('EXPORT_CONTEXT_CHANGED'); }
        sink.reset(); const session = this.session(120000); let stopped = '';
        const timer = setInterval(() => { if (!current()) { stopped = 'EXPORT_CONTEXT_CHANGED'; session.close(); } }, 100);
        try {
          const request = new rcp.Request(this.base + '/api/me/data-exports/' + job + '/download', 'GET', { 'Authorization': 'Bearer ' + token, 'X-JobsIM-Download-Token': ticket, 'Accept': 'application/zip', 'Cache-Control': 'no-store' });
          request.destination = (bytes: ArrayBuffer) => { try { if (!current()) { ExportRules.fail('EXPORT_CONTEXT_CHANGED'); } sink.write(bytes); } catch (error) { stopped = (error as Error).message; session.close(); throw error as Error; } };
          request.configuration = { tracing: { httpEventsHandler: { onHeaderReceive: (headers: rcp.ResponseHeaders) => { const value = headers['content-length']; if (typeof value === 'string' && /^\d+$/.test(value) && Number(value) > ExportRules.limit) { stopped = 'EXPORT_FILE_LIMIT'; session.close(); } } } } };
          const response = await session.fetch(request);
          if (stopped) { throw new Error(stopped); }
          if (response.statusCode !== 200) { let code = 'EXPORT_DOWNLOAD_FAILED'; try { code = (JSON.parse(sink.errorText()) as ErrorResponse).error.code; } catch (_) {} throw new ApiFailure(response.statusCode, code); }
          const mime = response.headers['content-type']; const disposition = response.headers['content-disposition']; const length = response.headers['content-length'];
          if (typeof mime !== 'string' || mime.split(';')[0].trim().toLowerCase() !== 'application/zip' || typeof disposition !== 'string' || length !== undefined && (typeof length !== 'string' || !/^\d+$/.test(length))) { ExportRules.fail('EXPORT_FILE_INVALID'); }
          if (!current()) { ExportRules.fail('EXPORT_CONTEXT_CHANGED'); }
          return await sink.finish(job, ExportRules.fileName(disposition), typeof length === 'string' ? Number(length) : -1);
        } catch (error) { if (stopped) { throw new Error(stopped); } throw error as Error; }
        finally { clearInterval(timer); session.close(); }
      });
    } catch (error) { sink.dispose(); throw error as Error; }
  }
  async avatar(path: string, requestId: string, expectedVersion: string): Promise<AvatarOperation> {
    return this.authorized<AvatarOperation>((token: string) => this.avatarRaw(path, requestId, expectedVersion, token));
  }
  private async avatarRaw(path: string, requestId: string, expectedVersion: string, token: string): Promise<AvatarOperation> {
    const file = fs.openSync(path, fs.OpenMode.READ_ONLY);
    let buffer: ArrayBuffer;
    try {
      const size = fs.statSync(file.fd).size;
      if (size > 2 * 1024 * 1024) { throw new Error('Avatar limit: 2 MiB'); }
      buffer = new ArrayBuffer(size);
      if (fs.readSync(file.fd, buffer) !== size) { throw new Error('Incomplete avatar'); }
    } finally { fs.closeSync(file); }
    const session = this.session(30000);
    try {
      const fields: rcp.MultipartFormFields = { file: { remoteFileName: 'avatar', contentType: 'application/octet-stream', contentOrPath: { content: buffer } }, requestId: requestId, expectedVersion: expectedVersion };
      return this.decode<AvatarOperation>(await session.fetch(new rcp.Request(this.base + '/api/me/avatar', 'POST', { Authorization: 'Bearer ' + token }, new rcp.MultipartForm(fields))));
    } catch (error) { if (error instanceof ApiFailure) { throw error; } throw new ApiFailure(0, 'TRANSPORT_UNKNOWN'); } finally { session.close(); }
  }

}

// Created by Jobs on 2026年11月16日，星期一.
import fs from '@ohos.file.fs';
import util from '@ohos.util';
import common from '@ohos.app.ability.common';
import picker from '@ohos.file.picker';
import cryptoFramework from '@ohos.security.cryptoFramework';
import { ExportRules, ExportPrepared } from './ExportProtocol';

export interface ExportSink {
  reset: () => void; write: (bytes: ArrayBuffer) => void; errorText: () => string;
  finish: (job: string, name: string, length: number) => Promise<ExportPrepared>;
  dispose: () => void;
}
export class ExportFiles implements ExportSink {
  private static active: Set<string> = new Set();
  private directory: string; private path: string; private file?: fs.File; private count: number = 0; private current: () => boolean;
  constructor(context: common.UIAbilityContext, current: () => boolean, namespace: string = '') {
    this.current = current; const root = context.cacheDir + '/personal-exports';
    if (!fs.accessSync(root)) { fs.mkdirSync(root); }
    this.directory = namespace ? root + '/' + encodeURIComponent(namespace) : root;
    if (!fs.accessSync(this.directory)) { fs.mkdirSync(this.directory); }
    this.path = this.directory + '/' + util.generateRandomUUID(true) + '.part'; ExportFiles.active.add(this.path);
  }
  static cleanup(context: common.UIAbilityContext): void {
    const directory = context.cacheDir + '/personal-exports'; if (!fs.accessSync(directory)) { return; }
    for (const name of fs.listFileSync(directory)) {
      const path = directory + '/' + name;
      if (/^[0-9a-f-]{36}\.(part|zip)$/.test(name) && !ExportFiles.active.has(path)) { fs.unlinkSync(path); }
    }
  }
  private check(): void { if (!this.current()) { ExportRules.fail('EXPORT_CONTEXT_CHANGED'); } }
  reset(): void {
    this.check(); if (this.file) { fs.closeSync(this.file); this.file = undefined; }
    this.file = fs.openSync(this.path, fs.OpenMode.CREATE | fs.OpenMode.READ_WRITE | fs.OpenMode.TRUNC); this.count = 0;
  }
  write(bytes: ArrayBuffer): void {
    this.check(); if (!this.file) { ExportRules.fail('EXPORT_FILE_INVALID'); }
    if (this.count + bytes.byteLength > ExportRules.limit) { ExportRules.fail('EXPORT_FILE_LIMIT'); }
    let offset = 0;
    while (offset < bytes.byteLength) { this.check(); const written = fs.writeSync(this.file.fd, bytes.slice(offset)); if (written <= 0) { ExportRules.fail('EXPORT_FILE_INCOMPLETE'); } offset += written; }
    this.count += bytes.byteLength;
  }
  private read(offset: number, length: number): Uint8Array {
    if (!this.file || offset < 0 || length < 0 || offset + length > this.count) { ExportRules.fail('EXPORT_FILE_INVALID'); }
    const buffer = new ArrayBuffer(length); fs.lseek(this.file.fd, offset, fs.WhenceType.SEEK_SET);
    if (fs.readSync(this.file.fd, buffer) !== length) { ExportRules.fail('EXPORT_FILE_INCOMPLETE'); }
    return new Uint8Array(buffer);
  }
  errorText(): string { if (this.count > 2097152) { return ''; } try { return util.TextDecoder.create('utf-8', { fatal: true }).decodeToString(this.read(0, this.count)); } catch (_) { return ''; } }
  private fsyncDirectory(): void { const directory = fs.openSync(this.directory, fs.OpenMode.READ_ONLY); try { fs.fsyncSync(directory.fd); } finally { fs.closeSync(directory); } }
  private async digest(): Promise<string> {
    const md = cryptoFramework.createMd('SHA256'); let offset = 0;
    while (offset < this.count) { this.check(); const chunk = this.read(offset, Math.min(65536, this.count - offset)); await md.update({ data: chunk }); offset += chunk.byteLength; }
    return Array.from((await md.digest()).data).map((byte: number) => byte.toString(16).padStart(2, '0')).join('');
  }
  async finish(job: string, name: string, length: number): Promise<ExportPrepared> {
    this.check(); if (!this.file || length >= 0 && length !== this.count) { ExportRules.fail('EXPORT_FILE_INCOMPLETE'); }
    ExportRules.zip(this.count, (offset: number, size: number) => this.read(offset, size));
    const digest = await this.digest(); this.check(); fs.fsyncSync(this.file.fd); fs.closeSync(this.file); this.file = undefined;
    const destination = this.path.replace(/\.part$/, '.zip'); fs.renameSync(this.path, destination); ExportFiles.active.delete(this.path); this.path = destination; ExportFiles.active.add(this.path); this.fsyncDirectory();
    return { path: this.path, name: name, size: this.count, digest: digest, job: job };
  }
  async save(context: common.UIAbilityContext, prepared: ExportPrepared, authorize: () => Promise<void>): Promise<boolean> {
    this.check(); if (prepared.path !== this.path || !this.path.endsWith('.zip')) { ExportRules.fail('EXPORT_FILE_INVALID'); }
    const selector = new picker.DocumentViewPicker(context);
    const options = new picker.DocumentSaveOptions(); options.newFileNames = [prepared.name]; options.fileSuffixChoices = ['ZIP|.zip'];
    const destinations = await selector.save(options);
    if (!destinations.length) { return false; }
    if (destinations.length !== 1 || !destinations[0].startsWith('file://')) { ExportRules.fail('EXPORT_SAVE_FAILED'); }
    try {
      this.check(); await authorize(); this.check();
      this.file = fs.openSync(this.path, fs.OpenMode.READ_ONLY);
      this.count = fs.statSync(this.file.fd).size;
      if (this.count !== prepared.size || await this.digest() !== prepared.digest) { ExportRules.fail('EXPORT_FILE_INVALID'); }
      ExportRules.zip(this.count, (offset: number, size: number) => this.read(offset, size));
      const output = fs.openSync(destinations[0], fs.OpenMode.WRITE_ONLY | fs.OpenMode.TRUNC);
      try {
        let offset = 0;
        while (offset < this.count) { this.check(); const bytes = this.read(offset, Math.min(65536, this.count - offset)); let written = 0; while (written < bytes.byteLength) { this.check(); const count = fs.writeSync(output.fd, bytes.buffer.slice(written)); if (count <= 0) { ExportRules.fail('EXPORT_SAVE_FAILED'); } written += count; } offset += bytes.byteLength; }
        this.check(); fs.fsyncSync(output.fd); this.check();
      } finally { fs.closeSync(output); }
      return true;
    } catch (_) {
      // The system provider may retain a partial new document. Never call it saved.
      try { fs.unlinkSync(destinations[0]); } catch (_) {}
      throw new Error('EXPORT_SAVE_FAILED');
    } finally { if (this.file) { fs.closeSync(this.file); this.file = undefined; } }
  }
  dispose(): void { if (this.file) { fs.closeSync(this.file); this.file = undefined; } if (fs.accessSync(this.path)) { fs.unlinkSync(this.path); } ExportFiles.active.delete(this.path); }
}
